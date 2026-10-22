// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// ErrNoCipher indica que se pidió cifrar/descifrar credenciales sin clave
// configurada (SYTUE_DOCS_SECRET_ENC_KEY ausente). Guardar un secreto externo en
// claro nunca es aceptable: sin clave, la conexión con credenciales se rechaza.
var ErrNoCipher = errors.New("connectors: cifrado de credenciales no configurado")

// ErrCiphertext indica un texto cifrado corrupto o con clave incorrecta.
var ErrCiphertext = errors.New("connectors: credencial cifrada inválida")

// Sealer cifra/descifra las credenciales de conexión en reposo con AES-256-GCM
// (AEAD autenticado), 100 % stdlib. El blob resultante (nonce||ciphertext||tag)
// se guarda tal cual en la columna bytea; el descifrado ocurre SOLO en memoria al
// ejecutar. Mismo esquema que services/id/keycrypto y services/cloud/crypto (no
// importables por ser paquetes internal de otros servicios).
type Sealer struct {
	aead cipher.AEAD
}

// NewSealer construye un Sealer a partir de una clave de 32 bytes (AES-256). Si
// key es vacía devuelve (nil, nil): el servicio arranca sin cifrado y rechaza
// crear conexiones con credenciales (ErrNoCipher). Un Sealer nil no cifra.
func NewSealer(key []byte) (*Sealer, error) {
	if len(key) == 0 {
		return nil, nil //nolint:nilnil // contrato documentado: sin clave ⇒ sin Sealer (decide el llamante).
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("connectors: la clave debe medir 32 bytes (AES-256), tiene %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("connectors: AES: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("connectors: GCM: %w", err)
	}
	return &Sealer{aead: aead}, nil
}

// Enabled indica si hay cifrado disponible.
func (s *Sealer) Enabled() bool { return s != nil && s.aead != nil }

// Seal cifra plaintext y devuelve nonce||ciphertext||tag. Sin Sealer => ErrNoCipher.
func (s *Sealer) Seal(plaintext []byte) ([]byte, error) { return s.SealPara(plaintext, nil) }

// SealPara es Seal atado a `para` (datos asociados del AEAD): solo lo abre OpenPara con
// el mismo `para`. Así un blob no vale en otra fila ni para otra cosa.
func (s *Sealer) SealPara(plaintext, para []byte) ([]byte, error) {
	if !s.Enabled() {
		return nil, ErrNoCipher
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("connectors: generar nonce: %w", err)
	}
	return s.aead.Seal(nonce, nonce, plaintext, para), nil
}

// Open descifra un blob producido por Seal. Sin Sealer => ErrNoCipher.
func (s *Sealer) Open(blob []byte) ([]byte, error) { return s.OpenPara(blob, nil) }

// OpenPara descifra un blob producido por SealPara con el mismo `para`.
func (s *Sealer) OpenPara(blob, para []byte) ([]byte, error) {
	if !s.Enabled() {
		return nil, ErrNoCipher
	}
	ns := s.aead.NonceSize()
	if len(blob) < ns {
		return nil, ErrCiphertext
	}
	nonce, ct := blob[:ns], blob[ns:]
	plaintext, err := s.aead.Open(nil, nonce, ct, para)
	if err != nil {
		return nil, ErrCiphertext
	}
	return plaintext, nil
}
