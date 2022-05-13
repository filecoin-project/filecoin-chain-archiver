package cmds

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/cli/util"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/filecoin-chain-archiver/build"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/nodelocker/api"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/nodelocker/api/apiclient"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/nodelocker/service"
)

var (
	routeTimeout       = 30 * time.Second
	svrShutdownTimeout = 1 * time.Second
	ctxCancelWait      = 1 * time.Second
)

type versionKey struct{}

var cmdService = &cli.Command{
	Name:        "nodelocker",
	Usage:       "Commands for the nodelocker service",
	Description: "Description",
	Flags:       []cli.Flag{},
	Subcommands: []*cli.Command{
		{
			Name:  "operator",
			Usage: "commands for interacting with the running service through the operator jsonrpc api",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "operator-api",
					Usage:   "host and port of operator api",
					EnvVars: []string{"FCA_NODELOCKER_OPERATOR_API"},
					Value:   "http://localhost:5101",
				},
				&cli.StringFlag{
					Name:    "api-info",
					EnvVars: []string{"FCA_NODELOCKER_OPERATOR_API_INFO"},
					Hidden:  true,
				},
			},
			Before: func(cctx *cli.Context) error {
				if cctx.IsSet("api-info") {
					return nil
				}

				apiInfo := fmt.Sprintf("%s", cctx.String("operator-api"))
				return cctx.Set("api-info", apiInfo)
			},
			Subcommands: []*cli.Command{
				{
					Name:  "list",
					Usage: "list current locks",
					Flags: []cli.Flag{},
					Action: func(cctx *cli.Context) error {
						ctx := context.Background()

						api, closer, err := getCliClient(ctx, cctx)
						defer closer()
						if err != nil {
							return err
						}

						locks, err := api.FetchLocks(ctx)
						if err != nil {
							return err
						}

						for i, lock := range locks {
							fmt.Printf("%d\t%s\t%s\n", i, lock.PeerID, lock.Expiry)
						}

						return nil
					},
				},
				{
					Name:  "lock",
					Usage: "list current locks",
					Flags: []cli.Flag{},
					Action: func(cctx *cli.Context) error {
						ctx := context.Background()

						api, closer, err := getCliClient(ctx, cctx)
						defer closer()
						if err != nil {
							return err
						}

						fmt.Println(cctx.Args().Get(0))
						fmt.Println(cctx.Args().Get(1))

						lock, err := api.Lock(ctx, cctx.Args().Get(0), cctx.Args().Get(1))
						if err != nil {
							return err
						}

						fmt.Printf("%s, %T, %s", lock.PeerID, lock.Aquired, lock.Expiry)

						return nil
					},
				},
				{
					Name:  "version",
					Usage: "prints local and remote version",
					Action: func(cctx *cli.Context) error {
						ctx := context.Background()

						api, closer, err := getCliClient(ctx, cctx)
						defer closer()
						if err != nil {
							return err
						}

						version, err := api.Version(ctx)
						if err != nil {
							return err
						}

						fmt.Printf("local:  %s\n", build.Version())
						fmt.Printf("remote: %s\n", version)

						return nil
					},
				},
				{
					Name:  "log-list",
					Usage: "list available loggers",
					Action: func(cctx *cli.Context) error {
						ctx := context.Background()

						api, closer, err := getCliClient(ctx, cctx)
						defer closer()
						if err != nil {
							return err
						}

						loggers, err := api.LogList(ctx)
						if err != nil {
							return err
						}

						for _, logger := range loggers {
							fmt.Println(logger)
						}

						return nil
					},
				},
				{
					Name:      "log-set-level",
					Usage:     "set log level",
					ArgsUsage: "<level>",
					Description: TrimDescription(`
						The logger flag can be specified multiple times.
						eg) log set-level --logger foo --logger bar debug
					`),
					Flags: []cli.Flag{
						&cli.StringSliceFlag{
							Name:  "logger",
							Usage: "limit to log system",
							Value: &cli.StringSlice{},
						},
					},
					Action: func(cctx *cli.Context) error {
						ctx := context.Background()

						api, closer, err := getCliClient(ctx, cctx)
						defer closer()
						if err != nil {
							return err
						}

						if !cctx.Args().Present() {
							return fmt.Errorf("level is required")
						}

						loggers := cctx.StringSlice("logger")
						if len(loggers) == 0 {
							var err error
							loggers, err = api.LogList(ctx)
							if err != nil {
								return err
							}
						}

						for _, logger := range loggers {
							if err := api.LogSetLevel(ctx, logger, cctx.Args().First()); err != nil {
								return xerrors.Errorf("setting log level on %s: %w", logger, err)
							}
						}

						return nil
					},
				},
			},
		},
		{
			Name:  "run",
			Usage: "start the service",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "service-listen",
					Usage:   "host and port to listen on",
					EnvVars: []string{"FCA_NODELOCKER_SERVICE_LISTEN"},
					Value:   "localhost:5100",
				},
				&cli.StringFlag{
					Name:    "operator-listen",
					Usage:   "host and port to listen on",
					EnvVars: []string{"FCA_NODELOCKER_OPERATOR_LISTEN"},
					Value:   "localhost:5101",
				},
			},
			Action: func(cctx *cli.Context) error {
				ctx, cancelFunc := context.WithCancel(context.Background())
				ctx = context.WithValue(ctx, versionKey{}, build.Version())

				signalChan := make(chan os.Signal, 1)
				signal.Notify(signalChan, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)

				s := service.NewLockerService(ctx)

				if err := s.SetupService(); err != nil {
					return err
				}

				svr := &http.Server{
					Addr:    cctx.String("service-listen"),
					Handler: s.ServiceRouter,
					BaseContext: func(listener net.Listener) context.Context {
						return context.Background()
					},
				}

				go func() {
					logger.Debugw("service running")
					err := svr.ListenAndServe()
					switch err {
					case nil:
					case http.ErrServerClosed:
						logger.Infow("server closed")
					case context.Canceled:
						logger.Infow("context cancled")
					default:
						logger.Errorw("error shutting down service server", "err", err)
					}
				}()

				if err := s.SetupOperator(); err != nil {
					return err
				}

				osvr := http.Server{
					Addr:    cctx.String("operator-listen"),
					Handler: s.OperatorRouter,
					BaseContext: func(listener net.Listener) context.Context {
						return context.Background()
					},
				}

				go func() {
					logger.Debugw("operator running")
					err := osvr.ListenAndServe()
					switch err {
					case nil:
					case http.ErrServerClosed:
						logger.Infow("server closed")
					case context.Canceled:
						logger.Infow("context cancled")
					default:
						logger.Errorw("error shutting down internal server", "err", err)
					}
				}()

				logger.Infow("waiting for signal")
				<-signalChan
				s.Shutdown()

				t := time.NewTimer(svrShutdownTimeout)

				shutdownChan := make(chan error)
				go func() {
					shutdownChan <- svr.Shutdown(ctx)
				}()

				select {
				case err := <-shutdownChan:
					if err != nil {
						logger.Errorw("shutdown finished with an error", "err", err)
					} else {
						logger.Infow("shutdown finished successfully")
					}
				case <-t.C:
					logger.Warnw("shutdown timed out")
				}

				cancelFunc()
				time.Sleep(ctxCancelWait)

				logger.Infow("closing down database connections")
				s.Close()

				if err := osvr.Shutdown(ctx); err != nil {
					switch err {
					case nil:
					case http.ErrServerClosed:
						logger.Infow("server closed")
					case context.Canceled:
						logger.Infow("context cancled")
					default:
						logger.Errorw("error shutting down operator server", "err", err)
					}
				}

				logger.Infow("existing")

				return nil
			},
		},
	},
}

func getCliClient(ctx context.Context, cctx *cli.Context) (api.Operator, jsonrpc.ClientCloser, error) {
	ai := cliutil.ParseApiInfo(cctx.String("api-info"))
	url, err := ai.DialArgs("v0")
	if err != nil {
		return nil, func() {}, err
	}

	return apiclient.NewOperatorClient(ctx, url, ai.AuthHeader())
}
