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

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// hubDoble apunta lo que se le pide y contesta vacío.
type hubDoble struct {
	mu       sync.Mutex
	origenes []string
	apps     []string
	// paginas son las respuestas de la búsqueda, una por `offset`; desdes, lo pedido.
	paginas []api.HubBusqueda
	desdes  []int
	// colgada, si está, retiene la PRIMERA petición de «más» (`desde > 0`) hasta que se
	// cierra, y entonces contesta `tardia`: un Hub lento.
	colgada chan struct{}
	tardia  api.HubBusqueda
}

func (h *hubDoble) RecentDe(_ context.Context, o []string, _ int) (api.HubBusqueda, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.origenes = o
	return api.HubBusqueda{}, nil
}

func (h *hubDoble) Search(ctx context.Context, q string, limite int) (api.HubBusqueda, error) {
	return h.SearchDesde(ctx, q, limite, 0)
}

// SearchDesde contesta con la página que toque de `paginas` y apunta cada desplazamiento.
func (h *hubDoble) SearchDesde(_ context.Context, _ string, _, desde int) (api.HubBusqueda, error) {
	h.mu.Lock()
	if c := h.colgada; c != nil && desde > 0 {
		h.colgada = nil
		h.desdes = append(h.desdes, desde)
		h.mu.Unlock()
		<-c
		return h.tardia, nil
	}
	defer h.mu.Unlock()
	h.desdes = append(h.desdes, desde)
	if i := desde / PorPaginaBusqueda; i < len(h.paginas) {
		return h.paginas[i], nil
	}
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

// TestHoyEsLoQueDaElBFF: el correo de la bandeja de entrada o, sin ella, la suma de
// todos los buzones. El corte de la agenda (seis, por hora de inicio) es de
// api.RecortarHoy y se prueba allí (TestLaAgendaDeHoyEsLaDelBFF).
func TestHoyEsLoQueDaElBFF(t *testing.T) {
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

// TestLaBusquedaCargaMas: con el Hub diciendo que quedan, sale «Cargar más resultados»;
// pulsarlo pide la página siguiente por `offset`, la pega detrás sin repetir y, cuando
// el Hub dice que no quedan, el botón se va.
//
// Visto en ROJO a mano el 2026-11-05: sin el `UnirBusqueda` (pegando la página tal
// cual), el resultado que se repite entre las dos sale dos veces y el test lo dice.
func TestLaBusquedaCargaMas(t *testing.T) {
	ayer := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	primera := hits(PorPaginaBusqueda, "mail", ayer)
	segunda := append([]api.HubHit{primera[len(primera)-1]}, hits(3, "drive", ayer)...)
	d := &hubDoble{paginas: []api.HubBusqueda{
		{Hits: primera, Total: 23, HasMore: true},
		{Hits: segunda, Total: 23, HasMore: false},
	}}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).Con(d)
	lee := func(f func(b *Buscar) bool) func() bool {
		return func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return f(v.buscar)
		}
	}
	boton := func() *ui.Boton {
		var out *ui.Boton
		fyne.DoAndWait(func() {
			for _, o := range test.WidgetRenderer(v.buscar).Objects() {
				if b, ok := o.(*ui.Boton); ok && b.Texto == v.buscar.T.T("search.loadMore", nil) {
					out = b
				}
			}
		})
		return out
	}
	v.buscar.AlBuscar("factura")
	esperar(t, "la primera página", lee(func(b *Buscar) bool { return b.Estado == Listo && len(b.Items) == PorPaginaBusqueda }))
	if !lee(func(b *Buscar) bool { return b.HayMas })() || boton() == nil {
		t.Fatal("con el Hub diciendo que quedan, no hay «Cargar más resultados»")
	}
	boton().Tapped(&fyne.PointEvent{})
	esperar(t, "la segunda página", lee(func(b *Buscar) bool { return len(b.Items) == 23 }))
	d.mu.Lock()
	desdes := slices.Clone(d.desdes)
	d.mu.Unlock()
	if !slices.Equal(desdes, []int{0, PorPaginaBusqueda}) {
		t.Errorf("al Hub le llegaron los desplazamientos %v", desdes)
	}
	if lee(func(b *Buscar) bool { return b.HayMas })() || boton() != nil {
		t.Error("sin más en el Hub, el botón sigue")
	}
	if lee(func(b *Buscar) bool { return b.Items[len(b.Items)-1].Source != "drive" })() {
		t.Error("la página nueva no va al final")
	}
}

