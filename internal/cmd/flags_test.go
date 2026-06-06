package cmd

import (
	"flag"
	"reflect"
	"testing"
	"time"
)

func TestNormalizeArgsKeepsServeVerboseAsBoolFlag(t *testing.T) {
	got := NormalizeArgs([]string{"serve", "pod", "--verbose"})
	want := []string{"serve", "--verbose", "pod"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeArgs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeArgsSupportsServeFlagValues(t *testing.T) {
	got := NormalizeArgs([]string{"--kubeconfig", "dev.yaml", "serve", "--verbose", "--listen=:9090", "--request-timeout", "45s", "--kube-api-budget", "25"})
	want := []string{"--kubeconfig", "dev.yaml", "serve", "--verbose", "--listen=:9090", "--request-timeout", "45s", "--kube-api-budget", "25"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeArgs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeArgsSupportsGraphRenderFlag(t *testing.T) {
	got := NormalizeArgs([]string{"graph", "pod", "api-1", "--namespace", "payments", "--render", "mermaid"})
	want := []string{"graph", "--namespace", "payments", "--render", "mermaid", "pod", "api-1"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeArgs() = %#v, want %#v", got, want)
	}
}

func TestVerboseFlagUsesEnvironmentDefault(t *testing.T) {
	t.Setenv("VERBOSE", "true")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	VerboseFlag(fs)

	if got := BoolValue(fs, "verbose"); !got {
		t.Fatal("BoolValue(verbose) = false, want true")
	}
}

func TestListenFlagDefault(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	ListenFlag(fs)

	if got := StringValue(fs, "listen"); got != ":8080" {
		t.Fatalf("StringValue(listen) = %q, want %q", got, ":8080")
	}
}

func TestRequestTimeoutFlagUsesEnvironmentDefault(t *testing.T) {
	t.Setenv("REQUEST_TIMEOUT", "45s")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	RequestTimeoutFlag(fs)

	got, err := DurationValue(fs, "request-timeout")
	if err != nil {
		t.Fatalf("DurationValue(request-timeout) error = %v", err)
	}
	if got != 45*time.Second {
		t.Fatalf("DurationValue(request-timeout) = %v, want 45s", got)
	}
}

func TestKubeAPIBudgetFlagUsesEnvironmentDefault(t *testing.T) {
	t.Setenv("KUBE_API_BUDGET", "25")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	KubeAPIBudgetFlag(fs)

	got, err := IntValue(fs, "kube-api-budget")
	if err != nil {
		t.Fatalf("IntValue(kube-api-budget) error = %v", err)
	}
	if got != 25 {
		t.Fatalf("IntValue(kube-api-budget) = %d, want 25", got)
	}
}

func TestResourceAndNameRejectsFlagLikeName(t *testing.T) {
	if _, _, err := ResourceAndName([]string{"pod", "--output"}, "usage"); err == nil {
		t.Fatal("ResourceAndName() error = nil, want error")
	}
}
