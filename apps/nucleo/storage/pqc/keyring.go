// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// StorageKeyBinding is trusted client context, not input taken from a secret.
// Homeserver is the stable Matrix server identifier supplied by the maintained
// client's user-ID codec, not one device's HTTP proxy or well-known base URL.
type StorageKeyBinding struct {
	Scope           storage.Scope           `json:"scope"`
	IdentityVersion storage.IdentityVersion `json:"identity_version"`
	Homeserver      string                  `json:"homeserver"`
	UserID          string                  `json:"user_id"`
	RecoveryKeyID   string                  `json:"recovery_key_id"`
}

type StorageKeySecret struct {
	Version         int                     `json:"v"`
	Scope           storage.Scope           `json:"scope"`
	IdentityVersion storage.IdentityVersion `json:"identity_version"`
	Homeserver      string                  `json:"homeserver"`
	UserID          string                  `json:"user_id"`
	RecoveryKeyID   string                  `json:"recovery_key_id"`
	Profile         string                  `json:"profile"`
	KeyID           string                  `json:"key_id"`
	Recipient       string                  `json:"recipient"`
	Identity        string                  `json:"identity"`
}

type StorageKeyPin struct {
	Version       int    `json:"v"`
	Profile       string `json:"profile"`
	RecoveryKeyID string `json:"recovery_key_id"`
	KeyID         string `json:"key_id"`
	Recipient     string `json:"recipient"`
}

func (binding StorageKeyBinding) Valid() bool {
	return binding.Scope.Valid() && binding.IdentityVersion.Valid() && binding.Homeserver != "" && binding.UserID != "" && binding.RecoveryKeyID != ""
}

func StorageKeySecretID(scope storage.Scope, keyID string) string {
	raw, _ := json.Marshal(scope)
	sum := sha256.Sum256(append(append([]byte("org.sytue.storage.pq.key.v1\x00"), raw...), []byte("\x00"+keyID)...))
	return "pqc_key_" + hex.EncodeToString(sum[:])
}

func StorageKeyPinID(scope storage.Scope) string {
	raw, _ := json.Marshal(scope)
	sum := sha256.Sum256(append([]byte("org.sytue.storage.pq.pin.v1\x00"), raw...))
	return "pqc_pin_" + hex.EncodeToString(sum[:])
}

func (secret StorageKeySecret) Pin() StorageKeyPin {
	return StorageKeyPin{Version: secret.Version, Profile: secret.Profile, RecoveryKeyID: secret.RecoveryKeyID, KeyID: secret.KeyID, Recipient: secret.Recipient}
}

func ValidStorageKeyPin(pin StorageKeyPin, recoveryID string) bool {
	if pin.Version != 1 || pin.Profile != HybridProfile || pin.RecoveryKeyID != recoveryID {
		return false
	}
	recipient, err := ParseRecipient(HybridProfile, pin.Recipient)
	if err != nil {
		return false
	}
	keyID, err := RecipientID(recipient)
	return err == nil && keyID == pin.KeyID
}

// DecodeStorageKeySecret is shared by native and Wasm. It never upgrades a
// profile, replaces an active pin, accepts another owner, or exposes a key before
// matching the caller's immutable scope/server/recovery binding.
func DecodeStorageKeySecret(raw []byte, binding StorageKeyBinding) (StorageKeySecret, *Identity, error) {
	var secret StorageKeySecret
	if !binding.Valid() || len(raw) == 0 || len(raw) > 16<<10 || json.Unmarshal(raw, &secret) != nil ||
		secret.Scope != binding.Scope || secret.IdentityVersion != binding.IdentityVersion ||
		secret.Homeserver != binding.Homeserver || secret.UserID != binding.UserID ||
		!ValidStorageKeyPin(secret.Pin(), binding.RecoveryKeyID) {
		return StorageKeySecret{}, nil, ErrIdentity
	}
	identity, err := ParseIdentity(secret.Identity)
	if err != nil || identity.Profile() != HybridProfile || identity.Recipient().String() != secret.Recipient {
		return StorageKeySecret{}, nil, ErrIdentity
	}
	return secret, identity, nil
}

func NewStorageKeySecret(binding StorageKeyBinding) (StorageKeySecret, error) {
	if !binding.Valid() {
		return StorageKeySecret{}, ErrIdentity
	}
	identity, err := GenerateIdentity()
	if err != nil {
		return StorageKeySecret{}, err
	}
	keyID, err := RecipientID(identity.Recipient())
	if err != nil {
		return StorageKeySecret{}, err
	}
	return StorageKeySecret{
		Version: 1, Scope: binding.Scope, IdentityVersion: binding.IdentityVersion,
		Homeserver: binding.Homeserver, UserID: binding.UserID, RecoveryKeyID: binding.RecoveryKeyID,
		Profile: HybridProfile, KeyID: keyID, Recipient: identity.Recipient().String(), Identity: identity.String(),
	}, nil
}
