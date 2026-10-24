package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	filestorageDomain "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
	notificationDomain "github.com/saransh1220/blueprint-audio/internal/modules/notification/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const coverageTestMP3 = "SUQzBAAAAAAAI1RTU0UAAAAPAAADTGF2ZjU3LjcxLjEwMAAAAAAAAAAAAAAA//NgxAAdI/3kAUMYAAAAKu7uBgAAIREREd3d3dwMAAABOuaAYt+J/+iIhaIiIiJ/u7u5//9cAEJ/6O7u7/u7u5/+7ufEAwN3f0R3d3d3f//9E///93d+u7u7v//ERHf93c/0L9Hd3d3d0LiIiF/7u7l/+iAYGBu7vo7u/9cAEIGJdRkMtpsbBo9D6hoNBqLv8AvDJXXo/zsRNehi//NixBol6r7uX5iRIv+EFoA4bcpBaYG6ga2BL2SIo+AVYlMcZOMp1IGgYnGTL4nwvldMsp9qAYkFwIsmeZjRO2wXMCdDwgGKQHgn16Rmmh/z6CBTPidyDkTLRw7oOm57/+QMiZ43UggmYl9yDl9lM1fqTf//zcvl963LjKOKBILmjDU3f/Wb/9xQwmq28GRTlt2zWsJJBugJoak/BP/zYsQSJHNW2j/PWALsLp9JKVJlM25CqLiqfiEy6tQMD7eB4TdFplR6HFY7TpajY2rE1EdBci2qfLbuHOduci2WnWy6LbJq1rVWu3Q69zG1C0tb/CZ2aYrTrzznf///3DnLmzk4QQtKF1N0HFta+recr+pmnXbuIdMV////+yrZWxlzT7WmJpb/9QGfrorHZUFqOW6qYbUDJos="

// queueRepoStub adds the maintenance methods the shared stub does not implement.
type queueRepoStub struct {
	*uploadRepositoryStub

	requeueFn func(context.Context, time.Time) (int64, error)
	expireFn  func(context.Context, time.Time) (int64, error)
}

func (s *queueRepoStub) RequeueStaleJobs(ctx context.Context, before time.Time) (int64, error) {
	if s.requeueFn == nil {
		return 0, nil
	}
	return s.requeueFn(ctx, before)
}

func (s *queueRepoStub) ExpireUploadSessions(ctx context.Context, before time.Time) (int64, error) {
	if s.expireFn == nil {
		return 0, nil
	}
	return s.expireFn(ctx, before)
}

type notifierStub struct {
	mu    sync.Mutex
	calls []notifyCall
	err   error
}

type notifyCall struct {
	userID  uuid.UUID
	title   string
	message string
	kind    notificationDomain.NotificationType
}

func (n *notifierStub) Create(
	_ context.Context,
	userID uuid.UUID,
	title, message string,
	kind notificationDomain.NotificationType,
) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls = append(n.calls, notifyCall{userID, title, message, kind})
	return n.err
}

func (n *notifierStub) snapshot() []notifyCall {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]notifyCall(nil), n.calls...)
}

// procFixture is an in-memory object store plus a beat bundle that processes cleanly
// until a test breaks one specific thing.
type procFixture struct {
	bundle *domain.ProcessingBundle
	store  *objectStoreStub

	mu       sync.Mutex
	files    map[string][]byte
	ctypes   map[string]string
	etags    map[string]string
	deleted  []string
	uploaded map[string][]byte

	statErr   map[string]error
	openErr   map[string]error
	urlErr    map[string]error
	copyErr   error
	uploadErr error
	sizeLie   map[string]int64
}

