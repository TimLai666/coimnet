package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestMemoryHelpDiscoversAllCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), nil, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"prepare", "train", "infer", "rollout"} {
		if !strings.Contains(stdout.String(), command) {
			t.Fatalf("help output %q does not mention %s", stdout.String(), command)
		}
	}
}

func TestMemoryUnknownCommandAndPositionalArgumentsFail(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"unknown"}, &stdout, &stderr); err == nil {
		t.Fatal("accepted unknown command")
	}
	if err := run(context.Background(), []string{"prepare", "positional"}, &stdout, &stderr); err == nil {
		t.Fatal("accepted positional arguments")
	}
	if err := run(context.Background(), []string{"help", "positional"}, &stdout, &stderr); err == nil {
		t.Fatal("accepted help positional argument")
	}
}
