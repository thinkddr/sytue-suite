// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package recoverywrap cifra en el dispositivo la clave de recuperación de Matrix
// con una clave que el servidor nunca ve (ADR 0082). El servicio Rooms guarda solo
// el resultado (Wrapper) y sus parámetros públicos: ni la base de datos, ni la KEK,
// ni Sytue ID bastan para abrirlo.
//
// El formato es el de docs/architecture.md («Custodia envuelta de la clave de
// recuperación») y lo fijan los vectores de testdata/vectors.json, que también
// comprueban las pruebas de la web.
package recoverywrap

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Label separa estas claves de cualquier otro uso de la misma contraseña o passkey.
const Label = "sytue-matrix-recovery-wrap-v1"

// Version es el campo `v` del formato.
const Version = 1

// Tipos de envoltorio.
const (
	KindPassword = "password"
	KindPasskey  = "passkey"
)

// Algoritmos de derivación.
const (
	AlgArgon2id   = "argon2id"
	AlgPasskeyPRF = "webauthn-prf-hkdf-sha256" //nolint:gosec // G101: nombre de un algoritmo, no una credencial.
)

// Parámetros de Argon2id por defecto (RFC 9106 §4, segunda recomendación con p=1
// para que el navegador lo calcule en un solo hilo): 64 MiB, 3 pasadas.
const (
	DefaultMemoryKiB = 64 * 1024
	DefaultTime      = 3
	DefaultThreads   = 1
)

// Límites que el servidor y los clientes aceptan: por debajo es demasiado barato
// de atacar; por encima, una forma de colgar un dispositivo con un envoltorio.
const (
	MinMemoryKiB = 64 * 1024
	MaxMemoryKiB = 1024 * 1024
	MinTime      = 3
	MaxTime      = 10
	MinThreads   = 1
	MaxThreads   = 4
)

const (
	saltLen      = 16
	prfSaltLen   = 32
	prfOutputLen = 32
	nonceLen     = 12
	keyLen       = 32
	maxKeyID     = 128
	maxAccount   = 255
	maxCred      = 1023
	minCipher    = 16 + 1
	maxCipher    = 256 + 16
)

var (
	// ErrInvalid es un envoltorio o unos parámetros que no siguen el formato.
	ErrInvalid = errors.New("recoverywrap: invalid wrapper")
	// ErrWrongSecret es una contraseña (o passkey) que no abre el envoltorio. No
	// distingue «contraseña equivocada» de «envoltorio manipulado»: las dos cosas
	// se ven igual desde el dispositivo.
	ErrWrongSecret = errors.New("recoverywrap: wrong password or passkey")
)

// KDF son los parámetros públicos de la derivación. Para una contraseña: Alg,
// Memory, Time, Threads y Salt. Para una passkey: Alg y PRFSalt (la entrada
// `first` de la extensión PRF de WebAuthn).
type KDF struct {
	Alg     string `json:"alg"`
	Memory  uint32 `json:"m,omitempty"`
	Time    uint32 `json:"t,omitempty"`
	Threads uint8  `json:"p,omitempty"`
	Salt    []byte `json:"salt,omitempty"`
	PRFSalt []byte `json:"prf_salt,omitempty"`
}

// Wrapper es lo que viaja al servidor y vuelve: ninguna parte es secreta.
// Los []byte van en base64 estándar con relleno (encoding/json), salvo
// CredentialID, que va en base64url sin relleno, como en WebAuthn.
type Wrapper struct {
	ID           string `json:"id,omitempty"`
	V            int    `json:"v"`
	Kind         string `json:"kind"`
	KeyID        string `json:"key_id"`
	CredentialID string `json:"credential_id"`
	KDF          KDF    `json:"kdf"`
	Nonce        []byte `json:"nonce"`
	Ciphertext   []byte `json:"ciphertext"`
	CreatedAt    string `json:"created_at,omitempty"`
}