func newProcFixture(t *testing.T) *procFixture {
	t.Helper()

	mp3, err := base64.StdEncoding.DecodeString(coverageTestMP3)
	require.NoError(t, err)
	var img bytes.Buffer
	require.NoError(t, png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 40, 20))))

	stems := make([]byte, 2048)
	copy(stems, []byte{'P', 'K', 0x03, 0x04})
	binary.LittleEndian.PutUint16(stems[8:10], 8)
	binary.LittleEndian.PutUint16(stems[26:28], 5)

	specID := uuid.New()
	fx := &procFixture{
		files:    map[string][]byte{},
		ctypes:   map[string]string{},
		etags:    map[string]string{},
		uploaded: map[string][]byte{},
		statErr:  map[string]error{},
		openErr:  map[string]error{},
		urlErr:   map[string]error{},
		sizeLie:  map[string]int64{},
	}
	bundle := &domain.ProcessingBundle{
		Job:  domain.SpecProcessingJob{ID: uuid.New(), SpecID: specID},
		Spec: domain.Spec{ID: specID, ProducerID: uuid.New(), Title: "Night Drive", Category: domain.CategoryBeat},
	}
	add := func(kind domain.UploadAssetKind, name, ctype string, data []byte) {
		src := "incoming/" + name
		dst := "processing/" + name
		size := int64(len(data))
		ct := ctype
		etag := "etag-" + name
		fx.files[src], fx.ctypes[src], fx.etags[src] = data, ctype, etag
		bundle.Assets = append(bundle.Assets, domain.SpecUploadAsset{
			Kind: kind, ObjectKey: src, FinalObjectKey: dst,
			ActualSize: &size, ActualContentType: &ct, ETag: &etag,
		})
	}
	add(domain.UploadAssetImage, "cover.png", "image/png", img.Bytes())
	add(domain.UploadAssetPreview, "preview.mp3", "audio/mpeg", mp3)
	add(domain.UploadAssetWAV, "track.wav", "audio/wav", minimalPCM16WAV())
	add(domain.UploadAssetStems, "stems.zip", "application/zip", stems)
	fx.bundle = bundle

	fx.store = &objectStoreStub{
		statObjectFn: func(_ context.Context, key string) (filestorageDomain.ObjectInfo, error) {
			fx.mu.Lock()
			defer fx.mu.Unlock()
			if err := fx.statErr[key]; err != nil {
				return filestorageDomain.ObjectInfo{}, err
			}
			data, ok := fx.files[key]
			if !ok {
				return filestorageDomain.ObjectInfo{}, errors.New("not found: " + key)
			}
			size := int64(len(data))
			if lie, ok := fx.sizeLie[key]; ok {
				size = lie
			}
			return filestorageDomain.ObjectInfo{
				Key: key, Size: size, ContentType: fx.ctypes[key], ETag: fx.etags[key],
			}, nil
		},
		openObjectFn: func(_ context.Context, key string) (io.ReadCloser, error) {
			fx.mu.Lock()
			defer fx.mu.Unlock()
			if err := fx.openErr[key]; err != nil {
				return nil, err
			}
			return io.NopCloser(bytes.NewReader(fx.files[key])), nil
		},
		copyObjectFn: func(_ context.Context, src, dst, _ string) (string, error) {
			fx.mu.Lock()
			defer fx.mu.Unlock()
			if fx.copyErr != nil {
				return "", fx.copyErr
			}
			fx.files[dst], fx.ctypes[dst], fx.etags[dst] = fx.files[src], fx.ctypes[src], fx.etags[src]
			return dst, nil
		},
		objectURLFn: func(key string) (string, error) {
			fx.mu.Lock()
			defer fx.mu.Unlock()
			if err := fx.urlErr[key]; err != nil {
				return "", err
			}
			return "https://cdn.test/" + key, nil
		},
		uploadWithKeyFn: func(_ context.Context, r io.Reader, key, _ string) (string, error) {
			fx.mu.Lock()
			defer fx.mu.Unlock()
			if fx.uploadErr != nil {
				return "", fx.uploadErr
			}
			data, _ := io.ReadAll(r)
			fx.uploaded[key] = data
			fx.files[key] = data
			return key, nil
		},
		deleteFn: func(_ context.Context, key string) error {
			fx.mu.Lock()
			defer fx.mu.Unlock()
			fx.deleted = append(fx.deleted, key)
			return nil
		},
	}
	return fx
}

func (fx *procFixture) deletedKeys() []string {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	return append([]string(nil), fx.deleted...)
}

func (fx *procFixture) asset(kind domain.UploadAssetKind) *domain.SpecUploadAsset {
	for i := range fx.bundle.Assets {
		if fx.bundle.Assets[i].Kind == kind {
			return &fx.bundle.Assets[i]
		}
	}
	return nil
}

func (fx *procFixture) dropAsset(kind domain.UploadAssetKind) {
	kept := fx.bundle.Assets[:0]
	for _, a := range fx.bundle.Assets {
		if a.Kind != kind {
			kept = append(kept, a)
		}
	}
	fx.bundle.Assets = kept
}

func TestSpecUploadProcessor_RequeueAndExpireDelegateToRepository(t *testing.T) {
	t.Parallel()

	var staleBefore, expiredBefore time.Time
	repo := &queueRepoStub{
		uploadRepositoryStub: &uploadRepositoryStub{},
		requeueFn: func(_ context.Context, before time.Time) (int64, error) {
			staleBefore = before
			return 3, nil
		},
		expireFn: func(_ context.Context, before time.Time) (int64, error) {
			expiredBefore = before
			return 0, errors.New("expire boom")
		},
	}
	p := NewSpecUploadProcessor(repo, &objectStoreStub{}, nil)

	start := time.Now().UTC()
	n, err := p.RequeueStale(context.Background(), time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	assert.WithinDuration(t, start.Add(-time.Hour), staleBefore, 5*time.Second)

	_, err = p.ExpireUploads(context.Background())
	require.EqualError(t, err, "expire boom")
	assert.WithinDuration(t, start, expiredBefore, 5*time.Second)
}

func TestSpecUploadProcessor_ProcessNextClaimOutcomes(t *testing.T) {
	t.Parallel()

	t.Run("no job available is not an error", func(t *testing.T) {
		t.Parallel()
		repo := &uploadRepositoryStub{claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) {
			return nil, domain.ErrNoProcessingJob
		}}
		processed, err := NewSpecUploadProcessor(repo, &objectStoreStub{}, nil).
			ProcessNext(context.Background(), "w", time.Hour)
		require.NoError(t, err)
		assert.False(t, processed)
	})

	t.Run("claim failure is returned", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("db down")
		repo := &uploadRepositoryStub{claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) {
			return nil, boom
		}}
		processed, err := NewSpecUploadProcessor(repo, &objectStoreStub{}, nil).
			ProcessNext(context.Background(), "w", time.Hour)
		require.ErrorIs(t, err, boom)
		assert.False(t, processed)
	})
}

