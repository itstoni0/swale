package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "root",
			args: nil,
			want: []string{"Usage:", "init", "scaffold"},
		},
		{
			name: "root-help",
			args: []string{"--help"},
			want: []string{"Usage:", "init", "scaffold"},
		},
		{
			name: "root-short-help",
			args: []string{"-h"},
			want: []string{"Usage:", "init", "scaffold"},
		},
		{
			name: "init",
			args: []string{"init", "--help"},
			want: []string{"Usage:", "swale init"},
		},
		{
			name: "init-short-help",
			args: []string{"init", "-h"},
			want: []string{"Usage:", "swale init"},
		},
		{
			name: "scaffold",
			args: []string{"scaffold", "--help"},
			want: []string{"Usage:", "swale scaffold"},
		},
		{
			name: "scaffold-short-help",
			args: []string{"scaffold", "-h"},
			want: []string{"Usage:", "swale scaffold"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := run(t, tt.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0, stderr = %s", code, stderr)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			for _, phrase := range tt.want {
				if !strings.Contains(stdout, phrase) {
					t.Fatalf("stdout = %q, want substring %q", stdout, phrase)
				}
			}
			if strings.Contains(stdout, "not implemented yet") {
				t.Fatalf("stdout = %q, want no stub message", stdout)
			}
		})
	}
}

func TestStubs(t *testing.T) {
	for _, name := range []string{"init", "scaffold"} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := run(t, name)
			if code == 0 {
				t.Fatalf("exit code = 0, want non-zero, stderr = %s", stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, "Usage:") {
				t.Fatalf("stderr = %q, want usage", stderr)
			}
			if !strings.Contains(stderr, "not implemented yet") {
				t.Fatalf("stderr = %q, want the stub message", stderr)
			}
		})
	}
}

func TestUnknownCommand(t *testing.T) {
	code, stdout, stderr := run(t, "nope")
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero, stderr = %s", stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr = %q, want an unknown-command error", stderr)
	}
}

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Execute(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}
