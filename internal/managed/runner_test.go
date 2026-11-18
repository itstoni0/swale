package managed_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itstoni0/swale/internal/managed"
)

// ExecRunner implements the command runner.
var _ managed.Runner = managed.ExecRunner{}

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
}

func shellQuote(t *testing.T, path string) string {
	t.Helper()
	if strings.Contains(path, "'") {
		t.Fatalf("path %q contains a quote", path)
	}
	return "'" + path + "'"
}

func installPair(t *testing.T, body string) (bin, work, sentinel string) {
	t.Helper()
	bin = t.TempDir()
	work = t.TempDir()
	sentinel = filepath.Join(work, "ran")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + shellQuote(t, sentinel) + "\nexit 0\n"
	if body != "" {
		script = body
	}
	writeScript(t, filepath.Join(bin, "terraform"), script)
	writeScript(t, filepath.Join(bin, "terragrunt"), script)
	t.Setenv("PATH", bin)
	return bin, work, sentinel
}

func sentinelAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		data, readErr := os.ReadFile(path)
		t.Fatalf("command ran: %s (read %v)", data, readErr)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestExecRunnerRefusesStateWrite(t *testing.T) {
	_, work, sentinel := installPair(t, "")
	refused := []struct {
		name string
		args []string
	}{
		{name: "terraform", args: []string{"apply"}},
		{name: "terraform", args: []string{"apply", "-auto-approve"}},
		{name: "terraform", args: []string{"destroy"}},
		{name: "terraform", args: []string{"import", "aws_vpc.main", "vpc-sample"}},
		{name: "terraform", args: []string{"taint", "aws_vpc.main"}},
		{name: "terraform", args: []string{"untaint", "aws_vpc.main"}},
		{name: "terraform", args: []string{"refresh"}},
		{name: "terraform", args: []string{"force-unlock", "lock"}},
		{name: "terraform", args: []string{"state", "rm", "aws_vpc.main"}},
		{name: "terraform", args: []string{"state", "mv", "a", "b"}},
		{name: "terraform", args: []string{"state", "push", "state.json"}},
		{name: "terraform", args: []string{"plan"}},
		{name: "terraform", args: []string{"show"}},
		{name: "terraform", args: []string{"show", "json"}},
		{name: "terraform"},
		{name: "terragrunt", args: []string{"apply"}},
		{name: "terragrunt", args: []string{"destroy"}},
		{name: "terragrunt", args: []string{"import", "aws_vpc.main", "vpc-sample"}},
		{name: "terragrunt", args: []string{"taint", "aws_vpc.main"}},
		{name: "terragrunt", args: []string{"untaint", "aws_vpc.main"}},
		{name: "terragrunt", args: []string{"refresh"}},
		{name: "terragrunt", args: []string{"force-unlock", "lock"}},
		{name: "terragrunt", args: []string{"state", "rm", "aws_vpc.main"}},
		{name: "terragrunt", args: []string{"state", "mv", "a", "b"}},
		{name: "terragrunt", args: []string{"state", "push", "state.json"}},
		{name: "terragrunt", args: []string{"find"}},
		{name: "terragrunt", args: []string{"find", "--json", "--hidden"}},
		{name: "Terraform", args: []string{"show", "-json"}},
		{name: "bash", args: []string{"-c", "echo"}},
		{name: ""},
	}
	for _, tc := range refused {
		t.Run(commandLine(tc.name, tc.args), func(t *testing.T) {
			if err := os.Remove(sentinel); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			_, err := managed.ExecRunner{}.Run(context.Background(), work, tc.name, tc.args...)
			if !errors.Is(err, managed.ErrNotAllowlisted) {
				t.Fatalf("error %v, want %v", err, managed.ErrNotAllowlisted)
			}
			if !strings.HasPrefix(err.Error(), "run command:") {
				t.Fatalf("error %q does not start with %q", err, "run command:")
			}
			sentinelAbsent(t, sentinel)
		})
	}
}

