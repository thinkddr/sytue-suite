// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
)

// LO QUE EL DRIVE PRIVADO NO PODÍA (ADR 0086, cuarta entrega): sellos y marcas,
// comparar, verificar, adjuntos y la firma con el certificado del sistema en dos
// pasos, todo en local y sin tocar HTTP.

func privatePDFWithoutHTTP(t *testing.T) (*PDF, *int) {
	t.Helper()
	requests := 0
	private, err := NewPDF("https://docs.invalid", "https://drive.invalid", nil, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("private PDF reached HTTP")
	})}).WithPrivateDrive(&privatePDFDataFixture{})
	if err != nil {
		t.Fatal(err)
	}
	return private, &requests
}

func pdfDeTexto(textos ...string) []byte {
	w := pdfsimple.Nuevo()
	for i, txt := range textos {
		if i > 0 {
			w.NuevaPagina()
		}
		w.Texto(txt, "F1", 12)
	}
	return w.Bytes()
}

func TestPrivatePDFStampsComparesAndVerifiesLocally(t *testing.T) {
	private, requests := privatePDFWithoutHTTP(t)
	doc := PDFFichero{Nombre: "privado.pdf", Datos: pdfDeTexto("uno", "dos")}

	sellado, err := private.Motor(t.Context(), "/pdf/pages", map[string]string{"op": "marca-agua", "texto": "CONFIDENCIAL"}, nil, doc)
	if err != nil {
		t.Fatalf("marca de agua: %v", err)
	}
	leido, err := localpdf.LeerPDF(sellado.Datos)
	if err != nil {
		t.Fatal(err)
	}
	if texto, _ := leido.TextoPlano(1); !strings.Contains(texto, "CONFIDENCIAL") {
		t.Fatalf("la marca no está en la página 2: %q", texto)
	}
	if _, err := private.Motor(t.Context(), "/pdf/pages", map[string]string{"op": "pie"}, nil, doc); err == nil {
		t.Fatal("un pie sin texto pasó")
	}

	otro := PDFFichero{Nombre: "otro.pdf", Datos: pdfDeTexto("uno", "tres")}
	comparado, err := private.Motor(t.Context(), "/pdf/compare", nil, nil, doc, otro)
	if err != nil {
		t.Fatalf("comparar: %v", err)
	}
	var diferencias struct {
		Diferencias []localpdf.DiferenciaPagina `json:"diferencias"`
		Iguales     bool                        `json:"iguales"`
	}
	if json.Unmarshal(comparado.Datos, &diferencias) != nil || diferencias.Iguales || len(diferencias.Diferencias) != 1 || diferencias.Diferencias[0].Pagina != 2 {
		t.Fatalf("comparación = %s", comparado.Datos)
	}
	visual, err := private.Motor(t.Context(), "/pdf/compare", map[string]string{"modo": "visual"}, nil, doc, otro)
	if err != nil || !bytes.Contains(visual.Datos, []byte(`"modo":"visual"`)) {
		t.Fatalf("comparación visual: %v %s", err, visual.Datos)
	}
	if _, err := private.Motor(t.Context(), "/pdf/compare", nil, nil, doc); err == nil {
		t.Fatal("comparar un solo documento")
	}

	informe, err := private.Motor(t.Context(), "/pdf/verify", nil, nil, doc)
	if err != nil || !bytes.Contains(informe.Datos, []byte(`"anclas":false`)) {
		t.Fatalf("verificar: %v %s", err, informe.Datos)
	}
	_, err = private.Motor(t.Context(), "/pdf/attachment", map[string]string{"nombre": "factura.xml"}, nil, doc)
	var e *Error
	if !errors.As(err, &e) || e.Code() != "not_found" {
		t.Fatalf("adjunto que no está: %v", err)
	}
	if *requests != 0 {
		t.Fatalf("el Drive privado habló con el servicio %d veces", *requests)
	}
}

// La firma con el certificado del SISTEMA en el dispositivo: prepara, el «sistema»
// (una clave en memoria) firma el resumen y se completa sin sello, anexando, y la
// firma verifica. Un preparado tocado o una firma de otra clave no pasan, y la
// preparación se gasta al completar.
func TestPrivatePDFSignsWithTheSystemCertificateInTwoLocalSteps(t *testing.T) {
	private, requests := privatePDFWithoutHTTP(t)
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(9),
		Subject:      pkix.Name{CommonName: "Titular de la tarjeta"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	blanco := PDFEnBlanco()
	doc := PDFFichero{Nombre: "privado.pdf", Datos: blanco}
	prep, err := private.PrepararFirma(t.Context(), doc, "", PDFFirmaOpciones{Motivo: "Conforme", Bloquear: true}, [][]byte{der})
	if err != nil {
		t.Fatalf("preparar: %v", err)
	}
	if prep.ID == "" || prep.Algoritmo != "ecdsa" || prep.Sello != "" || prep.Firmante.Nombre != "Titular de la tarjeta" || len(prep.Resumen) != 32 {
		t.Fatalf("preparada = %+v", prep)
	}

	// Una firma de OTRA clave no se acepta, y la preparación sigue viva.
	otra, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ajena, _ := otra.Sign(rand.Reader, prep.Resumen, crypto.SHA256)
	if _, _, err := private.CompletarFirma(t.Context(), prep, ajena, PDFFirmaDER, false); err == nil {
		t.Fatal("una firma de otra clave completó")
	}
	firma, err := clave.Sign(rand.Reader, prep.Resumen, crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	tocado := prep
	tocado.Preparado = append(bytes.Clone(prep.Preparado), ' ')
	if _, _, err := private.CompletarFirma(t.Context(), tocado, firma, PDFFirmaDER, false); err == nil {
		t.Fatal("un preparado tocado completó")
	}
	firmado, sellado, err := private.CompletarFirma(t.Context(), prep, firma, PDFFirmaDER, false)
	if err != nil {
		t.Fatalf("completar: %v", err)
	}
	if sellado || !bytes.HasPrefix(firmado, blanco) {
		t.Fatalf("sellado = %v, anexado = %v", sellado, bytes.HasPrefix(firmado, blanco))
	}
	leido, err := localpdf.LeerPDF(firmado)
	if err != nil {
		t.Fatal(err)
	}
	anclas := x509.NewCertPool()
	cert, _ := x509.ParseCertificate(der)
	anclas.AddCert(cert)
	firmas := leido.VerificarFirmas(localpdf.OpcionesVerificacion{Anclas: anclas})
	if len(firmas) != 1 || firmas[0].Veredicto != localpdf.VeredictoValida || firmas[0].Certificacion != 1 {
		t.Fatalf("firmas = %+v", firmas)
	}
	// Gastada: la misma preparación no firma dos veces.
	if _, _, err := private.CompletarFirma(t.Context(), prep, firma, PDFFirmaDER, false); !PDFFirmaCaducada(err) {
		t.Fatalf("completar dos veces: %v", err)
	}
	if _, err := private.Motor(t.Context(), "/pdf/sign/complete", map[string]string{"id": "x", "firma": base64.StdEncoding.EncodeToString(firma)}, nil, doc); !PDFFirmaCaducada(err) {
		t.Fatalf("una preparación que no existe: %v", err)
	}
	if *requests != 0 {
		t.Fatalf("la firma del sistema habló con el servicio %d veces", *requests)
	}
}
