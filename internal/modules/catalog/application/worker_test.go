package application

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeWorkerPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"keeps allowed characters", "Worker_1.a-b", "Worker_1.a-b"},
		{"replaces unsafe characters", "my host/name!", "my-host-name"},
		{"trims surrounding dashes", "  node  ", "node"},
		{"empty falls back", "", "worker"},
		{"only unsafe falls back", "!!!", "worker"},
		{"non-ascii replaced per rune", "héllo", "h-llo"},
		{"truncated to 83 chars", strings.Repeat("a", 200), strings.Repeat("a", 83)},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, NormalizeWorkerPrefix(tt.input))
		})
	}
}

func TestStartUploadWorker_IdlePollingRunsMaintenanceOnceAndStopsOnCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var claims, expires, requeues int32
	var workerID atomic.Value
	var gotStaleBefore atomic.Value
	repo := &queueRepoStub{
		uploadRepositoryStub: &uploadRepositoryStub{
			claimNextJobFn: func(_ context.Context, id string) (*domain.ProcessingBundle, error) {
				workerID.Store(id)
				if atomic.AddInt32(&claims, 1) >= 3 {
					cancel()
				}
				return nil, domain.ErrNoProcessingJob
			},
		},
		expireFn: func(context.Context, time.Time) (int64, error) {
			atomic.AddInt32(&expires, 1)
			return 2, nil
		},
		requeueFn: func(_ context.Context, before time.Time) (int64, error) {
			atomic.AddInt32(&requeues, 1)
			gotStaleBefore.Store(before)
			return 1, nil
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		StartUploadWorker(ctx,
			NewSpecUploadProcessor(repo, &objectStoreStub{}, nil),
			config.WorkerConfig{ID: " my host! ", PollInterval: 2 * time.Millisecond, LeaseDuration: time.Hour},
		)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}

	assert.GreaterOrEqual(t, atomic.LoadInt32(&claims), int32(3))
	assert.Equal(t, int32(1), atomic.LoadInt32(&expires), "maintenance is throttled by the requeue interval")
	assert.Equal(t, int32(1), atomic.LoadInt32(&requeues))
	id, _ := workerID.Load().(string)
	assert.True(t, strings.HasPrefix(id, "my-host-"), "worker id %q", id)
	_, err := uuid.Parse(strings.TrimPrefix(id, "my-host-"))
	assert.NoError(t, err, "worker id ends with a UUID")
	before, _ := gotStaleBefore.Load().(time.Time)
	assert.WithinDuration(t, time.Now().Add(-time.Hour), before, 10*time.Second)
}

func TestStartUploadWorker_ContinuesAfterErrorsAndProcessesBackToBack(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fx := newProcFixture(t)
	fx.dropAsset(domain.UploadAssetImage) // job fails fast, but counts as processed
	var claims, fails int32
	repo := &queueRepoStub{
		uploadRepositoryStub: &uploadRepositoryStub{
			claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) {
				switch atomic.AddInt32(&claims, 1) {
				case 1:
					return fx.bundle, nil // processed => loop continues without waiting
				case 2:
					return nil, errors.New("claim exploded") // logged, then idle wait
				default:
					cancel()
					return nil, domain.ErrNoProcessingJob
				}
			},
			failJobFn: func(context.Context, uuid.UUID, string, string) error {
				atomic.AddInt32(&fails, 1)
				return nil
			},
		},
		expireFn:  func(context.Context, time.Time) (int64, error) { return 0, errors.New("expire failed") },
		requeueFn: func(context.Context, time.Time) (int64, error) { return 0, errors.New("requeue failed") },
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Zero values exercise the default poll/lease fallbacks; the 2s default poll
		// is never waited out because the claim callback cancels the context.
		StartUploadWorker(ctx,
			NewSpecUploadProcessor(repo, fx.store, &notifierStub{}),
			config.WorkerConfig{PollInterval: time.Millisecond},
		)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}

	assert.Equal(t, int32(3), atomic.LoadInt32(&claims))
	assert.Equal(t, int32(1), atomic.LoadInt32(&fails))
}

func TestStartUploadWorker_CancelledContextNeverClaims(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &uploadRepositoryStub{
		claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) {
			t.Error("claim must not run for a canceled context")
			return nil, domain.ErrNoProcessingJob
		},
	}
	StartUploadWorker(ctx, NewSpecUploadProcessor(repo, &objectStoreStub{}, nil), config.WorkerConfig{})
}

func TestStartUploadWorker_DefaultPollWaitIsInterruptedByCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &queueRepoStub{uploadRepositoryStub: &uploadRepositoryStub{
		claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) {
			go func() {
				time.Sleep(20 * time.Millisecond)
				cancel()
			}()
			return nil, domain.ErrNoProcessingJob
		},
	}}

	done := make(chan struct{})
	start := time.Now()
	go func() {
		defer close(done)
		StartUploadWorker(ctx, NewSpecUploadProcessor(repo, &objectStoreStub{}, nil), config.WorkerConfig{})
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker blocked on the default 2s poll instead of honoring cancellation")
	}
	assert.Less(t, time.Since(start), time.Second)
	require.Error(t, ctx.Err())
}
