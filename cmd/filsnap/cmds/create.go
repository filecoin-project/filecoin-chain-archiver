package cmds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"syscall"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/travisperson/filsnap/pkg/config"
	"github.com/travisperson/filsnap/pkg/consensus"
	"github.com/travisperson/filsnap/pkg/export"
	"github.com/travisperson/filsnap/pkg/nodelocker/client"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
)

var cmdCreate = &cli.Command{
	Name:  "create",
	Usage: "create a chain export",
	Description: TrimDescription(`
		Creating a snapshot can be configured in a few ways. The primary configuration is to use an epoch interval
		to calculate the appropiate epoch height.

		The epoch height is calculated by computing the current expected height, and finding the next interval that
		occurs after it, offset by the confidence. The current expected height is calculated using the current time,
		and the genesis timestamp.

		Eg: interval=100; confidence=15;

		            /- 500
		|----------|----------|----------|----------|
	           |----------|
		485 - /            \ - 585

		The calculation for the current expected height can be by-passed by using the 'after' flag. When set, the interval
		that occurs after the 'after' flag will be used for the epoch height.

		An exact epoch height can also be supplied with the 'height' flag.
	`),
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "nodelocker-api",
			Usage:   "host and port of nodelocker api",
			Value:   "http://127.0.0.1:5100",
			EnvVars: []string{"FILSNAP_CREATE_NODELOCKER_API"},
		},
		&cli.StringFlag{
			Name:    "bucket",
			Usage:   "bucket name for export upload",
			EnvVars: []string{"FILSNAP_CREATE_BUCKET"},
		},
		&cli.StringFlag{
			Name:    "bucket-endpoint",
			Usage:   "bucket host and port for upload",
			EnvVars: []string{"FILSNAP_CREATE_BUCKET_ENDPOINT"},
		},
		&cli.StringFlag{
			Name:    "access-key",
			Usage:   "access key for upload",
			EnvVars: []string{"FILSNAP_CREATE_ACCESS_KEY"},
		},
		&cli.StringFlag{
			Name:    "secret-key",
			Usage:   "secret key for upload",
			EnvVars: []string{"FILSNAP_CREATE_SECRET_KEY"},
		},
		&cli.BoolFlag{
			Name:    "discard",
			Usage:   "discard output, do not upload",
			EnvVars: []string{"FILSNAP_CREATE_DISCARD"},
			Value:   false,
		},
		&cli.StringFlag{
			Name:    "config-path",
			Usage:   "path to configuration file",
			EnvVars: []string{"FILSNAP_CONFIG_PATH"},
			Value:   "./config.toml",
		},
		&cli.IntFlag{
			Name:    "interval",
			Usage:   "interval used to determine next export height",
			EnvVars: []string{"FILSNAP_CREATE_INTERVAL"},
			Value:   120,
		},
		&cli.IntFlag{
			Name:    "confidence",
			Usage:   "number of tipsets that should exist after the determine export height",
			EnvVars: []string{"FILSNAP_CREATE_CONFIDENCE"},
			Value:   15,
		},
		&cli.IntFlag{
			Name:    "after",
			Usage:   "use interval height after this height",
			EnvVars: []string{"FILSNAP_CREATE_AFTER"},
			Value:   0,
		},
		&cli.IntFlag{
			Name:    "height",
			Usage:   "create a snapshot from the given height",
			EnvVars: []string{"FILSNAP_CREATE_HEIGHT"},
			Value:   0,
		},
		&cli.IntFlag{
			Name:    "stateroot-count",
			Usage:   "number of stateroots to included in snapshot",
			EnvVars: []string{"FILSNAP_CREATE_STATEROOT_COUNT"},
			Value:   2000,
		},
		&cli.DurationFlag{
			Name:    "progress-update",
			Usage:   "how frequenty to provide provide update logs",
			EnvVars: []string{"FILSNAP_CREATE_PROGRESS_UPDATE"},
			Value:   60 * time.Second,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()

		flagBucketEndpoint := cctx.String("bucket-endpoint")
		flagBucketAccessKey := cctx.String("access-key")
		flagBucketSecretKey := cctx.String("secret-key")
		flagBucket := cctx.String("bucket")
		flagDiscard := cctx.Bool("discard")
		flagProgressUpdate := cctx.Duration("progress-update")
		flagNodeLockerAPI := cctx.String("nodelocker-api")
		flagConfigPath := cctx.String("config-path")
		flagInterval := cctx.Int("interval")
		flagConfidence := cctx.Int("confidence")
		flagHeight := cctx.Int("height")
		flagAfter := cctx.Int("after")
		flagStaterootCount := cctx.Int("stateroot-count")

		icfg, err := config.FromFile(flagConfigPath, &config.Config{})
		if err != nil {
			return err
		}

		cfg := icfg.(*config.Config)

		addrs, err := NodeMultiaddrs(cfg)
		if err != nil {
			return err
		}

		var nodes []api.FullNode

		for _, addr := range addrs {
			node, closer, err := CreateLotusClient(ctx, addr)
			if err != nil {
				if errors.Is(err, syscall.ECONNREFUSED) {
					logger.Warnw("failed to dial node", "err", err)
					continue
				}

				return err
			}

			defer closer()

			nodes = append(nodes, node)
		}

		if len(nodes) == 0 {
			return xerrors.Errorf("no nodes")
		}

		cm := consensus.NewConsensusManager(nodes)

		same, err := cm.CheckGenesis(ctx)
		if err != nil {
			return err
		}

		if !same {
			return xerrors.Errorf("nodes do not share the same genesis")
		}

		gtp, err := cm.GetGenesis(ctx)
		if err != nil {
			return err
		}

		now := time.Now()
		expected := export.GetExpectedHeightAt(gtp, now, 30*time.Second)

		var height abi.ChainEpoch
		if cctx.IsSet("height") {
			height = abi.ChainEpoch(flagHeight)
		} else {
			after := abi.ChainEpoch(flagAfter)
			if !cctx.IsSet("after") {
				after = expected
			}

			height = export.GetNextSnapshotHeight(after, abi.ChainEpoch(flagInterval), abi.ChainEpoch(flagConfidence), cctx.IsSet("after"))
		}

		confidenceHeight := height + abi.ChainEpoch(flagConfidence)

		t := export.TimeAtHeight(gtp, confidenceHeight, 30*time.Second)

		logger.Infow("snapshot", "snapshot_height", height, "current_height", expected, "confidence_height", confidenceHeight, "run_at", t)

		time.Sleep(time.Until(t))

		tsk, err := cm.GetTipset(ctx, height)
		if err != nil {
			return err
		}

		nl, err := client.NewNodeLocker(ctx, flagNodeLockerAPI)
		if err != nil {
			return err
		}

		filterList, err := nl.LockedPeers(ctx)
		if err != nil {
			return err
		}

		node, peerID, err := cm.GetNodeWithTipSet(ctx, tsk, filterList)
		if err != nil {
			return err
		}

		lock, locked, err := nl.Lock(ctx, peerID)
		if err != nil {
			return err
		}

		if !locked {
			return xerrors.Errorf("failed to aquire lock")
		}

		r, w := io.Pipe()

		e := export.NewExport(node, tsk, abi.ChainEpoch(flagStaterootCount), true, w)
		errCh := make(chan error)
		go func() {
			errCh <- e.Export(ctx)
		}()

		go func() {
			lock := lock
			for {
				select {
				case <-time.After(time.Until(lock.Expiry()) / 2):
					locked, err := lock.Renew(ctx)
					if err != nil {
						logger.Errorw("error updating lock", "err", err)
						continue
					}

					if !locked {
						logger.Errorw("failed to acquire lock")
						continue
					}

					logger.Debugw("lock aquired", "expiry", lock.Expiry())
				}
			}
		}()

		go func() {
			var lastSize int
			for {
				select {
				case <-time.After(flagProgressUpdate):
					size, done := e.Progress()
					if size == 0 {
						continue
					}

					if done {
						return
					}

					logger.Infow("update", "total", size, "speed", (size-lastSize)/int(flagProgressUpdate/time.Second))
					lastSize = size
				}
			}
		}()

		if flagDiscard {
			logger.Infow("discarding output")
			io.Copy(ioutil.Discard, r)
		} else {
			minioClient, err := minio.New(flagBucketEndpoint, &minio.Options{
				Creds:  credentials.NewStaticV4(flagBucketAccessKey, flagBucketSecretKey, ""),
				Secure: false,
			})

			info, err := minioClient.PutObject(ctx, flagBucket, fmt.Sprintf("%d.car", height), r, -1, minio.PutObjectOptions{})
			if err != nil {
				return err
			}

			logger.Infow("upload",
				"bucket", info.Bucket,
				"key", info.Key,
				"etag", info.ETag,
				"size", info.Size,
				"location", info.Location,
				"version_id", info.VersionID,
				"expiration", info.Expiration,
				"expiration_rule_id", info.ExpirationRuleID,
			)
		}

		if err := <-errCh; err != nil {
			return err
		}

		logger.Infow("finished")

		return nil
	},
}
