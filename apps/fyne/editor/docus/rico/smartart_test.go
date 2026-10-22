// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"slices"
	"strings"
	"testing"
)

// TestElSmartArtEsElDeLaWeb son los vectores de `docs-smartart.test.ts`: la lectura
// tolerante de `data-items`, el saneado de la disposición, el SVG con un nodo por
// elemento y su texto escapado, y que la figura va y vuelve con su configuración. Se vio
// en ROJO sin `data-smartart` en esFigura: el diagrama dejaba el documento en solo lectura.
func TestElSmartArtEsElDeLaWeb(t *testing.T) {
	if got := LeerElementos(`[{"text":"a"},{"text":"b","level":2.7},{"level":-3},7]`); !slices.Equal(got,
		[]ElementoSmart{{Texto: "a"}, {Texto: "b", Nivel: 2}, {}, {}}) {
		t.Errorf("elementos: %+v", got)
	}
	if len(LeerElementos("no es json")) != 0 || SanearDisposicion("radial") != "list" {
		t.Error("lo que no se lee es una lista vacía y la disposición, la lista")
	}
	sa := SmartArt{Disposicion: "hierarchy", Elementos: ElementosIniciales("hierarchy")}
	sa.Elementos[1].Texto = `<b>A & B</b>`
	h := sa.HTML("Diagrama jerarquía", "Vacío")
	if strings.Count(h, "<foreignObject") != 3 || strings.Count(h, `<path d="M`) != 2 || strings.Contains(h, "<b>") {
		t.Fatalf("el HTML: %s", h)
	}
	d := Parse(`<p>a</p>`)
	if _, ok := d.InsertarHTML(d.Bloques(), 0, 1, h+`<p><br></p>`); !ok || !d.Guardable() {
		t.Fatalf("no se inserta o no se guarda: %v", d.NoEntendido())
	}
	if !strings.Contains(d.HTML(), `data-items="[{&quot;text&quot;:&quot;Dirección&quot;},{&quot;text&quot;:&quot;&lt;b&gt;A &amp; B&lt;/b&gt;&quot;,&quot;level&quot;:1}`) {
		t.Errorf("los elementos, como los escribe la web: %s", d.HTML())
	}
	otra := Parse(d.HTML())
	bs := otra.Bloques()
	if !otra.Editable() || otra.HTML() != d.HTML() || len(bs) != 3 || bs[1].Ilustracion == nil || bs[1].Ilustracion.Clase() != "smartart" {
		t.Fatalf("no vuelve: %s", d.HTML())
	}
	if got := *bs[1].Ilustracion.Smart; got.Disposicion != "hierarchy" || !slices.Equal(got.Elementos, sa.Elementos) {
		t.Errorf("releído: %+v", got)
	}
	if ClaseDe(h) != "smartart" || sa.AltoSmart() != 260 {
		t.Errorf("clase %q, alto %v", ClaseDe(h), sa.AltoSmart())
	}
	if v := (SmartArt{Disposicion: "list"}).HTML("x", "Vacío"); !strings.Contains(v, ">Vacío</text>") {
		t.Errorf("sin elementos, el aviso: %s", v)
	}
}
