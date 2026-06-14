//go:build ignore

// Command gen produces kernel_arm64.s with go-asmgen: the PMULL folding kernel
// for CRC-64.
//
// foldKernel(p []byte, init uint64, c *foldConstants) (hi, lo uint64) folds
// p (a whole number of 16-byte blocks, len(p) >= 64) into a single reflected
// 128-bit accumulator, mirroring foldKernelGo. The accumulator vector V0 holds
// [D1=hi | D0=lo] where lo = data bytes 0..7 (loaded little-endian = reflected
// bit order). Each 16-byte step computes
//
//	acc = clmul(acc.lo, stepLo) ^ clmul(acc.hi, stepHi) ^ next16
//
// with VPMULL (low doublewords -> acc.lo*stepLo) and VPMULL2 (high doublewords
// -> acc.hi*stepHi). The constant vector V2 holds [D1=stepHi | D0=stepLo],
// loaded from *foldConstants (kStepLo at offset 0, kStepHi at offset 8) with a
// single VLD1 .D2.
//
// Run: go run kernel_arm64_gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
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

	b := arm64.NewFunc("foldKernel", sig, 0)
	b.LoadArg("p_base", "R0").
		LoadArg("p_len", "R1").
		LoadArg("init", "R2").
		LoadArg("c", "R3").
		// V0 = first 16 bytes; merge init into the low doubleword. V1 must be
		// cleared first because VMOV Rn,Vd.D[0] writes only lane 0 and leaves
		// lane 1 (the high doubleword) holding stale data.
		Raw("VLD1 (R0), [V0.B16]").
		Raw("VEOR V1.B16, V1.B16, V1.B16").
		Raw("VMOV R2, V1.D[0]").
		Raw("VEOR V1.B16, V0.B16, V0.B16").
		Raw("ADD $16, R0, R0").
		Raw("SUB $16, R1, R1").
		// V2 = [stepHi | stepLo].
		Raw("VLD1 (R3), [V2.D2]").
		Label("loop").
		Raw("CMP $16, R1").
		Raw("BLT done").
		Raw("VPMULL V0.D1, V2.D1, V3.Q1").  // V3 = acc.lo * stepLo
		Raw("VPMULL2 V0.D2, V2.D2, V4.Q1"). // V4 = acc.hi * stepHi
		Raw("VEOR V4.B16, V3.B16, V0.B16"). // V0 = V3 ^ V4
		Raw("VLD1 (R0), [V5.B16]").         // next 16 bytes
		Raw("VEOR V5.B16, V0.B16, V0.B16"). // V0 ^= next
		Raw("ADD $16, R0, R0").
		Raw("SUB $16, R1, R1").
		Raw("B loop").
		Label("done").
		Raw("VMOV V0.D[0], R4"). // lo
		Raw("VMOV V0.D[1], R5"). // hi
		StoreRet("R5", "hi").
		StoreRet("R4", "lo").
		Ret()

	f := emit.NewFile("arm64")
	f.Add(b.Func())
	if err := os.WriteFile("kernel_arm64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote kernel_arm64.s")
}
