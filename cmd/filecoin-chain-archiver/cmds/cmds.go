package cmds

import (
	"strings"

	"github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
)

var logger = log.Logger("filecoin-chain-archiver/cmds")

var Commands = []*cli.Command{cmdCreate, cmdDefaultConfig, cmdService, cmdIndexService}

func TrimDescription(desc string) string {
	lines := strings.Split(desc, "\n")
	lines = lines[1:]
	for i, line := range lines {
		lines[i] = strings.TrimLeft(line, "\t")
	}
	return strings.Join(lines, "\n")
}
