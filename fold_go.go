package crc64

// clmul64 returns the 128-bit carryless product of a and b in normal bit order.
func clmul64(a, b uint64) (hi, lo uint64) {
	for i := 0; i < 64; i++ {
		if (b>>uint(i))&1 == 1 {
			if i == 0 {
				lo ^= a
			} else {
				lo ^= a << uint(i)
				hi ^= a >> uint(64-i)
			}
		}
	}
	return
}

// foldKernelGo is the portable reference fold and the exact specification the
// per-arch assembly must reproduce, bit for bit. It consumes p (a whole number
// of 16-byte blocks, len(p) >= 16) and returns the folded reflected 128-bit
// accumulator (hi:lo). init (= ^crc) is merged into the low word of the first
// block.
//
// A single 128-bit accumulator is folded forward one 16-byte block at a time
// using PLAIN carryless multiplication (exactly what PCLMULQDQ / PMULL /
// VPMSUMD / VGFMG compute in hardware on the little-endian-loaded words):
//
//	acc = clmul(acc.lo, stepLo) ^ clmul(acc.hi, stepHi) ^ next16
//
// where stepLo = reflect(x^191 mod P) and stepHi = reflect(x^127 mod P). The
// "off-by-one" exponents (191/127 instead of 192/128) are the standard
// reflected-CLMUL shift correction: the carryless product of two reflected
// 64-bit values is a 127-bit reflected value, one bit short of the 128-bit
// position, so the fold constant is x^(n-1). This single-lane form keeps every
// architecture's kernel identical in structure and trivially comparable to this
// reference; the carryless-multiply unit folds 16 bytes per few cycles, far
// faster than the scalar table.
func foldKernelGo(p []byte, init uint64, c *foldConstants) (hi, lo uint64) {
	lo = le64(p[0:]) ^ init
	hi = le64(p[8:])
	p = p[16:]
	for len(p) >= 16 {
		ah, al := clmul64(lo, c[kStepLo]) // acc.lo * reflect(x^191)
		bh, bl := clmul64(hi, c[kStepHi]) // acc.hi * reflect(x^127)
		hi = ah ^ bh ^ le64(p[8:])
		lo = al ^ bl ^ le64(p[0:])
		p = p[16:]
	}
	return hi, lo
}

func le64(b []byte) uint64 {
	_ = b[7]
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}
