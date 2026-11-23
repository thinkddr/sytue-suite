// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// El relevo del sello para la firma del dispositivo (ADR 0086): solo deja pasar un
// resumen de 32 bytes, se lo manda a la autoridad configurada y devuelve el token tal
// cual, después de que `PedirSello` compruebe que sella ESA huella.

var (
	oidSHA256Prueba = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidTSTInfo      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 4}
	oidSignedData   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
)

type imprintPrueba struct {
	Alg    pkix.AlgorithmIdentifier
	Huella []byte
}

type peticionPrueba struct {
	Version   int
	Imprint   imprintPrueba
	Politica  asn1.ObjectIdentifier `asn1:"optional"`
	Nonce     *big.Int              `asn1:"optional"`
	PedirCert bool                  `asn1:"optional,default:false"`
}

type tstInfoPrueba struct {
	Version  int
	Politica asn1.ObjectIdentifier
	Imprint  imprintPrueba
	Serie    *big.Int
	Generado time.Time
	Nonce    *big.Int `asn1:"optional"`
}

type encapsuladoPrueba struct {
	Tipo  asn1.ObjectIdentifier
	Datos asn1.RawValue `asn1:"optional,tag:0,explicit"`
}

type signedDataPrueba struct {
	Version    int
	Algoritmos asn1.RawValue `asn1:"set"`
	Contenido  encapsuladoPrueba
	Firmantes  asn1.RawValue `asn1:"set"`
}

type contentInfoPrueba struct {
	Tipo      asn1.ObjectIdentifier
	Contenido asn1.RawValue
}

type respuestaPrueba struct {
	Estado struct{ Estado int }
	Token  asn1.RawValue `asn1:"optional"`
}

// tokenPrueba es un TimeStampToken con la estructura de verdad (sin firmar: sin
// anclas configuradas, `PedirSello` solo comprueba QUÉ sella).
func tokenPrueba(t *testing.T, huella []byte, nonce *big.Int) []byte {
	t.Helper()
	info, err := asn1.Marshal(tstInfoPrueba{
		Version: 1, Politica: asn1.ObjectIdentifier{1, 2, 3, 4},
		Imprint:  imprintPrueba{Alg: pkix.AlgorithmIdentifier{Algorithm: oidSHA256Prueba, Parameters: asn1.NullRawValue}, Huella: huella},
		Serie:    big.NewInt(7),
		Generado: time.Now().UTC().Truncate(time.Second),
		Nonce:    nonce,
	})
	if err != nil {
		t.Fatal(err)
	}
	sobre, _ := asn1.Marshal(info)
	sd, err := asn1.Marshal(signedDataPrueba{
		Version:    3,
		Algoritmos: asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true},
		Contenido: encapsuladoPrueba{Tipo: oidTSTInfo, Datos: asn1.RawValue{
			Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: sobre,
		}},
		Firmantes: asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := asn1.Marshal(contentInfoPrueba{Tipo: oidSignedData, Contenido: asn1.RawValue{
		Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: sd,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// autoridadPrueba sella lo que le pidan; `otra` hace que selle otra huella (el
// intermediario que cuela un token auténtico de otra cosa). Guarda lo que recibió.
func autoridadPrueba(t *testing.T, otra bool, recibido *[]byte, llamadas *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamadas.Add(1)
		cuerpo, _ := io.ReadAll(r.Body)
		*recibido = cuerpo
		var pet peticionPrueba
		if _, err := asn1.Unmarshal(cuerpo, &pet); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		huella := pet.Imprint.Huella
		if otra {
			huella = bytes.Repeat([]byte{7}, 32)
		}
		var resp respuestaPrueba
		resp.Token = asn1.RawValue{FullBytes: tokenPrueba(t, huella, pet.Nonce)}
		datos, _ := asn1.Marshal(resp)
		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(datos)
	}))
}

func peticionSello(t *testing.T, metodo, cuerpo string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(metodo, "/api/v1/pdf/timestamp", strings.NewReader(cuerpo))
	r.Header.Set("Content-Type", "application/json")
	return r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: "irati", Tenant: "acme"}))
}

