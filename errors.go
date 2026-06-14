package crc64

import "errors"

var (
	errInvalidIdentifier = errors.New("hash/crc64: invalid hash state identifier")
	errInvalidSize       = errors.New("hash/crc64: invalid hash state size")
	errTablesMismatch    = errors.New("hash/crc64: tables do not match")
)
