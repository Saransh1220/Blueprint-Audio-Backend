package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeWorkerPrefix(t *testing.T) {
	require.Equal(t, "worker", normalizeWorkerPrefix("   "))
	require.Equal(t, "render-worker-1", normalizeWorkerPrefix("render worker/1"))
	require.LessOrEqual(t, len(normalizeWorkerPrefix(strings.Repeat("x", 200))), 83)
}
