// SPDX-License-Identifier: Apache-2.0
package storage

// CipherUsage counts committed opaque objects still held by a provider, including
// tombstones awaiting confirmed deletion. It never counts plaintext MIME sizes.
type CipherUsage struct {
	Version            int   `json:"v"`
	Scope              Scope `json:"scope"`
	Bytes              int64 `json:"bytes"`
	SytueBytes         int64 `json:"sytue_bytes"`
	ExternalBytes      int64 `json:"external_bytes"`
	PendingDeleteBytes int64 `json:"pending_delete_bytes"`
	Blobs              int64 `json:"blobs"`
}

func (u CipherUsage) Valid(scope Scope) bool {
	return scope.Valid() && u.Version == 1 && u.Scope == scope && u.Bytes >= 0 && u.SytueBytes >= 0 && u.SytueBytes <= u.Bytes && u.ExternalBytes == u.Bytes-u.SytueBytes && u.PendingDeleteBytes >= 0 && u.PendingDeleteBytes <= u.Bytes && u.Blobs >= 0
}
