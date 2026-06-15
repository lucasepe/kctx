# kctx MCP

`kctx` exposes a small read-only Model Context Protocol surface for AI-agent
workflows.

Supported serve modes:

```bash
kctx serve --mode mcp
kctx serve --mode mcp-sse
```

Current tools:

- `get_namespace_health`
- `explain_resource`
- `trace_service`
- `get_pod_graph`
- `dump_namespace`

The detailed release-chart-first MCP/SSE test guide is the source of truth for
community testing:

- [MCP/SSE Release Test Guide](docs/mcp-sse/01-overview.md)

The guide does not require cloning this repository. It starts from the released
Helm chart package and uses standalone shell snippets for kind, Online Boutique,
smoke testing, ngrok, Codex, and Claude Code.

## Security Boundary

The MCP HTTP/SSE server is read-only, but it does not yet include built-in
AuthN/AuthZ. Use it only in local labs, trusted internal networks, or behind an
external access-control layer.

