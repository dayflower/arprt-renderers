GO ?= go
GOFMT ?= gofmt
PYTHON ?= python3
IMAGE ?= arprt-renderer-prometheus:dev
VERSION ?= dev
REVISION ?= unknown

.PHONY: setup check build image container-check

setup:
	$(GO) mod download
	$(GO) mod verify

check:
	test -z "$$($(GOFMT) -l *.go cmd/renderer/*.go)"
	$(GO) vet ./...
	$(GO) test -mod=readonly ./...
	$(PYTHON) -m unittest -v test_release

container-check:
	$(PYTHON) check_container.py --image $(IMAGE)

build:
	CGO_ENABLED=0 $(GO) build -mod=readonly -trimpath -o dist/renderer ./cmd/renderer

image:
	docker build --provenance=false --build-arg VERSION=$(VERSION) --build-arg REVISION=$(REVISION) -t $(IMAGE) -f Dockerfile ../..
