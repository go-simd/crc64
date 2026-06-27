//go:build ignore

// Command gen produces kernel_ppc64le.s with go-asmgen: the VPMSUMD folding
// kernel for CRC-64 on POWER8+ (VSX is baseline, so no runtime feature gate).
//
// foldKernel(p []byte, init uint64, c *foldConstants) (hi, lo uint64) folds p
// (a whole number of 16-byte blocks, len(p) >= 64) into a single reflected
// 128-bit accumulator, mirroring foldKernelGo. VPMSUMD computes, in one
// instruction, clmul(a.d0,b.d0) ^ clmul(a.d1,b.d1) — exactly the per-step fold
// acc.lo*stepLo ^ acc.hi*stepHi — given a constant vector whose doublewords line
// up with the accumulator's.
//
// Byte order: rather than wrestle with VSX vector load element order on
// little-endian POWER, every 64-bit word (data, init, constants) is moved
// through general-purpose registers with plain MOVD little-endian loads (which
// give exactly le64) and assembled into vectors as [d0=Rhi, d1=Rlo]. We keep
// acc.lo in d1 and acc.hi in d0 throughout, matching the constant vector
// [d0=stepHi, d1=stepLo], so VPMSUMD's doubleword pairing is correct. Results are
// read back with MFVSRD (d0) and a half-swap + MFVSRD (d1).
//
// ISA baseline: the original kernel assembled vectors with MTVSRDD Rhi,Rlo
// (one instruction), which is ISA-3.0 (POWER9) and SIGILLs on POWER8. POWER8 is
// VSX/ISA-2.07 — it has the single-register MTVSRD plus XXPERMDI — so we build
// [d0=Rhi, d1=Rlo] in three ISA-2.07 ops instead: MTVSRD Rhi->Vt.d0,
// MTVSRD Rlo->Vscratch.d0, then XXPERMDI Vt,Vscratch,$0 (== XXMRGHD) merges the
// two high doublewords into [d0=Rhi, d1=Rlo]. The emitVSRDD helper below emits
// that sequence so the kernel runs natively on POWER8 (no POWER9 gate needed —
// VPMSUMD is ISA-2.07). Vscratch = VS37 (V5), unused elsewhere in the kernel.
//
// This mapping is confirmed by the qemu FuzzChecksum run + cfarm112 (POWER8E).
//
// Run: go run kernel_ppc64le_gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/ppc64"
)

// emitVSRDD assembles a VSR as [d0=rhi, d1=rlo] using only ISA-2.07 ops, the
// POWER8-safe replacement for the ISA-3.0 MTVSRDD rhi,rlo,vst. scratch must be a
// VSR not live across the sequence; vst and scratch must differ.
func emitVSRDD(b *ppc64.Builder, rhi, rlo, vst, scratch string) *ppc64.Builder {
	return b.
		Raw("MTVSRD %s, %s", rhi, vst).         // vst   = [d0=rhi, d1=0]
		Raw("MTVSRD %s, %s", rlo, scratch).     // scr   = [d0=rlo, d1=0]
		Raw("XXPERMDI %s, %s, $0, %s", vst, scratch, vst) // vst = [d0=rhi, d1=rlo]
}

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

	b := ppc64.NewFunc("foldKernel", sig, 0)
	b.LoadArg("p_base", "R3").
		LoadArg("p_len", "R4").
		LoadArg("init", "R5").
		LoadArg("c", "R6").
		// Constants: stepLo @0, stepHi @8 -> V3 = [d0=stepHi, d1=stepLo].
		Raw("MOVD 0(R6), R9").  // stepLo
		Raw("MOVD 8(R6), R10")  // stepHi
	emitVSRDD(b, "R10", "R9", "VS35", "VS37"). // d0=stepHi, d1=stepLo  (V3)
		// First block -> acc V0 = [d0=hi=bytes8..15, d1=lo=bytes0..7^init].
		Raw("MOVD 0(R3), R7"). // lo = le64(b[0:])
		Raw("MOVD 8(R3), R8"). // hi = le64(b[8:])
		Raw("XOR R5, R7, R7")  // lo ^= init
	emitVSRDD(b, "R8", "R7", "VS32", "VS37"). // d0=hi, d1=lo  (V0)
		Raw("ADD $16, R3, R3").
		Raw("ADD $-16, R4, R4").
		Label("loop").
		Raw("CMP R4, $16").
		Raw("BLT done").
		Raw("VPMSUMD V0, V3, V2"). // V2 = acc.lo*stepLo ^ acc.hi*stepHi
		Raw("MOVD 0(R3), R7").     // next lo
		Raw("MOVD 8(R3), R8")      // next hi
	emitVSRDD(b, "R8", "R7", "VS32", "VS37"). // V0 = next block
		Raw("XXLXOR VS32, VS34, VS32"). // V0 ^= V2
		Raw("ADD $16, R3, R3").
		Raw("ADD $-16, R4, R4").
		Raw("BR loop").
		Label("done").
		Raw("MFVSRD VS32, R7"). // d0 = hi
		Raw("XXPERMDI VS32, VS32, $2, VS33").
		Raw("MFVSRD VS33, R8"). // d1 = lo
		StoreRet("R7", "hi").
		StoreRet("R8", "lo").
		Ret()

	f := emit.NewFile("ppc64le")
	f.Add(b.Func())
	if err := os.WriteFile("kernel_ppc64le.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote kernel_ppc64le.s")
}
