# Performance parity — go-simd/crc64 vs stdlib

**Reference:** `hash/crc64` (Go stdlib — the slicing-by-8 table CRC). go-simd/crc64
runs a carry-less-multiply (PCLMULQDQ on amd64 / PMULL on arm64, plus
ppc64le/s390x) folding kernel with a scalar table tail. ECMA polynomial, inputs
64 B … 1 MiB, single core. `b.SetBytes(len)` so `go test` reports MB/s.

## amd64 (AVX2/PCLMULQDQ, GitHub Actions x86_64 runner — ratios valid, absolute ns/op CI-noisy)

**Methodology.** GitHub Actions `ubuntu-latest` runner, **AMD EPYC 7763** (`avx2`
+ `pclmulqdq` present, **no `avx512*`** — confirmed from `/proc/cpuinfo`),
`GOAMD64` baseline, Go stable, single core. `-count=6`, **min-of-6**. The runner
is shared, so absolute throughput is noisy; the **ratio vs `hash/crc64`** is
measured back-to-back on the *same* CPU and is valid. Reproduce via
`gh workflow run bench-amd64.yml`.

| size | go-simd (MB/s) | stdlib `hash/crc64` | ×stdlib | verdict |
|------|---------------:|--------------------:|--------:|---------|
| 64 B   | 1367 | 1444 | 0.95× | trails stdlib (sub-fold, table wins) |
| 256 B  | 1600 | 1630 | 0.98× | ~parity |
| 1 KiB  | 2484 | 1674 | 1.48× | wins |
| 16 KiB | 6620 | 1699 | 3.90× | wins |
| 1 MiB  | 7397 | 1685 | 4.39× | **wins ~4.4×** |

* The PCLMULQDQ folding kernel **wins at scale (1.5–4.4× stdlib for ≥ 1 KiB)**,
  the margin growing as the fixed fold setup amortizes; the stdlib slicing-by-8
  table stays flat at ~1.7 GB/s.
* **Honest finding (amd64):** at **64–256 B the SIMD path is ~0.95–0.98× stdlib**
  — below the fold width the carry-less-multiply setup costs more than the
  table lookup, so the scalar table CRC ties or marginally wins. Expected for
  sub-fold inputs; the kernel dispatches to the table tail there anyway.

### Notes
* Checksums are bit-exact to `hash/crc64` (ECMA) on every input (100% coverage,
  fuzz-clean).
* arm64 (M4 Max PMULL) numbers are not yet captured in this file; the amd64
  column above is the GitHub Actions measurement. Different hardware/ISA rows are
  not directly comparable in absolute terms.
