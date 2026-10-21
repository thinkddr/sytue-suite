// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package firma verifica lo que firma la publicación (auditoría 2026-10-06, CI-05):
// el SHA256SUMS de los instaladores de escritorio. El formato y quien firma están en
// scripts/firma-publicacion.sh; aquí solo se verifica, con la biblioteca estándar.
//
// La clave de confianza va EMBEBIDA al compilar (clave/publicacion.pub, copia de
// deploy/firma/publicacion.pub), nunca la da el servidor: si la diera quien sirve los
// instaladores, firmar no protegería de nada. Sin clave, no hay nada que exigir y el
// actualizador sigue como antes, con la suma que da el Hub.
package firma

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

//go:embed all:clave
var clave embed.FS

// Publica es la clave de publicación embebida, o nil si este binario no trae ninguna.
func Publica() []byte {
	b, err := clave.ReadFile("clave/publicacion.pub")
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	return b
}

// ErrFirma es que el SHA256SUMS no está firmado con la clave de publicación.
var ErrFirma = errors.New("firma: el SHA256SUMS no verifica con la clave de publicación")

// VerificarSumas comprueba la firma de `sumas` con la clave `pub` (PEM PKIX, ECDSA
// P-256), que sea de la versión `version` (con o sin «v») y devuelve la SHA-256 de
// cada instalador por su nombre.
func VerificarSumas(pub, sumas []byte, firmaB64, version string) (map[string]string, error) {
	bloque, _ := pem.Decode(pub)
	if bloque == nil {
		return nil, errors.New("firma: la clave pública no es PEM")
	}
	k, err := x509.ParsePKIXPublicKey(bloque.Bytes)
	if err != nil {
		return nil, fmt.Errorf("firma: %w", err)
	}
	ec, ok := k.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("firma: la clave pública no es ECDSA")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(firmaB64))
	if err != nil || len(sig) == 0 {
		return nil, ErrFirma
	}
	h := sha256.Sum256(sumas)
	if !ecdsa.VerifyASN1(ec, h[:], sig) {
		return nil, ErrFirma
	}
	// Lo firmado tiene que ser de ESTA versión: si no, un SHA256SUMS viejo y bien
	// firmado serviría para colar un instalador viejo.
	esta := "v" + strings.TrimPrefix(version, "v")
	out := map[string]string{}
	cabecera, deVersion := false, false
	s := bufio.NewScanner(bytes.NewReader(sumas))
	for n := 0; s.Scan(); n++ {
		l := s.Text()
		switch {
		case n == 0:
			cabecera = l == "# sytue-instaladores 1"
		case l == "# version: "+esta:
			deVersion = true
		case strings.HasPrefix(l, "#") || l == "":
		default:
			suma, nombre, ok := strings.Cut(l, "  ")
			if _, err := hex.DecodeString(suma); !ok || err != nil || len(suma) != 64 {
				return nil, fmt.Errorf("firma: línea de SHA256SUMS mal formada: %q", l)
			}
			out[nombre] = strings.ToLower(suma)
		}
	}
	if !cabecera {
		return nil, errors.New("firma: no es un SHA256SUMS «sytue-instaladores 1»")
	}
	if !deVersion {
		return nil, fmt.Errorf("firma: el SHA256SUMS no es de la versión %s", esta)
	}
	return out, nil
}