func TestSpecUploadProcessor_ProcessNextSuccess(t *testing.T) {
	t.Parallel()

	fx := newProcFixture(t)
	var completed domain.ProcessedSpecFiles
	var completedWorker string
	repo := &uploadRepositoryStub{
		claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) { return fx.bundle, nil },
		completeJobFn: func(_ context.Context, id uuid.UUID, worker string, res domain.ProcessedSpecFiles) error {
			assert.Equal(t, fx.bundle.Job.ID, id)
			completedWorker = worker
			completed = res
			return nil
		},
	}
	notifier := &notifierStub{}

	processed, err := NewSpecUploadProcessor(repo, fx.store, notifier).
		ProcessNext(context.Background(), "worker-ok", time.Hour)
	require.NoError(t, err)
	assert.True(t, processed)
	assert.Equal(t, "worker-ok", completedWorker)

	specID := fx.bundle.Spec.ID
	assert.Equal(t, fmt.Sprintf("https://cdn.test/images/%s.jpg", specID), completed.ImageURL)
	assert.Equal(t, "https://cdn.test/processing/preview.mp3", completed.PreviewURL)
	require.NotNil(t, completed.WAVURL)
	assert.Equal(t, "https://cdn.test/processing/track.wav", *completed.WAVURL)
	require.NotNil(t, completed.StemsURL)
	assert.Equal(t, "https://cdn.test/processing/stems.zip", *completed.StemsURL)
	assert.Positive(t, completed.Duration)
	assert.Len(t, completed.WaveformPeaks, 64)
	assert.Contains(t, fx.uploaded, fmt.Sprintf("images/%s.jpg", specID))

	deleted := fx.deletedKeys()
	for _, a := range fx.bundle.Assets {
		assert.Contains(t, deleted, a.ObjectKey)
	}
	assert.Contains(t, deleted, "processing/cover.png", "image source copy is removed after completion")
	assert.NotContains(t, deleted, "processing/track.wav")

	calls := notifier.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, fx.bundle.Spec.ProducerID, calls[0].userID)
	assert.Equal(t, "Upload Complete", calls[0].title)
	assert.Equal(t, "'Night Drive' is now live!", calls[0].message)
	assert.Equal(t, notificationDomain.NotificationTypeSuccess, calls[0].kind)
}

func TestSpecUploadProcessor_ProcessNextCompleteJobErrorKeepsObjects(t *testing.T) {
	t.Parallel()

	fx := newProcFixture(t)
	boom := errors.New("commit ambiguous")
	repo := &uploadRepositoryStub{
		claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) { return fx.bundle, nil },
		completeJobFn: func(context.Context, uuid.UUID, string, domain.ProcessedSpecFiles) error {
			return boom
		},
	}
	notifier := &notifierStub{}

	processed, err := NewSpecUploadProcessor(repo, fx.store, notifier).
		ProcessNext(context.Background(), "w", time.Hour)
	require.ErrorIs(t, err, boom)
	assert.True(t, processed)
	assert.Empty(t, fx.deletedKeys())
	assert.Empty(t, notifier.snapshot())
}

func TestSpecUploadProcessor_ProcessNextFailureNotifiesProducer(t *testing.T) {
	t.Parallel()

	fx := newProcFixture(t)
	fx.dropAsset(domain.UploadAssetPreview)
	var reason string
	repo := &uploadRepositoryStub{
		claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) { return fx.bundle, nil },
		failJobFn: func(_ context.Context, _ uuid.UUID, _, r string) error {
			reason = r
			return nil
		},
	}
	notifier := &notifierStub{err: errors.New("notify down")}

	processed, err := NewSpecUploadProcessor(repo, fx.store, notifier).
		ProcessNext(context.Background(), "w", time.Hour)
	require.NoError(t, err, "notifier errors must not fail the job")
	assert.True(t, processed)
	assert.Equal(t, "MP3 preview is missing", reason)
	assert.Contains(t, fx.deletedKeys(), fmt.Sprintf("images/%s.jpg", fx.bundle.Spec.ID))

	calls := notifier.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, "Upload Failed", calls[0].title)
	assert.Contains(t, calls[0].message, "Night Drive")
	assert.Equal(t, notificationDomain.NotificationTypeError, calls[0].kind)
}

