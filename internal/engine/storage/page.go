// Package storage implements the low-level page-based storage layer for VoidDB.
// Each file is divided into fixed-size 4 KB pages, aligned to the OS page size
// for zero-copy mmap access. Pages carry a type tag, a CRC-32 checksum, and a
// free-space pointer so the upper layers can pack variable-length records
// without internal fragmentation.
package storage

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	// PageSize is 4 KB – matches OS virtual memory page size for mmap efficiency.
	PageSize = 4096

	// PageHeaderSize is the number of bytes consumed by the fixed page header.
	// Layout (24 bytes):
	//   [0:4]  magic   uint32  = 0x564F4944 ("VOID")
	//   [4:5]  type    uint8
	//   [5:6]  flags   uint8
	//   [6:8]  version uint16
	//   [8:12] crc32   uint32   (covers bytes [12:PageSize])
	//   [12:14] numSlots uint16
	//   [14:16] freeOff  uint16  (offset of start of free space)
	//   [16:24] nextPage uint64  (linked list for overflow)
	PageHeaderSize = 24

	// PageBodySize is the usable payload area per page.
	PageBodySize = PageSize - PageHeaderSize

	// pageMagic identifies valid VoidDB pages.
	pageMagic = uint32(0x564F4944)
)

// PageType classifies the purpose of a page.
type PageType uint8

const (
	// PageTypeFree means the page is not in use.
	PageTypeFree PageType = 0
	// PageTypeData stores document records.
	PageTypeData PageType = 1
	// PageTypeIndex stores B+ tree index nodes.
	PageTypeIndex PageType = 2
	// PageTypeWAL stores write-ahead log entries.
	PageTypeWAL PageType = 3
	// PageTypeMeta stores database-level metadata.
	PageTypeMeta PageType = 4
	// PageTypeOverflow continues a large record that overflows one page.
	PageTypeOverflow PageType = 5
)

// Page is an in-memory representation of a 4 KB storage page.
// The raw [PageSize]byte slice is always kept in sync with the struct fields
// via Marshal/Unmarshal so that writes to mmap'd files are zero-copy.
type Page struct {
	raw [PageSize]byte
}

// NewPage allocates a new blank page of the given type.
func NewPage(pt PageType) *Page {
	p := &Page{}
	binary.LittleEndian.PutUint32(p.raw[0:4], pageMagic)
	p.raw[4] = byte(pt)
	p.raw[5] = 0 // flags
	binary.LittleEndian.PutUint16(p.raw[6:8], 1) // version
	binary.LittleEndian.PutUint16(p.raw[12:14], 0)
	binary.LittleEndian.PutUint16(p.raw[14:16], PageHeaderSize)
	binary.LittleEndian.PutUint64(p.raw[16:24], 0)
	return p
}

