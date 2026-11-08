// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// etiquetasPintadas son los hashtags que conHashtags convierte en enlace, en orden.
func etiquetasPintadas(html string) []string {
	var out []string
	for _, b := range conHashtags(Leer(html)).Bloques {
		for _, t := range b.Tramos {
			if tag, ok := hashtagDe(t.Enlace); ok {
				out = append(out, tag)
			}
		}
	}
	return out
}

// Los MISMOS casos que client/hashtags.test.ts y `TestHashtags` del servicio: si la regla
// cambia allí y no aquí, se pintaría como etiqueta lo que la búsqueda no encuentra. Visto
// en rojo con la clase de antes de la `#` sin `/` (sale `ancla` de `sitio.eu/#ancla`).
func TestLosHashtagsSonLosDelServicio(t *testing.T) {
	for html, quiero := range map[string][]string{
		"<p>Llamar a #Clientes y cerrar #plan-2026 (#urgente)</p>":                 {"Clientes", "plan-2026", "urgente"},
		"<p>#reunión</p><p>#日本 al empezar línea</p>":                               {"reunión", "日本"},
		"<p>ver https://sitio.eu/#ancla y texto#pegado</p>":                        nil,
		"<p>it&#39;s el #1 con &#x27;comillas&#x27;</p>":                           nil,
		"<p>fondo #ff0000 y ## Título</p>":                                         nil,
		"<p>la #decade del #plan- y #b2b</p>":                                      {"decade", "plan", "b2b"},
		`<p><a href="https://sytue.eu/#x">un #enlace</a> <code>#define</code></p>`: nil,
		`<p><a href="https://x.eu">a</a> #despues</p>`:                             {"despues"},
	} {
		if got := etiquetasPintadas(html); !reflect.DeepEqual(got, quiero) {
			t.Errorf("%s: %v, quiero %v", html, got, quiero)
		}
	}
}

// TestPintarUnHashtagNoCambiaLaNota: la copia lleva el enlace y el texto de alrededor; el
// documento original, que es el que se edita y se guarda, sigue igual.
func TestPintarUnHashtagNoCambiaLaNota(t *testing.T) {
	d := Leer("<p>Ver <b>#plan</b>- hoy</p>")
	antes := d.HTML()
	pintado := conHashtags(d)
	if d.HTML() != antes {
		t.Fatalf("pintar los hashtags ha cambiado la nota: %s", d.HTML())
	}
	textos := make([]string, 0, len(pintado.Bloques[0].Tramos))
	for _, tr := range pintado.Bloques[0].Tramos {
		textos = append(textos, tr.Texto+"|"+tr.Enlace)
	}
	if quiero := []string{"Ver |", "#plan|#plan", "- hoy|"}; !reflect.DeepEqual(textos, quiero) {
		t.Errorf("los tramos pintados son %q, quiero %q", textos, quiero)
	}
	if conHashtags(Leer("<p>sin etiquetas</p>")).Bloques[0].Tramos[0].Enlace != "" {
		t.Error("sin hashtags no hay enlace que pintar")
	}
}

// TestPulsarUnHashtagAbreSuBusqueda: es lo que hace la web al pulsar `a[data-hashtag]`.
func TestPulsarUnHashtagAbreSuBusqueda(t *testing.T) {
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 800))
	p := NuevaPantalla(trInterno(t), "es", nil, nil, w)
	w.SetContent(p.Objeto())
	p.seguirEnlace(hrefHashtag("viaje"))
	if p.vista != VistaBusqueda || p.busqueda.Consulta != "#viaje" || p.busqueda.Campo().Texto() != "#viaje" {
		t.Errorf("pulsar #viaje deja la vista %v con la consulta %q", p.vista, p.busqueda.Consulta)
	}
}
