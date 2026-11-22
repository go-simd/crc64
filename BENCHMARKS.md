# Performance parity — go-simd/crc64 vs stdlib

**Reference:** `hash/crc64` (Go stdlib — the slicing-by-8 table CRC). go-simd/crc64
runs a carry-less-multiply (PCLMULQDQ on amd64 / PMULL on arm64 / VPMSUMD on
ppc64le / VGFMAG on s390x) folding kernel from 32 B (amd64, arm64, s390x) or
48 B (ppc64le) and an own slicing-by-8 table path below that. ECMA polynomial, inputs 1 B … 10 MiB, single core.
Checksums are bit-exact to `hash/crc64` on every input (the stdlib is the
oracle of every test; fuzz-clean).

**Methodology.** GitHub Actions runners, shared, so absolute ns/op is noisy; the
**ratio vs `hash/crc64`** is measured back to back in the *same* job
(`BenchmarkVs` runs `simd` then `std` at every size, `-count=6`, **median of 6**)
and is valid. Reproduce with `gh workflow run bench-amd64.yml` (amd64, AVX2,
`ubuntu-latest`) and `bench-arm64.yml` (arm64, `ubuntu-24.04-arm`). Go 1.27.3.

## Result (go-simd/crc64#20): at least the stdlib at every size

| size | amd64 simd | amd64 std | amd64 × | arm64 simd | arm64 std | arm64 × |
|------|-----------:|----------:|--------:|-----------:|----------:|--------:|
| 1 B    | 3.50 ns | 6.56 ns | 1.9× | 3.24 ns | 5.65 ns | 1.7× |
| 16 B   | 8.30 ns | 22.6 ns | 2.7× | 9.27 ns | 24.4 ns | 2.6× |
| 64 B   | 14.8 ns | 42.2 ns | 2.9× | 16.1 ns | 46.9 ns | 2.9× |
| 256 B  | 37.6 ns | 154 ns | 4.1× | 40.5 ns | 153 ns | 3.8× |
| 1 KiB  | 162 ns | 599 ns | 3.7× | 139 ns | 564 ns | 4.1× |
| 4 KiB  | 661 ns | 2383 ns | 3.6× | 536 ns | 2215 ns | 4.1× |
| 16 KiB | 2.66 µs | 9.53 µs | 3.6× | 2.12 µs | 8.89 µs | 4.2× |
| 1 MiB  | 171 µs | 611 µs | 3.6× | 136 µs | 570 µs | 4.2× |
| 10 MiB | 1.77 ms | 6.17 ms | 3.5× | 1.36 ms | 5.71 ms | 4.2× |

