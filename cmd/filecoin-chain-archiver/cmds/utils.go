package cmds

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"

	"github.com/filecoin-project/go-jsonrpc"

	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/config"
)

func NodeMultiaddrs(cfg *config.Config) ([]string, error) {
	var multiaddrs []string

	for _, node := range cfg.Nodes {
		var multiaddr string
		if len(node.TokenPath) != 0 {
			bs, err := ioutil.ReadFile(node.TokenPath)
			if err != nil {
				return []string{}, err
			}

			multiaddr = fmt.Sprintf("%s:%s", bs, node.Address)
		} else {
			multiaddr = node.Address
		}

		multiaddrs = append(multiaddrs, multiaddr)
	}

	return multiaddrs, nil
}

func CreateLotusClient(ctx context.Context, multiaddr string) (api.FullNode, jsonrpc.ClientCloser, error) {
	ainfo := cliutil.ParseApiInfo(multiaddr)

	darg, err := ainfo.DialArgs("v1")
	if err != nil {
		return nil, nil, err
	}

	return client.NewFullNodeRPCV1(ctx, darg, ainfo.AuthHeader())
}