func TestSpecUploadProcessor_ProcessNextCancelledSkipsFailJob(t *testing.T) {
	t.Parallel()

	fx := newProcFixture(t)
	fx.store.statObjectFn = func(ctx context.Context, _ string) (filestorageDomain.ObjectInfo, error) {
		return filestorageDomain.ObjectInfo{}, context.Canceled
	}
	repo := &uploadRepositoryStub{
		claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) { return fx.bundle, nil },
		failJobFn: func(context.Context, uuid.UUID, string, string) error {
			t.Error("FailJob must not run when processing was canceled")
			return nil
		},
	}

	processed, err := NewSpecUploadProcessor(repo, fx.store, nil).
		ProcessNext(context.Background(), "w", time.Hour)
	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, processed)
	assert.Empty(t, fx.deletedKeys())
}

func TestSpecUploadProcessor_ProcessNextFailJobErrorIsWrapped(t *testing.T) {
	t.Parallel()

	fx := newProcFixture(t)
	fx.dropAsset(domain.UploadAssetImage)
	markErr := errors.New("mark failed")
	repo := &uploadRepositoryStub{
		claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) { return fx.bundle, nil },
		failJobFn:      func(context.Context, uuid.UUID, string, string) error { return markErr },
	}

	processed, err := NewSpecUploadProcessor(repo, fx.store, nil).
		ProcessNext(context.Background(), "w", time.Hour)
	require.ErrorIs(t, err, markErr)
	assert.ErrorContains(t, err, "cover image is missing")
	assert.True(t, processed)
	assert.Empty(t, fx.deletedKeys())
}

func TestSpecUploadProcessor_HeartbeatKeepsLeaseWhileProcessing(t *testing.T) {
	t.Parallel()

	fx := newProcFixture(t)
	var mu sync.Mutex
	beats := 0
	slow := fx.store.statObjectFn
	fx.store.statObjectFn = func(ctx context.Context, key string) (filestorageDomain.ObjectInfo, error) {
		time.Sleep(15 * time.Millisecond)
		return slow(ctx, key)
	}
	repo := &uploadRepositoryStub{
		claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) { return fx.bundle, nil },
		heartbeatJobFn: func(_ context.Context, id uuid.UUID, worker string) error {
			assert.Equal(t, fx.bundle.Job.ID, id)
			assert.Equal(t, "hb", worker)
			mu.Lock()
			beats++
			mu.Unlock()
			return nil
		},
		completeJobFn: func(context.Context, uuid.UUID, string, domain.ProcessedSpecFiles) error { return nil },
	}

	processed, err := NewSpecUploadProcessor(repo, fx.store, nil).
		ProcessNext(context.Background(), "hb", time.Millisecond)
	require.NoError(t, err)
	assert.True(t, processed)
	mu.Lock()
	defer mu.Unlock()
	assert.Positive(t, beats)
}

func TestSpecUploadProcessor_HeartbeatDefaultsIntervalAndStopsOnCancel(t *testing.T) {
	t.Parallel()

	p := NewSpecUploadProcessor(&uploadRepositoryStub{}, &objectStoreStub{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go p.heartbeat(ctx, func() {}, done, uuid.New(), "w", 0)
	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not stop after cancellation")
	}
}

func TestSpecUploadProcessor_HeartbeatErrorAfterCancelIsIgnored(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	repo := &uploadRepositoryStub{
		heartbeatJobFn: func(context.Context, uuid.UUID, string) error {
			cancel()
			return errors.New("ctx gone")
		},
	}
	p := NewSpecUploadProcessor(repo, &objectStoreStub{}, nil)
	done := make(chan error, 1)
	go p.heartbeat(ctx, func() { t.Error("must not cancel processing") }, done, uuid.New(), "w", time.Millisecond)
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not return")
	}
}

