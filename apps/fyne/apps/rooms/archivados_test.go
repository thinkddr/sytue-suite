// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestReglasDeArchivados es el gemelo de lib/archivados.test.ts.
func TestReglasDeArchivados(t *testing.T) {
	cs := []api.RoomsChannel{
		{ID: "zeta", Name: "zeta", Kind: "public", Archived: true},
		{ID: "activo", Name: "activo", Kind: "public"},
		{ID: "dm", Name: "dm", Kind: "direct", Archived: true},
		{ID: "alfa", Name: "Alfa", Kind: "private", Archived: true},
	}
	got := SoloArchivados(i18n.Locale("es"), cs)
	if len(got) != 2 || got[0].ID != "alfa" || got[1].ID != "zeta" {
		t.Errorf("solo archivados, sin directos y por nombre: %+v", got)
	}
	// La colación del idioma, como `localeCompare`: la tilde y la eñe no mandan al final.
	acentos := []api.RoomsChannel{
		{ID: "z", Name: "zeta", Archived: true},
		{ID: "ñ", Name: "Ñandú", Archived: true},
		{ID: "n", Name: "nube", Archived: true},
		{ID: "á", Name: "área", Archived: true},
		{ID: "a", Name: "Alfa", Archived: true},
	}
	orden := make([]string, 0, len(acentos))
	for _, c := range SoloArchivados(i18n.Locale("es"), acentos) {
		orden = append(orden, c.ID)
	}
	if !slices.Equal(orden, []string{"a", "á", "n", "ñ", "z"}) {
		t.Errorf("por nombre con la colación del español: %v", orden)
	}
	for ids, quedan := range map[[2]string]string{
		{"abc", "a"}: "b", {"abc", "b"}: "c", {"abc", "c"}: "b", {"a", "a"}: "", {"abc", "x"}: "",
	} {
		if got := FilaTrasQuitar(strings.Split(ids[0], ""), ids[1]); got != quedan {
			t.Errorf("quitar %q de %q deja el foco en %q, no en %q", ids[1], ids[0], got, quedan)
		}
	}
	for estado, clave := range map[int]string{403: "archived.forbidden", 409: "archived.nameTaken", 400: "archived.invalidName", 502: "archived.failed"} {
		if c := ClaveErrorArchivado(&api.Error{Status: estado}); c != clave {
			t.Errorf("un %d avisa %q y debía %q", estado, c, clave)
		}
	}
	if c := ClaveErrorArchivado(errors.New("red")); c != "archived.failed" {
		t.Errorf("un fallo de red avisa %q", c)
	}
	if NombreNuevo("ventas", "  ventas-2026 ") != "ventas-2026" || NombreNuevo("ventas", " ventas ") != "" || NombreNuevo("ventas", "  ") != "" {
		t.Error("NombreNuevo no es el de la web")
	}
}

