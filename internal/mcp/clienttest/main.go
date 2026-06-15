// Command clienttest is a small MCP HTTP/SSE smoke-test client for kctx.
//
// It is intentionally dependency-free and exists to validate the transport
// without relying on a desktop agent, IDE plugin, or hosted AI product.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	var cfg config
	flag.StringVar(&cfg.url, "url", "http://localhost:8888/mcp/sse", "MCP SSE endpoint")
	flag.StringVar(&cfg.namespace, "namespace", "boutique", "namespace to inspect")
	flag.StringVar(&cfg.service, "service", "frontend", "service to trace; empty disables trace_service")
	flag.StringVar(&cfg.pod, "pod", "", "pod to graph; empty disables get_pod_graph")
	flag.BoolVar(&cfg.dumpNamespace, "dump-namespace", true, "call dump_namespace after lighter smoke-test calls")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "overall test timeout")
	flag.BoolVar(&cfg.verbose, "verbose", false, "print full JSON-RPC responses")
	flag.Parse()

	if err := run(context.Background(), cfg); err != nil {
		fmt.Fprintf(os.Stderr, "mcp-sse test failed: %v\n", err)
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
	stream, err := openSSE(ctx, client, cfg.url)
	if err != nil {
		return err
	}
	defer stream.close()

	endpoint, err := stream.waitForEndpoint()
	if err != nil {
		return err
	}
	messageURL, err := resolveEndpoint(cfg.url, endpoint)
	if err != nil {
		return err
	}

	fmt.Printf("connected: %s\n", cfg.url)
	fmt.Printf("session:   %s\n\n", endpoint)

	if err := postAndPrint(ctx, client, stream, messageURL, cfg.verbose, rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]string{
				"name":    "kctx-mcp-smoke-test",
				"version": "dev",
			},
		},
	}); err != nil {
		return err
	}

	if err := postAndPrint(ctx, client, stream, messageURL, cfg.verbose, rpcRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
		Params:  map[string]any{},
	}); err != nil {
		return err
	}

	if err := postAndPrint(ctx, client, stream, messageURL, cfg.verbose, rpcRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "get_namespace_health",
			"arguments": map[string]string{
				"namespace": cfg.namespace,
			},
		},
	}); err != nil {
		return err
	}

	nextID := 4
	if cfg.service != "" {
		if err := postAndPrint(ctx, client, stream, messageURL, cfg.verbose, rpcRequest{
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
		}); err != nil {
			return err
		}
		nextID++
	}

	if cfg.pod != "" {
		if err := postAndPrint(ctx, client, stream, messageURL, cfg.verbose, rpcRequest{
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
		}); err != nil {
			return err
		}
		nextID++
	}

	if cfg.dumpNamespace {
		if err := postAndPrint(ctx, client, stream, messageURL, cfg.verbose, rpcRequest{
			JSONRPC: "2.0",
			ID:      nextID,
			Method:  "tools/call",
			Params: map[string]any{
				"name": "dump_namespace",
				"arguments": map[string]string{
					"namespace": cfg.namespace,
				},
			},
		}); err != nil {
			return err
		}
	}

	return nil
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

func postAndPrint(ctx context.Context, client *http.Client, stream *sseStream, endpoint string, verbose bool, req rpcRequest) error {
	fmt.Printf(">>> %s\n", req.Method)
	if err := postRPC(ctx, client, endpoint, req); err != nil {
		return err
	}
	event, err := stream.nextMessage()
	if err != nil {
		return err
	}
	printResponse(event.Data, verbose)
	fmt.Println()
	return nil
}

