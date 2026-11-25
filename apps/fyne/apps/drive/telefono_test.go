// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Lo que la auditoría estética del 2026-11-24 vio en Drive en el teléfono.

// exploradorEnElTelefono monta el explorador con cuatro carpetas y tres ficheros, en
// Marco, con la barra de la suite y el dedo, a 390 × 844.
func exploradorEnElTelefono(t *testing.T) (*Explorador, fyne.Window) {
	t.Helper()
	t.Cleanup(ui.TactilDePrueba(true))
	t.Cleanup(tema.FormaDeLosGuardianes)
	tema.ElegirForma("marco")
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Exito, 390
	for i, n := range []string{"Actas", "Contratos firmados", "Diseño", "Fotografías del viaje"} {
		e.Elementos = append(e.Elementos, Elemento{ID: fmt.Sprintf("c%d", i), Nombre: n, Carpeta: true, Tamano: -1})
	}
	for i := range 3 {
		e.Elementos = append(e.Elementos, Elemento{ID: fmt.Sprintf("f%d", i), Nombre: fmt.Sprintf("acta-%d.md", i), Tamano: 10})
	}
	arm := ui.NuevoArmazon(e.Objeto(tema.Claro))
	barra, parar := ui.BarraSuiteDePrueba(e.tr, iconos.Drive)
	t.Cleanup(parar)
	w := test.NewTempWindow(t, barra.Montar(arm))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 844))
	return e, w
}

// TestLaFranjaDeCarpetasNoPartaUnaTarjeta: en Marco la franja de carpetas mide hasta el
// 45 % del cuerpo, y a 390 × 844 eso dejaba la tercera tarjeta partida bajo la cabecera
// «NOMBRE», con su nombre tapado. Ahora la franja acaba entre dos filas: cada tarjeta se
// ve entera o no se ve, y las demás se alcanzan desplazando la franja.
//
// VISTO EN ROJO con el 45 % a secas: «Diseño» enseña 71 de sus 102 px.
func TestLaFranjaDeCarpetasNoPartaUnaTarjeta(t *testing.T) {
	e, _ := exploradorEnElTelefono(t)
	franja := e.franjaScroll
	if franja == nil || len(e.tarjetas) != 4 {
		t.Fatalf("sin franja de carpetas: %v, %d tarjetas", franja, len(e.tarjetas))
	}
	d := fyne.CurrentApp().Driver()
	arriba := d.AbsolutePositionForObject(franja).Y
	abajo := arriba + franja.Size().Height
	enteras := 0
	for _, tj := range e.tarjetas {
		y := d.AbsolutePositionForObject(tj.objeto).Y
		fin := y + tj.objeto.Size().Height
		switch {
		case y >= arriba-0.5 && fin <= abajo+0.5:
			enteras++
		case y < abajo-0.5:
			t.Errorf("«%s» se ve partida: de %v a %v en una franja que acaba en %v", tj.el.Nombre, y, fin, abajo)
		}
	}
	if enteras == 0 || enteras == len(e.tarjetas) {
		t.Errorf("se ven enteras %d de %d tarjetas: el fixture no recorta la franja", enteras, len(e.tarjetas))
	}
	if franja.Size().Height > TopeFranja*e.listaFranja.Size().Height+0.5 {
		t.Errorf("la franja mide %v y pasa del 45 %% del cuerpo", franja.Size().Height)
	}
}

