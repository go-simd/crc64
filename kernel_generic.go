//go:build !amd64 && !arm64 && !ppc64le && !s390x

package crc64

// On targets without a dedicated CLMUL kernel (riscv64 — Go does not yet expose
// the Zbc carryless-multiply instructions — and loong64, plus any other GOARCH)
// hasKernel is false: update() defers entirely to the stdlib scalar path, which
// is faster than a software-emulated carryless multiply on these CPUs. foldKernel
// still exists so update() type-checks, and forwards to the portable Go fold so
// that flipping hasKernel on Just Works (used by the dispatch test to drive the
// fold path on these arches). It is a var, not a const, for exactly that reason.
var hasKernel = false

func foldKernel(p []byte, init uint64, c *foldConstants) (hi, lo uint64) {
	return foldKernelGo(p, init, c)
}
