package types

import (
	"time"
)

type ServerNodeLockResponse struct {
	PeerID  string
	Expiry  time.Time
	Aquired bool
}