func TestSpecUploadProcessor_ProcessErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name    string
		mutate  func(fx *procFixture)
		wantErr string
	}{
		{
			name: "duplicate asset kind",
			mutate: func(fx *procFixture) {
				fx.bundle.Assets = append(fx.bundle.Assets, *fx.asset(domain.UploadAssetImage))
			},
			wantErr: "duplicate image asset",
		},
		{
			name:    "source stat fails",
			mutate:  func(fx *procFixture) { fx.statErr["incoming/cover.png"] = boom },
			wantErr: "verify image asset before copy: boom",
		},
		{
			name:    "source size changed",
			mutate:  func(fx *procFixture) { fx.sizeLie["incoming/cover.png"] = 1 },
			wantErr: "image asset changed after upload completion",
		},
		{
			name:    "source etag changed",
			mutate:  func(fx *procFixture) { fx.etags["incoming/cover.png"] = "other" },
			wantErr: "image asset changed after upload completion",
		},
		{
			name:    "source content type changed",
			mutate:  func(fx *procFixture) { fx.ctypes["incoming/cover.png"] = "image/gif" },
			wantErr: "image asset changed after upload completion",
		},
		{
			name:    "recorded size missing",
			mutate:  func(fx *procFixture) { fx.asset(domain.UploadAssetImage).ActualSize = nil },
			wantErr: "image asset changed after upload completion",
		},
		{
			name:    "copy fails",
			mutate:  func(fx *procFixture) { fx.copyErr = boom },
			wantErr: "copy image asset: boom",
		},
		{
			name:    "copied object stat fails",
			mutate:  func(fx *procFixture) { fx.statErr["processing/cover.png"] = boom },
			wantErr: "verify copied image asset: boom",
		},
		{
			name:    "copied object size differs",
			mutate:  func(fx *procFixture) { fx.sizeLie["processing/cover.png"] = 3 },
			wantErr: "copied image asset size does not match",
		},
		{
			name:    "cover missing",
			mutate:  func(fx *procFixture) { fx.dropAsset(domain.UploadAssetImage) },
			wantErr: "cover image is missing",
		},
		{
			name:    "preview missing",
			mutate:  func(fx *procFixture) { fx.dropAsset(domain.UploadAssetPreview) },
			wantErr: "MP3 preview is missing",
		},
		{
			name: "cover too large to read",
			mutate: func(fx *procFixture) {
				fx.sizeLie["processing/cover.png"] = uploadSizeLimits[domain.UploadAssetImage] + 1
				fx.sizeLie["incoming/cover.png"] = uploadSizeLimits[domain.UploadAssetImage] + 1
				size := uploadSizeLimits[domain.UploadAssetImage] + 1
				fx.asset(domain.UploadAssetImage).ActualSize = &size
			},
			wantErr: "read cover image: object exceeds its size limit",
		},
		{
			name: "cover is not an image",
			mutate: func(fx *procFixture) {
				fx.files["incoming/cover.png"] = []byte("not an image")
				size := int64(len("not an image"))
				fx.asset(domain.UploadAssetImage).ActualSize = &size
			},
			wantErr: "cover image must be a valid JPEG or PNG",
		},
		{
			name:    "processed cover upload fails",
			mutate:  func(fx *procFixture) { fx.uploadErr = boom },
			wantErr: "upload processed cover: boom",
		},
		{
			name:    "preview cannot be opened",
			mutate:  func(fx *procFixture) { fx.openErr["processing/preview.mp3"] = boom },
			wantErr: "open preview: boom",
		},
		{
			name: "preview is not an mp3",
			mutate: func(fx *procFixture) {
				fx.files["incoming/preview.mp3"] = bytes.Repeat([]byte{0}, 64)
				size := int64(64)
				fx.asset(domain.UploadAssetPreview).ActualSize = &size
			},
			wantErr: "",
		},
		{
			name:    "wav missing for beat",
			mutate:  func(fx *procFixture) { fx.dropAsset(domain.UploadAssetWAV) },
			wantErr: "WAV asset is missing",
		},
		{
			name: "wav invalid",
			mutate: func(fx *procFixture) {
				fx.files["incoming/track.wav"] = bytes.Repeat([]byte{1}, 64)
				size := int64(64)
				fx.asset(domain.UploadAssetWAV).ActualSize = &size
			},
			wantErr: "wav must be a WAV file",
		},
		{
			name:    "wav url fails",
			mutate:  func(fx *procFixture) { fx.urlErr["processing/track.wav"] = boom },
			wantErr: "boom",
		},
		{
			name:    "stems missing for beat",
			mutate:  func(fx *procFixture) { fx.dropAsset(domain.UploadAssetStems) },
			wantErr: "stems asset is missing",
		},
		{
			name: "stems invalid",
			mutate: func(fx *procFixture) {
				fx.files["incoming/stems.zip"] = bytes.Repeat([]byte{7}, 2048)
			},
			wantErr: "stems archive contents do not match its extension",
		},
		{
			name:    "stems url fails",
			mutate:  func(fx *procFixture) { fx.urlErr["processing/stems.zip"] = boom },
			wantErr: "boom",
		},
		{
			name: "cover url fails",
			mutate: func(fx *procFixture) {
				fx.urlErr[fmt.Sprintf("images/%s.jpg", fx.bundle.Spec.ID)] = boom
			},
			wantErr: "boom",
		},
		{
			name:    "preview url fails",
			mutate:  func(fx *procFixture) { fx.urlErr["processing/preview.mp3"] = boom },
			wantErr: "boom",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fx := newProcFixture(t)
			tt.mutate(fx)
			p := NewSpecUploadProcessor(&uploadRepositoryStub{}, fx.store, nil)

			_, keys, err := p.process(context.Background(), fx.bundle)
			require.Error(t, err)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			}
			assert.Contains(t, keys, fmt.Sprintf("images/%s.jpg", fx.bundle.Spec.ID),
				"cleanup always includes the processed cover key")
		})
	}
}

func TestSpecUploadProcessor_ProcessNonBeatDoesNotRequireWAVOrStems(t *testing.T) {
	t.Parallel()

	fx := newProcFixture(t)
	fx.dropAsset(domain.UploadAssetWAV)
	fx.dropAsset(domain.UploadAssetStems)
	fx.bundle.Spec.Category = domain.CategorySample

	result, _, err := NewSpecUploadProcessor(&uploadRepositoryStub{}, fx.store, nil).
		process(context.Background(), fx.bundle)
	require.NoError(t, err)
	assert.Nil(t, result.WAVURL)
	assert.Nil(t, result.StemsURL)
	assert.NotEmpty(t, result.ImageURL)
}

