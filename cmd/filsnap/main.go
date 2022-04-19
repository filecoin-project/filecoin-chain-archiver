package main

import (
	"os"
	"strings"

	"github.com/travisperson/filsnap/build"
	"github.com/travisperson/filsnap/cmd/filsnap/cmds"

	"github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var logger = log.Logger("filsnap")

func main() {
	app := &cli.App{
		Name:    "filsnap",
		Usage:   "simple chain export mvp",
		Version: build.Version(),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level-named",
				Usage:   "common delimiated list of named loggers and log levels formatted as name:level",
				EnvVars: []string{"FILSNAP_LOG_LEVEL_NAMED"},
				Value:   "",
			},
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "set all filsnap loggers to level",
				EnvVars: []string{"FILSNAP_LOG_LEVEL"},
				Value:   "warn",
			},
		},
		Before: func(cctx *cli.Context) error {
			return setupLogging(cctx)
		},
		Commands: cmds.Commands,
	}

	err := app.Run(os.Args)
	if err != nil {
		logger.Errorw("exit", "error", err)
		os.Exit(1)
	}
}

func setupLogging(cctx *cli.Context) error {
	ll := cctx.String("log-level")
	if err := log.SetLogLevelRegex("filsnap/*", ll); err != nil {
		return xerrors.Errorf("set log level: %w", err)
	}

	llnamed := cctx.String("log-level-named")
	if llnamed != "" {
		for _, llname := range strings.Split(llnamed, ",") {
			parts := strings.Split(llname, ":")
			if len(parts) != 2 {
				return xerrors.Errorf("invalid named log level format: %q", llname)
			}
			if err := log.SetLogLevel(parts[0], parts[1]); err != nil {
				return xerrors.Errorf("set named log level %q to %q: %w", parts[0], parts[1], err)
			}

		}
	}

	return nil
}
