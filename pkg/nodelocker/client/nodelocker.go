package client

import (
	"context"
	"math/rand"
	"time"

	"github.com/filecoin-project/lotus/cli/util"
	"github.com/travisperson/filsnap/pkg/nodelocker"
	"github.com/travisperson/filsnap/pkg/nodelocker/api/apiclient"
)

const charset = "abcdefghijklmnopqrstuvwxyz"

func randomString() string {
	b := make([]byte, 10)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

type NodeLockerConn interface {
	FetchLocks(context.Context) ([]nodelocker.NodeLock, error)
	Lock(context.Context, string, string) (nodelocker.NodeLock, error)
}

type NodeLocker struct {
	conn   NodeLockerConn
	closer func()
	secret string
}

type NodeLock struct {
	conn   NodeLockerConn
	peerID string
	secret string
	expiry time.Time
}

func (nl *NodeLock) Renew(ctx context.Context) (bool, error) {
	lock, err := nl.conn.Lock(ctx, nl.peerID, nl.secret)
	if err != nil {
		return false, err
	}

	nl.expiry = lock.Expiry

	return lock.Aquired, nil
}

func (nl *NodeLock) Expiry() time.Time {
	return nl.expiry
}

func NewNodeLocker(ctx context.Context, endpoint string) (*NodeLocker, error) {
	ai := cliutil.ParseApiInfo(endpoint)
	url, err := ai.DialArgs("v0")
	if err != nil {
		return nil, err
	}

	conn, closer, err := apiclient.NewServiceClient(ctx, url, ai.AuthHeader())
	if err != nil {
		return nil, err
	}

	secret := randomString()
	return &NodeLocker{
		conn:   conn,
		closer: closer,
		secret: secret,
	}, nil
}

func (nl *NodeLocker) LockedPeers(ctx context.Context) ([]string, error) {
	locks, err := nl.conn.FetchLocks(ctx)
	if err != nil {
		return []string{}, err
	}

	peers := []string{}
	for _, lock := range locks {
		peers = append(peers, lock.PeerID)
	}

	return peers, nil
}

func (nl *NodeLocker) Lock(ctx context.Context, peerID string) (*NodeLock, bool, error) {
	lock := &NodeLock{
		conn:   nl.conn,
		peerID: peerID,
		secret: nl.secret,
	}

	locked, err := lock.Renew(ctx)
	if err != nil {
		return nil, false, err
	}

	if !locked {
		return nil, false, nil
	}

	return lock, locked, nil
}

func (nl *NodeLocker) Close() {
	nl.closer()
}
