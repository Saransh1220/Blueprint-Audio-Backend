package local

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
	"github.com/stretchr/testify/require"
)

func TestLocalStorage_EndToEnd(t *testing.T) {
	base := t.TempDir()
	ls, err := NewLocalStorage(base, "http://localhost/uploads")
	require.NoError(t, err)

	url, err := ls.UploadFile(context.Background(), "a/b.txt", bytes.NewBufferString("hello"), "text/plain")
	require.NoError(t, err)
	require.Equal(t, "http://localhost/uploads/a/b.txt", url)

	full := filepath.Join(base, "a/b.txt")
	_, err = os.Stat(full)
	require.NoError(t, err)

	p, err := ls.GetPresignedURL(context.Background(), "a/b.txt", time.Minute)
	require.NoError(t, err)
	require.Equal(t, url, p)

	d, err := ls.GetPresignedDownloadURL(context.Background(), "a/b.txt", "x.txt", time.Minute)
	require.NoError(t, err)
	require.Equal(t, url, d)

	k, err := ls.GetKeyFromURL(url)
	require.NoError(t, err)
	require.Equal(t, "a/b.txt", k)

	objectURL, err := ls.ObjectURL("a/b.txt")
	require.NoError(t, err)
	require.Equal(t, url, objectURL)

	_, err = ls.CreatePresignedUpload(context.Background(), "a/b.txt", "text/plain", 5, time.Minute)
	require.ErrorIs(t, err, domain.ErrDirectUploadUnsupported)
	_, err = ls.StatObject(context.Background(), "a/b.txt")
	require.ErrorIs(t, err, domain.ErrDirectUploadUnsupported)
	_, err = ls.OpenObject(context.Background(), "a/b.txt")
	require.ErrorIs(t, err, domain.ErrDirectUploadUnsupported)
	_, err = ls.CopyObject(context.Background(), "a/b.txt", "a/c.txt", "etag")
	require.ErrorIs(t, err, domain.ErrDirectUploadUnsupported)

	err = ls.DeleteFile(context.Background(), "a/b.txt")
	require.NoError(t, err)

	_, err = ls.GetKeyFromURL("http://bad/x")
	require.Error(t, err)
}
