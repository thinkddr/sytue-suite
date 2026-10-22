// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// Los dos pasos por HTTP. Lo que se prueba aquí es la LIGADURA —de una persona,
// de un solo uso, con el documento que se preparó— y qué pasa cuando algo sale
// mal a mitad; que la firma esté bien hecha ya lo validan los tests del motor y
// tres herramientas de fuera (`domain/pdfsigndospasos_externo_test.go`).

// firmasEnMemoria es el almacén de la 0007 sin base: una fila por preparación,
// con su dueño, su caducidad y su reserva.
type firmasEnMemoria struct {
	mu    sync.Mutex
	filas map[string]*filaFirma
	n     int
}

type filaFirma struct {
	f       store.FirmaPreparada
	enCurso bool
}

func (m *firmasEnMemoria) GuardarFirmaPreparada(_ context.Context, f store.FirmaPreparada, maximo int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	abiertas := 0
	for _, x := range m.filas {
		if x.f.Sujeto == f.Sujeto {
			abiertas++
		}
	}
	if abiertas >= maximo {
		return "", store.ErrDemasiadasFirmas
	}
	m.n++
	id := "00000000-0000-4000-8000-" + strconv.FormatInt(int64(100000000000+m.n), 10)
	f.ID = id
	if m.filas == nil {
		m.filas = map[string]*filaFirma{}
	}
	m.filas[id] = &filaFirma{f: f}
	return id, nil
}

func (m *firmasEnMemoria) TomarFirmaPreparada(_ context.Context, id, sujeto string, _ time.Duration) (store.FirmaPreparada, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	x, ok := m.filas[id]
	if !ok || x.f.Sujeto != sujeto || x.enCurso || time.Now().After(x.f.Caduca) {
		return store.FirmaPreparada{}, store.ErrFirmaNoDisponible
	}
	x.enCurso = true
	return x.f, nil
}

func (m *firmasEnMemoria) SoltarFirmaPreparada(_ context.Context, id string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if x, ok := m.filas[id]; ok {
		x.enCurso = false
	}
	return nil
}

func (m *firmasEnMemoria) GastarFirmaPreparada(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.filas, id)
	return nil
}

// certificadoDeFirma emite un certificado de firma autofirmado y devuelve su clave:
// «la tarjeta».
func certificadoDeFirma(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &x509.Certificate{
		SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "Irati Etxeberria"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, p, p, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	return c, k
}

// multipartFirma arma el formulario: los campos y, si vienen, el PDF y el certificado.
func multipartFirma(t *testing.T, sub, ruta string, campos map[string]string, pdf, cert []byte) *http.Request {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	if pdf != nil {
		p, _ := w.CreateFormFile("file", "contrato.pdf")
		_, _ = p.Write(pdf)
	}
	if cert != nil {
		p, _ := w.CreateFormFile("certificado", "firmante.der")
		_, _ = p.Write(cert)
	}
	for k, v := range campos {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	r := httptest.NewRequest(http.MethodPost, ruta, &b)
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: sub, Tenant: "acme"}))
}

