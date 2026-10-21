// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLeerUnEnlaceEntrante: lo que `pages/index.astro` lee de la query, y el cuaderno de
// la dirección canónica `/notebooks/<id>` que redirige a ella.
func TestLeerUnEnlaceEntrante(t *testing.T) {
	casos := []struct {
		url    string
		quiero Entrante
		ok     bool
	}{
		{"sytue-notes:/?notebook=c1&note=n1&h=destino-2", Entrante{Cuaderno: "c1", Nota: "n1", Ancla: "destino-2"}, true},
		{"sytue-notes:/notebooks/c2?note=n7", Entrante{Cuaderno: "c2", Nota: "n7"}, true},
		{"sytue-notes:/?link=calendar_event:e1&label=Reuni%C3%B3n", Entrante{Vinculo: "calendar_event:e1", Rotulo: "Reunión"}, true},
		{"sytue-notes:/", Entrante{}, false},
		{"sytue-notes:/ajustes", Entrante{}, false},
	}
	for _, c := range casos {
		u, err := url.Parse(c.url)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := LeerEntrante(u); got != c.quiero || ok != c.ok {
			t.Errorf("%s da %+v (%v), quiero %+v (%v)", c.url, got, ok, c.quiero, c.ok)
		}
	}
}

// TestUnEnlaceEntranteLlegaAntesDeCargar: el enlace llega con la app recién abierta,
// sin cuadernos todavía. Al cargarlos se abre el SUYO y no el primero, se selecciona su
// nota y, cuando están sus notas, se crea UNA nota vinculada del tipo que toca
// (`TIPO_POR_DESTINO`), con el rótulo de título y de enlace, que es lo que hace la web.
//
// VISTO EN ROJO (2026-09-24, en el frontal): sin `aplicarEntrante` en `cargarCuadernos`
// se abre «Mudanza», el primero; y sin quitar el enlace al usarlo, cargar las notas
// otra vez creaba una segunda nota vinculada.
func TestUnEnlaceEntranteLlegaAntesDeCargar(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"GET /notebooks":               `{"notebooks":[{"id":"c1","title":"Mudanza"},{"id":"c2","title":"Trabajo"}]}`,
		"GET /notebooks/c2/notes/page": `{"notes":[{"id":"n7","notebook_id":"c2","kind":"text"}]}`,
		"POST /notebooks/c2/notes":     `{"id":"n9","notebook_id":"c2","kind":"event","title":"Reunión"}`,
	})
	p.cuadernos, p.activo, p.notas = nil, Cuaderno{}, nil
	u, _ := url.Parse("sytue-notes:/notebooks/c2?note=n7&link=calendar_event:e1&label=Reuni%C3%B3n")
	if !p.AtenderEnlace(u) {
		t.Fatal("el enlace no se reconoce")
	}
	p.cargarCuadernos()
	creada := func() bool {
		for _, r := range falso.rutas() {
			if r == "POST /notebooks/c2/notes" {
				return true
			}
		}
		return false
	}
	// La nota vinculada se PREGUNTA (TestUnEnlaceNoCreaLaNotaSinPreguntar): aquí se dice que sí.
	esperar(t, func() bool {
		var preguntada bool
		fyne.DoAndWait(func() { preguntada = p.vinculoPendiente != nil })
		return preguntada
	})
	fyne.DoAndWait(p.aceptarVinculo)
	esperar(t, func() bool {
		var listo bool
		fyne.DoAndWait(func() { _, listo = p.nota("n9") })
		return listo && creada()
	})
	var activo string
	fyne.DoAndWait(func() { activo = p.activo.ID })
	if activo != "c2" {
		t.Fatalf("se ha abierto %q y no el cuaderno del enlace", activo)
	}
	c := falso.ultima(t, "POST /notebooks/c2/notes")
	links, _ := c["links"].([]any)
	l, _ := links[0].(map[string]any)
	if c["kind"] != "event" || c["title"] != "Reunión" || l["target_kind"] != "calendar_event" ||
		l["target_id"] != "e1" || l["label"] != "Reunión" {
		t.Errorf("la nota vinculada viaja con %v", c)
	}
	// Volver a cargar las notas no la crea otra vez.
	fyne.DoAndWait(func() { p.aplicarEntrante() })
	n := 0
	for _, r := range falso.rutas() {
		if strings.HasPrefix(r, "POST ") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("el enlace ha creado %d notas", n)
	}
}