func pngWithIHDR(width, height uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 0 // grayscale
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(ihdr)))
	chunk := append([]byte("IHDR"), ihdr...)
	buf.Write(chunk)
	_ = binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return buf.Bytes()
}

func TestNormalizeCoverImageRejections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []byte
		wantErr string
	}{
		{"garbage", []byte("definitely not an image"), "valid JPEG or PNG"},
		{"width over limit", pngWithIHDR(maxCoverDimension+1, 10), "dimensions are too large"},
		{"height over limit", pngWithIHDR(10, maxCoverDimension+1), "dimensions are too large"},
		{"pixel count over limit", pngWithIHDR(7000, 6000), "dimensions are too large"},
		{"header ok but no pixel data", pngWithIHDR(10, 10), "decode cover image"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := normalizeCoverImage(tt.input)
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.Nil(t, out)
		})
	}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestSpecUploadProcessor_ReadBounded(t *testing.T) {
	t.Parallel()

	const key = "k"
	boom := errors.New("boom")
	tests := []struct {
		name    string
		stat    func(context.Context, string) (filestorageDomain.ObjectInfo, error)
		open    func(context.Context, string) (io.ReadCloser, error)
		want    string
		wantErr string
	}{
		{
			name:    "stat error",
			stat:    func(context.Context, string) (filestorageDomain.ObjectInfo, error) { return filestorageDomain.ObjectInfo{}, boom },
			wantErr: "boom",
		},
		{
			name: "zero size",
			stat: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
				return filestorageDomain.ObjectInfo{Size: 0}, nil
			},
			wantErr: "exceeds its size limit",
		},
		{
			name: "size above limit",
			stat: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
				return filestorageDomain.ObjectInfo{Size: 11}, nil
			},
			wantErr: "exceeds its size limit",
		},
		{
			name: "open error",
			stat: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
				return filestorageDomain.ObjectInfo{Size: 5}, nil
			},
			open:    func(context.Context, string) (io.ReadCloser, error) { return nil, boom },
			wantErr: "boom",
		},
		{
			name: "read error",
			stat: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
				return filestorageDomain.ObjectInfo{Size: 5}, nil
			},
			open: func(context.Context, string) (io.ReadCloser, error) {
				return io.NopCloser(errReader{err: boom}), nil
			},
			wantErr: "boom",
		},
		{
			name: "stream larger than advertised",
			stat: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
				return filestorageDomain.ObjectInfo{Size: 5}, nil
			},
			open: func(context.Context, string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{1}, 50))), nil
			},
			wantErr: "exceeds its size limit",
		},
		{
			name: "within limit",
			stat: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
				return filestorageDomain.ObjectInfo{Size: 5}, nil
			},
			open: func(context.Context, string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader([]byte("hello"))), nil
			},
			want: "hello",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &objectStoreStub{statObjectFn: tt.stat, openObjectFn: tt.open}
			data, err := NewSpecUploadProcessor(&uploadRepositoryStub{}, store, nil).
				readBounded(context.Background(), key, 10)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(data))
		})
	}
}

type wavChunk struct {
	id   string
	body []byte
}

func fmtBody(format, channels uint16, rate, byteRate uint32, align, bits uint16) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b[0:], format)
	binary.LittleEndian.PutUint16(b[2:], channels)
	binary.LittleEndian.PutUint32(b[4:], rate)
	binary.LittleEndian.PutUint32(b[8:], byteRate)
	binary.LittleEndian.PutUint16(b[12:], align)
	binary.LittleEndian.PutUint16(b[14:], bits)
	return b
}

