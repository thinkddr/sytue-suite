// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// «Compartir en Rooms» llega por el esquema con el texto ya puesto: la pantalla tapa la
// sala con él en la caja, busca entre los canales y, al elegir uno, publica, se retira y
// abre ese canal. Sin texto, los destinos se apagan.
//
// VISTO EN ROJO (2026-09-24, en el frontal, saboteado a mano): sin rehacer los destinos
// en el Refresh de la pantalla, la búsqueda «ana» sigue pintando los dos.
func TestCompartirEnRooms(t *testing.T) {
	u, _ := url.Parse(api.Esquema("rooms") + ":" + api.RutaCompartir(api.Compartir{Origen: "drive", ID: "f1", Texto: "Acta.pdf https://x/y"}))
	c, ok := api.LeerCompartir(u)
	if !ok {
		t.Fatalf("no se lee %s", u)
	}
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.Compartir(c)
	p := sa.compartir
	if p == nil || sa.P.Pagina() != p || p.Texto != "Acta.pdf https://x/y" || p.Estado != Listo || len(p.Canales) != 2 {
		t.Fatalf("la pantalla abre con %+v", p)
	}
	test.WidgetRenderer(p)
	p.AlBuscar("ana")
	if len(p.Canales) != 1 || p.Canales[0].ID != "d1" {
		t.Fatalf("«ana» deja %+v", p.Canales)
	}
	// Lo PINTADO, no solo los datos: la lista es el último hijo de la columna.
	if lista := p.cuerpo.Objects[len(p.cuerpo.Objects)-1].(*fyne.Container); len(lista.Objects) != 1 {
		t.Fatalf("«ana» pinta %d destinos", len(lista.Objects))
	}
	s.errPost = errors.New("caído")
	p.AlCompartir(p.Canales[0])
	if sa.P.Pagina() == nil {
		t.Fatal("con el servicio caído la pantalla se va")
	}
	s.errPost = nil
	p.AlCompartir(p.Canales[0])
	if !s.pidio("Post(d1,Acta.pdf https://x/y)") || sa.P.Pagina() != nil || sa.abierto != "d1" {
		t.Errorf("compartir: %v, página %v, abierto %q", s.llamadas, sa.P.Pagina(), sa.abierto)
	}
}
