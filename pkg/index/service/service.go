package service

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/gorilla/mux"
	"github.com/ipfs/go-log/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
	"github.com/slok/go-http-metrics/middleware/std"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/filecoin-project/filecoin-chain-archiver/pkg/config"
	"github.com/filecoin-project/filecoin-chain-archiver/pkg/index"
)

var logger = log.Logger("filecoin-chain-archiver/service/index-resolver")

type IndexService struct {
	ctx            context.Context
	ServiceRouter  *mux.Router
	OperatorRouter *mux.Router

	rpc *jsonrpc.RPCServer

	ready   bool
	readyMu sync.Mutex

	resolver index.Resolver
}

func NewIndexService(ctx context.Context) *IndexService {
	return &IndexService{
		ctx:            ctx,
		ServiceRouter:  mux.NewRouter(),
		OperatorRouter: mux.NewRouter(),
		rpc:            jsonrpc.NewServer(),
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Infow("request", "uri", r.RequestURI)
		next.ServeHTTP(w, r)
	})
}

func (bs *IndexService) SetupService(configPath string) error {
	defer bs.setReady()
	mdlw := middleware.New(middleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{}),
	})
	bs.ServiceRouter.Use(std.HandlerProvider("", mdlw))
	bs.ServiceRouter.Use(loggingMiddleware)

	icfg, err := config.FromFile(configPath, config.DefaultIndexServiceConfig())
	if err != nil {
		return err
	}

	cfg := icfg.(*config.IndexServiceConfig)

	if err := bs.setupResolver(cfg); err != nil {
		return err
	}

	bs.ServiceRouter.HandleFunc("/minimal/latest", func(w http.ResponseWriter, r *http.Request) {
		value, err := bs.resolver.Resolve(context.Background(), "minimal/latest")
		if err != nil {
			logger.Errorw("error resolving", "err", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}

		w.Header().Set("Location", value)
		w.WriteHeader(http.StatusFound)
	})

	return bs.dumpRoutes(bs.ServiceRouter)
}

func (bs *IndexService) setupResolver(cfg *config.IndexServiceConfig) error {
	s3ResolverCfg := cfg.S3Resolver
	u, err := url.Parse(s3ResolverCfg.Endpoint)
	if err != nil {
		return err
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}

	akBytes, err := ioutil.ReadFile(s3ResolverCfg.AccessKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", s3ResolverCfg.AccessKeyPath, err)
	}
	skBytes, err := ioutil.ReadFile(s3ResolverCfg.SecretKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", s3ResolverCfg.SecretKeyPath, err)
	}

	accessKey := strings.TrimSuffix(string(akBytes), "\n")
	secretKey := strings.TrimSuffix(string(skBytes), "\n")

	minioClient, err := minio.New(fmt.Sprintf("%s:%s", host, port), &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: u.Scheme == "https",
	})
	if err != nil {
		return err
	}

	s3IndexResolver := index.NewIndexS3Resolver(minioClient, s3ResolverCfg.Bucket)
	cachedResolver := index.NewCachedResolver(s3IndexResolver)

	bs.resolver = cachedResolver

	return nil
}

func (bs *IndexService) SetupOperator() error {
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

func (bs *IndexService) dumpRoutes(router *mux.Router) error {
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

func (bs *IndexService) setReady() {
	bs.readyMu.Lock()
	defer bs.readyMu.Unlock()
	bs.ready = true
}

func (bs *IndexService) IsReady() bool {
	bs.readyMu.Lock()
	defer bs.readyMu.Unlock()
	return bs.ready
}

func (bs *IndexService) Shutdown() {
	bs.unsetReady()
}

func (bs *IndexService) unsetReady() {
	bs.readyMu.Lock()
	defer bs.readyMu.Unlock()
	bs.ready = false
}

func (bs *IndexService) Close() {
}
