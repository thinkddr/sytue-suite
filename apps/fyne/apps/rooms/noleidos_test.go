// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestTotalNoLeidos es el gemelo de lib/totalNoLeidos.test.ts: suman todos los canales
// menos los silenciados.
func TestTotalNoLeidos(t *testing.T) {
	cs := []Canal{
		{ID: "a", SinLeer: 3},
		{ID: "dm", Clase: Directo, SinLeer: 2},
		{ID: "ruido", SinLeer: 40, Silenciado: true, Nivel: AvisarNada},
		{ID: "menciones", SinLeer: 1, Nivel: AvisarMenciones},
		{ID: "leido"},
	}
	if n := TotalNoLeidos(cs); n != 6 {
		t.Errorf("el total es %d y son 6", n)
	}
	if n := TotalNoLeidos(nil); n != 0 {
		t.Errorf("sin canales el total es %d", n)
	}
}

// TestElIconoLlevaLosNoLeidos: la sala dice el total al arrancar —sin el canal abierto,
// que se está leyendo— y otra vez cuando cambia al leer el resto, pero no lo repite si
// no cambia.
func TestElIconoLlevaLosNoLeidos(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	var dichos []int
	sa.AlCambiarNoLeidos = func(n int) { dichos = append(dichos, n) }
	sa.Arrancar(context.Background())
	if len(dichos) == 0 || dichos[len(dichos)-1] != 4 {
		t.Fatalf("al arrancar el icono dice %v; quiero 4 (los del directo)", dichos)
	}
	antes := len(dichos)
	sa.pintarCanales()
	if len(dichos) != antes {
		t.Errorf("repintar sin cambios volvió a decirlo: %v", dichos)
	}
	for _, c := range sa.P.Lateral.Canales {
		if c.ID == "d1" {
			sa.Elegir(c)
		}
	}
	if dichos[len(dichos)-1] != 0 {
		t.Errorf("leído el directo el icono dice %v; quiero 0", dichos)
	}
}

// TestElIconoCuentaElCanalAbiertoEnSegundoPlano es el gemelo del caso de
// totalNoLeidos.test.ts: con la app en segundo plano, lo que llega al canal abierto no
// lo lee nadie y suma al icono; lo propio no; al volver, se quita.
func TestElIconoCuentaElCanalAbiertoEnSegundoPlano(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	var dichos []int
	sa.AlCambiarNoLeidos = func(n int) { dichos = append(dichos, n) }
	sa.Arrancar(context.Background())
	base := dichos[len(dichos)-1]
	llega := func(id, autor string) {
		carga, _ := json.Marshal(api.RoomsMessage{ID: id, ChannelID: sa.abierto, AuthorSub: autor, Body: "hola"})
		sa.alLlegar(api.RoomsEvent{Type: "message", ChannelID: sa.abierto, Subject: autor, Payload: carga})
	}

	llega("m0", "otra") // a la vista: se está leyendo
	if got := dichos[len(dichos)-1]; got != base {
		t.Fatalf("con la app delante el icono pasó de %d a %d", base, got)
	}
	sa.EnSegundoPlano()
	llega("m1", "otra")
	llega("m2", "otra")
	llega("m2", "otra") // la misma trama otra vez (reconexión): no suma
	llega("m3", sa.Yo)
	if got := dichos[len(dichos)-1]; got != base+2 {
		t.Fatalf("en segundo plano el icono dice %d; quiero %d", got, base+2)
	}
	sa.EnPrimerPlano()
	if got := dichos[len(dichos)-1]; got != base {
		t.Errorf("al volver el icono dice %d; quiero %d", got, base)
	}
}

