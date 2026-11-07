GO ?= go
GOFMT ?= gofmt

.PHONY: setup check build container-check

setup:
	$(GO) mod download
	$(GO) mod verify

check:
	test -z "$$($(GOFMT) -l *.go cmd/renderer/*.go)"
	$(GO) vet ./...
	$(GO) test -mod=readonly ./...

container-check:
	@echo 'No production image exists yet; container validation is unavailable.' >&2
	@exit 1

build:
	CGO_ENABLED=0 $(GO) build -trimpath -o dist/renderer ./cmd/renderer
