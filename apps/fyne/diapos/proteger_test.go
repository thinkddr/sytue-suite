// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"os"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// Los VECTORES COMPARTIDOS con la web: los mismos casos los exige
// `packages/office/test/slides-protect.test.ts` a `cambioProtegido`. Son una sola regla
// escrita dos veces (y una tercera en el servidor): si un caso cambia, cambia en los dos.
const rutaVectoresProteccion = "../../frontend/packages/office/test/vectores/diapos-proteccion.json"

// TestLaProteccionCoincideConLaWeb recorre la tabla compartida. Cada caso trae el
// modelo de después (y el de antes si no es la base) y si el paso toca lo bloqueado.
//
// VISTO EN ROJO: quitando la comparación del marco de CambioProtegido fallan los cuatro
// casos de tema, patrón y tamaño.
func TestLaProteccionCoincideConLaWeb(t *testing.T) {
	crudo, err := os.ReadFile(rutaVectoresProteccion)
	if err != nil {
		t.Fatalf("no se pueden leer los vectores: %v", err)
	}
	var doc struct {
		Base  json.RawMessage `json:"base"`
		Casos []struct {
			Nombre    string          `json:"nombre"`
			Antes     json.RawMessage `json:"antes"`
			Despues   json.RawMessage `json:"despues"`
			Protegido bool            `json:"protegido"`
		} `json:"casos"`
	}
	if err := json.Unmarshal(crudo, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Casos) < 20 {
		t.Fatalf("solo hay %d casos: la tabla se ha quedado corta", len(doc.Casos))
	}
	leer := func(nombre string, b json.RawMessage) *Modelo {
		t.Helper()
		m, err := Leer(b)
		if err != nil || !m.Fiel(b) {
			t.Fatalf("%s: el vector no se lee entero (%v)", nombre, err)
		}
		return m
	}
	for _, c := range doc.Casos {
		antes := doc.Base
		if c.Antes != nil {
			antes = c.Antes
		}
		if got := CambioProtegido(leer(c.Nombre, antes), leer(c.Nombre, c.Despues)); got != c.Protegido {
			t.Errorf("%s: CambioProtegido = %v, la web dice %v", c.Nombre, got, c.Protegido)
		}
	}
}

// protegida es la presentación de los vectores: s1 bloqueada, b3 bloqueada (en s3) y el
// marco fijo; s2 y b4 libres.
const protegida = `{"slides":[` +
	`{"id":"s1","boxes":[{"id":"b1","x":10,"y":10,"w":50,"h":20,"html":"a","kind":"shape"}]},` +
	`{"id":"s2","boxes":[{"id":"b2","x":10,"y":10,"w":50,"h":20,"html":"b","kind":"shape"}]},` +
	`{"id":"s3","boxes":[{"id":"b3","x":10,"y":10,"w":50,"h":20,"html":"c","kind":"shape"},` +
	`{"id":"b4","x":10,"y":50,"w":50,"h":20,"html":"d","kind":"shape"}]}],` +
	`"theme":"carbon","protect":{"hasPassword":true,"slides":["s1"],"boxes":["b3"],"master":true}}`

func avisosProtegido(e *Editor) int {
	n := 0
	for _, a := range e.Avisos().Avisos() {
		if a.Titulo == e.t("slides.msg.protected") {
			n++
		}
	}
	return n
}

// TestUnaPresentacionProtegidaSeAbreParaEditar: antes `Fiel` no conocía `protect` y la
// presentación protegida se abría en solo lectura, también para su dueño.
func TestUnaPresentacionProtegidaSeAbreParaEditar(t *testing.T) {
	_, ctx, _ := montar(t, protegida)
	if ctx.SoloLectura {
		t.Fatal("una presentación protegida se ha abierto en solo lectura")
	}
}

