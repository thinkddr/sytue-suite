// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// documentoTipico es un informe de oficina corriente: título, seis apartados con párrafos,
// una lista, una tabla de 8×4 con cabecera, un enlace y, si conImagen, una foto de 800×500.
func documentoTipico(t *testing.T, apartados int, conImagen bool) string {
	t.Helper()
	parrafo := "<p>La accesibilidad no se añade al final: se escribe con el documento. " +
		strings.Repeat("Este párrafo tiene la longitud de uno corriente de un informe de oficina, con sus comas y sus <strong>negritas</strong>. ", 4) + "</p>"
	var b strings.Builder
	b.WriteString("<h1>Informe trimestral</h1>")
	for i := range apartados {
		b.WriteString("<h2>Apartado " + strconv.Itoa(i+1) + "</h2>" + strings.Repeat(parrafo, 5))
		if i == 1 {
			b.WriteString(`<ul><li>Uno</li><li>Dos<ul><li>Dos-uno</li></ul></li><li>Tres con <a href="https://sytue.eu">enlace</a></li></ul>`)
		}
		if i == 2 {
			b.WriteString("<table><tr><th>Región</th><th>T1</th><th>T2</th><th>Notas</th></tr>" +
				strings.Repeat("<tr><td>Norte</td><td>1.200 €</td><td>1.450 €</td><td>Crece por el canal nuevo</td></tr>", 7) + "</table>")
		}
		if i == 3 && conImagen {
			im := image.NewNRGBA(image.Rect(0, 0, 800, 500))
			for y := range 500 {
				for x := range 800 {
					im.SetNRGBA(x, y, color.NRGBA{R: uint8(x * y), G: uint8(x ^ y), B: uint8(x + y), A: 255})
				}
			}
			var p bytes.Buffer
			if err := png.Encode(&p, im); err != nil {
				t.Fatal(err)
			}
			b.WriteString(`<p><img alt="Gráfico de ventas" src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(p.Bytes()) + `"></p>`)
		}
	}
	return b.String()
}

var reLength1 = regexp.MustCompile(`/Length1 (\d+) /Filter /FlateDecode /Length (\d+)`)

// TestMedirPDFAccesible es una SONDA, no un guardián: mide lo que cuesta el PDF accesible
// frente al simple (tiempo, memoria reservada y tamaño) y cuánto de ese tamaño son fuentes.
// Los números van a docs/paridad-pdf.md. SYTUE_PDF_MEDIR_UA=1 la enciende.
func TestMedirPDFAccesible(t *testing.T) {
	if os.Getenv("SYTUE_PDF_MEDIR_UA") == "" {
		t.Skip("solo con SYTUE_PDF_MEDIR_UA=1")
	}
	medir := func(f func() []byte) (out []byte, d time.Duration, mib float64) {
		ds := make([]time.Duration, 0, 7)
		allocs := make([]uint64, 0, 7)
		for range 7 {
			runtime.GC()
			var a, b runtime.MemStats
			runtime.ReadMemStats(&a)
			ini := time.Now()
			out = f()
			ds = append(ds, time.Since(ini))
			runtime.ReadMemStats(&b)
			allocs = append(allocs, b.TotalAlloc-a.TotalAlloc)
		}
		slices.Sort(ds)
		slices.Sort(allocs)
		return out, ds[len(ds)/2], float64(allocs[len(allocs)/2]) / (1 << 20)
	}
	ini := time.Now()
	if _, _, err := ExportPDFAccesible(KindDocument, "x", "es", stateWithContent(t, "doc:html", "<p>á Ω Ж</p>")); err != nil {
		t.Fatal(err)
	}
	t.Logf("primera exportación del proceso (lee las 8 caras incrustadas): %v", time.Since(ini))
	for _, c := range []struct {
		nombre string
		html   string
	}{
		{"una línea", "<p>Hola</p>"},
		{"típico sin imagen (6 apartados)", documentoTipico(t, 6, false)},
		{"típico con imagen 800×500", documentoTipico(t, 6, true)},
		{"típico con griego y cirílico", documentoTipico(t, 6, false) + "<p>Ελληνικά: προσβασιμότητα. Български: достъпност.</p>"},
		{"largo (120 apartados)", documentoTipico(t, 120, false)},
	} {
		st := stateWithContent(t, "doc:html", c.html)
		ua, dUA, mUA := medir(func() []byte {
			b, _, err := ExportPDFAccesible(KindDocument, "Informe trimestral", "es", st)
			if err != nil {
				t.Fatal(err)
			}
			return b
		})
		simple, dS, mS := medir(func() []byte {
			b, _, _, err := Export(KindDocument, "Informe trimestral", st, FormatPDF)
			if err != nil {
				t.Fatal(err)
			}
			return b
		})
		fuentes, crudo, caras := 0, 0, 0
		for _, m := range reLength1.FindAllSubmatch(ua, -1) {
			a, _ := strconv.Atoi(string(m[1]))
			z, _ := strconv.Atoi(string(m[2]))
			crudo, fuentes, caras = crudo+a, fuentes+z, caras+1
		}
		t.Logf("%-34s | páginas %3d | accesible %7d B en %8v, %6.1f MiB | simple %7d B en %8v, %5.1f MiB | fuentes: %d caras, %d B (%d B sin comprimir) = %d %%",
			c.nombre, bytes.Count(ua, []byte("/Type /Page /Parent")), len(ua), dUA.Round(10*time.Microsecond), mUA,
			len(simple), dS.Round(10*time.Microsecond), mS, caras, fuentes, crudo, 100*fuentes/len(ua))
	}
}
