package nodelocker

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/ipfs/go-log/v2"
)

var logger = log.Logger("filecoin-chain-archiver/pkg/nodelocker")

type NodeLock struct {
	PeerID  string
	Expiry  time.Time
	Aquired bool
}

type nodeLock struct {
	peerID string
	expiry time.Time
	secret string
}

type NodeLocker struct {
	locksMu sync.Mutex
	locks   list.List
}

func NewNodeLocker() *NodeLocker {
	return &NodeLocker{}
}

func (snl *NodeLocker) expiry() {
	now := time.Now()

	expired := []*list.Element{}
	for e := snl.locks.Front(); e != nil; e = e.Next() {
		lock := e.Value.(nodeLock)
		if now.After(lock.expiry) {
			logger.Infow("expired", "peer", lock.peerID)
			expired = append(expired, e)
		}
	}

	for _, e := range expired {
		snl.locks.Remove(e)
	}
}

func (snl *NodeLocker) FetchLocks(ctx context.Context) ([]NodeLock, error) {
	snl.locksMu.Lock()
	defer snl.locksMu.Unlock()

	snl.expiry()

	locks := []NodeLock{}
	for e := snl.locks.Front(); e != nil; e = e.Next() {
		lock := e.Value.(nodeLock)
		locks = append(locks, NodeLock{
			PeerID:  lock.peerID,
			Expiry:  lock.expiry,
			Aquired: true,
		})
	}

	return locks, nil
}

func (snl *NodeLocker) Lock(ctx context.Context, peerID, secret string) (NodeLock, error) {
	snl.locksMu.Lock()
	defer snl.locksMu.Unlock()

	snl.expiry()

	now := time.Now()
	for e := snl.locks.Front(); e != nil; e = e.Next() {
		lock := e.Value.(nodeLock)
		logger.Debugw("peer", "local", lock.peerID, "provided", peerID)
		if lock.peerID == peerID {
			logger.Debugw("secret", "local", lock.secret, "provided", secret)
			if lock.secret == secret {
				lock.expiry = now.Add(60 * time.Second)
				e.Value = lock

				logger.Infow("updated lock", "expiry", lock.expiry, "peer", lock.peerID, "secret", lock.secret)
				return NodeLock{
					PeerID:  lock.peerID,
					Expiry:  lock.expiry,
					Aquired: true,
				}, nil
			} else {
				logger.Infow("lock failed", "expiry", lock.expiry, "peer", lock.peerID, "secret", lock.secret)
				return NodeLock{
					PeerID:  lock.peerID,
					Expiry:  lock.expiry,
					Aquired: false,
				}, nil
			}
		}
	}

	lock := nodeLock{
		peerID: peerID,
		expiry: now.Add(60 * time.Second),
		secret: secret,
	}

	logger.Infow("new lock", "expiry", lock.expiry, "peer", lock.peerID, "secret", lock.secret, "now", now)

	snl.locks.PushBack(lock)

	return NodeLock{
		PeerID:  lock.peerID,
		Expiry:  lock.expiry,
		Aquired: true,
	}, nil
}
