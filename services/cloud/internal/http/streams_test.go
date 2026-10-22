// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"io"
	"path"
	"slices"
	"strings"
	"testing"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// sink hace de ResponseWriter mínimo para el emisor de frames: solo hacen falta
// el io.Writer y el Flush, y contar los flushes comprueba que el log sale en vivo
// y no se queda en el búfer de la respuesta hasta que se llene.
type sink struct {
	strings.Builder
	flushes int
}

func (s *sink) Flush() { s.flushes++ }

func TestWriteLogFramesUnaLineaPorEvento(t *testing.T) {
	var out sink
	writeLogFrames(&out, &out, strings.NewReader("arrancando\nescuchando en :8080\nGET / 200\n"))

	// Cada línea con su prefijo: es lo que promete el OpenAPI y lo único que un
	// cliente que siga la especificación (EventSource) acepta.
	want := "data: arrancando\n\ndata: escuchando en :8080\n\ndata: GET / 200\n\n"
	if out.String() != want {
		t.Errorf("frames emitidos:\n%q\nesperado:\n%q", out.String(), want)
	}
	if out.flushes != 3 {
		t.Errorf("%d flushes para 3 líneas: cada línea debe salir en el momento", out.flushes)
	}
}

func TestWriteLogFramesUltimaLineaSinSalto(t *testing.T) {
	// El proceso muere sin cerrar la última línea: se emite igual, o el motivo real
	// del fallo —justo lo que se está mirando— no llegaría nunca al visor.
	var out sink
	writeLogFrames(&out, &out, strings.NewReader("panic: nil map"))
	if got := out.String(); got != "data: panic: nil map\n\n" {
		t.Errorf("la última línea sin salto debe emitirse, salió %q", got)
	}
}

func TestWriteLogFramesLimpiaRetornosDeCarro(t *testing.T) {
	// En SSE un \r suelto TAMBIÉN termina la línea, así que uno intercalado —los
	// deja cualquier barra de progreso de npm o pip— partiría el evento en dos.
	var out sink
	writeLogFrames(&out, &out, strings.NewReader("windows\r\n50%\r100%\n"))
	if got := out.String(); got != "data: windows\n\ndata: 50%100%\n\n" {
		t.Errorf("los retornos de carro deben desaparecer del payload, salió %q", got)
	}
}

func TestWriteLogFramesIgnoraLineasVacias(t *testing.T) {
	// Un evento con data vacío no lo despacha el cliente: no se escribe.
	var out sink
	writeLogFrames(&out, &out, strings.NewReader("\n\nhola\n\n"))
	if got := out.String(); got != "data: hola\n\n" {
		t.Errorf("las líneas en blanco no deben generar eventos, salió %q", got)
	}
}

// EL CASO QUE PROTEGE EL PROCESO: los logs los escribe código del cliente, así que
// una app puede escupir megabytes sin un solo salto de línea. Ni se pierde el
// contenido ni el búfer crece con él: se parte en eventos del tamaño del tope.
func TestWriteLogFramesParteLineasEnormes(t *testing.T) {
	const largo = maxLogFrame*3 + 7
	var out sink
	writeLogFrames(&out, &out, strings.NewReader(strings.Repeat("x", largo)+"\n"))

	got := out.String()
	if n := strings.Count(got, "data: "); n != 4 {
		t.Errorf("%d eventos: una línea de %d bytes debe partirse en trozos del tope", n, largo)
	}
	if n := strings.Count(got, "x"); n != largo {
		t.Errorf("se emitieron %d bytes de %d: partir no puede perder contenido", n, largo)
	}
}

// Un lector que falla a media línea no puede colgar el bucle ni tragarse lo leído.
func TestWriteLogFramesCorteDeConexion(t *testing.T) {
	var out sink
	writeLogFrames(&out, &out, io.MultiReader(
		strings.NewReader("antes del corte\nmedia lí"),
		errReader{},
	))
	if got := out.String(); got != "data: antes del corte\n\ndata: media lí\n\n" {
		t.Errorf("al cortarse debe emitirse lo ya leído, salió %q", got)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// TestWSOriginPatternsCerrada fija la frontera de confianza del handshake del
// WebSocket de terminal. Cloud ejecuta código de sus clientes bajo
// *.apps.sytue.eu y *.preview.sytue.eu: un comodín de zona ("*.sytue.eu") los
// daría por buenos, así que la allowlist debe ser la lista cerrada de primera
// parte. Se evalúa con path.Match, el matcher exacto que usa coder/websocket.
func TestWSOriginPatternsCerrada(t *testing.T) {
	rechazados := []string{
		"app-de-un-cliente.apps.sytue.eu",
		"pr-42.preview.sytue.eu",
		"cualquiera.apps.sytue.localhost",
		"sytue.eu.atacante.com",
	}
	prod := wsOriginPatterns(pcfg.EnvProd)
	for _, host := range rechazados {
		for _, pat := range prod {
			ok, err := path.Match(strings.ToLower(pat), strings.ToLower(host))
			if err != nil {
				t.Fatalf("patrón inválido %q: %v", pat, err)
			}
			if ok {
				t.Errorf("el patrón %q autoriza %q; la allowlist WS no debe confiar en dominios de código de cliente", pat, host)
			}
		}
	}
	if !slices.Contains(prod, "cloud.sytue.eu") {
		t.Errorf("wsOriginPatterns debe incluir el dashboard: %v", prod)
	}
	// En producción no entra NINGÚN host de desarrollo: la lista era una sola
	// constante con prod y dev concatenados, y eso viajaba tal cual a prod.
	for _, pat := range prod {
		if strings.HasSuffix(pat, ".localhost") {
			t.Errorf("la allowlist WS de prod incluye un host de desarrollo: %q", pat)
		}
	}
	// Y en dev sí, o no se puede trabajar contra sytue.localhost.
	if !slices.Contains(wsOriginPatterns(pcfg.EnvDev), "cloud.sytue.localhost") {
		t.Error("en dev la allowlist WS debe seguir admitiendo cloud.sytue.localhost")
	}
}
