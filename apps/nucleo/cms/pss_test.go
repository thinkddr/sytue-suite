// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cms

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"testing"
)

func paramsPSS(t *testing.T, hash, mgfHash asn1.ObjectIdentifier, sal, trailer int) asn1.RawValue {
	t.Helper()
	mgfParams, err := asn1.Marshal(pkix.AlgorithmIdentifier{Algorithm: mgfHash, Parameters: asn1.NullRawValue})
	if err != nil {
		t.Fatal(err)
	}
	der, err := asn1.Marshal(parametrosPSS{
		Hash:    pkix.AlgorithmIdentifier{Algorithm: hash, Parameters: asn1.NullRawValue},
		MGF:     pkix.AlgorithmIdentifier{Algorithm: oidMGF1, Parameters: asn1.RawValue{FullBytes: mgfParams}},
		Sal:     sal,
		Trailer: trailer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return asn1.RawValue{FullBytes: der}
}

// RSASSA-PSS declara su longitud de sal. Una firma con la sal igual al resumen
// (OpenSSL, Adobe) y otra con la máxima que cabe en la clave (pyHanko) son las dos
// legítimas, y x509.CheckSignature solo acepta la primera: se encontró con un PDF
// firmado por pyHanko que se daba por «no corresponde».
func TestPSSRespetaLaSalDeclarada(t *testing.T) {
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	datos := []byte("atributos firmados")
	resumen := sha256.Sum256(datos)
	maxima := clave.Size() - sha256.Size - 2

	for _, c := range []struct {
		nombre  string
		firmaS  int // sal con la que se firma
		declara int // sal que dice el parámetro
		quiero  bool
	}{
		{"sal igual al resumen", sha256.Size, sha256.Size, true},
		{"sal máxima", maxima, maxima, true},
		{"declara una sal y firma con otra", maxima, sha256.Size, false},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			firma, err := rsa.SignPSS(rand.Reader, clave, crypto.SHA256, resumen[:],
				&rsa.PSSOptions{SaltLength: c.firmaS, Hash: crypto.SHA256})
			if err != nil {
				t.Fatal(err)
			}
			err = comprobarPSS(&clave.PublicKey, paramsPSS(t, oidSHA256, oidSHA256, c.declara, 1), crypto.SHA256, datos, firma)
			if (err == nil) != c.quiero {
				t.Fatalf("err = %v, quería ok=%v", err, c.quiero)
			}
			if err != nil && !errors.Is(err, ErrNoCorresponde) {
				t.Errorf("una firma que no cuadra tiene que ser ErrNoCorresponde: %v", err)
			}
		})
	}
}

func TestPSSRechazaLoQueNoSabeComprobar(t *testing.T) {
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	datos := []byte("atributos firmados")
	resumen := sha256.Sum256(datos)
	firma, err := rsa.SignPSS(rand.Reader, clave, crypto.SHA256, resumen[:],
		&rsa.PSSOptions{SaltLength: sha256.Size, Hash: crypto.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	if err := comprobarPSS(&clave.PublicKey, paramsPSS(t, oidSHA256, oidSHA256, sha256.Size, 1), crypto.SHA256, datos, firma); err != nil {
		t.Fatalf("la firma buena tiene que pasar: %v", err)
	}
	malos := map[string]asn1.RawValue{
		"sin parámetros":                   {},
		"resumen distinto del de la firma": paramsPSS(t, oidSHA384, oidSHA256, sha256.Size, 1),
		"MGF1 con otro resumen":            paramsPSS(t, oidSHA256, oidSHA384, sha256.Size, 1),
		"trailerField que no es 1":         paramsPSS(t, oidSHA256, oidSHA256, sha256.Size, 2),
		"sal imposible":                    paramsPSS(t, oidSHA256, oidSHA256, clave.Size()+1, 1),
		"basura":                           {FullBytes: []byte{0x30, 0x03, 0x01, 0x01, 0xff}},
	}
	for nombre, p := range malos {
		if err := comprobarPSS(&clave.PublicKey, p, crypto.SHA256, datos, firma); err == nil {
			t.Errorf("%s: se aceptó", nombre)
		}
	}
	if err := comprobarPSS(&clave.PublicKey, paramsPSS(t, oidSHA256, oidSHA256, sha256.Size, 1), crypto.SHA256, []byte("otros datos"), firma); !errors.Is(err, ErrNoCorresponde) {
		t.Errorf("datos distintos: %v", err)
	}
}
