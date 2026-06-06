// Command validate checks kctx JSON outputs against the documented contract.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const schemaVersion = "kctx.io/v1alpha1"

var signalSeverities = map[string]bool{
	"info":    true,
	"warning": true,
	"error":   true,
}

var errorCodes = map[string]bool{
	"bad_request":           true,
	"not_found":             true,
	"forbidden":             true,
	"method_not_allowed":    true,
	"unsupported_resource":  true,
	"timeout":               true,
	"limit_exceeded":        true,
	"client_closed_request": true,
	"internal_error":        true,
}

func main() {
	schemaDir := flag.String("schemas", "schemas/kctx.io/v1alpha1", "directory containing JSON schema documents")
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: validate-json-contract [--schemas dir] <json files...>")
		os.Exit(2)
	}

	var failed bool
	if err := validateSchemaFiles(*schemaDir); err != nil {
		fmt.Fprintf(os.Stderr, "schema validation failed: %v\n", err)
		failed = true
	}

	for _, path := range flag.Args() {
		if err := validateFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

func validateSchemaFiles(dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "*.schema.json"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no schema files found in %s", dir)
	}
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
	}
	return nil
}

func validateFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	return validateDocument(doc)
}

func validateDocument(doc map[string]any) error {
	if got := stringField(doc, "schemaVersion"); got != schemaVersion {
		return fmt.Errorf("schemaVersion = %q, want %q", got, schemaVersion)
	}
	kind := stringField(doc, "kind")
	if kind == "" {
		return fmt.Errorf("kind is required")
	}

	switch kind {
	case "Error":
		return validateError(doc)
	case "PodContext":
		return validateArrays(doc, "owners", "services", "volumes", "events", "signals", "relations")
	case "NamespaceHealth":
		return validateArrays(doc, "workloads", "pods", "services", "pvcs", "events", "signals")
	case "NamespaceDump":
		return validateArrays(doc, "entities", "relations", "signals", "events")
	case "ServiceTrace":
		return validateArrays(doc, "ports", "endpoints", "pods", "owners", "relations", "signals")
	case "PodGraph", "ResourceGraph":
		return validateArrays(doc, "nodes", "edges")
	case "ResourceContext":
		return nil
	case "DynamicResourceContext":
		return validateArrays(doc, "owners", "related", "relations", "signals")
	default:
		return fmt.Errorf("unsupported kind %q", kind)
	}
}

func validateError(doc map[string]any) error {
	errValue, ok := doc["error"].(map[string]any)
	if !ok {
		return fmt.Errorf("error object is required")
	}
	code := stringField(errValue, "code")
	if !errorCodes[code] {
		return fmt.Errorf("error.code = %q is not documented", code)
	}
	if _, ok := errValue["message"].(string); !ok {
		return fmt.Errorf("error.message must be a string")
	}
	if details, ok := errValue["details"]; ok {
		detailMap, ok := details.(map[string]any)
		if !ok {
			return fmt.Errorf("error.details must be an object")
		}
		for key, value := range detailMap {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("error.details[%q] must be a string", key)
			}
		}
	}
	return nil
}

func validateArrays(doc map[string]any, fields ...string) error {
	for _, field := range fields {
		items, ok := doc[field].([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", field)
		}
		if field == "signals" {
			if err := validateSignals(items); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSignals(items []any) error {
	for i, item := range items {
		signal, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("signals[%d] must be an object", i)
		}
		severity := stringField(signal, "severity")
		if !signalSeverities[severity] {
			return fmt.Errorf("signals[%d].severity = %q, want info, warning, or error", i, severity)
		}
		if stringField(signal, "reason") == "" {
			return fmt.Errorf("signals[%d].reason is required", i)
		}
		if _, ok := signal["message"].(string); !ok {
			return fmt.Errorf("signals[%d].message must be a string", i)
		}
	}
	return nil
}

func stringField(doc map[string]any, field string) string {
	value, _ := doc[field].(string)
	return value
}
