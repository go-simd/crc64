//go:build amd64

package crc64

import (
	stdcrc64 "hash/crc64"
	"math/rand"
	"testing"

	"golang.org/x/sys/cpu"
)

// TestDispatchAMD64 drives both update() branches — the PCLMULQDQ kernel and the
// stdlib scalar fallback — regardless of what the host CPU has. The fallback is
// always safe to force; the kernel runs only when the CPU actually has
// PCLMULQDQ (forcing it on a CPU without it would #UD). The native amd64 CI
// runner has PCLMULQDQ, so both branches are covered there.
func TestDispatchAMD64(t *testing.T) {
	saved := hasKernel
	defer func() { hasKernel = saved }()

	rng := rand.New(rand.NewSource(11))
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

	if cpu.X86.HasPCLMULQDQ {
		hasKernel = true
		check("clmul")
	} else {
		t.Log("CPU lacks PCLMULQDQ; CLMUL branch not exercised on this host")
	}
}
