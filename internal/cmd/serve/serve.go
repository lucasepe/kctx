package serve

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/lucasepe/kctx/internal/cmd"
	"github.com/lucasepe/kctx/internal/mcp"
	"github.com/lucasepe/kctx/internal/server"
	"github.com/lucasepe/kctx/internal/util/logger"
	"github.com/lucasepe/x/cl"
)

var _ cl.Task = (*Command)(nil)

func Task(appName string) cl.Task {
	return TaskWithVersion(appName, "dev")
}

// TaskWithVersion creates the serve command and advertises version through
// agent-facing protocols such as MCP.
func TaskWithVersion(appName, version string) cl.Task {
	return &Command{
		appName: appName,
		version: version,
		listen:  ":8080",
	}
}

type Command struct {
	appName                      string
	version                      string
	listen                       string
	mode                         string
	requestTimeout               time.Duration
	kubeAPIBudget                int
	mcpMaxRequestBytes           int64
	mcpMaxResponseBytes          int64
	mcpStructuredContentMaxBytes int64
	verbose                      bool
}

func New(appName string) *Command {
	return Task(appName).(*Command)
}

func (c *Command) Name() string {
	return "serve"
}

func (c *Command) Synopsis() string {
	return "Expose kctx as a local read-only HTTP or MCP interface"
}

func (c *Command) Usage() string {
	var w bytes.Buffer
	fmt.Fprintf(&w, "%s\n\n", c.Synopsis())
	fmt.Fprint(&w, "USAGE:\n\n")
	fmt.Fprintf(&w, "  %s serve [--mode http|mcp|mcp-http] [--listen :8080] [--request-timeout 30s] [--kube-api-budget 100] [--verbose]\n\n", c.appName)
	fmt.Fprint(&w, "DESCRIPTION:\n\n")
	fmt.Fprintln(&w, "  Starts a lightweight local server whose surfaces mirror the CLI commands.")
	fmt.Fprintln(&w, "  The default HTTP mode exposes explicit routes for Pod context, Pod graphs,")
	fmt.Fprintln(&w, "  Service traces, namespace health, and namespace dumps. MCP stdio mode")
	fmt.Fprintln(&w, "  exposes read-only tools for local agents, while MCP HTTP mode exposes the")
	fmt.Fprintln(&w, "  same tools over Streamable HTTP at /mcp. kctx is not a generic Kubernetes REST proxy and")
	fmt.Fprintln(&w, "  it does not expose arbitrary CRD semantics.")
	fmt.Fprintln(&w)
	fmt.Fprint(&w, "OPTIONS:\n\n")
	fmt.Fprintln(&w, "  --mode              Serve mode: http, mcp, or mcp-http (env: SERVE_MODE)")
	fmt.Fprintln(&w, "  --listen            HTTP listen address; MCP HTTP defaults to 127.0.0.1:8080 unless set (env: LISTEN_ADDR)")
	fmt.Fprintln(&w, "  --request-timeout   Per-request timeout; 0 disables it (env: REQUEST_TIMEOUT)")
	fmt.Fprintln(&w, "  --kube-api-budget   Kubernetes API calls per request; 0 disables it (env: KUBE_API_BUDGET)")
	fmt.Fprintln(&w, "  --mcp-max-request-bytes             MCP HTTP request body limit; 0 disables it (env: MCP_MAX_REQUEST_BYTES)")
	fmt.Fprintln(&w, "  --mcp-max-response-bytes            MCP HTTP response limit; 0 disables it (env: MCP_MAX_RESPONSE_BYTES)")
	fmt.Fprintln(&w, "  --mcp-structured-content-max-bytes  Omit structuredContent above this compact JSON size; 0 always includes it (env: MCP_STRUCTURED_CONTENT_MAX_BYTES)")
	fmt.Fprintln(&w, "  --verbose           Enable debug logging (env: VERBOSE)")
	fmt.Fprintln(&w)
	fmt.Fprint(&w, "EXAMPLES:\n\n")
	fmt.Fprintf(&w, "  %s serve\n", c.appName)
	fmt.Fprintf(&w, "  %s serve --mode mcp\n", c.appName)
	fmt.Fprintf(&w, "  %s serve --mode mcp-http --listen :8080\n", c.appName)
	fmt.Fprintf(&w, "  %s serve --verbose\n", c.appName)
	fmt.Fprintf(&w, "  %s serve --listen :9090\n\n", c.appName)
	return w.String()
}

func (c *Command) Ctx() context.Context {
	return context.Background()
}

func (c *Command) SetFlags(fs *flag.FlagSet) {
	cmd.ServeModeFlag(fs)
	cmd.ListenFlag(fs)
	cmd.RequestTimeoutFlag(fs)
	cmd.KubeAPIBudgetFlag(fs)
	cmd.MCPMaxRequestBytesFlag(fs)
	cmd.MCPMaxResponseBytesFlag(fs)
	cmd.MCPStructuredContentMaxBytesFlag(fs)
	cmd.VerboseFlag(fs)
}

