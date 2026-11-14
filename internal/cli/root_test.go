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
			args: []string{"--help"},
			want: []string{"Usage:", "init", "scaffold"},
		},
		{
			name: "init",
			args: []string{"init", "--help"},
			want: []string{"Usage:", "init"},
		},
		{
			name: "scaffold",
			args: []string{"scaffold", "--help"},
			want: []string{"Usage:", "scaffold"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, output := run(t, tt.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0, output = %s", code, output)
			}
			for _, phrase := range tt.want {
				if !strings.Contains(output, phrase) {
					t.Fatalf("output = %q, want substring %q", output, phrase)
				}
			}
			if strings.Contains(output, "not implemented yet") {
				t.Fatalf("help output = %q, want no stub message", output)
			}
		})
	}
}

func TestRootWithoutArgs(t *testing.T) {
	code, output := run(t)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0, output = %s", code, output)
	}
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("output = %q, want usage", output)
	}
	if strings.Contains(output, "not implemented yet") {
		t.Fatalf("output = %q, want no stub message", output)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, output := run(t, "nope")
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero, output = %s", output)
	}
	if !strings.Contains(output, "unknown command") {
		t.Fatalf("output = %q, want an unknown-command error", output)
	}
}

func TestStubs(t *testing.T) {
	for _, name := range []string{"init", "scaffold"} {
		t.Run(name, func(t *testing.T) {
			code, output := run(t, name)
			if code == 0 {
				t.Fatalf("exit code = 0, want non-zero, output = %s", output)
			}
			if !strings.Contains(output, "not implemented yet") {
				t.Fatalf("output = %q, want the stub message", output)
			}
			if !strings.Contains(output, "Usage:") {
				t.Fatalf("output = %q, want usage", output)
			}
		})
	}
}

func run(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Execute(args, &stdout, &stderr)
	return code, stdout.String() + stderr.String()
}
