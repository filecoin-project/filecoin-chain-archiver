package export

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/ipfs/go-log/v2"

	"golang.org/x/xerrors"
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
	tsk        types.TipSetKey
	nroots     abi.ChainEpoch
	oldmsgskip bool
	output     io.WriteCloser

	sizeMu sync.Mutex
	size   int

	finished bool
}

func NewExport(node api.FullNode, tsk types.TipSetKey, nroots abi.ChainEpoch, oldmsgskip bool, output io.WriteCloser) *Export {
	return &Export{
		node:       node,
		tsk:        tsk,
		nroots:     nroots,
		oldmsgskip: oldmsgskip,
		output:     output,
		sizeMu:     sync.Mutex{},
		size:       0,
		finished:   false,
	}
}

func (e *Export) Progress() (int, bool) {
	defer e.sizeMu.Unlock()
	e.sizeMu.Lock()

	return e.size, e.finished
}

func (e *Export) update(more int) int {
	defer e.sizeMu.Unlock()
	e.sizeMu.Lock()
	e.size = e.size + more
	return more
}

func (e *Export) done() {
	e.output.Close()
	e.finished = true
}

func (e *Export) Export(ctx context.Context) error {
	defer e.done()
	/*
	if err := e.node.Shutdown(ctx); err != nil {
		return err
	}

	if err := waitAPIDown(ctx, e.node); err != nil {
		return fmt.Errorf("node failed to go offline: %w", err)
	}
	*/
	if err := waitAPI(ctx, e.node); err != nil {
		return fmt.Errorf("node is not online: %w", err)
	}

	logger.Infow("starting export")
	stream, err := e.node.ChainExport(ctx, e.nroots, e.oldmsgskip, e.tsk)
	if err != nil {
		return err
	}

	var last bool
	for b := range stream {
		last = e.update(len(b)) == 0

		if _, err := e.output.Write(b); err != nil {
			return err
		}
	}

	if !last {
		return xerrors.Errorf("incomplete export (remote connection lost?)")
	}

	return nil
}
