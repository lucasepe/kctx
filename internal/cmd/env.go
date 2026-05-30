package cmd

import (
	"fmt"
	"io"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/kube"
	"github.com/lucasepe/x/cl"
	"github.com/lucasepe/x/log"
	"k8s.io/client-go/rest"
)

type EngineFactory func() (*engine.Engine, error)

type Env struct {
	Out        io.Writer
	NewEngine  EngineFactory
	Err        error
	Kubeconfig string
	InCluster  bool

	engine *engine.Engine
}

func DefaultEngine() (*engine.Engine, error) {
	return NewEngine("", false)
}

func NewEngine(kubeconfig string, inCluster bool) (*engine.Engine, error) {
	var (
		reader *kube.Client
		err    error
	)
	if inCluster && kubeconfig == "" {
		reader, err = kube.NewInClusterClient()
		if err == rest.ErrNotInCluster {
			reader, err = kube.NewDefaultClient()
		}
	} else if kubeconfig != "" {
		reader, err = kube.NewClientFromKubeconfig(kubeconfig)
	} else {
		reader, err = kube.NewDefaultClient()
	}
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	return engine.NewWithDynamic(reader, reader), nil
}

func EnvFrom(args ...any) (*Env, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("missing command environment")
	}
	env, ok := args[0].(*Env)
	if !ok {
		return nil, fmt.Errorf("invalid command environment")
	}
	return env, nil
}

func (env *Env) Engine() (*engine.Engine, error) {
	if env.engine != nil {
		return env.engine, nil
	}
	newEngine := env.NewEngine
	if newEngine == nil {
		newEngine = func() (*engine.Engine, error) {
			return NewEngine(env.Kubeconfig, env.InCluster)
		}
	}
	eng, err := newEngine()
	if err != nil {
		return nil, err
	}
	env.engine = eng
	return env.engine, nil
}

func (env *Env) Fail(task string, err error, fields ...log.Field) cl.ExitStatus {
	env.Err = err
	log.E("command failed", append([]log.Field{
		log.String("task", task),
		log.Err("err", err),
	}, fields...)...)
	return cl.ExitFailure
}
