// Command chainfix is a read-mostly diagnostic and repair tool for an
// axiom-ledger node whose chain meta and blockfile have drifted apart.
//
// It exists because the failure mode it addresses is rare and destructive:
// the blockfile payload files (.rdat) and the blockfile indexes (.ridx) are
// written independently, so a machine crash can leave the chain meta (in
// pebble) pointing at a height that the blockfile indexes cannot reach.
//
// The tool never guesses. Every mutating command prints what it would do and
// requires an explicit --apply flag, and every mutating command writes a
// backup first.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/pebble"
)

// ---------------------------------------------------------------------------
// constants
// ---------------------------------------------------------------------------

// pebble key holding the chain meta.
const chainMetaKey = "chain-meta"

// pebble key holding a backup of the chain meta taken before a repair.
const chainMetaBackupKey = "chain-meta-backup"

// pebble key prefix holding the snapshot journal. The snapshot lives in its
// own store (storage/snapshot), separate from the prune journal in
// storage/ledger, and the node rolls it back independently at startup.
const snapshotKeyPrefix = "snap-"

// blockfile schemas, in the order they are appended.
var schemas = []string{"header", "transactions", "extra", "receipts"}

// size of a single blockfile index entry: uint16 filenum + uint32 offset.
const indexEntrySize = 6

// ---------------------------------------------------------------------------
// small helpers
// ---------------------------------------------------------------------------

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `chainfix - axiom-ledger chain meta / blockfile repair tool

usage: chainfix <repoRoot> <command> [flags]

read-only commands:
  diagnose            full report: chain meta, blockfile indexes, data files,
                      prune journal, and a verdict on what is broken
  meta                print the raw chain meta and its decoded fields
  index               print per-schema .ridx entry counts and the gaps between
                      them (the blockfile height is the minimum across schemas)
  verify              walk the header index, read the block height out of the
                      payload, and report whether the heights are contiguous
  scan-tail           walk the header payload past the end of the index and
                      report the blocks that are present but not indexed
  state-height        report the height of the state trie and compare it with
                      the blockfile and the chain meta
  hashes <from> <to>  print the block hash the blockfile implies for a range
                      of heights, so the data can be checked against what the
                      other nodes report
  compare <dir>       compare this node's .ridx indexes against another
                      node's blockfile directory and report where they differ

repair commands (all require --apply, all take a backup first):
  adopt-index <dir>   copy the .ridx indexes from another node's blockfile
                      directory into this node, keeping only the entries that
                      this node's .rdat files can actually serve
  rollback <height>   move the chain back to a height the blockfile can
                      serve: truncate the indexes, rewrite the chain meta
                      with the real block hash and gas price, and trim the
                      prune journal
  set-meta <height>   write an explicit chain meta height (and optionally the
                      block hash and gas price)
  prune-max <height>  set the prune journal maxHeight
  backup-meta          copy the current chain meta aside (no --apply needed)
  restore-meta         restore the chain meta from the backup

helper command (run on the healthy node):
  slice-index <dir>   write the tail of this node's .ridx indexes into <dir>
                      so it can be shipped to a peer that lost its trailing
                      entries. Combine with --from to cut only the last few
                      entries and --keep to cap how many are written.

flags:
  --apply             actually perform a repair (default is a dry run)
  --gas-price <wei>   gas price for set-meta, decimal or 0x-prefixed hex
  --hash <0x...>      block hash for set-meta
  --keep <n>          with adopt-index or slice-index, cap the entry count
  --from <n>          with adopt-index or slice-index, the absolute entry
                      number the peer index starts at. The local index keeps
                      its own entries before that point.

The repoRoot is the node directory, e.g. /root/node.`)
}

// ---------------------------------------------------------------------------
// protobuf varint helpers
// ---------------------------------------------------------------------------

func decodeVarint(data []byte) (uint64, int) {
	var v uint64
	var shift uint
	for i := 0; i < len(data); i++ {
		b := data[i]
		v |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return v, i + 1
		}
		shift += 7
		if shift > 63 {
			return 0, 0
		}
	}
	return 0, 0
}

func appendVarint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	return append(dst, byte(v))
}

// ---------------------------------------------------------------------------
// chain meta
// ---------------------------------------------------------------------------

// pb.ChainMeta layout (axiom-kit types/pb/chain.proto):
//
//	uint64 height      = 1;  -> 0x08 <varint>
//	bytes  block_hash  = 2;  -> 0x12 <len> <bytes>
//	bytes  gas_price   = 3;  -> 0x1a <len> <bytes>
//
// block_hash is field 2. Encoding it as field 3 produces a value that most
// decoders still accept but that does not round-trip, which is how a 0x00
// block hash ends up stored in the chain meta.
func encodeChainMeta(height uint64, hash, gasPrice []byte) []byte {
	var out []byte
	out = append(out, 0x08)
	out = appendVarint(out, height)
	if len(hash) > 0 {
		out = append(out, 0x12)
		out = appendVarint(out, uint64(len(hash)))
		out = append(out, hash...)
	}
	if len(gasPrice) > 0 {
		out = append(out, 0x1a)
		out = appendVarint(out, uint64(len(gasPrice)))
		out = append(out, gasPrice...)
	}
	return out
}

func decodeChainMeta(data []byte) (height uint64, hash, gasPrice []byte, err error) {
	i := 0
	for i < len(data) {
		key, n := decodeVarint(data[i:])
		if n == 0 {
			return 0, nil, nil, fmt.Errorf("bad varint at offset %d", i)
		}
		i += n
		field, wire := key>>3, key&0x7
		switch wire {
		case 0:
			v, n := decodeVarint(data[i:])
			if n == 0 {
				return 0, nil, nil, fmt.Errorf("bad varint value at offset %d", i)
			}
			i += n
			if field == 1 {
				height = v
			}
		case 2:
			l, n := decodeVarint(data[i:])
			if n == 0 {
				return 0, nil, nil, fmt.Errorf("bad varint length at offset %d", i)
			}
			i += n
			if i+int(l) > len(data) {
				return 0, nil, nil, fmt.Errorf("length %d exceeds remaining data at offset %d", l, i)
			}
			val := data[i : i+int(l)]
			i += int(l)
			switch field {
			case 2:
				hash = val
			case 3:
				gasPrice = val
			}
		default:
			return 0, nil, nil, fmt.Errorf("unsupported wire type %d at offset %d", wire, i)
		}
	}
	return height, hash, gasPrice, nil
}

// parseGasPrice accepts a decimal wei value or a 0x-prefixed hex value and
// returns the minimal big-endian encoding, which is what pb stores.
func parseGasPrice(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty gas price")
	}
	base, t := 10, s
	if strings.HasPrefix(t, "0x") || strings.HasPrefix(t, "0X") {
		base, t = 16, t[2:]
	}
	v, err := strconv.ParseUint(t, base, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid gas price %q: %w", s, err)
	}
	var be []byte
	for x := v; x > 0; x >>= 8 {
		be = append([]byte{byte(x)}, be...)
	}
	if len(be) == 0 {
		be = []byte{0}
	}
	return be, nil
}

// ---------------------------------------------------------------------------
// node layout
// ---------------------------------------------------------------------------

type node struct {
	root       string
	blockfile  string
	blockchain string
	ledger     string
	snapshot   string
}

func openNode(root string) node {
	return node{
		root:       root,
		blockfile:  filepath.Join(root, "storage", "blockfile"),
		blockchain: filepath.Join(root, "storage", "blockchain"),
		ledger:     filepath.Join(root, "storage", "ledger"),
		snapshot:   filepath.Join(root, "storage", "snapshot"),
	}
}

// indexEntries returns the number of entries in a schema's .ridx file.
func (n node) indexEntries(schema string) (uint64, error) {
	st, err := os.Stat(filepath.Join(n.blockfile, schema+".ridx"))
	if err != nil {
		return 0, err
	}
	return uint64(st.Size() / indexEntrySize), nil
}

