// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"
)

// Reintentos de red y caché verificada de las fuentes públicas de AOSP que baja
// apps/fyne/scripts/android-work-context-test.py (la comprobación cruzada del
// contexto de trabajo Android contra el escritor JSON de Android fijado). Se
// sustituyen urlopen y sleep en proceso: ni red ni esperas.

const androidWorkContextDescarga = `
import base64, hashlib, io, tempfile, urllib.error
from email.message import Message
from unittest import mock
module = cargar("android_work_context", SCRIPTS / "android-work-context-test.py")
DATA = b"synthetic public source bytes"
DIGEST = hashlib.sha256(DATA).hexdigest()

def failure(code, after=None):
    headers = Message()
    if after is not None:
        headers["Retry-After"] = after
    return urllib.error.HTTPError(module.SOURCE, code, "synthetic test response", headers, None)

def llamadas_sleep(sleep):
    return [list(c.args) for c in sleep.call_args_list]
`

func TestAndroidWorkContextDescargaLimiteDeRitmoYLuegoExitoRespetaRetryAfter(t *testing.T) {
	var r struct {
		Igual    bool    `json:"igual"`
		Llamadas int     `json:"llamadas"`
		Sleep    [][]any `json:"sleep"`
	}
	pyRestoCorrer(t, androidWorkContextDescarga+`
with mock.patch.object(module.urllib.request, "urlopen", side_effect=[failure(429, "2"), io.BytesIO(base64.b64encode(DATA))]) as request, mock.patch.object(module.time, "sleep") as sleep:
    igual = module.download_source("source") == DATA
devolver({"igual": igual, "llamadas": request.call_count, "sleep": llamadas_sleep(sleep)})
`, &r)
	if !r.Igual || r.Llamadas != 2 || !reflect.DeepEqual(r.Sleep, [][]any{{2.0}}) {
		t.Fatalf("datos iguales=%v, %d peticiones, sleep %v; se esperaba true, 2 y [[2]]", r.Igual, r.Llamadas, r.Sleep)
	}
}

func TestAndroidWorkContextDescargaRetryAfterConFechaAcotadoYCabecerasInvalidasUsanBackoff(t *testing.T) {
	var r []float64
	pyRestoCorrer(t, androidWorkContextDescarga+`
headers = Message()
headers["Retry-After"] = "Thu, 01 Jan 1970 00:01:40 GMT"
with mock.patch.object(module.time, "time", return_value=10):
    fecha = module.retry_delay(headers, 0)
headers.replace_header("Retry-After", "invalid")
invalida = module.retry_delay(headers, 1)
headers.replace_header("Retry-After", "9999999999")
enorme = module.retry_delay(headers, 0)
headers.replace_header("Retry-After", "0")
cero = module.retry_delay(headers, 0)
devolver([fecha, invalida, enorme, cero])
`, &r)
	// Fecha a 90 s → tope de 5; inválida en el intento 1 → 2**1; enorme → tope; 0 → 0.
	if !reflect.DeepEqual(r, []float64{5, 2, 5, 0}) {
		t.Fatalf("esperas %v, no [5 2 5 0]", r)
	}
}

func TestAndroidWorkContextDescargaAgotarLosIntentosConservaElErrorHTTP(t *testing.T) {
	var r struct {
		Error    *pyRestoExcepcion `json:"error"`
		Codigo   int               `json:"codigo"`
		Llamadas int               `json:"llamadas"`
		Sleep    [][]any           `json:"sleep"`
	}
	pyRestoCorrer(t, androidWorkContextDescarga+`
codigo = None
with mock.patch.object(module.urllib.request, "urlopen", side_effect=[failure(503) for _ in range(4)]) as request, mock.patch.object(module.time, "sleep") as sleep:
    try:
        module.download_source("source")
        error = None
    except Exception as e:
        error = excepcion(e)
        codigo = getattr(e, "code", None)
devolver({"error": error, "codigo": codigo, "llamadas": request.call_count, "sleep": llamadas_sleep(sleep)})
`, &r)
	pyRestoLanza(t, r.Error, "HTTPError", "")
	if r.Codigo != 503 || r.Llamadas != 4 || !reflect.DeepEqual(r.Sleep, [][]any{{1.0}, {2.0}, {4.0}}) {
		t.Fatalf("código %d, %d peticiones, sleep %v; se esperaba 503, 4 y [[1] [2] [4]]", r.Codigo, r.Llamadas, r.Sleep)
	}
}

func TestAndroidWorkContextDescarga404YFallosNoTransitoriosNoSeOcultan(t *testing.T) {
	for nombre, caso := range map[string]struct{ error, tipo string }{
		"404":            {"failure(404)", "HTTPError"},
		"no-transitorio": {`urllib.error.URLError(ValueError("not transient"))`, "URLError"},
	} {
		t.Run(nombre, func(t *testing.T) {
			var r struct {
				Error    *pyRestoExcepcion `json:"error"`
				Tipo     string            `json:"tipo"`
				Llamadas int               `json:"llamadas"`
				Sleep    [][]any           `json:"sleep"`
			}
			pyRestoCorrer(t, androidWorkContextDescarga+`
error_ = `+caso.error+`
with mock.patch.object(module.urllib.request, "urlopen", side_effect=error_) as request, mock.patch.object(module.time, "sleep") as sleep:
    error = fallo(module.download_source, "source")
devolver({"error": error, "tipo": type(error_).__name__, "llamadas": request.call_count, "sleep": llamadas_sleep(sleep)})
`, &r)
			// assertRaises(type(error)): la misma clase que se inyectó, o una subclase.
			pyRestoLanza(t, r.Error, r.Tipo, "")
			if r.Tipo != caso.tipo || r.Llamadas != 1 || len(r.Sleep) != 0 {
				t.Fatalf("%s: %d peticiones y sleep %v; se esperaba 1 y ninguno", r.Tipo, r.Llamadas, r.Sleep)
			}
		})
	}
}

