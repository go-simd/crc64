package crc64

import (
	stdcrc64 "hash/crc64"
	"math/bits"
)

// minBulk is the smallest input for which the CLMUL kernel is engaged. Below it
// the per-call setup (constant lookup, the 128->64 reduction and the scalar
// tail) outweighs the fold's throughput advantage, so the stdlib scalar path is
// used instead. Measured crossover on native amd64/arm64 is a few hundred bytes;
// 512 keeps the kernel strictly in its winning regime.
const minBulk = 512

// polyOf recovers the reflected polynomial from a table. In hash/crc64's
// reflected table, entry 128 (0b1000_0000) shifts its single set bit down to
// bit 0 over the eight inner iterations and XORs the polynomial exactly once,
// so tab[128] == poly for every polynomial.
func polyOf(tab *Table) uint64 { return tab[128] }

// foldConstants holds the reflected CLMUL fold constants for a given polynomial
// as a flat array, so both the Go and the assembly kernels index it at the same
// offsets. Every value is reflect64(x^n mod P) for the labelled n, derived from
// the polynomial itself (no magic numbers). The two constants sit at adjacent
// offsets so a single 128-bit load brings the pair into one vector register in
// the lane order [stepHi | stepLo].
const (
	kStepLo = iota // reflect(x^191 mod P): folds the accumulator low word
	kStepHi        // reflect(x^127 mod P): folds the accumulator high word
	numConst
)

type foldConstants [numConst]uint64

func foldK(n int, polyRef uint64) uint64 {
	polyNorm := bits.Reverse64(polyRef)
	rem := uint64(1)
	for i := 0; i < n; i++ {
		msb := rem >> 63
		rem <<= 1
		if msb == 1 {
			rem ^= polyNorm
		}
	}
	return bits.Reverse64(rem)
}

func makeConstants(polyRef uint64) foldConstants {
	return foldConstants{
		kStepLo: foldK(191, polyRef),
		kStepHi: foldK(127, polyRef),
	}
}

// reduce128 reduces the reflected 128-bit value (hi:lo)*x^64 modulo the
// polynomial to a 64-bit reflected CRC remainder. It runs once per Checksum
// call on a fixed 128 bits, so the straightforward bit-serial form is used:
// it is provably correct and entirely outside the hot loop.
func reduce128(hi, lo, polyRef uint64) uint64 {
	// rev128(hi,lo) = (rev(lo), rev(hi)). Reversing the whole 128-bit reflected
	// value to normal bit order puts rev(hi) in the low 64 bits and rev(lo) in
	// the high 64 bits.
	polyNorm := bits.Reverse64(polyRef)
	// limbs cover bits 0..63, 64..127, 128..191 (value shifted left by x^64).
	b0, b1, b2 := uint64(0), bits.Reverse64(hi), bits.Reverse64(lo)
	for k := 191; k >= 64; k-- {
		var word *uint64
		switch k / 64 {
		case 1:
			word = &b1
		case 2:
			word = &b2
		}
		if (*word>>uint(k%64))&1 == 0 {
			continue
		}
		*word ^= uint64(1) << uint(k%64)
		sh := k - 64
		off := uint(sh % 64)
		switch sh / 64 {
		case 0:
			if off == 0 {
				b0 ^= polyNorm
			} else {
				b0 ^= polyNorm << off
				b1 ^= polyNorm >> (64 - off)
			}
		case 1:
			if off == 0 {
				b1 ^= polyNorm
			} else {
				b1 ^= polyNorm << off
				b2 ^= polyNorm >> (64 - off)
			}
		}
	}
	return bits.Reverse64(b0)
}

// Precomputed fold constants for the two predefined polynomials. Caching them
// (and passing a pointer to the cached value, rather than a freshly-built local)
// keeps the hot path allocation-free for the overwhelmingly common ISO and ECMA
// cases — a stack-allocated foldConstants would escape to the heap when its
// address is handed to the assembly kernel.
var (
	constsISO  = makeConstants(ISO)
	constsECMA = makeConstants(ECMA)
)

// constantsFor returns a pointer to the fold constants for poly without
// allocating for the predefined polynomials.
func constantsFor(poly uint64) *foldConstants {
	switch poly {
	case ISO:
		return &constsISO
	case ECMA:
		return &constsECMA
	default:
		c := makeConstants(poly)
		return &c
	}
}

// update computes the CRC-64 of p starting from crc, bit-identical to
// hash/crc64.Update. It routes large inputs through the CLMUL kernel and uses
// the standard scalar code for small inputs and the sub-16-byte tail.
func update(crc uint64, tab *Table, p []byte) uint64 {
	if len(p) < minBulk || !hasKernel {
		return stdcrc64.Update(crc, tab, p)
	}
	poly := polyOf(tab)
	c := constantsFor(poly)

	// Number of whole 16-byte blocks the kernel will consume.
	blocks := len(p) / 16
	bulkLen := blocks * 16

	// The kernel XORs init into the low word of the first 16-byte block. crc is
	// the finalized value; hash/crc64's internal running state is ^crc.
	init := ^crc

	hi, lo := foldKernel(p[:bulkLen], init, c)

	folded := reduce128(hi, lo, poly)
	res := ^folded

	if tail := p[bulkLen:]; len(tail) > 0 {
		res = stdcrc64.Update(res, tab, tail)
	}
	return res
}