func TestSelloDelDispositivoSinAutoridad(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	h.pdfSelloDisponible(rec, peticionSello(t, http.MethodGet, ""))
	var d selloDisponibleJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || d.Disponible || d.Autoridad != "" {
		t.Fatalf("disponible = %+v (%v)", d, err)
	}
	resumen := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	rec = httptest.NewRecorder()
	h.pdfSellar(rec, peticionSello(t, http.MethodPost, `{"resumen":"`+resumen+`"}`))
	if rec.Code != http.StatusNotFound || codigoDe(rec) != "pdf_timestamp_unconfigured" {
		t.Fatalf("= %d %s", rec.Code, rec.Body)
	}
}

func TestSelloDelDispositivoPorHTTP(t *testing.T) {
	var recibido []byte
	var llamadas atomic.Int32
	tsa := autoridadPrueba(t, false, &recibido, &llamadas)
	defer tsa.Close()
	h := &Handler{sello: domain.OpcionesSello{URL: tsa.URL, Timeout: 5 * time.Second}}

	rec := httptest.NewRecorder()
	h.pdfSelloDisponible(rec, peticionSello(t, http.MethodGet, ""))
	var d selloDisponibleJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || !d.Disponible || d.Autoridad != "127.0.0.1" {
		t.Fatalf("disponible = %+v (%v)", d, err)
	}

	firma := []byte("el valor de una firma hecha en el navegador")
	huella := sha256.Sum256(firma)
	rec = httptest.NewRecorder()
	h.pdfSellar(rec, peticionSello(t, http.MethodPost,
		`{"resumen":"`+base64.StdEncoding.EncodeToString(huella[:])+`"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("= %d %s", rec.Code, rec.Body)
	}
	var s selloJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil || len(s.Token) == 0 {
		t.Fatalf("token = %v (%v)", s, err)
	}
	// A la autoridad le llegó la huella y nada más que una TimeStampReq.
	var pet peticionPrueba
	if _, err := asn1.Unmarshal(recibido, &pet); err != nil || !bytes.Equal(pet.Imprint.Huella, huella[:]) || pet.Nonce == nil {
		t.Fatalf("la autoridad recibió %x (%v)", recibido, err)
	}
	if fecha, err := domain.FechaDelSello(s.Token); err != nil || fecha.IsZero() {
		t.Fatalf("el token no es un sello: %v", err)
	}

	t.Run("solo un resumen SHA-256", func(t *testing.T) {
		antes := llamadas.Load()
		for _, cuerpo := range []string{
			`{"resumen":"` + base64.StdEncoding.EncodeToString(firma) + `"}`,
			`{"resumen":""}`,
			`{"pdf":"JVBERi0="}`,
		} {
			rec := httptest.NewRecorder()
			h.pdfSellar(rec, peticionSello(t, http.MethodPost, cuerpo))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s = %d %s", cuerpo, rec.Code, rec.Body)
			}
		}
		if llamadas.Load() != antes {
			t.Error("una petición inválida llegó a la autoridad")
		}
	})
	t.Run("sin sesión, no", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/pdf/timestamp", strings.NewReader(`{}`))
		h.pdfSellar(rec, r)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("= %d", rec.Code)
		}
	})
}

func TestSelloDelDispositivoDeOtraHuellaNoPasa(t *testing.T) {
	var recibido []byte
	var llamadas atomic.Int32
	tsa := autoridadPrueba(t, true, &recibido, &llamadas)
	defer tsa.Close()
	h := &Handler{sello: domain.OpcionesSello{URL: tsa.URL, Timeout: 5 * time.Second}}
	huella := sha256.Sum256([]byte("firma"))
	rec := httptest.NewRecorder()
	h.pdfSellar(rec, peticionSello(t, http.MethodPost,
		`{"resumen":"`+base64.StdEncoding.EncodeToString(huella[:])+`"}`))
	if rec.Code != http.StatusBadGateway || codigoDe(rec) != "pdf_timestamp_unavailable" {
		t.Fatalf("= %d %s", rec.Code, rec.Body)
	}
}
