// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func hilos(cs []Conversacion) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.HiloID)
	}
	return out
}

// TestCadaFiltroRapidoDejaVerLoSuyo es la tabla de `filtrarConversaciones`, con la
// conversación abierta que se queda aunque ya no cumpla.
//
// VISTO EN ROJO (2026-11-05): sin la excepción de la abierta, leerla con «No leídos»
// puesto la saca de la lista bajo el cursor.
func TestCadaFiltroRapidoDejaVerLoSuyo(t *testing.T) {
	cs := []Conversacion{
		{HiloID: "nueva", SinLeer: true},
		{HiloID: "estrella", Destacada: true},
		{HiloID: "adjunto", ConAdjunto: true},
	}
	for f, quiero := range map[FiltroRapido][]string{
		FiltroTodas: {"nueva", "estrella", "adjunto"}, FiltroNoLeidas: {"nueva"},
		FiltroDestacadas: {"estrella"}, FiltroConAdjuntos: {"adjunto"},
	} {
		if got := hilos(FiltrarConversaciones(cs, f, "")); !slices.Equal(got, quiero) {
			t.Errorf("filtro %d: %v, quería %v", f, got, quiero)
		}
	}
	if got := hilos(FiltrarConversaciones(cs, FiltroNoLeidas, "adjunto")); !slices.Equal(got, []string{"nueva", "adjunto"}) {
		t.Errorf("la abierta se ha ido: %v", got)
	}
}

// TestLaFranjaDeFiltrosConmutaYDiceCuandoNoHayNada: los tres botones de la franja
// filtran la lista, el activo es `secondary`, pulsarlo otra vez vuelve a todas, y un
// filtro sin nada que enseñar pide la página siguiente y lo dice con «Ver todas».
//
// VISTO EN ROJO (2026-11-05): sin `MasPagina` en `pintarLista` el filtro vacío se
// queda en la primera página, y sin el caso de `ponerVacio` dice «No hay correos».
func TestLaFranjaDeFiltrosConmutaYDiceCuandoNoHayNada(t *testing.T) {
	s := servicioDeEjemplo()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	w.Canvas().Capture()
	r := b.P.Bandeja.r
	if len(r.filtros) != 3 || r.filtros[0].Texto != "No leídos" {
		t.Fatalf("la franja de filtros: %d botones", len(r.filtros))
	}

	test.Tap(r.filtros[0])
	if got := hilos(b.P.Bandeja.Conversaciones); !slices.Equal(got, []string{"h1"}) {
		t.Errorf("«No leídos» deja ver %v", got)
	}
	if r.filtros[0].Variante != ui.Secundario || r.filtros[1].Variante != ui.Fantasma {
		t.Error("el filtro activo no se pinta como `secondary`")
	}

	// La ventana ya ha pedido las dos páginas al pintar sus filas; se finge que queda
	// otra para ver que un filtro sin filas la pide él.
	b.cursor = "c2"
	listas := s.pidio("Messages")
	test.Tap(r.filtros[1]) // destacados: no hay ninguno
	if !slices.Contains(s.llamadas[len(s.llamadas)-1:], "Messages(in,c2)") || s.pidio("Messages") != listas+1 {
		t.Errorf("un filtro vacío no ha pedido la página siguiente: %v", s.llamadas)
	}
	if b.P.Bandeja.Estado != Vacio || r.vacio.Titulo != "Nada de aquí cumple este filtro" {
		t.Errorf("el vacío del filtro: estado %v, título %q", b.P.Bandeja.Estado, r.vacio.Titulo)
	}

	test.Tap(r.filtros[1]) // pulsar el activo vuelve a todas
	if b.P.Bandeja.Filtro != FiltroTodas || len(b.P.Bandeja.Conversaciones) != 3 {
		t.Errorf("tras quitar el filtro: %v, %v", b.P.Bandeja.Filtro, hilos(b.P.Bandeja.Conversaciones))
	}
}

// TestUnFiltroNoTapaQueLaListaNoCargo: pulsar un filtro con la lista en error, sin red
// o sin permiso sigue diciendo eso, no «nada cumple este filtro» ni «no hay correos».
//
// VISTO EN ROJO (2026-11-05): con `pintarLista` sin condición en `ElegirFiltro`, el 403
// pasa a Vacio al pulsar «No leídos».
func TestUnFiltroNoTapaQueLaListaNoCargo(t *testing.T) {
	for _, c := range []struct {
		err  error
		want Estado
	}{
		{&api.Error{Status: http.StatusForbidden}, SinPermiso},
		{&api.Error{Status: http.StatusBadGateway}, Error},
	} {
		s := servicioDeEjemplo()
		s.errLista = c.err
		b, _ := buzonDePrueba(t, s)
		b.Arrancar(context.Background())
		for _, f := range []FiltroRapido{FiltroNoLeidas, FiltroTodas} {
			b.ElegirFiltro(f)
			if b.P.Bandeja.Estado != c.want {
				t.Errorf("filtro %d con la lista en %d la deja en %d", f, c.want, b.P.Bandeja.Estado)
			}
		}
	}
}

