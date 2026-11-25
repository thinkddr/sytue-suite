// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fmt"
	"image"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// LOS BANCOS DE LA CUADRÍCULA: repintar «Mi unidad» en tarjetas con 500 y 2.000
// elementos, marcar uno y que llegue la miniatura de uno, en escritorio (1280×800). Son
// los de la medición del 2026-11-24 (docs/roadmap.md, «Rendimiento de las apps nativas»).
//
//	go test -tags goolm -run '^$' -bench 'Drive' -benchmem ./apps/drive/

func elementosDeBanco(n int) []Elemento {
	hoy := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	out := make([]Elemento, 0, n)
	for i := range n {
		el := Elemento{ID: fmt.Sprintf("f%04d", i), Nombre: fmt.Sprintf("informe-%04d.pdf", i), Tamano: int64(1000 * i), Modificado: hoy, Tipo: "application/pdf"}
		switch {
		case i < 12:
			el = Elemento{ID: fmt.Sprintf("d%04d", i), Nombre: fmt.Sprintf("Carpeta %d", i), Carpeta: true, Tamano: -1, Modificado: hoy}
		case i%5 == 0:
			el.Nombre, el.Tipo = fmt.Sprintf("foto-%04d.png", i), "image/png"
		}
		out = append(out, el)
	}
	return out
}

func pantallaDeBanco(b testing.TB, n int) *Pantalla {
	b.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Drive())
	if err != nil {
		b.Fatal(err)
	}
	w := test.NewWindow(nil)
	w.SetPadded(false)
	b.Cleanup(w.Close)
	p := NuevaPantalla(tr, nil, nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	e := p.explor
	e.Rejilla, e.Fase, e.Elementos = true, Exito, elementosDeBanco(n)
	// Las miniaturas ya pedidas: que no se pidan en cada repintado.
	for _, el := range e.Elementos {
		if ConMiniatura(el) {
			e.Miniaturas[el.ID] = nil
		}
	}
	p.Repintar()
	return p
}

// BenchmarkDriveRepintar es repintar la cuadrícula sin cambiar nada.
func BenchmarkDriveRepintar(b *testing.B) {
	for _, n := range []int{500, 2000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			p := pantallaDeBanco(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				p.Repintar()
			}
		})
	}
}

// BenchmarkDriveMarcar es marcar y desmarcar un fichero de la cuadrícula.
func BenchmarkDriveMarcar(b *testing.B) {
	for _, n := range []int{500, 2000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			p := pantallaDeBanco(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				p.marcar("f0013")
			}
		})
	}
}

// BenchmarkDriveMiniatura es que llegue la miniatura de una imagen de la cuadrícula.
func BenchmarkDriveMiniatura(b *testing.B) {
	p := pantallaDeBanco(b, 2000)
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		p.llegaMiniatura("f0015", img)
	}
}

// TestMarcarActualizaEnSuSitio: marcar un fichero de la cuadrícula no rehace la pantalla:
// las tarjetas montadas, la cabecera, el cuerpo y el menú son los mismos, la marcada lo
// dice y la franja de lote aparece y se va. VISTO EN ROJO con `marcar` repintando.
func TestMarcarActualizaEnSuSitio(t *testing.T) {
	p := pantallaDeBanco(t, 500)
	e := p.explor
	menu, cuerpo, vista := p.arm.Menu, e.franjas.cuerpo, e.vista
	tarjeta := func(id string) *TarjetaDrive {
		for _, tp := range e.tarjetas {
			if tp.el.ID == id {
				return tp.t
			}
		}
		t.Fatalf("la tarjeta %s no está montada", id)
		return nil
	}
	antes := tarjeta("f0013")
	p.marcar("f0013")
	if p.arm.Menu != menu || e.franjas.cuerpo != cuerpo || e.vista != vista || tarjeta("f0013") != antes {
		t.Fatal("marcar ha rehecho la pantalla")
	}
	if !antes.Seleccionada || !tarjeta("f0014").SeleccionActiva || len(vista.Objects) != 3 {
		t.Errorf("marcada %v, la vecina con casilla %v y %d franjas (cabecera, lote y cuerpo)",
			antes.Seleccionada, tarjeta("f0014").SeleccionActiva, len(vista.Objects))
	}
	p.marcar("f0013")
	if antes.Seleccionada || tarjeta("f0014").SeleccionActiva || len(vista.Objects) != 2 {
		t.Errorf("desmarcada: %v, la vecina con casilla %v y %d franjas", antes.Seleccionada,
			tarjeta("f0014").SeleccionActiva, len(vista.Objects))
	}
}
