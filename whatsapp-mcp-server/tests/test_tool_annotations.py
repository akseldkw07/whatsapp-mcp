"""Every tool declares all four MCP annotation hints, so clients can tell reads from sends."""

import asyncio

import main

# Reads of the local SQLite store: never leave this machine.
READ_LOCAL = {
    "search_contacts",
    "get_contact",
    "list_messages",
    "list_chats",
    "get_chat",
    "get_direct_chat_by_contact",
    "get_contact_chats",
    "get_last_interaction",
    "get_message_context",
}
# Store a file or a transcript on this machine; nothing is sent to anyone in the chat.
WRITE_LOCAL = {"download_media", "view_media", "transcribe_audio"}
# Reversible change visible to others: a reaction can be removed.
WRITE = {"send_reaction"}
# Outward-facing and irreversible: a message or a read receipt cannot be taken back.
DESTRUCTIVE = {"send_message", "send_file", "send_audio_message", "mark_messages_read"}

EXPECTED = {
    **{name: (True, False, True, False) for name in READ_LOCAL},
    **{name: (False, False, False, False) for name in WRITE_LOCAL},
    **{name: (False, False, False, True) for name in WRITE},
    **{name: (False, True, False, True) for name in DESTRUCTIVE},
}


def hints(tool):
    a = tool.annotations
    assert a is not None, f"{tool.name} has no annotations"
    return (a.readOnlyHint, a.destructiveHint, a.idempotentHint, a.openWorldHint)


def test_every_tool_is_annotated_as_expected():
    tools = {tool.name: tool for tool in asyncio.run(main.mcp.list_tools())}

    assert set(tools) == set(EXPECTED), "a tool was added or removed: classify it here"
    assert {name: hints(tool) for name, tool in tools.items()} == EXPECTED
