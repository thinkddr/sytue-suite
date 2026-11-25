// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"bytes"
	"fmt"
	"image"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// LOS BANCOS DE CONTACTS: una tecla en el buscador, un píxel de ancho de la ventana y
// elegir un contacto, con libretas de 600 y 3.000 contactos. Son los de la medición del
// 2026-11-24 (docs/roadmap.md, «Rendimiento de las apps nativas»).
//
//	go test -tags goolm -run '^$' -bench 'Contacts' -benchmem ./contactos/

func appDeBanco(b testing.TB, n int) (*App, fyne.Window) {
	b.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Contacts())
	if err != nil {
		b.Fatal(err)
	}
	a := NuevaApp(tr, nil)
	cs := make([]api.Contact, n)
	for i := range cs {
		cs[i] = api.Contact{
			ID: fmt.Sprintf("c%04d", i), UID: fmt.Sprintf("u-c%04d", i),
			FormattedName: fmt.Sprintf("%c%s %04d", 'A'+rune(i%26), "ontacto", i),
			Emails:        []api.ContactEmail{{Email: fmt.Sprintf("c%04d@sytue.eu", i)}},
		}
	}
	a.todos = porInicial(cs, "es")
	libreta := api.AddressBook{ID: "l1", DisplayName: "Personal"}
	a.Pantalla.Libretas, a.Pantalla.Activa = []api.AddressBook{libreta}, &libreta
	a.Pantalla.Estado = ui.EstadoExito
	w := test.NewWindow(a.Objeto())
	b.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 744))
	a.repintar()
	return a, w
}

// BenchmarkContactsTecla es una tecla en el buscador.
func BenchmarkContactsTecla(b *testing.B) {
	for _, n := range []int{600, 3000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			a, _ := appDeBanco(b, n)
			textos := []string{"c", "co", "con", "cont", "conta"}
			b.ReportAllocs()
			b.ResetTimer()
			i := 0
			for b.Loop() {
				a.Pantalla.AlBuscar(textos[i%len(textos)])
				i++
			}
		})
	}
}

// BenchmarkContactsRedimensionar es arrastrar el borde de la ventana un píxel, entre
// dos cortes, con un contacto abierto.
func BenchmarkContactsRedimensionar(b *testing.B) {
	a, w := appDeBanco(b, 600)
	a.elegir(a.Pantalla.Contactos[0])
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		w.Resize(fyne.NewSize(float32(1400+i%2), 744))
		i++
	}
}

// BenchmarkContactsElegir es abrir un contacto de la lista.
func BenchmarkContactsElegir(b *testing.B) {
	a, _ := appDeBanco(b, 600)
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		a.elegir(a.Pantalla.Contactos[i%20])
		i++
	}
}

// BenchmarkContactsRedimensionarEstrecha es lo mismo en una ventana estrecha (1000 px) y
// en el teléfono (390), donde la tarjeta de la ficha no llega a su ancho máximo y cada
// píxel de la ventana la cambia. El contacto lleva nota, que toma el ancho de la tarjeta.
func BenchmarkContactsRedimensionarEstrecha(b *testing.B) {
	for _, ancho := range []float32{1000, 390} {
		b.Run(fmt.Sprint(ancho), func(b *testing.B) {
			a, w := appDeBanco(b, 600)
			c := a.Pantalla.Contactos[0]
			c.Note = "Prefiere que le escriban por la mañana.\nPedir presupuesto otra vez para la mudanza."
			a.elegir(c)
			w.Resize(fyne.NewSize(ancho, 744))
			b.ReportAllocs()
			b.ResetTimer()
			i := 0
			for b.Loop() {
				w.Resize(fyne.NewSize(ancho+float32(i%2), 744))
				i++
			}
		})
	}
}

// TestRedimensionarEstrechaAjustaLaFichaEnSuSitio: en una ventana estrecha con un
// contacto abierto, un píxel de ancho no rehace la ficha: la misma, con la nota partida
// al ancho nuevo de su columna. En el teléfono, la lista (que no se ve) espera a volver a
// verse. VISTO EN ROJO con la ficha rehecha a cada píxel (`huellaAncho` cambiaba con el
// ancho de la tarjeta).
func TestRedimensionarEstrechaAjustaLaFichaEnSuSitio(t *testing.T) {
	a, w := appDeBanco(t, 600)
	p := a.Pantalla
	c := p.Contactos[0]
	c.Note = "Prefiere que le escriban por la mañana."
	w.Resize(fyne.NewSize(1000, 744))
	a.elegir(c)
	ficha := p.armazon.Contenido
	w.Resize(fyne.NewSize(1001, 744))
	if p.armazon.Contenido != ficha {
		t.Fatal("un píxel de ancho ha rehecho la ficha")
	}
	var nota *ui.Parrafo
	recorrer(ficha, func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.Parrafo); ok && x.Texto == c.Note {
			nota = x
		}
	})
	if nota == nil || nota.Ancho != p.anchoValor() {
		t.Fatalf("la nota no está al ancho nuevo de su columna (%v)", p.anchoValor())
	}
	// Lo ajustado en su sitio se pinta igual que la ficha rehecha a ese ancho.
	enSitio := w.Canvas().Capture()
	a.repintar()
	if rehecha := w.Canvas().Capture(); !bytes.Equal(enSitio.(*image.NRGBA).Pix, rehecha.(*image.NRGBA).Pix) {
		t.Error("la ficha ajustada en su sitio no se pinta como la rehecha")
	}

	w.Resize(fyne.NewSize(390, 744))
	lista := p.armazon.Panel
	w.Resize(fyne.NewSize(391, 744))
	if p.armazon.Panel != lista || !p.listaPendiente {
		t.Error("en el teléfono con la ficha delante, la lista se ha rehecho sin verse")
	}
	p.AlVolver()
	if p.armazon.Panel == lista || p.listaPendiente {
		t.Error("al volver a la lista no se ha rehecho al ancho de ahora")
	}
}

// recorrer visita o y lo que tiene dentro, contenedores y widgets.
func recorrer(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
	f(o)
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			recorrer(h, f)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			recorrer(h, f)
		}
	}
}
