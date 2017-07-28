
COVERDIR = .coverage
TOOLDIR = tools

GO_SRC := $(shell find . -name '*.go' ! -path '*/vendor/*' ! -path 'tools/*' ! -path 'assets/bindata.go' ) assets/bindata.go
GO_DIRS := $(shell find . -type d -name '*.go' ! -path '*/vendor/*' ! -path 'tools/*' )
GO_PKGS := $(shell go list ./... | grep -v '/vendor/')

GO_CMDS := $(shell find cmd -mindepth 1 -type d -printf "%f ")

VERSION ?= $(shell git describe --dirty)

CONCURRENT_LINTERS ?= $(shell cat /proc/cpuinfo | grep processor | wc -l)
LINTER_DEADLINE ?= 30s

export PATH := $(TOOLDIR)/bin:$(PATH)
SHELL := env PATH=$(PATH) /bin/bash

# Set BUILD_ENV to development to build web assets in dev mode.
BUILD_ENV ?= production

WEB_SRC_ASSETS := $(shell find node_modules -type f) $(shell find web -type f)
WEB_BUILT_ASSETS := $(shell find assets/static -type f) $(shell find assets/static -type d)

WEBPACK := ./node_modules/.bin/webpack
WEBPACK_DEV_SERVER := ./node_modules/.bin/webpack-dev-server

all: style lint test $(GO_CMDS)

binary: $(GO_CMDS)

% : cmd/% $(GO_SRC)
	CGO_ENABLED=0 go build -a -ldflags "-extldflags '-static' -X main.Version=$(VERSION)" -o $@ ./$<

assets/bindata.go: assets/static $(WEB_BUILT_ASSETS)
	go-bindata \
		-pkg=assets \
		-o assets/bindata.go \
		-ignore=bindata\.go \
		-ignore=.*\.map$ \
		-prefix=assets/static \
		assets/static/...

web: $(WEB_SRC_ASSETS)
	$(WEBPACK)

web-live: callbackserver
	./$(BINARY).x86_64 --listen.addr=tcp://0.0.0.0:8080 --debug.static-proxy=http://localhost:23182 --log-level debug & \
		$(WEBPACK_DEV_SERVER) --port 23182 & \
		wait $(jobs -p)

style: tools
	gometalinter --disable-all --enable=gofmt --vendor

lint: tools
	@echo Using $(CONCURRENT_LINTERS) processes
	gometalinter -j $(CONCURRENT_LINTERS) --deadline=$(LINTER_DEADLINE) --disable=gotype $(GO_DIRS)

fmt: tools
	gofmt -s -w $(GO_SRC)

test: tools $(GO_SRC)
	@mkdir -p $(COVERDIR)
	@rm -f $(COVERDIR)/*
	for pkg in $(GO_PKGS) ; do \
		go test -v -covermode count -coverprofile=$(COVERDIR)/$$(echo $$pkg | tr '/' '-').out $$pkg || exit 1 ; \
	done
	gocovmerge $(shell find $(COVERDIR) -name '*.out') > cover.out

tools:
	$(MAKE) -C $(TOOLDIR)

.PHONY: tools style fmt test all binary web web-live
