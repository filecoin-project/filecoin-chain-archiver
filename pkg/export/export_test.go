package export

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
)

func TestGetNextSnapshotHeight(t *testing.T) {
	assert.Equal(t, 123, 123, "they should be equal")
	require.Equal(t, 123, 123, "they should be equal")

	assert.Equal(t, abi.ChainEpoch(500), GetNextSnapshotHeight(484, 100, 15, true))
	assert.Equal(t, abi.ChainEpoch(400), GetNextSnapshotHeight(484, 100, 15, false))
	assert.Equal(t, abi.ChainEpoch(500), GetNextSnapshotHeight(485, 100, 15, false))
	assert.Equal(t, abi.ChainEpoch(500), GetNextSnapshotHeight(495, 100, 15, false))
	assert.Equal(t, abi.ChainEpoch(500), GetNextSnapshotHeight(500, 100, 15, false))
	assert.Equal(t, abi.ChainEpoch(500), GetNextSnapshotHeight(505, 100, 15, false))
	assert.Equal(t, abi.ChainEpoch(500), GetNextSnapshotHeight(515, 100, 15, false))
	assert.Equal(t, abi.ChainEpoch(600), GetNextSnapshotHeight(585, 100, 15, false))
	assert.Equal(t, abi.ChainEpoch(600), GetNextSnapshotHeight(595, 100, 15, false))
}
