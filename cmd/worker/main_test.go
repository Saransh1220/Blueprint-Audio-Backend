package main

import (
	"strings"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/application"
	"github.com/stretchr/testify/require"
)

func TestNormalizeWorkerPrefix(t *testing.T) {
	require.Equal(t, "worker", application.NormalizeWorkerPrefix("   "))
	require.Equal(t, "render-worker-1", application.NormalizeWorkerPrefix("render worker/1"))
	require.LessOrEqual(t, len(application.NormalizeWorkerPrefix(strings.Repeat("x", 200))), 83)
}
