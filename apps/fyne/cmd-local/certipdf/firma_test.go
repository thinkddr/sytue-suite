// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/apps/certipdf"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/certificados"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
)

// prefsDePrueba es `local.Personal` en memoria.
type prefsDePrueba map[string]string

func (p prefsDePrueba) Leer(c string) string { return p[c] }
func (p prefsDePrueba) Poner(c, v string)    { p[c] = v }

func TestLaAutoridadDeSelloLaEligeLaPersona(t *testing.T) {
	a := &ajustesFirma{personal: prefsDePrueba{}, porDefecto: t.TempDir()}
	actual, lista := a.sello()
	if actual != "" || len(lista) == 0 || lista[0].URL != "http://tsa.izenpe.com" {
		t.Fatal("de partida: sin sello y con los preajustes", actual, lista)
	}
	if a.opcionesSello().URL != "" {
		t.Fatal("sin elegir, la firma no lleva sello")
	}
	otra := append(slices.Clone(lista), certipdf.AutoridadSello{Nombre: "tsa.example", URL: "https://tsa.example/tsr"})
	a.ponerSello("https://tsa.example/tsr", otra)
	actual, lista = a.sello()
	if actual != "https://tsa.example/tsr" || len(lista) != 2 || a.opcionesSello().URL != actual || a.opcionesSello().Timeout == 0 {
		t.Fatal("la lista es editable y la elegida se recuerda", actual, lista)
	}
	a.ponerSello("", nil)
	if _, lista = a.sello(); len(lista) != 0 {
		t.Fatal("la persona puede vaciar la lista", lista)
	}
}

