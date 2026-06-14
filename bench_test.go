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

func BenchmarkChecksum(b *testing.B) {
	tab := MakeTable(ECMA)
	for _, n := range []int{64, 256, 1024, 16384, 1 << 20} {
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
	for _, n := range []int{64, 256, 1024, 16384, 1 << 20} {
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
		return "1M"
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
