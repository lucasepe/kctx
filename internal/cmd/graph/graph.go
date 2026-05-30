package graph

import (
	"bytes"
	"context"
	"flag"
	"fmt"

	"github.com/lucasepe/kctx/internal/cmd"
	"github.com/lucasepe/kctx/internal/engine"
	modelgraph "github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/render"
	"github.com/lucasepe/x/cl"
	"github.com/lucasepe/x/log"
)

var _ cl.Task = (*Command)(nil)

func Task(appName string) cl.Task {
	return &Command{
		appName:   appName,
		namespace: "default",
		format:    "human",
	}
}

type Command struct {
	appName      string
	namespace    string
	resourceName string
	format       string
}

func New(appName string) *Command {
	return Task(appName).(*Command)
}

func (c *Command) Name() string {
	return "graph"
}

func (c *Command) Synopsis() string {
	return "Build a structured graph around a Pod"
}

func (c *Command) Usage() string {
	var w bytes.Buffer
	fmt.Fprintf(&w, "%s\n\n", c.Synopsis())
	fmt.Fprint(&w, "USAGE:\n\n")
	fmt.Fprintf(&w, "  %s graph pod <name> [--namespace namespace] [--output human|json|mermaid|dot]\n\n", c.appName)
	fmt.Fprint(&w, "DESCRIPTION:\n\n")
	fmt.Fprintln(&w, "  Builds a deterministic dependency and ownership graph around one Pod.")
	fmt.Fprintln(&w)
	fmt.Fprint(&w, "EXAMPLES:\n\n")
	fmt.Fprintf(&w, "  %s graph pod api-xyz --namespace payments\n", c.appName)
	fmt.Fprintf(&w, "  %s graph pod api-xyz --namespace payments --output mermaid\n\n", c.appName)
	return w.String()
}

func (c *Command) Ctx() context.Context {
	return context.Background()
}

func (c *Command) SetFlags(fs *flag.FlagSet) {
	cmd.CommonNamespacedFlags(fs)
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
	result, err := eng.BuildPodGraph(ctx, engine.BuildPodGraphRequest{
		Namespace: c.namespace,
		Name:      c.resourceName,
	})
	if err != nil {
		return env.Fail(c.Name(), err, log.String("name", c.resourceName))
	}
	if err := c.render(env, result); err != nil {
		return env.Fail(c.Name(), err, log.String("name", c.resourceName))
	}
	return cl.ExitSuccess
}

func (c *Command) configure(fs *flag.FlagSet) error {
	resourceName, err := cmd.ResourceName(fs.Args(), "pod", c.Usage())
	if err != nil {
		return err
	}
	c.namespace = cmd.NamespaceValue(fs)
	c.resourceName = resourceName
	c.format = cmd.OutputValue(fs)
	return cmd.EnsureOutputAllowed(c.Name(), c.format, c.Usage())
}

func (c *Command) render(env *cmd.Env, result *modelgraph.Graph) error {
	switch c.format {
	case "human":
		return render.HumanGraph(env.Out, result)
	case "json":
		return render.JSON(env.Out, result)
	case "mermaid":
		return render.MermaidGraph(env.Out, result)
	case "dot":
		return render.DOTGraph(env.Out, result)
	default:
		return cmd.EnsureOutputAllowed(c.Name(), c.format, c.Usage())
	}
}