// esperar espera a que algo que corre en segundo plano se cumpla, con un tope.
func esperar(t *testing.T, listo func() bool) {
	t.Helper()
	fin := time.Now().Add(10 * time.Second)
	for !listo() {
		if time.Now().After(fin) {
			t.Fatal("no ha llegado a tiempo")
		}
		fyne.DoAndWait(func() {}) // lo que el fondo encoló con fyne.Do corre en esta rutina
		time.Sleep(10 * time.Millisecond)
	}
}

// TestElVinculoEsperaAlCuadernoCifrado: en un cuaderno cifrado cerrado la nota vinculada
// no se crea —la web espera a `!bloqueado`—, y se crea, cifrada, en cuanto se abre.
func TestElVinculoEsperaAlCuadernoCifrado(t *testing.T) {
	_, clave, err := api.NuevoCifradoNotas("contraseña larga de prueba")
	if err != nil {
		t.Fatal(err)
	}
	p, falso := pantallaConServicio(t, map[string]string{
		"POST /notebooks/c1/encrypted-notes": `{"id":"n9","notebook_id":"c1","kind":"event"}`,
	})
	p.activo.Cifrado, p.bloqueado = true, true
	u, _ := url.Parse("sytue-notes:/?link=calendar_event:e1&label=Reuni%C3%B3n")
	p.AtenderEnlace(u)
	fyne.DoAndWait(func() {})
	if r := falso.rutas(); len(r) != 0 {
		t.Fatalf("cerrado, el enlace ya ha pedido %v", r)
	}
	p.abrirCon("c1", clave)
	// Abierto, la nota se pregunta; se dice que sí.
	esperar(t, func() bool {
		var preguntada bool
		fyne.DoAndWait(func() { preguntada = p.vinculoPendiente != nil })
		return preguntada
	})
	fyne.DoAndWait(p.aceptarVinculo)
	esperar(t, func() bool {
		var listo bool
		fyne.DoAndWait(func() { _, listo = p.nota("n9") })
		return listo
	})
	c := falso.ultima(t, "POST /notebooks/c1/encrypted-notes")
	claro, err := api.DesempaquetarNota(clave, c["content_cipher"].(string))
	if c["title"] != "" || err != nil || claro.Title != "Reunión" {
		t.Errorf("la nota vinculada viaja con %v (dentro, %+v %v)", c, claro, err)
	}
}

// TestUnEnlaceNoCreaLaNotaSinPreguntar: un enlace `sytue-notes:…?link=` lo abre cualquier
// página, cualquier correo o cualquier otra app del teléfono. Con el cuaderno abierto y
// sus notas cargadas, atenderlo creaba la nota en el acto, con el título que trajera
// (NAT-05). Ahora queda preguntada y no se pide nada al servicio hasta que se acepta.
func TestUnEnlaceNoCreaLaNotaSinPreguntar(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"POST /notebooks/c1/notes": `{"id":"n9","notebook_id":"c1","kind":"event","title":"Reunión"}`,
	})
	u, _ := url.Parse("sytue-notes:/?link=calendar_event:e1&label=Reuni%C3%B3n")
	if !p.AtenderEnlace(u) {
		t.Fatal("el enlace no se reconoce")
	}
	// Lo que el fondo fuera a pedir, que le dé tiempo a pedirlo.
	for range 30 {
		fyne.DoAndWait(func() {})
		time.Sleep(10 * time.Millisecond)
	}
	if r := falso.rutas(); len(r) != 0 {
		t.Fatalf("sin que nadie haya dicho que sí, el enlace ya ha pedido %v", r)
	}
}
