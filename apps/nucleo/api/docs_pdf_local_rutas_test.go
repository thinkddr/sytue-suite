// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"errors"
	"image"
	"image/png"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

// Las rutas de `localMotor` de P-03 (capas de `/pdf/pages`, `/pdf/sign/prepare` y
// `/complete`), P-05 (sello), P-06 (anclas) y P-07 (`/pdf/ocr`), cada una con sus PDF de
// prueba y sin red hacia el servicio (`sinRed`).

func motorLocal(t *testing.T) *PDF {
	t.Helper()
	pdf, err := NewPDF("https://docs.invalid", "https://drive.invalid", nil, sinRed(t)).WithPrivateDrive(&privatePDFDataFixture{})
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

func textoDe(t *testing.T, datos []byte) string {
	t.Helper()
	doc, err := localpdf.LeerPDF(datos)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := range doc.NumPaginas() {
		s, err := doc.TextoPlano(i)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(s + "\n")
	}
	return b.String()
}

func TestLocalMotorWritesPageLayersOnDevice(t *testing.T) {
	pdf := motorLocal(t)
	doc := PDFFichero{Nombre: "a.pdf", Datos: dosPaginas("Primera hoja", "Segunda hoja")}
	original := bytes.Clone(doc.Datos)
	casos := []struct {
		campos map[string]string
		quiere []string
	}{
		{map[string]string{"op": "marca-agua", "texto": "BORRADOR"}, []string{"BORRADOR"}},
		{map[string]string{"op": "fondo", "texto": "COPIA"}, []string{"COPIA"}},
		{map[string]string{"op": "numerar", "posicion": "superior-derecha"}, []string{"1 / 2", "2 / 2"}},
		{map[string]string{"op": "bates", "texto": "ACME-", "desde": "7", "digitos": "4"}, []string{"ACME-0007", "ACME-0008"}},
		{map[string]string{"op": "encabezado", "texto": "Confidencial {pagina}"}, []string{"Confidencial 1", "Confidencial 2"}},
		{map[string]string{"op": "pie", "texto": "Pie de página"}, []string{"Pie de página"}},
	}
	for _, c := range casos {
		r, err := pdf.Motor(t.Context(), "/pdf/pages", c.campos, nil, doc)
		if err != nil {
			t.Fatal(c.campos["op"], err)
		}
		if r.Cabecera.Get("Content-Type") != "application/pdf" {
			t.Fatal(c.campos["op"], "no devolvió un PDF")
		}
		texto := textoDe(t, r.Datos)
		for _, q := range c.quiere {
			if !strings.Contains(texto, q) {
				t.Fatalf("%s: falta %q en %q", c.campos["op"], q, texto)
			}
		}
	}
	if !bytes.Equal(doc.Datos, original) {
		t.Fatal("una capa cambió el documento de entrada")
	}
	var e *Error
	for _, mal := range []map[string]string{
		{"op": "marca-agua"},
		{"op": "pie", "texto": strings.Repeat("x", 501)},
		{"op": "bates", "digitos": "40"},
	} {
		if _, err := pdf.Motor(t.Context(), "/pdf/pages", mal, nil, doc); !errors.As(err, &e) || e.Status != http.StatusBadRequest {
			t.Fatal("una capa mal pedida es 400", mal, err)
		}
	}
	if _, err := pdf.Motor(t.Context(), "/pdf/pages", map[string]string{"op": "fondo", "texto": "x"}, nil, doc, doc); !errors.As(err, &e) || e.Status != http.StatusBadRequest || !strings.Contains(e.Body, "single PDF") {
		t.Fatal("una capa va sobre un solo documento", err)
	}
	// Las operaciones de página de siempre siguen yendo por su camino.
	if r, err := pdf.Motor(t.Context(), "/pdf/pages", map[string]string{"op": "extraer", "paginas": "2"}, nil, doc); err != nil || !strings.Contains(textoDe(t, r.Datos), "Segunda") {
		t.Fatal("extraer dejó de funcionar", err)
	}
}

// identidadDePrueba es un certificado de firma autofirmado y su clave.
func identidadDePrueba(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "Ana Pérez"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, clave
}

// firmarComoElSistema es lo que hace un almacén: firmar el resumen con la clave.
func firmarComoElSistema(t *testing.T, clave *ecdsa.PrivateKey, prep PDFFirmaPreparada) []byte {
	t.Helper()
	firma, err := ecdsa.SignASN1(rand.Reader, clave, prep.Resumen)
	if err != nil {
		t.Fatal(err)
	}
	return firma
}

func TestLocalMotorSignsWithASystemCertificateInTwoSteps(t *testing.T) {
	pdf := motorLocal(t)
	cert, clave := identidadDePrueba(t)
	doc := PDFFichero{Nombre: "contrato.pdf", Datos: dosPaginas("Contrato", "Firmas")}
	opts := PDFFirmaOpciones{Motivo: "Conforme", Lugar: "Bilbao", Rect: [4]float64{50, 50, 250, 110}}

	prep, err := pdf.PrepararFirma(t.Context(), doc, "", opts, [][]byte{cert.Raw})
	if err != nil {
		t.Fatal(err)
	}
	if prep.ID == "" || prep.Algoritmo != "ecdsa" || prep.Firmante.Nombre != "Ana Pérez" || prep.Paginas != 2 || prep.Sello != "" || len(prep.Resumen) != 32 {
		t.Fatalf("preparación incompleta: %+v", prep)
	}
	firma := firmarComoElSistema(t, clave, prep)

	// Un preparado tocado no se firma, y la preparación sigue viva para el bueno.
	tocado := prep
	tocado.Preparado = append(bytes.Clone(prep.Preparado), ' ')
	if _, _, err := pdf.CompletarFirma(t.Context(), tocado, firma, PDFFirmaDER, false); !errorCon(err, "pdf_prepared_mismatch") {
		t.Fatal("un preparado alterado se rechaza", err)
	}
	// Una firma de otra clave tampoco.
	_, otra := identidadDePrueba(t)
	if _, _, err := pdf.CompletarFirma(t.Context(), prep, firmarComoElSistema(t, otra, prep), PDFFirmaDER, false); !errorCon(err, "pdf_signature_mismatch") {
		t.Fatal("una firma de otra clave se rechaza", err)
	}

	firmado, sellado, err := pdf.CompletarFirma(t.Context(), prep, firma, PDFFirmaDER, false)
	if err != nil || sellado {
		t.Fatal("completar sin autoridad configurada", sellado, err)
	}
	// Gastada: la segunda vez ya no está.
	if _, _, err := pdf.CompletarFirma(t.Context(), prep, firma, PDFFirmaDER, false); !PDFFirmaCaducada(err) {
		t.Fatal("una preparación se gasta al completar", err)
	}

	// Sin anclas, la identidad no se juzga y se dice.
	r, err := pdf.Motor(t.Context(), "/pdf/verify", nil, nil, PDFFichero{Nombre: "f.pdf", Datos: firmado})
	if err != nil || !strings.Contains(string(r.Datos), `"anclas":false`) || !strings.Contains(string(r.Datos), "sin_anclas") || !strings.Contains(string(r.Datos), "Ana Pérez") {
		t.Fatal("verificar sin anclas", string(r.Datos), err)
	}
	// Con el certificado entre las anclas de la persona (P-06), sí.
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	pdf.ConLocal(PDFLocal{Anclas: func() *x509.CertPool { return pool }})
	r, err = pdf.Motor(t.Context(), "/pdf/verify", nil, nil, PDFFichero{Nombre: "f.pdf", Datos: firmado})
	if err != nil || !strings.Contains(string(r.Datos), `"anclas":true`) || strings.Contains(string(r.Datos), "sin_anclas") || strings.Contains(string(r.Datos), "no_confiable") {
		t.Fatal("verificar con anclas", string(r.Datos), err)
	}
	// Un pool vacío cuenta como «sin anclas».
	pdf.ConLocal(PDFLocal{Anclas: x509.NewCertPool})
	if r, err = pdf.Motor(t.Context(), "/pdf/verify", nil, nil, PDFFichero{Nombre: "f.pdf", Datos: firmado}); err != nil || !strings.Contains(string(r.Datos), `"anclas":false`) {
		t.Fatal("una carpeta sin certificados no es tener anclas", string(r.Datos), err)
	}
}

func TestLocalSignPreparationsAreBounded(t *testing.T) {
	pdf := motorLocal(t)
	cert, _ := identidadDePrueba(t)
	doc := PDFFichero{Nombre: "a.pdf", Datos: dosPaginas("Uno")}
	for i := range localPreparedMax {
		if _, err := pdf.PrepararFirma(t.Context(), doc, "", PDFFirmaOpciones{}, [][]byte{cert.Raw}); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, err := pdf.PrepararFirma(t.Context(), doc, "", PDFFirmaOpciones{}, [][]byte{cert.Raw}); !errorCon(err, "pdf_signature_too_many") {
		t.Fatal("hay un tope de preparaciones abiertas", err)
	}
	// La caducidad libera hueco.
	pdf.signatures.mu.Lock()
	for id, item := range pdf.signatures.items {
		item.expires = time.Now().Add(-time.Second)
		pdf.signatures.items[id] = item
	}
	pdf.signatures.mu.Unlock()
	if _, err := pdf.PrepararFirma(t.Context(), doc, "", PDFFirmaOpciones{}, [][]byte{cert.Raw}); err != nil {
		t.Fatal("las caducadas no cuentan", err)
	}
	// Sin certificado, o con algo que no lo es.
	if _, err := pdf.Motor(t.Context(), "/pdf/sign/prepare", map[string]string{"firma": "{}"}, []PDFParte{{Campo: "certificado", Datos: []byte("no")}}, doc); err == nil {
		t.Fatal("un certificado ilegible se rechaza")
	}
	if _, err := pdf.Motor(t.Context(), "/pdf/sign/prepare", map[string]string{"firma": `{"Clave":"x"}`}, []PDFParte{{Campo: "certificado", Datos: cert.Raw}}, doc); err == nil {
		t.Fatal("la firma no acepta campos de más")
	}
}

func errorCon(err error, codigo string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code() == codigo
}

// autoridadDePrueba es una TSA falsa que habla RFC 3161 y sella lo que se le pide.
func autoridadDePrueba(t *testing.T, peticiones *atomic.Int32) *httptest.Server {
	t.Helper()
	type huella struct {
		Algoritmo pkix.AlgorithmIdentifier
		Huella    []byte
	}
	type peticion struct {
		Version          int
		HuellaMensaje    huella
		Politica         asn1.ObjectIdentifier `asn1:"optional"`
		Nonce            *big.Int              `asn1:"optional"`
		PedirCertificado bool                  `asn1:"optional,default:false"`
	}
	type info struct {
		Version       int
		Politica      asn1.ObjectIdentifier
		HuellaMensaje huella
		Serie         *big.Int
		Generado      time.Time
		Nonce         *big.Int `asn1:"optional"`
	}
	type datos struct {
		Tipo  asn1.ObjectIdentifier
		Datos asn1.RawValue `asn1:"optional,tag:0"`
	}
	type firmado struct {
		Version           int
		AlgoritmosResumen asn1.RawValue `asn1:"set"`
		Contenido         datos
		Firmantes         asn1.RawValue `asn1:"set"`
	}
	type contenido struct {
		Tipo      asn1.ObjectIdentifier
		Contenido asn1.RawValue `asn1:"optional,tag:0"`
	}
	type estado struct{ Estado int }
	type respuesta struct {
		Estado estado
		Token  asn1.RawValue `asn1:"optional"`
	}
	debe := func(b []byte, err error) []byte {
		if err != nil {
			t.Error(err)
		}
		return b
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peticiones.Add(1)
		cuerpo, _ := io.ReadAll(r.Body)
		var p peticion
		if _, err := asn1.Unmarshal(cuerpo, &p); err != nil || r.Header.Get("Content-Type") != "application/timestamp-query" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		tst := debe(asn1.Marshal(info{
			Version: 1, Politica: asn1.ObjectIdentifier{1, 2, 3}, HuellaMensaje: p.HuellaMensaje,
			Serie: big.NewInt(1), Generado: time.Now().UTC().Truncate(time.Second), Nonce: p.Nonce,
		}))
		sobre := debe(asn1.Marshal(tst))
		cms := debe(asn1.Marshal(firmado{
			Version:           3,
			AlgoritmosResumen: asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true},
			Contenido:         datos{Tipo: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 4}, Datos: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: sobre}},
			Firmantes:         asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true},
		}))
		token := debe(asn1.Marshal(contenido{
			Tipo:      asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2},
			Contenido: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: cms},
		}))
		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(debe(asn1.Marshal(respuesta{Token: asn1.RawValue{FullBytes: token}})))
	}))
}