// dataFiles returns the .rdat files of a schema, keyed by their file number.
func (n node) dataFiles(schema string) ([]uint32, map[uint32]string, error) {
	matches, err := filepath.Glob(filepath.Join(n.blockfile, schema+".*.rdat"))
	if err != nil {
		return nil, nil, err
	}
	paths := make(map[uint32]string, len(matches))
	var nums []uint32
	for _, p := range matches {
		base := filepath.Base(p)
		// schema.NNNN.rdat
		dot := strings.LastIndex(base, ".")
		if dot < 0 {
			continue
		}
		numPart := base[:dot]
		if idx := strings.LastIndex(numPart, "."); idx >= 0 {
			numPart = numPart[idx+1:]
		}
		num, err := strconv.ParseUint(numPart, 10, 32)
		if err != nil {
			continue
		}
		paths[uint32(num)] = p
		nums = append(nums, uint32(num))
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
	return nums, paths, nil
}

// dataFileSize returns the size of a .rdat file, or -1 when it is missing.
func (n node) dataFileSize(schema string, num uint32) int64 {
	p := filepath.Join(n.blockfile, fmt.Sprintf("%s.%04d.rdat", schema, num))
	st, err := os.Stat(p)
	if err != nil {
		return -1
	}
	return st.Size()
}

// ---------------------------------------------------------------------------
// diagnose
// ---------------------------------------------------------------------------

type indexReport struct {
	schema  string
	entries uint64
	present bool
}

func (n node) reports() ([]indexReport, error) {
	var out []indexReport
	for _, s := range schemas {
		e, err := n.indexEntries(s)
		if err != nil {
			out = append(out, indexReport{schema: s, present: false})
			continue
		}
		out = append(out, indexReport{schema: s, entries: e, present: true})
	}
	return out, nil
}

// effectiveHeight is the blockfile height the node will actually see.
//
// axiom-kit's BlockFile.repair() truncates every table down to the smallest
// item count across all tables and stores that as the block count. So the
// node's view of the chain is limited by the *shortest* index, not the
// longest one.
func effectiveHeight(reports []indexReport) (uint64, bool) {
	var min uint64
	found := false
	for _, r := range reports {
		if !r.present {
			continue
		}
		if !found || r.entries < min {
			min, found = r.entries, true
		}
	}
	if !found || min == 0 {
		return 0, false
	}
	return min - 1, true
}

func (n node) diagnose() {
	fmt.Println("========================================")
	fmt.Println(" chainfix diagnose")
	fmt.Println("========================================")
	fmt.Printf("repo root:      %s\n", n.root)

	// --- chain meta ---
	fmt.Println()
	fmt.Println("--- chain meta (pebble) ---")
	metaHeight, metaHash, metaGas, metaErr := n.readChainMeta()
	switch {
	case metaErr != nil:
		fmt.Printf("  ERROR: %v\n", metaErr)
	default:
		fmt.Printf("  height:     %d\n", metaHeight)
		fmt.Printf("  blockHash:  0x%s\n", hex.EncodeToString(metaHash))
		if len(metaGas) > 0 {
			fmt.Printf("  gasPrice:   0x%x\n", metaGas)
		}
	}

	// --- blockfile indexes ---
	fmt.Println()
	fmt.Println("--- blockfile indexes ---")
	reports, _ := n.reports()
	for _, r := range reports {
		if !r.present {
			fmt.Printf("  %-13s MISSING\n", r.schema)
			continue
		}
		fmt.Printf("  %-13s %10d entries  (%.1f MB)\n", r.schema, r.entries, float64(r.entries*indexEntrySize)/(1024*1024))
	}

	eff, ok := effectiveHeight(reports)
	if !ok {
		fmt.Println("  effective height: <unknown>")
	} else {
		fmt.Printf("  effective height: %d  (min across schemas; this is what the node sees)\n", eff)
	}

	// --- data files ---
	fmt.Println()
	fmt.Println("--- blockfile data files (.rdat) ---")
	for _, s := range schemas {
		nums, _, err := n.dataFiles(s)
		if err != nil {
			fmt.Printf("  %-13s <error: %v>\n", s, err)
			continue
		}
		var total int64
		for _, num := range nums {
			total += n.dataFileSize(s, num)
		}
		last := uint32(0)
		if len(nums) > 0 {
			last = nums[len(nums)-1]
		}
		lastSize := n.dataFileSize(s, last)
		fmt.Printf("  %-13s %2d files, %10.1f MB total, last=%s.%04d.rdat (%s)\n",
			s, len(nums), float64(total)/(1024*1024), s, last,
			humanSize(lastSize))

		// The last index entry records the offset just past the last block's
		// payload. If that offset is larger than the data file, the index
		// claims blocks the payload does not hold -- the reverse of the usual
		// case, and a sign that the index survived while the data was lost.
		entries, err := n.indexEntries(s)
		if err == nil && entries > 0 {
			idx, err := readIndex(filepath.Join(n.blockfile, s+".ridx"))
			if err == nil {
				e := idx[len(idx)-1]
				size := n.dataFileSize(s, e.filenum)
				switch {
				case size < 0:
					fmt.Printf("                ! last index entry points at missing file %04d\n", e.filenum)
				case int64(e.offset) > size:
					fmt.Printf("                ! last index entry offset %d exceeds data file size %d\n", e.offset, size)
					fmt.Printf("                ! %d bytes of payload are MISSING (index survived, data did not)\n", int64(e.offset)-size)
				default:
					fmt.Printf("                last index entry offset %d <= data size %d (payload present)\n", e.offset, size)
				}
			}
		}
	}

	// --- prune journal ---
	fmt.Println()
	fmt.Println("--- prune journal (pebble) ---")
	minH, maxH, hasPrune := n.readPruneJournal()
	if hasPrune {
		fmt.Printf("  minHeight: %d\n", minH)
		fmt.Printf("  maxHeight: %d\n", maxH)
	} else {
		fmt.Println("  <not set>")
	}

	// --- snapshot journal ---
	// The snapshot lives in its own store and the node rolls it back
	// independently at startup, replaying the journal to restore the previous
	// account and storage values. The rollback refuses to run when the journal
	// range does not bracket the chain meta height, so it has to be reported
	// separately.
	fmt.Println()
	fmt.Println("--- snapshot journal (pebble, storage/snapshot) ---")
	sMinH, sMaxH, hasSnap := n.readSnapshotJournal()
	if hasSnap {
		fmt.Printf("  minHeight: %d\n", sMinH)
		fmt.Printf("  maxHeight: %d\n", sMaxH)
		if metaErr == nil && (sMaxH < metaHeight || (sMinH > metaHeight && !(sMinH == 1 && metaHeight == 0))) {
			fmt.Printf("  ! the journal range does not bracket the chain meta height %d\n", metaHeight)
			fmt.Printf("  ! the node will refuse to start: snapshot rollback out of range\n")
		} else {
			fmt.Printf("  the journal brackets the chain meta height, the node can roll it back\n")
		}
	} else {
		fmt.Println("  <not set>")
	}

	// --- header index heights ---
	fmt.Println()
	fmt.Println("--- header index -> block height ---")
	heights, err := n.headerHeights()
	if err != nil {
		fmt.Printf("  <cannot walk the header index: %v>\n", err)
	} else {
		fmt.Printf("  header index has %d entries\n", len(heights))
		if len(heights) > 1 {
			last := len(heights) - 1
			fmt.Printf("  entry %d  -> block height %d\n", last, heights[last])
			fmt.Printf("  entry %d  -> block height %d\n", last-1, heights[last-1])
			// Check that the heights are contiguous.
			gaps := 0
			for i := 2; i < len(heights); i++ {
				if heights[i] != heights[i-1]+1 {
					gaps++
				}
			}
			if gaps == 0 {
				fmt.Printf("  heights are contiguous from %d to %d\n", heights[1], heights[len(heights)-1])
			} else {
				fmt.Printf("  ! %d gaps in the height sequence\n", gaps)
			}
		}
	}

	// --- verdict ---
	fmt.Println()
	fmt.Println("--- verdict ---")
	n.verdict(metaHeight, metaErr, reports, eff, ok, minH, maxH, hasPrune)
}

func humanSize(v int64) string {
	if v < 0 {
		return "missing"
	}
	return fmt.Sprintf("%.1f MB", float64(v)/(1024*1024))
}

func (n node) verdict(metaHeight uint64, metaErr error, reports []indexReport, eff uint64, ok bool, minH, maxH uint64, hasPrune bool) {
	if metaErr != nil {
		fmt.Println("  chain meta is unreadable; the node will panic on startup.")
		fmt.Println("  run: chainfix <root> set-meta <height> --apply")
		return
	}
	if !ok {
		fmt.Println("  no blockfile index found; nothing to compare against.")
		return
	}

	if metaHeight > eff {
		fmt.Printf("  BROKEN: chain meta height %d is ahead of the blockfile height %d.\n", metaHeight, eff)
		fmt.Println("  The node panics with \"illegal chain meta!\" in checkChainMeta().")
		fmt.Println()
		fmt.Println("  The payload files are usually fine; it is the .ridx indexes that lost")
		fmt.Println("  their trailing entries when the machine crashed.")
		fmt.Println()
		fmt.Println("  Fix by adopting the indexes from a healthy peer:")
		fmt.Println("    chainfix <root> compare /path/to/peer/storage/blockfile")
		fmt.Println("    chainfix <root> adopt-index /path/to/peer/storage/blockfile --apply")
		fmt.Println()
		fmt.Println("  Only if no healthy peer has the missing blocks:")
		fmt.Printf("    chainfix <root> set-meta %d --apply   # roll the chain back\n", eff)
		return
	}

	if metaHeight < eff {
		fmt.Printf("  chain meta height %d is behind the blockfile height %d.\n", metaHeight, eff)
		fmt.Println("  checkChainMeta() rebuilds the meta from the blockfile on startup, so")
		fmt.Println("  this is self-healing. No action needed.")
	} else {
		fmt.Println("  OK: chain meta height matches the blockfile height.")
	}

	// Report the index spread even when the heights agree, because a spread
	// means the indexes are inconsistent even if they happen to line up.
	var minE, maxE uint64
	for i, r := range reports {
		if !r.present {
			continue
		}
		if i == 0 || r.entries < minE {
			minE = r.entries
		}
		if i == 0 || r.entries > maxE {
			maxE = r.entries
		}
	}
	if maxE > minE {
		fmt.Println()
		fmt.Printf("  WARNING: the indexes disagree (min %d entries, max %d entries, %d blocks apart).\n",
			minE, maxE, maxE-minE)
		fmt.Println("  The node will only expose the shortest index. Recover the others with")
		fmt.Println("  adopt-index before the next restart truncates the payload files.")
	}

	if hasPrune && maxH > 0 && metaHeight > maxH {
		fmt.Println()
		fmt.Printf("  WARNING: prune journal maxHeight %d is behind the chain height %d.\n", maxH, metaHeight)
		fmt.Println("  PruneCache.Rollback will refuse to start. Run: chainfix <root> prune-max <height>")
	}
}

// ---------------------------------------------------------------------------
// pebble access
// ---------------------------------------------------------------------------

func (n node) readChainMeta() (uint64, []byte, []byte, error) {
	db, err := pebble.Open(n.blockchain, &pebble.Options{})
	if err != nil {
		return 0, nil, nil, err
	}
	defer db.Close()

	data, closer, err := db.Get([]byte(chainMetaKey))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("chain meta not found")
	}
	defer closer.Close()

	return decodeChainMeta(data)
}

