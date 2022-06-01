package cmds

import (
	"fmt"

	"github.com/filecoin-project/filecoin-chain-archiver/pkg/config"
	"github.com/urfave/cli/v2"
)

var cmdDefaultConfig = &cli.Command{
	Name:        "default-config",
	Usage:       "prints the default configuration",
	Description: TrimDescription(``),
	Flags:       []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		var icfg interface{}

		cfg := config.DefaultExportWorkerConfig()
		cfg.Nodes = append(cfg.Nodes, config.Node{
			Address:   "/ip4/127.0.0.1/1234",
			TokenPath: "/path/to/token",
		})
		icfg = cfg

		bs, err := config.ConfigComment(icfg)
		if err != nil {
			return err
		}

		fmt.Printf("%s", string(bs))

		return nil
	},
}
