// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"maunium.net/go/mautrix/crypto/ssss"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// storageOwnerProof uses maintained Matrix Secret Storage encryption to attest
// private application data with its owner's symmetric recovery secret. Backup
// encryption alone is anonymous: anyone knows the backup's public encryption key.
// This proof stays INSIDE Megolm and does not change Matrix key/recipient policy.
type storageOwnerProof struct {
	Version   int            `json:"v"`
	Algorithm ssss.Algorithm `json:"algorithm"`
	KeyID     string         `json:"key_id"`
	ssss.EncryptedKeyData
}

func ownerKeyID(value string) bool {
	if len(value) < 1 || len(value) > 255 {
		return false
	}
	for _, c := range value {
		if c < 0x21 || c > 0x7e {
			return false
		}
	}
	return true
}

func ownerProofBytes(value string, length int) bool {
	trimmed := strings.TrimRight(value, "=")
	if len(value) > base64.StdEncoding.EncodedLen(length) {
		return false
	}
	decoded, err := base64.RawStdEncoding.DecodeString(trimmed)
	return err == nil && len(decoded) == length && base64.RawStdEncoding.EncodeToString(decoded) == trimmed && (value == trimmed || value == base64.StdEncoding.EncodeToString(decoded))
}

func sealStorageOwnerRecord(room string, record storage.Record, key *ssss.Key) (storageOwnerProof, error) {
	if key == nil || len(key.Key) != 32 || !ownerKeyID(key.ID) {
		return storageOwnerProof{}, ErrRecoveryRequired
	}
	digest, err := storage.RecordDigest(room, record)
	if err != nil {
		return storageOwnerProof{}, err
	}
	return storageOwnerProof{Version: 1, Algorithm: ssss.AlgorithmAESHMACSHA2, KeyID: key.ID, EncryptedKeyData: key.Encrypt(storage.OwnerProofDomain+"."+digest, []byte(digest))}, nil
}

func verifyStorageOwnerRecord(room string, record storage.Record, proof storageOwnerProof, key *ssss.Key) error {
	// Check all lengths and encodings before invoking the SDK's decoder. Its SSSS
	// primitive implements encryption/MAC; this adapter binds application context.
	if !proof.valid() || key == nil || key.ID != proof.KeyID || len(key.Key) != 32 {
		return ErrUnverified
	}
	digest, err := storage.RecordDigest(room, record)
	if err != nil {
		return ErrUnverified
	}
	cleartext, err := key.Decrypt(storage.OwnerProofDomain+"."+digest, proof.EncryptedKeyData)
	defer clear(cleartext)
	if err != nil || subtle.ConstantTimeCompare(cleartext, []byte(digest)) != 1 {
		return ErrUnverified
	}
	return nil
}

func (p storageOwnerProof) valid() bool {
	return p.Version == 1 && p.Algorithm == ssss.AlgorithmAESHMACSHA2 && ownerKeyID(p.KeyID) && ownerProofBytes(p.IV, 16) && ownerProofBytes(p.MAC, 32) && ownerProofBytes(p.Ciphertext, 86)
}
