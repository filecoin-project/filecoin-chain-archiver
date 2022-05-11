package service

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/gorilla/mux"
	"github.com/ipfs/go-log/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
	"github.com/slok/go-http-metrics/middleware/std"

	"github.com/filecoin-project/filecoin-chain-archiver/pkg/nodelocker"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/nodelocker/api"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/nodelocker/operator"
)

var logger = log.Logger("filecoin-chain-archiver/service/nodelocker")

type NodeLockerService struct {
	ctx            context.Context
	ServiceRouter  *mux.Router
	OperatorRouter *mux.Router

	rpc      *jsonrpc.RPCServer
	operator api.Operator

	ready   bool
	readyMu sync.Mutex

	locker *nodelocker.NodeLocker
}

func NewLockerService(ctx context.Context) *NodeLockerService {
	return &NodeLockerService{
		ctx:            ctx,
		ServiceRouter:  mux.NewRouter(),
		OperatorRouter: mux.NewRouter(),
		rpc:            jsonrpc.NewServer(),
	}

}

func (bs *NodeLockerService) SetupService() error {
	defer bs.setReady()
	mdlw := middleware.New(middleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{}),
	})
	bs.ServiceRouter.Use(std.HandlerProvider("", mdlw))

	bs.rpc.Register("NodeLocker", bs)
	bs.ServiceRouter.Handle("/rpc/v0", bs.rpc)

	bs.locker = nodelocker.NewNodeLocker()

	return nil
}

func (bs *NodeLockerService) FetchLocks(ctx context.Context) ([]nodelocker.NodeLock, error) {
	return bs.locker.FetchLocks(ctx)
}

func (bs *NodeLockerService) Lock(ctx context.Context, peerID, secret string) (nodelocker.NodeLock, error) {
	return bs.locker.Lock(ctx, peerID, secret)
}

func (bs *NodeLockerService) SetupOperator() error {
	bs.operator = &operator.OperatorImpl{NodeLocker: bs.locker}
	bs.rpc.Register("Operator", bs.operator)
	bs.OperatorRouter.Handle("/rpc/v0", bs.rpc)

	bs.OperatorRouter.PathPrefix("/debug/pprof/").Handler(http.DefaultServeMux)

	bs.OperatorRouter.HandleFunc("/liveness", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	bs.OperatorRouter.HandleFunc("/readiness", func(w http.ResponseWriter, r *http.Request) {
		isReady := bs.IsReady()

		if isReady {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})

	bs.OperatorRouter.Handle("/metrics", promhttp.Handler())

	return bs.dumpRoutes(bs.OperatorRouter)
}

func (bs *NodeLockerService) dumpRoutes(router *mux.Router) error {
	return router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err == nil {
			logger.Debugw("route template", "path", pathTemplate)
		}
		pathRegexp, err := route.GetPathRegexp()
		if err == nil {
			logger.Debugw("route regexp", "path", pathRegexp)
		}
		queriesTemplates, err := route.GetQueriesTemplates()
		if err == nil {
			logger.Debugw("queries templates", "queries", strings.Join(queriesTemplates, ","))
		}
		queriesRegexps, err := route.GetQueriesRegexp()
		if err == nil {
			logger.Debugw("queries regex", "queries", strings.Join(queriesRegexps, ","))
		}
		methods, err := route.GetMethods()
		if err == nil {
			logger.Debugw("method", "queries", strings.Join(methods, ","))
		}
		return nil
	})
}

func (bs *NodeLockerService) setReady() {
	bs.readyMu.Lock()
	defer bs.readyMu.Unlock()
	bs.ready = true
}

func (bs *NodeLockerService) IsReady() bool {
	bs.readyMu.Lock()
	defer bs.readyMu.Unlock()
	return bs.ready
}

func (bs *NodeLockerService) Shutdown() {
	bs.unsetReady()
}

func (bs *NodeLockerService) unsetReady() {
	bs.readyMu.Lock()
	defer bs.readyMu.Unlock()
	bs.ready = false
}

func (bs *NodeLockerService) Close() {
}
