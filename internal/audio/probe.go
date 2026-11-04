package audio

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Info is what a file says about itself without decoding it: the shape of its
// PCM and how long it lasts. The library scan uses it so that adding a folder
// of thousands of files stays fast.
type Info struct {
	Format   Format
	Duration time.Duration
	Bitrate  int // bits per second, zero when unknown
}

// Probe reads just enough of path to describe it.
func Probe(path string) (Info, error) {
	file, err := os.Open(path)
	if err != nil {
		return Info{}, err
	}
	defer file.Close()
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return probeMP3(file)
	case ".flac":
		source, err := newFLACSource(file)
		if err != nil {
			return Info{}, err
		}
		return Info{Format: source.Format(), Duration: source.Duration()}, nil
	case ".wav", ".wave":
		source, err := newWAVSource(file)
		if err != nil {
			return Info{}, err
		}
		info := Info{Format: source.Format(), Duration: source.Duration()}
		if info.Duration > 0 {
			info.Bitrate = int(float64(fileSize(file)) * 8 / info.Duration.Seconds())
		}
		return info, nil
	}
	return Info{}, ErrUnsupported
}

func fileSize(file *os.File) int64 {
	stat, err := file.Stat()
	if err != nil {
		return 0
	}
	return stat.Size()
}

// mp3Bitrates is indexed by version*4+layer, where version is the MPEG version
// field (3 is MPEG1, 2 is MPEG2, 0 is MPEG2.5) and layer is the layer field
// (1 is Layer I, 3 is Layer III). The values are kilobits per second.
var mp3Bitrates = map[int][15]int{
	3*4 + 1: {0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448}, // MPEG1 Layer I
	3*4 + 2: {0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384},    // MPEG1 Layer II
	3*4 + 3: {0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320},     // MPEG1 Layer III
	2*4 + 1: {0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256},    // MPEG2 Layer I
	2*4 + 2: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},         // MPEG2 Layer II
	2*4 + 3: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},         // MPEG2 Layer III
	0*4 + 1: {0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256},    // MPEG2.5 Layer I
	0*4 + 2: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},         // MPEG2.5 Layer II
	0*4 + 3: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},         // MPEG2.5 Layer III
}

var mp3SampleRates = map[int][3]int{
	3: {44100, 48000, 32000}, // MPEG1
	2: {22050, 24000, 16000}, // MPEG2
	0: {11025, 12000, 8000},  // MPEG2.5
}

// probeMP3 reads the first MPEG frame header. A Xing or Info header, which
// encoders write for variable-bitrate files, gives the exact frame count; other
// files are estimated from the first frame's bitrate.
func probeMP3(file *os.File) (Info, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Info{}, err
	}
	head := make([]byte, 64*1024)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return Info{}, err
	}
	head = head[:n]
	start := 0
	if len(head) >= 10 && string(head[0:3]) == "ID3" {
		size := int(head[6]&0x7F)<<21 | int(head[7]&0x7F)<<14 | int(head[8]&0x7F)<<7 | int(head[9]&0x7F)
		start = 10 + size
	}
	for i := start; i+4 <= len(head); i++ {
		if head[i] != 0xFF || head[i+1]&0xE0 != 0xE0 {
			continue
		}
		version := int(head[i+1] >> 3 & 0x3)
		layer := int(head[i+1] >> 1 & 0x3)
		bitrateIndex := int(head[i+2] >> 4 & 0xF)
		rateIndex := int(head[i+2] >> 2 & 0x3)
		if version == 1 || layer == 0 || bitrateIndex == 0 || bitrateIndex == 15 || rateIndex == 3 {
			continue
		}
		bitrate := mp3Bitrates[version*4+layer][bitrateIndex] * 1000
		sampleRate := mp3SampleRates[version][rateIndex]
		if bitrate == 0 || sampleRate == 0 {
			continue
		}
		samplesPerFrame := 1152
		switch {
		case layer == 1:
			samplesPerFrame = 384
		case version != 3:
			samplesPerFrame = 576
		}
		info := Info{Format: Format{SampleRate: sampleRate, Channels: 2}, Bitrate: bitrate}
		if frames := xingFrames(head[i:], version); frames > 0 {
			info.Duration = time.Duration(float64(frames) * float64(samplesPerFrame) / float64(sampleRate) * float64(time.Second))
			info.Bitrate = int(float64(fileSize(file)) * 8 / info.Duration.Seconds())
			return info, nil
		}
		if size := fileSize(file) - int64(start); size > 0 {
			info.Duration = time.Duration(float64(size) * 8 / float64(bitrate) * float64(time.Second))
		}
		return info, nil
	}
	return Info{}, errors.New("audio: no MPEG frame found")
}

// xingFrames reads the frame count from a Xing or Info header at the start of
// the first frame, or zero when there is none.
func xingFrames(frame []byte, version int) int {
	if len(frame) < 12 {
		return 0
	}
	offset := 4
	if version == 3 {
		offset = 4 + 32 // MPEG1 stereo side information
	} else {
		offset = 4 + 17
	}
	if offset+8 > len(frame) {
		return 0
	}
	tag := string(frame[offset : offset+4])
	if tag != "Xing" && tag != "Info" {
		return 0
	}
	flags := binary.BigEndian.Uint32(frame[offset+4:])
	if flags&0x1 == 0 || offset+12 > len(frame) {
		return 0
	}
	return int(binary.BigEndian.Uint32(frame[offset+8:]))
}
