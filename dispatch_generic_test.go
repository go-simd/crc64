//go:build !amd64 && !arm64 && !ppc64le && !s390x

package crc64

import (
	stdcrc64 "hash/crc64"
	"math/rand"
	"testing"
)

// TestDispatchGeneric covers the generic foldKernel forwarder on arches without
// a dedicated CLMUL kernel (riscv64, loong64). update() does not call it during
// normal operation there (hasKernel is false, so the stdlib scalar path is
// used), but the forwarder must still be correct, so we drive it directly and
// check it against the standard library.
func TestDispatchGeneric(t *testing.T) {
	saved := hasKernel
	defer func() { hasKernel = saved }()

	rng := rand.New(rand.NewSource(31))
	// Drive update() down both branches: the default stdlib scalar path
	// (hasKernel == false) and the Go-fold path (hasKernel forced true). Both
	// must agree with the standard library.
	for _, hk := range []bool{false, true} {
		hasKernel = hk
		for _, p := range polys {
			tab := MakeTable(p.poly)
			std := stdcrc64.MakeTable(p.poly)
			for _, n := range []int{0, 8, 16, 63, 64, 80, 127, 128, 256, 600, 1000, 4096} {
				data := make([]byte, n)
				rng.Read(data)
				if got, want := Checksum(data, tab), stdcrc64.Checksum(data, std); got != want {
					t.Fatalf("hasKernel=%v %s n=%d: got %016x want %016x", hk, p.name, n, got, want)
				}
			}
		}
	}
}
