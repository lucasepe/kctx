package render

import (
	"fmt"
	"io"
	"sort"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/model"
)

func HumanPodContext(w io.Writer, ctx *engine.ResolvePodContextResponse) error {
	if _, err := fmt.Fprintf(w, "Pod %s/%s\n", ctx.Pod.Namespace, ctx.Pod.Name); err != nil {
		return err
	}

	writeSection(w, "Status")
	fmt.Fprintf(w, "  Phase: %s\n", ctx.Status.Phase)
	fmt.Fprintf(w, "  Ready: %t\n", ctx.Status.Ready)
	fmt.Fprintf(w, "  Restarts: %d\n", ctx.Status.Restarts)
	for _, container := range ctx.Status.Containers {
		detail := ""
		if container.Reason != "" {
			detail = ": " + container.Reason
		}
		last := ""
		if container.LastStateReason != "" {
			last = fmt.Sprintf(" lastState=%s/%s", container.LastState, container.LastStateReason)
		}
		fmt.Fprintf(w, "  Container %s: ready=%t restarts=%d state=%s%s%s\n", container.Name, container.Ready, container.RestartCount, container.State, detail, last)
	}

	if len(ctx.Signals) > 0 {
		writeSection(w, "Signals")
		for _, signal := range ctx.Signals {
			fmt.Fprintf(w, "  %s %s: %s\n", signal.Severity, signal.Reason, signal.Message)
		}
	}

	if len(ctx.Owners) > 0 {
		writeSection(w, "Owners")
		for _, owner := range ctx.Owners {
			fmt.Fprintf(w, "  %s %s/%s\n", owner.Kind, owner.Namespace, owner.Name)
		}
	}

	if ctx.Node != nil {
		writeSection(w, "Scheduled on")
		fmt.Fprintf(w, "  Node %s\n", ctx.Node.Name)
	}

	if len(ctx.Services) > 0 {
		writeSection(w, "Selected by Services")
		for _, service := range ctx.Services {
			fmt.Fprintf(w, "  Service %s\n", service.Name)
		}
	}

	if len(ctx.Volumes) > 0 {
		writeSection(w, "Dependencies")
		for _, volume := range sortedVolumes(ctx.Volumes) {
			fmt.Fprintf(w, "  %s %s\n", volume.Type, volume.Name)
		}
	}

	if len(ctx.Events) > 0 {
		writeSection(w, "Recent Events")
		for _, event := range ctx.Events {
			fmt.Fprintf(w, "  %s %s: %s\n", event.Type, event.Reason, event.Message)
		}
	}

	return nil
}

func writeSection(w io.Writer, name string) {
	fmt.Fprintf(w, "\n%s\n", name)
}

func sortedVolumes(volumes []model.VolumeRef) []model.VolumeRef {
	out := append([]model.VolumeRef(nil), volumes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == out[j].Type {
			return out[i].Name < out[j].Name
		}
		return out[i].Type < out[j].Type
	})
	return out
}
