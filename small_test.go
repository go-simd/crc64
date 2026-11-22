package crc64

import (
	stdcrc64 "hash/crc64"
	"math/rand"
	"testing"
)

// TestReduceTableMatchesBitSerial pins reduceTable to the bit-serial oracle.
func TestReduceTableMatchesBitSerial(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	for _, p := range polys {
		tab := MakeTable(p.poly)
		sl := slicingFor(tab)
		if (p.name == "custom") != (sl == nil) {
			t.Fatalf("%s: slicingFor nil=%v", p.name, sl == nil)
		}
		for i := 0; i < 2000; i++ {
			hi, lo := rng.Uint64(), rng.Uint64()
			if i == 0 {
				hi, lo = 0, 0
			}
			want := reduce128(hi, lo, p.poly)
			if got := reduceTable(hi, lo, tab, sl); got != want {
				t.Fatalf("%s hi=%016x lo=%016x: got %016x want %016x", p.name, hi, lo, got, want)
			}
			if sl != nil {
				if got := reduceTable(hi, lo, tab, nil); got != want {
					t.Fatalf("%s byte-wise hi=%016x lo=%016x: got %016x want %016x", p.name, hi, lo, got, want)
				}
			}
		}
	}
}

// TestSmallInputsExact checks every length 0..300 with random start values and
// misaligned slices, on the small path and on the kernel path, against the
// stdlib oracle.
func TestSmallInputsExact(t *testing.T) {
	saved := minBulk
	defer func() { minBulk = saved }()
	rng := rand.New(rand.NewSource(22))
	for _, mb := range []int{saved, 16, 1 << 30} {
		minBulk = mb
		for _, p := range polys {
			tab := MakeTable(p.poly)
			buf := make([]byte, 320)
			rng.Read(buf)
			for n := 0; n <= 300; n++ {
				for off := 0; off < 3; off++ {
					data := buf[off : off+n]
					crc := rng.Uint64()
					if got, want := Update(crc, tab, data), stdcrc64.Update(crc, tab, data); got != want {
						t.Fatalf("minBulk=%d %s n=%d off=%d: got %016x want %016x", mb, p.name, n, off, got, want)
					}
				}
			}
		}
	}
}
