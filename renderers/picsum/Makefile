UV ?= uv
PYTHON ?= .venv/bin/python
IMAGE ?= arprt-renderer-picsum:dev
VERSION ?= dev
REVISION ?= unknown

.PHONY: setup check image container-check

setup:
	$(UV) venv --allow-existing --python 3.13 .venv
	$(UV) pip sync --python $(PYTHON) requirements.txt

check:
	$(PYTHON) -m unittest discover -s . -v

image:
	docker build --provenance=false --build-arg VERSION=$(VERSION) --build-arg REVISION=$(REVISION) -t $(IMAGE) -f Dockerfile ../..

container-check:
	python3 check_container.py --image $(IMAGE)
