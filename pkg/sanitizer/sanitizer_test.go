package sanitizer_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/AlxPolt/sw-engineer-challenge/pkg/sanitizer"
)

func TestSanitize_PasswordRedacted(t *testing.T) {
	cases := []string{
		`password=supersecret`,
		`password: supersecret`,
		`PASSWORD=supersecret`,
		`pwd=supersecret`,
	}
	for _, input := range cases {
		out := sanitizer.Sanitize(input)
		if strings.Contains(out, "supersecret") {
			t.Errorf("password leaked in output: input=%q output=%q", input, out)
		}
	}
}

func TestSanitize_TokenRedacted(t *testing.T) {
	cases := []string{
		`token=eyJhbGciOiJIUzI1NiJ9.payload.sig`,
		`Token: Bearer abc123xyz`,
		`authorization: Bearer mysecrettoken`,
	}
	for _, input := range cases {
		out := sanitizer.Sanitize(input)
		if strings.Contains(out, "abc123xyz") || strings.Contains(out, "mysecrettoken") {
			t.Errorf("token leaked in output: input=%q output=%q", input, out)
		}
	}
}

func TestSanitize_URLCredentialsRedacted(t *testing.T) {
	cases := []struct {
		input   string
		leakStr string
	}{
		{"nats://alxuser:alxpass@nats:4222", "alxpass"},
		{"https://admin:hunter2@influxdb:8181", "hunter2"},
		{"postgres://user:dbpassword@localhost/db", "dbpassword"},
	}

	for _, tc := range cases {
		out := sanitizer.Sanitize(tc.input)
		if strings.Contains(out, tc.leakStr) {
			t.Errorf("URL credential leaked: input=%q output=%q", tc.input, out)
		}
	}

}

func TestSanitize_InnocentStringUnchanged(t *testing.T) {
	input := "event received: criticality=7 subject=events_subject"
	out := sanitizer.Sanitize(input)
	if out != input {
		t.Errorf("innocent string modified: got %q, want %q", out, input)
	}
}

func TestSanitize_EmptyString(t *testing.T) {
	if got := sanitizer.Sanitize(""); got != "" {
		t.Errorf("empty string: got %q, want \"\"", got)
	}
}

func TestSanitize_LongStringTruncated(t *testing.T) {
	long := strings.Repeat("a", 2000)
	out := sanitizer.Sanitize(long)
	if len([]rune(out)) > 1015 { // 1000 + len("...[truncated]")
		t.Errorf("string not truncated: output length %d", len(out))
	}
	if !strings.HasSuffix(out, "[truncated]") {
		t.Errorf("truncated string missing suffix: %q", out)
	}
}

func TestSanitizeError_NilReturnsEmpty(t *testing.T) {
	if got := sanitizer.SanitizeError(nil); got != "" {
		t.Errorf("nil error: got %q, want \"\"", got)
	}
}

func TestSanitizeError_RedactsCredentials(t *testing.T) {
	err := errors.New("failed to connect: nats://alxuser:alxpass@nats:4222 timed out")
	out := sanitizer.SanitizeError(err)
	if strings.Contains(out, "alxpass") {
		t.Errorf("password leaked in error: %q", out)
	}
}
