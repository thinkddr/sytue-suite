// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certificados

import (
	"crypto/x509"
	"encoding/asn1"
	"testing"
)

// Lo que se enseña al elegir: certificados de persona, no de código ni de servidor.
func TestSirveParaFirmar(t *testing.T) {
	for _, c := range []struct {
		nombre string
		cert   x509.Certificate
		quiere bool
	}{
		{"sin restricciones", x509.Certificate{PublicKeyAlgorithm: x509.RSA}, true},
		{"el de FIRMA del DNIe", x509.Certificate{PublicKeyAlgorithm: x509.RSA, KeyUsage: x509.KeyUsageContentCommitment}, true},
		{"FNMT de persona", x509.Certificate{PublicKeyAlgorithm: x509.RSA, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageEmailProtection}}, true},
		{"firma de documentos", x509.Certificate{PublicKeyAlgorithm: x509.ECDSA, UnknownExtKeyUsage: []asn1.ObjectIdentifier{oidFirmaDeDocumentos}}, true},
		{"firma de código", x509.Certificate{PublicKeyAlgorithm: x509.RSA, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}}, false},
		{"de servidor", x509.Certificate{PublicKeyAlgorithm: x509.RSA, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}, false},
		{"solo de cifrado", x509.Certificate{PublicKeyAlgorithm: x509.RSA, KeyUsage: x509.KeyUsageKeyEncipherment}, false},
		{"una CA", x509.Certificate{PublicKeyAlgorithm: x509.RSA, IsCA: true}, false},
		{"Ed25519", x509.Certificate{PublicKeyAlgorithm: x509.Ed25519}, false},
	} {
		if got := sirveParaFirmar(&c.cert); got != c.quiere {
			t.Errorf("%s: %v", c.nombre, got)
		}
	}
}
