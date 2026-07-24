package application

import (
	"bytes"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type readerOnly struct {
	reader io.Reader
}

func (r readerOnly) Read(buffer []byte) (int, error) {
	return r.reader.Read(buffer)
}

func TestExtractWaveformPeaksRejectsInvalidBarCounts(t *testing.T) {
	for _, barCount := range []int{-1, 0, 513} {
		_, err := ExtractWaveformPeaks(strings.NewReader("not an mp3"), barCount)
		require.Error(t, err)
	}
}

func TestExtractWaveformPeaksRejectsInvalidMP3(t *testing.T) {
	_, err := ExtractWaveformPeaks(strings.NewReader("not an mp3"), 64)
	require.Error(t, err)
}

func TestExtractWaveformPeaksDecodesMP3(t *testing.T) {
	const encodedMP3 = "SUQzBAAAAAAAI1RTU0UAAAAPAAADTGF2ZjU3LjcxLjEwMAAAAAAAAAAAAAAA//NgxAAdI/3kAUMYAAAAKu7uBgAAIREREd3d3dwMAAABOuaAYt+J/+iIhaIiIiJ/u7u5//9cAEJ/6O7u7/u7u5/+7ufEAwN3f0R3d3d3f//9E///93d+u7u7v//ERHf93c/0L9Hd3d3d0LiIiF/7u7l/+iAYGBu7vo7u/9cAEIGJdRkMtpsbBo9D6hoNBqLv8AvDJXXo/zsRNehi//NixBol6r7uX5iRIv+EFoA4bcpBaYG6ga2BL2SIo+AVYlMcZOMp1IGgYnGTL4nwvldMsp9qAYkFwIsmeZjRO2wXMCdDwgGKQHgn16Rmmh/z6CBTPidyDkTLRw7oOm57/+QMiZ43UggmYl9yDl9lM1fqTf//zcvl963LjKOKBILmjDU3f/Wb/9xQwmq28GRTlt2zWsJJBugJoak/BP/zYsQSJHNW2j/PWALsLp9JKVJlM25CqLiqfiEy6tQMD7eB4TdFplR6HFY7TpajY2rE1EdBci2qfLbuHOduci2WnWy6LbJq1rVWu3Q69zG1C0tb/CZ2aYrTrzznf///3DnLmzk4QQtKF1N0HFta+recr+pmnXbuIdMV////+yrZWxlzT7WmJpb/9QGfrorHZUFqOW6qYbUDJos="
	data, err := base64.StdEncoding.DecodeString(encodedMP3)
	require.NoError(t, err)

	analysis, err := AnalyzeMP3(readerOnly{reader: bytes.NewReader(data)}, 8)
	require.NoError(t, err)
	require.Greater(t, analysis.Duration, 0)
	require.Len(t, analysis.WaveformPeaks, 8)
	for _, peak := range analysis.WaveformPeaks {
		require.GreaterOrEqual(t, peak, int64(8))
		require.LessOrEqual(t, peak, int64(100))
	}
}

func TestValidateDecodedMP3FrameCountLimitsDuration(t *testing.T) {
	const sampleRate = 44_100
	require.NoError(t, validateDecodedMP3FrameCount(
		int64(sampleRate)*maxPreviewDurationSeconds, sampleRate,
	))
	require.Error(t, validateDecodedMP3FrameCount(
		int64(sampleRate)*maxPreviewDurationSeconds+1, sampleRate,
	))
}
