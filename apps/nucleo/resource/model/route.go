// SPDX-License-Identifier: Apache-2.0
// Package model contains public routing/sequence values shared by resource and
// Matrix session certificates, without importing either transport or cipher.
package model

import (
	"encoding/hex"
	"errors"
	"math"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrInvalid = errors.New("resource: invalid routing or sequence")

// Decimal preserves the complete nonnegative SQL int64 sequence across JSON and
// Wasm. A JavaScript number is deliberately not accepted, even below 2^53.
type Decimal string

func Sequence(value int64) (Decimal, error) {
	if value < 0 {
		return "", ErrInvalid
	}
	return Decimal(strconv.FormatInt(value, 10)), nil
}

func (s Decimal) Int64() (int64, error) {
	value, err := strconv.ParseInt(string(s), 10, 64)
	if err != nil || value < 0 || strconv.FormatInt(value, 10) != string(s) {
		return 0, ErrInvalid
	}
	return value, nil
}

func (s Decimal) Next(count int64) (Decimal, error) {
	value, err := s.Int64()
	if err != nil || count < 1 || value > math.MaxInt64-count {
		return "", ErrInvalid
	}
	return Sequence(value + count)
}

// Resource routing is public; names, labels, ACL password hashes, application
// content, file keys and descriptors are encrypted application data.
type Resource struct {
	Tenant  string `json:"tenant"`
	Product string `json:"product"`
	Kind    string `json:"kind"`
	ID      string `json:"id"`
}

func (r Resource) Valid() bool {
	return (storage.Scope{Tenant: r.Tenant, Subject: "resource", Product: r.Product}).Valid() && storage.Identifier(r.Kind) && storage.Identifier(r.ID)
}

type Position struct {
	Seq              Decimal `json:"seq"`
	HeadCipherSHA256 string  `json:"head_cipher_sha256"`
}

func (p Position) Valid() bool {
	seq, err := p.Seq.Int64()
	return err == nil && (seq == 0 && p.HeadCipherSHA256 == "" || Digest(p.HeadCipherSHA256))
}

func Digest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && hex.EncodeToString(raw) == value
}
