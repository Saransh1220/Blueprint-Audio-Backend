package application

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/hajimehoshi/go-mp3"
)

// ExtractWaveformPeaks decodes an MP3 as a stream and reduces it to normalized
// stereo amplitude bars without retaining the decoded PCM in memory.
func ExtractWaveformPeaks(source io.Reader, barCount int) ([]int64, error) {
	if barCount <= 0 || barCount > 512 {
		return nil, fmt.Errorf("invalid waveform bar count")
	}
	decoder, err := mp3.NewDecoder(source)
	if err != nil {
		return nil, fmt.Errorf("decode MP3 waveform: %w", err)
	}
	totalFrames := decoder.Length() / 4 // signed 16-bit, stereo
	if totalFrames <= 0 {
		return nil, fmt.Errorf("MP3 has no decodable samples")
	}

	sums := make([]uint64, barCount)
	counts := make([]uint64, barCount)
	buffer := make([]byte, 32*1024)
	var frame int64
	for {
		read, readErr := decoder.Read(buffer)
		for offset := 0; offset+3 < read; offset += 4 {
			left := int64(int16(binary.LittleEndian.Uint16(buffer[offset : offset+2])))
			right := int64(int16(binary.LittleEndian.Uint16(buffer[offset+2 : offset+4])))
			if left < 0 {
				left = -left
			}
			if right < 0 {
				right = -right
			}
			bar := int(frame * int64(barCount) / totalFrames)
			if bar >= barCount {
				bar = barCount - 1
			}
			sums[bar] += uint64((left + right) / 2)
			counts[bar]++
			frame++
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read decoded MP3: %w", readErr)
		}
	}

	averages := make([]float64, barCount)
	peak := float64(1)
	for i := range averages {
		if counts[i] > 0 {
			averages[i] = float64(sums[i]) / float64(counts[i])
		}
		if averages[i] > peak {
			peak = averages[i]
		}
	}
	result := make([]int64, barCount)
	for i, average := range averages {
		result[i] = int64(math.Max(8, math.Round(average/peak*100)))
	}
	return result, nil
}
