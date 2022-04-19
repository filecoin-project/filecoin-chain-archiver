module github.com/travisperson/filsnap

go 1.16

require (
	github.com/BurntSushi/toml v0.4.1
	github.com/filecoin-project/go-jsonrpc v0.1.5
	github.com/filecoin-project/go-state-types v0.1.3
	github.com/filecoin-project/lotus v1.15.1
	github.com/gorilla/mux v1.8.0
	github.com/ipfs/go-log/v2 v2.5.1
	github.com/minio/minio-go/v7 v7.0.24
	github.com/multiformats/go-multiaddr v0.5.0
	github.com/prometheus/client_golang v1.11.0
	github.com/slok/go-http-metrics v0.10.0
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli/v2 v2.4.0
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f
)

replace github.com/filecoin-project/filecoin-ffi => github.com/filecoin-project/ffi-stub v0.3.0
