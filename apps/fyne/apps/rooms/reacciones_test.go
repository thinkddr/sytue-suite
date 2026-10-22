// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// «Quién reaccionó» es `reactedByLabel`: tres nombres como mucho y «N más»; sin
// ningún nombre conocido, solo cuántos.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con MaxNombresReaccion a 4 salen los cuatro
// nombres y ningún «1 más».
func TestQuienReacciono(t *testing.T) {
	tr := traductor(t)
	nombres := map[string]string{"a": "Ana", "b": "Beto", "c": "Carla", "d": "Dani"}
	if got, quiero := QuienReacciono(tr, "es", "👍", []string{"a", "b", "c", "d"}, nombres),
		tr.T("reaction.byNames", map[string]any{"count": 4, "names": "Ana, Beto, Carla y " + tr.T("reaction.andMore", map[string]any{"count": 1}), "emoji": "👍"}); got != quiero {
		t.Errorf("con cuatro: %q; quiero %q", got, quiero)
	}
	if got := QuienReacciono(tr, "es", "👍", []string{"x", "y"}, nombres); got != tr.T("reaction.byCount", map[string]any{"count": 2, "emoji": "👍"}) {
		t.Errorf("sin nombres: %q", got)
	}
	if QuienReacciono(tr, "es", "👍", nil, nombres) != "" {
		t.Error("sin subs dice algo")
	}
}

// Tras las reacciones va el «+», que abre el selector colgando de él; y la píldora
// lleva su pista con los nombres, que se abre al pasar por encima.
func TestElMasTrasLasReaccionesAbreElSelector(t *testing.T) {
	s := servicioConDos()
	s.roster = []api.RoomsMember{{Subject: "ana", Name: "Ana Ruiz"}, {Subject: "yo", Name: "Yo"}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial
	var desde fyne.CanvasObject
	h.AlEmojis = func(_ Mensaje, d fyne.CanvasObject) { desde = d }
	h.Refresh()
	f := filaDe(t, h, "m4")
	test.WidgetRenderer(f)
	if f.masReaccion == nil {
		t.Fatal("la fila con reacciones no lleva el «+»")
	}
	test.Tap(f.masReaccion)
	if desde != f.masReaccion {
		t.Errorf("el «+» abre el selector colgando de %v", desde)
	}
	if got := f.altoTira(); got != AltoMasReaccion {
		t.Errorf("la tira mide %v y con el «+» son %v", got, AltoMasReaccion)
	}
	if q := h.Mensajes[3].Reacciones[0].Quien; q == "" || f.reacciones[0] == nil {
		t.Errorf("la reacción no sabe quién: %q", q)
	}
	if _, ok := f.reacciones[0].(interface{ Abierta() bool }); !ok {
		t.Errorf("la píldora va sin su pista: %T", f.reacciones[0])
	}
}

// «Reenviar» publica en el destino la cita con su autor y el enlace al original; la
// caja filtra los canales por nombre.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con el `> ` fuera de cada línea en
// CuerpoReenviado, la segunda línea del original sale sin citar.
func TestReenviarPublicaLaCita(t *testing.T) {
	if got := CuerpoReenviado("hola\nadiós", "Ana", "https://x/y"); got != "Reenviado de **Ana**:\n> hola\n> adiós\n\nhttps://x/y" {
		t.Errorf("la cita es %q", got)
	}
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Apex = "sytue.eu"
	sa.Arrancar(context.Background())
	m := sa.P.Canal.Historial.Mensajes[0]
	m.Adjuntos = []Adjunto{{ID: "privado-original", Nombre: "privado.txt"}}
	sa.AbrirReenviar(m)
	r := sa.reenviar
	test.WidgetRenderer(r)
	if len(r.Candidatos()) != 2 {
		t.Fatalf("hay %d destinos y son 2", len(r.Candidatos()))
	}
	r.Caja().PonerTexto("ana")
	if cs := r.Candidatos(); len(cs) != 1 || cs[0].ID != "d1" {
		t.Fatalf("«ana» deja %+v", cs)
	}
	r.AlElegir(r.Candidatos()[0])
	if !s.pidio("Post(d1," + CuerpoReenviado("hola", "Ana", sa.enlaceA(m)) + ")") {
		t.Errorf("reenviar pidió %v", s.llamadas)
	}
	if len(s.envios) != 0 {
		t.Fatalf("reenviar una referencia copió los descriptores privados del original: %+v", s.envios)
	}
}

// «Guardar en Notepads» abre Notepads por su esquema con el vínculo al mensaje, y
// «Visto por» pide los lectores.
func TestGuardarEnNotesYVistoPor(t *testing.T) {
	s := servicioConDos()
	s.lectores = []api.RoomsReader{{Subject: "ana", Name: "Ana"}}
	sa, _ := salaDePrueba(t, s)
	ab := &abridorFalso{}
	sa.Abridor = ab
	sa.Arrancar(context.Background())
	m := sa.P.Canal.Historial.Mensajes[0]
	sa.GuardarEnNotes(m)
	if len(ab.urls) != 1 || !strings.HasPrefix(ab.urls[0], "sytue-notes:") || !strings.Contains(ab.urls[0], "link=room_message") ||
		!strings.Contains(ab.urls[0], "m1") || !strings.Contains(ab.urls[0], "label=hola") {
		t.Errorf("abre %v", ab.urls)
	}
	sa.AbrirLectores(m)
	if !s.pidio("Readers(m1)") {
		t.Errorf("«Visto por» no pide los lectores: %v", s.llamadas)
	}
	sa.pregunta.Cerrar()
}

// abridorFalso apunta lo que se le pide abrir.
type abridorFalso struct{ urls []string }

func (a *abridorFalso) OpenURL(u *url.URL) error {
	a.urls = append(a.urls, u.String())
	return nil
}
