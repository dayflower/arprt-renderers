GO ?= go
GOFMT ?= gofmt
PYTHON ?= python3
IMAGE ?= arprt-renderer-rss:dev
VERSION ?= dev
REVISION ?= unknown

.PHONY: setup build check image container-check

setup:
	$(GO) mod download
	$(GO) mod verify

build:
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -mod=readonly -trimpath -ldflags="-s -w" -o dist/renderer .

check:
	test -z "$$($(GOFMT) -l *.go)"
	$(GO) vet ./...
	$(GO) test -mod=readonly ./...
	$(PYTHON) -m unittest -v test_release

image:
	docker build --provenance=false --build-arg VERSION=$(VERSION) --build-arg REVISION=$(REVISION) -t $(IMAGE) -f Dockerfile ../..

container-check:
	$(PYTHON) check_container.py --image $(IMAGE)
