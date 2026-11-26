// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Una autoridad que el sistema no conoce se autentica con su raíz en la carpeta de
// confianza; sin ella, su certificado de sellado no cuadra y no se sellaría.
func TestLasAnclasDelSelloSonLasDelSistemaYLasDeLaCarpeta(t *testing.T) {
	clave, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ahora := time.Now()
	raizPlantilla := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Raíz de prueba"},
		NotBefore: ahora.Add(-time.Hour), NotAfter: ahora.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign,
	}
	raizDER, err := x509.CreateCertificate(rand.Reader, raizPlantilla, raizPlantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	raiz, _ := x509.ParseCertificate(raizDER)
	tsaPlantilla := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "TSA de prueba"},
		NotBefore: ahora.Add(-time.Hour), NotAfter: ahora.Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
	}
	tsaDER, err := x509.CreateCertificate(rand.Reader, tsaPlantilla, raiz, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	tsa, _ := x509.ParseCertificate(tsaDER)
	opts := func(p *x509.CertPool) x509.VerifyOptions {
		return x509.VerifyOptions{Roots: p, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping}}
	}
	vacia := t.TempDir()
	if _, err := tsa.Verify(opts(anclasDeSello(vacia))); err == nil {
		t.Fatal("una autoridad desconocida no debería autenticarse")
	}
	carpeta := t.TempDir()
	if err := os.WriteFile(filepath.Join(carpeta, "raiz.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raizDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := tsa.Verify(opts(anclasDeSello(carpeta))); err != nil {
		t.Fatalf("con su raíz en la carpeta debería autenticarse: %v", err)
	}
}

// La autoridad elegida, y solo ella, pasa el portero; en HTTP plano también (RFC 3161),
// porque su token se autentica. Sin autoridad elegida, no sale nada.
func TestElSelloSalePorElPorteroSoloHaciaLaAutoridadElegida(t *testing.T) {
	p := red.Abrir(filepath.Join(t.TempDir(), "red.json"))
	a := &ajustesFirma{personal: prefsDePrueba{}, porDefecto: t.TempDir(), red: p}
	p.Fuente(a.destinoSello)
	if err := p.Permitido(red.SelloDeTiempo, "tsa.izenpe.com", 80, false); err == nil {
		t.Fatal("sin autoridad elegida, el portero deja salir")
	}
	a.ponerSello("http://tsa.izenpe.com", autoridadesDePartida)
	if err := p.Permitido(red.SelloDeTiempo, "tsa.izenpe.com", 80, false); err != nil {
		t.Fatalf("la autoridad elegida no pasa: %v", err)
	}
	if err := p.Permitido(red.SelloDeTiempo, "otra.example", 80, false); err == nil {
		t.Fatal("otra autoridad pasa")
	}
	if err := p.Permitido(red.IA, "tsa.izenpe.com", 80, false); err == nil {
		t.Fatal("la autoridad pasa para otro propósito")
	}
	if o := a.opcionesSello(); o.Cliente == nil || o.Anclas == nil {
		t.Fatal("el sello no sale por el portero o sin anclas")
	}
}
