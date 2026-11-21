# Fork notes (akseldkw07/whatsapp-mcp)

This is a personal fork of `verygoodplugins/whatsapp-mcp`. It keeps upstream's layout (Go
bridge, uv-managed Python server, ruff) so `git merge upstream/main` stays clean. Everything
fork-specific is listed here. The fork is public: never commit `.env`, the message store,
logs or phone numbers.

## What the fork adds

| File | Purpose |
| --- | --- |
| `.pre-commit-config.yaml` | Symlink to `../mcp-utils/precommit/mcp.yaml`, the shared hooks for the sibling MCP repos. Change hooks there, never here. |
| `Makefile` | `make test` (Go and Python tests) is what the shared pre-push hook runs. `make test-launchd` runs upstream's slower installer test. `make build`, `make deploy`. |
| `scripts/deploy.py` | Run by the shared post-merge hook on `main`: installs the locked Python dependencies, rebuilds the bridge and restarts its launchd jobs through upstream's `scripts/install-launchd-macos.sh`, then stops the running MCP server so mcp-compat starts the new code. |
| `whatsapp-mcp-server/pyproject.toml` | `[tool.black] line-length` matches ruff's; `tdd-guard-pytest` and `tdd_guard_project_root` report test runs to the tdd-guard Claude Code plugin. |
| `pyproject.toml` (root) | Tool settings only, for files outside `whatsapp-mcp-server/`: black's line length and pyright's path to the server's `.venv`. |
| `docs/TODO.md` | Open fork-local work. |

## Setup on a new machine

1. Check out next to `mcp-utils` (the hooks symlink points at `../mcp-utils`).
2. `node ../mcp-utils/scripts/sync-precommit.mjs --install` installs the git hooks.
3. `cd whatsapp-mcp-server && uv sync --extra dev`.
4. `WEBHOOK_ENABLED=false scripts/install-launchd-macos.sh` builds the bridge and installs its
   launchd jobs. Webhooks are off because nothing here listens for them.
5. Link the phone: the bridge's `GET /api/connection` returns the pairing QR.
6. Register the server with the MCP client as
   `<checkout>/whatsapp-mcp-server/.venv/bin/python <checkout>/whatsapp-mcp-server/main.py`
   (absolute paths). `scripts/deploy.py` finds the running server by that command line.

## Building the bridge

Go links the bridge with cgo. With a conda or micromamba environment active, its `cc` shadows
Apple's and the link fails (`ld: symbol(s) not found for architecture arm64`). The `Makefile`
and `scripts/deploy.py` set `CC=/usr/bin/cc`; do the same when running `go` or the launchd
installer by hand.

## GitHub

- Upstream's `release-please.yml` and `release.yml` are disabled in the fork's Actions
  settings, not deleted.
- `upstream` has push disabled. PRs and issues stay in the fork.
