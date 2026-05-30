package cmd

import (
	"flag"
	"reflect"
	"testing"
)

func TestNormalizeArgsKeepsServeVerboseAsBoolFlag(t *testing.T) {
	got := NormalizeArgs([]string{"serve", "pod", "--verbose"})
	want := []string{"serve", "--verbose", "pod"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeArgs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeArgsSupportsServeFlagValues(t *testing.T) {
	got := NormalizeArgs([]string{"--kubeconfig", "dev.yaml", "serve", "--verbose", "--listen=:9090"})
	want := []string{"--kubeconfig", "dev.yaml", "serve", "--verbose", "--listen=:9090"}

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