func (c *Command) Execute(ctx context.Context, fs *flag.FlagSet, args ...any) cl.ExitStatus {
	env, err := cmd.EnvFrom(args...)
	if err != nil {
		return cl.ExitFailure
	}
	if err := c.configure(fs); err != nil {
		return env.Fail(c.Name(), err)
	}

	eng, err := env.Engine()
	if err != nil {
		return env.Fail(c.Name(), err)
	}

	log := logger.New("kctx", c.verbose)
	switch c.mode {
	case "http":
		err = server.New(eng,
			log,
			server.WithRequestTimeout(c.requestTimeout),
			server.WithKubeAPIBudget(c.kubeAPIBudget)).
			ListenAndServe(ctx, c.listen)
	case "mcp":
		err = mcp.New(eng,
			log,
			mcp.WithVersion(c.version),
			mcp.WithRequestTimeout(c.requestTimeout),
			mcp.WithKubeAPIBudget(c.kubeAPIBudget),
			mcp.WithMaxToolResultBytes(c.mcpMaxResponseBytes),
			mcp.WithStructuredContentMaxBytes(c.mcpStructuredContentMaxBytes)).
			Serve(ctx, os.Stdin, os.Stdout)
	case "mcp-http":
		protocol := mcp.New(eng,
			log,
			mcp.WithVersion(c.version),
			mcp.WithRequestTimeout(c.requestTimeout),
			mcp.WithKubeAPIBudget(c.kubeAPIBudget),
			mcp.WithMaxToolResultBytes(c.mcpMaxResponseBytes),
			mcp.WithStructuredContentMaxBytes(c.mcpStructuredContentMaxBytes))
		err = mcp.NewHTTPServer(protocol, log,
			mcp.WithMaxRequestBytes(c.mcpMaxRequestBytes),
			mcp.WithMaxResponseBytes(c.mcpMaxResponseBytes)).
			ListenAndServe(ctx, c.listen)
	default:
		err = fmt.Errorf("invalid serve mode %q: want http, mcp, or mcp-http", c.mode)
	}
	if err != nil {
		return env.Fail(c.Name(), err)
	}

	return cl.ExitSuccess
}

func (c *Command) configure(fs *flag.FlagSet) error {
	if err := cmd.NoArgs(fs.Args(), c.Usage()); err != nil {
		return err
	}
	c.mode = cmd.StringValue(fs, "mode")
	c.listen = cmd.StringValue(fs, "listen")
	if isHTTPMCPMode(c.mode) && !flagWasSet(fs, "listen") {
		if _, ok := os.LookupEnv("LISTEN_ADDR"); !ok {
			c.listen = "127.0.0.1:8080"
		}
	}
	requestTimeout, err := cmd.DurationValue(fs, "request-timeout")
	if err != nil {
		return fmt.Errorf("invalid request timeout: %w", err)
	}
	if requestTimeout < 0 {
		return fmt.Errorf("invalid request timeout: must be non-negative")
	}
	c.requestTimeout = requestTimeout
	kubeAPIBudget, err := cmd.IntValue(fs, "kube-api-budget")
	if err != nil {
		return fmt.Errorf("invalid Kubernetes API budget: %w", err)
	}
	if kubeAPIBudget < 0 {
		return fmt.Errorf("invalid Kubernetes API budget: must be non-negative")
	}
	c.kubeAPIBudget = kubeAPIBudget
	mcpMaxRequestBytes, err := cmd.Int64Value(fs, "mcp-max-request-bytes")
	if err != nil {
		return fmt.Errorf("invalid MCP max request bytes: %w", err)
	}
	if mcpMaxRequestBytes < 0 {
		return fmt.Errorf("invalid MCP max request bytes: must be non-negative")
	}
	c.mcpMaxRequestBytes = mcpMaxRequestBytes
	mcpMaxResponseBytes, err := cmd.Int64Value(fs, "mcp-max-response-bytes")
	if err != nil {
		return fmt.Errorf("invalid MCP max response bytes: %w", err)
	}
	if mcpMaxResponseBytes < 0 {
		return fmt.Errorf("invalid MCP max response bytes: must be non-negative")
	}
	c.mcpMaxResponseBytes = mcpMaxResponseBytes
	mcpStructuredContentMaxBytes, err := cmd.Int64Value(fs, "mcp-structured-content-max-bytes")
	if err != nil {
		return fmt.Errorf("invalid MCP structured content max bytes: %w", err)
	}
	if mcpStructuredContentMaxBytes < 0 {
		return fmt.Errorf("invalid MCP structured content max bytes: must be non-negative")
	}
	c.mcpStructuredContentMaxBytes = mcpStructuredContentMaxBytes
	c.verbose = cmd.BoolValue(fs, "verbose")
	return nil
}

func isHTTPMCPMode(mode string) bool {
	return mode == "mcp-http"
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	visited := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			visited = true
		}
	})
	return visited
}
