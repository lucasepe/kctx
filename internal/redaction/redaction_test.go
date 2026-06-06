package redaction

import "testing"

func TestTextRedactsSimpleSecretPatterns(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "key value",
			in:   "failed with token=abc123 and retry",
			want: "failed with token=[REDACTED] and retry",
		},
		{
			name: "colon value",
			in:   "password: hunter2",
			want: "password: [REDACTED]",
		},
		{
			name: "bearer token",
			in:   "server rejected Bearer eyJhbGciOiJIUzI1NiJ9",
			want: "server rejected Bearer [REDACTED]",
		},
		{
			name: "url user info",
			in:   "syncing https://token123@github.com/example/private.git",
			want: "syncing https://[REDACTED]@github.com/example/private.git",
		},
		{
			name: "normal event message",
			in:   "Back-off restarting failed container api",
			want: "Back-off restarting failed container api",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Text(tc.in); got != tc.want {
				t.Fatalf("Text(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStringMapCopiesAndMasksSensitiveKeys(t *testing.T) {
	in := map[string]string{
		"app":                  "payments",
		"example.com/token":    "abc123",
		"config":               "password=secret",
		"private-key-checksum": "sha256:abcd",
	}

	got := StringMap(in)
	if got["app"] != "payments" {
		t.Fatalf("app label = %q, want payments", got["app"])
	}
	if got["example.com/token"] != Mask {
		t.Fatalf("token label = %q, want %q", got["example.com/token"], Mask)
	}
	if got["config"] != "password=[REDACTED]" {
		t.Fatalf("config label = %q, want redacted text", got["config"])
	}
	if got["private-key-checksum"] != Mask {
		t.Fatalf("private key label = %q, want %q", got["private-key-checksum"], Mask)
	}

	in["app"] = "changed"
	if got["app"] != "payments" {
		t.Fatalf("StringMap returned a map sharing input storage")
	}
}
