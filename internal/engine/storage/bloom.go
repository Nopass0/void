// Package storage – bloom.go provides the Bloom filter used by segments
// to avoid unnecessary disk I/O when looking up missing keys.
package storage

import "math"

// BloomFilter is a probabilistic membership test data structure.
// False positives are possible; false negatives are not.
type BloomFilter struct {
	bits    []uint64
	numBits uint64
	numHash uint32
}

// NewBloomFilter creates a filter sized for n expected items at false-positive
// probability p (e.g. 0.01 = 1%).
func NewBloomFilter(n int, p float64) *BloomFilter {
	if n <= 0 {
		n = 1
	}
	m := uint64(math.Ceil(-float64(n) * math.Log(p) / (math.Log(2) * math.Log(2))))
	if m < 64 {
		m = 64
	}
	m = (m + 63) &^ 63
	k := uint32(math.Round(float64(m) / float64(n) * math.Log(2)))
	if k < 1 {
		k = 1
	}
	return &BloomFilter{bits: make([]uint64, m/64), numBits: m, numHash: k}
}

// BloomFromBytes reconstructs a BloomFilter from serialized bytes.
func BloomFromBytes(data []byte) *BloomFilter {
	if len(data) < 12 {
		return NewBloomFilter(1, 0.01)
	}
	var numBits uint64
	for i := 0; i < 8; i++ {
		numBits |= uint64(data[i]) << (i * 8)
	}
	var numHash uint32
	for i := 0; i < 4; i++ {
		numHash |= uint32(data[8+i]) << (i * 8)
	}
	wordsNeeded := int(numBits / 64)
	bits := make([]uint64, wordsNeeded)
	for i := 0; i < wordsNeeded && 12+i*8+8 <= len(data); i++ {
		off := 12 + i*8
		var w uint64
		for j := 0; j < 8; j++ {
			w |= uint64(data[off+j]) << (j * 8)
		}
		bits[i] = w
	}
	return &BloomFilter{bits: bits, numBits: numBits, numHash: numHash}
}

// Add inserts key into the filter.
func (f *BloomFilter) Add(key []byte) {
	h1, h2 := bloomHash(key)
	for i := uint32(0); i < f.numHash; i++ {
		bit := (h1 + uint64(i)*h2) % f.numBits
		f.bits[bit/64] |= 1 << (bit % 64)
	}
}

// Test returns false if key is definitely absent.
func (f *BloomFilter) Test(key []byte) bool {
	h1, h2 := bloomHash(key)
	for i := uint32(0); i < f.numHash; i++ {
		bit := (h1 + uint64(i)*h2) % f.numBits
		if f.bits[bit/64]&(1<<(bit%64)) == 0 {
			return false
		}
	}
	return true
}

// Bytes serializes the filter.
func (f *BloomFilter) Bytes() []byte {
	buf := make([]byte, 12+len(f.bits)*8)
	n := f.numBits
	for i := 0; i < 8; i++ {
		buf[i] = byte(n)
		n >>= 8
	}
	k := f.numHash
	for i := 0; i < 4; i++ {
		buf[8+i] = byte(k)
		k >>= 8
	}
	for i, w := range f.bits {
		off := 12 + i*8
		for j := 0; j < 8; j++ {
			buf[off+j] = byte(w)
			w >>= 8
		}
	}
	return buf
}

// bloomHash returns two 64-bit hashes for double-hashing trick.
func bloomHash(key []byte) (uint64, uint64) {
	const (
		prime64  = uint64(1099511628211)
		offset64 = uint64(14695981039346656037)
	)
	h1 := offset64
	for _, b := range key {
		h1 ^= uint64(b)
		h1 *= prime64
	}
	h2 := offset64 ^ 0xdeadbeefcafe
	for i := len(key) - 1; i >= 0; i-- {
		h2 ^= uint64(key[i])
		h2 *= prime64
	}
	h2 |= 1
	return h1, h2
}
