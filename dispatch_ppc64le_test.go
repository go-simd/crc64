//go:build ppc64le

package crc64

import (
	stdcrc64 "hash/crc64"
	"math/rand"
	"testing"

	"golang.org/x/sys/cpu"
)

// TestDispatchPPC64LE drives both update() branches — the VPMSUMD kernel and the
// stdlib scalar fallback — and compares the assembly kernel against the portable
// Go reference. The VPMSUMD fold emits ISA-3.0 (POWER9) instructions (MTVSRDD)
// that raise SIGILL on POWER8, so the kernel-forcing branches run only when the
// host is actually POWER9+ (mirroring the amd64 force tests, which skip when the
// CPU lacks the feature). The scalar-fallback branch is always exercised. The
// power9-targeted QEMU CI job and the native POWER9/POWER10 farm runs cover the
// kernel branch.
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

	// VPMSUMD kernel: only force it on when the CPU is POWER9+, otherwise the
	// MTVSRDD in foldKernel would SIGILL (e.g. on a POWER8 farm node).
	if !cpu.PPC64.IsPOWER9 {
		t.Log("CPU is pre-POWER9; VPMSUMD kernel branch not exercised on this host")
		return
	}
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
