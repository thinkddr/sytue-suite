// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package webauthn implementa el parseo de las estructuras de WebAuthn L2
// (clientDataJSON, attestationObject CBOR, authenticatorData y clave pública
// COSE) necesarias para registrar passkeys en Sytue ID.
//
// Clean-room: construido a partir de la especificación W3C WebAuthn Level 2 y
// RFC 8949 (CBOR) / RFC 9052 (COSE). Incluye un decodificador CBOR mínimo
// (subconjunto necesario) para no añadir dependencias; no porta código AGPL.
//
// AVISO DE SEGURIDAD: la verificación de attestation aquí es la mínima viable
// (tipo de operación, reto, origen, rpIdHash y flags). La validación completa de
// formatos de attestation (packed, tpm, android-key, etc.) y de cadenas de
// certificación debe completarse y auditarse ANTES de producción
// (docs/CONVENTIONS.md §0.7).
package webauthn

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ClientData es el clientDataJSON deserializado (WebAuthn L2 §5.8.1).
type ClientData struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"` // base64url
	Origin    string `json:"origin"`
}

// AuthenticatorData representa los campos de authenticatorData (§6.1).
type AuthenticatorData struct {
	RPIDHash      []byte
	Flags         byte
	SignCount     uint32
	AAGUID        []byte
	CredentialID  []byte
	COSEPublicKey []byte // clave pública en codificación COSE_Key (CBOR)
}

// Flags de authenticatorData.
const (
	flagUserPresent  = 0x01
	flagUserVerified = 0x04
	flagAttested     = 0x40
)

// UserPresent indica el bit UP.
func (a AuthenticatorData) UserPresent() bool { return a.Flags&flagUserPresent != 0 }

// UserVerified indica el bit UV.
func (a AuthenticatorData) UserVerified() bool { return a.Flags&flagUserVerified != 0 }

// HasAttestedCredential indica el bit AT (hay credential data adjunto).
func (a AuthenticatorData) HasAttestedCredential() bool { return a.Flags&flagAttested != 0 }

// RPIDHashEquals compara el rpIdHash con el esperado.
func (a AuthenticatorData) RPIDHashEquals(want []byte) bool {
	if len(a.RPIDHash) != len(want) {
		return false
	}
	var diff byte
	for i := range want {
		diff |= a.RPIDHash[i] ^ want[i]
	}
	return diff == 0
}

// AttestationObject es el attestationObject decodificado (§6.5).
type AttestationObject struct {
	Format   string
	AuthData AuthenticatorData
}

// ParseAttestationObject decodifica el attestationObject (CBOR map con claves
// "fmt", "authData", "attStmt") y extrae los datos del autenticador.
func ParseAttestationObject(raw []byte) (AttestationObject, error) {
	d := &cborDecoder{buf: raw}
	m, err := d.readMap()
	if err != nil {
		return AttestationObject{}, fmt.Errorf("webauthn: CBOR attestationObject: %w", err)
	}
	format, _ := m["fmt"].(string)
	authBytes, ok := m["authData"].([]byte)
	if !ok {
		return AttestationObject{}, errors.New("webauthn: authData ausente")
	}
	ad, err := ParseAuthenticatorData(authBytes)
	if err != nil {
		return AttestationObject{}, err
	}
	return AttestationObject{Format: format, AuthData: ad}, nil
}

// ParseAuthenticatorData decodifica el buffer de authenticatorData (§6.1).
func ParseAuthenticatorData(b []byte) (AuthenticatorData, error) {
	if len(b) < 37 {
		return AuthenticatorData{}, errors.New("webauthn: authData demasiado corto")
	}
	ad := AuthenticatorData{
		RPIDHash:  b[0:32],
		Flags:     b[32],
		SignCount: binary.BigEndian.Uint32(b[33:37]),
	}
	if !ad.HasAttestedCredential() {
		return ad, nil
	}
	if len(b) < 55 {
		return AuthenticatorData{}, errors.New("webauthn: attestedCredentialData truncado")
	}
	ad.AAGUID = b[37:53]
	credLen := int(binary.BigEndian.Uint16(b[53:55]))
	off := 55
	if len(b) < off+credLen {
		return AuthenticatorData{}, errors.New("webauthn: credentialId truncado")
	}
	ad.CredentialID = b[off : off+credLen]
	off += credLen
	// El resto es la clave pública COSE (CBOR). Se conserva en crudo; su uso para
	// verificar firmas de assertion vendrá con el flujo de login WebAuthn.
	ad.COSEPublicKey = b[off:]
	return ad, nil
}
