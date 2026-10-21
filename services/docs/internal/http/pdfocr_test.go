// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/ocr"
)

// `disponible` decide si Sytue PDF ENSEÑA la herramienta de reconocer texto, y
// se responde preguntándole al servicio, no mirando si hay una URL puesta.
//
// El caso que lo motiva es el perfil piloto: allí Sytue OCR no se despliega
// —`ocr.enabled: false`, 2×512 MiB que ese hardware no tiene— pero el chart de
// docs pasa `SYTUE_DOCS_OCR_URL` igualmente, así que el cliente existe y apunta
// a un Service que no. Antes eso se traducía en un botón que solo fallaba al
// pulsarlo; ahora la herramienta se apaga sola y el resto del estudio sigue.

func TestOcrVivo_SinClienteNoHayHerramienta(t *testing.T) {
	t.Parallel()
	if vivo, idiomas := ocrVivo(context.Background(), nil, ""); vivo || idiomas != nil {
		t.Errorf("= (%v, %v), quiero (false, nil) sin cliente", vivo, idiomas)
	}
}

func TestOcrVivo_ServicioQueRespondeTraeSusIdiomas(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/languages" {
			t.Errorf("ruta consultada = %q", r.URL.Path)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"idiomas":["spa","eng"]}`))
	}))
	defer srv.Close()

	vivo, idiomas := ocrVivo(context.Background(), ocr.Nuevo(srv.URL, time.Second), "")
	if !vivo || len(idiomas) != 2 {
		t.Fatalf("= (%v, %v), quiero (true, [spa eng])", vivo, idiomas)
	}
}

// El caso del piloto: hay cliente porque hay URL, pero al otro lado no hay nadie.
func TestOcrVivo_ServicioAusenteApagaLaHerramienta(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // el puerto queda cerrado: es «configurado pero no desplegado»

	if vivo, _ := ocrVivo(context.Background(), ocr.Nuevo(url, time.Second), ""); vivo {
		t.Error("un servicio que no responde no puede contar como disponible")
	}
}

// Un OCR que contesta mal tampoco vale: enseñar el botón para que falle al
// pulsarlo es peor que no enseñarlo.
func TestOcrVivo_ErrorDelServicioApagaLaHerramienta(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if vivo, _ := ocrVivo(context.Background(), ocr.Nuevo(srv.URL, time.Second), ""); vivo {
		t.Error("un 500 del servicio no puede contar como disponible")
	}
}

// Y un OCR COLGADO no puede dejar el panel esperando: el sondeo tiene su propio
// tope, más corto que el de la petición entera.
func TestOcrVivo_ServicioColgadoNoBloqueaElPanel(t *testing.T) {
	t.Parallel()
	libre := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-libre:
		case <-r.Context().Done():
		}
	}))
	defer close(libre)
	defer srv.Close()

	// El cliente se construye con un tope mayor que el del sondeo, para que lo
	// que corte sea el sondeo y no el cliente.
	inicio := time.Now()
	vivo, _ := ocrVivo(context.Background(), ocr.Nuevo(srv.URL, time.Minute), "")
	if vivo {
		t.Error("un servicio colgado no puede contar como disponible")
	}
	if tardanza := time.Since(inicio); tardanza > sondeoOCR+2*time.Second {
		t.Errorf("el sondeo tardó %s; su tope es %s", tardanza, sondeoOCR)
	}
}

// Apagar el botón cubre el caso normal, pero NO cubre a quien pide reconocer de
// todas formas: la pestaña que llevaba abierta desde antes, la app de
// escritorio, un reintento. Ahí el fallo de red tiene que salir por la MISMA
// puerta que apagó el botón —503 `ocr_not_configured`— y no como un 502 con la
// dirección interna del clúster dentro del mensaje, que es lo que el BFF reenvía
// tal cual al navegador.

func TestErrorDeOCR_ServicioAusenteEsNoDisponible(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // «configurado pero no desplegado», igual que en el piloto

	_, err := ocr.Nuevo(url, time.Second).Reconocer(context.Background(), []byte("x"), "", "")
	if err == nil {
		t.Fatal("un servicio cerrado tiene que fallar")
	}
	var api *httpx.Error
	if !errors.As(errorDeOCR(err), &api) {
		t.Fatalf("errorDeOCR(%v) no devolvió un *httpx.Error", err)
	}
	if api.Status != http.StatusServiceUnavailable || api.Code != "ocr_not_configured" {
		t.Errorf("= %d/%s, quiero 503/ocr_not_configured", api.Status, api.Code)
	}
	if strings.Contains(api.Message, url) {
		t.Errorf("el mensaje que ve el usuario lleva la URL interna: %q", api.Message)
	}
}

func TestErrorDeOCR_NoFiltraElFalloCrudo(t *testing.T) {
	t.Parallel()
	// Lo que responda el otro servicio es para el REGISTRO, no para la pantalla de
	// quien está firmando un contrato.
	crudo := "http://sytue-ocr.interno.svc:8080 se ha roto"
	var api *httpx.Error
	if !errors.As(errorDeOCR(errors.New(crudo)), &api) {
		t.Fatal("errorDeOCR no devolvió un *httpx.Error")
	}
	if api.Status != http.StatusBadGateway || api.Code != "ocr_failed" {
		t.Errorf("= %d/%s, quiero 502/ocr_failed", api.Status, api.Code)
	}
	if strings.Contains(api.Message, crudo) {
		t.Errorf("el mensaje al cliente repite el fallo crudo: %q", api.Message)
	}
	// Y sigue estando para quien lea los registros: `httpx.Fail` registra
	// `apiErr.Error()`, que arrastra la causa envuelta.
	if !strings.Contains(api.Error(), crudo) {
		t.Errorf("la causa se perdió por el camino: %q", api.Error())
	}
}
