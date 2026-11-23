// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recoverywrap

// Clave de firma de una bóveda post-cuántica directa (ADR 0087).
//
// La bóveda de la ADR 0086 prueba quién es su dueño con un HMAC, que solo puede
// comprobar quien ya tiene la clave. Para que un SERVIDOR pueda saber que una
// acción viene de la persona —el alta de una clave de entrada de correo, un turno
// de trabajo— hace falta una firma. Esta clave vive dentro del secreto de la
// bóveda (`PQVaultSecret.SigningKey`), sellada con la misma clave de desbloqueo
// de Sytue ID, y su parte pública es la que Sytue ID registra como ancla.
//
// Es híbrida, como el cifrado: Ed25519 + ML-DSA-65 (FIPS 204). Una firma vale
// solo si verifican LAS DOS mitades, así que quien rompa una de las dos familias
// no puede falsificar nada. Cada firma lleva un contexto de uso: una firma hecha
// para una cosa no sirve para otra.

import (
	"crypto/ed25519"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PQVaultSigningProfile nombra el par híbrido de firma de una bóveda.
const PQVaultSigningProfile = "ed25519-mldsa65-v1"

// LabelFirmaBovedaPQ separa estas firmas de cualquier otro uso de las mismas claves.
const LabelFirmaBovedaPQ = "sytue-pq-vault-sign-v1"

const (
	pqSigningSeedLen = ed25519.SeedSize + mldsa.PrivateKeySize
	// PQVaultSignatureLen es el tamaño fijo de una firma híbrida.
	PQVaultSignatureLen = ed25519.SignatureSize + mldsa.MLDSA65SignatureSize
	maxPQSignContext    = 128
)

// PQVaultSigner es la clave privada de firma de una bóveda, ya abierta.
type PQVaultSigner struct {
	ed ed25519.PrivateKey
	ml *mldsa.PrivateKey
}

// NewPQVaultSigningKey genera una clave de firma nueva en la forma en que se
// guarda dentro del secreto de la bóveda: las dos semillas, en base64.
func NewPQVaultSigningKey() (string, error) {
	seed := make([]byte, pqSigningSeedLen)
	defer clear(seed)
	if _, err := io.ReadFull(rand.Reader, seed); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(seed), nil
}

// ParsePQVaultSigningKey abre la forma guardada de una clave de firma.
func ParsePQVaultSigningKey(encoded string) (*PQVaultSigner, error) {
	seed, err := base64.StdEncoding.DecodeString(encoded)
	defer clear(seed)
	if err != nil || len(seed) != pqSigningSeedLen {
		return nil, ErrInvalid
	}
	ml, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), seed[ed25519.SeedSize:])
	if err != nil {
		return nil, ErrInvalid
	}
	return &PQVaultSigner{ed: ed25519.NewKeyFromSeed(seed[:ed25519.SeedSize]), ml: ml}, nil
}

// Public es la parte pública: lo único de esta clave que sale de la bóveda.
func (s *PQVaultSigner) Public() PQVaultVerifyKey {
	pub, _ := s.ed.Public().(ed25519.PublicKey)
	return PQVaultVerifyKey{
		Profile: PQVaultSigningProfile,
		Ed25519: append([]byte(nil), pub...),
		MLDSA65: s.ml.PublicKey().Bytes(),
	}
}

// Sign firma `message` para un uso concreto (`context`, p. ej. "ingress.key.enroll").
// La mitad ML-DSA es aleatorizada (hedged), como recomienda FIPS 204.
func (s *PQVaultSigner) Sign(context string, message []byte) ([]byte, error) {
	framed, err := pqSignFrame(context, message)
	if err != nil {
		return nil, err
	}
	ml, err := s.ml.Sign(rand.Reader, framed, nil)
	if err != nil {
		return nil, err
	}
	return append(ed25519.Sign(s.ed, framed), ml...), nil
}

// pqSignFrame antepone la etiqueta y el contexto: lo que de verdad se firma. El
// contexto no lleva NUL, así que dos pares (contexto, mensaje) distintos nunca
// dan los mismos bytes.
func pqSignFrame(context string, message []byte) ([]byte, error) {
	if context == "" || len(context) > maxPQSignContext || strings.ContainsAny(context, "\x00\r\n") {
		return nil, ErrInvalid
	}
	framed := make([]byte, 0, len(LabelFirmaBovedaPQ)+len(context)+len(message)+2)
	framed = append(framed, LabelFirmaBovedaPQ...)
	framed = append(framed, 0)
	framed = append(framed, context...)
	framed = append(framed, 0)
	return append(framed, message...), nil
}

