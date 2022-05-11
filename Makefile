SHELL=/usr/bin/env bash

unexport GOFLAGS

GOCC?=go

ldflags=-X=github.com/filecoin-project/filecoin-chain-archiver/build.CurrentCommit=+git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))

ifneq ($(strip $(LDFLAGS)),)
	ldflags+=-extldflags=$(LDFLAGS)
endif

GOFLAGS+=-ldflags="$(ldflags)"

BINS:=

all: filecoin-chain-archiver
.PHONY: all

clean:
	rm -rf $(BINS)
.PHONY: clean

filecoin-chain-archiver:
	$(GOCC) build $(GOFLAGS) -o filecoin-chain-archiver ./cmd/filecoin-chain-archiver/
.PHONY: filecoin-chain-archiver
BINS+=filecoin-chain-archiver
