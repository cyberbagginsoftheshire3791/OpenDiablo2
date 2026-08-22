package main

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// cmdResult is the outcome of one external command.
type cmdResult struct {
	ok       bool
	output   string // combined stdout+stderr, trimmed
	duration time.Duration
	err      error
}

// runCmd runs name args... in dir with a timeout and returns the combined
// output. It never panics on a missing binary: that is reported as a failure
// with the error text as output.
func runCmd(dir string, timeout time.Duration, name string, args ...string) cmdResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	start := time.Now()
	err := cmd.Run()
	res := cmdResult{
		output:   strings.TrimSpace(buf.String()),
		duration: time.Since(start),
		err:      err,
	}

	if ctx.Err() == context.DeadlineExceeded {
		res.output = strings.TrimSpace(res.output + "\n(timed out after " + timeout.String() + ")")

		return res
	}

	if err != nil {
		if res.output == "" {
			res.output = err.Error()
		}

		return res
	}

	res.ok = true

	return res
}

// truncate keeps the first max bytes of s and marks the cut.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}

	return s[:max] + "\n… (truncated)"
}