func postRPC(ctx context.Context, client *http.Client, endpoint string, req rpcRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode %s request: %w", req.Method, err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create %s request: %w", req.Method, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("post %s: %w", req.Method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("post %s status = %d, want 202: %s", req.Method, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func printJSON(raw string) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		fmt.Println(raw)
		return
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Println(raw)
		return
	}
	fmt.Println(string(data))
}

func printResponse(raw string, verbose bool) {
	if verbose {
		printJSON(raw)
		return
	}

	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		fmt.Println(raw)
		return
	}
	if errValue, ok := resp["error"]; ok {
		fmt.Printf("error: %v\n", errValue)
		return
	}

	result, _ := resp["result"].(map[string]any)
	if result == nil {
		fmt.Println("ok")
		return
	}

	if serverInfo, ok := result["serverInfo"].(map[string]any); ok {
		fmt.Printf("server: %v %v\n", serverInfo["name"], serverInfo["version"])
		fmt.Printf("protocol: %v\n", result["protocolVersion"])
		return
	}

	if tools, ok := result["tools"].([]any); ok {
		fmt.Printf("tools: %d\n", len(tools))
		for _, item := range tools {
			tool, _ := item.(map[string]any)
			if tool != nil {
				fmt.Printf("- %v\n", tool["name"])
			}
		}
		return
	}

	isError, _ := result["isError"].(bool)
	structured, _ := result["structuredContent"].(map[string]any)
	if structured == nil {
		fmt.Printf("tool result: isError=%v\n", isError)
		return
	}
	fmt.Printf("tool result: kind=%v isError=%v\n", structured["kind"], isError)
	if ns, ok := structured["namespace"].(string); ok {
		fmt.Printf("namespace: %s\n", ns)
	}
	if service, ok := structured["service"].(map[string]any); ok {
		fmt.Printf("service: %v/%v\n", service["namespace"], service["name"])
	}
	if summary, ok := structured["summary"].(map[string]any); ok {
		printSummary(summary)
	}
	printCollectionSize("nodes", structured)
	printCollectionSize("edges", structured)
	printCollectionSize("entities", structured)
	printCollectionSize("relations", structured)
	printCollectionSize("endpoints", structured)
	printCollectionSize("pods", structured)
	printCollectionSize("signals", structured)
	printCollectionSize("events", structured)
}

func printSummary(summary map[string]any) {
	fmt.Println("summary:")
	for _, key := range []string{"podsTotal", "podsReady", "podsNotReady", "servicesTotal", "servicesWithoutEndpoints", "warningEvents", "errorSignals"} {
		if value, ok := summary[key]; ok {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}
}

func printCollectionSize(name string, value map[string]any) {
	items, ok := value[name].([]any)
	if ok {
		fmt.Printf("%s: %d\n", name, len(items))
	}
}

func resolveEndpoint(base, endpoint string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse endpoint URL: %w", err)
	}
	return baseURL.ResolveReference(endpointURL).String(), nil
}

func openSSE(ctx context.Context, client *http.Client, endpoint string) (*sseStream, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open SSE stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("open SSE stream status = %d, want 200: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return &sseStream{body: resp.Body, reader: bufio.NewReader(resp.Body)}, nil
}

type sseStream struct {
	body   io.Closer
	reader *bufio.Reader
}

func (s *sseStream) close() {
	_ = s.body.Close()
}

func (s *sseStream) waitForEndpoint() (string, error) {
	for {
		event, err := s.readEvent()
		if err != nil {
			return "", err
		}
		if event.Name == "endpoint" {
			return event.Data, nil
		}
	}
}

func (s *sseStream) nextMessage() (sseEvent, error) {
	for {
		event, err := s.readEvent()
		if err != nil {
			return sseEvent{}, err
		}
		if event.Name == "message" {
			return event, nil
		}
	}
}

func (s *sseStream) readEvent() (sseEvent, error) {
	var event sseEvent
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return sseEvent{}, fmt.Errorf("read SSE event: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if event.Name != "" || event.Data != "" {
				return event, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			event.Name = strings.TrimPrefix(line, "event: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			if event.Data == "" {
				event.Data = strings.TrimPrefix(line, "data: ")
			} else {
				event.Data += "\n" + strings.TrimPrefix(line, "data: ")
			}
		}
	}
}

type sseEvent struct {
	Name string
	Data string
}
