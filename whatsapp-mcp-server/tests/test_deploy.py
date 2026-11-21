"""scripts/deploy.py: which processes a deploy stops."""

import importlib.util
from pathlib import Path

DEPLOY = Path(__file__).resolve().parents[2] / "scripts" / "deploy.py"


def load_deploy():
    spec = importlib.util.spec_from_file_location("deploy", DEPLOY)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_server_pids_matches_only_this_checkouts_server():
    deploy = load_deploy()
    main_py = deploy.MAIN
    python = deploy.SERVER_DIR / ".venv" / "bin" / "python"
    ps = "\n".join(
        [
            f"  101 {python} {main_py}",
            f"  102 /usr/bin/node /x/mcp-utils/dist/compat/cli.js --name whatsapp -- {python} {main_py}",
            "  103 /other/checkout/.venv/bin/python /other/checkout/whatsapp-mcp-server/main.py",
            f"  104 uv --directory {deploy.SERVER_DIR} run main.py",
            f"  105 vim {main_py}.bak",
        ]
    )

    assert deploy.server_pids(ps) == [101]