func TestLocalSignTimestampIsOptionalAndOnlyWhereConfigured(t *testing.T) {
	var peticiones atomic.Int32
	tsa := autoridadDePrueba(t, &peticiones)
	defer tsa.Close()
	pdf := motorLocal(t)
	cert, clave := identidadDePrueba(t)
	doc := PDFFichero{Nombre: "a.pdf", Datos: dosPaginas("Sellar")}
	direccion := tsa.URL + "/tsa"
	pdf.ConLocal(PDFLocal{Sello: func() localpdf.OpcionesSello { return localpdf.OpcionesSello{URL: direccion, Timeout: 2 * time.Second} }})

	prep, err := pdf.PrepararFirma(t.Context(), doc, "", PDFFirmaOpciones{}, [][]byte{cert.Raw})
	if err != nil || prep.Sello != "127.0.0.1" {
		t.Fatal("la preparación dice qué autoridad sellará", prep.Sello, err)
	}
	firmado, sellado, err := pdf.CompletarFirma(t.Context(), prep, firmarComoElSistema(t, clave, prep), PDFFirmaDER, false)
	if err != nil || !sellado || peticiones.Load() != 1 {
		t.Fatal("con autoridad, la firma va sellada", sellado, peticiones.Load(), err)
	}
	r, err := pdf.Motor(t.Context(), "/pdf/verify", nil, nil, PDFFichero{Nombre: "f.pdf", Datos: firmado})
	var informe struct {
		Firmas []struct {
			Sello *struct {
				Fecha string `json:"fecha"`
			} `json:"sello"`
		} `json:"firmas"`
	}
	if err != nil || json.Unmarshal(r.Datos, &informe) != nil || len(informe.Firmas) != 1 || informe.Firmas[0].Sello == nil {
		t.Fatal("el sello no quedó en la firma", string(r.Datos), err)
	}

	// La firma con fichero de certificado también lleva el sello de la autoridad elegida.
	claveDER, err := x509.MarshalECPrivateKey(clave)
	if err != nil {
		t.Fatal(err)
	}
	partes := []PDFParte{
		{Campo: "certificado", Nombre: "c.pem", Datos: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})},
		{Campo: "clave_priv", Nombre: "k.pem", Datos: pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: claveDER})},
	}
	r, err = pdf.Motor(t.Context(), "/pdf/sign", map[string]string{"op": "certificado", "firma": `{"Pagina":0}`}, partes, doc)
	if err != nil || r.Cabecera.Get("X-Sytue-Sello") != "si" || peticiones.Load() != 2 {
		t.Fatal("firma con fichero y sello", r.Cabecera, peticiones.Load(), err)
	}

	// La autoridad no responde: no se firma a escondidas sin sello; la misma firma vale
	// para seguir sin él.
	direccion = "http://127.0.0.1:1/tsa"
	prep, err = pdf.PrepararFirma(t.Context(), doc, "", PDFFirmaOpciones{}, [][]byte{cert.Raw})
	if err != nil {
		t.Fatal(err)
	}
	firma := firmarComoElSistema(t, clave, prep)
	if _, _, err := pdf.CompletarFirma(t.Context(), prep, firma, PDFFirmaDER, false); !PDFSelloCaido(err) {
		t.Fatal("una autoridad caída se dice", err)
	}
	if _, sellado, err := pdf.CompletarFirma(t.Context(), prep, firma, PDFFirmaDER, true); err != nil || sellado {
		t.Fatal("tras la autoridad caída se puede seguir sin sello", sellado, err)
	}

	// Con fichero de certificado, una autoridad caída no tumba la firma, y se dice.
	r, err = pdf.Motor(t.Context(), "/pdf/sign", map[string]string{"op": "certificado", "firma": `{"Pagina":0}`}, partes, doc)
	if err != nil || r.Cabecera.Get("X-Sytue-Sello") != "no" {
		t.Fatal("firma con fichero y autoridad caída", r.Cabecera, err)
	}

	// Sin autoridad configurada no se abre ninguna conexión.
	direccion = ""
	antes := peticiones.Load()
	prep, err = pdf.PrepararFirma(t.Context(), doc, "", PDFFirmaOpciones{}, [][]byte{cert.Raw})
	if err != nil || prep.Sello != "" {
		t.Fatal(prep.Sello, err)
	}
	if _, sellado, err := pdf.CompletarFirma(t.Context(), prep, firmarComoElSistema(t, clave, prep), PDFFirmaDER, false); err != nil || sellado || peticiones.Load() != antes {
		t.Fatal("sin autoridad no hay sello ni red", sellado, err)
	}
}

