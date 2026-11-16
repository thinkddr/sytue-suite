// SPDX-License-Identifier: Apache-2.0
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// SourcePreflightInfo has constant size regardless of the library. Its CAS is
// bound to every descriptor in the immutable paged preflight inventory.
type SourcePreflightInfo struct {
	Scope     storage.Scope `json:"scope"`
	Epoch     string        `json:"epoch"`
	CAS       string        `json:"cas"`
	SHA256    string        `json:"sha256"`
	Rows      int64         `json:"rows"`
	Blobs     int64         `json:"blobs"`
	Originals int64         `json:"originals"`
}

func (p SourcePreflightInfo) Validate(scope storage.Scope, epoch string) error {
	sum, err := hex.DecodeString(p.SHA256)
	if !scope.Valid() || scope.Product != "photos" || p.Scope != scope || !storage.MigrationEpoch(epoch) || p.Epoch != epoch || !storage.MigrationEpoch(p.CAS) || err != nil || len(sum) != 32 || hex.EncodeToString(sum) != p.SHA256 || p.Rows < 0 || p.Rows >= 1<<53 || p.Blobs < 0 || p.Blobs > p.Rows || p.Originals < 0 || p.Originals > p.Rows-p.Blobs {
		return ErrSnapshot
	}
	return nil
}

func SourceInventoryHash(scope storage.Scope, epoch string) (hash.Hash, error) {
	if !scope.Valid() || scope.Product != "photos" || !storage.MigrationEpoch(epoch) {
		return nil, ErrSnapshot
	}
	sum := sha256.New()
	_, _ = sum.Write([]byte("org.sytue.photos.retained-source.v1\x00" + scope.Tenant + "\x00" + scope.Subject + "\x00" + epoch + "\x00"))
	return sum, nil
}

func AppendSourceInventoryHash(sum hash.Hash, row storage.StreamSourceRow) error {
	if sum == nil || row.Validate() != nil || !SourceKind(row.Kind) && row.Kind != "variant" {
		return ErrSnapshot
	}
	_, _ = sum.Write([]byte(row.Kind + "\x00" + row.ID + "\x00" + strconv.FormatInt(row.Size, 10) + "\x00" + row.SHA256 + "\x00"))
	return nil
}
