// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// hubDoble apunta lo que se le pide y contesta vacío.
type hubDoble struct {
	mu       sync.Mutex
	origenes []string
	apps     []string
}

func (h *hubDoble) RecentDe(_ context.Context, o []string, _ int) (api.HubBusqueda, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.origenes = o
	return api.HubBusqueda{}, nil
}

func (h *hubDoble) Search(context.Context, string, int) (api.HubBusqueda, error) {
	return api.HubBusqueda{}, nil
}

func (h *hubDoble) Apps(context.Context) ([]string, error) { return h.apps, nil }

// TestElFiltroDeLaActividadVaAlHub: la web manda `sources=<clave>` y así los veinte que
// pide son veinte de ESE producto. Filtrar en el cliente daba menos filas con el filtro.
//
// Visto en ROJO a mano el 2026-09-22 (en el frontal): pasando `nil` en vez de los
// orígenes, el test dice que al Hub no le llegó ningún filtro.
func TestElFiltroDeLaActividadVaAlHub(t *testing.T) {
	d := &hubDoble{}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).Con(d)
	v.cargarActividad("drive")
	if !slices.Equal(d.origenes, []string{"drive"}) {
		t.Errorf("al Hub le llegó el filtro %v y tenía que ser [drive]", d.origenes)
	}
	v.cargarActividad("")
	if d.origenes != nil {
		t.Errorf("sin filtro al Hub le llegó %v", d.origenes)
	}
}

// TestLaRejillaEsLoDesplegado: lo que el Hub dice que está levantado es lo que se ofrece,
// con el Hub siempre (es la puerta de vuelta a casa).
func TestLaRejillaEsLoDesplegado(t *testing.T) {
	d := &hubDoble{apps: []string{"mail", "docs"}}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).Con(d)
	v.cargarDesplegadas()
	claves := make([]string, 0, len(v.lanzador.Apps))
	for _, p := range v.lanzador.Apps {
		claves = append(claves, p.Marca.Clave())
	}
	if !slices.Equal(claves, []string{"one", "mail", "docs", "sheets", "slides", "pdf"}) {
		t.Errorf("con mail y docs levantados se ofrecen %v", claves)
	}
}

// TestDestinoDeRuta: lo que llega por `sytue-one:` abre SU pantalla, con y sin el
// prefijo `/one` que trae la dirección web de un aviso.
func TestDestinoDeRuta(t *testing.T) {
	for _, c := range []struct {
		ruta string
		d    Destino
		ok   bool
	}{
		{"/notificaciones", AAvisos, true},
		{"/one/actividad", AActividad, true},
		{"/buscar/", ABuscar, true},
		{"/", AInicio, true},
		{"/one", AInicio, true},
		{"", AInicio, true},
		{"/compose", AInicio, false},
	} {
		if d, ok := DestinoDeRuta(c.ruta); d != c.d || ok != c.ok {
			t.Errorf("%q abre %v (%v) y tenía que abrir %v (%v)", c.ruta, d, ok, c.d, c.ok)
		}
	}
}

// TestHoyEsLoQueDaElBFF: seis como mucho, por hora de inicio, y el correo de la bandeja
// de entrada o, sin ella, la suma de todos los buzones.
//
// Visto en ROJO a mano el 2026-09-22 (en el frontal): recortando antes de ordenar, se
// cuela el evento de las 23:00 y el test lo dice.
func TestHoyEsLoQueDaElBFF(t *testing.T) {
	horas := []int{23, 9, 10, 11, 12, 13, 14, 8}
	eventos := make([]EventoDeHoy, 0, len(horas))
	for _, h := range horas {
		eventos = append(eventos, EventoDeHoy{ID: fmt.Sprint(h), Inicio: time.Date(2026, 9, 21, h, 0, 0, 0, time.UTC)})
	}
	got := RecortarHoy(eventos)
	if len(got) != MaxEventosHoy || got[0].Inicio.Hour() != 8 || got[5].Inicio.Hour() != 13 {
		t.Errorf("salen %d eventos, del %v al %v", len(got), got[0].Inicio, got[len(got)-1].Inicio)
	}
	if n := SinLeerDe([]api.Mailbox{{Role: "sent", UnreadEmails: 3}, {Role: "inbox", UnreadEmails: 17}}); n != 17 {
		t.Errorf("con bandeja de entrada salen %d sin leer y son 17", n)
	}
	if n := SinLeerDe([]api.Mailbox{{UnreadEmails: 3}, {UnreadEmails: 4}}); n != 7 {
		t.Errorf("sin bandeja de entrada salen %d sin leer y son 7", n)
	}
	if p, ok := ProductoDe("hub"); !ok || p.Marca.Clave() != "one" {
		t.Errorf("un aviso de origen «hub» no se nombra como Sytue Hub: %+v", p)
	}
}

// cuentaDoble contesta siempre los mismos claims.
type cuentaDoble struct{ r auth.Reclamos }

func (c cuentaDoble) Reclamos(context.Context) (auth.Reclamos, error) { return c.r, nil }

func (c cuentaDoble) Organizacion() string { return c.r.Tenant }

