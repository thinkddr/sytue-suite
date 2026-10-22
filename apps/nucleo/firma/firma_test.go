// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package firma

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func par(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return k, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func firmar(t *testing.T, k *ecdsa.PrivateKey, b []byte) string {
	t.Helper()
	h := sha256.Sum256(b)
	sig, err := ecdsa.SignASN1(rand.Reader, k, h[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(sig)
}

const sumas = "# sytue-instaladores 1\n# version: v3.12.0\n# commit: 0123456789abcdef0123456789abcdef01234567\n" +
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  sytue-one-macos-universal.dmg\n" +
	"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  sytue-one-windows-amd64-installer.exe\n"

func TestVerificarSumas(t *testing.T) {
	buena, pub := par(t)
	otra, _ := par(t)
	f := firmar(t, buena, []byte(sumas))

	got, err := VerificarSumas(pub, []byte(sumas), f, "3.12.0")
	if err != nil || got["sytue-one-macos-universal.dmg"] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || len(got) != 2 {
		t.Fatalf("lo bien firmado no verifica: %v %v", got, err)
	}
	if _, err := VerificarSumas(pub, []byte(sumas), firmar(t, otra, []byte(sumas)), "3.12.0"); !errors.Is(err, ErrFirma) {
		t.Errorf("firmado con otra clave verificó: %v", err)
	}
	tocado := bytes.Replace([]byte(sumas), []byte("aaaa"), []byte("cccc"), 1)
	if _, err := VerificarSumas(pub, tocado, f, "3.12.0"); !errors.Is(err, ErrFirma) {
		t.Errorf("con una suma cambiada verificó: %v", err)
	}
	if _, err := VerificarSumas(pub, []byte(sumas), f, "v3.13.0"); err == nil {
		t.Error("un SHA256SUMS bien firmado de otra versión valió")
	}
	if _, err := VerificarSumas(pub, []byte(sumas), "", "3.12.0"); !errors.Is(err, ErrFirma) {
		t.Errorf("sin firma verificó: %v", err)
	}
}

// La clave embebida es la del repositorio, byte a byte, o no hay ninguna de las dos:
// una app con otra clave rechazaría todas las versiones, y una sin la clave que el CI ya
// usa seguiría en modo aviso sin que nadie lo notara.
func TestLaClaveEmbebidaEsLaDelRepositorio(t *testing.T) {
	repo, errRepo := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "firma", "publicacion.pub"))
	emb, errEmb := os.ReadFile(filepath.Join("clave", "publicacion.pub"))
	switch {
	case os.IsNotExist(errRepo) && os.IsNotExist(errEmb):
		if Publica() != nil {
			t.Fatal("sin clave en el repositorio, el binario trae una")
		}
	case errRepo != nil || errEmb != nil:
		t.Fatalf("deploy/firma/publicacion.pub (%v) y apps/nucleo/firma/clave/publicacion.pub (%v) tienen que existir las dos o ninguna", errRepo, errEmb)
	case !bytes.Equal(repo, emb):
		t.Fatal("apps/nucleo/firma/clave/publicacion.pub no es la copia de deploy/firma/publicacion.pub")
	case !bytes.Equal(Publica(), repo):
		t.Fatal("Publica() no devuelve la clave embebida")
	}
}
