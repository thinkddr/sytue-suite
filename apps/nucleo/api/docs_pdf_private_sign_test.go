// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

// TestPrivatePDFSignsWithCertificateOnDevice es «Firmar en el dispositivo» del nativo
// (ADR 0086): con el PDF en el Drive privado, la firma con certificado se hace en LOCAL,
// anexando en PAdES, y ni el documento ni la identidad salen al motor del servidor. La
// identidad se genera aquí (no hay claves en el repositorio); el firmado verifica.
func TestPrivatePDFSignsWithCertificateOnDevice(t *testing.T) {
	calls := 0
	original := NewPDF("https://docs.example", "https://drive.example", nil, &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}, Request: r}, nil
	})})
	private, err := original.WithPrivateDrive(&privatePDFDataFixture{})
	if err != nil {
		t.Fatal(err)
	}

	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: "Firmante del dispositivo"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(clave)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	clavePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

	blanco := PDFEnBlanco()
	respuesta, err := private.Motor(t.Context(), "/pdf/sign",
		map[string]string{"op": "certificado", "firma": `{"Motivo":"Conforme"}`},
		[]PDFParte{{Campo: "certificado", Datos: certPEM}, {Campo: "clave_priv", Datos: clavePEM}},
		PDFFichero{Nombre: "privado.pdf", Datos: blanco})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("la firma privada habló con el servicio %d veces", calls)
	}
	if respuesta.Cabecera.Get("X-Sytue-Firma-Anexada") != "true" || respuesta.Cabecera.Get("X-Sytue-Sello") != "no" {
		t.Fatalf("cabeceras = %v", respuesta.Cabecera)
	}
	if !bytes.Equal(respuesta.Datos[:len(blanco)], blanco) {
		t.Fatal("la firma no anexó: el original cambió")
	}
	doc, err := localpdf.LeerPDF(respuesta.Datos)
	if err != nil {
		t.Fatal(err)
	}
	anclas := x509.NewCertPool()
	cert, _ := x509.ParseCertificate(der)
	anclas.AddCert(cert)
	firmas := doc.VerificarFirmas(localpdf.OpcionesVerificacion{Anclas: anclas})
	if len(firmas) != 1 || !firmas[0].Integra || firmas[0].Veredicto != localpdf.VeredictoValida || firmas[0].Nombre != "Firmante del dispositivo" {
		t.Fatalf("firmas = %+v", firmas)
	}

	// Sin la clave no se firma nada, y tampoco se pregunta fuera.
	_, err = private.Motor(t.Context(), "/pdf/sign",
		map[string]string{"op": "certificado", "firma": `{}`},
		[]PDFParte{{Campo: "certificado", Datos: certPEM}},
		PDFFichero{Nombre: "privado.pdf", Datos: blanco})
	if err == nil || calls != 0 {
		t.Fatalf("una identidad incompleta firmó o salió: %v, %d", err, calls)
	}
}
