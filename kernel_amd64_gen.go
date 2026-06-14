//go:build ignore

// Command gen produces kernel_amd64.s with go-asmgen: the PCLMULQDQ folding
// kernel for CRC-64.
//
// foldKernel(p []byte, init uint64, c *foldConstants) (hi, lo uint64) folds p
// (a whole number of 16-byte blocks, len(p) >= 64) into a single reflected
// 128-bit accumulator. It mirrors foldKernelGo exactly: the accumulator XMM
// holds [hi64 | lo64] where lo64 = data bytes 0..7 (loaded little-endian, which
// matches the reflected bit order), and each 16-byte step computes
//
//	acc = clmul(acc.lo, stepLo) ^ clmul(acc.hi, stepHi) ^ next16
//
// via PCLMULQDQ $0x00 (lo*lo) and $0x11 (hi*hi) against the constant register
// Kstep = [stepHi | stepLo]. The constants live in *foldConstants as a flat
// uint64 array; kStepLo and kStepHi are adjacent (offsets 0,1) so a single MOVOU
// loads the pair in the right lane order.
//
// This is a single-lane loop: simple and provably correct on every x86-64 CPU
// with PCLMULQDQ.
//
// Run: go run kernel_amd64_gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// constant array offsets (must match update.go): kStepLo=0, kStepHi=1.
const kStepLo = 0

func main() {
	sig := abi.LayoutArgs(
		[]abi.Arg{
			abi.Slice("p"),
			abi.Scalar("init", abi.Uint64),
			abi.Scalar("c", abi.Uint64), // *foldConstants pointer
		},
		[]abi.Arg{
			abi.Scalar("hi", abi.Uint64),
			abi.Scalar("lo", abi.Uint64),
		},
	)

	b := amd64.NewFunc("foldKernel", sig, 0)
	b.LoadArg("p_base", "SI").
		LoadArg("p_len", "CX").
		LoadArg("init", "DX").
		LoadArg("c", "R8").
		// X0 = first 16 bytes; merge init into the low 64 bits.
		Raw("MOVOU (SI), X0").
		Raw("MOVQ DX, X1").
		Raw("PXOR X1, X0").
		Raw("ADDQ $16, SI").
		Raw("SUBQ $16, CX").
		// Kstep = [stepHi | stepLo] (adjacent array entries).
		Raw("MOVOU %d(R8), X2", kStepLo*8).
		Label("loop").
		Raw("CMPQ CX, $16").
		Raw("JB done").
		Raw("MOVOA X0, X3").            // copy accumulator
		Raw("PCLMULQDQ $0x00, X2, X0"). // X0 = acc.lo * stepLo
		Raw("PCLMULQDQ $0x11, X2, X3"). // X3 = acc.hi * stepHi
		Raw("PXOR X3, X0").
		Raw("MOVOU (SI), X4"). // next 16 bytes
		Raw("PXOR X4, X0").
		Raw("ADDQ $16, SI").
		Raw("SUBQ $16, CX").
		Raw("JMP loop").
		Label("done").
		// Store accumulator: lo = X0[0], hi = X0[1].
		Raw("MOVQ X0, AX").
		Raw("PEXTRQ $1, X0, BX").
		StoreRet("BX", "hi").
		StoreRet("AX", "lo").
		Ret()

	f := emit.NewFile("amd64")
	f.Add(b.Func())
	if err := os.WriteFile("kernel_amd64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote kernel_amd64.s")
}
