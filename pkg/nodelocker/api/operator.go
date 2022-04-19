package api

import (
	"context"
)

type Operator interface {
	NodeLocker

	Version(context.Context) (string, error)           //perm:read
	LogList(context.Context) ([]string, error)         //perm:write
	LogSetLevel(context.Context, string, string) error //perm:write
}

type OperatorStruct struct {
	NodeLockerStruct

	Internal struct {
		Version     func(p0 context.Context) (string, error)             `perm:"read"`
		LogList     func(p0 context.Context) ([]string, error)           `perm:"write"`
		LogSetLevel func(p0 context.Context, p1 string, p2 string) error `perm:"write"`
	}
}

func (s *OperatorStruct) Version(p0 context.Context) (string, error) {
	return s.Internal.Version(p0)
}

func (s *OperatorStruct) LogList(p0 context.Context) ([]string, error) {
	return s.Internal.LogList(p0)
}

func (s *OperatorStruct) LogSetLevel(p0 context.Context, p1 string, p2 string) error {
	return s.Internal.LogSetLevel(p0, p1, p2)
}
