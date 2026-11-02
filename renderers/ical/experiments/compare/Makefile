GO ?= go

.PHONY: setup check

setup:
	$(GO) mod download
	$(GO) mod verify

check:
	$(GO) vet ./...
	$(GO) test -mod=readonly -timeout=20s -v ./...
