package serve

import (
	"flag"
	"os"
	"testing"
)

func TestConfigureDefaultsMCPHTTPListenToLocalhost(t *testing.T) {
	unsetEnv(t, "LISTEN_ADDR")
	unsetEnv(t, "SERVE_MODE")

	cmd := New("kctx")
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	cmd.SetFlags(fs)
	if err := fs.Parse([]string{"--mode", "mcp-http"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := cmd.configure(fs); err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	if cmd.listen != "127.0.0.1:8080" {
		t.Fatalf("listen = %q, want 127.0.0.1:8080", cmd.listen)
	}
}

func TestConfigureKeepsExplicitMCPHTTPListen(t *testing.T) {
	unsetEnv(t, "LISTEN_ADDR")
	unsetEnv(t, "SERVE_MODE")

	cmd := New("kctx")
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	cmd.SetFlags(fs)
	if err := fs.Parse([]string{"--mode", "mcp-http", "--listen", ":9090"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := cmd.configure(fs); err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	if cmd.listen != ":9090" {
		t.Fatalf("listen = %q, want :9090", cmd.listen)
	}
}

func TestConfigureKeepsEnvMCPHTTPListen(t *testing.T) {
	t.Setenv("LISTEN_ADDR", ":8080")
	t.Setenv("SERVE_MODE", "")

	cmd := New("kctx")
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	cmd.SetFlags(fs)
	if err := fs.Parse([]string{"--mode", "mcp-http"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := cmd.configure(fs); err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	if cmd.listen != ":8080" {
		t.Fatalf("listen = %q, want :8080", cmd.listen)
	}
}

func TestConfigureKeepsHTTPListenDefault(t *testing.T) {
	unsetEnv(t, "LISTEN_ADDR")
	unsetEnv(t, "SERVE_MODE")

	cmd := New("kctx")
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	cmd.SetFlags(fs)
	if err := fs.Parse([]string{"--mode", "http"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := cmd.configure(fs); err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	if cmd.listen != ":8080" {
		t.Fatalf("listen = %q, want :8080", cmd.listen)
	}
}

func TestConfigureParsesMCPLimits(t *testing.T) {
	unsetEnv(t, "LISTEN_ADDR")
	unsetEnv(t, "SERVE_MODE")

	cmd := New("kctx")
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	cmd.SetFlags(fs)
	err := fs.Parse([]string{
		"--mode", "mcp-http",
		"--mcp-max-request-bytes", "2048",
		"--mcp-max-response-bytes", "4096",
		"--mcp-structured-content-max-bytes", "1024",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := cmd.configure(fs); err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	if cmd.mcpMaxRequestBytes != 2048 {
		t.Fatalf("mcpMaxRequestBytes = %d, want 2048", cmd.mcpMaxRequestBytes)
	}
	if cmd.mcpMaxResponseBytes != 4096 {
		t.Fatalf("mcpMaxResponseBytes = %d, want 4096", cmd.mcpMaxResponseBytes)
	}
	if cmd.mcpStructuredContentMaxBytes != 1024 {
		t.Fatalf("mcpStructuredContentMaxBytes = %d, want 1024", cmd.mcpStructuredContentMaxBytes)
	}
}

func TestConfigureRejectsNegativeMCPLimit(t *testing.T) {
	unsetEnv(t, "LISTEN_ADDR")
	unsetEnv(t, "SERVE_MODE")

	cmd := New("kctx")
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	cmd.SetFlags(fs)
	if err := fs.Parse([]string{"--mode", "mcp-http", "--mcp-max-response-bytes", "-1"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := cmd.configure(fs); err == nil {
		t.Fatal("configure() error = nil, want error")
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%s) error = %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}
