// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func certificadoDePrueba(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "sp.ejemplo"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kder, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kder}))
}

// TestValidarCertificadoSP: el campo es público, así que lo que importa es lo que se
// rechaza — sobre todo la clave privada pegada en lugar del certificado, que se
// publicaría a cualquier admin en la siguiente lectura.
func TestValidarCertificadoSP(t *testing.T) {
	cert, key := certificadoDePrueba(t)
	casos := []struct {
		nombre string
		pem    string
		vale   bool
		dice   string
	}{
		{"vacío (es opcional)", "", true, ""},
		{"un certificado", cert, true, ""},
		{"una cadena de dos", cert + cert, true, ""},
		{"la clave privada", key, false, "private key"},
		{"certificado y clave juntos", cert + key, false, "private key"},
		{"texto que no es PEM", "MIIBhello", false, "BEGIN CERTIFICATE"},
		{"PEM con basura detrás", cert + "\nsobra", false, "BEGIN CERTIFICATE"},
		{"otro tipo de bloque", string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte{1}})), false, "CERTIFICATE blocks"},
		{"bloque CERTIFICATE que no es X.509", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("no")})), false, "X.509"},
		{"demasiado grande", strings.Repeat(cert, 40), false, "too large"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := validarCertificadoSP(c.pem)
			if c.vale {
				if err != nil {
					t.Fatalf("rechazado: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("aceptado, quiero 400")
			}
			if !strings.Contains(err.Error(), c.dice) {
				t.Fatalf("error %q no explica %q", err, c.dice)
			}
		})
	}
}