func preparar(t *testing.T, h *Handler, sub string, cert *x509.Certificate) firmaPreparadaJSON {
	t.Helper()
	w := pdfsimple.Nuevo()
	w.Texto("contrato de servicios", "F1", 12)
	rec := httptest.NewRecorder()
	h.pdfFirmaPreparar(rec, multipartFirma(t, sub, "/api/v1/pdf/sign/prepare", map[string]string{
		// El nombre que diga la app no cuenta: lo pone el certificado.
		"firma": `{"Motivo":"Conforme","Lugar":"Donostia","Nombre":"Otra Persona"}`,
	}, w.Bytes(), cert.Raw))
	if rec.Code != http.StatusOK {
		t.Fatalf("preparar: %d %s", rec.Code, rec.Body)
	}
	var p firmaPreparadaJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func completar(t *testing.T, h *Handler, sub string, p firmaPreparadaJSON, pdf, firma []byte, extra map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	campos := map[string]string{"id": p.ID, "firma": base64.StdEncoding.EncodeToString(firma)}
	for k, v := range extra {
		campos[k] = v
	}
	rec := httptest.NewRecorder()
	h.pdfFirmaCompletar(rec, multipartFirma(t, sub, "/api/v1/pdf/sign/complete", campos, pdf, nil))
	return rec
}

func codigoDe(rec *httptest.ResponseRecorder) string {
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	return e.Error.Code
}

func TestFirmaDelSistemaPorHTTP(t *testing.T) {
	cert, clave := certificadoDeFirma(t)
	h := &Handler{firmas: &firmasEnMemoria{}}
	p := preparar(t, h, "irati", cert)
	if p.Firmante.Nombre != "Irati Etxeberria" || p.Nivel != "avanzada" || p.Algoritmo != "rsa" || p.Paginas != 1 {
		t.Errorf("lo que ve la persona: %+v", p.Firmante)
	}
	firma, err := rsa.SignPKCS1v15(rand.Reader, clave, crypto.SHA256, p.Resumen)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("otra persona no la completa", func(t *testing.T) {
		if rec := completar(t, h, "otro", p, p.Preparado, firma, nil); rec.Code != http.StatusConflict || codigoDe(rec) != "pdf_signature_expired" {
			t.Errorf("= %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("con otro documento, no", func(t *testing.T) {
		otro := bytes.Clone(p.Preparado)
		otro[20] ^= 1
		if rec := completar(t, h, "irati", p, otro, firma, nil); rec.Code != http.StatusConflict || codigoDe(rec) != "pdf_prepared_mismatch" {
			t.Errorf("= %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("con una firma de otra cosa, no", func(t *testing.T) {
		mala, _ := rsa.SignPKCS1v15(rand.Reader, clave, crypto.SHA256, bytes.Repeat([]byte{9}, 32))
		if rec := completar(t, h, "irati", p, p.Preparado, mala, nil); rec.Code != http.StatusBadRequest || codigoDe(rec) != "pdf_signature_mismatch" {
			t.Errorf("= %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("sin autoridad que responda, no firma en silencio", func(t *testing.T) {
		caida := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer caida.Close()
		h.sello = domain.OpcionesSello{URL: caida.URL, Timeout: time.Second}
		defer func() { h.sello = domain.OpcionesSello{} }()
		if rec := completar(t, h, "irati", p, p.Preparado, firma, nil); rec.Code != http.StatusBadGateway || codigoDe(rec) != "pdf_timestamp_unavailable" {
			t.Errorf("= %d %s", rec.Code, rec.Body)
		}
	})

	// Tras los tres fallos, la preparación sigue ahí y la MISMA firma vale: el
	// sello no entra en lo firmado, así que no hay que volver a pedir el PIN.
	rec := completar(t, h, "irati", p, p.Preparado, firma, map[string]string{"sin_sello": "1"})
	if rec.Code != http.StatusOK || rec.Header().Get("X-Sytue-Sello") != "no" {
		t.Fatalf("completar: %d %q %s", rec.Code, rec.Header().Get("X-Sytue-Sello"), rec.Body)
	}
	d, err := domain.LeerPDF(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("el PDF firmado no abre: %v", err)
	}
	if f := d.FirmasDelDocumento(); len(f) != 1 || f[0].Nombre != "Irati Etxeberria" || !f[0].CubreTodo {
		t.Errorf("firmas = %+v", f)
	}

	t.Run("una vez gastada, no vale otra vuelta", func(t *testing.T) {
		if rec := completar(t, h, "irati", p, p.Preparado, firma, map[string]string{"sin_sello": "1"}); rec.Code != http.StatusConflict {
			t.Errorf("= %d %s", rec.Code, rec.Body)
		}
	})
}

func TestPrepararExigeUnCertificadoQueFirme(t *testing.T) {
	h := &Handler{firmas: &firmasEnMemoria{}}
	w := pdfsimple.Nuevo()
	w.Texto("x", "F1", 12)
	rec := httptest.NewRecorder()
	h.pdfFirmaPreparar(rec, multipartFirma(t, "irati", "/api/v1/pdf/sign/prepare", map[string]string{"firma": `{}`}, w.Bytes(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("sin certificado: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.pdfFirmaPreparar(rec, multipartFirma(t, "irati", "/api/v1/pdf/sign/prepare", map[string]string{"firma": `{}`}, w.Bytes(), []byte("no es un certificado")))
	if rec.Code != http.StatusBadRequest || codigoDe(rec) != "pdf_bad_certificate" {
		t.Errorf("con basura: %d %s", rec.Code, rec.Body)
	}
}

func TestNadiePreparaFirmasSinFin(t *testing.T) {
	cert, _ := certificadoDeFirma(t)
	h := &Handler{firmas: &firmasEnMemoria{}}
	for range maxFirmasAbiertas {
		preparar(t, h, "irati", cert)
	}
	w := pdfsimple.Nuevo()
	w.Texto("x", "F1", 12)
	rec := httptest.NewRecorder()
	h.pdfFirmaPreparar(rec, multipartFirma(t, "irati", "/api/v1/pdf/sign/prepare", map[string]string{"firma": `{}`}, w.Bytes(), cert.Raw))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("una de más: %d %s", rec.Code, rec.Body)
	}
}

// El contrato del primer paso para quien firma (NAT-04): con `por_firmar`, `hueco` y
// `anexado` la app comprueba por su cuenta QUÉ firma —este documento, con este
// certificado— en vez de fiarse del resumen que le dan. Los nombres de los campos y lo
// que prometen quedan fijados aquí.
func TestPrepararDaConQueComprobarLoQueSeFirma(t *testing.T) {
	cert, _ := certificadoDeFirma(t)
	h := &Handler{firmas: &firmasEnMemoria{}}
	w := pdfsimple.Nuevo()
	w.Texto("contrato de servicios", "F1", 12)
	original := w.Bytes()
	doc, err := domain.LeerPDF(original)
	if err != nil {
		t.Fatal(err)
	}
	cifrado, err := domain.ProtegerPDF(doc, domain.OpcionesProteccion{ContrasenaUsuario: "abc", Permitir: domain.PermisosTodos()})
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		nombre  string
		pdf     []byte
		campos  map[string]string
		anexado bool
	}{
		{"documento que admite anexar", original, map[string]string{"firma": `{}`}, true},
		{"documento cifrado, que se reescribe", cifrado, map[string]string{"firma": `{}`, "clave": "abc"}, false},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.pdfFirmaPreparar(rec, multipartFirma(t, "irati", "/api/v1/pdf/sign/prepare", c.campos, c.pdf, cert.Raw))
			if rec.Code != http.StatusOK {
				t.Fatalf("preparar: %d %s", rec.Code, rec.Body)
			}
			// Por nombre de campo: es lo que la app va a leer.
			var p struct {
				Preparado []byte `json:"preparado"`
				Resumen   []byte `json:"resumen"`
				PorFirmar []byte `json:"por_firmar"`
				Hueco     []int  `json:"hueco"`
				Anexado   *bool  `json:"anexado"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if len(p.PorFirmar) == 0 || len(p.Hueco) != 2 || p.Anexado == nil {
				t.Fatalf("faltan campos: por_firmar de %d bytes, hueco = %v, anexado presente = %v",
					len(p.PorFirmar), p.Hueco, p.Anexado != nil)
			}
			inicio, fin := p.Hueco[0], p.Hueco[1]
			if inicio <= 0 || fin <= inicio || fin > len(p.Preparado) || p.Preparado[inicio] != '<' || p.Preparado[fin-1] != '>' {
				t.Fatalf("hueco = %v no delimita el /Contents con sus < > en un preparado de %d bytes", p.Hueco, len(p.Preparado))
			}
			if suma := sha256.Sum256(p.PorFirmar); !bytes.Equal(suma[:], p.Resumen) {
				t.Error("resumen no es el SHA-256 de por_firmar")
			}

			var atributos []struct {
				Tipo    asn1.ObjectIdentifier
				Valores []asn1.RawValue `asn1:"set"`
			}
			if resto, err := asn1.UnmarshalWithParams(p.PorFirmar, &atributos, "set"); err != nil || len(resto) != 0 {
				t.Fatalf("por_firmar no es el SET OF de atributos: %v (sobran %d bytes)", err, len(resto))
			}
			valor := map[string][]byte{}
			for _, a := range atributos {
				if len(a.Valores) != 1 {
					t.Fatalf("el atributo %v lleva %d valores", a.Tipo, len(a.Valores))
				}
				valor[a.Tipo.String()] = a.Valores[0].FullBytes
			}
			var tipo asn1.ObjectIdentifier
			if _, err := asn1.Unmarshal(valor["1.2.840.113549.1.9.3"], &tipo); err != nil || tipo.String() != "1.2.840.113549.1.7.1" {
				t.Errorf("contentType = %v (%v), esperaba id-data", tipo, err)
			}
			var resumenDoc []byte
			sinHueco := sha256.Sum256(append(bytes.Clone(p.Preparado[:inicio]), p.Preparado[fin:]...))
			if _, err := asn1.Unmarshal(valor["1.2.840.113549.1.9.4"], &resumenDoc); err != nil || !bytes.Equal(resumenDoc, sinHueco[:]) {
				t.Errorf("messageDigest no es el SHA-256 de preparado sin el hueco (%v)", err)
			}
			var firmante struct {
				Certs []struct {
					Huella []byte
					Emisor asn1.RawValue
				}
			}
			delCert := sha256.Sum256(cert.Raw)
			if _, err := asn1.Unmarshal(valor["1.2.840.113549.1.9.16.2.47"], &firmante); err != nil ||
				len(firmante.Certs) != 1 || !bytes.Equal(firmante.Certs[0].Huella, delCert[:]) {
				t.Errorf("signing-certificate-v2 no lleva el SHA-256 del certificado elegido (%v)", err)
			}

			if *p.Anexado != c.anexado {
				t.Errorf("anexado = %v, esperaba %v", *p.Anexado, c.anexado)
			}
			if *p.Anexado && !bytes.HasPrefix(p.Preparado, c.pdf) {
				t.Error("anexado, pero preparado no empieza por los bytes del documento tal como llegó")
			}
		})
	}
}
