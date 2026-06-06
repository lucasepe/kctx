// Command normalize canonicalizes kctx JSON outputs for golden comparisons.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	timestampPlaceholder = "<timestamp>"
	uidPlaceholder       = "<uid>"
	clusterIPPlaceholder = "<cluster-ip>"
	imagePullMessage     = "<image-pull-message>"
)

var e2eImagePattern = regexp.MustCompile(`ghcr\.io/lucasepe/kctx-e2e-image-does-not-exist:never`)
var endpointSliceNamePattern = regexp.MustCompile(`kctx-orphan-service-[a-z0-9]+`)

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "normalize: %v\n", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer) error {
	decoder := json.NewDecoder(in)
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}

	value = normalizeValue("", value)

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	return nil
}

func normalizeValue(key string, value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			out[childKey] = normalizeValue(childKey, childValue)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeValue(key, item)
		}
		sort.SliceStable(out, func(i, j int) bool {
			return stableJSON(out[i]) < stableJSON(out[j])
		})
		return out
	case string:
		return normalizeString(key, typed)
	default:
		return typed
	}
}

func stableJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func normalizeString(key, value string) string {
	switch key {
	case "generatedAt", "timestamp", "firstTimestamp", "lastTimestamp":
		return timestampPlaceholder
	case "uid":
		return uidPlaceholder
	case "clusterIP":
		return clusterIPPlaceholder
	case "message":
		return normalizeMessage(value)
	default:
		return normalizeGeneratedNames(value)
	}
}

func normalizeGeneratedNames(value string) string {
	return endpointSliceNamePattern.ReplaceAllString(value, "kctx-orphan-service-<suffix>")
}

func normalizeMessage(value string) string {
	if e2eImagePattern.MatchString(value) {
		return imagePullMessage
	}
	if strings.Contains(value, "Back-off pulling image") || strings.Contains(value, "Failed to pull image") {
		return imagePullMessage
	}
	if strings.Contains(value, "ErrImagePull") || strings.Contains(value, "ImagePullBackOff") {
		return imagePullMessage
	}

	var buf bytes.Buffer
	buf.WriteString(value)
	return buf.String()
}
