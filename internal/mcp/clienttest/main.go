// Command clienttest is a small MCP Streamable HTTP smoke-test client for kctx.
//
// It is intentionally dependency-free and exists to validate the transport
// without relying on a desktop agent, IDE plugin, or hosted AI product.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const protocolVersion = "2025-06-18"

func main() {
	var cfg config
	flag.StringVar(&cfg.url, "url", "http://localhost:8888/mcp", "MCP Streamable HTTP endpoint")
	flag.StringVar(&cfg.namespace, "namespace", "boutique", "namespace to inspect")
	flag.StringVar(&cfg.service, "service", "frontend", "service to trace; empty disables trace_service")
	flag.StringVar(&cfg.pod, "pod", "", "pod to graph; empty disables get_pod_graph")
	flag.BoolVar(&cfg.dumpNamespace, "dump-namespace", true, "call dump_namespace after lighter smoke-test calls")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "overall test timeout")
	flag.BoolVar(&cfg.verbose, "verbose", false, "print full JSON-RPC responses")
	flag.Parse()

	if err := run(context.Background(), cfg); err != nil {
		fmt.Fprintf(os.Stderr, "mcp-http test failed: %v\n", err)
		os.Exit(1)
	}
}

type config struct {
	url           string
	namespace     string
	service       string
	pod           string
	dumpNamespace bool
	timeout       time.Duration
	verbose       bool
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func run(ctx context.Context, cfg config) error {
	if cfg.url == "" {
		return errors.New("url is required")
	}
	if cfg.namespace == "" {
		return errors.New("namespace is required")
	}
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.timeout)
		defer cancel()
	}

	client := &http.Client{}
	sessionID, err := initialize(ctx, client, cfg)
	if err != nil {
		return err
	}
	fmt.Printf("connected: %s\n", cfg.url)
	fmt.Printf("session:   %s\n\n", sessionID)

	calls := []rpcRequest{
		{JSONRPC: "2.0", ID: 2, Method: "tools/list", Params: map[string]any{}},
		{
			JSONRPC: "2.0",
			ID:      3,
			Method:  "tools/call",
			Params: map[string]any{
				"name":      "get_namespace_health",
				"arguments": map[string]string{"namespace": cfg.namespace},
			},
		},
	}
	nextID := 4
	if cfg.service != "" {
		calls = append(calls, rpcRequest{
			JSONRPC: "2.0",
			ID:      nextID,
			Method:  "tools/call",
			Params: map[string]any{
				"name": "trace_service",
				"arguments": map[string]string{
					"namespace": cfg.namespace,
					"name":      cfg.service,
				},
			},
		})
		nextID++
	}
	if cfg.pod != "" {
		calls = append(calls, rpcRequest{
			JSONRPC: "2.0",
			ID:      nextID,
			Method:  "tools/call",
			Params: map[string]any{
				"name": "get_pod_graph",
				"arguments": map[string]string{
					"namespace": cfg.namespace,
					"name":      cfg.pod,
				},
			},
		})
		nextID++
	}
	if cfg.dumpNamespace {
		calls = append(calls, rpcRequest{
			JSONRPC: "2.0",
			ID:      nextID,
			Method:  "tools/call",
			Params: map[string]any{
				"name":      "dump_namespace",
				"arguments": map[string]string{"namespace": cfg.namespace},
			},
		})
	}

	for _, call := range calls {
		if err := postAndPrint(ctx, client, cfg.url, sessionID, cfg.verbose, call); err != nil {
			return err
		}
	}
	if err := deleteSession(ctx, client, cfg.url, sessionID); err != nil {
		return err
	}
	fmt.Println("smoke test completed")
	return nil
}

func initialize(ctx context.Context, client *http.Client, cfg config) (string, error) {
	resp, err := post(ctx, client, cfg.url, "", rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo": map[string]string{
				"name":    "kctx-mcp-smoke-test",
				"version": "dev",
			},
		},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		return "", errors.New("initialize response did not include Mcp-Session-Id")
	}
	if err := decodeRPCResponse(resp.Body); err != nil {
		return "", fmt.Errorf("initialize: %w", err)
	}
	return sessionID, nil
}

func postAndPrint(ctx context.Context, client *http.Client, url, sessionID string, verbose bool, req rpcRequest) error {
	resp, err := post(ctx, client, url, sessionID, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if err := decodeRPCResponse(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("%s: %w", req.Method, err)
	}
	if verbose {
		fmt.Printf("%s response:\n%s\n\n", req.Method, data)
		return nil
	}
	fmt.Printf("%s ok\n", req.Method)
	return nil
}

func post(ctx context.Context, client *http.Client, url, sessionID string, value any) (*http.Response, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", url, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("post status = %d: %s", resp.StatusCode, bytes.TrimSpace(data))
	}
	return resp, nil
}

func deleteSession(ctx context.Context, client *http.Client, url, sessionID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}
	req.Header.Set("Mcp-Session-Id", sessionID)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete session status = %d: %s", resp.StatusCode, bytes.TrimSpace(data))
	}
	return nil
}

func decodeRPCResponse(r io.Reader) error {
	var resp rpcResponse
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("json-rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return nil
}
