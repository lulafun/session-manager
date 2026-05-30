package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type ForkOptions struct {
	CodexBin       string
	TargetProvider string
	Model          string
	WorkDir        string
	Last           bool
	DryRun         bool
	ExtraArgs      []string
}

func BuildForkCommand(sessionID string, opts ForkOptions) ([]string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" && !opts.Last {
		return nil, fmt.Errorf("session id is required")
	}
	if sessionID != "" && opts.Last {
		return nil, fmt.Errorf("pass either a session id or --last, not both")
	}
	codexBin := opts.CodexBin
	if codexBin == "" {
		codexBin = "codex"
	}
	args := []string{codexBin, "fork"}
	if opts.TargetProvider != "" {
		args = append(args, "-c", "model_provider="+opts.TargetProvider)
	}
	if opts.Model != "" {
		args = append(args, "-m", opts.Model)
	}
	if opts.WorkDir != "" {
		args = append(args, "-C", opts.WorkDir)
	}
	if opts.Last {
		args = append(args, "--last")
	}
	args = append(args, opts.ExtraArgs...)
	if sessionID != "" {
		args = append(args, sessionID)
	}
	return args, nil
}

func RunForkCommand(sessionID string, opts ForkOptions) error {
	args, err := BuildForkCommand(sessionID, opts)
	if err != nil {
		return err
	}
	if opts.DryRun {
		fmt.Println(shellQuote(args))
		return nil
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func shellQuote(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "" {
			quoted = append(quoted, "''")
			continue
		}
		if strings.IndexFunc(arg, func(r rune) bool {
			return !(r == '_' || r == '-' || r == '.' || r == '/' || r == '=' ||
				r == ':' || r == '+' || r == ',' ||
				(r >= '0' && r <= '9') ||
				(r >= 'a' && r <= 'z') ||
				(r >= 'A' && r <= 'Z'))
		}) == -1 {
			quoted = append(quoted, arg)
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
	}
	return strings.Join(quoted, " ")
}
