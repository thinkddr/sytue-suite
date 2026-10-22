// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package auth implementa la autenticación de remitente del correo: firma DKIM
// de salientes (RFC 6376) y verificación SPF (RFC 7208) / DKIM / DMARC (RFC
// 7489) de entrantes, sobre las librerías permisivas emersion/go-msgauth (MIT)
// y blackfriday-free DNS de la stdlib. Sin código de terceros AGPL.
package auth

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/emersion/go-msgauth/dkim"
)

// Signer firma mensajes salientes con DKIM. La clave privada (Ed25519 o RSA) se
// inyecta desde el gestor de secretos en PEM; NUNCA vive en git.
type Signer struct {
	domain   string
	selector string
	signer   crypto.Signer
}

// NewSigner construye un Signer a partir de la clave privada PEM. domain es el
// dominio firmante (d=) y selector el selector DNS (s=). Si keyPEM está vacío,
// devuelve (nil, nil): el llamador omite la firma (solo dev).
func NewSigner(domain, selector, keyPEM string) (*Signer, error) {
	if keyPEM == "" {
		return nil, nil //nolint:nilnil // contrato documentado: sin clave ⇒ sin firma DKIM (solo dev).
	}
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("auth: clave DKIM PEM inválida")
	}
	var key crypto.Signer
	switch block.Type {
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: parsear clave DKIM PKCS8: %w", err)
		}
		s, ok := k.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("auth: tipo de clave DKIM no soportado")
		}
		key = s
	case "RSA PRIVATE KEY":
		k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: parsear clave DKIM RSA: %w", err)
		}
		key = k
	default:
		return nil, fmt.Errorf("auth: bloque PEM de clave DKIM inesperado: %q", block.Type)
	}
	return &Signer{domain: domain, selector: selector, signer: key}, nil
}

// Sign firma el mensaje crudo y devuelve el .eml con la cabecera DKIM-Signature
// antepuesta. Firma las cabeceras canónicas y el cuerpo (relaxed/relaxed).
func (s *Signer) Sign(raw []byte) ([]byte, error) {
	if s == nil {
		return raw, nil // firma desactivada (dev)
	}
	opts := &dkim.SignOptions{
		Domain:   s.domain,
		Selector: s.selector,
		Signer:   s.signer,
		Hash:     crypto.SHA256,
		HeaderKeys: []string{
			"From", "To", "Cc", "Subject", "Date", "Message-Id",
			"MIME-Version", "Content-Type", "Reply-To",
		},
	}
	if _, ok := s.signer.(ed25519.PrivateKey); ok {
		// Ed25519-SHA256 (RFC 8463): go-msgauth selecciona el algoritmo por tipo
		// de clave; el hash declarado se ignora para ed25519.
		opts.Hash = crypto.SHA256
	}
	var out bytes.Buffer
	if err := dkim.Sign(&out, bytes.NewReader(raw), opts); err != nil {
		return nil, fmt.Errorf("auth: firmar DKIM: %w", err)
	}
	return out.Bytes(), nil
}

// maxFirmasDKIM acota cuántas firmas se verifican por mensaje: cada una es una
// consulta DNS a un dominio que elige el remitente, y un correo con mil cabeceras
// DKIM-Signature nos pondría a hacer mil.
const maxFirmasDKIM = 8

// VerifyDKIM verifica las firmas DKIM de un mensaje entrante. Devuelve el
// veredicto (RFC 8601: «pass» si alguna firma es válida) y, aparte, los dominios
// `d=` de las firmas VÁLIDAS.
//
// Los dominios son lo que importa para DMARC: una firma válida solo dice que
// QUIEN FIRMA responde del mensaje, no que sea el dominio del `From:`. Cualquiera
// firma con su propio dominio un correo que diga venir de otro; quien decide si
// eso autentica al remitente es la alineación (VerifyDMARC), y para eso necesita
// saber quién firmó. Devolver solo «pass» obligaba al llamante a suponerlo.
//
// res resuelve la clave pública (TXT de <selector>._domainkey.<d=>); nil usa el
// DNS del sistema.
func VerifyDKIM(ctx context.Context, res Resolver, raw []byte) (Result, []string, error) {
	opts := &dkim.VerifyOptions{MaxVerifications: maxFirmasDKIM}
	if res != nil {
		opts.LookupTXT = func(name string) ([]string, error) { return res.LookupTXT(ctx, name) }
	}
	verifications, err := dkim.VerifyWithOptions(bytes.NewReader(raw), opts)
	if err != nil && !errors.Is(err, dkim.ErrTooManySignatures) {
		return ResultTempError, nil, fmt.Errorf("auth: verificar DKIM: %w", err)
	}
	if len(verifications) == 0 {
		return ResultNone, nil, nil
	}
	var validos []string
	for _, v := range verifications {
		if v.Err == nil && v.Domain != "" {
			validos = append(validos, strings.ToLower(v.Domain))
		}
	}
	if len(validos) > 0 {
		return ResultPass, validos, nil
	}
	// Distinguir error permanente (firma mal formada) de temporal (DNS).
	if dkim.IsTempFail(verifications[0].Err) {
		return ResultTempError, nil, nil
	}
	if dkim.IsPermFail(verifications[0].Err) {
		return ResultPermError, nil, nil
	}
	return ResultFail, nil, nil
}
