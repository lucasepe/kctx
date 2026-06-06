// Package redaction contains the small, explicit data-safety rules used before
// kctx returns Kubernetes-derived text or metadata to callers.
package redaction

import (
	"regexp"
	"strings"
)

// Mask is the stable placeholder used when kctx hides a sensitive value.
const Mask = "[REDACTED]"

var (
	keyValuePattern = regexp.MustCompile(`(?i)\b([a-z0-9_.\/-]*(?:password|passwd|token|secret|credential|credentials|api[-_]?key|private[-_]?key|authorization|auth|cookie|session)[a-z0-9_.\/-]*)\s*=\s*("[^"]*"|'[^']*'|[^\s,;]+)`)
	keyColonPattern = regexp.MustCompile(`(?i)\b([a-z0-9_.\/-]*(?:password|passwd|token|secret|credential|credentials|api[-_]?key|private[-_]?key|authorization|auth|cookie|session)[a-z0-9_.\/-]*)\s*:\s*("[^"]*"|'[^']*'|[^\s,;]+)`)
	bearerPattern   = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+\/=-]+`)
	urlUserPattern  = regexp.MustCompile(`(?i)\b(https?://)[^/@\s]+@`)
)

// Text redacts simple secret-looking values from human-oriented messages.
//
// The function intentionally recognizes only common, easy-to-review patterns.
// It is not a parser for shell, JSON, YAML, or Kubernetes Events.
func Text(value string) string {
	if value == "" {
		return ""
	}

	value = bearerPattern.ReplaceAllString(value, "Bearer "+Mask)
	value = urlUserPattern.ReplaceAllString(value, "${1}"+Mask+"@")
	value = keyValuePattern.ReplaceAllString(value, "$1="+Mask)
	value = keyColonPattern.ReplaceAllString(value, "$1: "+Mask)
	return value
}

// StringMap returns a defensive copy of metadata with sensitive values masked.
//
// Kubernetes labels and annotations are often operational metadata, but they can
// still carry tokens, internal credentials, or auth-related hints. kctx keeps
// the key for context and replaces only values whose key is sensitive.
func StringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	out := make(map[string]string, len(values))
	for key, value := range values {
		if SensitiveKey(key) {
			out[key] = Mask
			continue
		}
		out[key] = Text(value)
	}
	return out
}

// SensitiveKey reports whether a metadata key commonly names a secret-bearing
// value. It uses plain substring checks so the policy remains obvious.
func SensitiveKey(key string) bool {
	key = strings.ToLower(key)
	for _, word := range sensitiveKeyWords {
		if strings.Contains(key, word) {
			return true
		}
	}
	return false
}

var sensitiveKeyWords = []string{
	"password",
	"passwd",
	"token",
	"secret",
	"credential",
	"credentials",
	"api-key",
	"api_key",
	"apikey",
	"private-key",
	"private_key",
	"authorization",
	"auth",
	"cookie",
	"session",
}