func buildWAV(chunks ...wavChunk) []byte {
	var body bytes.Buffer
	body.WriteString("WAVE")
	for _, c := range chunks {
		body.WriteString(c.id)
		_ = binary.Write(&body, binary.LittleEndian, uint32(len(c.body)))
		body.Write(c.body)
		if len(c.body)%2 == 1 {
			body.WriteByte(0)
		}
	}
	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

func TestSpecUploadProcessor_ValidateWAV(t *testing.T) {
	t.Parallel()

	goodFmt := wavChunk{"fmt ", fmtBody(1, 1, 8000, 16000, 2, 16)}
	goodData := wavChunk{"data", []byte{1, 0, 2, 0}}
	pad := func(b []byte) []byte { return append(b, make([]byte, 64)...) }

	tests := []struct {
		name    string
		content []byte
		wantErr string
	}{
		{"extensible float with extra chunk and odd padding",
			buildWAV(wavChunk{"LIST", []byte{1, 2, 3}}, wavChunk{"fmt ", append(fmtBody(3, 2, 48000, 384000, 8, 32), 0, 0, 0, 0)}, goodData),
			""},
		{"too small", []byte("RIFF"), "too small"},
		{"wrong magic", pad([]byte("RIFX0000WAVE")), "must be a WAV file"},
		{"riff size mismatch", func() []byte {
			w := buildWAV(goodFmt, goodData)
			binary.LittleEndian.PutUint32(w[4:8], 999)
			return w
		}(), "RIFF size does not match"},
		{"truncated chunk header", func() []byte {
			w := append(buildWAV(goodFmt, goodData), 1, 2, 3)
			binary.LittleEndian.PutUint32(w[4:8], uint32(len(w)-8))
			return w
		}(), "truncated chunk header"},
		{"chunk exceeds bounds", func() []byte {
			w := buildWAV(goodFmt, goodData)
			binary.LittleEndian.PutUint32(w[len(w)-8:], 9999)
			return w
		}(), "exceeds RIFF bounds"},
		{"duplicate fmt", buildWAV(goodFmt, goodFmt, goodData), "one valid fmt chunk"},
		{"short fmt chunk", buildWAV(wavChunk{"fmt ", make([]byte, 8)}, wavChunk{"data", make([]byte, 64)}), "one valid fmt chunk"},
		{"unsupported encoding", buildWAV(wavChunk{"fmt ", fmtBody(2, 1, 8000, 16000, 2, 16)}, goodData), "encoding is not supported"},
		{"bad channel count", buildWAV(wavChunk{"fmt ", fmtBody(1, 0, 8000, 0, 0, 16)}, goodData), "channel count or sample rate"},
		{"bad sample rate", buildWAV(wavChunk{"fmt ", fmtBody(1, 1, 100, 200, 2, 16)}, goodData), "channel count or sample rate"},
		{"bad bit depth", buildWAV(wavChunk{"fmt ", fmtBody(1, 1, 8000, 8000, 1, 12)}, goodData), "bit depth is invalid"},
		{"bad block align", buildWAV(wavChunk{"fmt ", fmtBody(1, 1, 8000, 16000, 4, 16)}, goodData), "byte rate or block alignment"},
		{"bad byte rate", buildWAV(wavChunk{"fmt ", fmtBody(1, 1, 8000, 1, 2, 16)}, goodData), "byte rate or block alignment"},
		{"empty data chunk", buildWAV(goodFmt, wavChunk{"data", nil}, wavChunk{"junk", make([]byte, 40)}), "data chunk is empty"},
		{"missing data chunk", buildWAV(goodFmt, wavChunk{"junk", make([]byte, 40)}), "valid fmt and non-empty data"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &objectStoreStub{
				statObjectFn: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
					return filestorageDomain.ObjectInfo{Size: int64(len(tt.content))}, nil
				},
				openObjectFn: func(context.Context, string) (io.ReadCloser, error) {
					return io.NopCloser(bytes.NewReader(tt.content)), nil
				},
			}
			err := NewSpecUploadProcessor(&uploadRepositoryStub{}, store, nil).
				validateWAV(context.Background(), "a.wav")
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSpecUploadProcessor_ValidateWAVStorageFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	ok := minimalPCM16WAV()
	info := filestorageDomain.ObjectInfo{Size: int64(len(ok))}

	t.Run("stat", func(t *testing.T) {
		t.Parallel()
		store := &objectStoreStub{statObjectFn: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
			return filestorageDomain.ObjectInfo{}, boom
		}}
		err := NewSpecUploadProcessor(&uploadRepositoryStub{}, store, nil).validateWAV(context.Background(), "k")
		require.ErrorIs(t, err, boom)
		assert.ErrorContains(t, err, "stat WAV")
	})
	t.Run("open", func(t *testing.T) {
		t.Parallel()
		store := &objectStoreStub{
			statObjectFn: func(context.Context, string) (filestorageDomain.ObjectInfo, error) { return info, nil },
			openObjectFn: func(context.Context, string) (io.ReadCloser, error) { return nil, boom },
		}
		err := NewSpecUploadProcessor(&uploadRepositoryStub{}, store, nil).validateWAV(context.Background(), "k")
		require.ErrorIs(t, err, boom)
		assert.ErrorContains(t, err, "open WAV")
	})
	t.Run("truncated stream", func(t *testing.T) {
		t.Parallel()
		store := &objectStoreStub{
			statObjectFn: func(context.Context, string) (filestorageDomain.ObjectInfo, error) { return info, nil },
			openObjectFn: func(context.Context, string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(ok[:8])), nil
			},
		}
		err := NewSpecUploadProcessor(&uploadRepositoryStub{}, store, nil).validateWAV(context.Background(), "k")
		require.Error(t, err)
		assert.ErrorContains(t, err, "read WAV header")
	})
	t.Run("stream ends inside chunks", func(t *testing.T) {
		t.Parallel()
		for _, cut := range []int{14, 24, 30, 40, 47} {
			cut := cut
			store := &objectStoreStub{
				statObjectFn: func(context.Context, string) (filestorageDomain.ObjectInfo, error) { return info, nil },
				openObjectFn: func(context.Context, string) (io.ReadCloser, error) {
					return io.NopCloser(bytes.NewReader(ok[:cut])), nil
				},
			}
			err := NewSpecUploadProcessor(&uploadRepositoryStub{}, store, nil).validateWAV(context.Background(), "k")
			require.Error(t, err, "cut at %d", cut)
		}
	})
}

