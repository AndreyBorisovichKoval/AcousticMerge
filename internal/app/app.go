package app

// C:\_Projects_Go\AcousticMerge\internal\app\app.go
// Package: app
// Назначение: Двухпроходный мердж WAV (PCM16), 2 прогресс-бара (PASS1 scan / PASS2 merge), кроссфейд, нормализация.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"acousticmerge/internal/ui"
)

type wavPCM struct {
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
}
type wavData struct {
	PCM  wavPCM
	Data []int16
}
func (w wavData) TotalSamples() int64 { return int64(len(w.Data)) }

type fileInfo struct {
	Name string
	Path string
	ModTime time.Time
}

type Config = ui.Config

func Run(cfg *Config, U ui.UIAPI) {
	// Параметры
	U.LogInfo("starting…")
	U.PrintKV("Source:", cfg.Src)
	U.PrintKV("Output:", cfg.Out)
	U.PrintKV("Gain:", fmt.Sprintf("%.1f%%", cfg.GainPct))
	U.PrintKV("Order:", string(cfg.Order))
	if cfg.DoNormalize {
		U.PrintKV("Normalize:", fmt.Sprintf("%.2f dBFS", cfg.NormalizeDB))
	}
	if cfg.CrossfadeMS > 0 {
		U.PrintKV("Crossfade:", fmt.Sprintf("%d ms", cfg.CrossfadeMS))
	}
	fmt.Println()

	// Сбор WAV
	files, err := collectWavsRecursive(cfg.Src)
	if err != nil { fatal(U, err) }
	if len(files) == 0 { fatal(U, fmt.Errorf("в папке %s нет WAV-файлов", cfg.Src)) }
	U.LogInfo("found %d files", len(files))

	// Сортировка
	switch cfg.Order {
	case ui.OrderByName:
		sort.Slice(files, func(i, j int) bool {
			return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
		})
	case ui.OrderByMTime:
		sort.Slice(files, func(i, j int) bool { return files[i].ModTime.Before(files[j].ModTime) })
	default:
		fatal(U, fmt.Errorf("неизвестный --order: %s", cfg.Order))
	}

	// Эталон
	first, refPCM, err := readWav(files[0].Path)
	if err != nil { fatal(U, fmt.Errorf("%s: %w", files[0].Path, err)) }
	if refPCM.AudioFormat != 1 || refPCM.BitsPerSample != 16 {
		fatal(U, fmt.Errorf("поддерживается только PCM16 (fmt=1, bps=16). Встретили: fmt=%d, bps=%d",
			refPCM.AudioFormat, refPCM.BitsPerSample))
	}
	channels := int(refPCM.NumChannels)
	sampleRate := int(refPCM.SampleRate)
	U.PrintKV("Format:", fmt.Sprintf("%d Hz, %d ch, %d bps (PCM16)",
		refPCM.SampleRate, refPCM.NumChannels, refPCM.BitsPerSample))

	// Проверка формата strict
	if cfg.StrictFormat {
		for i := 1; i < len(files); i++ {
			_, pcm, err := readWav(files[i].Path)
			if err != nil { fatal(U, fmt.Errorf("%s: %w", files[i].Path, err)) }
			if pcm.AudioFormat != refPCM.AudioFormat ||
				pcm.NumChannels != refPCM.NumChannels ||
				pcm.SampleRate != refPCM.SampleRate ||
				pcm.BitsPerSample != refPCM.BitsPerSample {
				fatal(U, fmt.Errorf("формат файла %s отличается от эталона (strict-mode)", files[i].Name))
			}
			if i%4096 == 0 {
				U.LogInfo("verified %d/%d", i, len(files)-1)
			}
		}
	}

	// Подготовка кроссфейда
	fadeTotal := 0
	if cfg.CrossfadeMS > 0 {
		fadeSamplesPerChan := int((float64(sampleRate) * float64(cfg.CrossfadeMS)) / 1000.0)
		if fadeSamplesPerChan > 0 {
			fadeTotal = fadeSamplesPerChan * channels
		}
	}

	// PASS1: totalSamples и peak
	gain := float32(cfg.GainPct / 100.0)
	var totalSamples int64
	var peak float64
	var havePrev bool
	var prevTail []float32

	totalSamples += int64(len(first.Data))
	if cfg.DoNormalize || fadeTotal > 0 {
		updatePeakWhole(&peak, first.Data, gain)
	}
	if fadeTotal > 0 {
		prevTail = takeTailAsFloat(first.Data, gain, fadeTotal)
		havePrev = true
	}

	U.PrintBar("PASS1 scan:", 1, len(files))
	for i := 1; i < len(files); i++ {
		w, _, err := readWav(files[i].Path)
		if err != nil { fatal(U, fmt.Errorf("%s: %w", files[i].Path, err)) }
		totalSamples += int64(len(w.Data))
		if cfg.DoNormalize || fadeTotal > 0 {
			if fadeTotal > 0 && havePrev && len(w.Data) >= fadeTotal {
				head := takeHeadAsFloat(w.Data, gain, fadeTotal)
				updatePeakCrossfade(&peak, prevTail, head)
				updatePeakWhole(&peak, w.Data[fadeTotal:], gain)
				prevTail = takeTailAsFloat(w.Data, gain, fadeTotal)
				havePrev = true
			} else {
				updatePeakWhole(&peak, w.Data, gain)
			}
		}
		U.PrintBar("PASS1 scan:", i+1, len(files))
	}
	U.EndBar()

	if fadeTotal > 0 && len(files) > 1 {
		totalSamples -= int64(fadeTotal * (len(files) - 1))
	}
	if totalSamples < 0 { totalSamples = 0 }

	// Нормализация
	scale := float32(1.0)
	if cfg.DoNormalize && peak > 0 {
		desired := math.Pow(10.0, cfg.NormalizeDB/20.0)
		if desired > 1.0 { desired = 1.0 }
		scale = float32(desired / peak)
		U.LogInfo("normalized: peak %.3f -> %.3f (scale=%.6f)", peak, desired, scale)
	}

	// Длительность
	durSec := float64(totalSamples) / float64(sampleRate*channels)
	U.PrintKV("Duration:", fmt.Sprintf("%.3f s", durSec))
	fmt.Println()

	if cfg.DryRun {
		U.LogOK("dry-run: запись отключена")
		return
	}

	// Создание вывода
	outPath, err := nextAvailablePath(cfg.Out)
	if err != nil { fatal(U, err) }
	if err := ensureDir(filepath.Dir(outPath)); err != nil { fatal(U, err) }
	outf, bw, err := createWavWriter(outPath, uint32(sampleRate), uint16(channels), uint32(totalSamples))
	if err != nil { fatal(U, err) }
	defer func() { bw.Flush(); outf.Close() }()

	// PASS2: запись с фейдом
	written := int64(0)
	writePCM16 := func(vals []int16) {
		if len(vals) == 0 { return }
		if err := binary.Write(bw, binary.LittleEndian, vals); err != nil {
			fatal(U, err)
		}
		written += int64(len(vals))
	}
	toPCM16Scaled := func(f []float32, start, end int) []int16 {
		if start < 0 { start = 0 }
		if end > len(f) { end = len(f) }
		n := end - start
		out := make([]int16, n)
		for i := 0; i < n; i++ {
			v := float64(f[start+i] * gain * scale)
			if v > 1.0 { v = 1.0 }
			if v < -1.0 { v = -1.0 }
			out[i] = int16(math.Round(v * 32767.0))
		}
		return out
	}
	int16ToFloat32 := func(src []int16) []float32 {
		out := make([]float32, len(src))
		const s = 1.0 / 32768.0
		for i := range src { out[i] = float32(src[i]) * float32(s) }
		return out
	}

	firstF := int16ToFloat32(first.Data)
	if fadeTotal > 0 && len(firstF) >= fadeTotal {
		writePCM16(toPCM16Scaled(firstF, 0, len(firstF)-fadeTotal))
		prevTail = firstF[len(firstF)-fadeTotal:]
		havePrev = true
	} else {
		writePCM16(toPCM16Scaled(firstF, 0, len(firstF)))
		havePrev = false
	}

	U.PrintBar("PASS2 merge:", 1, len(files))
	for i := 1; i < len(files); i++ {
		w, _, err := readWav(files[i].Path)
		if err != nil { fatal(U, fmt.Errorf("%s: %w", files[i].Path, err)) }
		curF := int16ToFloat32(w.Data)

		if fadeTotal > 0 && havePrev && len(curF) >= fadeTotal {
			// смешанный фейд
			mix := make([]float32, fadeTotal)
			for k := 0; k < fadeTotal; k++ {
				alpha := float64(k) / float64(fadeTotal)
				mix[k] = float32((1.0-alpha)*float64(prevTail[k]) + alpha*float64(curF[k]))
			}
			writePCM16(toPCM16Scaled(mix, 0, len(mix)))

			// середина
			midStart := fadeTotal
			midEnd := len(curF)
			if i < len(files)-1 && len(curF) >= 2*fadeTotal {
				midEnd = len(curF) - fadeTotal
				prevTail = curF[len(curF)-fadeTotal:]
				havePrev = true
			} else {
				havePrev = false
			}
			if midEnd > midStart {
				writePCM16(toPCM16Scaled(curF, midStart, midEnd))
			}

			// хвост последнего файла
			if i == len(files)-1 && len(curF) >= fadeTotal {
				tail := curF[len(curF)-fadeTotal:]
				writePCM16(toPCM16Scaled(tail, 0, len(tail)))
			}
		} else {
			writePCM16(toPCM16Scaled(curF, 0, len(curF)))
			havePrev = false
		}
		U.PrintBar("PASS2 merge:", i+1, len(files))
	}
	U.EndBar()

	if written != int64(totalSamples) {
		U.LogWarn("written samples=%d, planned=%d (OK при очень коротких файлах/фейде)", written, totalSamples)
	}
	U.LogOK("Output saved: %s", outPath)
}

