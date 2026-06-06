package explain

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

func Task(appName string) cl.Task {
	return &explainCmd{
		ctx:       context.Background(),
		appName:   appName,
		resource:  "pod",
		namespace: "default",
	}
}

var _ cl.Task = (*explainCmd)(nil)

type explainCmd struct {
	ctx          context.Context
	appName      string
	namespace    string
	resource     string
	resourceName string
}

func New(appName string) *explainCmd {
	return &explainCmd{appName: appName}
}

func (c *explainCmd) Name() string {
	return "explain"
}

func (c *explainCmd) Synopsis() string {
	return "Resolve structured context around a Kubernetes Pod"
}

func (c *explainCmd) Usage() string {
	var w bytes.Buffer
	fmt.Fprintf(&w, "%s\n\n", c.Synopsis())

	fmt.Fprint(&w, "USAGE:\n\n")
	fmt.Fprintf(&w, "  %s %s pod <name> [--namespace namespace]\n\n", c.appName, c.Name())

	fmt.Fprint(&w, "DESCRIPTION:\n\n")
	fmt.Fprintln(&w, "  Resolves a Pod through Kubernetes discovery first, then builds rich")
	fmt.Fprintln(&w, "  operational context: status, owner chain, node placement, selected")
	fmt.Fprintln(&w, "  Services, mounted configuration, storage dependencies, recent Events,")
	fmt.Fprintln(&w, "  and factual operational signals.")
	fmt.Fprintln(&w)
	fmt.Fprintln(&w, "  Resource aliases such as pod, pods, and po are accepted. Generic dynamic")
	fmt.Fprintln(&w, "  resource resolution exists as an internal extension point, but deep")
	fmt.Fprintln(&w, "  operational semantics are only exposed where kctx has explicit knowledge.")
	fmt.Fprintln(&w)

	fmt.Fprint(&w, "EXAMPLES:\n\n")
	fmt.Fprintf(&w, "  %s %s pod api-xyz --namespace payments\n", c.appName, c.Name())
	fmt.Fprintf(&w, "  %s %s po api-xyz --namespace payments\n\n", c.appName, c.Name())

	return w.String()
}

func (c *explainCmd) Ctx() context.Context {
	return context.Background()
}

func (c *explainCmd) SetFlags(fs *flag.FlagSet) {
	cmd.CommonNamespacedFlags(fs)
}

func (c *explainCmd) Execute(ctx context.Context, fs *flag.FlagSet, args ...any) cl.ExitStatus {
	env, err := cmd.EnvFrom(args...)
	if err != nil {
		return cl.ExitFailure
	}
	if err := c.configure(fs); err != nil {
		return env.FailJSON(c.Name(), err)
	}

	eng, err := env.Engine()
	if err != nil {
		return env.FailJSON(c.Name(), err)
	}

	result, err := eng.ResolveContext(ctx, engine.ResolveContextRequest{
		Resource:  c.resource,
		Namespace: c.namespace,
		Name:      c.resourceName,
	})
	if err != nil {
		return env.FailJSON(c.Name(), err,
			log.String("resource", c.resource),
			log.String("name", c.resourceName),
		)
	}

	if err := c.render(env, result); err != nil {
		return env.FailJSON(c.Name(), err,
			log.String("resource", c.resource),
			log.String("name", c.resourceName),
		)
	}
	return cl.ExitSuccess
}

func (c *explainCmd) configure(fs *flag.FlagSet) error {
	resource, resourceName, err := cmd.ResourceAndName(fs.Args(), c.Usage())
	if err != nil {
		return err
	}

	c.namespace = cmd.NamespaceValue(fs)
	c.resource = resource
	c.resourceName = resourceName
	return nil
}

func (c *explainCmd) render(env *cmd.Env, result *engine.ResolveContextResponse) error {
	if result.Pod != nil {
		return render.JSON(env.Out, result.Pod)
	}
	return render.JSON(env.Out, result)
}
