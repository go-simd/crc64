//go:build ignore

// Command gen produces kernel_s390x.s with go-asmgen: the VGFMAG folding kernel
// for CRC-64 on z13+ (the vector facility is gated at runtime on
// cpu.S390X.HasVX).
//
// foldKernel(p []byte, init uint64, c *foldConstants) (hi, lo uint64) folds p
// (a whole number of 16-byte blocks, len(p) >= 64) into a single reflected
// 128-bit accumulator, mirroring foldKernelGo. VGFMAG is a Galois-field
// multiply-and-accumulate over doublewords: VGFMAG K, A, B, A computes
//
//	A = gfmul(K.d0, A.d0) ^ gfmul(K.d1, A.d1) ^ B
//
// i.e. exactly one fold step acc = acc.lo*stepLo ^ acc.hi*stepHi ^ next.
//
// BIG-ENDIAN: s390x is big-endian; the reflected CRC needs little-endian words.
// Rather than byte-reverse whole vectors with VPERM (which would also reverse
// bytes *inside* each doubleword and corrupt the GF multiply), each 64-bit word
// is loaded little-endian into a GPR with MOVDBR (load byte-reversed = le64) and
// assembled into a vector with VLVGP Rd0,Rd1. We keep acc.hi in element 0 (d0)
// and acc.lo in element 1 (d1), and the constant vector as [d0=stepHi,
// d1=stepLo], so VGFMAG's doubleword pairing matches the Go reference. Results
// are read back with VLGVG. This layout is confirmed by the qemu FuzzChecksum
// run.
//
// Run: go run kernel_s390x_gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/s390x"
)

func main() {
	sig := abi.LayoutArgs(
		[]abi.Arg{
			abi.Slice("p"),
			abi.Scalar("init", abi.Uint64),
			abi.Scalar("c", abi.Uint64),
		},
		[]abi.Arg{
			abi.Scalar("hi", abi.Uint64),
			abi.Scalar("lo", abi.Uint64),
		},
	)

	b := s390x.NewFunc("foldKernel", sig, 0)
	b.LoadArg("p_base", "R1").
		LoadArg("p_len", "R2").
		LoadArg("init", "R3").
		LoadArg("c", "R4").
		// Constants live in Go memory as native (big-endian on s390x) uint64s, so
		// they load with a plain MOVD (NOT the byte-reversed MOVDBR used for the
		// raw data bytes). stepLo@0, stepHi@8 -> V3 = [d0=stepHi, d1=stepLo].
		Raw("MOVD (R4), R6").    // stepLo
		Raw("MOVD 8(R4), R8").   // stepHi
		Raw("VLVGP R8, R6, V3"). // d0=stepHi, d1=stepLo
		// First block -> acc V0 = [d0=hi, d1=lo^init].
		Raw("MOVDBR (R1), R9").   // lo = le64(b[0:])
		Raw("MOVDBR 8(R1), R10"). // hi = le64(b[8:])
		Raw("XOR R3, R9").        // lo ^= init
		Raw("VLVGP R10, R9, V0"). // d0=hi, d1=lo
		Raw("ADD $16, R1").
		Raw("ADD $-16, R2").
		Label("loop").
		Raw("CMPBLT R2, $16, done").
		Raw("MOVDBR (R1), R9").
		Raw("MOVDBR 8(R1), R10").
		Raw("VLVGP R10, R9, V2").     // next block
		Raw("VGFMAG V3, V0, V2, V0"). // V0 = gfmul(V3,V0) ^ V2
		Raw("ADD $16, R1").
		Raw("ADD $-16, R2").
		Raw("BR loop").
		Label("done").
		Raw("VLGVG $0, V0, R11"). // hi = d0
		Raw("VLGVG $1, V0, R12"). // lo = d1
		StoreRet("R11", "hi").
		StoreRet("R12", "lo").
		Ret()

	f := emit.NewFile("s390x")
	f.Add(b.Func())
	if err := os.WriteFile("kernel_s390x.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote kernel_s390x.s")
}