func TestExecRunnerAllowsReadOnlyCommands(t *testing.T) {
	bin, work, sentinel := installPair(t, "")
	allowed := []struct {
		name string
		args []string
	}{
		{name: filepath.Join(bin, "terraform"), args: []string{"show", "-json"}},
		{name: "terraform", args: []string{"state", "pull"}},
		{name: "terragrunt", args: []string{"show", "-json"}},
		{name: "terragrunt", args: []string{"state", "pull"}},
		{name: "terragrunt", args: []string{"find", "--json"}},
	}
	for _, tc := range allowed {
		t.Run(commandLine(tc.name, tc.args), func(t *testing.T) {
			if err := os.Remove(sentinel); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			out, err := managed.ExecRunner{}.Run(context.Background(), work, tc.name, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != 0 {
				t.Fatalf("stdout = %q, want empty", out)
			}
			if _, err := os.Stat(sentinel); err != nil {
				t.Fatalf("command did not run: %v", err)
			}
		})
	}
}

func TestExecRunnerHidesAWSCredentials(t *testing.T) {
	bin := t.TempDir()
	work := t.TempDir()
	envFile := filepath.Join(work, "env.txt")
	pwdFile := filepath.Join(work, "pwd.txt")
	script := "#!/bin/sh\n" +
		"pwd > " + shellQuote(t, pwdFile) + "\n" +
		"printf '%s\\n' " +
		"\"PATH=$PATH\" " +
		"\"HOME=$HOME\" " +
		"\"TMPDIR=$TMPDIR\" " +
		"\"AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID\" " +
		"\"AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY\" " +
		"\"AWS_SESSION_TOKEN=$AWS_SESSION_TOKEN\" " +
		"\"AWS_PROFILE=$AWS_PROFILE\" " +
		"\"TF_CLI_ARGS=$TF_CLI_ARGS\" " +
		"\"TF_CLI_ARGS_show=$TF_CLI_ARGS_show\" " +
		"\"TF_IN_AUTOMATION=$TF_IN_AUTOMATION\" " +
		"\"TF_INPUT=$TF_INPUT\" " +
		"\"NO_COLOR=$NO_COLOR\" " +
		"\"TG_NO_COLOR=$TG_NO_COLOR\" " +
		"\"AWS_EC2_METADATA_DISABLED=$AWS_EC2_METADATA_DISABLED\" " +
		"\"AWS_CONFIG_FILE=$AWS_CONFIG_FILE\" " +
		"\"AWS_SHARED_CREDENTIALS_FILE=$AWS_SHARED_CREDENTIALS_FILE\" " +
		"> " + shellQuote(t, envFile) + "\n" +
		"printf '%s' '{\"ok\":true}'\n"
	writeScript(t, filepath.Join(bin, "terraform"), script)
	t.Setenv("PATH", bin)
	t.Setenv("AWS_ACCESS_KEY_ID", "should-not-pass")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "should-not-pass")
	t.Setenv("AWS_SESSION_TOKEN", "should-not-pass")
	t.Setenv("AWS_PROFILE", "real-profile")
	t.Setenv("TF_CLI_ARGS", "apply")
	t.Setenv("TF_CLI_ARGS_show", "-refresh")

	out, err := managed.ExecRunner{}.Run(context.Background(), work, "terraform", "show", "-json")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"ok":true}` {
		t.Fatalf("stdout = %q", out)
	}
	pwd, err := os.ReadFile(pwdFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(pwd)) != work {
		t.Fatalf("working directory = %q, want %s", strings.TrimSpace(string(pwd)), work)
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, secret := range []string{"should-not-pass", "real-profile", "TF_CLI_ARGS=apply", "TF_CLI_ARGS_show=-refresh"} {
		if strings.Contains(text, secret) {
			t.Fatalf("environment contains %q\n%s", secret, text)
		}
	}
	for _, line := range []string{
		"PATH=" + bin,
		"HOME=" + os.Getenv("HOME"),
		"TF_IN_AUTOMATION=1",
		"TF_INPUT=0",
		"NO_COLOR=1",
		"TG_NO_COLOR=1",
		"AWS_EC2_METADATA_DISABLED=true",
		"AWS_CONFIG_FILE=/dev/null",
		"AWS_SHARED_CREDENTIALS_FILE=/dev/null",
	} {
		if !strings.Contains(text, line) {
			t.Fatalf("environment lacks %q\n%s", line, text)
		}
	}
}

func TestExecRunnerReportsStderr(t *testing.T) {
	bin := t.TempDir()
	work := t.TempDir()
	script := "#!/bin/sh\necho forced-failure >&2\nexit 3\n"
	writeScript(t, filepath.Join(bin, "terraform"), script)
	t.Setenv("PATH", bin)
	_, err := managed.ExecRunner{}.Run(context.Background(), work, "terraform", "show", "-json")
	if err == nil {
		t.Fatal("got no error")
	}
	if errors.Is(err, managed.ErrNotAllowlisted) {
		t.Fatal(err)
	}
	if !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("error %q does not name the exit status", err)
	}
	if !strings.Contains(err.Error(), "forced-failure") {
		t.Fatalf("error %q does not contain stderr", err)
	}
}

func TestExecRunnerMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := managed.ExecRunner{}.Run(context.Background(), t.TempDir(), "terraform", "show", "-json")
	if err == nil {
		t.Fatal("got no error")
	}
	if errors.Is(err, managed.ErrNotAllowlisted) {
		t.Fatal(err)
	}
	if !strings.Contains(err.Error(), "executable file not found") {
		t.Fatalf("error %q, want a missing executable", err)
	}
	if strings.Contains(err.Error(), "forced-failure") {
		t.Fatalf("error %q contains stderr from another test", err)
	}
}

func TestExecRunnerRejectsNilContext(t *testing.T) {
	_, work, sentinel := installPair(t, "")
	_, err := managed.ExecRunner{}.Run(nil, work, "terraform", "show", "-json")
	if !errors.Is(err, managed.ErrNilContext) {
		t.Fatalf("error %v, want %v", err, managed.ErrNilContext)
	}
	sentinelAbsent(t, sentinel)
}

func TestExecRunnerHonorsContext(t *testing.T) {
	bin := t.TempDir()
	work := t.TempDir()
	writeScript(t, filepath.Join(bin, "terraform"), "#!/bin/sh\n/bin/sleep 30\n")
	t.Setenv("PATH", bin)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := managed.ExecRunner{}.Run(ctx, work, "terraform", "show", "-json")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("got no error")
	}
	if errors.Is(err, managed.ErrNotAllowlisted) {
		t.Fatal(err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("context did not stop the command, elapsed %s, err %v", elapsed, err)
	}
}
