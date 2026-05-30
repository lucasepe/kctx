package serve

import (
	"bytes"
	"context"
	"flag"
	"fmt"

	"github.com/lucasepe/kctx/internal/cmd"
	"github.com/lucasepe/kctx/internal/server"
	"github.com/lucasepe/kctx/internal/util/logger"
	"github.com/lucasepe/x/cl"
)

var _ cl.Task = (*Command)(nil)

func Task(appName string) cl.Task {
	return &Command{
		appName: appName,
		listen:  ":8080",
	}
}

type Command struct {
	appName string
	listen  string
	verbose bool
}

func New(appName string) *Command {
	return Task(appName).(*Command)
}

func (c *Command) Name() string {
	return "serve"
}

func (c *Command) Synopsis() string {
	return "Expose kctx as a local read-only HTTP interface"
}

func (c *Command) Usage() string {
	var w bytes.Buffer
	fmt.Fprintf(&w, "%s\n\n", c.Synopsis())
	fmt.Fprint(&w, "USAGE:\n\n")
	fmt.Fprintf(&w, "  %s serve [--listen :8080] [--verbose]\n\n", c.appName)
	fmt.Fprint(&w, "DESCRIPTION:\n\n")
	fmt.Fprintln(&w, "  Starts a lightweight local HTTP server whose routes mirror the CLI commands.")
	fmt.Fprintln(&w, "  The HTTP surface stays explicit and operational: Pod context, Pod graphs,")
	fmt.Fprintln(&w, "  Service traces, namespace health, and namespace dumps. It is not a generic")
	fmt.Fprintln(&w, "  Kubernetes REST proxy and it does not expose arbitrary CRD semantics.")
	fmt.Fprintln(&w)
	fmt.Fprint(&w, "OPTIONS:\n\n")
	fmt.Fprintln(&w, "  --listen    HTTP listen address (env: LISTEN_ADDR)")
	fmt.Fprintln(&w, "  --verbose   Enable debug logging (env: VERBOSE)")
	fmt.Fprintln(&w)
	fmt.Fprint(&w, "EXAMPLES:\n\n")
	fmt.Fprintf(&w, "  %s serve\n", c.appName)
	fmt.Fprintf(&w, "  %s serve --verbose\n", c.appName)
	fmt.Fprintf(&w, "  %s serve --listen :9090\n\n", c.appName)
	return w.String()
}

func (c *Command) Ctx() context.Context {
	return context.Background()
}

func (c *Command) SetFlags(fs *flag.FlagSet) {
	cmd.ListenFlag(fs)
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

	err = server.New(eng,
		logger.New("kctx", c.verbose)).
		ListenAndServe(ctx, c.listen)
	if err != nil {
		return env.Fail(c.Name(), err)
	}

	return cl.ExitSuccess
}

func (c *Command) configure(fs *flag.FlagSet) error {
	if err := cmd.NoArgs(fs.Args(), c.Usage()); err != nil {
		return err
	}
	c.listen = cmd.StringValue(fs, "listen")
	c.verbose = cmd.BoolValue(fs, "verbose")
	return nil
}
