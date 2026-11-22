# Performance parity — go-simd/crc64 vs stdlib

**Reference:** `hash/crc64` (Go stdlib — the slicing-by-8 table CRC). go-simd/crc64
runs a carry-less-multiply (PCLMULQDQ on amd64 / PMULL on arm64, plus
ppc64le/s390x) folding kernel from 32 B (amd64, arm64) and an own slicing-by-8
table path below that. ECMA polynomial, inputs 1 B … 10 MiB, single core.
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

## Notes
* ppc64le and s390x keep their previous `minBulk` (512, 4096 on pre-POWER9
  ppc64le): not measured here. They benefit from the faster reduction and the
  slicing-by-8 path below it, but their crossover is unmeasured.
* The local macOS arm64 machine was loaded (1-min load far above 8) when the
  final numbers were taken, so the table above comes from the CI runners.
