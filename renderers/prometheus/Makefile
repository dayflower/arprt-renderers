GO ?= go
GOFMT ?= gofmt

.PHONY: setup check container-check

setup:
	$(GO) mod download
	$(GO) mod verify

check:
	test -z "$$($(GOFMT) -l *.go)"
	$(GO) vet ./...
	$(GO) test -mod=readonly ./...

container-check:
	@echo 'No production runtime exists yet; container validation is unavailable.' >&2
	@exit 1
