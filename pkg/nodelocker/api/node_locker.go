package api

import (
	"context"

	"github.com/filecoin-project/filecoin-chain-archiver/pkg/nodelocker"
)

// This needs to be lifted to something under pkg/export-service/* so that it can be imported here, but defined closer to the export serice.
type NodeLocker interface {
	FetchLocks(context.Context) ([]nodelocker.NodeLock, error)         //perm:write
	Lock(context.Context, string, string) (nodelocker.NodeLock, error) //perm:write
}

type NodeLockerStruct struct {
	Internal struct {
		FetchLocks func(p0 context.Context) ([]nodelocker.NodeLock, error)                     `perm:"write"`
		Lock       func(p0 context.Context, p1 string, p2 string) (nodelocker.NodeLock, error) `perm:"write"`
	}
}

func (s *NodeLockerStruct) FetchLocks(p0 context.Context) ([]nodelocker.NodeLock, error) {
	return s.Internal.FetchLocks(p0)
}

func (s *NodeLockerStruct) Lock(p0 context.Context, p1 string, p2 string) (nodelocker.NodeLock, error) {
	return s.Internal.Lock(p0, p1, p2)
}
