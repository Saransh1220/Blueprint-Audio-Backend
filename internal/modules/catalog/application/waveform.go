package application

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/hajimehoshi/go-mp3"
)

type MP3Analysis struct {
	WaveformPeaks []int64
	Duration      int
}

const maxPreviewDurationSeconds = 30 * 60

// AnalyzeMP3 decodes an MP3 once, deriving both its duration and normalized
// waveform without retaining the decoded PCM in memory. It intentionally does
// not use Decoder.Length because object-storage response bodies are streams,
// not io.Seeker values, and go-mp3 reports an unknown length for them.
func AnalyzeMP3(source io.Reader, barCount int) (MP3Analysis, error) {
	if barCount <= 0 || barCount > 512 {
		return MP3Analysis{}, fmt.Errorf("invalid waveform bar count")
	}
	decoder, err := mp3.NewDecoder(source)
	if err != nil {
		return MP3Analysis{}, fmt.Errorf("decode MP3 waveform: %w", err)
	}
	sampleRate := decoder.SampleRate()
	if sampleRate <= 0 {
		return MP3Analysis{}, fmt.Errorf("MP3 has an invalid sample rate")
	}

	type waveformChunk struct {
		sum   uint64
		count uint64
	}
	const maxWaveformChunks = 16_384
	chunkFrames := int64(sampleRate / 20) // approximately 50 ms
	if chunkFrames < 1 {
		chunkFrames = 1
	}
	chunks := make([]waveformChunk, 0, 512)
	var chunk waveformChunk
	var totalFrames int64

	// Preserve incomplete stereo frames across decoder.Read calls.
	buffer := make([]byte, 32*1024+3)
	pending := 0
	flushChunk := func() {
		if chunk.count == 0 {
			return
		}
		chunks = append(chunks, chunk)
		chunk = waveformChunk{}
		if len(chunks) < maxWaveformChunks {
			return
		}
		// Keep memory bounded for unusually long previews. Adjacent summaries
		// are losslessly merged for average-amplitude purposes, and future
		// chunks cover the same doubled time window.
		merged := chunks[:0]
		for i := 0; i < len(chunks); i += 2 {
			if i+1 == len(chunks) {
				merged = append(merged, chunks[i])
				continue
			}
			merged = append(merged, waveformChunk{
				sum:   chunks[i].sum + chunks[i+1].sum,
				count: chunks[i].count + chunks[i+1].count,
			})
		}
		chunks = merged
		chunkFrames *= 2
	}

	for {
		read, readErr := decoder.Read(buffer[pending : len(buffer)-3+pending])
		available := pending + read
		complete := available - available%4
		for offset := 0; offset < complete; offset += 4 {
			left := int64(int16(binary.LittleEndian.Uint16(buffer[offset : offset+2])))
			right := int64(int16(binary.LittleEndian.Uint16(buffer[offset+2 : offset+4])))
			if left < 0 {
				left = -left
			}
			if right < 0 {
				right = -right
			}
			chunk.sum += uint64((left + right) / 2)
			chunk.count++
			totalFrames++
			if err := validateDecodedMP3FrameCount(totalFrames, sampleRate); err != nil {
				return MP3Analysis{}, err
			}
			if int64(chunk.count) >= chunkFrames {
				flushChunk()
			}
		}
		pending = available - complete
		copy(buffer[:pending], buffer[complete:available])
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return MP3Analysis{}, fmt.Errorf("read decoded MP3: %w", readErr)
		}
	}
	flushChunk()
	if totalFrames <= 0 || len(chunks) == 0 {
		return MP3Analysis{}, fmt.Errorf("MP3 has no decodable samples")
	}

	chunkAverages := make([]float64, len(chunks))
	for i, value := range chunks {
		chunkAverages[i] = float64(value.sum) / float64(value.count)
	}
	averages := make([]float64, barCount)
	sums := make([]float64, barCount)
	counts := make([]int, barCount)
	for i, average := range chunkAverages {
		bar := i * barCount / len(chunkAverages)
		if bar >= barCount {
			bar = barCount - 1
		}
		sums[bar] += average
		counts[bar]++
	}
	peak := float64(1)
	for i := range averages {
		if counts[i] > 0 {
			averages[i] = sums[i] / float64(counts[i])
		} else {
			// Very short previews may have fewer chunks than requested bars.
			// Repeat the closest time chunk instead of producing artificial
			// silent gaps.
			chunkIndex := ((2*i + 1) * len(chunkAverages)) / (2 * barCount)
			if chunkIndex >= len(chunkAverages) {
				chunkIndex = len(chunkAverages) - 1
			}
			averages[i] = chunkAverages[chunkIndex]
		}
		if averages[i] > peak {
			peak = averages[i]
		}
	}
	result := make([]int64, barCount)
	for i, average := range averages {
		result[i] = int64(math.Max(8, math.Round(average/peak*100)))
	}
	duration := int(math.Ceil(float64(totalFrames) / float64(sampleRate)))
	if duration < 1 {
		duration = 1
	}
	return MP3Analysis{WaveformPeaks: result, Duration: duration}, nil
}

func validateDecodedMP3FrameCount(frameCount int64, sampleRate int) error {
	if sampleRate <= 0 {
		return fmt.Errorf("MP3 has an invalid sample rate")
	}
	if frameCount > int64(sampleRate)*maxPreviewDurationSeconds {
		return fmt.Errorf("MP3 preview exceeds the 30 minute duration limit")
	}
	return nil
}

// ExtractWaveformPeaks remains as a compatibility wrapper for existing callers.
func ExtractWaveformPeaks(source io.Reader, barCount int) ([]int64, error) {
	analysis, err := AnalyzeMP3(source, barCount)
	if err != nil {
		return nil, err
	}
	return analysis.WaveformPeaks, nil
}