// Visto en ROJO a mano el 2026-11-05: comparando el total con lo enseñado (como antes),
// el caso de los repetidos sigue ofreciendo más y el test lo dice.
func TestHayMasBusquedaTieneTope(t *testing.T) {
	if HayMasBusqueda(api.HubBusqueda{HasMore: true}, MaxPaginasBusqueda) {
		t.Error("pasado el tope de páginas sigue ofreciendo más")
	}
	if !HayMasBusqueda(api.HubBusqueda{Total: 40}, 1) {
		t.Error("con un total mayor que lo pedido no ofrece más")
	}
	// Dos páginas pedidas (40) con un repetido entre ellas: se ven 39 de 40 y el Hub dice
	// que no quedan. Con el total contra lo enseñado, el botón no se iba nunca.
	if HayMasBusqueda(api.HubBusqueda{Total: 2 * PorPaginaBusqueda}, 2) {
		t.Error("tras quitar un repetido sigue ofreciendo más sin que quede nada")
	}
}

// TestCargarMasBusquedaDeOtraBusqueda: un «Cargar más» que se queda colgado no se pega a
// la búsqueda siguiente cuando por fin llega, ni deja mudo el «Cargar más» de esta.
//
// Visto en ROJO a mano el 2026-11-05: sin soltar `cargandoMasBusqueda` al empezar la
// búsqueda nueva, su «Cargar más» no hace nada y el test lo dice; y sin el descarte por
// generación, la página tardía de la anterior acaba en la lista.
func TestCargarMasBusquedaDeOtraBusqueda(t *testing.T) {
	ayer := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	colgada := make(chan struct{})
	d := &hubDoble{
		paginas: []api.HubBusqueda{
			{Hits: hits(PorPaginaBusqueda, "mail", ayer), Total: 23, HasMore: true},
			{Hits: hits(3, "drive", ayer), Total: 23},
		},
		colgada: colgada,
		tardia:  api.HubBusqueda{Hits: hits(2, "rooms", ayer), Total: 23},
	}
	// Las de drive con ids propios: que no se confundan con las de mail.
	for i := range d.paginas[1].Hits {
		d.paginas[1].Hits[i].ID = fmt.Sprintf("drive-%d", i)
	}
	for i := range d.tardia.Hits {
		d.tardia.Hits[i].ID = fmt.Sprintf("rooms-%d", i)
	}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).Con(d)
	lee := func(f func(b *Buscar) bool) func() bool {
		return func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return f(v.buscar)
		}
	}
	v.buscar.AlBuscar("factura")
	esperar(t, "la primera búsqueda", lee(func(b *Buscar) bool { return b.Estado == Listo && b.HayMas }))
	acabada := make(chan struct{})
	go func() { v.cargarMasBusqueda(); close(acabada) }()
	esperar(t, "el «Cargar más» colgado", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.desdes) == 2
	})

	v.buscar.AlBuscar("otra")
	esperar(t, "la segunda búsqueda", lee(func(b *Buscar) bool {
		return b.Termino == "otra" && b.Estado == Listo && b.Paginas == 1 && b.HayMas
	}))
	v.cargarMasBusqueda()
	esperar(t, "su «Cargar más», con el otro aún colgado", lee(func(b *Buscar) bool { return len(b.Items) == 23 }))

	close(colgada)
	<-acabada
	fyne.DoAndWait(func() {})
	if lee(func(b *Buscar) bool {
		return slices.ContainsFunc(b.Items, func(h api.HubHit) bool { return h.Source == "rooms" }) || b.Paginas != 2
	})() {
		t.Error("la página de la búsqueda anterior se ha pegado a la de ahora")
	}
}

// buscarConMas monta la búsqueda en una ventana baja, con su desplazamiento, y con la
// primera página de veinte puesta y «Cargar más» a la vista.
func buscarConMas(t *testing.T, d *hubDoble) (*Vista, fyne.Canvas, *buscarRenderer) {
	t.Helper()
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).Con(d)
	ven := test.NewWindow(v.Desplazable())
	t.Cleanup(ven.Close)
	ven.Resize(fyne.NewSize(1280, 400))
	v.Ir(ABuscar)
	v.buscar.AlBuscar("factura")
	esperar(t, "la primera página", func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return v.buscar.Estado == Listo && len(v.buscar.Items) == PorPaginaBusqueda && v.buscar.HayMas
	})
	var r *buscarRenderer
	fyne.DoAndWait(func() { r = test.WidgetRenderer(v.buscar).(*buscarRenderer) })
	return v, ven.Canvas(), r
}

