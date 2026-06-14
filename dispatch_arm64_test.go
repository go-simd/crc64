//go:build arm64

package crc64

import (
	stdcrc64 "hash/crc64"
	"math/rand"
	"testing"
)

// TestDispatchARM64 drives both update() branches — the PMULL kernel and the
// stdlib scalar fallback — and additionally compares the assembly kernel against
// the portable Go reference. The PMULL instruction is always decodable on the
// arm64 targets this package runs on; cpu.ARM64.HasPMULL is merely unset on
// darwin (the cpu package reads Linux HWCAP), so we force the kernel on to
// exercise the real assembly on every host, including the macOS dev machine.
func TestDispatchARM64(t *testing.T) {
	saved := hasKernel
	defer func() { hasKernel = saved }()

	rng := rand.New(rand.NewSource(12))
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

	hasKernel = false
	check("fallback")
	hasKernel = true
	check("pmull")

	// Compare the assembly kernel directly against the Go reference.
	rng2 := rand.New(rand.NewSource(13))
	for _, p := range polys {
		c := makeConstants(p.poly)
		for _, n := range []int{16, 64, 80, 256, 4096} {
			data := make([]byte, n)
			rng2.Read(data)
			init := rng2.Uint64()
			gh, gl := foldKernelGo(data, init, &c)
			kh, kl := foldKernel(data, init, &c)
			if gh != kh || gl != kl {
				t.Fatalf("%s n=%d: pmull=(%016x,%016x) go=(%016x,%016x)", p.name, n, kh, kl, gh, gl)
			}
		}
	}
}
