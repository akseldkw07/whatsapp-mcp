# Fork-local entry points for the shared MCP hooks (see docs/FORK.md).
# `make test` is what the pre-push hook runs (mcp-utils/precommit/hooks/run-tests).

# Go's cgo link needs Apple's compiler: a conda/micromamba `cc` on PATH fails to link.
ifneq ($(wildcard /usr/bin/cc),)
export CC := /usr/bin/cc
endif

.PHONY: test test-go test-python test-launchd build deploy

test: test-go test-python

test-go:
	cd whatsapp-bridge && go test ./...

test-python:
	cd whatsapp-mcp-server && uv run --extra dev pytest -q

# Upstream's launchd installer test: about 40 s, so not part of `test`.
test-launchd:
	zsh scripts/tests/test-launchd-macos.sh

build:
	cd whatsapp-bridge && go build -o whatsapp-bridge .

deploy:
	python3 scripts/deploy.py
