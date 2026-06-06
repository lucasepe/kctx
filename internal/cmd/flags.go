package cmd

import (
	"errors"
	"flag"
	"strconv"
	"strings"
	"time"

	xenv "github.com/lucasepe/x/env"
)

func CommonNamespacedFlags(fs *flag.FlagSet) {
	ns := xenv.Str("NAMESPACE", "default")
	fs.String("namespace", ns, "namespace")
}

func RenderFlag(fs *flag.FlagSet) {
	fs.String("render", "", "graph renderer: mermaid or dot")
}

func RenderValue(fs *flag.FlagSet) string {
	return ValueFromVisitedFlag(fs, "render", "")
}

func ListenFlag(fs *flag.FlagSet) {
	addr := xenv.Str("LISTEN_ADDR", ":8080")
	fs.String("listen", addr, "HTTP listen address")
}

func VerboseFlag(fs *flag.FlagSet) {
	fs.Bool("verbose", xenv.True("VERBOSE"), "enable debug logging")
}

func RequestTimeoutFlag(fs *flag.FlagSet) {
	timeout := xenv.Str("REQUEST_TIMEOUT", "30s")
	fs.String("request-timeout", timeout, "per-request timeout; 0 disables it")
}

func KubeAPIBudgetFlag(fs *flag.FlagSet) {
	budget := xenv.Int("KUBE_API_BUDGET", 100)
	fs.Int("kube-api-budget", budget, "Kubernetes API call budget per request; 0 disables it")
}

func NamespaceValue(fs *flag.FlagSet) string {
	ns := xenv.Str("NAMESPACE", "default")
	return ValueFromVisitedFlag(fs, "namespace", ns)
}

func StringValue(fs *flag.FlagSet, name string) string {
	return ValueFromVisitedFlag(fs, name, "")
}

func BoolValue(fs *flag.FlagSet, name string) bool {
	value := "false"
	visited := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			visited = true
			value = f.Value.String()
		}
	})
	if f := fs.Lookup(name); f != nil && !visited {
		value = f.DefValue
	}
	out, err := strconv.ParseBool(value)
	return err == nil && out
}

func DurationValue(fs *flag.FlagSet, name string) (time.Duration, error) {
	value := StringValue(fs, name)
	out, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	return out, nil
}

func IntValue(fs *flag.FlagSet, name string) (int, error) {
	value := StringValue(fs, name)
	return strconv.Atoi(value)
}

func CommandName(fs *flag.FlagSet) string {
	args := fs.Args()
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func ValueFromVisitedFlag(fs *flag.FlagSet, name, def string) string {
	value := def
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			value = f.Value.String()
		}
	})
	if value == "" {
		if f := fs.Lookup(name); f != nil {
			value = f.DefValue
		}
	}
	return value
}

func ResourceName(args []string, wantResource, usage string) (string, error) {
	if len(args) != 2 || args[0] != wantResource || args[1] == "" || strings.HasPrefix(args[1], "--") {
		return "", errors.New(usage)
	}
	return args[1], nil
}

func ResourceAndName(args []string, usage string) (string, string, error) {
	if len(args) != 2 || args[0] == "" || args[1] == "" || strings.HasPrefix(args[0], "--") || strings.HasPrefix(args[1], "--") {
		return "", "", errors.New(usage)
	}
	return args[0], args[1], nil
}

func NoArgs(args []string, usage string) error {
	if len(args) != 0 {
		return errors.New(usage)
	}
	return nil
}

func NormalizeArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}

	var globalFlags []string
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if IsGlobalFlag(arg) {
			globalFlags = append(globalFlags, arg)
			if arg == "--kubeconfig" && i+1 < len(args) {
				i++
				globalFlags = append(globalFlags, args[i])
			}
			continue
		}
		rest = append(rest, arg)
	}
	if len(rest) == 0 {
		return globalFlags
	}

	command := rest[0]
	var flags []string
	var positionals []string
	for i := 1; i < len(rest); i++ {
		arg := rest[i]
		if IsKnownFlag(command, arg) {
			flags = append(flags, arg)
			if !IsBoolFlag(command, arg) && !strings.Contains(arg, "=") {
				if i+1 < len(rest) {
					i++
					flags = append(flags, rest[i])
				}
			}
			continue
		}
		positionals = append(positionals, arg)
	}

	out := make([]string, 0, len(args))
	out = append(out, globalFlags...)
	out = append(out, command)
	out = append(out, flags...)
	out = append(out, positionals...)
	return out
}

func IsGlobalFlag(arg string) bool {
	return arg == "--kubeconfig" ||
		len(arg) > len("--kubeconfig=") && arg[:len("--kubeconfig=")] == "--kubeconfig="
}

func IsKnownFlag(command, arg string) bool {
	switch command {
	case "explain", "trace":
		return arg == "--namespace" || strings.HasPrefix(arg, "--namespace=")
	case "graph":
		return arg == "--namespace" || strings.HasPrefix(arg, "--namespace=") ||
			arg == "--render" || strings.HasPrefix(arg, "--render=")
	case "health", "dump":
		return false
	case "serve":
		return arg == "--listen" || strings.HasPrefix(arg, "--listen=") ||
			arg == "--request-timeout" || strings.HasPrefix(arg, "--request-timeout=") ||
			arg == "--kube-api-budget" || strings.HasPrefix(arg, "--kube-api-budget=") ||
			arg == "--verbose" || strings.HasPrefix(arg, "--verbose=")
	default:
		return false
	}
}

func IsBoolFlag(command, arg string) bool {
	switch command {
	case "serve":
		return arg == "--verbose" || strings.HasPrefix(arg, "--verbose=")
	default:
		return false
	}
}
