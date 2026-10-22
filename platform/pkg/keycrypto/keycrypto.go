// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package keycrypto cifra material sensible (claves privadas de firma, secretos
// de conectores externos…) en reposo a nivel de aplicación con AES-256-GCM (AEAD
// autenticado). La clave de cifrado (KEK) proviene del gestor de secretos / KMS
// vía configuración (p. ej. SYTUE_INTEGRATIONS_KEY_ENCRYPTION_KEY,
// SYTUE_ID_KEY_ENCRYPTION_KEY), NUNCA en git ni en claro en la base de datos
// (docs/CONVENTIONS.md §0.7, §10).
//
// Formato del texto cifrado (transportado como texto en una columna):
//
//	"encv1:" + base64std( nonce(12B) || ciphertext || tag )
//
// El prefijo versiona el esquema y, además, permite distinguir un valor cifrado
// de un valor en claro heredado (migración sin downtime): Decrypt devuelve el
// valor tal cual si no lleva el prefijo, de modo que los valores preexistentes en
// claro se siguen leyendo hasta su rotación.
//
// Nota de origen: promovido 1:1 desde services/id/internal/keycrypto para ser el
// hogar común de la suite (mismo precedente que platform/pkg/ssrfhttp). La
// migración ya está completa: no queda ninguna copia interna en services/, y
// este es el único hogar del cifrado en reposo de la suite.
package keycrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// prefix marca un valor cifrado con el esquema v1 (AES-256-GCM).
const prefix = "encv1:"

// KeyLen es la longitud exigida de la clave de cifrado (AES-256 ⇒ 32 bytes).
const KeyLen = 32

// ErrKeyLen indica que la KEK no mide 32 bytes.
var ErrKeyLen = fmt.Errorf("keycrypto: la clave de cifrado debe medir %d bytes (AES-256)", KeyLen)

// Cipher cifra y descifra material sensible con AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// New construye un Cipher a partir de una KEK de 32 bytes. Si key es nil/vacía,
// devuelve (nil, nil): el llamante decide si operar sin cifrado (solo dev) o
// abortar (prod). Un Cipher nil trata Encrypt/Decrypt como identidad.
func New(key []byte) (*Cipher, error) {
	if len(key) == 0 {
		return nil, nil //nolint:nilnil // contrato documentado: sin KEK ⇒ sin cifrado (decide el llamante).
	}
	if len(key) != KeyLen {
		return nil, ErrKeyLen
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("keycrypto: AES: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("keycrypto: GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Enabled indica si hay cifrado configurado (Cipher no nil).
func (c *Cipher) Enabled() bool { return c != nil && c.aead != nil }

// Encrypt cifra el texto plano y devuelve el sobre "encv1:base64(...)". Con un
// Cipher nil (sin KEK) devuelve el texto plano sin cambios (modo dev).
func (c *Cipher) Encrypt(plain string) (string, error) {
	if !c.Enabled() {
		return plain, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("keycrypto: nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt descifra un sobre "encv1:...". Si el valor NO lleva el prefijo, se
// asume valor en claro heredado y se devuelve tal cual (compatibilidad de
// migración). Con un Cipher nil, un valor cifrado es un error (no hay clave).
func (c *Cipher) Decrypt(stored string) (string, error) {
	if !strings.HasPrefix(stored, prefix) {
		// Valor en claro heredado (pre-cifrado). Se lee tal cual.
		return stored, nil
	}
	if !c.Enabled() {
		return "", errors.New("keycrypto: valor cifrado pero no hay clave de cifrado configurada")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", fmt.Errorf("keycrypto: base64: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("keycrypto: texto cifrado truncado")
	}
	nonce, ct := raw[:ns], raw[ns:]
	plain, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("keycrypto: descifrado/autenticación: %w", err)
	}
	return string(plain), nil
}
