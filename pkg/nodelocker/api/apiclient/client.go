package apiclient

import (
	"context"
	"net/http"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/nodelocker/api"
)

func NewOperatorClient(ctx context.Context, addr string, requestHeader http.Header) (api.Operator, jsonrpc.ClientCloser, error) {
	var res api.OperatorStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Operator",
		[]interface{}{
			&res.Internal,
			&res.NodeLockerStruct.Internal,
		},
		requestHeader,
	)

	return &res, closer, err
}

func NewServiceClient(ctx context.Context, addr string, requestHeader http.Header) (api.NodeLocker, jsonrpc.ClientCloser, error) {
	var res api.NodeLockerStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "NodeLocker",
		[]interface{}{
			&res.Internal,
		},
		requestHeader,
	)

	return &res, closer, err
}
