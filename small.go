package crc64

import stdcrc64 "hash/crc64"

// slicing8 is the slicing-by-8 table set of one polynomial: t[0] is the plain
// byte table, t[k][i] is t[0][i] advanced by k further zero bytes.
type slicing8 [8]Table

func makeSlicing8(tab *Table) *slicing8 {
	var t slicing8
	t[0] = *tab
	for i := 0; i < 256; i++ {
		crc := tab[i]
		for j := 1; j < 8; j++ {
			crc = tab[byte(crc)] ^ (crc >> 8)
			t[j][i] = crc
		}
	}
	return &t
}

// The predefined polynomials get their slicing tables at package init (about
// 4 us), so the small-input path has no sync.Once load and no lookup: the table
// is selected by comparing the caller's *Table with the stdlib's cached ISO and
// ECMA tables (hash/crc64.MakeTable returns the same pointer every call).
var (
	tabISO    = stdcrc64.MakeTable(ISO)
	tabECMA   = stdcrc64.MakeTable(ECMA)
	sliceISO  = makeSlicing8(tabISO)
	sliceECMA = makeSlicing8(tabECMA)
)

// slicingFor returns the slicing tables when tab is the predefined ISO or ECMA
// table, nil for a custom polynomial.
func slicingFor(tab *Table) *slicing8 {
	switch tab {
	case tabECMA:
		return sliceECMA
	case tabISO:
		return sliceISO
	}
	return nil
}

// updateSlicing8 is hash/crc64's slicing-by-8 update, on tables that need no
// once-guard. crc is the finalized value (the running state is ^crc).
func updateSlicing8(crc uint64, t *slicing8, p []byte) uint64 {
	crc = ^crc
	for len(p) >= 8 {
		crc ^= le64(p)
		crc = t[7][byte(crc)] ^ t[6][byte(crc>>8)] ^ t[5][byte(crc>>16)] ^
			t[4][byte(crc>>24)] ^ t[3][byte(crc>>32)] ^ t[2][byte(crc>>40)] ^
			t[1][byte(crc>>48)] ^ t[0][crc>>56]
		p = p[8:]
	}
	t0 := &t[0]
	for _, v := range p {
		crc = t0[byte(crc)^v] ^ (crc >> 8)
	}
	return ^crc
}

// reduceTable reduces the folded reflected accumulator (hi:lo) to the 64-bit
// raw CRC state. (hi:lo) * x^64 mod P is exactly the raw table-CRC state of the
// 16 bytes lo||hi (little endian) started from state 0, so two slicing-by-8
// steps replace the 128-iteration bit-serial reduce128 (which stays as the
// test oracle). t is nil for a custom polynomial: a byte-wise walk of tab.
func reduceTable(hi, lo uint64, tab *Table, t *slicing8) uint64 {
	if t == nil {
		s := lo
		for i := 0; i < 8; i++ {
			s = tab[byte(s)] ^ (s >> 8)
		}
		s ^= hi
		for i := 0; i < 8; i++ {
			s = tab[byte(s)] ^ (s >> 8)
		}
		return s
	}
	s := lo
	s = t[7][byte(s)] ^ t[6][byte(s>>8)] ^ t[5][byte(s>>16)] ^
		t[4][byte(s>>24)] ^ t[3][byte(s>>32)] ^ t[2][byte(s>>40)] ^
		t[1][byte(s>>48)] ^ t[0][s>>56]
	s ^= hi
	return t[7][byte(s)] ^ t[6][byte(s>>8)] ^ t[5][byte(s>>16)] ^
		t[4][byte(s>>24)] ^ t[3][byte(s>>32)] ^ t[2][byte(s>>40)] ^
		t[1][byte(s>>48)] ^ t[0][s>>56]
}
