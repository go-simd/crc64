package crc64

import (
	"bytes"
	"encoding"
	stdcrc64 "hash/crc64"
	"math/rand"
	"testing"
)

var polys = []struct {
	name string
	poly uint64
}{
	{"ISO", ISO},
	{"ECMA", ECMA},
	{"custom", 0x95AC9329AC4BC9B5}, // CRC-64/XZ-ish custom reflected poly
}

// TestChecksum cross-checks Checksum against the standard library across a wide
// range of lengths (covering the <16 tail, the 16-byte step, the 64-byte
// 4-lane loop, and every off-by-one around block boundaries) for several
// polynomials.
func TestChecksum(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, p := range polys {
		tab := MakeTable(p.poly)
		std := stdcrc64.MakeTable(p.poly)
		for n := 0; n <= 600; n++ {
			data := make([]byte, n)
			rng.Read(data)
			got := Checksum(data, tab)
			want := stdcrc64.Checksum(data, std)
			if got != want {
				t.Fatalf("%s n=%d: got %016x want %016x", p.name, n, got, want)
			}
		}
		// A few large random sizes.
		for _, n := range []int{1024, 4096, 65537, 1<<16 + 13} {
			data := make([]byte, n)
			rng.Read(data)
			if got, want := Checksum(data, tab), stdcrc64.Checksum(data, std); got != want {
				t.Fatalf("%s n=%d: got %016x want %016x", p.name, n, got, want)
			}
		}
	}
}

// TestUpdate exercises incremental updates (the New/Write path) and confirms
// they match the stdlib for chunked input.
func TestUpdate(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for _, p := range polys {
		tab := MakeTable(p.poly)
		std := stdcrc64.MakeTable(p.poly)
		data := make([]byte, 5000)
		rng.Read(data)
		// Update in irregular chunks.
		var crc uint64
		i := 0
		for i < len(data) {
			step := 1 + rng.Intn(257)
			if i+step > len(data) {
				step = len(data) - i
			}
			crc = Update(crc, tab, data[i:i+step])
			i += step
		}
		if want := stdcrc64.Checksum(data, std); crc != want {
			t.Fatalf("%s chunked Update: got %016x want %016x", p.name, crc, want)
		}
	}
}