// TestLaBarraDeSeleccionCabeEnElTelefono: con dos marcados, «Mover», «Descargar» y
// «Papelera» con su rótulo no caben entre el número y el aspa a 390, y la tira dejaba
// «Papeler» contra el borde. En el teléfono, si no caben, van solo con su glifo (con el
// rótulo como nombre accesible); con ratón, o si caben, con su rótulo.
//
// VISTO EN ROJO sin el paso a solo glifo: las tres siguen con su rótulo.
func TestLaBarraDeSeleccionCabeEnElTelefono(t *testing.T) {
	e, _ := exploradorEnElTelefono(t)
	e.Seleccion["f0"], e.Seleccion["c1"] = true, true
	b, ok := e.barraSeleccion(tema.Claro, 2).(*ui.BarraDeLote)
	if !ok {
		t.Fatal("la barra de selección no es una ui.BarraDeLote")
	}
	ventana := test.NewTempWindow(t, b)
	ventana.SetPadded(false)
	ventana.Resize(fyne.NewSize(390, ui.MedidaBarraDeLoteEn(b, 390).Height))
	for _, a := range b.Acciones {
		if !a.SoloIcono || a.Rotulo != a.Texto || a.Texto == "" {
			t.Errorf("«%s» en el teléfono: solo glifo %v, nombre %q", a.Texto, a.SoloIcono, a.Rotulo)
		}
		if fin := fyne.CurrentApp().Driver().AbsolutePositionForObject(a).X + a.Size().Width; fin > 390 {
			t.Errorf("«%s» acaba en x = %v, fuera de la barra", a.Texto, fin)
		}
	}

	// Con ratón la barra es la de siempre, con sus rótulos.
	restaurar := ui.TactilDePrueba(false)
	defer restaurar()
	b, _ = e.barraSeleccion(tema.Claro, 2).(*ui.BarraDeLote)
	for _, a := range b.Acciones {
		if a.SoloIcono {
			t.Errorf("con ratón «%s» pierde su rótulo", a.Texto)
		}
	}
}

// TestLaFranjaDeCarpetasDejaSitioAlSubirFlotante: con la franja de carpetas en el
// teléfono la lista va dentro de otro desplazamiento y no reservaba el hueco del «Subir»
// flotante, que tapaba su última fila (auditoría del 2026-11-24). Si la franja cabe, el
// final de la lista es el de la pantalla y la lista lleva su fila de reserva, como sin
// franja; en una ventana baja (franja compacta) la reserva va al final del desplazamiento
// de fuera y la lista de dos filas no la lleva. VISTO EN ROJO con la reserva apagada.
func TestLaFranjaDeCarpetasDejaSitioAlSubirFlotante(t *testing.T) {
	e, w := exploradorEnElTelefono(t)
	if e.listaFranja == nil {
		t.Fatal("sin el desplazamiento de la franja: el fixture no la monta")
	}
	if !e.reservaFilas.Activa() {
		t.Error("con la franja a 390 × 844 la lista no reserva el hueco del «Subir» flotante")
	}
	contenido, ok := e.listaFranja.Content.(*fyne.Container)
	if !ok {
		t.Fatal("el desplazamiento de la franja no lleva su columna")
	}
	col, ok := contenido.Layout.(*columnaLista)
	if !ok {
		t.Fatal("la columna de la franja no es una columnaLista")
	}
	// Con la franja normal, la lista acaba a `PieLista` del final del cuerpo: la reserva
	// de su última fila es la que la separa del botón.
	if fin, cuerpo := e.filasObj.Position().Y+e.filasObj.Size().Height, contenido.Size().Height; fin < cuerpo-PieLista-1 {
		t.Errorf("la lista acaba en %v y el cuerpo en %v: sobra hueco además de la reserva", fin, cuerpo)
	}

	// Una ventana baja: la franja compacta pone la reserva al final del desplazamiento.
	w.Resize(fyne.NewSize(390, 380))
	if !col.compacta {
		t.Fatalf("a 390 × 380 la franja no es compacta (cuerpo de %v)", e.listaFranja.Size().Height)
	}
	if e.reservaFilas.Activa() {
		t.Error("con la franja compacta la lista de dos filas aún lleva su fila de reserva")
	}
	e.listaFranja.ScrollToBottom()
	fin := e.filasObj.Position().Y + e.filasObj.Size().Height - e.listaFranja.Offset.Y
	boton := e.listaFranja.Size().Height - ui.LadoBotonFlotante - ui.MargenBotonFlotante
	if fin > boton+0.5 {
		t.Errorf("abajo del todo la lista acaba en %v y el «Subir» empieza en %v", fin, boton)
	}
	if inicio := fin - e.filasObj.Size().Height; inicio < -0.5 {
		t.Errorf("abajo del todo la lista empieza en %v, fuera del desplazamiento", inicio)
	}
}