// reconocedorFalso devuelve una palabra por imagen.
type reconocedorFalso struct {
	llamadas int
	idiomas  string
	fallo    error
}

func (r *reconocedorFalso) Idiomas(context.Context) ([]string, error) {
	return []string{"eng", "spa"}, r.fallo
}

func (r *reconocedorFalso) Reconocer(_ context.Context, imagen []byte, tipo, idiomas string) ([]PDFPalabra, error) {
	r.llamadas++
	r.idiomas = idiomas
	if r.fallo != nil {
		return nil, r.fallo
	}
	if len(imagen) == 0 || tipo == "" {
		return nil, errors.New("sin imagen")
	}
	return []PDFPalabra{{Texto: "Escaneado", X: 10, Y: 10, Ancho: 120, Alto: 20}}, nil
}

func TestLocalMotorRecognisesScansWithTheDeviceOCR(t *testing.T) {
	pdf := motorLocal(t)
	var foto bytes.Buffer
	if err := png.Encode(&foto, image.NewRGBA(image.Rect(0, 0, 400, 300))); err != nil {
		t.Fatal(err)
	}
	escaneo, err := pdf.Motor(t.Context(), "/pdf/tools", map[string]string{"op": "imagenes-a-pdf"}, nil, PDFFichero{Nombre: "hoja.png", Datos: foto.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	doc := PDFFichero{Nombre: "escaneo.pdf", Datos: escaneo.Datos}

	// Sin reconocedor: la herramienta no se ofrece y reconocer lo dice.
	r, err := pdf.Motor(t.Context(), "/pdf/ocr", map[string]string{"op": "estado"}, nil, doc)
	if err != nil || !strings.Contains(string(r.Datos), `"disponible":false`) || !strings.Contains(string(r.Datos), `"paginas_sin_texto":[0]`) {
		t.Fatal("estado sin reconocedor", string(r.Datos), err)
	}
	if _, err := pdf.Motor(t.Context(), "/pdf/ocr", map[string]string{"op": "reconocer"}, nil, doc); !errorCon(err, "ocr_not_configured") {
		t.Fatal("reconocer sin reconocedor", err)
	}

	rec := &reconocedorFalso{}
	pdf.ConLocal(PDFLocal{Reconocedor: rec})
	r, err = pdf.Motor(t.Context(), "/pdf/ocr", map[string]string{"op": "estado"}, nil, doc)
	if err != nil || !strings.Contains(string(r.Datos), `"disponible":true`) || !strings.Contains(string(r.Datos), `"idiomas":["eng","spa"]`) {
		t.Fatal("estado con reconocedor", string(r.Datos), err)
	}
	r, err = pdf.Motor(t.Context(), "/pdf/ocr", map[string]string{"op": "reconocer", "idiomas": "spa+eng"}, nil, doc)
	if err != nil || r.Cabecera.Get("X-Sytue-Ocr-Paginas") != "1" || rec.idiomas != "spa+eng" {
		t.Fatal("reconocer", r.Cabecera, err)
	}
	if !strings.Contains(textoDe(t, r.Datos), "Escaneado") {
		t.Fatal("la capa de texto no quedó escrita")
	}
	// Lo ya reconocido no se vuelve a ofrecer.
	if _, err := pdf.Motor(t.Context(), "/pdf/ocr", map[string]string{"op": "reconocer"}, nil, PDFFichero{Nombre: "x.pdf", Datos: r.Datos}); !errorCon(err, "pdf_already_has_text") {
		t.Fatal("un documento con texto en todas sus páginas", err)
	}
	// Un reconocedor que se cae no deja un PDF a medias.
	rec.fallo = ErrOCRNoDisponible
	if _, err := pdf.Motor(t.Context(), "/pdf/ocr", map[string]string{"op": "reconocer"}, nil, doc); !errorCon(err, "ocr_not_configured") {
		t.Fatal("un reconocedor que ya no está", err)
	}
	// `capa` escribe un texto reconocido fuera.
	r, err = pdf.Motor(t.Context(), "/pdf/ocr", map[string]string{"op": "capa", "textos": `{"0":[{"texto":"Fuera","rect":[10,10,100,30]}]}`}, nil, doc)
	if err != nil || !strings.Contains(textoDe(t, r.Datos), "Fuera") {
		t.Fatal("capa", err)
	}
}