// PQVaultVerifyKey es la clave pública de firma de una bóveda: lo que registra
// Sytue ID y con lo que verifica un servicio. Ninguna parte es secreta.
type PQVaultVerifyKey struct {
	Profile string `json:"profile"`
	Ed25519 []byte `json:"ed25519"`
	MLDSA65 []byte `json:"mldsa65"`
}

// Validate comprueba la forma de la clave pública.
func (k PQVaultVerifyKey) Validate() error {
	if k.Profile != PQVaultSigningProfile || len(k.Ed25519) != ed25519.PublicKeySize ||
		len(k.MLDSA65) != mldsa.MLDSA65PublicKeySize {
		return ErrInvalid
	}
	if _, err := mldsa.NewPublicKey(mldsa.MLDSA65(), k.MLDSA65); err != nil {
		return ErrInvalid
	}
	return nil
}

// ID identifica la clave: SHA-256 de la etiqueta y sus dos mitades, en hexadecimal.
// Las dos tienen longitud fija, así que no hace falta separador entre ellas.
func (k PQVaultVerifyKey) ID() (string, error) {
	if err := k.Validate(); err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(LabelFirmaBovedaPQ + "/id\x00" + k.Profile + "\x00"))
	h.Write(k.Ed25519)
	h.Write(k.MLDSA65)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Verify comprueba una firma híbrida para ese contexto. ErrWrongSecret si alguna
// de las dos mitades no verifica: nunca basta con una.
func (k PQVaultVerifyKey) Verify(context string, message, signature []byte) error {
	if err := k.Validate(); err != nil {
		return err
	}
	if len(signature) != PQVaultSignatureLen {
		return ErrInvalid
	}
	framed, err := pqSignFrame(context, message)
	if err != nil {
		return err
	}
	ml, err := mldsa.NewPublicKey(mldsa.MLDSA65(), k.MLDSA65)
	if err != nil {
		return ErrInvalid
	}
	// Las dos se comprueban siempre, aunque la primera ya haya fallado.
	edOK := ed25519.Verify(ed25519.PublicKey(k.Ed25519), framed, signature[:ed25519.SignatureSize])
	mlOK := mldsa.Verify(ml, framed, signature[ed25519.SignatureSize:], nil) == nil
	if !edOK || !mlOK {
		return ErrWrongSecret
	}
	return nil
}

// El registro de la clave de firma en Sytue ID (ADR 0087). Lo que se firma es una
// DECLARACIÓN: «en el producto P de la organización T, la generación N de la clave
// de la persona S es K, y sustituye a K'». La firma la nueva clave (prueba de que
// quien la registra la tiene) y, en un cambio con continuidad, también la
// anterior (prueba de que quien tenía la vieja aprueba la nueva). Mismos bytes,
// contextos distintos: una firma de alta nunca vale como aprobación.
const (
	// VaultKeyEnrollContext es el contexto de la firma de la clave NUEVA.
	VaultKeyEnrollContext = "sytue-id.vault-key.enroll"
	// VaultKeyRotateContext es el contexto de la firma de la clave ANTERIOR.
	VaultKeyRotateContext = "sytue-id.vault-key.rotate"
	labelDeclaracionClave = "sytue-vault-key-v1"
)

// PQVaultKeyStatement es la declaración canónica de una generación de la clave de
// firma. `previousKeyID` va vacío en el alta (generación 1) y en nada más.
func PQVaultKeyStatement(subject, tenant, product string, generation int64, keyID, previousKeyID string) ([]byte, error) {
	if !validPQScopePart(subject, maxPQSubjectLen) || !validPQScopePart(tenant, maxPQTenantLen) ||
		!storage.Product(product) || generation < 1 || !idDeClaveDeFirma(keyID) ||
		(generation == 1) != (previousKeyID == "") ||
		(previousKeyID != "" && (!idDeClaveDeFirma(previousKeyID) || previousKeyID == keyID)) {
		return nil, ErrInvalid
	}
	return []byte(labelDeclaracionClave + "\x00" + subject + "\x00" + tenant + "\x00" + product + "\x00" +
		strconv.FormatInt(generation, 10) + "\x00" + keyID + "\x00" + previousKeyID), nil
}

// idDeClaveDeFirma: 64 caracteres hexadecimales en minúscula, como PQVaultVerifyKey.ID.
func idDeClaveDeFirma(id string) bool {
	if len(id) != sha256.Size*2 {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
