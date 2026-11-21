# TODO (fork-local)

- [ ] Fence untrusted third-party text. Message text, contact names, group names and file
  names reach the model unfenced from every read tool (`list_messages`, `list_chats`,
  `search_contacts`, `get_message_context`, ...). Wrap them with the mcp-utils fence, the way
  `imessage-mcp` does (`server/untrusted.js`). The Python `mcp_utils.untrusted` module gives
  byte-identical output to the JS one. Using it makes this repo depend on mcp-utils, so CI
  then needs read access to it (mcp-utils README, "CI access to mcp-utils").
- [ ] Add a connection-status MCP tool. The bridge exposes `GET /api/connection` and
  `GET /api/health`, but no tool reports them, so a client can't tell "not linked" from "no
  messages". Needs the owner's OK: it is a new tool, and upstream may add one.
