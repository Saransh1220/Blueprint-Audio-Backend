package main

import (
	"context"
	"io"
	"testing"
)

func TestRunRejectsUnsafeFlagsBeforeConnecting(t *testing.T) {
	for _, args := range [][]string{
		{"--apply"},
		{"--apply", "--dry-run=false"},
		{"--rollback-batch=00000000-0000-0000-0000-000000000001"},
		{"--dry-run=false"},
		{"--apply", "--dry-run=false", "--manifest=reviewed.json", "--rollback-batch=00000000-0000-0000-0000-000000000001"},
	} {
		t.Run(args[0], func(t *testing.T) {
			if err := run(context.Background(), args, io.Discard); err == nil {
				t.Fatal("unsafe flags must fail before database access")
			}
		})
	}
}