// ---------- утилиты и WAV I/O ----------

func fatal(U ui.UIAPI, err error) { U.LogErr("%v", err); os.Exit(1) }

func collectWavsRecursive(dir string) ([]fileInfo, error) {
	var out []fileInfo
	walkFn := func(path string, d os.DirEntry, err error) error {
		if err != nil { return err }
		if d.IsDir() { return nil }
		name := d.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".wav") { return nil }
		fi, err := d.Info()
		if err != nil { return err }
		out = append(out, fileInfo{Name: name, Path: path, ModTime: fi.ModTime()})
		return nil
	}
	if err := filepath.WalkDir(dir, walkFn); err != nil { return nil, err }
	return out, nil
}

func ensureDir(dir string) error {
	if dir == "" { return nil }
	return os.MkdirAll(dir, 0755)
}

func nextAvailablePath(p string) (string, error) {
	dir := filepath.Dir(p)
	base := filepath.Base(p)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	for i := 1; i < 10000; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s_%d%s", name, i, ext))
		if _, err := os.Stat(cand); errors.Is(err, os.ErrNotExist) {
			return cand, nil
		}
	}
	return "", fmt.Errorf("не удалось подобрать свободное имя для %s", p)
}

func createWavWriter(path string, sampleRate uint32, channels uint16, totalSamples uint32) (*os.File, *bufio.Writer, error) {
	f, err := os.Create(path)
	if err != nil { return nil, nil, err }
	bw := bufio.NewWriter(f)

	dataSize := totalSamples * 2 // PCM16: 2 байта на сэмпл
	fmtSize := uint32(16)
	riffSize := uint32(4 + (8 + fmtSize) + (8 + dataSize))

	// RIFF/WAVE
	if _, err := bw.WriteString("RIFF"); err != nil { f.Close(); return nil, nil, err }
	if err := binary.Write(bw, binary.LittleEndian, riffSize); err != nil { f.Close(); return nil, nil, err }
	if _, err := bw.WriteString("WAVE"); err != nil { f.Close(); return nil, nil, err }

	// fmt
	if _, err := bw.WriteString("fmt "); err != nil { f.Close(); return nil, nil, err }
	if err := binary.Write(bw, binary.LittleEndian, fmtSize); err != nil { f.Close(); return nil, nil, err }
	var audioFormat uint16 = 1 // PCM
	byteRate := sampleRate * uint32(channels) * 2
	blockAlign := channels * 2
	var bitsPerSample uint16 = 16
	if err := binary.Write(bw, binary.LittleEndian, audioFormat); err != nil { f.Close(); return nil, nil, err }
	if err := binary.Write(bw, binary.LittleEndian, channels); err != nil { f.Close(); return nil, nil, err }
	if err := binary.Write(bw, binary.LittleEndian, sampleRate); err != nil { f.Close(); return nil, nil, err }
	if err := binary.Write(bw, binary.LittleEndian, byteRate); err != nil { f.Close(); return nil, nil, err }
	if err := binary.Write(bw, binary.LittleEndian, blockAlign); err != nil { f.Close(); return nil, nil, err }
	if err := binary.Write(bw, binary.LittleEndian, bitsPerSample); err != nil { f.Close(); return nil, nil, err }

	// data
	if _, err := bw.WriteString("data"); err != nil { f.Close(); return nil, nil, err }
	if err := binary.Write(bw, binary.LittleEndian, dataSize); err != nil { f.Close(); return nil, nil, err }
	return f, bw, nil
}