Before (v0.3.0, issue #20, Zen 4): 64 B 0.95×, 16 B 0.97×, 256 B 1.00×.

What changed: the 128-iteration bit-serial reduction that ran on every kernel
call is now two slicing-by-8 steps (exact: the folded accumulator times x^64 mod P
is the raw table CRC of its 16 bytes); inputs below the crossover use an own
slicing-by-8 on tables built at package init (no `sync.Once`, no extra call);
and the kernel is engaged from 32 B instead of 512 B.

## Crossover (`BenchmarkCrossover`, slicing path vs kernel on the same input)

| size | amd64 slicing | amd64 kernel | arm64 slicing | arm64 kernel |
|------|--------------:|-------------:|--------------:|-------------:|
| 16 B | 8.30 ns | 10.4 ns | 9.23 ns | 12.6 ns |
| 32 B | 15.4 ns | 12.1 ns | 17.3 ns | 13.7 ns |
| 64 B | 33.6 ns | 14.8 ns | 34.3 ns | 16.1 ns |

The kernel wins from 32 B on both, hence `minBulk = 32` on amd64 and arm64. On
Apple M4 Max (PMULL, local, load about 8): 4.4 ns vs 5.7 ns at 16 B, 9.0 vs
6.2 ns at 32 B, 110.6 vs 14.8 ns at 256 B.

## ppc64le and s390x (real hardware)

Same binary and same `BenchmarkVs` (`simd`, `std`, and an A/A control `std2` that
is the same code as `std`), `-count=6 -benchtime=200ms`, median, run on idle
hosts (1-min load 0.0 to 0.1 before the run):

* s390x: IBM z17 (LinuxONE community cloud, z/VM guest, 8 vCPU, VXE2, VGFMAG kernel).
* ppc64le POWER9: GCC compile farm cfarm29 (32 cores).
* ppc64le POWER8E: GCC compile farm cfarm112 (160 shared cores, noisier: A/A up to 15%).

| size | s390x simd / std | POWER9 simd / std | POWER8E simd / std |
|------|-----------------:|------------------:|-------------------:|
| 1 B    | 3.96 / 5.28 ns (1.3×) | 8.47 / 22.9 ns (2.7×) | 17.4 / 43.2 ns (2.5×) |
| 16 B   | 11.9 / 26.9 ns (2.3×) | 14.7 / 43.7 ns (3.0×) | 23.6 / 104 ns (4.4×) |
| 64 B   | 23.5 / 50.5 ns (2.2×) | 31.0 / 157 ns (5.1×) | 62.0 / 261 ns (4.2×) |
| 256 B  | 33.2 / 150 ns (4.5×) | 56.7 / 325 ns (5.7×) | 107 / 480 ns (4.5×) |
| 1 KiB  | 77.0 / 553 ns (7.2×) | 164 / 967 ns (5.9×) | 291 / 1493 ns (5.1×) |
| 1 MiB  | 59.8 / 549 µs (9.2×) | 142 / 815 µs (5.7×) | 248 / 1330 µs (5.4×) |
| 10 MiB | 598 / 5504 µs (9.2×) | 1.43 / 8.09 ms (5.7×) | 2.64 / 13.4 ms (5.1×) |

The A/A ratio (`std` / `std2`) was within 0 to 4% on every size on s390x and
POWER9, so the gaps above are not noise. Sizes between the table rows
(`BenchmarkShort`: 2, 7, 15, 17, 24, 31, 33, 47, 49, 127 B) are also faster than
`hash/crc64` on all three: from 1.2× (7 B, s390x) to 4× (127 B, POWER9).

Before (v0.3.1), same hosts: s390x 256 B was **0.87×** (173 vs 150 ns) and 64 B
1.10×, because the kernel only started at 512 B; POWER9 256 B 1.6×; POWER8E
1 KiB 1.1× (the kernel only started at 4096 B).

Crossover (`BenchmarkCrossover`, slicing path vs kernel on the same input):

| size | s390x slicing / kernel | POWER9 slicing / kernel | POWER8E slicing / kernel |
|------|-----------------------:|------------------------:|-------------------------:|
| 16 B | 11.9 / 18.6 ns | 14.6 / 24.3 ns | 25.5 / 49.2 ns |
| 32 B | 22.0 / 19.9 ns | 22.7 / 27.1 ns | 44.3 / 54.1 ns |
| 48 B | 32.3 / 20.6 ns | 34.8 / 29.2 ns | 77.9 / 57.7 ns |
| 64 B | 42.5 / 21.8 ns | 46.9 / 31.2 ns | 97.4 / 61.7 ns |

Hence `minBulk = 32` on s390x and `minBulk = 48` on ppc64le (POWER8 and POWER9;
the old 512 B and 4096 B floors predate the table reduction, which removed the
kernel's fixed cost).

## Notes
* riscv64 and loong64 have no kernel: they always take the own slicing-by-8 path
  (and `hash/crc64` for custom polynomials), which on those targets is the same
  algorithm as the standard library.
* The local macOS arm64 machine was loaded (1-min load far above 8) when the
  final numbers were taken, so the table above comes from the CI runners.
