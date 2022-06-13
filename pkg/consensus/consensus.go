package consensus

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/ipfs/go-log/v2"
)

var logger = log.Logger("filecoin-chain-archiver/pkg/consensus")

type ConsensusManager struct {
	nodes []api.FullNode
}

func NewConsensusManager(nodes []api.FullNode) *ConsensusManager {
	return &ConsensusManager{
		nodes: nodes,
	}
}

func (cm *ConsensusManager) CheckGenesis(ctx context.Context) (bool, error) {
	consensus := make(map[types.TipSetKey]int)
	for _, node := range cm.nodes {
		ts, err := node.ChainGetGenesis(ctx)
		if err != nil {
			logger.Errorw("error checking genesis", "err", err)
			continue
		}

		if _, ok := consensus[ts.Key()]; !ok {
			consensus[ts.Key()] = 1
		} else {
			consensus[ts.Key()] = consensus[ts.Key()] + 1
		}
	}

	return len(consensus) == 1, nil
}

func (cm *ConsensusManager) GetGenesis(ctx context.Context) (*types.TipSet, error) {
	for _, node := range cm.nodes {
		gtp, err := node.ChainGetGenesis(ctx)
		if err != nil {
			logger.Errorw("error getting genesis", "err", err)
			continue
		}

		return gtp, nil
	}

	return nil, fmt.Errorf("could not get genesis")
}

func (cm *ConsensusManager) GetTipset(ctx context.Context, height abi.ChainEpoch) (types.TipSetKey, error) {
	consensus := make(map[types.TipSetKey]int)
	for _, node := range cm.nodes {
		ts, err := node.ChainGetTipSetByHeight(ctx, height, types.EmptyTSK)
		if err != nil {
			logger.Errorw("error checking tipset", "err", err)
			continue
		}

		if _, ok := consensus[ts.Key()]; !ok {
			consensus[ts.Key()] = 1
		} else {
			consensus[ts.Key()] = consensus[ts.Key()] + 1
		}
	}

	pick := types.EmptyTSK
	votes := 0

	for k, v := range consensus {
		if v > votes {
			votes = v
			pick = k
		}
	}

	return pick, nil
}

func (cm *ConsensusManager) ShiftStartNode(iteration int) {
	nodes := make([]api.FullNode, len(cm.nodes))

	for i := 0; i < len(cm.nodes); i++ {
		source := (i + iteration) % len(cm.nodes)
		logger.Debugw("shift start node", "assignment", i, "source")
		nodes[i] = cm.nodes[source]
	}

	cm.nodes = nodes
}

func (cm *ConsensusManager) GetNodeWithTipSet(ctx context.Context, tsk types.TipSetKey, filterList []string) (api.FullNode, string, error) {
	peerFilter := make(map[string]struct{})
	for _, peer := range filterList {
		peerFilter[peer] = struct{}{}
	}
	for _, node := range cm.nodes {
		id, err := node.ID(ctx)
		if err != nil {
			logger.Errorw("error getting node", "err", err)
			continue
		}

		if _, has := peerFilter[id.String()]; has {
			continue
		}

		_, err = node.ChainGetTipSet(ctx, tsk)
		if err != nil {
			logger.Errorw("error getting node", "err", err)
			continue
		}

		return node, id.String(), nil
	}

	return nil, "", fmt.Errorf("could not get node")
}
