package cmd

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	xenv "github.com/lucasepe/x/env"
)

func CommonNamespacedFlags(fs *flag.FlagSet) {
	ns := xenv.Str("NAMESPACE", "default")
	fs.String("namespace", ns, "namespace")
	OutputFlag(fs)
}

func OutputFlag(fs *flag.FlagSet) {
	output := xenv.Str("OUTPUT", "json")
	fs.String("output", output, "output format")
}

func ListenFlag(fs *flag.FlagSet) {
	addr := xenv.Str("LISTEN_ADDR", ":8080")
	fs.String("listen", addr, "HTTP listen address")
}

func VerboseFlag(fs *flag.FlagSet) {
	fs.Bool("verbose", xenv.True("VERBOSE"), "enable debug logging")
}

func NamespaceValue(fs *flag.FlagSet) string {
	ns := xenv.Str("NAMESPACE", "default")
	return ValueFromVisitedFlag(fs, "namespace", ns)
}

func OutputValue(fs *flag.FlagSet) string {
	output := xenv.Str("OUTPUT", "json")
	return ValueFromVisitedFlag(fs, "output", output)
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
	if len(args) != 2 || args[0] != wantResource || args[1] == "" {
		return "", errors.New(usage)
	}
	return args[1], nil
}

func ResourceAndName(args []string, usage string) (string, string, error) {
	if len(args) != 2 || args[0] == "" || args[1] == "" {
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

func EnsureOutputAllowed(command, output, usage string) error {
	switch command {
	case "explain":
		if output == "human" || output == "json" {
			return nil
		}
	case "graph":
		if output == "human" || output == "json" || output == "mermaid" || output == "dot" {
			return nil
		}
	case "trace":
		if output == "human" || output == "json" {
			return nil
		}
	case "health":
		if output == "human" || output == "json" {
			return nil
		}
	case "dump":
		if output == "json" {
			return nil
		}
	}
	return fmt.Errorf("unsupported output %q\n%s", output, usage)
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
	case "explain", "graph", "trace":
		return arg == "--namespace" || strings.HasPrefix(arg, "--namespace=") ||
			arg == "--output" || strings.HasPrefix(arg, "--output=")
	case "health", "dump":
		return arg == "--output" || strings.HasPrefix(arg, "--output=")
	case "serve":
		return arg == "--listen" || strings.HasPrefix(arg, "--listen=") ||
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
