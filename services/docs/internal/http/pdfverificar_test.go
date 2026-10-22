// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// TestVerificarPorHTTP: el informe llega por la puerta, y la confianza depende de
// las anclas DEL DESPLIEGUE, no de lo que mande quien llama.
func TestVerificarPorHTTP(t *testing.T) {
	cert, clave := certificadoDeFirma(t)
	h := &Handler{firmas: &firmasEnMemoria{}}
	p := preparar(t, h, "irati", cert)
	firma, err := rsa.SignPKCS1v15(rand.Reader, clave, crypto.SHA256, p.Resumen)
	if err != nil {
		t.Fatal(err)
	}
	rec := completar(t, h, "irati", p, p.Preparado, firma, map[string]string{"sin_sello": "1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("completar: %d %s", rec.Code, rec.Body)
	}
	firmado := rec.Body.Bytes()

	pedir := func() (out struct {
		Firmas []domain.FirmaVerificada `json:"firmas"`
		Anclas bool                     `json:"anclas"`
	},
	) {
		rec := httptest.NewRecorder()
		h.pdfVerificar(rec, multipartFirma(t, "irati", "/api/v1/pdf/verify", nil, firmado, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("verify: %d %s", rec.Code, rec.Body)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	sin := pedir()
	if sin.Anclas || len(sin.Firmas) != 1 || !sin.Firmas[0].Integra || sin.Firmas[0].Veredicto != domain.VeredictoIndeterminada {
		t.Fatalf("sin anclas: %+v", sin)
	}
	h.firmaAnclas = x509.NewCertPool()
	h.firmaAnclas.AddCert(cert)
	con := pedir()
	if !con.Anclas || con.Firmas[0].Veredicto != domain.VeredictoValida || con.Firmas[0].Firmante.Sujeto != "Irati Etxeberria" {
		t.Fatalf("con anclas: %+v", con.Firmas[0])
	}
}

// TestPedirFirmaNoAceptaUnaFirmaDeAdorno: entregar como «firmado» un PDF con un
// diccionario de firma que cubre el fichero pero cuya firma no cuadra no puede
// cerrar la petición.
func TestPedirFirmaNoAceptaUnaFirmaDeAdorno(t *testing.T) {
	cert, clave := certificadoDeFirma(t)
	w := pdfsimple.Nuevo()
	w.Texto("contrato de servicios", "F1", 12)
	original := w.Bytes()
	doc, err := domain.LeerPDF(original)
	if err != nil {
		t.Fatal(err)
	}
	claveDER, _ := x509.MarshalPKCS8PrivateKey(clave)
	firmado, err := domain.FirmarConCertificadoPDF(doc, domain.OpcionesFirma{
		CertificadoPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}),
		ClavePEM:       pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: claveDER}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := firmaNueva(original, firmado); err != nil {
		t.Fatalf("una firma de verdad no se acepta: %v", err)
	}
	// Un dígito del valor de la firma, cerca del final del CMS: la estructura se
	// sigue leyendo, la firma ya no cuadra.
	i := bytes.Index(firmado, []byte("/Contents <")) + len("/Contents <")
	fin := i + bytes.Index(firmado[i:], bytes.Repeat([]byte("0"), 64))
	adorno := bytes.Clone(firmado)
	if adorno[fin-3] == '1' {
		adorno[fin-3] = '2'
	} else {
		adorno[fin-3] = '1'
	}
	if err := firmaNueva(original, adorno); err == nil {
		t.Fatal("un PDF con la firma rota se ha aceptado como firmado")
	}
}
