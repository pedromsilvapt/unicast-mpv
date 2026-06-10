BINARY  := unicast-mpv
PKG     := ./cmd/unicast-mpv
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/unicast/unicast-mpv/cmd/unicast-mpv.version=$(VERSION)

DEB_MAINTAINER ?= Pedro Silva <pemiolsi@hotmail.com>
DEB_VENDOR     ?= Pedro Silva
DEB_HOMEPAGE   ?= https://github.com/pedromsilvapt/unicast-mpv

GITEA_USERNAME         ?=
GITEA_PASSWORD         ?=
GITEA_PACKAGE_URL      ?= https://gitea.home/api/packages/Silvas/debian
GITEA_DEB_DISTRIBUTION ?= stable
GITEA_DEB_COMPONENT    ?= main

.PHONY: build install test vet tidy clean snapshot release

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

install:
	go install $(PKG)

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

snapshot:
	DEB_MAINTAINER="$(DEB_MAINTAINER)" \
	DEB_VENDOR="$(DEB_VENDOR)" \
	DEB_HOMEPAGE="$(DEB_HOMEPAGE)" \
	GORELEASER_CURRENT_TAG=$(VERSION) \
		goreleaser release --snapshot --clean

release:
	DEB_MAINTAINER="$(DEB_MAINTAINER)" \
	DEB_VENDOR="$(DEB_VENDOR)" \
	DEB_HOMEPAGE="$(DEB_HOMEPAGE)" \
	GITEA_USERNAME="$(GITEA_USERNAME)" \
	GITEA_PASSWORD="$(GITEA_PASSWORD)" \
	GITEA_PACKAGE_URL="$(GITEA_PACKAGE_URL)" \
	GITEA_DEB_DISTRIBUTION="$(GITEA_DEB_DISTRIBUTION)" \
	GITEA_DEB_COMPONENT="$(GITEA_DEB_COMPONENT)" \
		goreleaser release --clean

clean:
	rm -rf bin/ dist/
