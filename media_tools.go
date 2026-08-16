package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/thesyncim/goh264"
	xdraw "golang.org/x/image/draw"
	fontdraw "golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type splitTrack struct {
	in    *mp4.TrakBox
	outID uint32
	first uint32
	last  uint32
}

func splitMP4(path string, seconds float64) (SplitResult, error) {
	if !isISOBaseMedia(path) {
		return SplitResult{}, fmt.Errorf("split şu anda MP4/MOV dosyalarını destekliyor")
	}
	f, parsed, err := decodeMP4(path)
	if err != nil {
		return SplitResult{}, err
	}
	defer f.Close()
	if parsed.IsFragmented() || parsed.Moov == nil {
		return SplitResult{}, fmt.Errorf("yalnızca progressive MP4 destekleniyor")
	}
	duration := float64(parsed.Moov.Mvhd.Duration) / float64(parsed.Moov.Mvhd.Timescale)
	if seconds <= 0.05 || seconds >= duration-0.05 {
		return SplitResult{}, fmt.Errorf("bölme noktası videonun içinde olmalı")
	}
	actual, err := nextVideoSyncTime(parsed, seconds)
	if err != nil {
		return SplitResult{}, err
	}
	if actual <= 0.05 || actual >= duration-0.05 {
		return SplitResult{}, fmt.Errorf("bu noktaya yakın kullanılabilir bir anahtar kare yok")
	}
	base := strings.TrimSuffix(path, filepath.Ext(path))
	ext := ".mp4"
	first := availableOutput(base+"_part1", ext)
	second := availableOutput(base+"_part2", ext)
	if err := writeMP4Part(f, parsed, first, 0, actual); err != nil {
		return SplitResult{}, err
	}
	if err := writeMP4Part(f, parsed, second, actual, duration); err != nil {
		_ = os.Remove(first)
		return SplitResult{}, err
	}
	return SplitResult{FirstPath: first, SecondPath: second, SplitTime: actual}, nil
}

func nextVideoSyncTime(parsed *mp4.File, seconds float64) (float64, error) {
	for _, track := range parsed.Moov.Traks {
		if track.Mdia.Hdlr.HandlerType != "vide" {
			continue
		}
		stbl := track.Mdia.Minf.Stbl
		scale := track.Mdia.Mdhd.Timescale
		nr, err := stbl.Stts.GetSampleNrAtTime(uint64(seconds * float64(scale)))
		if err != nil {
			return 0, err
		}
		if stbl.Stss != nil {
			original := nr
			for nr <= track.GetNrSamples() && !stbl.Stss.IsSyncSample(nr) {
				nr++
			}
			if nr > track.GetNrSamples() {
				nr = original
				for nr > 1 && !stbl.Stss.IsSyncSample(nr) {
					nr--
				}
			}
		}
		if nr > track.GetNrSamples() {
			return 0, fmt.Errorf("bölme noktasından sonra anahtar kare yok")
		}
		t, _ := stbl.Stts.GetDecodeTime(nr)
		return float64(t) / float64(scale), nil
	}
	return seconds, nil
}

func writeMP4Part(reader *os.File, source *mp4.File, output string, start, end float64) error {
	init := mp4.CreateEmptyInit()
	tracks := make([]splitTrack, 0, len(source.Moov.Traks))
	for _, in := range source.Moov.Traks {
		kind := in.Mdia.Hdlr.HandlerType
		if kind != "vide" && kind != "soun" {
			continue
		}
		mediaType := map[string]string{"vide": "video", "soun": "audio"}[kind]
		out := init.AddEmptyTrack(in.Mdia.Mdhd.Timescale, mediaType, in.Mdia.Mdhd.GetLanguage())
		if err := copySampleDescription(in.Mdia.Minf.Stbl.Stsd, out.Mdia.Minf.Stbl.Stsd); err != nil {
			return err
		}
		tracks = append(tracks, splitTrack{in: in, outID: out.Tkhd.TrackID})
	}
	ids := make([]uint32, len(tracks))
	for i := range tracks {
		ids[i] = tracks[i].outID
	}
	frag, err := mp4.CreateMultiTrackFragment(1, ids)
	if err != nil {
		return err
	}
	for i := range tracks {
		track := &tracks[i]
		stbl := track.in.Mdia.Minf.Stbl
		scale := track.in.Mdia.Mdhd.Timescale
		first, err := stbl.Stts.GetSampleNrAtTime(uint64(start * float64(scale)))
		if err != nil && start > 0 {
			return err
		}
		if first < 1 {
			first = 1
		}
		last, err := stbl.Stts.GetSampleNrAtTime(uint64(end * float64(scale)))
		if err != nil {
			last = track.in.GetNrSamples() + 1
		}
		if last > 1 {
			last--
		}
		if last > track.in.GetNrSamples() {
			last = track.in.GetNrSamples()
		}
		if first > last {
			continue
		}
		track.first, track.last = first, last
		for n := first; n <= last; n++ {
			sample := sampleMetadata(stbl, n)
			if err := frag.AddSampleToTrack(sample, track.outID, 0); err != nil {
				return err
			}
		}
	}
	segment := mp4.NewMediaSegment()
	segment.AddFragment(frag)
	out, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			_ = os.Remove(output)
		}
	}()
	if err := init.Encode(out); err != nil {
		return err
	}
	if err := segment.Encode(out); err != nil {
		return err
	}
	for _, track := range tracks {
		for n := track.first; n > 0 && n <= track.last; n++ {
			if err := copySampleBytes(reader, out, track.in, n); err != nil {
				return err
			}
		}
	}
	ok = true
	return out.Close()
}