// TestArchivadosRenombrarYDesarchivar: la pestaña se pide al elegirla; renombrar con
// Intro manda el nombre y lo pinta; un 403 avisa sin tocar la lista; y desarchivar
// devuelve el canal a la barra y lo quita de la pestaña. En cada paso el foco queda en
// un control VIVO de la pantalla (docs/design.md §2.6): «Renombrar» tras Intro o
// Escape, la fila siguiente tras desarchivar y la pestaña si la lista se vacía; y por
// eso un segundo Escape cierra el diálogo en vez de perderse en un campo que ya no está.
func TestArchivadosRenombrarYDesarchivar(t *testing.T) {
	s := servicioConDos()
	s.archivados = []api.RoomsChannel{
		{ID: "a1", Name: "viejo", Kind: "public", Archived: true, Purpose: "Lo de 2025"},
		{ID: "a2", Name: "zeta", Kind: "public", Archived: true},
		{ID: "c1", Name: "general", Kind: "public"},
	}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.AbrirExplorar()
	a := sa.archivados
	if s.pidio("ArchivedChannels(t1)") {
		t.Fatal("los archivados se pidieron sin elegir la pestaña")
	}
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	pest := hallar[*ui.Pestanas](lienzo.Overlays().Top())
	if pest == nil {
		t.Fatal("«Explorar canales» no tiene pestañas")
	}
	pest.Elegir(1)
	if !s.pidio("ArchivedChannels(t1)") || a.Estado != Listo || len(a.Canales) != 2 || a.Canales[0].ID != "a1" {
		t.Fatalf("la pestaña quedó en %v con %+v", a.Estado, a.Canales)
	}

	// Renombrar: el botón abre el campo con el foco; Intro guarda.
	test.Tap(a.Filas()[0].Botones()[0])
	f := a.Filas()[0]
	if !f.Renombrando || a.Renombrando != "a1" {
		t.Fatal("«Renombrar» no abrió el campo")
	}
	if lienzo.Focused() != f.Campo() {
		t.Error("el campo de renombrar no tiene el foco")
	}
	f.Campo().PonerTexto("archivo-2025")
	f.Campo().TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !s.pidio("UpdateChannel(a1,nombre=archivo-2025)") || a.Canales[0].Nombre != "archivo-2025" || a.Renombrando != "" {
		t.Fatalf("renombrar: %v, lista %+v", s.llamadas, a.Canales)
	}
	if lienzo.Focused() != a.Filas()[0].Botones()[0] {
		t.Fatalf("tras Intro el foco está en %T y no en «Renombrar» de la fila", lienzo.Focused())
	}

	// Escape cierra el campo y deja el foco en «Renombrar»; un segundo Escape ya no se
	// lo queda nadie de la fila: es del diálogo.
	test.Tap(a.Filas()[0].Botones()[0])
	lienzo.Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if a.Renombrando != "" || lienzo.Focused() != a.Filas()[0].Botones()[0] {
		t.Fatalf("tras Escape: renombrando %q, foco en %T", a.Renombrando, lienzo.Focused())
	}
	// Y «Cancelar» igual.
	test.Tap(a.Filas()[0].Botones()[0])
	test.Tap(a.Filas()[0].Botones()[1])
	if a.Renombrando != "" || lienzo.Focused() != a.Filas()[0].Botones()[0] {
		t.Fatalf("tras «Cancelar»: renombrando %q, foco en %T", a.Renombrando, lienzo.Focused())
	}

	// Sin permiso: aviso, la lista igual y el foco de vuelta en su «Desarchivar».
	s.errAccion = &api.Error{Status: 403}
	a.AlDesarchivar(a.Canales[0])
	if a.Aviso != sa.P.T.T("archived.forbidden", nil) || len(a.Canales) != 2 || len(a.Ocupado) != 0 {
		t.Fatalf("un 403 deja aviso %q, %d canales, ocupados %v", a.Aviso, len(a.Canales), a.Ocupado)
	}
	if lienzo.Focused() != a.Filas()[0].Botones()[1] {
		t.Fatalf("tras el 403 el foco está en %T y no en «Desarchivar»", lienzo.Focused())
	}

	// Desarchivar la primera deja el foco en la que ocupa su sitio.
	s.errAccion = nil
	antes := len(sa.P.Lateral.Canales)
	test.Tap(a.Filas()[0].Botones()[1])
	if !s.pidio("UpdateChannel(a1,desarchivado)") || len(a.Canales) != 1 || len(sa.P.Lateral.Canales) != antes+1 {
		t.Errorf("desarchivar: %v, %d en la pestaña, %d en la barra (antes %d)", s.llamadas, len(a.Canales), len(sa.P.Lateral.Canales), antes)
	}
	if f := a.Filas(); len(f) != 1 || f[0].Canal.ID != "a2" || lienzo.Focused() != f[0].Botones()[1] {
		t.Fatalf("tras desarchivar el foco está en %T y no en «Desarchivar» de la siguiente", lienzo.Focused())
	}

	// La última: la lista se vacía y el foco va a la pestaña, desde la que Escape
	// cierra el diálogo.
	test.Tap(a.Filas()[0].Botones()[1])
	if len(a.Canales) != 0 || lienzo.Focused() != fyne.Focusable(pest.PestanaActiva()) {
		t.Fatalf("con la lista vacía el foco está en %T y no en la pestaña", lienzo.Focused())
	}
	capa := lienzo.Overlays().Top()
	lienzo.Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if lienzo.Overlays().Top() == capa {
		t.Error("Escape desde la pestaña no cerró el diálogo")
	}
}

