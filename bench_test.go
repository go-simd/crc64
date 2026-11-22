package crc64

import (
	stdcrc64 "hash/crc64"
	"testing"
)

func benchData(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*131 + 7)
	}
	return b
}

// benchSizes include the short inputs (1, 16, 64, 256 B) where the fixed cost
// of the kernel dominates: go-simd/crc64#20.
var benchSizes = []int{1, 16, 64, 256, 1024, 4096, 16384, 1 << 20}

func BenchmarkChecksum(b *testing.B) {
	tab := MakeTable(ECMA)
	for _, n := range benchSizes {
		data := benchData(n)
		b.Run(sizeName(n), func(b *testing.B) {
			b.SetBytes(int64(n))
			for i := 0; i < b.N; i++ {
				_ = Checksum(data, tab)
			}
		})
	}
}

func BenchmarkStdlibChecksum(b *testing.B) {
	tab := stdcrc64.MakeTable(stdcrc64.ECMA)
	for _, n := range benchSizes {
		data := benchData(n)
		b.Run(sizeName(n), func(b *testing.B) {
			b.SetBytes(int64(n))
			for i := 0; i < b.N; i++ {
				_ = stdcrc64.Checksum(data, tab)
			}
		})
	}
}

func sizeName(n int) string {
	switch {
	case n >= 1<<20:
		return itoa(n>>20) + "M"
	case n >= 1024:
		return itoa(n/1024) + "K"
	default:
		return itoa(n)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// BenchmarkVs runs this package and hash/crc64 back to back at every size, so
// one -count=N run interleaves the two (go-simd/crc64#20).
func BenchmarkVs(b *testing.B) {
	tab := MakeTable(ECMA)
	std := stdcrc64.MakeTable(stdcrc64.ECMA)
	for _, n := range append(append([]int{}, benchSizes...), 10<<20) {
		data := benchData(n)
		b.Run(sizeName(n)+"/simd", func(b *testing.B) {
			b.SetBytes(int64(n))
			for i := 0; i < b.N; i++ {
				_ = Checksum(data, tab)
			}
		})
		b.Run(sizeName(n)+"/std", func(b *testing.B) {
			b.SetBytes(int64(n))
			for i := 0; i < b.N; i++ {
				_ = stdcrc64.Checksum(data, std)
			}
		})
	}
}
