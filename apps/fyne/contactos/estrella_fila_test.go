// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// centroBotonEstrella es el centro del botón de la estrella de la fila: pegado al
// margen derecho, `LadoBotonEstrellaFila` de ancho.
func centroBotonEstrella() (int, int) {
	return int(float32(anchoPanel-1) - ui.MargenFila - ui.SueloTactil(LadoBotonEstrellaFila)/2), altoFila / 2
}

// TestLaEstrellaDeLaFilaDestacaSinAbrir: con red, tocar la estrella del final destaca y
// no abre la ficha; el resto de la fila sigue abriendo.
func TestLaEstrellaDeLaFilaDestacaSinAbrir(t *testing.T) {
	f := NuevaFilaContacto()
	var abiertas, destacadas int
	f.AlAbrir, f.AlDestacar = func() { abiertas++ }, func() { destacadas++ }
	f.Resize(fyne.NewSize(anchoPanel-1, altoFila))
	x, y := centroBotonEstrella()
	f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(float32(x), float32(y))})
	if destacadas != 1 || abiertas != 0 {
		t.Errorf("en la estrella: %d destacadas y %d abiertas", destacadas, abiertas)
	}
	f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(150, 24)})
	if abiertas != 1 || destacadas != 1 {
		t.Errorf("en el nombre: %d abiertas y %d destacadas", abiertas, destacadas)
	}
}

// TestLaEstrellaVaciaSaleAlPasarYConElDedo: sin destacar, la estrella en trazo no se ve
// en reposo, sí al pasar el puntero por la fila y siempre en un teléfono.
func TestLaEstrellaVaciaSaleAlPasarYConElDedo(t *testing.T) {
	gris := tema.Color("text-muted", tema.Claro)
	tamano := fyne.NewSize(anchoPanel-1, altoFila)
	nueva := func() *FilaContacto {
		f := NuevaFilaContacto()
		f.Nombre, f.AlDestacar = "Ana", func() {}
		return f
	}
	x, y := centroBotonEstrella()

	if hayColorCerca(pintarObjeto(t, nueva(), tamano), x, y, gris) {
		t.Error("en reposo ya se ve la estrella vacía")
	}
	f := nueva()
	img := func() bool { return hayColorCerca(pintarObjeto(t, f, tamano), x, y, gris) }
	f.MouseIn(&desktop.MouseEvent{})
	if !img() {
		t.Error("al pasar el puntero la estrella vacía no sale")
	}

	restaurar := ui.TactilDePrueba(true)
	defer restaurar()
	x, y = centroBotonEstrella()
	if !hayColorCerca(pintarObjeto(t, nueva(), tamano), x, y, gris) {
		t.Error("en un teléfono la estrella vacía no se ve, y no hay puntero que pase")
	}
}

// TestLaFilaSoloOfreceDestacarConRed: la lista conecta la estrella con `AlDestacar` y
// con el contacto de ESA fila; sin red no hay botón.
func TestLaFilaSoloOfreceDestacarConRed(t *testing.T) {
	p := &Pantalla{T: traductor(t)}
	var visto string
	p.AlDestacar = func(c api.Contact) { visto = c.ID }
	f := NuevaFilaContacto()
	p.rellenarFila(f, persona("ana", "Ana"))
	if f.AlDestacar == nil {
		t.Fatal("con red la fila no ofrece destacar")
	}
	f.AlDestacar()
	if visto != "ana" {
		t.Errorf("se destacó %q", visto)
	}
	p.SinConexion = true
	p.rellenarFila(f, persona("ana", "Ana"))
	if f.AlDestacar != nil {
		t.Error("sin red la fila sigue ofreciendo destacar")
	}
}

// TestUnDobleToqueEnLaEstrellaNoCruzaPeticiones: mientras va la petición de un contacto,
// otro toque en su estrella no hace nada (el `busy` de la web); otro contacto sí puede, y
// al volver la petición, la estrella vuelve a responder.
func TestUnDobleToqueEnLaEstrellaNoCruzaPeticiones(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana"), persona("luis", "Luis")}}
	a := appDePrueba(t, s)
	a.Cargar()
	var enVuelo []func()
	a.enFondo = func(f func()) { enVuelo = append(enVuelo, f) }
	ana, luis := a.Pantalla.Contactos[0], a.Pantalla.Contactos[1]
	a.destacar(ana)
	a.destacar(ana)
	a.destacar(luis)
	if len(enVuelo) != 2 {
		t.Fatalf("con un doble toque salen %d peticiones, y debían ser 2 (una por contacto)", len(enVuelo))
	}
	if !a.favoritos["u-ana"] {
		t.Error("el segundo toque deshizo en pantalla el primero")
	}
	for _, f := range enVuelo {
		f()
	}
	fyne.DoAndWait(func() {})
	enVuelo = nil
	a.destacar(ana)
	if len(enVuelo) != 1 || a.favoritos["u-ana"] {
		t.Errorf("tras volver la petición la estrella no responde (%d en vuelo)", len(enVuelo))
	}
}

