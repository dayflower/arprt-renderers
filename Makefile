UV ?= uv
RUFF_VERSION := 0.17.1
RUFF := $(UV) tool run --from 'ruff==$(RUFF_VERSION)' ruff
RUFF_PATHS ?= .
RENDERER_DIRS := $(patsubst %/Makefile,%,$(wildcard renderers/*/Makefile))

.PHONY: setup lint format format-check test check

setup:
	@for directory in $(RENDERER_DIRS); do \
		$(MAKE) -C "$$directory" setup || exit $$?; \
	done

lint:
	$(RUFF) check $(RUFF_PATHS)

format:
	$(RUFF) check --select I --fix $(RUFF_PATHS)
	$(RUFF) format $(RUFF_PATHS)

format-check:
	$(RUFF) format --check $(RUFF_PATHS)

test:
	@for directory in $(RENDERER_DIRS); do \
		$(MAKE) -C "$$directory" check || exit $$?; \
	done

check: lint format-check test
