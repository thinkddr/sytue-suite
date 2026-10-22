// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// filasCumple busca las filas de cumpleaños de un árbol, en orden.
func filasCumple(o fyne.CanvasObject) []*filaCumple {
	var fuera []*filaCumple
	switch x := o.(type) {
	case *filaCumple:
		return []*filaCumple{x}
	case *fyne.Container:
		for _, h := range x.Objects {
			fuera = append(fuera, filasCumple(h)...)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			fuera = append(fuera, filasCumple(h)...)
		}
	}
	return fuera
}

// TestLosCumpleanosSeListanYAbrenLaFicha: del más cercano al más lejano, «Hoy» en
// negrita y con la edad si se sabe el año; pulsar uno cierra y abre su ficha.
func TestLosCumpleanosSeListanYAbrenLaFicha(t *testing.T) {
	hoy := time.Now()
	ana, bruno := persona("ana", "Ana"), persona("bruno", "Bruno")
	enTres := hoy.AddDate(-30, 0, 3)
	cumpleHoy := time.Date(1990, hoy.Month(), hoy.Day(), 0, 0, 0, 0, time.UTC)
	ana.Birthday, bruno.Birthday = &enTres, &cumpleHoy
	a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{ana, bruno, persona("carla", "Carla")}})
	a.Cargar()
	d := a.verCumples()
	filas := filasCumple(d.Cuerpo)
	if len(filas) != 2 {
		t.Fatalf("hay %d filas y son dos cumpleaños", len(filas))
	}
	if filas[0].nombre != "Bruno" || filas[0].cuando != "Hoy" || !filas[0].hoy {
		t.Errorf("la primera es %q, «%s»", filas[0].nombre, filas[0].cuando)
	}
	fecha := i18n.FormatDayMonthLong("es", time.Date(hoy.Year(), enTres.Month(), enTres.Day(), 0, 0, 0, 0, time.Local))
	if filas[1].cuando != "En 3 días" || filas[1].fecha != fecha+" · Cumple 30" {
		t.Errorf("la segunda dice «%s» y «%s»", filas[1].cuando, filas[1].fecha)
	}
	filas[1].Tapped(nil)
	if abierto(a) || a.Pantalla.Seleccionado == nil || a.Pantalla.Seleccionado.ID != "ana" {
		t.Errorf("al pulsar a Ana: diálogo abierto %v, ficha %v", abierto(a), a.Pantalla.Seleccionado)
	}
}

// TestSinCumpleanosLoDice: con ninguno guardado, la frase de la web y ninguna lista.
func TestSinCumpleanosLoDice(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}})
	a.Cargar()
	d := a.verCumples()
	if r, es := d.Cuerpo.(*ui.Rotulo); !es || r.Texto != "Ningún contacto tiene guardado su cumpleaños." {
		t.Errorf("el cuerpo es %#v", d.Cuerpo)
	}
}

// TestLaPapeleraRestauraYEliminaPreguntandoEnLaFila: restaurar va directo; eliminar
// pregunta en la misma fila («¿Eliminar para siempre?») antes de borrar de verdad.
func TestLaPapeleraRestauraYEliminaPreguntandoEnLaFila(t *testing.T) {
	s := &servicioFalso{
		contactos: []api.Contact{persona("ana", "Ana")},
		papelera: []api.TrashItem{
			{ID: "t1", FormattedName: "Carla Benet", PrimaryEmail: "carla@ejemplo.test"},
			{ID: "t2", FormattedName: "Héctor Lamas"},
		},
	}
	a := appDePrueba(t, s)
	a.Cargar()
	x := a.verPapelera()
	textos := textosDe(x.cuerpo)
	for _, quiero := range []string{"Contactos borrados. Restáuralos o elimínalos definitivamente.", "Carla Benet", "carla@ejemplo.test", "—"} {
		if !contieneTexto(textos, quiero) {
			t.Errorf("la papelera no dice %q: %v", quiero, textos)
		}
	}
	boton := func(texto string) *ui.Boton {
		for _, b := range botonesDe(x.cuerpo) {
			if b.Texto == texto {
				return b
			}
		}
		t.Fatalf("no hay botón «%s» en %v", texto, textosDe(x.cuerpo))
		return nil
	}
	boton("Restaurar").AlPulsar()
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != "POST /api/v1/contacts/t1/restore" {
		t.Fatalf("escrituras %v", s.escrituras)
	}
	if contieneTexto(textosDe(x.cuerpo), "Carla Benet") {
		t.Error("restaurada, Carla sigue en la papelera")
	}
	boton("Eliminar").AlPulsar()
	if len(s.escrituras) != 1 || !contieneTexto(textosDe(x.cuerpo), "¿Eliminar para siempre?") {
		t.Fatalf("el primer «Eliminar» no pregunta: %v", s.escrituras)
	}
	boton("Sí, eliminar").AlPulsar()
	if len(s.escrituras) != 2 || s.escrituras[1].ruta != "DELETE /api/v1/trash/t2" {
		t.Fatalf("escrituras %v", s.escrituras)
	}
	if !contieneTexto(textosDe(x.cuerpo), "La papelera está vacía.") {
		t.Errorf("vacía, dice %v", textosDe(x.cuerpo))
	}
}

