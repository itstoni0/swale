package managed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ErrNotAllowlisted reports that the command is not a read-only command.
var ErrNotAllowlisted = errors.New("command is not allowlisted")

// Runner runs one command and returns stdout.
// dir is the directory for the command.
type Runner interface {
	// Run runs name with args in dir.
	// Run returns stdout.
	Run(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
}

// ExecRunner runs an allowlisted command.
// ExecRunner gives the command a clean environment.
type ExecRunner struct{}

// Run runs one allowlisted command in dir.
// Run returns the command stdout.
// Run returns an error when the command is not allowlisted.
func (ExecRunner) Run(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	if err := allow(name, args); err != nil {
		return nil, fmt.Errorf("run command: %w", err)
	}
	if ctx == nil {
		return nil, fmt.Errorf("run command: %w", ErrNilContext)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = commandEnv()
	// The command uses its own process group.
	// Cancel stops that process group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		return nil
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			return nil, fmt.Errorf("run command: %w", err)
		}
		return nil, fmt.Errorf("run command: %w: %s", err, detail)
	}
	return stdout.Bytes(), nil
}

// allow reports an error when name and args are outside the read-only list.
// The list is `terragrunt find --json`.
// The list is `terragrunt show -json` and `terraform show -json`.
// The list is `terragrunt state pull` and `terraform state pull`.
func allow(name string, args []string) error {
	base := filepath.Base(name)
	switch base {
	case "terraform", "terragrunt":
	default:
		return fmt.Errorf("%w: %s", ErrNotAllowlisted, name)
	}
	if len(args) == 2 && args[0] == "show" && args[1] == "-json" {
		return nil
	}
	if len(args) == 2 && args[0] == "state" && args[1] == "pull" {
		return nil
	}
	if base == "terragrunt" && len(args) == 2 && args[0] == "find" && args[1] == "--json" {
		return nil
	}
	return fmt.Errorf("%w: %s %s", ErrNotAllowlisted, base, strings.Join(args, " "))
}

// commandEnv returns the environment for a read command.
// commandEnv sets PATH, HOME, and the Terraform automation flags.
// commandEnv sets the AWS credential files to /dev/null.
func commandEnv() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.Getenv("TMPDIR"),
		"TF_IN_AUTOMATION=1",
		"TF_INPUT=0",
		"NO_COLOR=1",
		"TG_NO_COLOR=1",
		"AWS_EC2_METADATA_DISABLED=true",
		"AWS_CONFIG_FILE=/dev/null",
		"AWS_SHARED_CREDENTIALS_FILE=/dev/null",
	}
}
