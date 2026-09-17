package stream

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// pipeToHub reads ffmpeg's fragmented-MP4 stdout, splits off the leading
// ftyp+moov initialization segment, and broadcasts everything through h:
// the init segment first (so it's cached for viewers that join later), then
// each subsequent chunk of moof+mdat fragment data as it arrives.
func pipeToHub(h *hub, r io.Reader) error {
	br := bufio.NewReaderSize(r, 64*1024)

	initSeg, err := readInitSegment(br)
	if err != nil {
		return fmt.Errorf("reading init segment: %w", err)
	}
	h.setInitSegment(initSeg)
	h.broadcast(initSeg)

	buf := make([]byte, 32*1024)
	for {
		n, err := br.Read(buf)
		if n > 0 {
			h.broadcast(buf[:n])
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// readInitSegment reads sequential top-level MP4 boxes from r until it has
// fully consumed a "moov" box and returns the exact bytes read. With
// -movflags empty_moov, ffmpeg's fmp4 muxer emits "ftyp" then "moov" exactly
// once at the very start, before any "moof"/"mdat" fragments, so this
// reliably captures just the initialization segment regardless of any
// leading boxes' sizes.
func readInitSegment(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	for {
		var header [8]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return nil, err
		}
		size := binary.BigEndian.Uint32(header[0:4])
		boxType := string(header[4:8])
		if size < 8 {
			return nil, fmt.Errorf("unexpected mp4 box size %d for %q", size, boxType)
		}

		buf.Write(header[:])
		if _, err := io.CopyN(&buf, r, int64(size)-8); err != nil {
			return nil, err
		}

		if boxType == "moov" {
			return buf.Bytes(), nil
		}
	}
}
