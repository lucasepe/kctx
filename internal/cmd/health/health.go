package health

import (
	"bytes"
	"context"
	"flag"
	"fmt"

	"github.com/lucasepe/kctx/internal/cmd"
	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/render"
	"github.com/lucasepe/x/cl"
	"github.com/lucasepe/x/log"
)

var _ cl.Task = (*Command)(nil)

func Task(appName string) cl.Task {
	return &Command{
		appName: appName,
		format:  "human",
	}
}

type Command struct {
	appName   string
	namespace string
	format    string
}

func New(appName string) *Command {
	return Task(appName).(*Command)
}

func (c *Command) Name() string {
	return "health"
}

func (c *Command) Synopsis() string {
	return "Summarize factual namespace health"
}

func (c *Command) Usage() string {
	var w bytes.Buffer
	fmt.Fprintf(&w, "%s\n\n", c.Synopsis())
	fmt.Fprint(&w, "USAGE:\n\n")
	fmt.Fprintf(&w, "  %s health namespace <namespace> [--output human|json]\n\n", c.appName)
	fmt.Fprint(&w, "DESCRIPTION:\n\n")
	fmt.Fprintln(&w, "  Produces a read-only health snapshot for one namespace using Pods,")
	fmt.Fprintln(&w, "  workloads, Services, EndpointSlices, PVCs, and recent warning Events.")
	fmt.Fprintln(&w)
	fmt.Fprint(&w, "EXAMPLES:\n\n")
	fmt.Fprintf(&w, "  %s health namespace payments\n", c.appName)
	fmt.Fprintf(&w, "  %s health namespace payments --output json\n\n", c.appName)
	return w.String()
}

func (c *Command) Ctx() context.Context {
	return context.Background()
}

func (c *Command) SetFlags(fs *flag.FlagSet) {
	cmd.OutputFlag(fs)
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
	result, err := eng.NamespaceHealth(ctx, engine.NamespaceHealthRequest{Namespace: c.namespace})
	if err != nil {
		return env.Fail(c.Name(), err, log.String("namespace", c.namespace))
	}
	if err := c.render(env, result); err != nil {
		return env.Fail(c.Name(), err, log.String("namespace", c.namespace))
	}
	return cl.ExitSuccess
}

func (c *Command) configure(fs *flag.FlagSet) error {
	namespace, err := cmd.ResourceName(fs.Args(), "namespace", c.Usage())
	if err != nil {
		return err
	}
	c.namespace = namespace
	c.format = cmd.OutputValue(fs)
	return cmd.EnsureOutputAllowed(c.Name(), c.format, c.Usage())
}

func (c *Command) render(env *cmd.Env, result *engine.NamespaceHealthResponse) error {
	switch c.format {
	case "human":
		return render.HumanNamespaceHealth(env.Out, result)
	case "json":
		return render.JSON(env.Out, result)
	default:
		return cmd.EnsureOutputAllowed(c.Name(), c.format, c.Usage())
	}
}
