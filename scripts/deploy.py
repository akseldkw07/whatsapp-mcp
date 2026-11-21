"""Deploy this checkout: the Go bridge and the Python MCP server. Standard library only.

Run by the shared post-merge hook on main (mcp-utils/precommit/hooks/post-merge), or by hand:
`make deploy`.

1. Installs the locked Python dependencies.
2. Rebuilds the bridge and restarts its launchd jobs by re-running upstream's
   scripts/install-launchd-macos.sh, which keeps the saved settings and the store. If the
   jobs were never installed, it only builds the binary.
3. Stops the running MCP server process. mcp-compat (which Claude Desktop talks to) starts
   the new code on the next tool call.
"""

import os
import signal
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SERVER_DIR = ROOT / "whatsapp-mcp-server"
MAIN = SERVER_DIR / "main.py"
BRIDGE_DIR = ROOT / "whatsapp-bridge"
BRIDGE_PLIST = Path.home() / "Library" / "LaunchAgents" / "com.whatsapp-mcp.bridge.plist"
SYSTEM_CC = Path("/usr/bin/cc")


def server_pids(ps_output: str) -> list[int]:
    """PIDs of this checkout's MCP server in `ps -axo pid=,args=` output.

    Only the server itself (argv ends with this checkout's main.py), never the mcp-compat
    proxy in front of it, whose argv also contains that path.
    """
    pids = []
    for line in ps_output.splitlines():
        pid, _, args = line.strip().partition(" ")
        args = args.strip()
        if args.endswith(f" {MAIN}") and "compat/cli.js" not in args:
            pids.append(int(pid))
    return pids


def build_env() -> dict[str, str]:
    """Go's cgo link must use Apple's compiler: a conda/micromamba `cc` on PATH fails to link."""
    env = dict(os.environ)
    if sys.platform == "darwin" and SYSTEM_CC.exists():
        env["CC"] = str(SYSTEM_CC)
    return env


def main() -> None:
    subprocess.run(["uv", "sync", "--locked", "--extra", "dev"], cwd=SERVER_DIR, check=True)

    if BRIDGE_PLIST.exists():
        subprocess.run(
            [str(ROOT / "scripts" / "install-launchd-macos.sh")],
            env=build_env(),
            check=True,
        )
        bridge = "bridge rebuilt and its launchd jobs restarted"
    else:
        subprocess.run(
            ["go", "build", "-o", "whatsapp-bridge", "."],
            cwd=BRIDGE_DIR,
            env=build_env(),
            check=True,
        )
        bridge = "bridge rebuilt (launchd jobs not installed: run scripts/install-launchd-macos.sh)"

    ps = subprocess.run(["ps", "-axo", "pid=,args="], capture_output=True, text=True, check=True)
    pids = server_pids(ps.stdout)
    for pid in pids:
        os.kill(pid, signal.SIGTERM)

    print(
        f"Deployed: {bridge}; stopped {len(pids)} MCP server process(es), "
        "mcp-compat starts the new code on the next tool call.",
        file=sys.stderr,
    )


if __name__ == "__main__":
    main()
