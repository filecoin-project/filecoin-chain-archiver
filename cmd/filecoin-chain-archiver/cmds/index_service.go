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

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/filecoin-chain-archiver/build"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/config"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/index/service"
)

var cmdIndexService = &cli.Command{
	Name:        "index-resolver-service",
	Usage:       "Commands for the index resolver service",
	Description: "The index resolver service provides a way to resolve snapshots",
	Flags:       []cli.Flag{},
	Subcommands: []*cli.Command{
		{
			Name:        "default-config",
			Usage:       "prints the default configuration",
			Description: TrimDescription(``),
			Flags:       []cli.Flag{},
			Action: func(cctx *cli.Context) error {
				var icfg interface{}

				cfg := config.DefaultIndexServiceConfig()
				icfg = cfg

				bs, err := config.ConfigComment(icfg)
				if err != nil {
					return err
				}

				fmt.Printf("%s", string(bs))

				return nil
			},
		},
		{
			Name:  "run",
			Usage: "start the service",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "service-listen",
					Usage:   "host and port to listen on",
					EnvVars: []string{"FCA_INDEX_RESOLVER_SERVICE_LISTEN"},
					Value:   "localhost:5200",
				},
				&cli.StringFlag{
					Name:    "operator-listen",
					Usage:   "host and port to listen on",
					EnvVars: []string{"FCA_INDEX_RESOLVER_OPERATOR_LISTEN"},
					Value:   "localhost:5201",
				},
				&cli.StringFlag{
					Name:    "config-path",
					Usage:   "path to configuration file",
					EnvVars: []string{"FCA_INDEX_RESOLVER_CONFIG_PATH"},
					Value:   "./config.toml",
				},
			},
			Action: func(cctx *cli.Context) error {
				ctx, cancelFunc := context.WithCancel(context.Background())
				ctx = context.WithValue(ctx, versionKey{}, build.Version())

				signalChan := make(chan os.Signal, 1)
				signal.Notify(signalChan, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)

				s := service.NewIndexService(ctx)

				if err := s.SetupService(cctx.String("config-path")); err != nil {
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