func TestSpecUploadProcessor_ValidateStems(t *testing.T) {
	t.Parallel()

	zipHeader := func(flags, compression, nameLen, extraLen uint16, size int) []byte {
		b := make([]byte, size)
		copy(b, []byte{'P', 'K', 0x03, 0x04})
		binary.LittleEndian.PutUint16(b[6:], flags)
		binary.LittleEndian.PutUint16(b[8:], compression)
		binary.LittleEndian.PutUint16(b[26:], nameLen)
		binary.LittleEndian.PutUint16(b[28:], extraLen)
		return b
	}
	rar4 := append([]byte{'R', 'a', 'r', '!', 0x1a, 0x07, 0x00}, make([]byte, 2048)...)
	rar5 := append([]byte{'R', 'a', 'r', '!', 0x1a, 0x07, 0x01, 0x00}, make([]byte, 2048)...)

	tests := []struct {
		name    string
		key     string
		content []byte
		wantErr string
	}{
		{"valid zip", "s.zip", zipHeader(0, 8, 5, 0, 2048), ""},
		{"valid zip stored upper-case ext", "s.ZIP", zipHeader(0, 0, 5, 4, 2048), ""},
		{"valid rar4", "s.rar", rar4, ""},
		{"valid rar5", "s.rar", rar5, ""},
		{"too small", "s.zip", zipHeader(0, 8, 5, 0, 100), "too small"},
		{"encrypted zip", "s.zip", zipHeader(1, 8, 5, 0, 2048), "encrypted"},
		{"unsupported compression", "s.zip", zipHeader(0, 14, 5, 0, 2048), "compression method"},
		{"empty file name", "s.zip", zipHeader(0, 8, 0, 0, 2048), "local file header is invalid"},
		{"name longer than archive", "s.zip", zipHeader(0, 8, 65535, 65535, 2048), "local file header is invalid"},
		{"zip bytes with rar extension", "s.rar", zipHeader(0, 8, 5, 0, 2048), "do not match"},
		{"rar bytes with zip extension", "s.zip", rar4, "do not match"},
		{"unknown extension", "s.7z", zipHeader(0, 8, 5, 0, 2048), "do not match"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &objectStoreStub{
				statObjectFn: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
					return filestorageDomain.ObjectInfo{Size: int64(len(tt.content))}, nil
				},
				openObjectFn: func(context.Context, string) (io.ReadCloser, error) {
					return io.NopCloser(bytes.NewReader(tt.content)), nil
				},
			}
			err := NewSpecUploadProcessor(&uploadRepositoryStub{}, store, nil).
				validateStems(context.Background(), tt.key)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}

	boom := errors.New("boom")
	t.Run("stat error", func(t *testing.T) {
		t.Parallel()
		store := &objectStoreStub{statObjectFn: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
			return filestorageDomain.ObjectInfo{}, boom
		}}
		err := NewSpecUploadProcessor(&uploadRepositoryStub{}, store, nil).validateStems(context.Background(), "s.zip")
		require.ErrorIs(t, err, boom)
		assert.ErrorContains(t, err, "stat stems archive")
	})
	t.Run("header read error", func(t *testing.T) {
		t.Parallel()
		store := &objectStoreStub{
			statObjectFn: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
				return filestorageDomain.ObjectInfo{Size: 4096}, nil
			},
			openObjectFn: func(context.Context, string) (io.ReadCloser, error) { return nil, boom },
		}
		err := NewSpecUploadProcessor(&uploadRepositoryStub{}, store, nil).validateStems(context.Background(), "s.zip")
		require.ErrorIs(t, err, boom)
		assert.ErrorContains(t, err, "read stems header")
	})
}

func TestIsRARHeader(t *testing.T) {
	t.Parallel()

	assert.True(t, isRARHeader([]byte{'R', 'a', 'r', '!', 0x1a, 0x07, 0x00, 9}))
	assert.True(t, isRARHeader([]byte{'R', 'a', 'r', '!', 0x1a, 0x07, 0x01, 0x00}))
	assert.False(t, isRARHeader([]byte{'R', 'a', 'r', '!', 0x1a, 0x07, 0x01, 0x01}))
	assert.False(t, isRARHeader(nil))
}

func TestSpecUploadProcessor_NotifyWithoutNotifierIsNoop(t *testing.T) {
	t.Parallel()

	p := NewSpecUploadProcessor(&uploadRepositoryStub{}, &objectStoreStub{}, nil)
	assert.NotPanics(t, func() {
		p.notify(context.Background(), uuid.New(), "t", "m", notificationDomain.NotificationTypeInfo)
	})
}
