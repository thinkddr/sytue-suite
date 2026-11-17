// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"net/url"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// areaComentario es el área de texto del panel abierto, o nil si no la hay.
func areaComentario(w fyne.Window) *ui.AreaTexto {
	var a *ui.AreaTexto
	enCapas(w, func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.AreaTexto); ok {
			a = x
		}
	})
	return a
}

func conHilo(f *falso) {
	f.comentarios = []m{
		{"id": "c1", "record_id": "r001", "author_sub": "u2", "author_name": "Bea", "body": "<b>¿Está al día?</b>", "created_at": "2026-10-04T09:00:00Z", "can_delete": false},
		{"id": "c2", "record_id": "r001", "parent_id": "c1", "author_sub": "u1", "author_name": "Yo", "body": "Sí", "created_at": "2026-10-04T10:00:00Z", "can_delete": true},
	}
}

// TestLosComentariosDeUnRegistro: la ficha dice cuántos hay, el panel los enseña en
// hilos con el texto tal cual, se responde a una raíz y se borra lo que se puede borrar.
func TestLosComentariosDeUnRegistro(t *testing.T) {
	f := nuevoFalso(3)
	conHilo(f)
	_, w := abrir(t, f)
	pulsar(t, w, "Fila 1")
	pulsar(t, w, "Comentarios (2)")
	got := textos(w)
	for _, quiero := range []string{"Comentarios de Persona 1", "Bea", "<b>¿Está al día?</b>", "Sí", "[Responder]", "[Borrar comentario]"} {
		if !strings.Contains(got, quiero) {
			t.Fatalf("el panel no enseña %q:\n%s", quiero, got)
		}
	}
	// Solo se borra lo que el servicio deja (`can_delete`): un botón, el de la respuesta.
	if strings.Count(got, "[Borrar comentario]") != 1 {
		t.Errorf("se ofrece borrar lo que no se puede:\n%s", got)
	}
	// Una respuesta no se responde: el hilo es de un nivel.
	if strings.Count(got, "[Responder]") != 1 {
		t.Errorf("se ofrece responder a una respuesta:\n%s", got)
	}
	capturar(t, w, "comentarios")

	pulsar(t, w, "Responder")
	if !strings.Contains(textos(w), "Respondiendo a Bea") {
		t.Fatalf("no dice a quién se responde:\n%s", textos(w))
	}
	a := areaComentario(w)
	if a == nil {
		t.Fatal("no hay dónde escribir")
	}
	if b := boton(w, "Comentar"); b == nil || !b.Inerte {
		t.Error("«Comentar» se puede pulsar sin texto")
	}
	a.PonerTexto("  Gracias  ")
	pulsar(t, w, "Comentar")
	if !f.vio(`POST /records/r001/comments {"body":"Gracias","parent_id":"c1"}`) {
		t.Fatalf("la respuesta no se envió con su raíz: %q", f.peticiones("POST"))
	}
	if got := textos(w); !strings.Contains(got, "Gracias") || strings.Contains(got, "Respondiendo a Bea") || a.Texto() != "" {
		t.Errorf("tras enviar, el hilo no se recarga o la caja no se vacía:\n%s", got)
	}

	pulsar(t, w, "Borrar comentario") // el primero que se puede borrar
	pulsar(t, w, "Borrar")
	if len(f.peticiones("DELETE /comments/")) != 1 {
		t.Fatalf("no se borró: %q", f.peticiones("DELETE"))
	}
	// La ficha, debajo, se ha enterado del nuevo número.
	if boton(w, "Comentarios (2)") == nil {
		t.Errorf("la ficha no cuenta de nuevo:\n%s", textos(w))
	}
}

