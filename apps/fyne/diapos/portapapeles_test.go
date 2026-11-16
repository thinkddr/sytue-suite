// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// Los VECTORES COMPARTIDOS con la web: los calcula `slides/portapapeles.ts` y los exige
// también `packages/office/test/slides-portapapeles.test.ts`. Lo que se copia en un lado
// se pega en el otro: el mismo texto del portapapeles tiene que dar el mismo modelo.
const rutaVectoresPortapapeles = "../../frontend/packages/office/test/vectores/diapos-portapapeles.json"

// arbol pasa un valor a su árbol JSON, para comparar sin depender del orden de las
// claves ni de cómo cada lado escribe un número.
func arbol(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var a any
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func generadorIDs() func() string {
	n := 0
	return func() string { n++; return "n" + strconv.Itoa(n) }
}

// TestPortapapelesComoLaWeb recorre la tabla compartida: el texto que la web deja en el
// portapapeles se lee igual, Recortar en Go da el mismo recorte, y pegarlo en cada
// destino da las mismas cajas, conectores e ids que en la web.
//
// VISTO EN ROJO: sin sanear los puntos de la trayectoria personalizada en AnimsDe (como
// `boxAnims`), los pegados de la imagen con un solo punto no coincidían; y pidiendo el
// id del grupo antes que el de la caja, todos los pegados de grupo salían con otros ids.
func TestPortapapelesComoLaWeb(t *testing.T) {
	crudo, err := os.ReadFile(rutaVectoresPortapapeles)
	if err != nil {
		t.Fatalf("no se pueden leer los vectores: %v", err)
	}
	type lado struct {
		Cajas      []Caja     `json:"boxes"`
		Conectores []Conector `json:"connectors"`
	}
	var doc struct {
		Origen  lado `json:"origen"`
		Pegados []struct {
			Nombre        string          `json:"nombre"`
			IDs           []string        `json:"ids"`
			Texto         string          `json:"texto"`
			Destino       lado            `json:"destino"`
			ConConectores bool            `json:"conConectores"` //nolint:tagliatelle // el JSON es el de la web
			Sale          json.RawMessage `json:"sale"`
		} `json:"pegados"`
		Lecturas []struct {
			Nombre string          `json:"nombre"`
			Texto  string          `json:"texto"`
			Sale   json.RawMessage `json:"sale"`
		} `json:"lecturas"`
	}
	if err := json.Unmarshal(crudo, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Pegados) < 20 || len(doc.Lecturas) < 8 {
		t.Fatalf("la tabla se ha quedado corta: %d pegados y %d lecturas", len(doc.Pegados), len(doc.Lecturas))
	}
	for _, c := range doc.Pegados {
		r, ok := LeerRecorte(c.Texto)
		if !ok {
			t.Errorf("%s: el texto de la web no se lee como recorte", c.Nombre)
			continue
		}
		// Lo que copia la app es lo que copia la web (y se lee igual).
		propio := Recortar(doc.Origen.Cajas, doc.Origen.Conectores, c.IDs)
		if !strings.HasPrefix(propio.Texto(), PrefijoRecorte) {
			t.Errorf("%s: el texto de la app no lleva el prefijo", c.Nombre)
		}
		deVuelta, ok := LeerRecorte(propio.Texto())
		if !ok || !reflect.DeepEqual(arbol(t, deVuelta), arbol(t, r)) {
			t.Errorf("%s: el recorte de la app no es el de la web", c.Nombre)
		}
		p := PegarRecorte(c.Destino.Cajas, c.Destino.Conectores, r, generadorIDs(), c.ConConectores)
		got := arbol(t, map[string]any{"boxes": p.Cajas, "connectors": p.Conectores, "ids": p.IDs})
		var want any
		if err := json.Unmarshal(c.Sale, &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			g, _ := json.Marshal(got)
			w, _ := json.Marshal(want)
			t.Errorf("%s: pegar no da lo de la web\n app: %s\n web: %s", c.Nombre, g, w)
		}
	}
	for _, l := range doc.Lecturas {
		r, ok := LeerRecorte(l.Texto)
		var want any
		if err := json.Unmarshal(l.Sale, &want); err != nil {
			t.Fatal(err)
		}
		if want == nil {
			if ok {
				t.Errorf("%s: la web lo rechaza y la app lo lee", l.Nombre)
			}
			continue
		}
		if !ok {
			t.Errorf("%s: la web lo lee y la app lo rechaza", l.Nombre)
			continue
		}
		if got := arbol(t, r); !reflect.DeepEqual(got, want) {
			g, _ := json.Marshal(got)
			w, _ := json.Marshal(want)
			t.Errorf("%s: se lee distinto\n app: %s\n web: %s", l.Nombre, g, w)
		}
	}
}

// grupoYConector es una diapositiva con dos formas agrupadas unidas por un conector,
// otra forma suelta y una caja de texto.
const grupoYConector = `{"slides":[{"id":"s","boxes":[` +
	`{"id":"t","x":10,"y":10,"w":40,"h":20,"html":"<p>hola</p>"},` +
	`{"id":"a","x":20,"y":50,"w":10,"h":10,"html":"","kind":"shape","shape":"rect","groupId":"g"},` +
	`{"id":"b","x":40,"y":50,"w":10,"h":10,"html":"","kind":"shape","shape":"ellipse","groupId":"g"},` +
	`{"id":"f","x":70,"y":70,"w":10,"h":10,"html":"","kind":"shape","shape":"rect"}],` +
	`"connectors":[{"id":"k","kind":"straight","from":{"boxId":"a"},"to":{"boxId":"b"}},` +
	`{"id":"kf","kind":"straight","from":{"boxId":"b"},"to":{"boxId":"f"}}]},` +
	`{"id":"s2","boxes":[]}]}`

func ctrl(k fyne.KeyName) *desktop.CustomShortcut {
	return &desktop.CustomShortcut{KeyName: k, Modifier: fyne.KeyModifierShortcutDefault}
}

// TestSeleccionCopiarYPegarEnOtraDiapositiva: Ctrl+C sobre el escenario lleva el grupo
// con su conector interno al portapapeles en el formato de la web; Ctrl+V en otra
// diapositiva lo pega con ids nuevos, en su sitio, con el conector reanclado y elegido.
//
// VISTO EN ROJO: sin TypedShortcut en la capa, Ctrl+C no dejaba nada en el portapapeles.
func TestSeleccionCopiarYPegarEnOtraDiapositiva(t *testing.T) {
	e, _, _ := montar(t, grupoYConector)
	cb := fyne.CurrentApp().Clipboard()
	cb.SetContent("")
	test.Tap(marcoDe(t, e, "a"))
	e.capa.TypedShortcut(&fyne.ShortcutCopy{Clipboard: cb})
	r, ok := LeerRecorte(cb.Content())
	if !ok {
		t.Fatalf("Ctrl+C no deja un recorte de Diapos en el portapapeles: %q", cb.Content())
	}
	if len(r.Cajas) != 2 || len(r.Conectores) != 1 || r.Conectores[0].ID != "k" {
		t.Fatalf("el recorte del grupo lleva %d cajas y %d conectores; tocaban 2 y el interno", len(r.Cajas), len(r.Conectores))
	}

	e.Elegir(1)
	e.capa.TypedShortcut(&fyne.ShortcutPaste{Clipboard: cb})
	d := e.m.Diapos[1]
	if len(d.Cajas) != 2 || len(d.Conectores) != 1 {
		t.Fatalf("pegar deja %d cajas y %d conectores en la otra diapositiva", len(d.Cajas), len(d.Conectores))
	}
	pa, pb := d.Cajas[0], d.Cajas[1]
	if pa.ID == "a" || pb.ID == "b" || pa.Grupo == "" || pa.Grupo == "g" || pa.Grupo != pb.Grupo {
		t.Errorf("las cajas pegadas reutilizan ids o pierden el grupo: %+v %+v", pa, pb)
	}
	if pa.X != 20 || pa.Y != 50 {
		t.Errorf("en otra diapositiva se pega en su sitio y quedó en (%v, %v)", pa.X, pa.Y)
	}
	if k := d.Conectores[0]; k.Desde.Caja != pa.ID || k.Hasta.Caja != pb.ID || k.ID == "k" {
		t.Errorf("el conector pegado no está reanclado a las copias: %+v", k)
	}
	if got := e.efectiva(); len(got) != 2 || !e.elegida(pa.ID) {
		t.Errorf("lo pegado tiene que quedar elegido y la selección es %v", got)
	}
	// La original sigue intacta.
	if len(e.m.Diapos[0].Cajas) != 4 || len(e.m.Diapos[0].Conectores) != 2 {
		t.Error("copiar no puede tocar la diapositiva de origen")
	}
}

// TestSeleccionCortarYDuplicar: Ctrl+X quita la selección con sus conectores y la deja
// en el portapapeles; Ctrl+V la devuelve en su sitio (su sitio está libre); Ctrl+D
// duplica en el sitio desplazado +2 %, y otra vez +4 %.
func TestSeleccionCortarYDuplicar(t *testing.T) {
	e, _, _ := montar(t, grupoYConector)
	cb := fyne.CurrentApp().Clipboard()
	test.Tap(marcoDe(t, e, "f"))
	e.capa.TypedShortcut(&fyne.ShortcutCut{Clipboard: cb})
	if e.m.caja(0, "f") != nil || len(e.m.Diapos[0].Conectores) != 1 || e.sel != "" {
		t.Fatalf("Ctrl+X no quita la forma con su conector (sel %q)", e.sel)
	}
	e.capa.TypedShortcut(&fyne.ShortcutPaste{Clipboard: cb})
	p := e.Primaria()
	if p == nil || p.ID == "f" || p.X != 70 || p.Y != 70 {
		t.Fatalf("pegar lo cortado tiene que volver a su sitio con otro id: %+v", p)
	}
	// El conector que iba a otra caja no viaja: no tendría a qué unirse.
	if len(e.m.Diapos[0].Conectores) != 1 {
		t.Errorf("pegar ha traído un conector a medias")
	}

	e.capa.TypedShortcut(ctrl(fyne.KeyD))
	e.capa.TypedShortcut(ctrl(fyne.KeyD))
	cs := e.m.Diapos[0].Cajas
	if len(cs) != 6 {
		t.Fatalf("dos Ctrl+D dejan %d cajas y tocaban 6", len(cs))
	}
	if c := cs[len(cs)-1]; c.X != 74 || c.Y != 74 || e.sel != c.ID {
		t.Errorf("el segundo duplicado queda en (%v, %v), elegido %v; tocaba (74, 74) y elegido", c.X, c.Y, e.sel == c.ID)
	}
	if c := cs[len(cs)-2]; c.X != 72 || c.Y != 72 {
		t.Errorf("el primer duplicado queda en (%v, %v) y tocaba (72, 72)", c.X, c.Y)
	}
}

// TestSeleccionEnSoloLecturaSeCopiaPeroNoSePega: lo que se lee se puede llevar, pero
// ni cortar quita ni pegar o duplicar ponen.
func TestSeleccionEnSoloLecturaSeCopiaPeroNoSePega(t *testing.T) {
	e, ctx, _ := montar(t, grupoYConector)
	ctx.SoloLectura = true
	cb := fyne.CurrentApp().Clipboard()
	e.SeleccionarSolo("f")
	e.capa.TypedShortcut(&fyne.ShortcutCut{Clipboard: cb})
	if _, ok := LeerRecorte(cb.Content()); !ok {
		t.Fatal("en solo lectura cortar tiene que copiar")
	}
	e.capa.TypedShortcut(&fyne.ShortcutPaste{Clipboard: cb})
	e.capa.TypedShortcut(ctrl(fyne.KeyD))
	if n := len(e.m.Diapos[0].Cajas); n != 4 || e.m.caja(0, "f") == nil {
		t.Errorf("en solo lectura el portapapeles ha cambiado la diapositiva (%d cajas)", n)
	}
}

// TestSeleccionPegarTextoAjenoNoHaceNada: un texto que no es un recorte no pega cajas.
func TestSeleccionPegarTextoAjenoNoHaceNada(t *testing.T) {
	e, ctx, _ := montar(t, grupoYConector)
	cb := fyne.CurrentApp().Clipboard()
	cb.SetContent("hola, mundo")
	e.SeleccionarSolo("f")
	e.capa.TypedShortcut(&fyne.ShortcutPaste{Clipboard: cb})
	if len(e.m.Diapos[0].Cajas) != 4 || ctx.Guardado == editor.Sucio {
		t.Error("pegar un texto cualquiera sobre el escenario ha cambiado algo")
	}
}

// TestSeleccionPegarEnUnaCajaEnEdicionPegaTexto: dentro de una caja que se escribe,
// pegar un recorte escribe su texto (nunca cajas ni su JSON).
func TestSeleccionPegarEnUnaCajaEnEdicionPegaTexto(t *testing.T) {
	e, _, _ := montar(t, grupoYConector)
	r := Recortar([]Caja{{ID: "x", W: 10, H: 10, HTML: "<h1>Uno &amp; dos</h1>"}, {ID: "y", W: 1, H: 1, Tipo: "shape"}}, nil, []string{"x", "y"})
	fyne.CurrentApp().Clipboard().SetContent(r.Texto())
	e.SeleccionarSolo("t")
	e.EmpezarEscritura("t")
	f := e.Folio()
	if f == nil {
		t.Fatal("la caja de texto no se abre para escribir")
	}
	f.AlFinal()
	enfocado := fyne.CurrentApp().Driver().CanvasForObject(e.capa).Focused()
	sc, ok := enfocado.(fyne.Shortcutable)
	if !ok {
		t.Fatalf("lo enfocado al escribir no recibe atajos (%T)", enfocado)
	}
	sc.TypedShortcut(&fyne.ShortcutPaste{Clipboard: fyne.CurrentApp().Clipboard()})
	e.TerminarEscritura()
	html := e.m.caja(0, "t").HTML
	if !strings.Contains(html, "holaUno &amp; dos") || strings.Contains(html, PrefijoRecorte) {
		t.Errorf("pegar un recorte en el texto deja %q", html)
	}
	if len(e.m.Diapos[0].Cajas) != 4 {
		t.Error("pegar dentro de un texto ha pegado cajas")
	}
}

// TestPortapapelesNoPasaDelTope: un pegado que dejaría la presentación por encima del
// tope de guardado se rechaza entero, como en la web.
func TestPortapapelesNoPasaDelTope(t *testing.T) {
	e, _, _ := montar(t, grupoYConector)
	cb := fyne.CurrentApp().Clipboard()
	grande := Caja{ID: "img", Tipo: "image", W: 10, H: 10, Src: "data:image/png;base64," + strings.Repeat("A", MaxCaracteres)}
	cb.SetContent(Recorte{Cajas: []Caja{grande}}.Texto())
	e.SeleccionarSolo("f")
	e.capa.TypedShortcut(&fyne.ShortcutPaste{Clipboard: cb})
	if len(e.m.Diapos[0].Cajas) != 4 || e.sel != "f" {
		t.Error("un pegado demasiado grande ha entrado")
	}
}
