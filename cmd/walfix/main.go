// Command walfix repairs a rosedblabs/wal segment file whose tail was
// truncated by a machine crash.
//
// The WAL stores its records as a sequence of chunks inside 32 KB blocks.
// Every chunk starts with a 7 byte header:
//
//	0..3  crc32 checksum of header[4:] + payload
//	4..5  payload length (little endian uint16)
//	6     chunk type
//
// A crash in the middle of a write leaves a half written chunk at the end of
// the last segment. The reader validates the checksum of every chunk and
// fails with "invalid crc, the data may be corrupted" on the first bad one.
//
// The repair truncates the segment at the last valid chunk boundary. The
// data before that point is intact and is preserved.
//
// It can also repair a WAL whose segment files were removed by mistake: the
// tool recreates the missing segment so that the WAL has at least two
// segments again, which the rosedb merge path requires.
package main

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	chunkHeaderSize = 7
	blockSize       = 32 * 1024
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: walfix <consensusDir> [--apply]")
		os.Exit(1)
	}
	dir := os.Args[1]
	apply := false
	for _, a := range os.Args[2:] {
		if a == "--apply" {
			apply = true
		}
	}

	segs, err := segmentFiles(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(segs) == 0 {
		fmt.Println("no segment files found; nothing to do")
		return
	}

	fmt.Println("--- walfix ---")
	fmt.Printf("dir: %s\n", dir)
	for _, s := range segs {
		fmt.Printf("  segment %s\n", filepath.Base(s))
	}

	last := segs[len(segs)-1]
	good, err := scanSegment(last)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	size := fileSize(last)
	fmt.Printf("\nlast segment: %s\n", filepath.Base(last))
	fmt.Printf("  size:            %d bytes\n", size)
	fmt.Printf("  valid prefix:    %d bytes (%d blocks)\n", good, good/blockSize)
	if good == size {
		fmt.Println("  the whole segment is valid; no repair needed")
	} else {
		fmt.Printf("  ! %d bytes of a partially written chunk at the tail\n", size-good)
	}

	if !apply {
		fmt.Println("\n(dry run; pass --apply to write)")
		return
	}

	if good != size {
		if err := os.Truncate(last, int64(good)); err != nil {
			fmt.Fprintf(os.Stderr, "FAILED to truncate %s: %v\n", last, err)
			os.Exit(1)
		}
		fmt.Printf("  truncated %s to %d bytes\n", filepath.Base(last), good)
	}

	// The rosedb merge path walks the segment readers and skips the ones
	// that have already been merged. The wal Reader.CurrentSegmentId has
	// no bounds check, so when the WAL ends up with a single segment the
	// skip loop runs past the end of the slice and the node panics with
	// "index out of range".
	//
	// Recreating the missing segment keeps the WAL shape the merge code
	// expects: the merged segment plus a live segment.
	if len(segs) < 2 {
		fmt.Println("  ! only one segment left; the merge path needs at least two")
		newSeg := filepath.Join(dir, fmt.Sprintf("%09d.SEG", segmentID(segs[0])+1))
		if _, err := os.Stat(newSeg); err == nil {
			fmt.Printf("  segment %s already exists\n", filepath.Base(newSeg))
			return
		}
		f, err := os.OpenFile(newSeg, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAILED to create %s: %v\n", newSeg, err)
			os.Exit(1)
		}
		f.Close()
		fmt.Printf("  created empty segment %s\n", filepath.Base(newSeg))
	}

	fmt.Println("\nDONE. Restart the node.")
}

// segmentFiles returns the segment files of a WAL directory, sorted by id.
func segmentFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", dir, err)
	}
	var segs []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".SEG") {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSuffix(name, ".SEG"))
		if err != nil {
			continue
		}
		segs = append(segs, filepath.Join(dir, name))
		_ = id
	}
	sort.Slice(segs, func(i, j int) bool {
		return segmentID(segs[i]) < segmentID(segs[j])
	})
	return segs, nil
}

// segmentID extracts the numeric id from a segment file name.
func segmentID(path string) int {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".SEG")
	id, err := strconv.Atoi(base)
	if err != nil {
		return 0
	}
	return id
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// scanSegment walks the chunks of a segment and returns the offset of the
// first invalid one. When every chunk is valid it returns the file size.
//
// The walk stops at the first chunk whose checksum does not match, which is
// exactly the chunk the crash left half written.
func scanSegment(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("cannot read %s: %w", path, err)
	}

	offset := int64(0)
	for offset < int64(len(data)) {
		// A chunk never crosses a block boundary.
		blockRemain := int64(blockSize) - (offset % blockSize)
		if blockRemain < chunkHeaderSize {
			// The rest of the block is padding.
			offset += blockRemain
			continue
		}
		if offset+chunkHeaderSize > int64(len(data)) {
			break
		}

		hdr := data[offset : offset+chunkHeaderSize]
		length := int64(binary.LittleEndian.Uint16(hdr[4:6]))
		end := offset + chunkHeaderSize + length
		if end > int64(len(data)) {
			// The payload was not fully written.
			break
		}

		checksum := crc32.ChecksumIEEE(hdr[4:])
		checksum = crc32.Update(checksum, crc32.IEEETable, data[offset+chunkHeaderSize:end])
		saved := binary.LittleEndian.Uint32(hdr[:4])
		if saved != checksum {
			break
		}

		offset = end
	}

	return offset, nil
}