// TestSumaNoVisto es el gemelo de `sumaNoVisto`.
func TestSumaNoVisto(t *testing.T) {
	raiz := "r"
	ajeno := api.RoomsMessage{AuthorSub: "otra"}
	if !SumaNoVisto(true, ajeno, "yo", Canal{}) {
		t.Error("en segundo plano un mensaje ajeno no suma")
	}
	if SumaNoVisto(false, ajeno, "yo", Canal{}) ||
		SumaNoVisto(true, api.RoomsMessage{AuthorSub: "yo"}, "yo", Canal{}) ||
		SumaNoVisto(true, api.RoomsMessage{AuthorSub: "otra", ParentID: &raiz}, "yo", Canal{}) ||
		SumaNoVisto(true, ajeno, "yo", Canal{Silenciado: true}) ||
		SumaNoVisto(true, ajeno, "yo", Canal{Nivel: AvisarNada}) {
		t.Error("suman los que no deben: a la vista, propios, de hilo, silenciados o con avisos a nada")
	}
}

// TestSaltarANoLeidos es el «Saltar a no leído» de MessageView: con separador de
// «Nuevos mensajes» el botón primario flota arriba; pulsarlo deja el separador arriba
// del todo y retira el botón; cargar otra vez el canal (otro separador) lo devuelve. Y
// cede su sitio a la píldora de «cargando anteriores».
func TestSaltarANoLeidos(t *testing.T) {
	s := servicioConDos()
	base := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)
	for i := range 40 {
		s.mensajes["c1"] = append(s.mensajes["c1"], api.RoomsMessage{
			ID: "n" + strconv.Itoa(i), AuthorSub: "ana", AuthorName: "Ana", Body: "línea", CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	s.pend = []api.RoomsUnread{{ChannelID: "c1", Unread: 30}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial
	visible := func() bool {
		return slices.Contains(h.objetos, fyne.CanvasObject(h.salto)) && h.salto.Visible()
	}
	if h.IndiceNoLeidos < 0 {
		t.Fatal("abierto con 30 sin leer no hay separador")
	}
	if !visible() {
		t.Fatal("con separador de no leídos no se ofrece «Saltar a no leído»")
	}
	if h.salto.Texto != h.T.T("message.jumpToUnread", nil) || h.salto.Variante != ui.Primario || h.salto.Talla != ui.TallaSM {
		t.Errorf("el botón es %q en variante %d y talla %d", h.salto.Texto, h.salto.Variante, h.salto.Talla)
	}
	if b := h.salto.Position(); b.Y != tema.Espacio(2) || b.X+h.salto.Size().Width/2 != h.Size().Width/2 {
		t.Errorf("el botón está en %v y va arriba y centrado (ancho %v)", b, h.Size().Width)
	}
	sc := h.cuerpo.(*container.Scroll)
	if !alFondo(sc) {
		t.Fatal("el canal no se abre por el final")
	}
	var sep *Separador
	for _, o := range sc.Content.(*fyne.Container).Objects {
		if x, ok := o.(*Separador); ok && x.Acento {
			sep = x
		}
	}
	if sep == nil {
		t.Fatal("no se pinta «Nuevos mensajes»")
	}

	test.Tap(h.salto)
	if sc.Offset.Y != sep.Position().Y {
		t.Errorf("tras saltar la vista está en y = %v y el separador en %v", sc.Offset.Y, sep.Position().Y)
	}
	if visible() || !h.SaltoUsado {
		t.Error("usado, el botón sigue ahí")
	}
	h.Refresh() // un repintado cualquiera (llega algo) no lo devuelve ni mueve la vista
	sc = h.cuerpo.(*container.Scroll)
	if visible() || sc.Offset.Y != sep.Position().Y {
		t.Errorf("tras repintar: botón %v, vista en %v", visible(), sc.Offset.Y)
	}

	sa.cargarMensajes("c1", 10)
	if !visible() {
		t.Error("con otro separador el botón no vuelve")
	}
	h.CargandoAnteriores = true
	h.Refresh()
	if visible() || !slices.Contains(h.objetos, fyne.CanvasObject(h.pildora)) {
		t.Error("cargando anteriores, el botón no cede el hueco a la píldora")
	}
	h.CargandoAnteriores = false
	sa.cargarMensajes("c1", 0)
	if visible() {
		t.Error("sin no leídos se ofrece saltar")
	}
}