func TestLasAnclasSeLeenDeLaCarpeta(t *testing.T) {
	dir := t.TempDir()
	a := &ajustesFirma{personal: prefsDePrueba{}, porDefecto: dir}
	if a.pool() != nil {
		t.Fatal("una carpeta vacía no son anclas")
	}
	c1, c2 := certificadoDePrueba(t, dir, "uno"), certificadoDePrueba(t, dir, "dos")
	der := filepath.Join(dir, "dos.der")
	if err := os.WriteFile(der, c2.Raw, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(dir, "dos.pem"))
	_ = os.WriteFile(filepath.Join(dir, "roto.pem"), []byte("-----BEGIN CERTIFICATE-----\nno\n-----END CERTIFICATE-----\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "notas.txt"), c1.Raw, 0o600)
	if _, n := a.anclas(); n != 2 {
		t.Fatal("se leen el PEM y el DER, y se salta lo demás", n)
	}
	otra := t.TempDir()
	a.personal.Poner(claveAnclas, otra)
	if a.carpetaAnclas() != otra || a.pool() != nil {
		t.Fatal("la carpeta elegida manda")
	}
}

// certificadoDePrueba genera con openssl una identidad RSA, la deja en `dir` como
// <nombre>.pem, <nombre>.key y <nombre>.p12 (clave «1234») y devuelve el certificado.
func certificadoDePrueba(t *testing.T, dir, nombre string) *x509.Certificate {
	t.Helper()
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("sin openssl")
	}
	base := filepath.Join(dir, nombre)
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(openssl, args...).CombinedOutput(); err != nil {
			t.Fatalf("openssl %v: %v\n%s", args, err, out)
		}
	}
	run("req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2", "-subj", "/CN=Persona "+nombre,
		"-addext", "keyUsage=digitalSignature,nonRepudiation", "-keyout", base+".key", "-out", base+".pem")
	run("pkcs12", "-export", "-inkey", base+".key", "-in", base+".pem", "-out", base+".p12", "-passout", "pass:1234")
	raw, err := os.ReadFile(base + ".pem")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := pem.Decode(raw)
	c, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// almacenPKCS12 es un almacén del sistema hecho con un `.p12` de fichero: en Linux, sin
// tarjeta, es como se prueba de punta a punta lo que harían el Llavero o CNG.
type almacenPKCS12 struct {
	ruta, clave string
}

func (a almacenPKCS12) abrir() ([]*x509.Certificate, crypto.Signer, error) {
	raw, err := os.ReadFile(a.ruta)
	if err != nil {
		return nil, nil, err
	}
	return localpdf.LeerPKCS12(raw, a.clave)
}

func (a almacenPKCS12) Certificados(context.Context) ([]certificados.Certificado, error) {
	cadena, _, err := a.abrir()
	if err != nil {
		return nil, err
	}
	return []certificados.Certificado{{Cadena: cadena, Almacen: "Fichero"}}, nil
}

func (a almacenPKCS12) Firmar(_ context.Context, _ certificados.Certificado, prep api.PDFFirmaPreparada, _ string) ([]byte, string, error) {
	_, clave, err := a.abrir()
	if err != nil {
		return nil, "", err
	}
	// Como CNG o el Llavero: el sistema firma el RESUMEN ya calculado.
	firma, err := clave.Sign(rand.Reader, prep.Resumen, crypto.SHA256)
	return firma, api.PDFFirmaDER, err
}

func TestFirmaConElCertificadoDelSistemaDePuntaAPunta(t *testing.T) {
	dir := t.TempDir()
	cert := certificadoDePrueba(t, dir, "ana")
	disco, err := archivos.Abrir(filepath.Join(dir, "Drive"), archivos.Opciones{Instalacion: "prueba", Autor: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := api.NewPDF("https://sin-servidor.invalid", "https://sin-servidor.invalid", nil, nil).WithPrivateDrive(disco)
	if err != nil {
		t.Fatal(err)
	}
	anclas := filepath.Join(dir, "anclas")
	if err := os.MkdirAll(anclas, 0o700); err != nil {
		t.Fatal(err)
	}
	ajustes := &ajustesFirma{personal: prefsDePrueba{}, porDefecto: anclas}
	pdf.ConLocal(api.PDFLocal{Sello: ajustes.opcionesSello, Anclas: ajustes.pool})

	// Un PDF suelto de otra carpeta, que se abre por su ruta y no se copia.
	suelto := filepath.Join(dir, "contrato.pdf")
	w := pdfsimple.Nuevo()
	w.Texto("Contrato de prueba", "F1", 14)
	original := w.Bytes()
	if err := os.WriteFile(suelto, original, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := certipdf.AbrirDocumento(t.Context(), pdf, suelto)
	if err != nil {
		t.Fatal(err)
	}
	alm := almacenPKCS12{ruta: filepath.Join(dir, "ana.p12"), clave: "1234"}
	certs, err := alm.Certificados(t.Context())
	if err != nil || len(certs) != 1 || !certs[0].Cadena[0].Equal(cert) {
		t.Fatal("el almacén de fichero", err)
	}
	prep, err := doc.PrepararFirma(t.Context(), api.PDFFirmaOpciones{Motivo: "Conforme", Rect: [4]float64{40, 40, 240, 100}}, certs[0].DER())
	if err != nil {
		t.Fatal(err)
	}
	if prep.Algoritmo != "rsa" || prep.Sello != "" || prep.Firmante.Nombre != "Persona ana" {
		t.Fatalf("preparación: %+v", prep)
	}
	firma, formato, err := alm.Firmar(t.Context(), certs[0], prep, "")
	if err != nil {
		t.Fatal(err)
	}
	firmado, sellado, err := pdf.CompletarFirma(t.Context(), prep, firma, formato, false)
	if err != nil || sellado {
		t.Fatal("completar sin autoridad", sellado, err)
	}
	f, err := doc.GuardarFirmado(t.Context(), firmado, "firmado")
	if err != nil || !strings.Contains(f.Name, "firmado") {
		t.Fatal("la copia firmada", f, err)
	}

	// Sin anclas: «no se sabe quién»; con el certificado en la carpeta, sí.
	r, err := pdf.Motor(t.Context(), "/pdf/verify", nil, nil, api.PDFFichero{Nombre: f.Name, Datos: firmado})
	if err != nil || !strings.Contains(string(r.Datos), `"anclas":false`) {
		t.Fatal(string(r.Datos), err)
	}
	if err := os.WriteFile(filepath.Join(anclas, "ana.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err = pdf.Motor(t.Context(), "/pdf/verify", nil, nil, api.PDFFichero{Nombre: f.Name, Datos: firmado})
	if err != nil || !strings.Contains(string(r.Datos), `"anclas":true`) || strings.Contains(string(r.Datos), "no_confiable") || strings.Contains(string(r.Datos), "sin_anclas") {
		t.Fatal("con el certificado entre las anclas", string(r.Datos), err)
	}
	// El original no se tocó.
	if raw, _ := os.ReadFile(suelto); !bytes.Equal(raw, original) {
		t.Fatal("firmar cambió el original")
	}
}
