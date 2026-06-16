# kctx MCP

`kctx` exposes a small read-only Model Context Protocol surface for AI-agent
workflows.

Supported serve modes:

```bash
kctx serve --mode mcp
kctx serve --mode mcp-http
```

`mcp` uses stdio for local agent integrations. `mcp-http` exposes the same tools
over the MCP Streamable HTTP endpoint:

```text
http://localhost:8080/mcp
```

The Streamable HTTP endpoint returns a `Mcp-Session-Id` header from
`initialize`. Clients must include that header on subsequent `/mcp` requests,
and may terminate the session with `DELETE /mcp`.

For browser-originated requests, the server validates `Origin` and rejects
cross-origin requests that are neither same-host nor loopback. Clients may send
`MCP-Protocol-Version`; unsupported values are rejected with `400 Bad Request`.
When omitted, the server accepts the request for backwards compatibility.

For large namespaces, tune these limits before assuming the client is broken:

- `REQUEST_TIMEOUT`: per-tool-call deadline.
- `KUBE_API_BUDGET`: Kubernetes API operations per tool call.
- `MCP_MAX_REQUEST_BYTES`: maximum incoming JSON-RPC request body.
- `MCP_MAX_RESPONSE_BYTES`: maximum outgoing JSON-RPC response.
- `MCP_STRUCTURED_CONTENT_MAX_BYTES`: above this compact JSON size, the text
  content becomes a short summary while the full payload remains in
  `structuredContent`.

Current tools:

- `get_namespace_health`
- `explain_resource`
- `trace_service`
- `get_pod_graph`
- `dump_namespace`

The detailed release-chart-first MCP HTTP test guide covers the Streamable HTTP
endpoint:

- [MCP HTTP Release Test Guide](docs/mcp-sse/01-overview.md)

The guide does not require cloning this repository. It starts from the released
Helm chart package and uses standalone shell snippets for kind, Online Boutique,
smoke testing, ngrok, Codex, and Claude Code.

## Security Boundary

The MCP HTTP server is read-only, but it does not yet include built-in
AuthN/AuthZ. Use it only in local labs, trusted internal networks, or behind an
external access-control layer.