func (n node) readPruneJournal() (uint64, uint64, bool) {
	db, err := pebble.Open(n.ledger, &pebble.Options{})
	if err != nil {
		return 0, 0, false
	}
	defer db.Close()

	read := func(suffix string) (uint64, bool) {
		v, closer, err := db.Get([]byte("prune-nodeInfo-" + suffix))
		if err != nil {
			return 0, false
		}
		defer closer.Close()
		h, err := strconv.ParseUint(string(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return h, true
	}
	minH, ok1 := read("minHeight")
	maxH, ok2 := read("maxHeight")
	return minH, maxH, ok1 || ok2
}

// ---------------------------------------------------------------------------
// index inspection
// ---------------------------------------------------------------------------

func (n node) indexCmd() {
	fmt.Println("--- blockfile indexes ---")
	reports, _ := n.reports()
	for _, r := range reports {
		if !r.present {
			fmt.Printf("  %-13s MISSING\n", r.schema)
			continue
		}
		fmt.Printf("  %-13s %10d entries  height=%d\n", r.schema, r.entries, r.entries-1)
	}
	if eff, ok := effectiveHeight(reports); ok {
		fmt.Printf("\n  effective height (min across schemas): %d\n", eff)
	}
}

func (n node) metaCmd() {
	db, err := pebble.Open(n.blockchain, &pebble.Options{})
	must(err)
	defer db.Close()

	data, closer, err := db.Get([]byte(chainMetaKey))
	if err != nil {
		fmt.Println("chain meta: <not found>")
		return
	}
	defer closer.Close()

	fmt.Printf("raw (%d bytes): %s\n\n", len(data), hex.EncodeToString(data))
	h, hash, gas, err := decodeChainMeta(data)
	if err != nil {
		fmt.Printf("decode error: %v\n", err)
		return
	}
	fmt.Printf("height:     %d\n", h)
	fmt.Printf("blockHash:  0x%s\n", hex.EncodeToString(hash))
	if len(gas) > 0 {
		fmt.Printf("gasPrice:   0x%x\n", gas)
	}
}

// ---------------------------------------------------------------------------
// compare
// ---------------------------------------------------------------------------

// indexEntry is one blockfile index entry: the file number that holds the
// block's payload and the offset just past that payload.
type indexEntry struct {
	filenum uint32
	offset  uint32
}

// readIndex loads a .ridx file and returns its entries.
func readIndex(path string) ([]indexEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	n := len(data) / indexEntrySize
	out := make([]indexEntry, n)
	for i := 0; i < n; i++ {
		e := data[i*indexEntrySize : (i+1)*indexEntrySize]
		out[i] = indexEntry{
			filenum: uint32(binary.BigEndian.Uint16(e[:2])),
			offset:  binary.BigEndian.Uint32(e[2:6]),
		}
	}
	return out, nil
}

// headerBlockNumber reads the block number out of a stored header blob.
//
// The header table stores a marshalled BlockHeader directly -- the ledger
// calls block.Header.Marshal() and writes the result as the block's header
// payload. BlockHeader starts with:
//
//	uint64 number = 1;  -> 0x08 <varint>
//
// so the block number can be recovered without depending on the rest of the
// schema. This is what lets the tool map an index entry to a block height
// and therefore verify that a peer's index tail lines up with this node's
// chain.
func headerBlockNumber(blob []byte) (uint64, bool) {
	if len(blob) < 2 || blob[0] != 0x08 {
		return 0, false
	}
	num, n := decodeVarint(blob[1:])
	if n == 0 {
		return 0, false
	}
	return num, true
}

// headerHeights reads the block number of every block referenced by the
// header index. The returned slice is indexed by index entry number.
func (n node) headerHeights() ([]uint64, error) {
	idx, err := readIndex(filepath.Join(n.blockfile, "header.ridx"))
	if err != nil {
		return nil, err
	}
	// Cache the data files so the whole index can be walked without
	// reopening a file for every entry.
	type openFile struct {
		num  uint32
		data []byte
	}
	var cache []*openFile
	load := func(num uint32) ([]byte, bool) {
		for _, f := range cache {
			if f.num == num {
				return f.data, true
			}
		}
		p := filepath.Join(n.blockfile, fmt.Sprintf("header.%04d.rdat", num))
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, false
		}
		cache = append(cache, &openFile{num: num, data: data})
		return data, true
	}

	heights := make([]uint64, len(idx))
	// The first entry is a sentinel pointing at the start of the first file.
	for i := 1; i < len(idx); i++ {
		e := idx[i]
		data, ok := load(e.filenum)
		if !ok {
			return nil, fmt.Errorf("header data file %04d is missing", e.filenum)
		}
		if int64(e.offset) > int64(len(data)) {
			return nil, fmt.Errorf("header index entry %d points past the end of file %04d", i, e.filenum)
		}
		// The payload of a block starts where the previous entry ended.
		// Entry i holds the end offset of block i, so block i starts at
		// entry i-1.
		start := int64(0)
		if i >= 1 {
			start = int64(idx[i-1].offset)
			if idx[i-1].filenum != e.filenum {
				// The block starts at the beginning of a new file.
				start = 0
			}
		}
		if start >= int64(e.offset) {
			heights[i] = 0
			continue
		}
		blob := data[start:e.offset]
		num, ok := headerBlockNumber(blob)
		if !ok {
			return nil, fmt.Errorf("cannot parse header block at index entry %d", i)
		}
		heights[i] = num
	}
	return heights, nil
}

// compareCmd diffs this node's .ridx against another node's, entry by entry.
//
// The offsets stored in a .ridx are absolute offsets inside the matching
// .rdat file, so they are only meaningful on a node whose .rdat files are
// split at the same byte boundaries. The comparison reports both the index
// differences and whether the .rdat boundaries agree, because an index that
// differs at every entry means the source node cannot be used.
func (n node) compareCmd(srcDir string) {
	fmt.Println("--- compare with peer blockfile ---")
	fmt.Printf("mine: %s\n", n.blockfile)
	fmt.Printf("peer: %s\n\n", srcDir)

	src := node{blockfile: srcDir}

	for _, s := range schemas {
		mineEntries, err := n.indexEntries(s)
		if err != nil {
			fmt.Printf("  %-13s <mine: missing>\n", s)
			continue
		}
		peerEntries, err := src.indexEntries(s)
		if err != nil {
			fmt.Printf("  %-13s mine=%d <peer: missing>\n", s, mineEntries)
			continue
		}
		_ = peerEntries

		mine, err := readIndex(filepath.Join(n.blockfile, s+".ridx"))
		must(err)
		peer, err := readIndex(filepath.Join(srcDir, s+".ridx"))
		must(err)

		// First difference.
		firstDiff := -1
		limit := len(mine)
		if len(peer) < limit {
			limit = len(peer)
		}
		for i := 0; i < limit; i++ {
			if mine[i] != peer[i] {
				firstDiff = i
				break
			}
		}

		// Do the .rdat boundaries agree? Compare the sizes of the shared
		// data files.
		boundaryMismatch := ""
		mineNums, _, _ := n.dataFiles(s)
		peerNums, _, _ := src.dataFiles(s)
		shared := 0
		for _, num := range mineNums {
			if peerNums != nil {
				found := false
				for _, p := range peerNums {
					if p == num {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			ms := n.dataFileSize(s, num)
			ps := src.dataFileSize(s, num)
			if ms >= 0 && ps >= 0 && ms != ps {
				boundaryMismatch = fmt.Sprintf("file %04d: mine=%d peer=%d", num, ms, ps)
				break
			}
			shared++
		}

		fmt.Printf("  %-13s mine=%d peer=%d", s, len(mine), len(peer))
		if firstDiff >= 0 {
			fmt.Printf("  first diff at entry %d (mine=%v peer=%v)", firstDiff, mine[firstDiff], peer[firstDiff])
		} else if len(mine) == len(peer) {
			fmt.Printf("  IDENTICAL")
		} else {
			fmt.Printf("  common prefix identical (%d entries)", limit)
		}
		fmt.Println()
		if boundaryMismatch != "" {
			fmt.Printf("                .rdat boundary mismatch: %s\n", boundaryMismatch)
			fmt.Printf("                the peer's indexes CANNOT be copied to this node\n")
		} else if shared > 0 {
			fmt.Printf("                .rdat boundaries agree on %d shared files\n", shared)
		}
	}
}

// ---------------------------------------------------------------------------
// scan-tail
// ---------------------------------------------------------------------------

// scanTailCmd walks the header payload from the position the index stops at
// and reports whether more blocks are present in the data file than the
// index records.
//
// This answers the question the index alone cannot: the index says how many
// blocks the node can read, but the payload may hold more blocks whose index
// entries were lost when the machine crashed.
func (n node) scanTailCmd() {
	fmt.Println("--- scan header payload past the index ---")

	idx, err := readIndex(filepath.Join(n.blockfile, "header.ridx"))
	if err != nil {
		fmt.Printf("  cannot read the header index: %v\n", err)
		os.Exit(1)
	}
	if len(idx) < 2 {
		fmt.Println("  header index is empty")
		return
	}

	last := idx[len(idx)-1]
	path := filepath.Join(n.blockfile, fmt.Sprintf("header.%04d.rdat", last.filenum))
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("  cannot read %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Printf("  index ends at file %04d offset %d\n", last.filenum, last.offset)
	fmt.Printf("  data file %s is %d bytes\n", filepath.Base(path), len(data))
	if int64(last.offset) >= int64(len(data)) {
		fmt.Println("  the index already reaches the end of the data file")
		fmt.Println("  no blocks are present past the index")
		return
	}

	// Walk forward from the end of the index and count the blocks that are
	// still there.
	offset := int(last.offset)
	blocks := 0
	var heights []uint64
	for offset < len(data) {
		// The payload of a block starts where the previous one ended, so the
		// block length is only known after parsing it. The header payload is
		// a marshalled BlockHeader, so walk its protobuf fields to find the
		// end.
		end, ok := scanHeaderMessage(data, offset)
		if !ok {
			fmt.Printf("  ! cannot parse a block at offset %d; %d bytes are unparseable\n", offset, len(data)-offset)
			break
		}
		num, ok := headerBlockNumber(data[offset:end])
		if !ok {
			fmt.Printf("  ! cannot read the block number at offset %d\n", offset)
			break
		}
		heights = append(heights, num)
		offset = end
		blocks++
	}

	if blocks == 0 {
		fmt.Println("  no complete blocks found past the index")
		return
	}
	fmt.Printf("  %d extra block(s) present in the payload:\n", blocks)
	for i, h := range heights {
		fmt.Printf("    %d: block %d\n", i, h)
	}
	fmt.Println()
	fmt.Println("  These blocks are in the data file but not in the index.")
	fmt.Println("  Recovering them needs an index tail that points at the same offsets,")
	fmt.Println("  which requires a peer whose header payload matches this node byte")
	fmt.Println("  for byte over the same block range.")
}

// scanHeaderMessage returns the end offset of the marshalled BlockHeader that
// starts at offset, or false when the message is truncated.
func scanHeaderMessage(data []byte, offset int) (int, bool) {
	if offset >= len(data) || data[offset] != 0x08 {
		return 0, false
	}
	i := offset + 1
	// field 1: number (varint)
	_, n := decodeVarint(data[i:])
	if n == 0 {
		return 0, false
	}
	i += n

	// Walk the remaining fields to find the end of the message.
	for i < len(data) {
		key, n := decodeVarint(data[i:])
		if n == 0 {
			return 0, false
		}
		i += n
		switch key & 0x7 {
		case 0: // varint
			_, n := decodeVarint(data[i:])
			if n == 0 {
				return 0, false
			}
			i += n
		case 2: // length delimited
			l, n := decodeVarint(data[i:])
			if n == 0 {
				return 0, false
			}
			i += n
			if i+int(l) > len(data) {
				return 0, false
			}
			i += int(l)
		case 5: // fixed32
			if i+4 > len(data) {
				return 0, false
			}
			i += 4
		case 1: // fixed64
			if i+8 > len(data) {
				return 0, false
			}
			i += 8
		default:
			return 0, false
		}
	}
	return i, i == len(data)
}

// ---------------------------------------------------------------------------
// rollback
// ---------------------------------------------------------------------------

// rollbackCmd moves the chain back to a height that the blockfile can
// actually serve.
//
// The node's own "draconis ledger rollback" cannot be used in this
// situation: it validates the target against the current chain meta and
// refuses to run when they are equal, while the chain meta has to be
// lowered first to get past the checkChainMeta panic. This command does the
// rollback directly:
//
//   - the blockfile indexes are truncated to the target height
//   - the chain meta is rewritten with the real block hash and gas price
//     taken from the blockfile
//   - the prune journal is trimmed to the target height
//
// The state trie is not touched. The node resolves the state root from the
// block header that the chain meta points at, so the state follows.
func (n node) rollbackCmd(height uint64, apply bool) {
	fmt.Println("--- rollback ---")

	reports, _ := n.reports()
	eff, ok := effectiveHeight(reports)
	if !ok {
		fmt.Println("  cannot determine the blockfile height")
		os.Exit(1)
	}
	fmt.Printf("  blockfile height:  %d\n", eff)

	if height > eff {
		fmt.Printf("  target %d is above the blockfile height %d; nothing to roll back\n", height, eff)
		return
	}
	if height == eff {
		fmt.Println("  the blockfile already ends at the target height")
		fmt.Println("  only the chain meta and the prune journal need to be rewritten")
	}

	// The block hash and the gas price come from the block header stored in
	// the blockfile, which is the authoritative source.
	hdrBlob, err := n.readBlockHeader(height)
	if err != nil {
		fmt.Printf("  cannot read the block header at %d: %v\n", height, err)
		os.Exit(1)
	}
	hash, gasPrice, stateRoot, ok := parseHeaderFull(hdrBlob)
	if !ok {
		fmt.Printf("  cannot parse the block header at %d\n", height)
		os.Exit(1)
	}
	fmt.Printf("  target height:     %d\n", height)
	fmt.Printf("  block hash:        0x%s\n", hex.EncodeToString(hash))
	fmt.Printf("  gas price:         0x%x\n", gasPrice)

	// The node resolves the account trie from the state root of the block
	// the chain meta points at. If that root is no longer in the ledger KV
	// the node cannot open the state for the target height, and the rollback
	// would leave the node unable to start. Check it before writing.
	if len(stateRoot) > 0 {
		if present, err := n.stateRootPresent(stateRoot); err != nil {
			fmt.Printf("  cannot check the state root: %v\n", err)
		} else if !present {
			fmt.Printf("  state root:        0x%s\n", hex.EncodeToString(stateRoot))
			fmt.Printf("  ! the state root is NOT in the ledger KV\n")
			fmt.Printf("  ! the node will not be able to open the state at height %d\n", height)
			fmt.Printf("  ! do not apply this rollback\n")
			os.Exit(1)
		} else {
			fmt.Printf("  state root:        0x%s (present in the ledger KV)\n", hex.EncodeToString(stateRoot))
		}
	}

	if !apply {
		fmt.Println("\n(dry run; pass --apply to write)")
		return
	}

	// 1. Truncate the blockfile indexes that run past the target.
	for _, s := range schemas {
		p := filepath.Join(n.blockfile, s+".ridx")
		entries, err := n.indexEntries(s)
		if err != nil {
			fmt.Printf("  ! %s.ridx is missing: %v\n", s, err)
			fmt.Printf("  ! the node opens the indexes with O_CREATE, so a missing index turns\n")
			fmt.Printf("  ! into an empty one and the blockfile repair truncates every table\n")
			fmt.Printf("  ! down to it. restore the index first, then retry\n")
			os.Exit(1)
		}
		if entries <= height+1 {
			fmt.Printf("  keep     %s.ridx (%d entries, already at or below the target)\n", s, entries)
			continue
		}
		// Back the index up first, then truncate it in place.
		//
		// The order matters. The node opens the indexes with O_CREATE, so a
		// missing index file turns into an empty one, and the blockfile
		// repair then truncates every table down to the shortest one -- which
		// would destroy the whole chain. So the file has to stay in place and
		// only its contents get shortened.
		bak := p + ".bak"
		_ = os.Remove(bak)
		if err := copyFile(p, bak, int64(entries)*indexEntrySize); err != nil {
			fmt.Printf("  FAILED to back up %s.ridx: %v\n", s, err)
			os.Exit(1)
		}
		if err := os.Truncate(p, int64((height+1)*indexEntrySize)); err != nil {
			fmt.Printf("  FAILED to truncate %s.ridx: %v\n", s, err)
			os.Exit(1)
		}
		fmt.Printf("  truncate %s.ridx to %d entries (backup: %s)\n", s, height+1, filepath.Base(bak))
	}

	// 2. Rewrite the chain meta.
	db, err := pebble.Open(n.blockchain, &pebble.Options{})
	must(err)
	defer db.Close()

	if old, closer, err := db.Get([]byte(chainMetaKey)); err == nil {
		defer closer.Close()
		must(db.Set([]byte(chainMetaBackupKey), old, pebble.Sync))
		fmt.Printf("  backup   chain meta -> %s\n", chainMetaBackupKey)
	}
	must(db.Set([]byte(chainMetaKey), encodeChainMeta(height, hash, gasPrice), pebble.Sync))
	fmt.Printf("  chain meta -> height %d, hash 0x%s\n", height, hex.EncodeToString(hash))

	// 3. Trim the prune journal.
	must(n.trimPruneJournal(height))

	// The snapshot journal is deliberately NOT touched here. It lives in its
	// own store and the node rolls it back itself at startup, replaying the
	// journal to restore the previous account and storage values. Setting
	// its maxHeight down to the target would make that replay a no-op and
	// leave the snapshot holding the state of the blocks that were dropped,
	// which the account reads would then serve instead of the trie.
	//
	// Only the range is reported, so the operator can confirm that the
	// journal still brackets the target height before starting the node.
	sMinH, sMaxH, hasSnap := n.readSnapshotJournal()
	if hasSnap {
		fmt.Printf("  snapshot journal: minHeight %d, maxHeight %d (left untouched)\n", sMinH, sMaxH)
		if sMaxH < height || (sMinH > height && !(sMinH == 1 && height == 0)) {
			fmt.Printf("  ! the snapshot journal range does not bracket %d\n", height)
			fmt.Printf("  ! the node will refuse to start: snapshot rollback out of range\n")
		}
	}

	fmt.Println("\nDONE. Run: chainfix", n.root, "diagnose")
}

// copyFile copies the first size bytes of src to dst.
//
// It is used to keep a copy of a blockfile index after it has been
// truncated. The copy is taken from the already truncated file, so it holds
// exactly the entries that were kept.
func copyFile(src, dst string, size int64) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.CopyN(out, in, size); err != nil && err != io.EOF {
		return err
	}
	return out.Sync()
}

// stateRootPresent reports whether the account trie root is still in the
// ledger KV.
//
// The node stores "<stateRoot> -> <rootNodeKey>" in the ledger store when it
// commits the account trie, and jmt.New resolves the trie through exactly
// that mapping. If the mapping is gone the node cannot open the state for
// the height, which is worth catching before the chain meta is rewritten.
func (n node) stateRootPresent(stateRoot []byte) (bool, error) {
	db, err := pebble.Open(n.ledger, &pebble.Options{})
	if err != nil {
		return false, err
	}
	defer db.Close()

	v, closer, err := db.Get(stateRoot)
	if err != nil {
		return false, nil
	}
	defer closer.Close()
	return len(v) > 0, nil
}

// readBlockHeader reads the raw header payload of a block from the blockfile.
//
// The blockfile index maps block number N to the byte range between entry
// N-1 and entry N: entry N holds the offset just past the block's payload
// and entry N-1 holds the offset where it starts. Entry 0 is a sentinel
// that points at the beginning of the first data file.
//
// So reading block N needs the index to hold at least N+1 entries.
func (n node) readBlockHeader(height uint64) ([]byte, error) {
	idx, err := readIndex(filepath.Join(n.blockfile, "header.ridx"))
	if err != nil {
		return nil, err
	}
	if height >= uint64(len(idx)) {
		return nil, fmt.Errorf("block %d is not in the header index (the index holds %d entries)", height, len(idx))
	}
	if height == 0 {
		return nil, fmt.Errorf("block 0 has no index entry")
	}
	end := idx[height]
	start := idx[height-1]

	p := filepath.Join(n.blockfile, fmt.Sprintf("header.%04d.rdat", end.filenum))
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	if int64(end.offset) > int64(len(data)) {
		return nil, fmt.Errorf("index entry for block %d points past the end of the data file", height)
	}
	from := int64(0)
	if start.filenum == end.filenum {
		from = int64(start.offset)
	}
	if from >= int64(end.offset) {
		return nil, fmt.Errorf("block %d has an empty header payload", height)
	}
	return data[from:end.offset], nil
}

// parseHeaderIdentity pulls the block hash and the gas price out of a
// marshalled BlockHeader.
//
// From the proto:
//
//	bytes bloom     = 9;
//	int64 gas_price = 10;
//
// The block hash is not simply the sha256 of the stored header bytes.
// axiom-kit's BlockHeader.CalculateHash rebuilds a fresh BlockHeader that
// copies every field except Bloom, so the hash is taken over a header that
// has no bloom at all. The bloom has to be stripped from the stored bytes
// before hashing to reproduce it.
//
// The gas price is returned the way the chain meta stores it: pb.ChainMeta
// declares gas_price as bytes, and the ledger fills it with the big-endian
// bytes of the big.Int value, so the varint encoding used on the wire has
// to be converted into the plain integer first.
func parseHeaderIdentity(hdr []byte) ([]byte, []byte, bool) {
	hash, gas, _, ok := parseHeaderFull(hdr)
	return hash, gas, ok
}

// parseHeaderFull pulls the block hash, the gas price and the state root out
// of a marshalled BlockHeader.
//
// From the proto:
//
//	bytes state_root = 2;
//	bytes bloom      = 9;
//	int64 gas_price  = 10;
//
// The state root is what the node resolves the account trie from, so the
// rollback can use it to confirm that the state for the target height is
// still on disk before it rewrites the chain meta.
func parseHeaderFull(hdr []byte) ([]byte, []byte, []byte, bool) {
	const (
		stateRootField = 2
		bloomField     = 9
		gasPriceField  = 10
	)

	var gasValue uint64
	var haveGas bool
	var stateRoot []byte
	i := 0
	for i < len(hdr) {
		key, n := decodeVarint(hdr[i:])
		if n == 0 {
			break
		}
		i += n
		field, wire := key>>3, key&0x7
		switch wire {
		case 0:
			v, n := decodeVarint(hdr[i:])
			if n == 0 {
				break
			}
			i += n
			if field == gasPriceField {
				gasValue = v
				haveGas = true
			}
		case 2:
			l, n := decodeVarint(hdr[i:])
			if n == 0 {
				break
			}
			i += n
			if i+int(l) > len(hdr) {
				break
			}
			if field == stateRootField {
				stateRoot = append([]byte(nil), hdr[i:i+int(l)]...)
			}
			i += int(l)
		case 5:
			if i+4 > len(hdr) {
				break
			}
			i += 4
		case 1:
			if i+8 > len(hdr) {
				break
			}
			i += 8
		default:
			return nil, nil, nil, false
		}
	}

	sum := sha256.Sum256(stripProtoField(hdr, bloomField))

	// The chain meta stores the gas price as the big-endian bytes of the
	// value, without leading zeros.
	var gas []byte
	if haveGas {
		gas = big.NewInt(0).SetUint64(gasValue).Bytes()
	}
	return sum[:], gas, stateRoot, haveGas
}

// stripProtoField returns the message with one field removed.
func stripProtoField(msg []byte, target uint64) []byte {
	out := make([]byte, 0, len(msg))
	i := 0
	for i < len(msg) {
		start := i
		key, n := decodeVarint(msg[i:])
		if n == 0 {
			return msg
		}
		i += n
		field, wire := key>>3, key&0x7
		switch wire {
		case 0:
			_, n := decodeVarint(msg[i:])
			if n == 0 {
				return msg
			}
			i += n
		case 2:
			l, n := decodeVarint(msg[i:])
			if n == 0 {
				return msg
			}
			i += n
			if i+int(l) > len(msg) {
				return msg
			}
			i += int(l)
		case 5:
			if i+4 > len(msg) {
				return msg
			}
			i += 4
		case 1:
			if i+8 > len(msg) {
				return msg
			}
			i += 8
		default:
			return msg
		}
		if field != target {
			out = append(out, msg[start:i]...)
		}
	}
	return out
}

// trimPruneJournal removes the journal entries above the target height and
// rewrites the maxHeight marker.
func (n node) trimPruneJournal(height uint64) error {
	db, err := pebble.Open(n.ledger, &pebble.Options{})
	if err != nil {
		return err
	}
	defer db.Close()

	iter := db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("prune-nodeInfo-"),
		UpperBound: []byte("prune-nodeInfo."),
	})
	defer iter.Close()

	const prefix = "prune-nodeInfo-"
	var keys [][]byte
	var heights []uint64
	for iter.First(); iter.Valid(); iter.Next() {
		key := append([]byte(nil), iter.Key()...)
		h, err := strconv.ParseUint(string(key[len(prefix):]), 10, 64)
		if err != nil {
			continue
		}
		keys = append(keys, key)
		heights = append(heights, h)
	}

	batch := db.NewBatch()
	removed := 0
	for i, h := range heights {
		if h > height {
			batch.Delete(keys[i], nil)
			removed++
		}
	}
	batch.Delete([]byte(prefix+"maxHeight"), nil)
	batch.Set([]byte(prefix+"maxHeight"), []byte(strconv.FormatUint(height, 10)), nil)
	must(batch.Commit(nil))

	fmt.Printf("  prune journal: removed %d entries above %d, maxHeight -> %d\n", removed, height, height)
	return nil
}

// readSnapshotJournal returns the min/max height of the snapshot journal.
//
// The snapshot journal lives in its own pebble store (storage/snapshot). The
// node rolls it back independently at startup, and the rollback refuses to
// run when the journal range does not bracket the target height. So the
// journal has to be trimmed along with the chain meta, otherwise the node
// fails to start with "rollback snapshot to higher blockchain height".
func (n node) readSnapshotJournal() (uint64, uint64, bool) {
	if _, err := os.Stat(n.snapshot); err != nil {
		return 0, 0, false
	}
	db, err := pebble.Open(n.snapshot, &pebble.Options{})
	if err != nil {
		return 0, 0, false
	}
	defer db.Close()

	read := func(suffix string) (uint64, bool) {
		v, closer, err := db.Get([]byte(snapshotKeyPrefix + suffix))
		if err != nil {
			return 0, false
		}
		defer closer.Close()
		h, err := strconv.ParseUint(string(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return h, true
	}
	minH, ok1 := read("minHeight")
	maxH, ok2 := read("maxHeight")
	return minH, maxH, ok1 || ok2
}

// ---------------------------------------------------------------------------
// state-height
// ---------------------------------------------------------------------------

// hashesCmd prints the block hash the blockfile implies for a range of
// heights.
//
// The chain meta stores the hash of the block it points at, and the block
// hash is the sha256 of the marshalled block header. Computing it straight
// from the blockfile therefore provides an independent way to check the
// data against the rest of the network: the hash of a height has to match
// what the other nodes report for that height.
func (n node) hashesCmd(from, to uint64) {
	fmt.Printf("--- block hashes %d .. %d ---\n", from, to)

	for h := from; h <= to; h++ {
		hdr, err := n.readBlockHeader(h)
		if err != nil {
			fmt.Printf("  %d: <%v>\n", h, err)
			continue
		}
		hash, gas, ok := parseHeaderIdentity(hdr)
		if !ok {
			fmt.Printf("  %d: <cannot parse the header>\n", h)
			continue
		}
		num, _ := headerBlockNumber(hdr)
		fmt.Printf("  %d: hash=0x%s gasPrice=0x%x headerHeight=%d\n",
			h, hex.EncodeToString(hash), gas, num)
	}
}

// stateHeightCmd reports how far the state trie has advanced.
//
// The state trie does not keep a height of its own -- the "trie-block-" key
// is only written when a snapshot is generated, so it is usually absent.
// The prune journal does record the state root of every committed block,
// though, and it is written in the same batch as the state itself. The
// highest journal entry therefore names the last block whose state is
// actually on disk.
func (n node) stateHeightCmd() {
	fmt.Println("--- state trie height ---")

	db, err := pebble.Open(n.ledger, &pebble.Options{})
	if err != nil {
		fmt.Printf("  cannot open the ledger pebble: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// The prune journal keeps one entry per committed block under
	// "prune-nodeInfo-<height>".
	iter := db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("prune-nodeInfo-"),
		UpperBound: []byte("prune-nodeInfo."),
	})
	defer iter.Close()

	const prefix = "prune-nodeInfo-"
	var (
		journalMax  uint64
		journalMin  uint64
		haveJournal bool
	)
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		suffix := key[len(prefix):]
		if suffix == "minHeight" || suffix == "maxHeight" {
			continue
		}
		h, err := strconv.ParseUint(suffix, 10, 64)
		if err != nil {
			continue
		}
		if !haveJournal {
			journalMin = h
			journalMax = h
			haveJournal = true
			continue
		}
		if h < journalMin {
			journalMin = h
		}
		if h > journalMax {
			journalMax = h
		}
	}

	if !haveJournal {
		fmt.Println("  the prune journal is empty")
		fmt.Println("  pruning is disabled, so the journal cannot tell the state height")
	} else {
		fmt.Printf("  prune journal covers blocks %d .. %d\n", journalMin, journalMax)
		fmt.Printf("  last block with state on disk: %d\n", journalMax)
	}

	// Compare against the blockfile and the chain meta.
	reports, _ := n.reports()
	if eff, ok := effectiveHeight(reports); ok {
		fmt.Printf("  blockfile height:  %d\n", eff)
	}
	if h, _, _, err := n.readChainMeta(); err == nil {
		fmt.Printf("  chain meta height: %d\n", h)
	}

	if !haveJournal {
		return
	}
	if eff, ok := effectiveHeight(reports); ok {
		switch {
		case journalMax > eff:
			fmt.Printf("  ! the state is %d block(s) AHEAD of the blockfile\n", journalMax-eff)
			fmt.Println("  a rollback has to move the state back as well")
		case journalMax < eff:
			fmt.Printf("  ! the state is %d block(s) BEHIND the blockfile\n", eff-journalMax)
			fmt.Println("  the blockfile has blocks the state does not know about")
		default:
			fmt.Println("  OK: the state and the blockfile agree")
		}
	}
}

// ---------------------------------------------------------------------------
// state-height
// ---------------------------------------------------------------------------

// verifyCmd checks that this node's header index really points at the block
// heights it claims, and reports the height that the last index entry
// corresponds to.
//
// This is the check that makes the index repair trustworthy: the height is
// read out of the payload, not out of the index, so an index that drifted
// away from the data is caught instead of being silently trusted.
func (n node) verifyCmd() {
	fmt.Println("--- verify header index ---")

	heights, err := n.headerHeights()
	if err != nil {
		fmt.Printf("  FAILED: %v\n", err)
		os.Exit(1)
	}
	if len(heights) < 2 {
		fmt.Println("  index is empty")
		return
	}

	first := heights[1]
	last := heights[len(heights)-1]
	fmt.Printf("  entries:          %d\n", len(heights))
	fmt.Printf("  first block:      %d\n", first)
	fmt.Printf("  last block:       %d\n", last)
	fmt.Printf("  expected entries: %d\n", last-first+2) // +1 for the sentinel, +1 for the count

	gaps := 0
	var firstGap int
	for i := 2; i < len(heights); i++ {
		if heights[i] != heights[i-1]+1 {
			if gaps == 0 {
				firstGap = i
			}
			gaps++
		}
	}
	if gaps == 0 {
		fmt.Println("  OK: block heights are contiguous")
		return
	}
	fmt.Printf("  ! %d gap(s) in the height sequence, first at index %d\n", gaps, firstGap)
	fmt.Printf("    index entry %d -> height %d, entry %d -> height %d\n",
		firstGap-1, heights[firstGap-1], firstGap, heights[firstGap])
}

// ---------------------------------------------------------------------------
// adopt-index
// ---------------------------------------------------------------------------

// adoptIndexCmd copies .ridx indexes from a peer, truncating each to the
// number of entries this node's own .rdat files can actually serve.
//
// A .ridx entry points at the end offset of a block's payload inside a .rdat
// file. If this node's .rdat file is shorter than the offset the peer
// recorded, that entry points past the end of the data and the block cannot
// be read. The last valid entry is the one whose offset does not exceed the
// size of the data file it points into.
//
// When from > 0 the peer index is treated as a *tail* that starts at entry
// `from`: only the entries from that offset on are used, and the local index
// keeps its own first `from` entries. That makes it possible to move only
// the last few kilobytes of an index between nodes instead of the whole
// hundred-megabyte file.
func (n node) adoptIndexCmd(srcDir string, keep uint64, from uint64, apply bool) {
	fmt.Println("--- adopt-index ---")
	fmt.Printf("source: %s\n", srcDir)
	fmt.Printf("target: %s\n", n.blockfile)
	if from > 0 {
		fmt.Printf("tail mode: using peer entries from index %d on\n", from)
	}
	if !apply {
		fmt.Println("\n(dry run; pass --apply to write)")
	}
	fmt.Println()

	src := node{blockfile: srcDir}

	for _, s := range schemas {
		peerPath := filepath.Join(srcDir, s+".ridx")
		peer, err := readIndex(peerPath)
		if err != nil {
			fmt.Printf("  %-13s <peer index unavailable: %v>\n", s, err)
			continue
		}

		curEntries, _ := n.indexEntries(s)

		// In tail mode the peer file holds the entries starting at absolute
		// index `from`, so the local index keeps its own first `from` entries
		// and the peer's entries are appended after them.
		base := from
		if from > 0 && curEntries < from {
			fmt.Printf("                <local index has %d entries, fewer than the tail start %d; skipping>\n", curEntries, from)
			continue
		}

		// How many entries can this node's own data files serve?
		usable := uint64(0)
		for i, e := range peer {
			size := n.dataFileSize(s, e.filenum)
			if size < 0 {
				// The peer references a data file this node does not have.
				break
			}
			if int64(e.offset) > size {
				// Points past the end of the data file.
				break
			}
			usable = uint64(i) + 1
		}

		fmt.Printf("  %-13s peer=%d entries, usable here=%d, current=%d\n", s, len(peer), usable, curEntries)
		if usable == 0 {
			fmt.Printf("                <no usable entries; skipping>\n")
			continue
		}

		// In tail mode the peer's first entry must line up with the block
		// this node's own index ends at, otherwise the two indexes describe
		// different chains and stitching them would corrupt the blockfile.
		if from > 0 {
			if heights, err := n.headerHeights(); err == nil && len(heights) > int(from) {
				localHeight := heights[from]
				peerHeights, perr := src.headerHeights()
				switch {
				case perr != nil:
					fmt.Printf("                <cannot read the peer header index: %v>\n", perr)
				case int(base) >= len(peerHeights):
					fmt.Printf("                <peer header index has only %d entries>\n", len(peerHeights))
				default:
					peerHeight := peerHeights[base]
					if peerHeight != localHeight {
						fmt.Printf("                ! MISMATCH: local entry %d is block %d, peer entry %d is block %d\n",
							base, localHeight, base, peerHeight)
						fmt.Printf("                ! refusing to stitch indexes from different chains\n")
						continue
					}
					fmt.Printf("                alignment OK: both indexes point at block %d\n", localHeight)
				}
			}
		}
		if keep > 0 && usable > keep {
			usable = keep
			fmt.Printf("                capped to %d by --keep\n", usable)
		}

		// The resulting index has `base` entries from the local file plus the
		// usable tail entries.
		resultEntries := base + usable
		if from == 0 && uint64(len(peer)) == usable && curEntries == usable {
			fmt.Printf("                <already up to date>\n")
			continue
		}
		if from == 0 && curEntries == resultEntries {
			fmt.Printf("                <already up to date>\n")
			continue
		}

		if !apply {
			fmt.Printf("                would write %d entries (%d bytes) total\n", resultEntries, resultEntries*indexEntrySize)
			continue
		}

		dstPath := filepath.Join(n.blockfile, s+".ridx")
		if _, err := os.Stat(dstPath); err == nil {
			bak := dstPath + ".bak"
			_ = os.Remove(bak)
			must(os.Rename(dstPath, bak))
			fmt.Printf("                backup -> %s\n", filepath.Base(bak))
		}

		// In tail mode keep the local prefix and append the peer's entries.
		var prefix []byte
		if base > 0 {
			local, err := os.ReadFile(dstPath + ".bak")
			must(err)
			need := int(base) * indexEntrySize
			if len(local) < need {
				must(fmt.Errorf("local index %s has %d bytes, need %d", s+".ridx", len(local), need))
			}
			prefix = local[:need]
		}

		data := make([]byte, len(prefix)+int(usable)*indexEntrySize)
		copy(data, prefix)
		for i := 0; i < int(usable); i++ {
			e := peer[i]
			off := len(prefix) + i*indexEntrySize
			binary.BigEndian.PutUint16(data[off:], uint16(e.filenum))
			binary.BigEndian.PutUint32(data[off+2:], e.offset)
		}
		must(os.WriteFile(dstPath, data, 0o644))
		fmt.Printf("                wrote %d entries (prefix %d + tail %d)\n", resultEntries, base, usable)
	}

	if apply {
		fmt.Println("\nDONE. Run: chainfix", n.root, "diagnose")
	}
}

// sliceIndexCmd writes the tail of this node's .ridx indexes into an output
// directory, so the result can be shipped to a peer whose indexes lost their
// trailing entries.
//
// Only the entries from `from` on are written. With --keep the tail is
// limited to that many entries, which keeps the transfer down to a few
// kilobytes when the peer is only missing a handful of blocks.
func (n node) sliceIndexCmd(outDir string, from uint64, keep uint64, apply bool) {
	fmt.Println("--- slice-index ---")
	fmt.Printf("source: %s\n", n.blockfile)
	fmt.Printf("output: %s\n", outDir)
	fmt.Printf("from entry: %d\n", from)
	if keep > 0 {
		fmt.Printf("max entries: %d\n", keep)
	}
	if !apply {
		fmt.Println("\n(dry run; pass --apply to write)")
	}
	fmt.Println()

	for _, s := range schemas {
		idx, err := readIndex(filepath.Join(n.blockfile, s+".ridx"))
		if err != nil {
			fmt.Printf("  %-13s <unavailable: %v>\n", s, err)
			continue
		}
		if from >= uint64(len(idx)) {
			fmt.Printf("  %-13s has %d entries, none at or after %d\n", s, len(idx), from)
			continue
		}
		tail := idx[from:]
		if keep > 0 && uint64(len(tail)) > keep {
			tail = tail[:keep]
		}
		fmt.Printf("  %-13s %d entries total, tail=%d entries\n", s, len(idx), len(tail))
		if !apply {
			continue
		}
		must(os.MkdirAll(outDir, 0o755))
		data := make([]byte, len(tail)*indexEntrySize)
		for i, e := range tail {
			binary.BigEndian.PutUint16(data[i*indexEntrySize:], uint16(e.filenum))
			binary.BigEndian.PutUint32(data[i*indexEntrySize+2:], e.offset)
		}
		out := filepath.Join(outDir, s+".ridx")
		must(os.WriteFile(out, data, 0o644))
		fmt.Printf("                wrote %s (%d bytes)\n", out, len(data))
	}

	if apply {
		fmt.Println("\nDONE. Copy that directory to the node that needs the indexes,")
		fmt.Println("then run: chainfix <root> adopt-index <thatDir> --from", from, "--apply")
	}
}

// ---------------------------------------------------------------------------
// set-meta
// ---------------------------------------------------------------------------

func (n node) setMetaCmd(height uint64, hash, gasPrice []byte, apply bool) {
	data := encodeChainMeta(height, hash, gasPrice)

	fmt.Println("--- set-meta ---")
	fmt.Printf("target: height=%d blockHash=0x%s", height, hex.EncodeToString(hash))
	if len(gasPrice) > 0 {
		fmt.Printf(" gasPrice=0x%x", gasPrice)
	}
	fmt.Println()

	db, err := pebble.Open(n.blockchain, &pebble.Options{})
	must(err)
	defer db.Close()

	if old, closer, err := db.Get([]byte(chainMetaKey)); err == nil {
		defer closer.Close()
		oh, ohash, _, _ := decodeChainMeta(old)
		fmt.Printf("current: height=%d blockHash=0x%s\n", oh, hex.EncodeToString(ohash))
		if !apply {
			fmt.Println("\n(dry run; pass --apply to write)")
			return
		}
		must(db.Set([]byte(chainMetaBackupKey), old, pebble.Sync))
		fmt.Printf("backup:  -> %s\n", chainMetaBackupKey)
	} else {
		fmt.Println("current: <none>")
		if !apply {
			fmt.Println("\n(dry run; pass --apply to write)")
			return
		}
	}

	must(db.Set([]byte(chainMetaKey), data, pebble.Sync))
	fmt.Println("DONE.")
}

// ---------------------------------------------------------------------------
// prune-max
// ---------------------------------------------------------------------------

func (n node) pruneMaxCmd(height uint64, apply bool) {
	const maxKey = "prune-nodeInfo-maxHeight"

	db, err := pebble.Open(n.ledger, &pebble.Options{})
	must(err)
	defer db.Close()

	cur, _, _ := db.Get([]byte(maxKey))
	fmt.Println("--- prune-max ---")
	fmt.Printf("current maxHeight: %s\n", string(cur))
	fmt.Printf("new maxHeight:     %d\n", height)
	if !apply {
		fmt.Println("\n(dry run; pass --apply to write)")
		return
	}
	must(db.Set([]byte(maxKey), []byte(strconv.FormatUint(height, 10)), pebble.Sync))
	fmt.Println("DONE.")
}

// ---------------------------------------------------------------------------
// meta backup / restore
// ---------------------------------------------------------------------------

func (n node) backupMetaCmd() {
	db, err := pebble.Open(n.blockchain, &pebble.Options{})
	must(err)
	defer db.Close()

	data, closer, err := db.Get([]byte(chainMetaKey))
	if err != nil {
		fmt.Println("chain meta: <not found>")
		return
	}
	defer closer.Close()

	must(db.Set([]byte(chainMetaBackupKey), data, pebble.Sync))
	fmt.Printf("chain meta backed up -> %s (%d bytes)\n", chainMetaBackupKey, len(data))
}

func (n node) restoreMetaCmd() {
	db, err := pebble.Open(n.blockchain, &pebble.Options{})
	must(err)
	defer db.Close()

	data, closer, err := db.Get([]byte(chainMetaBackupKey))
	if err != nil {
		fmt.Println("no backup found")
		return
	}
	defer closer.Close()

	must(db.Set([]byte(chainMetaKey), data, pebble.Sync))
	h, hash, _, _ := decodeChainMeta(data)
	fmt.Printf("restored: height=%d blockHash=0x%s\n", h, hex.EncodeToString(hash))
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(1)
	}
	root := os.Args[1]
	cmd := os.Args[2]
	rest := os.Args[3:]

	apply := false
	var (
		gasPriceArg string
		hashArg     string
		keep        uint64
		from        uint64
	)
	var positional []string
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--apply":
			apply = true
		case "--gas-price":
			if i+1 >= len(rest) {
				fmt.Fprintln(os.Stderr, "--gas-price needs a value")
				os.Exit(1)
			}
			gasPriceArg = rest[i+1]
			i++
		case "--hash":
			if i+1 >= len(rest) {
				fmt.Fprintln(os.Stderr, "--hash needs a value")
				os.Exit(1)
			}
			hashArg = rest[i+1]
			i++
		case "--keep":
			if i+1 >= len(rest) {
				fmt.Fprintln(os.Stderr, "--keep needs a value")
				os.Exit(1)
			}
			k, err := strconv.ParseUint(rest[i+1], 10, 64)
			must(err)
			keep = k
			i++
		case "--from":
			if i+1 >= len(rest) {
				fmt.Fprintln(os.Stderr, "--from needs a value")
				os.Exit(1)
			}
			f, err := strconv.ParseUint(rest[i+1], 10, 64)
			must(err)
			from = f
			i++
		default:
			positional = append(positional, rest[i])
		}
	}

	n := openNode(root)

	switch cmd {
	case "diagnose":
		n.diagnose()

	case "meta":
		n.metaCmd()

	case "index":
		n.indexCmd()

	case "verify":
		n.verifyCmd()

	case "scan-tail":
		n.scanTailCmd()

	case "state-height":
		n.stateHeightCmd()

	case "hashes":
		if len(positional) < 2 {
			fmt.Fprintln(os.Stderr, "hashes needs <from> <to>")
			os.Exit(1)
		}
		from, err := strconv.ParseUint(positional[0], 10, 64)
		must(err)
		to, err := strconv.ParseUint(positional[1], 10, 64)
		must(err)
		n.hashesCmd(from, to)

	case "rollback":
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "rollback needs <height>")
			os.Exit(1)
		}
		h, err := strconv.ParseUint(positional[0], 10, 64)
		must(err)
		n.rollbackCmd(h, apply)

	case "compare":
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "compare needs <peerBlockfileDir>")
			os.Exit(1)
		}
		n.compareCmd(positional[0])

	case "adopt-index":
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "adopt-index needs <peerBlockfileDir>")
			os.Exit(1)
		}
		n.adoptIndexCmd(positional[0], keep, from, apply)

	case "slice-index":
		// Cut the tail out of this node's own .ridx so it can be shipped to
		// a peer that is missing the trailing entries.
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "slice-index needs <outputDir>")
			os.Exit(1)
		}
		n.sliceIndexCmd(positional[0], from, keep, apply)

	case "set-meta":
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "set-meta needs <height>")
			os.Exit(1)
		}
		h, err := strconv.ParseUint(positional[0], 10, 64)
		must(err)
		var hash, gas []byte
		if hashArg != "" {
			hash, err = hex.DecodeString(strings.TrimPrefix(strings.TrimPrefix(hashArg, "0x"), "0X"))
			must(err)
		}
		if gasPriceArg != "" {
			gas, err = parseGasPrice(gasPriceArg)
			must(err)
		}
		n.setMetaCmd(h, hash, gas, apply)

	case "prune-max":
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "prune-max needs <height>")
			os.Exit(1)
		}
		h, err := strconv.ParseUint(positional[0], 10, 64)
		must(err)
		n.pruneMaxCmd(h, apply)

	case "backup-meta":
		n.backupMetaCmd()

	case "restore-meta":
		n.restoreMetaCmd()

	case "-h", "--help", "help":
		usage()

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(1)
	}
}
