// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recoverywrap

// Envoltorio de bóveda post-cuántica directa (ADR 0086).
//
// Cada producto de la suite guarda una identidad híbrida post-cuántica propia
// (`age-MLKEM768-X25519-v1`, ML-KEM-768 + X25519 + ChaCha20-Poly1305) y una
// clave de propietario de 32 bytes, envueltas en el navegador con una clave
// AES-256-GCM derivada por HKDF-SHA256 de la clave de desbloqueo de Sytue ID
// (ADR 0084/0085), con separación de dominio propia (`sytue-pq-vault-wrap-v1`).
//
// El servidor solo ve el envoltorio público (`PQVaultWrapper`): nunca recibe
// la identidad privada, ni la clave de propietario, ni la clave de desbloqueo.
// El AAD ata el texto cifrado a `(tenant, product, subject, key_id)`: copiar
// un envoltorio a otro producto, tenant o persona falla la autenticación AEAD.

import (
	"bytes"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// LabelBovedaPQ separa los envoltorios de bóveda post-cuántica de cualquier otro
// uso de la clave de desbloqueo de Sytue ID.
const LabelBovedaPQ = "sytue-pq-vault-wrap-v1"

// PQVaultVersion es el campo `v` de un secreto y un envoltorio de bóveda PQ.
const PQVaultVersion = 1

const (
	pqOwnerKeyLen    = 32
	maxPQSecretPlain = 4096
	maxPQCiphertext  = maxPQSecretPlain + 16
	maxPQTenantLen   = 128
	maxPQSubjectLen  = 255
)

// PQVaultSecret es el material privado que vive cifrado dentro de PQVaultWrapper.
// Nunca sale en claro hacia ningún servidor.
type PQVaultSecret struct {
	V        int    `json:"v"`
	Profile  string `json:"profile"`
	Identity string `json:"identity"`
	OwnerKey string `json:"owner_key"`
	// SigningKey es la clave de firma híbrida de la bóveda (ADR 0087,
	// firma_boveda.go). Opcional: las bóvedas anteriores no la tienen y la añaden
	// en su siguiente apertura; las apps que no firman nada la ignoran.
	SigningKey string `json:"signing_key,omitempty"`
}

// Signer abre la clave de firma de la bóveda. ErrInvalid si no tiene ninguna.
func (s PQVaultSecret) Signer() (*PQVaultSigner, error) {
	if s.SigningKey == "" {
		return nil, ErrInvalid
	}
	return ParsePQVaultSigningKey(s.SigningKey)
}

// Validate comprueba la forma del secreto y que la identidad privada coincide con
// el perfil post-cuántico híbrido declarado.
func (s PQVaultSecret) Validate() (*pqc.Identity, error) {
	if s.V != PQVaultVersion || s.Profile != pqc.HybridProfile {
		return nil, ErrInvalid
	}
	id, err := pqc.ParseIdentity(s.Identity)
	if err != nil || id.Profile() != s.Profile {
		return nil, ErrInvalid
	}
	ownerKey, err := base64.StdEncoding.DecodeString(s.OwnerKey)
	if err != nil || len(ownerKey) != pqOwnerKeyLen {
		clear(ownerKey)
		return nil, ErrInvalid
	}
	clear(ownerKey)
	if s.SigningKey != "" {
		if _, err := ParsePQVaultSigningKey(s.SigningKey); err != nil {
			return nil, ErrInvalid
		}
	}
	return id, nil
}

// PQVaultWrapper es el envoltorio público de una clave de bóveda PQ que guarda
// el backend (`services/drive`). Ninguna parte es secreta.
type PQVaultWrapper struct {
	V          int    `json:"v"`
	Kind       string `json:"kind"`
	Profile    string `json:"profile"`
	KeyID      string `json:"key_id"`
	Recipient  string `json:"recipient"`
	KDF        KDF    `json:"kdf"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// Validate comprueba el formato público del envoltorio sin abrirlo: lo usan tanto
// el servidor antes de persistirlo como los clientes antes de pasarlo al puente.
func (w PQVaultWrapper) Validate() error {
	if w.V != PQVaultVersion || w.Kind != KindSytueID || w.Profile != pqc.HybridProfile {
		return ErrInvalid
	}
	if len(w.Nonce) != nonceLen || len(w.Ciphertext) < minCipher || len(w.Ciphertext) > maxPQCiphertext {
		return ErrInvalid
	}
	if w.KDF.Alg != AlgSytueID || len(w.KDF.Hint) != HintLen ||
		w.KDF.Memory != 0 || w.KDF.Time != 0 || w.KDF.Threads != 0 ||
		len(w.KDF.Salt) != 0 || len(w.KDF.PRFSalt) != 0 {
		return ErrInvalid
	}
	rec, err := pqc.ParseRecipient(w.Profile, w.Recipient)
	if err != nil {
		return ErrInvalid
	}
	id, err := pqc.RecipientID(rec)
	if err != nil || id != w.KeyID {
		return ErrInvalid
	}
	return nil
}

// PQVaultKeyFromAUK deriva la clave AES-256-GCM de envoltorio de bóveda PQ desde
// la clave de desbloqueo de Sytue ID (AUK).
func PQVaultKeyFromAUK(unlockKey []byte) ([]byte, error) {
	if len(unlockKey) != UnlockKeyLen {
		return nil, ErrInvalid
	}
	return hkdf.Key(sha256.New, unlockKey, nil, LabelBovedaPQ+"/"+KindSytueID, keyLen)
}

// PQVaultAAD ata un envoltorio a su tenant, producto, persona (`sub`) e ID de clave.
func PQVaultAAD(tenant, product, subject, keyID string) ([]byte, error) {
	if !validPQScopePart(tenant, maxPQTenantLen) ||
		!storage.Product(product) ||
		!validPQScopePart(subject, maxPQSubjectLen) ||
		!validPQScopePart(keyID, maxKeyID) {
		return nil, ErrInvalid
	}
	return []byte(LabelBovedaPQ + "\x00" + tenant + "\x00" + product + "\x00" + subject + "\x00" + keyID), nil
}

func validPQScopePart(s string, maxLen int) bool {
	return s != "" && len(s) <= maxLen && !strings.ContainsAny(s, "\x00\r\n")
}

// WrapPQVault envuelve `secret` con la clave de desbloqueo `unlockKey` para el
// ámbito `(tenant, product, subject)` generando un nonce aleatorio.
func WrapPQVault(unlockKey []byte, tenant, product, subject string, secret PQVaultSecret) (PQVaultWrapper, error) {
	return SealPQVault(unlockKey, tenant, product, subject, secret, nil)
}

// SealPQVault es WrapPQVault con nonce explícito (para vectores deterministas).
func SealPQVault(unlockKey []byte, tenant, product, subject string, secret PQVaultSecret, nonce []byte) (PQVaultWrapper, error) {
	id, err := secret.Validate()
	if err != nil {
		return PQVaultWrapper{}, err
	}
	rec := id.Recipient()
	keyID, err := pqc.RecipientID(rec)
	if err != nil {
		return PQVaultWrapper{}, ErrInvalid
	}
	aad, err := PQVaultAAD(tenant, product, subject, keyID)
	if err != nil {
		return PQVaultWrapper{}, err
	}
	hint, err := Hint(unlockKey)
	if err != nil {
		return PQVaultWrapper{}, err
	}
	key, err := PQVaultKeyFromAUK(unlockKey)
	if err != nil {
		return PQVaultWrapper{}, err
	}
	defer clear(key)
	aead, err := newAEAD(key)
	if err != nil {
		return PQVaultWrapper{}, err
	}
	if nonce == nil {
		nonce = make([]byte, nonceLen)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return PQVaultWrapper{}, err
		}
	}
	if len(nonce) != nonceLen {
		return PQVaultWrapper{}, ErrInvalid
	}
	plain, err := json.Marshal(secret)
	if err != nil {
		return PQVaultWrapper{}, err
	}
	defer clear(plain)
	if len(plain) > maxPQSecretPlain {
		return PQVaultWrapper{}, ErrInvalid
	}
	w := PQVaultWrapper{
		V:          PQVaultVersion,
		Kind:       KindSytueID,
		Profile:    secret.Profile,
		KeyID:      keyID,
		Recipient:  rec.String(),
		KDF:        KDF{Alg: AlgSytueID, Hint: hint},
		Nonce:      bytes.Clone(nonce),
		Ciphertext: aead.Seal(nil, nonce, plain, aad),
	}
	if err := w.Validate(); err != nil {
		return PQVaultWrapper{}, err
	}
	return w, nil
}

// OpenPQVault abre un envoltorio de bóveda PQ y verifica que la identidad privada
// descifrada corresponde exactamente a `w.KeyID`, `w.Recipient` y `w.Profile`.
func OpenPQVault(unlockKey []byte, tenant, product, subject string, w PQVaultWrapper) (PQVaultSecret, error) {
	if err := w.Validate(); err != nil {
		return PQVaultSecret{}, err
	}
	aad, err := PQVaultAAD(tenant, product, subject, w.KeyID)
	if err != nil {
		return PQVaultSecret{}, err
	}
	hint, err := Hint(unlockKey)
	if err != nil {
		return PQVaultSecret{}, err
	}
	if !bytes.Equal(hint, w.KDF.Hint) {
		return PQVaultSecret{}, ErrWrongSecret
	}
	key, err := PQVaultKeyFromAUK(unlockKey)
	if err != nil {
		return PQVaultSecret{}, err
	}
	defer clear(key)
	aead, err := newAEAD(key)
	if err != nil {
		return PQVaultSecret{}, err
	}
	plain, err := aead.Open(nil, w.Nonce, w.Ciphertext, aad)
	if err != nil {
		return PQVaultSecret{}, ErrWrongSecret
	}
	defer clear(plain)
	var secret PQVaultSecret
	if err := json.Unmarshal(plain, &secret); err != nil {
		return PQVaultSecret{}, ErrInvalid
	}
	id, err := secret.Validate()
	if err != nil {
		return PQVaultSecret{}, ErrInvalid
	}
	gotID, err := pqc.RecipientID(id.Recipient())
	if err != nil || gotID != w.KeyID || id.Recipient().String() != w.Recipient || id.Profile() != w.Profile {
		return PQVaultSecret{}, ErrInvalid
	}
	return secret, nil
}
