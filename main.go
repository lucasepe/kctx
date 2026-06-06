package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/lucasepe/kctx/internal/cmd"
	"github.com/lucasepe/kctx/internal/cmd/dump"
	"github.com/lucasepe/kctx/internal/cmd/explain"
	"github.com/lucasepe/kctx/internal/cmd/graph"
	"github.com/lucasepe/kctx/internal/cmd/health"
	"github.com/lucasepe/kctx/internal/cmd/serve"
	"github.com/lucasepe/kctx/internal/cmd/trace"
	"github.com/lucasepe/x/cl"
)

const (
	appName = "kctx"
)

var (
	Version = "v0.0.0"
)

func main() {
	ctx := context.Background()

	top := flag.NewFlagSet(appName, flag.ExitOnError)
	kubeconfig := top.String("kubeconfig", "", "path to kubeconfig file")

	tool := cl.NewTool(top, appName)
	tool.Output = os.Stdout
	tool.Error = os.Stderr
	tool.Header = func(w io.Writer) {
		fmt.Fprintf(w, "╻┏ ┏━╸╺┳╸╻ ╻ (%s)\n", Version)
		fmt.Fprint(w, "┣┻┓┃   ┃ ┏╋┛  \n")
		fmt.Fprint(w, "╹ ╹┗━╸ ╹ ╹ ╹ by LS71 (https://github.com/lucasepe)\n\n")
		fmt.Fprint(w, "A Kubernetes context engine for humans and AI agents.\n")
		fmt.Fprintln(w)
	}

	tool.Footer = func(w io.Writer) {
		fmt.Fprintf(w, "Use \"%s <command> -h\" for more information about a command.\n", appName)
		fmt.Fprintln(w)
	}

	// Register tasks
	tool.Register(explain.Task(appName), "")
	tool.Register(graph.Task(appName), "")
	tool.Register(trace.Task(appName), "")
	tool.Register(health.Task(appName), "")
	tool.Register(dump.Task(appName), "")
	tool.Register(serve.Task(appName), "")

	top.Parse(cmd.NormalizeArgs(os.Args[1:]))

	env := &cmd.Env{
		Out:        os.Stdout,
		Kubeconfig: *kubeconfig,
		InCluster:  cmd.CommandName(top) == "serve",
	}
	os.Exit(int(tool.Execute(ctx, env)))
}