// pulsarMasConTeclado es Tab hasta «Cargar más» y Enter.
func pulsarMasConTeclado(t *testing.T, c fyne.Canvas, r *buscarRenderer) {
	t.Helper()
	fyne.DoAndWait(func() {
		if r.mas == nil {
			t.Fatal("no hay «Cargar más»")
		}
		c.Focus(r.mas)
		r.mas.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	})
}

// TestCargarMasConTecladoAterrizaEnElPrimeroNuevo: «Cargar más» pulsado con Enter rehace
// el botón y los grupos; el foco va al primer resultado NUEVO —el `#r-<n>` de la web— y
// queda a la vista, no en nada con el siguiente Tab empezando desde arriba.
//
// Visto en ROJO a mano el 2026-11-06: sin `devolverFoco`, el foco queda en nil; y sin
// `AlRevelarFoco`, la fila enfocada queda por debajo de la ventana.
func TestCargarMasConTecladoAterrizaEnElPrimeroNuevo(t *testing.T) {
	ayer := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	d := &hubDoble{paginas: []api.HubBusqueda{
		{Hits: hits(PorPaginaBusqueda, "mail", ayer), Total: 23, HasMore: true},
		{Hits: hits(3, "drive", ayer), Total: 23},
	}}
	v, c, r := buscarConMas(t, d)
	pulsarMasConTeclado(t, c, r)
	esperar(t, "el foco en el primero nuevo", func() bool {
		var ok bool
		fyne.DoAndWait(func() {
			ok = len(r.porRango) == 23 && c.Focused() == fyne.Focusable(r.porRango[PorPaginaBusqueda])
		})
		return ok
	})
	var y, alto float32
	fyne.DoAndWait(func() {
		f := r.porRango[PorPaginaBusqueda]
		y, alto = fyne.CurrentApp().Driver().AbsolutePositionForObject(f).Y, f.Size().Height
	})
	if y < 0 || y+alto > 400 {
		t.Errorf("el resultado enfocado está en y=%v (alto %v), fuera de una ventana de 400", y, alto)
	}
	v.Ir(AInicio)
}

// TestCargarMasSinNovedadLoDice: si la página siguiente llega vacía aunque el Hub dijera
// que quedan (o falla), la pantalla lo dice junto al botón —el aviso de la web— y el foco
// pasa al botón nuevo.
//
// Visto en ROJO a mano el 2026-11-06: sin `SinNovedad` no sale el aviso; y sin el
// respaldo al botón en `devolverFoco`, el foco queda en nil.
func TestCargarMasSinNovedadLoDice(t *testing.T) {
	ayer := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	d := &hubDoble{paginas: []api.HubBusqueda{
		{Hits: hits(PorPaginaBusqueda, "mail", ayer), Total: 40, HasMore: true},
		{Total: 40, HasMore: true},
	}}
	v, c, r := buscarConMas(t, d)
	aviso := v.buscar.T.T("search.loadMoreFailed", nil)
	if aviso == "search.loadMoreFailed" {
		t.Fatal("el aviso no está en el catálogo")
	}
	hayAviso := func() bool {
		var ok bool
		fyne.DoAndWait(func() {
			for _, o := range r.Objects() {
				if p, es := o.(*ui.Parrafo); es && p.Texto == aviso {
					ok = true
				}
			}
		})
		return ok
	}
	if hayAviso() {
		t.Fatal("el aviso sale antes de pedir nada")
	}
	pulsarMasConTeclado(t, c, r)
	esperar(t, "el aviso", hayAviso)
	esperar(t, "el foco en el botón nuevo", func() bool {
		var ok bool
		fyne.DoAndWait(func() { ok = r.mas != nil && c.Focused() == fyne.Focusable(r.mas) })
		return ok
	})
	v.Ir(AInicio)
}

// Visto en ROJO a mano el 2026-11-06: devolviendo siempre `total`, el «39 de 40» sigue
// sin botón para traer el 40, que no existe.
func TestTotalAMostrarSinMasEsLoEnsenado(t *testing.T) {
	if n := TotalAMostrar(api.HubBusqueda{Total: 40}, 2, 40, 39); n != 39 {
		t.Errorf("sin más en el Hub dice %d de total y se ven 39", n)
	}
	if n := TotalAMostrar(api.HubBusqueda{Total: 143, HasMore: true}, 1, 143, 20); n != 143 {
		t.Errorf("con más en el Hub dice %d de total y son 143", n)
	}
}
