SHELL=/usr/bin/env bash

unexport GOFLAGS

GOCC?=go

ldflags=-X=github.com/filecoin-project/filecoin-snapshot-mvp/build.CurrentCommit=+git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))

ifneq ($(strip $(LDFLAGS)),)
	ldflags+=-extldflags=$(LDFLAGS)
endif

GOFLAGS+=-ldflags="$(ldflags)"

BINS:=

all: filsnap
.PHONY: all

clean:
	rm -rf $(BINS)
.PHONY: clean

filsnap:
	$(GOCC) build $(GOFLAGS) -o filsnap ./cmd/filsnap/
.PHONY: filsnap
BINS+=filsnap
