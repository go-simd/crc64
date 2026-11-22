package crc64

import "testing"

// BenchmarkCrossover runs the same input through the small (slicing-by-8) path
// and the fold kernel, to place minBulk (go-simd/crc64#20). Run on the target
// hardware; the kernel row is skipped where no kernel exists.
func BenchmarkCrossover(b *testing.B) {
	tab := MakeTable(ECMA)
	saved := minBulk
	defer func() { minBulk = saved }()
	for _, n := range []int{16, 32, 48, 64, 96, 128, 192, 256, 384, 512} {
		data := benchData(n)
		for _, path := range []string{"slicing", "kernel"} {
			if path == "kernel" && !hasKernel {
				continue
			}
			b.Run(path+"/"+sizeName(n), func(b *testing.B) {
				if path == "kernel" {
					minBulk = 16
				} else {
					minBulk = 1 << 30
				}
				b.SetBytes(int64(n))
				for i := 0; i < b.N; i++ {
					_ = Checksum(data, tab)
				}
			})
		}
	}
}
