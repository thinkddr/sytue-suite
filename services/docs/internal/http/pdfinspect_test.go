// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// El extracto es lo que permite RECONOCER una página en una lista sin verla, así
// que lo que importa es que quepa en una línea y no corte una palabra por la
// mitad, que se lee peor de lo que ocupa.

func TestExtractoColapsaEspacios(t *testing.T) {
	// El texto de un PDF llega con saltos de línea por cada fragmento; sin
	// colapsarlos, el extracto rompería la fila de la lista.
	got := extractoDe("primera\nlínea\t y   otra", 100)
	if got != "primera línea y otra" {
		t.Errorf("= %q, quería «primera línea y otra»", got)
	}
}

func TestExtractoCortaPorPalabra(t *testing.T) {
	texto := "contrato de arrendamiento de vivienda entre las partes que se indican"
	got := extractoDe(texto, 30)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("= %q; un texto cortado debería acabar en puntos suspensivos", got)
	}
	// Ni corta a mitad de palabra ni se pasa del límite pedido.
	sinPuntos := strings.TrimSuffix(got, "…")
	if strings.HasSuffix(sinPuntos, " ") {
		t.Errorf("= %q; no debería acabar en espacio", got)
	}
	if len([]rune(sinPuntos)) > 30 {
		t.Errorf("= %q; se pasa del máximo de 30", got)
	}
	if !strings.HasPrefix(got, "contrato de") {
		t.Errorf("= %q; debería empezar por el principio del texto", got)
	}
}

func TestExtractoNoCortaSiCabe(t *testing.T) {
	got := extractoDe("corto", 100)
	if got != "corto" {
		t.Errorf("= %q, quería «corto» sin puntos suspensivos", got)
	}
}

func TestExtractoDePaginaSinTexto(t *testing.T) {
	// Una página que solo tiene una imagen devuelve vacío, y eso también es
	// información: la interfaz puede decir «sin texto» en vez de mentir.
	for _, entrada := range []string{"", "   ", "\n\t\n"} {
		if got := extractoDe(entrada, 50); got != "" {
			t.Errorf("%q → %q, quería vacío", entrada, got)
		}
	}
}

func TestExtractoConUnaPalabraMuyLarga(t *testing.T) {
	// Sin la guarda de «corta solo si el espacio está pasada la mitad», una
	// palabra larguísima se quedaría en un fragmento inútil de dos letras.
	got := extractoDe(strings.Repeat("a", 200), 20)
	if len([]rune(strings.TrimSuffix(got, "…"))) != 20 {
		t.Errorf("= %q; debería cortar a los 20 caracteres", got)
	}
}

// TestElContenidoTraeTodasLasPaginas: el visor pinta lo que trae `paginas`, así que una
// página que no esté ahí no existe para quien lee. Se cortaban en 500 —en un expediente
// de 1192, de la 501 en adelante no se llegaba— y el extracto de cada una se sacaba antes
// de contestar, que era el 100 % del tiempo de abrir.
func TestElContenidoTraeTodasLasPaginas(t *testing.T) {
	w := pdfsimple.Nuevo()
	const total = maxPaginasIndexadas + 20
	for i := range total {
		if i > 0 {
			w.NuevaPagina()
		}
		w.Texto("Página "+strconv.Itoa(i+1)+" del expediente", "F1", 12)
	}
	doc, err := domain.LeerPDF(w.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	todas := indiceDe(doc, doc.NumPaginas(), 0)
	if len(todas) != total || todas[total-1].Numero != total {
		t.Fatalf("el contenido trae %d páginas de %d", len(todas), total)
	}
	// Los extractos sí siguen acotados: diez mil no le sirven a nadie.
	if todas[maxPaginasIndexadas-1].Extracto == nil || todas[maxPaginasIndexadas].Extracto != nil {
		t.Error("los extractos ya no se cortan en maxPaginasIndexadas")
	}
	if e := todas[0].Extracto; e == nil || !strings.Contains(*e, "Página 1") {
		t.Errorf("el extracto de la primera es %v", e)
	}

	// Con presupuesto se lee lo que dé tiempo y el resto se queda SIN extracto —no
	// vacío: vacío es «no tiene texto», que es otra cosa—.
	ini := time.Now()
	rapido := indiceDe(doc, doc.NumPaginas(), time.Nanosecond)
	if time.Since(ini) > 5*time.Second {
		t.Errorf("con presupuesto el índice tarda %s", time.Since(ini))
	}
	if len(rapido) != total {
		t.Fatalf("con presupuesto trae %d páginas de %d", len(rapido), total)
	}
	if rapido[0].Extracto == nil {
		t.Error("ni la primera página trae extracto: es la que dice si el documento es un escaneo")
	}
	if rapido[total-1].Extracto != nil {
		t.Error("con presupuesto se han leído todas igual")
	}

	// Drive (`/pdf/inspect`) sigue con las primeras: su lista ofrece escribir el rango.
	if n := len(indiceDe(doc, maxPaginasIndexadas, 0)); n != maxPaginasIndexadas {
		t.Errorf("el índice de Drive trae %d", n)
	}
}