// TestLoBloqueadoNoSeMueveYSeAvisaUnaVez: empujar dos veces la caja bloqueada no cambia
// el modelo, no marca nada por guardar y avisa UNA vez (como `avisoProtegidoPermitido`);
// la caja libre de la misma diapositiva sí se mueve.
func TestLoBloqueadoNoSeMueveYSeAvisaUnaVez(t *testing.T) {
	var e *Editor
	var ctx *editor.Contexto
	fyne.DoAndWait(func() {
		e, ctx, _ = montar(t, protegida)
		e.Elegir(2)
		antes := e.Serializar()
		e.SeleccionarSolo("b3")
		e.Empujar(1, 0, false)
		e.Empujar(1, 0, false)
		if e.Serializar() != antes {
			t.Errorf("la caja bloqueada se ha movido:\n%s", e.Serializar())
		}
		if ctx.Guardado == editor.Sucio {
			t.Error("un cambio vetado ha dejado el documento por guardar")
		}
		if n := avisosProtegido(e); n != 1 {
			t.Errorf("%d avisos de «bloqueado»; esperaba uno", n)
		}
		e.SeleccionarSolo("b4")
		e.Empujar(1, 0, false)
		if c := e.m.caja(e.objetivo(), "b4"); c == nil || !casi(c.X, 11) {
			t.Errorf("la caja libre no se ha movido: %+v", c)
		}
		if ctx.Guardado != editor.Sucio {
			t.Error("el cambio aceptado no se ha marcado para guardar")
		}
	})
}

// TestLoBloqueadoNoSeArrastraNiSeEscribe: el arrastre de la caja bloqueada ni empieza
// (si no, soltar la devolvería a su sitio de golpe), su texto no se abre para escribir,
// y la diapositiva bloqueada no se borra ni pregunta.
func TestLoBloqueadoNoSeArrastraNiSeEscribe(t *testing.T) {
	fyne.DoAndWait(func() {
		e, _, _ := montar(t, protegida)
		e.Elegir(2)
		e.capa.empezar("b3", false)
		if e.capa.arrastre != nil {
			t.Error("el arrastre de la caja bloqueada ha empezado")
		}
		e.Elegir(0)
		e.m.Diapos[0].Cajas[0].Tipo = "" // texto, para poder abrirla
		e.base = e.m.clonar()
		e.EmpezarEscritura("b1")
		if e.Escribiendo() != "" {
			t.Error("la caja de una diapositiva bloqueada se ha abierto para escribir")
		}
		n := len(e.m.Diapos)
		e.Eliminar()
		if len(e.m.Diapos) != n {
			t.Error("la diapositiva bloqueada se ha borrado")
		}
	})
}

// TestElDuenoEditaLoBloqueado: el propietario no tiene vetos (`esPropietario` de la web).
func TestElDuenoEditaLoBloqueado(t *testing.T) {
	fyne.DoAndWait(func() {
		tr := traductor(t)
		ctx := &editor.Contexto{Tipo: editor.Presentacion, Estado: editor.Listo, Zoom: 100, T: tr}
		e := NuevoEditor(ctx)
		e.Propietario = true
		if !e.Cargar(protegida) {
			t.Fatal("la presentación protegida no se entiende entera")
		}
		e.Elegir(2)
		e.SeleccionarSolo("b3")
		e.Empujar(1, 0, false)
		if c := e.m.caja(e.objetivo(), "b3"); c == nil || !casi(c.X, 11) {
			t.Errorf("el dueño no ha podido mover la caja bloqueada: %+v", c)
		}
	})
}

// TestLaVueltaDeUnArrastreVetado: lo que un gesto movió en vivo vuelve con el veto, y la
// diapositiva activa no se pierde aunque el cambio la hubiera cambiado.
func TestLaVueltaDeUnArrastreVetado(t *testing.T) {
	fyne.DoAndWait(func() {
		e, _, _ := montar(t, protegida)
		e.Elegir(2)
		antes := e.Serializar()
		e.cambiar(func() {
			e.m.Diapos[2].Cajas[0].X = 70
			e.activa = 0
		}, true)
		if e.Serializar() != antes || e.activa != 2 {
			t.Errorf("el veto no ha devuelto el modelo (activa %d):\n%s", e.activa, e.Serializar())
		}
	})
}