// TestUnLectorLeeLosComentariosSinEscribir: sin rol de comentarista no hay caja de texto
// ni «Responder», pero el hilo se lee.
func TestUnLectorLeeLosComentariosSinEscribir(t *testing.T) {
	f := nuevoFalso(2)
	f.rol = api.BasesViewer
	conHilo(f)
	_, w := abrir(t, f)
	pulsar(t, w, "Fila 1")
	pulsar(t, w, "Comentarios (2)")
	got := textos(w)
	if !strings.Contains(got, "¿Está al día?") || !strings.Contains(got, "Puedes leer los comentarios pero no escribir.") {
		t.Fatalf("el lector no lee el hilo:\n%s", got)
	}
	if areaComentario(w) != nil || strings.Contains(got, "[Responder]") || strings.Contains(got, "[Comentar]") {
		t.Errorf("un lector puede escribir:\n%s", got)
	}
}

// TestUnComentaristaEscribeSinEditar: el comentarista no edita la ficha pero comenta.
func TestUnComentaristaEscribeSinEditar(t *testing.T) {
	f := nuevoFalso(2)
	f.rol = api.BasesCommenter
	_, w := abrir(t, f)
	pulsar(t, w, "Fila 1")
	if strings.Contains(textos(w), "[Guardar]") {
		t.Fatal("un comentarista edita el registro")
	}
	pulsar(t, w, "Comentarios (0)")
	if !strings.Contains(textos(w), "Todavía no hay comentarios.") || areaComentario(w) == nil {
		t.Fatalf("el comentarista no puede escribir:\n%s", textos(w))
	}
}

// TestElEnlaceDeUnAvisoAbreElHilo: `?table=&record=` abre la tabla, la ficha y el hilo.
func TestElEnlaceDeUnAvisoAbreElHilo(t *testing.T) {
	const base, tabla, reg = "0b7e4c1a-2f3d-4a5b-8c6d-7e8f9a0b1c2d", "1b7e4c1a-2f3d-4a5b-8c6d-7e8f9a0b1c2d", "2b7e4c1a-2f3d-4a5b-8c6d-7e8f9a0b1c2d"
	u, _ := url.Parse("sytue-bases:/bases/base/" + base + "?table=" + strings.ToUpper(tabla) + "&record=" + reg)
	if ta, re := RegistroDeEnlace(u); ta != tabla || re != reg {
		t.Fatalf("RegistroDeEnlace = %q, %q", ta, re)
	}
	u, _ = url.Parse("sytue-bases:/bases/base/" + base + "?table=x&record=" + reg)
	if ta, re := RegistroDeEnlace(u); ta != "" || re != "" {
		t.Fatalf("un enlace con una tabla que no es un id da %q, %q", ta, re)
	}

	f := nuevoFalso(3)
	conHilo(f)
	p, w := abrir(t, f)
	p.AbrirEnlace("b1", "t2", "e2") // otra tabla que la primera
	if p.abierta.Activa != "t2" {
		t.Fatalf("el enlace no abre su tabla: %q", p.abierta.Activa)
	}
	got := textos(w)
	if !strings.Contains(got, "Comentarios de Globex") || !strings.Contains(got, "Todavía no hay comentarios.") {
		t.Fatalf("el enlace no abre el hilo del registro:\n%s", got)
	}
	// Un enlace a una tabla que no se ve se queda en la primera, sin ficha.
	antes := len(f.peticiones("GET /records/e2"))
	p.AbrirEnlace("b1", "t9", "e2")
	if p.abierta.Activa != "t1" || len(f.peticiones("GET /records/e2")) != antes {
		t.Errorf("un enlace roto abre algo: tabla %q, %q", p.abierta.Activa, f.peticiones("GET /records/e2"))
	}
}

func TestHilosDe(t *testing.T) {
	hs := hilosDe([]api.BasesComment{{ID: "a"}, {ID: "b", ParentID: "a"}, {ID: "c", ParentID: "zz"}, {ID: "d", ParentID: "a"}})
	if len(hs) != 2 || hs[0].raiz.ID != "a" || len(hs[0].respuestas) != 2 || hs[1].raiz.ID != "c" {
		t.Fatalf("hilosDe = %+v", hs)
	}
}
