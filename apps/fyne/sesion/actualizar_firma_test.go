// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestConFirmaManda: con clave de publicación, la suma que vale es la del SHA256SUMS
// firmado de ESA versión; si el Hub da otra, o el fichero no está, o la firma o la
// versión no cuadran, no se instala (CI-05).
func TestConFirmaManda(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	pub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	suma := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sumas := []byte("# sytue-instaladores 1\n# version: v3.12.0\n# commit: 0123456789abcdef0123456789abcdef01234567\n" +
		suma + "  sytue-one-macos-universal.dmg\n")
	h := sha256.Sum256(sumas)
	b, _ := ecdsa.SignASN1(rand.Reader, k, h[:])
	sig := base64.StdEncoding.EncodeToString(b)
	i := api.Instalador{App: "hub", Fichero: "sytue-one-macos-universal.dmg", Version: "3.12.0", SHA256: suma}

	if err := conFirma(pub, sumas, sig, i); err != nil {
		t.Fatalf("lo firmado y que cuadra no pasó: %v", err)
	}
	otra := i
	otra.SHA256 = "bb" + suma[2:]
	if conFirma(pub, sumas, sig, otra) == nil {
		t.Error("el Hub dio otra suma y se aceptó")
	}
	otra = i
	otra.Fichero = "sytue-drive-macos-universal.dmg"
	if conFirma(pub, sumas, sig, otra) == nil {
		t.Error("un instalador fuera del SHA256SUMS se aceptó")
	}
	otra = i
	otra.Version = "3.13.0"
	if conFirma(pub, sumas, sig, otra) == nil {
		t.Error("un SHA256SUMS de otra versión se aceptó")
	}
	if conFirma(pub, sumas, "", i) == nil {
		t.Error("sin firma se aceptó")
	}
}
