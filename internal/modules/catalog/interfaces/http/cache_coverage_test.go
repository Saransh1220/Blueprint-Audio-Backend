package http

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSpecHandlerCacheHelpersWithoutRedis(t *testing.T) {
	handler := NewSpecHandler(nil, nil, nil, nil, nil)
	ctx := context.Background()
	value, ok := handler.cacheGet(ctx, "missing")
	require.False(t, ok)
	require.Empty(t, value)
	handler.cacheSet(ctx, "key", []byte("value"), time.Minute)
	handler.cacheDel(ctx, "key")
	handler.cacheDelSpec(ctx, uuid.New())
}
