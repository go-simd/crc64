//go:build s390x

package crc64

import (
	stdcrc64 "hash/crc64"
	"math/rand"
	"testing"

	"golang.org/x/sys/cpu"
)

// TestDispatchS390X drives both update() branches — the VGFMAG kernel and the
// stdlib scalar fallback — and compares the assembly kernel against the portable
// Go reference. The vector path runs whenever the CPU has the vector facility;
// the fallback is forced here for branch coverage.
func TestDispatchS390X(t *testing.T) {
	saved := hasKernel
	defer func() { hasKernel = saved }()

	rng := rand.New(rand.NewSource(22))
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

	if cpu.S390X.HasVX {
		hasKernel = true
		check("vgfmag")

		rng2 := rand.New(rand.NewSource(24))
		for _, p := range polys {
			c := makeConstants(p.poly)
			for _, n := range []int{16, 64, 80, 256, 4096} {
				data := make([]byte, n)
				rng2.Read(data)
				init := rng2.Uint64()
				gh, gl := foldKernelGo(data, init, &c)
				kh, kl := foldKernel(data, init, &c)
				if gh != kh || gl != kl {
					t.Fatalf("%s n=%d: vgfmag=(%016x,%016x) go=(%016x,%016x)", p.name, n, kh, kl, gh, gl)
				}
			}
		}
	} else {
		t.Log("CPU lacks the vector facility; VGFMAG branch not exercised on this host")
	}
}