func TestAndroidWorkContextDescargaUnTimeoutDeRedTransitorioSeReintenta(t *testing.T) {
	var r struct {
		Igual bool    `json:"igual"`
		Sleep [][]any `json:"sleep"`
	}
	pyRestoCorrer(t, androidWorkContextDescarga+`
with mock.patch.object(module.urllib.request, "urlopen", side_effect=[urllib.error.URLError(TimeoutError()), io.BytesIO(base64.b64encode(DATA))]), mock.patch.object(module.time, "sleep") as sleep:
    igual = module.download_source("source") == DATA
devolver({"igual": igual, "sleep": llamadas_sleep(sleep)})
`, &r)
	if !r.Igual || !reflect.DeepEqual(r.Sleep, [][]any{{1.0}}) {
		t.Fatalf("datos iguales=%v, sleep %v; se esperaba true y [[1]]", r.Igual, r.Sleep)
	}
}

func TestAndroidWorkContextDescargaCorruptaNiSeReintentaNiSeGuarda(t *testing.T) {
	cache := t.TempDir()
	var r struct {
		Error    *pyRestoExcepcion `json:"error"`
		Llamadas int               `json:"llamadas"`
		Sleep    [][]any           `json:"sleep"`
		Cache    []string          `json:"cache"`
	}
	pyRestoCorrer(t, androidWorkContextDescarga+`
tmp = pathlib.Path(ENTRADA)
with mock.patch.object(module.urllib.request, "urlopen", return_value=io.BytesIO(base64.b64encode(b"wrong bytes"))) as request, mock.patch.object(module.time, "sleep") as sleep:
    error = fallo(module.source_bytes, "fixture", "source", DIGEST, tmp)
devolver({"error": error, "llamadas": request.call_count, "sleep": llamadas_sleep(sleep), "cache": [p.name for p in tmp.iterdir()]})
`, &r, pyRestoConEntrada(t, cache))
	pyRestoLanza(t, r.Error, "ValueError", "source hash mismatch")
	if len(r.Cache) != 0 || r.Llamadas != 1 || len(r.Sleep) != 0 {
		t.Fatalf("caché %v, %d peticiones, sleep %v; se esperaba vacía, 1 y ninguno", r.Cache, r.Llamadas, r.Sleep)
	}
}

func TestAndroidWorkContextDescargaLaReproduccionCompruebaLaHuellaYNuncaTruncaNiReDescargaUnaCacheValida(t *testing.T) {
	cache := t.TempDir()
	datos := []byte("synthetic public source bytes")
	suma := sha256.Sum256(datos)
	huella := hex.EncodeToString(suma[:])
	var r struct {
		Primera    bool              `json:"primera"`
		Pedida     [][]any           `json:"pedida"`
		Cache      []string          `json:"cache"`
		Acierto    bool              `json:"acierto"`
		Error      *pyRestoExcepcion `json:"error"`
		Conservada string            `json:"conservada"`
	}
	pyRestoCorrer(t, androidWorkContextDescarga+`
cache = pathlib.Path(ENTRADA)
with mock.patch.object(module, "download_source", return_value=DATA) as request:
    primera = module.source_bytes("fixture", "source", DIGEST, cache) == DATA
pedida = [list(c.args) for c in request.call_args_list]
nombres = [p.name for p in cache.iterdir()]
with mock.patch.object(module, "download_source", side_effect=AssertionError("network on cache hit")):
    acierto = module.source_bytes("fixture", "source", DIGEST, cache) == DATA
    (cache / (DIGEST + ".java")).write_bytes(b"corrupt cached source")
    error = fallo(module.source_bytes, "fixture", "source", DIGEST, cache)
    conservada = (cache / (DIGEST + ".java")).read_bytes().decode()
devolver({"primera": primera, "pedida": pedida, "cache": nombres, "acierto": acierto, "error": error, "conservada": conservada})
`, &r, pyRestoConEntrada(t, cache))
	if !r.Primera || !reflect.DeepEqual(r.Pedida, [][]any{{"source"}}) {
		t.Fatalf("primera lectura %v, descargas %v; se esperaba una sola de «source»", r.Primera, r.Pedida)
	}
	if !reflect.DeepEqual(r.Cache, []string{huella + ".java"}) {
		t.Fatalf("caché %v, no [%s.java]", r.Cache, huella)
	}
	if !r.Acierto {
		t.Fatal("un acierto de caché no devolvió los datos")
	}
	pyRestoLanza(t, r.Error, "ValueError", "source hash mismatch")
	if r.Conservada != "corrupt cached source" {
		t.Fatalf("la caché corrupta se tocó: %q", r.Conservada)
	}
}
