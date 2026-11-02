GO ?= go
GOFMT ?= gofmt
PYTHON ?= python3
IMAGE ?= arprt-renderer-ical-semantics:dev

.PHONY: setup check build image container-check

setup:
	$(GO) mod download
	$(GO) mod verify
	$(MAKE) -C experiments/compare setup

check:
	test -z "$$($(GOFMT) -l semantic.go semantic_test.go cmd/ical-probe/main.go experiments/compare/compare_test.go)"
	$(GO) vet ./...
	$(GO) test -mod=readonly ./...
	$(MAKE) -C experiments/compare check

build:
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -mod=readonly -trimpath -o dist/ical-probe ./cmd/ical-probe

image:
	docker build --provenance=false -t $(IMAGE) -f Dockerfile ../..

# This is a semantics verification image, not the future production renderer.
container-check:
	$(PYTHON) check_container.py --image $(IMAGE)