func readWav(path string) (wavData, wavPCM, error) {
	f, err := os.Open(path)
	if err != nil { return wavData{}, wavPCM{}, err }
	defer f.Close()

	br := bufio.NewReader(f)

	// RIFF
	var riff [4]byte
	if _, err := io.ReadFull(br, riff[:]); err != nil { return wavData{}, wavPCM{}, err }
	if string(riff[:]) != "RIFF" { return wavData{}, wavPCM{}, errors.New("не RIFF") }
	// Skip ChunkSize
	if _, err := br.Discard(4); err != nil { return wavData{}, wavPCM{}, err }
	var wave [4]byte
	if _, err := io.ReadFull(br, wave[:]); err != nil { return wavData{}, wavPCM{}, err }
	if string(wave[:]) != "WAVE" { return wavData{}, wavPCM{}, errors.New("не WAVE") }

	var pcm wavPCM
	var data []int16

	for {
		var id [4]byte
		if _, err := io.ReadFull(br, id[:]); err != nil {
			if errors.Is(err, io.EOF) { break }
			return wavData{}, wavPCM{}, err
		}
		var size uint32
		if err := binary.Read(br, binary.LittleEndian, &size); err != nil { return wavData{}, wavPCM{}, err }

		switch string(id[:]) {
		case "fmt ":
			buf := make([]byte, size)
			if _, err := io.ReadFull(br, buf); err != nil { return wavData{}, wavPCM{}, err }
			b := bytes.NewReader(buf)
			if err := binary.Read(b, binary.LittleEndian, &pcm.AudioFormat); err != nil { return wavData{}, wavPCM{}, err }
			if err := binary.Read(b, binary.LittleEndian, &pcm.NumChannels); err != nil { return wavData{}, wavPCM{}, err }
			if err := binary.Read(b, binary.LittleEndian, &pcm.SampleRate); err != nil { return wavData{}, wavPCM{}, err }
			if err := binary.Read(b, binary.LittleEndian, &pcm.ByteRate); err != nil { return wavData{}, wavPCM{}, err }
			if err := binary.Read(b, binary.LittleEndian, &pcm.BlockAlign); err != nil { return wavData{}, wavPCM{}, err }
			if err := binary.Read(b, binary.LittleEndian, &pcm.BitsPerSample); err != nil { return wavData{}, wavPCM{}, err }
		case "data":
			if pcm.AudioFormat == 0 { return wavData{}, wavPCM{}, errors.New("встретили data до fmt") }
			if pcm.BitsPerSample != 16 { return wavData{}, wavPCM{}, fmt.Errorf("поддерживается только 16 бит, найдено: %d", pcm.BitsPerSample) }
			frames := int(size / 2)
			data = make([]int16, frames)
			if err := binary.Read(br, binary.LittleEndian, &data); err != nil { return wavData{}, wavPCM{}, err }
		default:
			if _, err := br.Discard(int(size)); err != nil { return wavData{}, wavPCM{}, err }
		}
		// выравнивание
		if size%2 == 1 {
			if _, err := br.Discard(1); err != nil { return wavData{}, wavPCM{}, err }
		}
	}

	if len(data) == 0 { return wavData{}, wavPCM{}, errors.New("нет аудио-данных (data chunk)") }
	return wavData{PCM: pcm, Data: data}, pcm, nil
}

