//go:build ppc64le

package crc64

import (
	stdcrc64 "hash/crc64"
	"math/rand"
	"testing"
)

// TestDispatchPPC64LE drives both update() branches — the VPMSUMD kernel and the
// stdlib scalar fallback — and compares the assembly kernel against the portable
// Go reference. The VPMSUMD fold is now built entirely from ISA-2.07 ops
// (VPMSUMD + MTVSRD/MFVSRD/XXPERMDI), the ppc64le/POWER8 baseline, so the kernel
// runs on every ppc64le host (POWER8 included) and both branches are exercised
// unconditionally here. The QEMU power8 and power9 CI jobs plus the native
// POWER8E/POWER9 farm runs all cover the kernel branch.
func TestDispatchPPC64LE(t *testing.T) {
	saved := hasKernel
	defer func() { hasKernel = saved }()

	rng := rand.New(rand.NewSource(21))
	check := func(label string) {
		for _, p := range polys {
			tab := MakeTable(p.poly)
			std := stdcrc64.MakeTable(p.poly)
			for _, n := range []int{64, 65, 79, 80, 127, 128, 256, 600, 1000, 4096} {
				data := make([]byte, n)
				rng.Read(data)
				if got, want := Checksum(data, tab), stdcrc64.Checksum(data, std); got != want {
					t.Fatalf("%s %s n=%d: got %016x want %016x", label, p.name, n, got, want)
				}
			}
		}
	}

	// Scalar fallback: always safe, exercised on every ppc64le host.
	hasKernel = false
	check("fallback")

	// VPMSUMD kernel: ISA-2.07 baseline, runs on every ppc64le host (POWER8+),
	// so force it on unconditionally.
	hasKernel = true
	check("vpmsumd")

	rng2 := rand.New(rand.NewSource(23))
	for _, p := range polys {
		c := makeConstants(p.poly)
		for _, n := range []int{16, 64, 80, 256, 4096} {
			data := make([]byte, n)
			rng2.Read(data)
			init := rng2.Uint64()
			gh, gl := foldKernelGo(data, init, &c)
			kh, kl := foldKernel(data, init, &c)
			if gh != kh || gl != kl {
				t.Fatalf("%s n=%d: vpmsumd=(%016x,%016x) go=(%016x,%016x)", p.name, n, kh, kl, gh, gl)
			}
		}
	}
}

// TestPPCMinBulk covers both branches of the per-CPU kernel-engagement floor on
// any host (the POWER9-emulated CI lane only ever exercises one branch of init).
func TestPPCMinBulk(t *testing.T) {
	if got := ppcMinBulk(true); got != 512 {
		t.Fatalf("ppcMinBulk(POWER9) = %d, want 512", got)
	}
	if got := ppcMinBulk(false); got != 4096 {
		t.Fatalf("ppcMinBulk(POWER8) = %d, want 4096", got)
	}
}
