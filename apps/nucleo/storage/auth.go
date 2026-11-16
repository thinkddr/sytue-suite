// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
)

const OwnerProofDomain = "org.sytue.storage.auth.v1"

// RecordDigest binds every application field and its private Matrix room. The
// ASCII tuple has identical Go/JavaScript encoding; payload JSON is never parsed
// or normalized. This is a content checksum, not a cipher or a signature.
func RecordDigest(room string, record Record) (string, error) {
	if !matrixID(room, '!') || record.Validate(record.Scope) != nil {
		return "", ErrRecord
	}
	fields := []string{room, record.Scope.Tenant, record.Scope.Subject, record.Scope.Product, record.ObjectID, record.RevisionID, record.Role}
	tuple := make([]string, 1, len(fields)+2+len(record.Previous))
	tuple[0] = OwnerProofDomain
	for _, value := range fields {
		tuple = append(tuple, base64.StdEncoding.EncodeToString([]byte(value)))
	}
	tuple = append(tuple, base64.StdEncoding.EncodeToString(record.Payload))
	for _, previous := range record.Previous {
		tuple = append(tuple, base64.StdEncoding.EncodeToString([]byte(previous)))
	}
	encoded, err := json.Marshal(tuple)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
