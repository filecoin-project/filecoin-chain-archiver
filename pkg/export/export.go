package export

import (
	"context"
	"fmt"
	"os"
	//"io"
	"sync"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/ipfs/go-log/v2"
	//"golang.org/x/xerrors"
)

var logger = log.Logger("filecoin-chain-archiver/pkg/export")

func GetExpectedHeightAt(gts *types.TipSet, at time.Time, blocktime time.Duration) abi.ChainEpoch {
	gt := time.Unix(int64(gts.MinTimestamp()), 0)
	expected := int64(at.Sub(gt) / blocktime)

	return abi.ChainEpoch(expected)
}

func TimeAtHeight(gts *types.TipSet, height abi.ChainEpoch, blocktime time.Duration) time.Time {
	gt := time.Unix(int64(gts.MinTimestamp()), 0)
	return gt.Add(time.Duration(height) * blocktime)
}

/*
	/- 500

|----------|----------|----------|----------|

	      |----------|
	485 - /          \ - 585
*/
func GetNextSnapshotHeight(current, interval, confidence abi.ChainEpoch, after bool) abi.ChainEpoch {
	next := ((current + interval) / interval) * interval
	if current+confidence < next && !after {
		return next - interval
	}

	return next
}

func waitAPIDown(ctx context.Context, node api.FullNode) error {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	logger.Infow("waiting for node to go offline")
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		_, err := node.Version(ctx)
		if err == nil {
			logger.Debugw("not offline yet")
			time.Sleep(time.Second)
			continue
		}

		return nil
	}
}

func waitAPI(ctx context.Context, node api.FullNode) error {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	logger.Infow("waiting for node to come online")
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		_, err := node.Version(ctx)
		if err != nil {
			logger.Debugw("not online yet", "err", err)
			time.Sleep(time.Second)
			continue
		}

		return nil
	}
}

type Export struct {
	node       api.FullNode
	head       types.TipSetKey
	tail       types.TipSetKey
	messages   bool
	receipts   bool
	stateroots bool
	workers    int
	exportName string
	exportDir string

	sizeMu   sync.Mutex
	size     int
	finished bool
}

func NewExport(node api.FullNode, head, tail types.TipSetKey, name, dir string) *Export {
	return &Export{
		node:       node,
		head:       head,
		tail:       tail,
		sizeMu:     sync.Mutex{},
		messages:   true,
		receipts:   true,
		stateroots: true,
		workers:    50,
		exportName: name,
		exportDir: dir,
	}
}

func (e *Export) Progress(path string) int64 {
	defer e.sizeMu.Unlock()
	e.sizeMu.Lock()
	file, err := os.Stat(path)
	if os.IsNotExist(err) {
		logger.Debugw("snapshot doesn't exist yet", "err", err)
	}

	return file.Size()
}

func (e *Export) done() {
	e.finished = true
}

func (e *Export) Export(ctx context.Context) error {
	defer e.done()
	if err := e.node.Shutdown(ctx); err != nil {
		return err
	}

	if err := waitAPIDown(ctx, e.node); err != nil {
		return fmt.Errorf("node failed to go offline: %w", err)
	}

	if err := waitAPI(ctx, e.node); err != nil {
		return fmt.Errorf("node failed to come back online: %w", err)
	}

	logger.Infow("starting export")
	// lotus chain export-range --internal --messages --receipts --stateroots --workers 50 --head "@${END}" --tail "@${START}" --write-buffer=5000000 export.car
	err := e.node.ChainExportRangeInternal(ctx, e.head, e.tail, api.ChainExportConfig{
		FileName: e.exportName,
		ExportDir: e.exportDir,
	})
	if err != nil {
		return err
	}

	return nil
}
