package cmds

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/filecoin-project/filecoin-chain-archiver/pkg/config"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/consensus"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/export"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/nodelocker/client"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
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
			Name:    "name-prefix",
			Usage:   "add a prefix to the snapshot name",
			Value:   "default/",
			EnvVars: []string{"FCA_CREATE_NAME_PREFIX"},
		},
		&cli.StringFlag{
			Name:    "nodelocker-api",
			Usage:   "host and port of nodelocker api",
			Value:   "http://127.0.0.1:5100",
			EnvVars: []string{"FCA_CREATE_NODELOCKER_API"},
		},
		&cli.StringFlag{
			Name:    "bucket",
			Usage:   "bucket name for export upload",
			EnvVars: []string{"FCA_CREATE_BUCKET"},
		},
		&cli.StringFlag{
			Name:    "bucket-endpoint",
			Usage:   "bucket host and port for upload",
			EnvVars: []string{"FCA_CREATE_BUCKET_ENDPOINT"},
		},
		&cli.StringFlag{
			Name:    "access-key",
			Usage:   "access key for upload",
			EnvVars: []string{"FCA_CREATE_ACCESS_KEY"},
		},
		&cli.StringFlag{
			Name:    "secret-key",
			Usage:   "secret key for upload",
			EnvVars: []string{"FCA_CREATE_SECRET_KEY"},
		},
		&cli.BoolFlag{
			Name:    "discard",
			Usage:   "discard output, do not upload",
			EnvVars: []string{"FCA_CREATE_DISCARD"},
			Value:   false,
		},
		&cli.StringFlag{
			Name:    "config-path",
			Usage:   "path to configuration file",
			EnvVars: []string{"FCA_CONFIG_PATH"},
			Value:   "./config.toml",
		},
		&cli.IntFlag{
			Name:    "interval",
			Usage:   "interval used to determine next export height",
			EnvVars: []string{"FCA_CREATE_INTERVAL"},
			Value:   120,
		},
		&cli.IntFlag{
			Name:    "confidence",
			Usage:   "number of tipsets that should exist after the determine export height",
			EnvVars: []string{"FCA_CREATE_CONFIDENCE"},
			Value:   15,
		},
		&cli.IntFlag{
			Name:    "after",
			Usage:   "use interval height after this height",
			EnvVars: []string{"FCA_CREATE_AFTER"},
			Value:   0,
		},
		&cli.IntFlag{
			Name:    "height",
			Usage:   "create a snapshot from the given height",
			EnvVars: []string{"FCA_CREATE_HEIGHT"},
			Value:   0,
		},
		&cli.IntFlag{
			Name:    "stateroot-count",
			Usage:   "number of stateroots to included in snapshot",
			EnvVars: []string{"FCA_CREATE_STATEROOT_COUNT"},
			Value:   2000,
		},
		&cli.DurationFlag{
			Name:    "progress-update",
			Usage:   "how frequenty to provide provide update logs",
			EnvVars: []string{"FCA_CREATE_PROGRESS_UPDATE"},
			Value:   60 * time.Second,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()

		flagBucketEndpoint := cctx.String("bucket-endpoint")
		flagBucketAccessKey := cctx.String("access-key")
		flagBucketSecretKey := cctx.String("secret-key")
		flagNamePrefix := cctx.String("name-prefix")
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

		u, err := url.Parse(flagBucketEndpoint)
		if err != nil {
			return err
		}

		icfg, err := config.FromFile(flagConfigPath, &config.ExportWorkerConfig{})
		if err != nil {
			return err
		}

		cfg := icfg.(*config.ExportWorkerConfig)

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
				} else {
					logger.Warnw("failed to create node client", "err", err)
				}

				continue
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

		var iteration int
		if cctx.IsSet("interval") {
			iteration = int(uint64(height)/uint64(flagInterval)) % len(nodes)
		} else {
			iteration = rand.Int() % len(nodes)
		}

		logger.Infow("iteration", "value", iteration)
		cm.ShiftStartNode(iteration)

		node, peerID, err := cm.GetNodeWithTipSet(ctx, tsk, filterList)
		if err != nil {
			return err
		}

		logger.Infow("node", "peer_id", peerID)

		lock, locked, err := nl.Lock(ctx, peerID)
		if err != nil {
			return err
		}

		if !locked {
			return xerrors.Errorf("failed to aquire lock")
		}

		r, w := io.Pipe()
		h := sha256.New()

		tr := io.TeeReader(r, h)

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

			if err := <-errCh; err != nil {
				return err
			}
		} else {
			host := u.Hostname()
			port := u.Port()
			if port == "" {
				port = "80"
				if u.Scheme == "https" {
					port = "443"
				}
			}

			logger.Infow("upload endpoint", "host", host, "port", port, "tls", u.Scheme == "https")

			minioClient, err := minio.New(fmt.Sprintf("%s:%s", host, port), &minio.Options{
				Creds:  credentials.NewStaticV4(flagBucketAccessKey, flagBucketSecretKey, ""),
				Secure: u.Scheme == "https",
			})

			t := export.TimeAtHeight(gtp, height, 30*time.Second)

			name := fmt.Sprintf("%d_%s", height, t.Format("2006_01_02T15_04_05Z"))

			logger.Infow("object", "name", name)

			info, err := minioClient.PutObject(ctx, flagBucket, fmt.Sprintf("%s%s.car", flagNamePrefix, name), tr, -1, minio.PutObjectOptions{
				ContentDisposition: fmt.Sprintf("attachment; filename=\"%s.car\"", name),
				ContentType:        "application/octet-stream",
			})
			if err != nil {
				return fmt.Errorf("failed to upload object (%s): %w", fmt.Sprintf("%s%s.car", flagNamePrefix, name), err)
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

			if err := <-errCh; err != nil {
				return err
			}

			latestLocation, err := url.QueryUnescape(info.Location)
			if err != nil {
				logger.Errorw("failed to decode location url", "location", info.Location, "err", err)
				latestLocation = info.Location
			}

			sha256sum := fmt.Sprintf("%x *%s.car\n", h.Sum(nil), name)

			info, err = minioClient.PutObject(ctx, flagBucket, fmt.Sprintf("%s%s.sha256sum", flagNamePrefix, name), strings.NewReader(sha256sum), -1, minio.PutObjectOptions{
				ContentDisposition: fmt.Sprintf("attachment; filename=\"%s.sha256sum\"", name),
				ContentType:        "text/plain",
			})
			if err != nil {
				logger.Errorw("failed to write sha256sum", "object", fmt.Sprintf("%s%s.sha256sum", flagNamePrefix, name), "err", err)
			}

			info, err = minioClient.PutObject(ctx, flagBucket, fmt.Sprintf("%slatest", flagNamePrefix), strings.NewReader(latestLocation), -1, minio.PutObjectOptions{
				WebsiteRedirectLocation: latestLocation,
				ContentType:             "text/plain",
			})
			if err != nil {
				return fmt.Errorf("failed to write latest", "object", fmt.Sprintf("%slatest", flagNamePrefix), "err", err)
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

		logger.Infow("finished", "digiest", fmt.Sprintf("%x", h.Sum(nil)))

		return nil
	},
}
