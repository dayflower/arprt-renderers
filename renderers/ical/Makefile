GO ?= go
GOFMT ?= gofmt
PYTHON ?= python3
IMAGE ?= arprt-renderer-ical:dev
VERSION ?= dev
REVISION ?= unknown

.PHONY: setup check build image container-check

setup:
	$(GO) mod download
	$(GO) mod verify
	$(MAKE) -C experiments/compare setup

check:
	test -z "$$($(GOFMT) -l *.go cmd/*/main.go experiments/compare/compare_test.go)"
	$(GO) vet ./...
	$(GO) test -mod=readonly ./...
	$(MAKE) -C experiments/compare check
	$(PYTHON) -m unittest -v test_release

build:
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -mod=readonly -trimpath -o dist/ical-probe ./cmd/ical-probe
	CGO_ENABLED=0 $(GO) build -mod=readonly -trimpath -o dist/renderer ./cmd/renderer

image:
	docker build --provenance=false --build-arg VERSION=$(VERSION) --build-arg REVISION=$(REVISION) -t $(IMAGE) -f Dockerfile ../..

container-check:
	$(PYTHON) check_container.py --image $(IMAGE)