// TestArchivadosOcupadoPorFila: cada fila está ocupada por su cuenta. Con dos
// desarchivados en curso, la respuesta del primero no reactiva el botón del segundo.
func TestArchivadosOcupadoPorFila(t *testing.T) {
	s := servicioConDos()
	s.archivados = []api.RoomsChannel{
		{ID: "a1", Name: "alfa", Kind: "public", Archived: true},
		{ID: "a2", Name: "beta", Kind: "public", Archived: true},
	}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.AbrirExplorar()
	a := sa.archivados
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	hallar[*ui.Pestanas](lienzo.Overlays().Top()).Elegir(1)

	var pendientes []func()
	sa.Lanzar = func(f func()) { pendientes = append(pendientes, f) }
	test.Tap(a.Filas()[0].Botones()[1])
	test.Tap(a.Filas()[1].Botones()[1])
	if len(pendientes) != 2 {
		t.Fatalf("se lanzaron %d peticiones, no 2", len(pendientes))
	}
	pendientes[0]()
	fyne.DoAndWait(func() {})
	f := a.Filas()
	if len(f) != 1 || f[0].Canal.ID != "a2" || !f[0].Botones()[1].Cargando || !f[0].Botones()[0].Inerte {
		t.Fatalf("terminar «alfa» liberó «beta» con su petición en curso: %d filas, ocupados %v", len(f), a.Ocupado)
	}
	pendientes[1]()
	fyne.DoAndWait(func() {})
	if len(a.Canales) != 0 || len(a.Ocupado) != 0 {
		t.Errorf("al terminar las dos: %d canales, ocupados %v", len(a.Canales), a.Ocupado)
	}
}

// Lo escrito en el campo de una fila sobrevive a que termine la petición de otra: cada
// pintado rehace las filas, y el campo volvía al nombre original a media escritura. Y
// el cursor se queda donde estaba: rehecho el campo, saltaba al final.
func TestArchivadosBorradorSobreviveAOtraFila(t *testing.T) {
	s := servicioConDos()
	s.archivados = []api.RoomsChannel{
		{ID: "a1", Name: "alfa", Kind: "public", Archived: true},
		{ID: "a2", Name: "beta", Kind: "public", Archived: true},
	}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.AbrirExplorar()
	a := sa.archivados
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	hallar[*ui.Pestanas](lienzo.Overlays().Top()).Elegir(1)

	var pendientes []func()
	sa.Lanzar = func(f func()) { pendientes = append(pendientes, f) }
	test.Tap(a.Filas()[0].Botones()[1]) // «Desarchivar» alfa, en curso
	test.Tap(a.Filas()[1].Botones()[0]) // «Renombrar» beta
	fyne.DoAndWait(func() {})
	test.Type(a.Filas()[1].Campo(), "-nuevo")
	// El cursor, a mitad del nombre: lo siguiente que se teclee tiene que caer ahí
	// aunque entretanto se repinte la lista.
	for range len("-nuevo") {
		a.Filas()[1].Campo().TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	}
	if len(pendientes) != 1 {
		t.Fatalf("se lanzaron %d peticiones, no 1", len(pendientes))
	}
	pendientes[0]()
	fyne.DoAndWait(func() {})
	f := a.Filas()
	if len(f) != 1 || !f[0].Renombrando {
		t.Fatalf("tras desarchivar alfa: %d filas, renombrando %q", len(f), a.Renombrando)
	}
	if got := f[0].Campo().Texto(); got != "beta-nuevo" {
		t.Errorf("el campo de beta quedó en %q, no en lo escrito", got)
	}
	if lienzo.Focused() != fyne.Focusable(f[0].Campo()) {
		t.Errorf("tras el repintado el foco está en %T, no en el campo", lienzo.Focused())
	}
	test.Type(f[0].Campo(), "!")
	if got := f[0].Campo().Texto(); got != "beta!-nuevo" {
		t.Errorf("tras el repintado lo tecleado cae en %q: el cursor saltó", got)
	}
	// «Cancelar» cierra y olvida el borrador: al reabrir, vuelve el nombre de ahora.
	test.Tap(f[0].Botones()[1])
	fyne.DoAndWait(func() {})
	test.Tap(a.Filas()[0].Botones()[0])
	fyne.DoAndWait(func() {})
	if got := a.Filas()[0].Campo().Texto(); got != "beta" {
		t.Errorf("al reabrir, el campo trae %q y no el nombre", got)
	}
}

// hallar es el primer objeto de tipo T colgando de `o`, entrando en contenedores y en
// los renderizadores de los widgets.
func hallar[T fyne.CanvasObject](o fyne.CanvasObject) T {
	var cero T
	if o == nil {
		return cero
	}
	if x, ok := o.(T); ok {
		return x
	}
	var hijos []fyne.CanvasObject
	switch c := o.(type) {
	case *fyne.Container:
		hijos = c.Objects
	case fyne.Widget:
		hijos = test.WidgetRenderer(c).Objects()
	}
	for _, h := range hijos {
		if x := hallar[T](h); any(x) != any(cero) {
			return x
		}
	}
	return cero
}