// estrellaDeLaFila busca, entre lo que pinta la fila, el control enfocable de la
// estrella: el que el gestor de foco de Fyne recorre con el tabulador.
func estrellaDeLaFila(t *testing.T, f *FilaContacto) *estrellaFila {
	t.Helper()
	for _, o := range test.TempWidgetRenderer(t, f).Objects() {
		if e, es := o.(*estrellaFila); es && o.Visible() {
			return e
		}
	}
	return nil
}

// TestLaEstrellaDeLaFilaSeUsaConElTeclado: con red, la estrella es un control propio
// —enfocable, con su nombre («Destacar a Ana») y que se pulsa con Intro y con Espacio—,
// como el `IconButton` de la web; sin red no hay parada en el tabulador.
func TestLaEstrellaDeLaFilaSeUsaConElTeclado(t *testing.T) {
	p := &Pantalla{T: traductor(t)}
	var destacadas int
	p.AlDestacar = func(api.Contact) { destacadas++ }
	f := NuevaFilaContacto()
	p.rellenarFila(f, persona("ana", "Ana"))
	f.Resize(fyne.NewSize(anchoPanel-1, altoFila))

	e := estrellaDeLaFila(t, f)
	if e == nil {
		t.Fatal("la estrella de la fila no es un control enfocable")
	}
	var _ fyne.Focusable = e
	if got, want := e.AccessibilityLabel(), p.t("list.star", i18n.Vars{"name": "Ana"}); got != want {
		t.Errorf("la estrella se anuncia como %q, y debía ser %q", got, want)
	}
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	e.TypedRune(' ')
	if destacadas != 2 {
		t.Errorf("con Intro y Espacio se destacó %d veces, y debían ser 2", destacadas)
	}

	p.Destacados = map[string]bool{api.CleanUID(persona("ana", "Ana").UID): true}
	p.rellenarFila(f, persona("ana", "Ana"))
	if got, want := estrellaDeLaFila(t, f).AccessibilityLabel(), p.t("list.unstar", i18n.Vars{"name": "Ana"}); got != want {
		t.Errorf("destacado, la estrella se anuncia como %q, y debía ser %q", got, want)
	}

	p.SinConexion = true
	p.rellenarFila(f, persona("ana", "Ana"))
	if estrellaDeLaFila(t, f) != nil {
		t.Error("sin red la estrella sigue siendo una parada del tabulador")
	}
}

// TestLaEstrellaVaciaSaleConElFoco: sin destacar y sin puntero encima, la estrella no se
// ve; con el foco del teclado, sí (`focus-visible:opacity-100`), o el foco estaría en
// un sitio invisible.
func TestLaEstrellaVaciaSaleConElFoco(t *testing.T) {
	gris := tema.Color("text-muted", tema.Claro)
	tamano := fyne.NewSize(anchoPanel-1, altoFila)
	f := NuevaFilaContacto()
	f.Nombre, f.AlDestacar = "Ana", func() {}
	x, y := centroBotonEstrella()
	img := pintarObjeto(t, f, tamano)
	if hayColorCerca(img, x, y, gris) {
		t.Fatal("en reposo ya se ve la estrella vacía")
	}
	estrellaDeLaFila(t, f).FocusGained()
	if !hayColorCerca(pintarObjeto(t, f, tamano), x, y, gris) {
		t.Error("con el foco la estrella vacía no se ve")
	}
}

// TestLaFilaSigueResaltadaConElPunteroEnLaEstrella: Fyne da el hover solo al control
// más hondo, así que al llegar a la estrella la fila recibe MouseOut. En la web el
// `hover:bg-bg-subtle` es de la fila y sigue mientras se está sobre la estrella: el
// velo no se apaga al pasar de la una a la otra, y sí al salir de las dos.
func TestLaFilaSigueResaltadaConElPunteroEnLaEstrella(t *testing.T) {
	velo := tema.Color("bg-subtle", tema.Claro)
	tamano := fyne.NewSize(anchoPanel-1, altoFila)
	f := NuevaFilaContacto()
	f.Nombre, f.AlDestacar = "Ana", func() {}
	// Un punto del velo lejos del texto y de la estrella: a la izquierda del botón.
	x, y := centroBotonEstrella()
	x -= 40
	f.MouseIn(&desktop.MouseEvent{})
	if !hayColorCerca(pintarObjeto(t, f, tamano), x, y, velo) {
		t.Fatal("con el puntero en la fila no sale el velo")
	}
	f.MouseOut()
	estrellaDeLaFila(t, f).MouseIn(&desktop.MouseEvent{})
	if !hayColorCerca(pintarObjeto(t, f, tamano), x, y, velo) {
		t.Error("con el puntero en la estrella la fila deja de estar resaltada")
	}
	estrellaDeLaFila(t, f).MouseOut()
	if hayColorCerca(pintarObjeto(t, f, tamano), x, y, velo) {
		t.Error("fuera de la fila y de la estrella el velo sigue")
	}
}