// PageFromBytes wraps an existing 4 KB slice without copying.
// Returns an error if the magic number or checksum is invalid.
func PageFromBytes(b []byte) (*Page, error) {
	if len(b) < PageSize {
		return nil, fmt.Errorf("storage/page: buffer too small (%d < %d)", len(b), PageSize)
	}
	p := &Page{}
	copy(p.raw[:], b[:PageSize])
	if err := p.validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// Raw returns a slice over the underlying [PageSize]byte array.
// Writes to this slice mutate the page directly (used for mmap writes).
func (p *Page) Raw() []byte { return p.raw[:] }

// Type returns the PageType stored in the header.
func (p *Page) Type() PageType { return PageType(p.raw[4]) }

// NumSlots returns the number of slot entries in the page.
func (p *Page) NumSlots() uint16 {
	return binary.LittleEndian.Uint16(p.raw[12:14])
}

// FreeOffset returns the byte offset where free space starts.
func (p *Page) FreeOffset() uint16 {
	return binary.LittleEndian.Uint16(p.raw[14:16])
}

// FreeSpace returns how many bytes are available for new records.
func (p *Page) FreeSpace() int {
	return PageSize - int(p.FreeOffset())
}

// NextPage returns the page number of the overflow continuation (0 = none).
func (p *Page) NextPage() uint64 {
	return binary.LittleEndian.Uint64(p.raw[16:24])
}

// SetNextPage sets the overflow linked-list pointer.
func (p *Page) SetNextPage(n uint64) {
	binary.LittleEndian.PutUint64(p.raw[16:24], n)
}

// Body returns the writable payload area (after the header).
func (p *Page) Body() []byte {
	return p.raw[PageHeaderSize:]
}

// AppendRecord writes data into the next available slot in the page.
// Returns the slot index and byte offset within the body, or an error if
// there is not enough free space.
func (p *Page) AppendRecord(data []byte) (slotIdx uint16, bodyOff uint16, err error) {
	needed := len(data) + 4 // 4 bytes for the slot pointer (offset+len uint16 pair)
	if p.FreeSpace() < needed {
		return 0, 0, fmt.Errorf("storage/page: not enough free space (%d < %d)", p.FreeSpace(), needed)
	}

	numSlots := p.NumSlots()
	freeOff := p.FreeOffset()

	// Slot directory grows from the beginning of the body;
	// record data grows from freeOff toward the end.
	// Slot entry: [bodyOffset uint16][recordLen uint16]
	slotDirEnd := PageHeaderSize + int(numSlots)*4
	if slotDirEnd+4+len(data) > PageSize {
		return 0, 0, fmt.Errorf("storage/page: slot directory collision")
	}

	// Write record at freeOff.
	copy(p.raw[freeOff:], data)

	// Write slot entry.
	slotPos := PageHeaderSize + int(numSlots)*4
	binary.LittleEndian.PutUint16(p.raw[slotPos:], freeOff)
	binary.LittleEndian.PutUint16(p.raw[slotPos+2:], uint16(len(data)))

	// Update header.
	binary.LittleEndian.PutUint16(p.raw[12:14], numSlots+1)
	binary.LittleEndian.PutUint16(p.raw[14:16], freeOff+uint16(len(data)))

	return numSlots, freeOff, nil
}

// ReadRecord returns the raw bytes for the record at slotIdx.
func (p *Page) ReadRecord(slotIdx uint16) ([]byte, error) {
	if slotIdx >= p.NumSlots() {
		return nil, fmt.Errorf("storage/page: slot %d out of range (numSlots=%d)", slotIdx, p.NumSlots())
	}
	slotPos := PageHeaderSize + int(slotIdx)*4
	off := binary.LittleEndian.Uint16(p.raw[slotPos:])
	rlen := binary.LittleEndian.Uint16(p.raw[slotPos+2:])
	if int(off)+int(rlen) > PageSize {
		return nil, fmt.Errorf("storage/page: record at slot %d is out of bounds", slotIdx)
	}
	return p.raw[off : off+rlen], nil
}

// Seal writes the CRC-32 checksum into the header so the page can be verified
// on next load. Call Seal before writing a page to disk.
func (p *Page) Seal() {
	// Zero out the checksum field before computing.
	binary.LittleEndian.PutUint32(p.raw[8:12], 0)
	checksum := crc32.ChecksumIEEE(p.raw[12:])
	binary.LittleEndian.PutUint32(p.raw[8:12], checksum)
}

// validate checks magic and CRC-32 after reading a page from disk.
func (p *Page) validate() error {
	magic := binary.LittleEndian.Uint32(p.raw[0:4])
	if magic != pageMagic {
		return fmt.Errorf("storage/page: invalid magic 0x%08X (expected 0x%08X)", magic, pageMagic)
	}
	stored := binary.LittleEndian.Uint32(p.raw[8:12])
	// Temporarily zero the stored checksum to recompute.
	binary.LittleEndian.PutUint32(p.raw[8:12], 0)
	computed := crc32.ChecksumIEEE(p.raw[12:])
	binary.LittleEndian.PutUint32(p.raw[8:12], stored)
	if stored != computed {
		return fmt.Errorf("storage/page: CRC mismatch (stored=%08X computed=%08X)", stored, computed)
	}
	return nil
}