// TestLasTeselasDeLaCuentaSonLasDeLaWeb: Sytue Admin solo a quien puede entrar, y SyERP
// apagado en una cuenta personal. Antes se pintaban las dos siempre: la puerta de Admin
// daba 403 a un miembro, y el ERP se abría a quien no lo tiene.
func TestLasTeselasDeLaCuentaSonLasDeLaWeb(t *testing.T) {
	tiene := func(v *Vista, clave string) (esta, apagada bool) {
		for _, x := range test.WidgetRenderer(v.lanzador).(*lanzadorRenderer).teselas {
			if x.Producto.Marca.Clave() == clave {
				return true, x.Deshabilitada
			}
		}
		return false, false
	}
	ahora := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	v := NuevaVista(traductor(t), nil, ahora)
	if esta, _ := tiene(v, "console"); esta {
		t.Error("sin saber quién es, ya se ofrece Sytue Admin")
	}

	v = NuevaVista(traductor(t), nil, ahora).ConCuenta(cuentaDoble{auth.Reclamos{Roles: []string{"admin"}}})
	v.cargarCuenta()
	if esta, _ := tiene(v, "console"); !esta {
		t.Error("un admin no ve Sytue Admin")
	}
	if _, apagada := tiene(v, "erp"); apagada {
		t.Error("una organización ve el ERP apagado")
	}

	v = NuevaVista(traductor(t), nil, ahora).ConCuenta(cuentaDoble{auth.Reclamos{OrgKind: "personal", Roles: []string{"owner"}, Plan: "personal_free"}})
	v.cargarCuenta()
	if esta, _ := tiene(v, "console"); esta {
		t.Error("una cuenta personal gratuita ve Sytue Admin")
	}
	if esta, apagada := tiene(v, "erp"); !esta || !apagada {
		t.Errorf("en una cuenta personal el ERP está=%v apagado=%v, y va presente y apagado", esta, apagada)
	}
}

// abridorDoble apunta lo que se le pide abrir.
type abridorDoble struct{ abiertas []string }

func (a *abridorDoble) OpenURL(u *url.URL) error {
	a.abiertas = append(a.abiertas, u.String())
	return nil
}

// TestTodoElPortalEsNativo: ninguna de las ocho pantallas del portal abre ya la web; el
// alta guiada tampoco, que era la última.
//
// Visto en ROJO a mano (en el frontal): con el `if !slices.Contains(Portadas(), d)` de
// antes en `Ir`, el alta abre el navegador y la pantalla no cambia.
func TestTodoElPortalEsNativo(t *testing.T) {
	a := &abridorDoble{}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	v.abrir = a
	// Sin cargas en segundo plano que sobrevivan a la prueba: pintarían a la vez que la
	// siguiente, y el conformador de texto no es de dos gorutinas.
	for d := AInicio; d <= ABienvenida; d++ {
		v.Ir(d)
		if v.Destino() != d {
			t.Errorf("ir a %s deja la pantalla en %v", d.Ruta(), v.Destino())
		}
		if d == ADescargas {
			for range 200 {
				fyne.DoAndWait(func() {}) // la carga pinta por la cola de fyne.Do
				v.mu.Lock()
				e := v.bajadas.Estado
				v.mu.Unlock()
				if e != Cargando {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
	}
	if len(a.abiertas) != 0 {
		t.Errorf("se ha abierto la web: %v", a.abiertas)
	}
}

// TestSinOrganizacionSeVaAlAlta: quien entra sin organización y sin haber terminado el
// alta guiada acaba en el asistente, como en `index.astro`; con organización
// no se pregunta siquiera, y si Sytue ID no contesta tampoco se manda a ninguna parte.
//
// Visto en ROJO a mano el 2026-09-23 (en el frontal): quitando el `r.Tenant == ""`, a
// quien ya tiene organización también se le abre el asistente.
func TestSinOrganizacionSeVaAlAlta(t *testing.T) {
	ahora := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		nombre   string
		tenant   string
		hecha    bool
		fallo    bool
		abreAlta bool
	}{
		{"sin organización y a medias", "", false, false, true},
		{"sin organización y hecha", "", true, false, false},
		{"con organización", "acme", false, false, false},
		{"Sytue ID no contesta", "", false, true, false},
	} {
		a := &abridorDoble{}
		v := NuevaVista(traductor(t), nil, ahora).ConCuenta(cuentaDoble{auth.Reclamos{Sub: "ada", Tenant: c.tenant}})
		v.abrir = a
		preguntado := false
		v.ConAlta(func(context.Context) (bool, error) {
			preguntado = true
			if c.fallo {
				return false, fmt.Errorf("sin respuesta")
			}
			return c.hecha, nil
		})
		v.cargarCuenta()
		if abre := v.Destino() == ABienvenida; abre != c.abreAlta {
			t.Errorf("%s: la pantalla queda en %v y tenía que ir al alta=%v", c.nombre, v.Destino(), c.abreAlta)
		}
		if c.tenant != "" && preguntado {
			t.Errorf("%s: se pregunta por el alta a quien ya tiene organización", c.nombre)
		}
	}
}