// armazonDe saca el armazón de la pantalla montada (va solo, o apilado con el botón
// flotante).
func armazonDe(t *testing.T, a *App) *ui.Armazon {
	t.Helper()
	switch x := a.raiz.Objects[0].(type) {
	case *ui.Armazon:
		return x
	case *fyne.Container:
		if ar, es := x.Objects[0].(*ui.Armazon); es {
			return ar
		}
	}
	t.Fatalf("la pantalla no es un armazón: %T", a.raiz.Objects[0])
	return nil
}

// TestEnElTelefonoElMenuEsUnCajon: a 390 la pantalla se entera del ancho, la barra
// móvil abre el cajón y lo que se activa dentro lo cierra; con una ficha abierta la
// barra ofrece volver a la lista, y volver la suelta.
func TestEnElTelefonoElMenuEsUnCajon(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}})
	a.Cargar()
	a.raiz.Resize(fyne.NewSize(390, 800))
	if a.Pantalla.Ventana != 390 {
		t.Fatalf("la pantalla cree que la ventana mide %v", a.Pantalla.Ventana)
	}
	barra, es := armazonDe(t, a).BarraMovil.(*ui.BarraMovil)
	if !es {
		t.Fatal("sin barra móvil no hay forma de abrir el menú en el teléfono")
	}
	barra.AlMenu()
	if !a.Pantalla.CajonAbierto || !armazonDe(t, a).CajonAbierto {
		t.Fatal("la hamburguesa no abre el cajón")
	}
	a.Pantalla.enMenu(a.Pantalla.AlVerCumpleanos)()
	if a.Pantalla.CajonAbierto || armazonDe(t, a).CajonAbierto {
		t.Error("abrir «Cumpleaños» desde el cajón lo deja abierto detrás del diálogo")
	}
	a.lienzo().Overlays().Top().(ui.Descartable).Descartar()

	a.Pantalla.AlElegirContacto(a.Pantalla.Contactos[0])
	barra = armazonDe(t, a).BarraMovil.(*ui.BarraMovil)
	if !barra.EnDetalle || barra.Titulo != "Contactos" {
		t.Fatalf("con la ficha abierta la barra está en detalle %v con «%s»", barra.EnDetalle, barra.Titulo)
	}
	barra.AlVolver()
	if a.Pantalla.DetalleAbierto || a.Pantalla.Seleccionado != nil {
		t.Error("«Volver a la lista» no suelta la ficha")
	}
}

// TestLaBarraDeLoteMandaLosMarcadosALaPapelera: marcar dos saca la barra con su
// recuento; «A papelera» manda sus UID limpios en UNA petición, avisa y suelta la
// selección.
func TestLaBarraDeLoteMandaLosMarcadosALaPapelera(t *testing.T) {
	ana, bruno := persona("ana", "Ana"), persona("bruno", "Bruno")
	ana.UID = "urn:uuid:u-ana"
	s := &servicioFalso{contactos: []api.Contact{ana, bruno, persona("carla", "Carla")}}
	a := appDePrueba(t, s)
	a.Cargar()
	if a.Pantalla.barraLote() != nil {
		t.Fatal("sin nada marcado ya hay barra de lote")
	}
	a.Pantalla.AlMarcar("ana")
	a.Pantalla.AlMarcar("bruno")
	b := a.Pantalla.barraLote()
	if b == nil || b.Recuento != "2 contactos seleccionados" {
		t.Fatalf("la barra es %v", b)
	}
	rotulos := make([]string, 0, len(b.Acciones))
	for _, x := range b.Acciones {
		rotulos = append(rotulos, x.Texto)
	}
	if quiero := []string{"Destacar", "Quitar destacado", "Añadir a etiqueta", "Quitar de etiqueta", "A papelera"}; !iguales(rotulos, quiero) {
		t.Errorf("las acciones son %v y en la web %v", rotulos, quiero)
	}
	b.Acciones[len(b.Acciones)-1].AlPulsar()
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != "POST /api/v1/contacts/batch/trash" ||
		s.escrituras[0].cuerpo != `{"uids":["u-ana","u-bruno"]}` {
		t.Fatalf("escrituras %v", s.escrituras)
	}
	if av := a.avisos.Avisos(); len(av) != 1 || av[0].Titulo != "2 contactos en la papelera" {
		t.Errorf("avisos %v", av)
	}
	if len(a.marcados) != 0 || a.Pantalla.barraLote() != nil {
		t.Error("hecho el lote, la selección sigue")
	}
}

// TestExportarLibretaYCSV: «Exportar vCard» deja UN `.vcf` con las tarjetas del
// servicio de toda la libreta, con el nombre del catálogo; «Exportar CSV», el CSV del
// servicio con el nombre que da él.
func TestExportarLibretaYCSV(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{persona("ana", "Ana"), persona("bruno", "Bruno")}})
	guardados := map[string]string{}
	a.Descargas = func(nombre string, contenido []byte) error {
		guardados[nombre] = string(contenido)
		return nil
	}
	a.Cargar()
	a.Pantalla.AlExportarVcard()
	a.Pantalla.AlExportarCSV()
	vcf := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:ana\r\nEND:VCARD\r\nBEGIN:VCARD\r\nVERSION:4.0\r\nFN:bruno\r\nEND:VCARD\r\n"
	if got := guardados["contactos.vcf"]; got != vcf {
		t.Errorf("el .vcf de la libreta es %q", got)
	}
	if got := guardados["contactos.csv"]; got != "Name\r\nAna\r\n" {
		t.Errorf("el CSV es %q (guardados: %v)", got, guardados)
	}
}