func copySampleDescription(in, out *mp4.StsdBox) error {
	switch {
	case in.AvcX != nil:
		out.AddChild(in.AvcX)
	case in.HvcX != nil:
		out.AddChild(in.HvcX)
	case in.Av01 != nil:
		out.AddChild(in.Av01)
	case in.Mp4a != nil:
		out.AddChild(in.Mp4a)
	case in.AC3 != nil:
		out.AddChild(in.AC3)
	case in.EC3 != nil:
		out.AddChild(in.EC3)
	case in.Opus != nil:
		out.AddChild(in.Opus)
	default:
		return fmt.Errorf("desteklenmeyen MP4 codec")
	}
	return nil
}

func copySampleBytes(reader io.ReadSeeker, writer io.Writer, track *mp4.TrakBox, n uint32) error {
	stbl := track.Mdia.Minf.Stbl
	chunk, chunkStart, err := stbl.Stsc.ChunkNrFromSampleNr(int(n))
	if err != nil {
		return err
	}
	var offset uint64
	if stbl.Stco != nil {
		offset = uint64(stbl.Stco.ChunkOffset[chunk-1])
	} else {
		offset = stbl.Co64.ChunkOffset[chunk-1]
	}
	for sample := chunkStart; sample < int(n); sample++ {
		offset += uint64(stbl.Stsz.GetSampleSize(sample))
	}
	size := stbl.Stsz.GetSampleSize(int(n))
	if _, err := reader.Seek(int64(offset), io.SeekStart); err != nil {
		return err
	}
	_, err = io.CopyN(writer, reader, int64(size))
	return err
}

func sampleMetadata(stbl *mp4.StblBox, n uint32) mp4.Sample {
	size := stbl.Stsz.GetSampleSize(int(n))
	_, dur := stbl.Stts.GetDecodeTime(n)
	cto := int32(0)
	if stbl.Ctts != nil {
		cto = stbl.Ctts.GetCompositionTimeOffset(n)
	}
	return mp4.Sample{Flags: sampleFlags(stbl, n), Dur: dur, Size: size, CompositionTimeOffset: cto}
}

func sampleFlags(stbl *mp4.StblBox, n uint32) uint32 {
	var f mp4.SampleFlags
	if stbl.Stss != nil {
		sync := stbl.Stss.IsSyncSample(n)
		f.SampleIsNonSync = !sync
		if sync {
			f.SampleDependsOn = 2
		}
	}
	return f.Encode()
}

func extractContactSheet(path string, count, cellW, columns int) (string, error) {
	info, err := probeMediaPureGo(path)
	if err != nil {
		return "", err
	}
	if info.Duration <= 0 {
		return "", fmt.Errorf("video süresi okunamadı")
	}
	if count < 1 || count > 60 {
		return "", fmt.Errorf("kare sayısı 1 ile 60 arasında olmalı")
	}
	if cellW < 160 || cellW > 640 {
		return "", fmt.Errorf("görsel genişliği 160 ile 640 piksel arasında olmalı")
	}
	cellH := cellW * 9 / 16
	rows := (count + columns - 1) / columns
	sheet := image.NewRGBA(image.Rect(0, 0, cellW*columns, cellH*rows))
	draw.Draw(sheet, sheet.Bounds(), &image.Uniform{color.RGBA{10, 11, 13, 255}}, image.Point{}, draw.Src)
	for i := 0; i < count; i++ {
		timestamp := info.Duration * float64(i+1) / float64(count+1)
		frame, err := decodeH264FrameAt(path, timestamp)
		if err != nil {
			return "", fmt.Errorf("%.1f saniyedeki kare: %w", timestamp, err)
		}
		img, err := frameImage(frame, cellW, cellH)
		if err != nil {
			return "", err
		}
		x, y := (i%columns)*cellW, (i/columns)*cellH
		draw.Draw(sheet, image.Rect(x, y, x+cellW, y+cellH), img, image.Point{}, draw.Src)
		drawTimestamp(sheet, x, y, timestamp)
	}
	output := availableOutput(strings.TrimSuffix(path, filepath.Ext(path))+"_contact-sheet", ".jpg")
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := jpeg.Encode(f, sheet, &jpeg.Options{Quality: 95}); err != nil {
		return "", err
	}
	return output, nil
}