// DSP утилиты
func updatePeakWhole(peak *float64, pcm16 []int16, gain float32) {
	const s = 1.0 / 32768.0
	for _, v := range pcm16 {
		f := float64(float32(v) * float32(s) * gain)
		av := math.Abs(f)
		if av > *peak { *peak = av }
	}
}
func updatePeakCrossfade(peak *float64, prevTail, curHead []float32) {
	n := len(prevTail)
	if len(curHead) < n { n = len(curHead) }
	if n == 0 { return }
	for i := 0; i < n; i++ {
		alpha := float64(i) / float64(n)
		mix := (1.0-alpha)*float64(prevTail[i]) + alpha*float64(curHead[i])
		av := math.Abs(mix)
		if av > *peak { *peak = av }
	}
}
func takeTailAsFloat(pcm16 []int16, gain float32, fadeTotal int) []float32 {
	if fadeTotal <= 0 || len(pcm16) < fadeTotal { return nil }
	const s = 1.0 / 32768.0
	out := make([]float32, fadeTotal)
	start := len(pcm16) - fadeTotal
	for i := 0; i < fadeTotal; i++ {
		out[i] = float32(pcm16[start+i]) * float32(s) * gain
	}
	return out
}
func takeHeadAsFloat(pcm16 []int16, gain float32, fadeTotal int) []float32 {
	if fadeTotal <= 0 || len(pcm16) < fadeTotal { return nil }
	const s = 1.0 / 32768.0
	out := make([]float32, fadeTotal)
	for i := 0; i < fadeTotal; i++ {
		out[i] = float32(pcm16[i]) * float32(s) * gain
	}
	return out
}