func TestNewHash(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	tab := MakeTable(ECMA)
	std := stdcrc64.MakeTable(ECMA)
	data := make([]byte, 3000)
	rng.Read(data)

	h := New(tab)
	if h.Size() != Size {
		t.Fatalf("Size = %d, want %d", h.Size(), Size)
	}
	if h.BlockSize() != 1 {
		t.Fatalf("BlockSize = %d, want 1", h.BlockSize())
	}
	h.Write(data[:1500])
	h.Write(data[1500:])
	if got, want := h.Sum64(), stdcrc64.Checksum(data, std); got != want {
		t.Fatalf("Sum64 = %016x, want %016x", got, want)
	}

	// Sum appends big-endian.
	sum := h.Sum(nil)
	if len(sum) != 8 {
		t.Fatalf("Sum len = %d, want 8", len(sum))
	}
	var rebuilt uint64
	for _, b := range sum {
		rebuilt = rebuilt<<8 | uint64(b)
	}
	if rebuilt != h.Sum64() {
		t.Fatalf("Sum bytes %x decode to %016x, want %016x", sum, rebuilt, h.Sum64())
	}

	// Sum(prefix) preserves the prefix.
	pre := []byte("pre")
	s2 := h.Sum(pre)
	if !bytes.Equal(s2[:3], pre) || len(s2) != 11 {
		t.Fatalf("Sum(prefix) = %x", s2)
	}

	h.Reset()
	if h.Sum64() != 0 {
		t.Fatalf("after Reset Sum64 = %016x, want 0", h.Sum64())
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	tab := MakeTable(ISO)
	h := New(tab)
	h.Write([]byte("the quick brown fox jumps over the lazy dog, repeatedly!!"))
	want := h.Sum64()

	m := h.(encoding.BinaryMarshaler)
	state, err := m.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if len(state) != marshaledSize {
		t.Fatalf("marshaled size = %d, want %d", len(state), marshaledSize)
	}

	h2 := New(tab)
	u := h2.(encoding.BinaryUnmarshaler)
	if err := u.UnmarshalBinary(state); err != nil {
		t.Fatal(err)
	}
	if h2.Sum64() != want {
		t.Fatalf("after unmarshal Sum64 = %016x, want %016x", h2.Sum64(), want)
	}

	// AppendBinary produces the same bytes.
	ab := h.(interface{ AppendBinary([]byte) ([]byte, error) })
	appended, err := ab.AppendBinary([]byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(appended[1:], state) {
		t.Fatalf("AppendBinary mismatch")
	}
}

func TestUnmarshalErrors(t *testing.T) {
	tab := MakeTable(ISO)
	u := New(tab).(encoding.BinaryUnmarshaler)

	if err := u.UnmarshalBinary([]byte("xx")); err != errInvalidIdentifier {
		t.Fatalf("short/bad magic: got %v", err)
	}
	if err := u.UnmarshalBinary([]byte(magic + "short")); err != errInvalidSize {
		t.Fatalf("bad size: got %v", err)
	}
	// Correct size & magic but wrong table fingerprint.
	good, _ := New(MakeTable(ECMA)).(encoding.BinaryMarshaler).MarshalBinary()
	if err := u.UnmarshalBinary(good); err != errTablesMismatch {
		t.Fatalf("table mismatch: got %v", err)
	}
}

// TestGoKernelReference exercises the portable Go fold (foldKernelGo, the shared
// constant derivation and the 128->64 reduction) directly on every arch,
// including those that route update() to the stdlib scalar path (riscv64,
// loong64) and so never reach the fold during normal operation. It reproduces a
// full checksum from the Go kernel and checks it against the standard library.
func TestGoKernelReference(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, p := range polys {
		c := makeConstants(p.poly)
		poly := polyOf(MakeTable(p.poly))
		std := stdcrc64.MakeTable(p.poly)
		for _, n := range []int{16, 32, 64, 80, 127, 128, 256, 4096} {
			data := make([]byte, n)
			rng.Read(data)
			blocks := n / 16
			bulk := blocks * 16
			hi, lo := foldKernelGo(data[:bulk], ^uint64(0), &c)
			res := ^reduce128(hi, lo, poly)
			if tail := data[bulk:]; len(tail) > 0 {
				res = stdcrc64.Update(res, std, tail)
			}
			if want := stdcrc64.Checksum(data, std); res != want {
				t.Fatalf("%s n=%d: go-kernel %016x want %016x", p.name, n, res, want)
			}
		}
	}
}

func TestTableSum(t *testing.T) {
	// tableSum of a nil table is the ISO checksum of an empty message.
	if got := tableSum(nil); got != stdcrc64.Checksum(nil, stdcrc64.MakeTable(ISO)) {
		t.Fatalf("tableSum(nil) = %016x", got)
	}
}

// FuzzChecksum is the authoritative correctness gate: for arbitrary inputs the
// SIMD checksum must equal hash/crc64 for both the ISO and ECMA polynomials.
func FuzzChecksum(f *testing.F) {
	for _, seed := range [][]byte{
		nil, {}, []byte("a"), []byte("123456789"),
		bytes.Repeat([]byte("z"), 63), bytes.Repeat([]byte("z"), 64),
		bytes.Repeat([]byte("Q"), 65), bytes.Repeat([]byte{0xff}, 4096),
	} {
		f.Add(seed)
	}
	isoT, ecmaT := MakeTable(ISO), MakeTable(ECMA)
	isoS, ecmaS := stdcrc64.MakeTable(ISO), stdcrc64.MakeTable(ECMA)
	f.Fuzz(func(t *testing.T, data []byte) {
		if got, want := Checksum(data, isoT), stdcrc64.Checksum(data, isoS); got != want {
			t.Fatalf("ISO len=%d: got %016x want %016x", len(data), got, want)
		}
		if got, want := Checksum(data, ecmaT), stdcrc64.Checksum(data, ecmaS); got != want {
			t.Fatalf("ECMA len=%d: got %016x want %016x", len(data), got, want)
		}
	})
}