// TestUnFiltroVacioParaTrasElTope: un filtro sin coincidencias pide TopePaginasFiltro
// páginas por su cuenta y para; el vacío ofrece entonces «Cargar más», que pide otra
// tanda. Sin tope, «Destacados» en un buzón enorme sin estrellas se lo trae entero.
//
// VISTO EN ROJO (2026-11-05): sin el tope, el filtro pide las doce páginas de golpe.
func TestUnFiltroVacioParaTrasElTope(t *testing.T) {
	s := servicioDeEjemplo()
	s.paginas, s.siguiente = map[string][]api.MailMessage{}, map[string]string{}
	cursor := ""
	// La primera página llena la ventana, para que el final de la lista no pida más
	// por su cuenta: aquí solo pide el filtro.
	for i := range 12 {
		n := 1
		if i == 0 {
			n = 30
		}
		for j := range n {
			id := "p" + numero(i) + "-" + numero(j)
			s.paginas[cursor] = append(s.paginas[cursor],
				sobre(id, "h"+id, "in", "Sin estrella", true, time.Duration(i*30+j)*time.Minute))
		}
		if i < 11 {
			s.siguiente[cursor] = "c" + numero(i+1)
			cursor = "c" + numero(i+1)
		}
	}
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	w.Canvas().Capture()
	antes := s.pidio("Messages")

	b.ElegirFiltro(FiltroDestacadas)
	if got := s.pidio("Messages") - antes; got != TopePaginasFiltro {
		t.Fatalf("el filtro vacío ha pedido %d páginas y el tope es %d", got, TopePaginasFiltro)
	}
	w.Canvas().Capture()
	r := b.P.Bandeja.r
	boton, ok := r.vacio.Accion.(*ui.Boton)
	if !b.P.Bandeja.FiltroTope || !ok || boton.Texto != "Cargar más" {
		t.Fatalf("tras el tope el vacío no ofrece «Cargar más»: tope %v, %#v", b.P.Bandeja.FiltroTope, r.vacio.Accion)
	}
	test.Tap(boton)
	if got := s.pidio("Messages") - antes; got != 2*TopePaginasFiltro {
		t.Errorf("«Cargar más» ha dejado %d páginas pedidas, quería %d", got, 2*TopePaginasFiltro)
	}
}

// TestRepintarConUnaPaginaEnCaminoNoGastaElTope: abrir, un movimiento optimista o un
// refresco repintan la lista mientras la página del filtro aún no ha llegado; esos
// repintados no piden nada (`MasPagina` espera a la que va en camino) y no pueden
// contar como páginas, o el filtro vacío ofrece «Cargar más» tras dos o tres.
//
// VISTO EN ROJO (2026-11-06): sin el `if b.pidiendo` de `pintarLista`, los cuatro
// repintados gastan el tope y al llegar la página solo se pide una más.
func TestRepintarConUnaPaginaEnCaminoNoGastaElTope(t *testing.T) {
	s := servicioDeEjemplo()
	s.paginas, s.siguiente = map[string][]api.MailMessage{}, map[string]string{}
	cursor := ""
	for i := range 12 {
		n := 1
		if i == 0 {
			n = 30
		}
		for j := range n {
			id := "p" + numero(i) + "-" + numero(j)
			s.paginas[cursor] = append(s.paginas[cursor],
				sobre(id, "h"+id, "in", "Sin estrella", true, time.Duration(i*30+j)*time.Minute))
		}
		if i < 11 {
			s.siguiente[cursor] = "c" + numero(i+1)
			cursor = "c" + numero(i+1)
		}
	}
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	w.Canvas().Capture()
	antes := s.pidio("Messages")

	// La página del filtro «en camino»: los repintados de mientras no piden.
	b.P.Bandeja.Filtro = FiltroDestacadas
	b.pidiendo = true
	for range 4 {
		b.pintarLista()
	}
	if got := s.pidio("Messages") - antes; got != 0 {
		t.Fatalf("con una página en camino se han pedido %d", got)
	}
	// Llega (sin nada que cumpla): desde aquí el filtro tiene sus cinco páginas.
	b.pidiendo = false
	b.pintarLista()
	if got := s.pidio("Messages") - antes; got != TopePaginasFiltro {
		t.Errorf("tras los repintados el filtro ha pedido %d páginas y el tope es %d", got, TopePaginasFiltro)
	}
}

// TestAbrirOtraSacaLaQueYaNoCumple: con «Destacados», a la abierta se le quita la
// estrella y se abre otra destacada ya leída; la primera sale de la lista, como en la
// web, aunque abrir la segunda no marque nada.
//
// VISTO EN ROJO (2026-11-05): sin `pintarLista` en `Abrir`, la primera se queda.
func TestAbrirOtraSacaLaQueYaNoCumple(t *testing.T) {
	s := servicioDeEjemplo()
	for i := range s.paginas[""] {
		s.paginas[""][i].Keywords = append(s.paginas[""][i].Keywords, api.MailFlagSeen, api.MailFlagFlagged)
	}
	b, _ := buzonDePrueba(t, s)
	b.Arrancar(context.Background())
	b.ElegirFiltro(FiltroDestacadas)
	convs := b.P.Bandeja.Conversaciones
	if len(convs) != 2 {
		t.Fatalf("destacadas: %v", hilos(convs))
	}
	b.Abrir(convs[0])
	b.marcar(idsDe(convs[:1]), api.MailFlagFlagged, false)
	if got := hilos(b.P.Bandeja.Conversaciones); !slices.Contains(got, convs[0].HiloID) {
		t.Fatalf("la abierta tiene que quedarse mientras se lee: %v", got)
	}
	b.Abrir(b.P.Bandeja.Conversaciones[1])
	if got := hilos(b.P.Bandeja.Conversaciones); slices.Contains(got, convs[0].HiloID) {
		t.Errorf("la que ya no es destacada sigue en la lista: %v", got)
	}
}