// Validate comprueba la forma del envoltorio, sin abrirlo. La usan el servidor
// antes de guardar y el cliente antes de gastar 64 MiB en una derivación.
func (w Wrapper) Validate() error {
	if w.V != Version || w.KeyID == "" || len(w.KeyID) > maxKeyID || strings.ContainsRune(w.KeyID, 0) ||
		len(w.Nonce) != nonceLen || len(w.Ciphertext) < minCipher || len(w.Ciphertext) > maxCipher {
		return ErrInvalid
	}
	switch w.Kind {
	case KindPassword:
		if w.CredentialID != "" || len(w.KDF.PRFSalt) != 0 || !w.KDF.validPassword() {
			return ErrInvalid
		}
	case KindPasskey:
		if !validCredential(w.CredentialID) || !w.KDF.validPasskey() {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func (k KDF) validPassword() bool {
	return k.Alg == AlgArgon2id && len(k.Salt) == saltLen &&
		k.Memory >= MinMemoryKiB && k.Memory <= MaxMemoryKiB &&
		k.Time >= MinTime && k.Time <= MaxTime &&
		k.Threads >= MinThreads && k.Threads <= MaxThreads
}

func (k KDF) validPasskey() bool {
	return k.Alg == AlgPasskeyPRF && len(k.PRFSalt) == prfSaltLen && len(k.Salt) == 0 &&
		k.Memory == 0 && k.Time == 0 && k.Threads == 0
}

func validCredential(id string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(id)
	return err == nil && len(raw) > 0 && len(raw) <= maxCred
}

// AAD ata el texto cifrado a su cuenta de Matrix, a su tipo y a su clave de
// almacenamiento: un envoltorio copiado a otra cuenta o a otra fila no se abre.
func AAD(kind, account, keyID, credentialID string) []byte {
	var b bytes.Buffer
	for i, part := range []string{Label, kind, account, keyID, credentialID} {
		if i > 0 {
			b.WriteByte(0)
		}
		b.WriteString(part)
	}
	return b.Bytes()
}

// PasswordKey deriva la clave de envoltorio de una contraseña.
func PasswordKey(password string, kdf KDF) ([]byte, error) {
	if password == "" || !kdf.validPassword() {
		return nil, ErrInvalid
	}
	ikm := argon2.IDKey([]byte(password), kdf.Salt, kdf.Time, kdf.Memory, kdf.Threads, keyLen)
	defer clear(ikm)
	return hkdf.Key(sha256.New, ikm, nil, Label+"/"+KindPassword, keyLen)
}

// PasskeyKey deriva la clave de envoltorio de la salida PRF de una passkey.
func PasskeyKey(prf []byte) ([]byte, error) {
	if len(prf) != prfOutputLen {
		return nil, ErrInvalid
	}
	return hkdf.Key(sha256.New, prf, nil, Label+"/"+KindPasskey, keyLen)
}

// NewPasswordKDF elige una sal nueva con los parámetros por defecto.
func NewPasswordKDF() (KDF, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return KDF{}, err
	}
	return KDF{Alg: AlgArgon2id, Memory: DefaultMemoryKiB, Time: DefaultTime, Threads: DefaultThreads, Salt: salt}, nil
}

// Seal envuelve una clave de recuperación con una clave ya derivada. nonce es nil
// salvo en los vectores de prueba.
func Seal(wrapKey []byte, kind, account, keyID, credentialID string, kdf KDF, recoveryKey string, nonce []byte) (Wrapper, error) {
	if recoveryKey == "" || account == "" || len(account) > maxAccount {
		return Wrapper{}, ErrInvalid
	}
	aead, err := newAEAD(wrapKey)
	if err != nil {
		return Wrapper{}, err
	}
	if nonce == nil {
		nonce = make([]byte, nonceLen)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return Wrapper{}, err
		}
	}
	if len(nonce) != nonceLen {
		return Wrapper{}, ErrInvalid
	}
	w := Wrapper{V: Version, Kind: kind, KeyID: keyID, CredentialID: credentialID, KDF: kdf, Nonce: bytes.Clone(nonce)}
	w.Ciphertext = aead.Seal(nil, w.Nonce, []byte(recoveryKey), AAD(kind, account, keyID, credentialID))
	if err := w.Validate(); err != nil {
		return Wrapper{}, err
	}
	return w, nil
}

// Open abre un envoltorio con una clave ya derivada.
func Open(wrapKey []byte, account string, w Wrapper) (string, error) {
	if err := w.Validate(); err != nil {
		return "", err
	}
	aead, err := newAEAD(wrapKey)
	if err != nil {
		return "", err
	}
	plain, err := aead.Open(nil, w.Nonce, w.Ciphertext, AAD(w.Kind, account, w.KeyID, w.CredentialID))
	if err != nil {
		return "", ErrWrongSecret
	}
	defer clear(plain)
	return string(plain), nil
}

// WrapPassword envuelve la clave con una contraseña y una sal nueva.
func WrapPassword(password, account, keyID, recoveryKey string) (Wrapper, error) {
	kdf, err := NewPasswordKDF()
	if err != nil {
		return Wrapper{}, err
	}
	key, err := PasswordKey(password, kdf)
	if err != nil {
		return Wrapper{}, err
	}
	defer clear(key)
	return Seal(key, KindPassword, account, keyID, "", kdf, recoveryKey, nil)
}

// UnwrapPassword abre un envoltorio de contraseña.
func UnwrapPassword(password, account string, w Wrapper) (string, error) {
	if err := w.Validate(); err != nil || w.Kind != KindPassword {
		return "", ErrInvalid
	}
	key, err := PasswordKey(password, w.KDF)
	if err != nil {
		return "", err
	}
	defer clear(key)
	return Open(key, account, w)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != keyLen {
		return nil, ErrInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
