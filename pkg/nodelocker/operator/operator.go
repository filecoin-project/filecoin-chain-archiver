package operator

import (
	"context"

	"github.com/travisperson/filsnap/build"
	"github.com/travisperson/filsnap/pkg/nodelocker"
	"github.com/travisperson/filsnap/pkg/nodelocker/api"

	"github.com/ipfs/go-log/v2"
)

var logger = log.Logger("filsnap/operator/thing")

type OperatorImpl struct {
	NodeLocker api.NodeLocker
}

func (s *OperatorImpl) FetchLocks(ctx context.Context) ([]nodelocker.NodeLock, error) {
	return s.NodeLocker.FetchLocks(ctx)
}

func (s *OperatorImpl) Lock(ctx context.Context, peerID, secret string) (nodelocker.NodeLock, error) {
	return s.NodeLocker.Lock(ctx, peerID, secret)
}

func (s *OperatorImpl) Version(ctx context.Context) (string, error) {
	return build.Version(), nil
}

func (s *OperatorImpl) LogList(ctx context.Context) ([]string, error) {
	return log.GetSubsystems(), nil
}

func (s *OperatorImpl) LogSetLevel(ctx context.Context, subsystem string, level string) error {
	return log.SetLogLevel(subsystem, level)
}
