// Package crc64 is a pure-Go, SIMD-accelerated drop-in replacement for the
// standard library's hash/crc64. It computes bit-identical CRC-64 checksums
// (both the ISO and ECMA polynomials, and any custom polynomial) but folds the
// bulk of the input with the host's carryless-multiply unit:
//
//	amd64    PCLMULQDQ / VPCLMULQDQ   (gated on cpu.X86.HasPCLMULQDQ)
//	arm64    PMULL / PMULL2           (gated on cpu.ARM64.HasPMULL)
//	ppc64le  VPMSUMD                  (VSX, baseline on POWER8+)
//	s390x    VGFMG / VGFMAG           (gated on cpu.S390X.HasVX)
//
// riscv64 and loong64 fall back to the stdlib-equivalent scalar table code.
// There is no cgo, no GOEXPERIMENT and no assembler intrinsic requirement: a
// plain `go build` produces the accelerated binary on every target above.
//
// The API mirrors hash/crc64 exactly, so it can be swapped in by changing only
// the import path.
package crc64

import (
	"hash"
	stdcrc64 "hash/crc64"
)

// The size of a CRC-64 checksum in bytes.
const Size = 8

// Predefined polynomials (identical to hash/crc64).
const (
	// ISO polynomial, defined in ISO 3309 and used in HDLC.
	ISO = stdcrc64.ISO
	// ECMA polynomial, defined in ECMA 182.
	ECMA = stdcrc64.ECMA
)

// Table is a 256-word table representing the polynomial for efficient
// processing. It is the same type as hash/crc64.Table so tables are
// interchangeable between the two packages.
type Table = stdcrc64.Table

// MakeTable returns a Table constructed from the specified polynomial.
// The contents of this Table must not be modified.
func MakeTable(poly uint64) *Table { return stdcrc64.MakeTable(poly) }

// New creates a new hash.Hash64 computing the CRC-64 checksum using the
// polynomial represented by the Table. Its Sum method lays the value out in
// big-endian byte order. The returned Hash64 also implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
//
// The hash uses the SIMD-accelerated kernel for its Write path.
func New(tab *Table) hash.Hash64 { return &digest{0, tab} }

// digest mirrors hash/crc64's digest but routes Write through the SIMD update.
type digest struct {
	crc uint64
	tab *Table
}

func (d *digest) Size() int      { return Size }
func (d *digest) BlockSize() int { return 1 }
func (d *digest) Reset()         { d.crc = 0 }

func (d *digest) Write(p []byte) (int, error) {
	d.crc = update(d.crc, d.tab, p)
	return len(p), nil
}

func (d *digest) Sum64() uint64 { return d.crc }

func (d *digest) Sum(in []byte) []byte {
	s := d.Sum64()
	return append(in, byte(s>>56), byte(s>>48), byte(s>>40), byte(s>>32),
		byte(s>>24), byte(s>>16), byte(s>>8), byte(s))
}

const (
	magic         = "crc\x02"
	marshaledSize = len(magic) + 8 + 8
)

func (d *digest) MarshalBinary() ([]byte, error) {
	b := make([]byte, 0, marshaledSize)
	b = append(b, magic...)
	b = appendUint64(b, tableSum(d.tab))
	b = appendUint64(b, d.crc)
	return b, nil
}

// AppendBinary implements encoding.BinaryAppender.
func (d *digest) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, magic...)
	b = appendUint64(b, tableSum(d.tab))
	b = appendUint64(b, d.crc)
	return b, nil
}

func (d *digest) UnmarshalBinary(b []byte) error {
	if len(b) < len(magic) || string(b[:len(magic)]) != magic {
		return errInvalidIdentifier
	}
	if len(b) != marshaledSize {
		return errInvalidSize
	}
	if tableSum(d.tab) != beUint64(b[4:]) {
		return errTablesMismatch
	}
	d.crc = beUint64(b[12:])
	return nil
}

// Checksum returns the CRC-64 checksum of data using the polynomial represented
// by the Table.
func Checksum(data []byte, tab *Table) uint64 { return update(0, tab, data) }

// Update returns the result of adding the bytes in p to the crc.
func Update(crc uint64, tab *Table, p []byte) uint64 { return update(crc, tab, p) }

// tableSum returns the ISO checksum of table t (matches hash/crc64.tableSum,
// used for the marshaled-state table fingerprint).
func tableSum(t *Table) uint64 {
	var a [2048]byte
	b := a[:0]
	if t != nil {
		for _, x := range t {
			b = appendUint64(b, x)
		}
	}
	return Checksum(b, MakeTable(ISO))
}

func appendUint64(b []byte, v uint64) []byte {
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
		byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func beUint64(b []byte) uint64 {
	_ = b[7]
	return uint64(b[7]) | uint64(b[6])<<8 | uint64(b[5])<<16 | uint64(b[4])<<24 |
		uint64(b[3])<<32 | uint64(b[2])<<40 | uint64(b[1])<<48 | uint64(b[0])<<56
}
