// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

package main

import (
	"encoding/hex"
	"encoding/json"
	"syscall/js"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func init() {
	domainOperations["storage.keyring.ids"] = storageKeyIDs
	domainOperations["storage.keyring.create"] = createStorageKey
	domainOperations["storage.keyring.open"] = openStorageKey
	domainOperations["storage.keyring.pin"] = openStoragePin
}

func storageKeyBindingArg(req js.Value) (pqc.StorageKeyBinding, error) {
	version, err := integerArg(req.Get("identity_version"), 2)
	if err != nil || version < 1 {
		return pqc.StorageKeyBinding{}, pqc.ErrIdentity
	}
	userID := req.Get("user_id").String()
	binding := pqc.StorageKeyBinding{Scope: scopeArg(req), IdentityVersion: storage.IdentityVersion(version), Homeserver: id.UserID(userID).Homeserver(), UserID: userID, RecoveryKeyID: req.Get("recovery_key_id").String()}
	if !binding.Valid() {
		return pqc.StorageKeyBinding{}, pqc.ErrIdentity
	}
	return binding, nil
}

func storageKeyIDs(req js.Value) (any, error) {
	binding, err := storageKeyBindingArg(req)
	if err != nil {
		return nil, err
	}
	keyID := req.Get("key_id").String()
	result := map[string]any{"pin_id": pqc.StorageKeyPinID(binding.Scope)}
	if keyID != "" {
		raw, err := hex.DecodeString(keyID)
		if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != keyID {
			return nil, pqc.ErrIdentity
		}
		result["secret_id"] = pqc.StorageKeySecretID(binding.Scope, keyID)
	}
	return result, nil
}

func storageKeyValue(secret pqc.StorageKeySecret) (any, error) {
	raw, err := json.Marshal(secret)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	pin, err := json.Marshal(secret.Pin())
	if err != nil {
		return nil, err
	}
	private := []byte(secret.Identity)
	defer clear(private)
	return map[string]any{"secret": bytesValue(raw), "pin": bytesValue(pin), "identity": bytesValue(private), "profile": secret.Profile, "key_id": secret.KeyID, "recipient": secret.Recipient, "secret_id": pqc.StorageKeySecretID(secret.Scope, secret.KeyID), "pin_id": pqc.StorageKeyPinID(secret.Scope)}, nil
}

func createStorageKey(req js.Value) (any, error) {
	binding, err := storageKeyBindingArg(req)
	if err != nil {
		return nil, err
	}
	secret, err := pqc.NewStorageKeySecret(binding)
	if err != nil {
		return nil, err
	}
	return storageKeyValue(secret)
}

func openStorageKey(req js.Value) (any, error) {
	binding, err := storageKeyBindingArg(req)
	if err != nil {
		return nil, err
	}
	raw, err := bytesArg(req.Get("secret"), 16<<10)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	secret, _, err := pqc.DecodeStorageKeySecret(raw, binding)
	if err != nil {
		return nil, err
	}
	return storageKeyValue(secret)
}

func openStoragePin(req js.Value) (any, error) {
	binding, err := storageKeyBindingArg(req)
	if err != nil {
		return nil, err
	}
	raw, err := bytesArg(req.Get("pin"), 16<<10)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var pin pqc.StorageKeyPin
	if json.Unmarshal(raw, &pin) != nil || !pqc.ValidStorageKeyPin(pin, binding.RecoveryKeyID) {
		return nil, pqc.ErrIdentity
	}
	canonical, err := json.Marshal(pin)
	if err != nil {
		return nil, err
	}
	return map[string]any{"pin": bytesValue(canonical), "profile": pin.Profile, "key_id": pin.KeyID, "recipient": pin.Recipient, "secret_id": pqc.StorageKeySecretID(binding.Scope, pin.KeyID), "pin_id": pqc.StorageKeyPinID(binding.Scope)}, nil
}