func drawTimestamp(img *image.RGBA, x, y int, seconds float64) {
	total := int(seconds + .5)
	label := fmt.Sprintf("%02d:%02d:%02d", total/3600, (total%3600)/60, total%60)
	padding := 5
	width := len(label)*7 + padding*2
	height := 13 + padding*2
	draw.Draw(img, image.Rect(x, y, x+width, y+height), &image.Uniform{color.RGBA{0, 0, 0, 190}}, image.Point{}, draw.Over)
	d := &fontdraw.Drawer{Dst: img, Src: &image.Uniform{color.White}, Face: basicfont.Face7x13, Dot: fixed.P(x+padding, y+padding+13)}
	d.DrawString(label)
}

func availableOutput(base, ext string) string {
	candidate := base + ext
	for i := 2; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s_%d%s", base, i, ext)
	}
}

func decodeH264FrameAt(path string, seconds float64) (*goh264.Frame, error) {
	f, parsed, err := decodeMP4(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var track *mp4.TrakBox
	for _, t := range parsed.Moov.Traks {
		if t.Mdia.Hdlr.HandlerType == "vide" {
			track = t
			break
		}
	}
	if track == nil {
		return nil, fmt.Errorf("video izi yok")
	}
	stbl := track.Mdia.Minf.Stbl
	if stbl.Stsd.AvcX == nil || stbl.Stsd.AvcX.AvcC == nil {
		return nil, fmt.Errorf("contact sheet H.264 gerektiriyor")
	}
	target, err := stbl.Stts.GetSampleNrAtTime(uint64(seconds * float64(track.Mdia.Mdhd.Timescale)))
	if err != nil {
		return nil, err
	}
	start := target
	if stbl.Stss != nil {
		for start > 1 && !stbl.Stss.IsSyncSample(start) {
			start--
		}
	}
	var configBuffer bytes.Buffer
	if err := stbl.Stsd.AvcX.AvcC.DecConfRec.Encode(&configBuffer); err != nil {
		return nil, err
	}
	decoder := goh264.NewDecoder()
	if _, err := decoder.ConfigureAVCC(configBuffer.Bytes()); err != nil {
		return nil, err
	}
	workspace := make([]byte, 2*1024*1024)
	for n := start; n <= target+8 && n <= track.GetNrSamples(); n++ {
		var packet bytes.Buffer
		if err := parsed.CopySampleData(&packet, f, track, n, n, workspace); err != nil {
			return nil, err
		}
		frames, e := decoder.DecodeConfiguredAVCFrames(packet.Bytes())
		if e == nil && len(frames) > 0 && n >= target {
			return frames[len(frames)-1], nil
		}
	}
	frames, err := decoder.DecodeConfiguredAVCFrames(nil)
	if err == nil && len(frames) > 0 {
		return frames[len(frames)-1], nil
	}
	return nil, fmt.Errorf("kare çözülemedi")
}

func frameImage(frame *goh264.Frame, outW, outH int) (image.Image, error) {
	if frame.BitDepthLuma != 8 {
		return nil, fmt.Errorf("yalnızca 8-bit kare destekleniyor")
	}
	w, h := frame.Width, frame.Height
	source := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			planeX, planeY := x+frame.CropLeft, y+frame.CropTop
			ci := (planeY/2)*frame.CStride + planeX/2
			cb, cr := uint8(128), uint8(128)
			if ci < len(frame.Cb) && ci < len(frame.Cr) {
				cb, cr = frame.Cb[ci], frame.Cr[ci]
			}
			r, g, b := color.YCbCrToRGB(frame.Y[planeY*frame.YStride+planeX], cb, cr)
			source.SetRGBA(x, y, color.RGBA{r, g, b, 255})
		}
	}
	destination := image.NewRGBA(image.Rect(0, 0, outW, outH))
	xdraw.CatmullRom.Scale(destination, destination.Bounds(), source, source.Bounds(), draw.Src, nil)
	return destination, nil
}
