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
| `whatsapp-mcp-server/main.py` | Every tool carries the four MCP annotation hints (upstream has none); `tests/test_tool_annotations.py` fails when a tool is added without being classified. Sends and read receipts are marked destructive, so Claude Desktop asks before each one. |
| `docs/TODO.md` | Open fork-local work. |

## Setup on a new machine

1. Check out next to `mcp-utils` (the hooks symlink points at `../mcp-utils`).
2. `node ../mcp-utils/scripts/sync-precommit.mjs --install` installs the git hooks.
3. `cd whatsapp-mcp-server && uv sync --extra dev`.
4. `WEBHOOK_ENABLED=false scripts/install-launchd-macos.sh` builds the bridge and installs its
   launchd jobs. Webhooks are off because nothing here listens for them.
   Set `WHATSAPP_DEVICE_NAME` too if the linked device should have a recognizable name; it
   only applies at pair time.
5. Link the phone: the bridge's `GET /api/connection` returns the pairing QR (send the token
   from `whatsapp-bridge/store/.bridge-token` as a bearer token). Scan it from WhatsApp >
   Settings > Linked Devices > Link a Device. `"state": "connected"` means it worked.
6. Register the server as
   `<checkout>/whatsapp-mcp-server/.venv/bin/python <checkout>/whatsapp-mcp-server/main.py`
   (absolute paths), not `uv run`: `scripts/deploy.py` finds the running server by that
   command line.
   - Claude Desktop: add that entry under `mcpServers` as `whatsapp`, run
     `node ../mcp-utils/scripts/install-desktop.mjs whatsapp` to route it through mcp-compat,
     then restart Desktop.
   - Claude Code: `claude mcp add -s user whatsapp -- <python> <main.py>`.

## Day to day

- Work on a branch; `main` only takes PRs. After a merge, `mpgd` pulls `main`, and the
  post-merge hook runs `scripts/deploy.py` and restarts Claude Desktop.
- Upstream changes come in through `git fetch upstream` and a `sync/upstream-<date>` branch
  (the daily task in mcp-utils does this).
- Status: `launchctl print gui/$(id -u)/com.whatsapp-mcp.bridge`, logs in
  `~/Library/Logs/whatsapp-mcp/`, settings in
  `~/Library/Application Support/whatsapp-mcp/launchd.env`.
- Reading never sends a read receipt: the read tools query the local SQLite store. Only
  `mark_messages_read` sends one.

## Building the bridge

Go links the bridge with cgo. With a conda or micromamba environment active, its `cc` shadows
Apple's and the link fails (`ld: symbol(s) not found for architecture arm64`). The `Makefile`
and `scripts/deploy.py` set `CC=/usr/bin/cc`; do the same when running `go` or the launchd
installer by hand.

## GitHub

- GitHub turns Actions off in a new fork: enable them once in the fork's Actions tab ("I
  understand my workflows"), then `gh workflow enable security.yml` for the scheduled one.
- Upstream's `release-please.yml`, `release.yml` and `dependabot-auto-merge.yml` are disabled
  in the fork (`gh workflow disable`), not deleted. `ci.yml` and `security.yml` run.
- Upstream's CI only runs for PRs into `main`, so a PR stacked on another branch shows no
  checks until it is retargeted.
- `AGENTS.md`, `CONTRIBUTING.md` and the README describe upstream's process (release-please,
  the babysit skill, `verygoodplugins` as origin). In this checkout `origin` is the fork.
- `upstream` has push disabled. PRs and issues stay in the fork.
