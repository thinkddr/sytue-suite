// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// LOS BANCOS DE LA BANDEJA: refrescarla con 50 y 2.000 conversaciones (un cuarto con
// etiquetas y alguna marcada) y desplazar la lista, que recicla filas. Son los de la
// medición del 2026-11-24 (docs/roadmap.md, «Rendimiento de las apps nativas»).
//
//	go test -tags goolm -run '^$' -bench 'Mail' -benchmem ./apps/mail/

func conversacionesDeBanco(n int) []Conversacion {
	base := time.Date(2026, 9, 11, 9, 30, 0, 0, time.UTC)
	ms := make([]Mensaje, 0, n)
	for i := range n {
		m := Mensaje{
			ID: fmt.Sprintf("m%d", i), HiloID: fmt.Sprintf("h%d", i),
			Asunto:   fmt.Sprintf("Asunto número %d del trimestre", i),
			Extracto: "Os paso las ventanas de mantenimiento de octubre y noviembre.",
			De:       []Direccion{{Nombre: "Sofía Nogueira Paz", Correo: "sofia@sytue.eu"}},
			Recibido: base.Add(-time.Duration(i) * time.Minute), Leido: i%3 == 0,
		}
		if i%4 == 0 {
			m.Etiquetas = map[string]bool{"e1": true, "e2": i%8 == 0}
		}
		ms = append(ms, m)
	}
	return AgruparPorHilo(ms)
}

func bandejaDeBanco(b *testing.B, n int) (*Bandeja, fyne.Window) {
	b.Helper()
	tr, err := Traductor("es")
	if err != nil {
		b.Fatal(err)
	}
	bj := NuevaBandeja(tr, "es")
	bj.Ahora = reloj()
	bj.Titulo, bj.Estado = "Recibidos", Listo
	bj.Etiquetas = []Etiqueta{{ID: "e1", Nombre: "Clientes", Color: "#0e7490"}, {ID: "e2", Nombre: "Urgente", Color: "#b0413a"}}
	bj.Conversaciones = conversacionesDeBanco(n)
	bj.Marcadas = map[string]bool{"h3": true, "h7": true}
	w := test.NewWindow(bj)
	b.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(320, 744))
	w.Canvas().Capture()
	return bj, w
}

// BenchmarkMailRefrescar es repintar la bandeja sin cambiar la lista: lo que pasa al
// marcar una casilla, al llegar un logo o con cada vuelta del sondeo.
func BenchmarkMailRefrescar(b *testing.B) {
	for _, n := range []int{50, 2000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			bj, _ := bandejaDeBanco(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				bj.Refresh()
			}
		})
	}
}

// BenchmarkMailDesplazar es bajar la lista un poco cada vez: la lista recoloca y
// recicla filas, y cada una se rellena.
func BenchmarkMailDesplazar(b *testing.B) {
	for _, n := range []int{50, 2000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			bj, _ := bandejaDeBanco(b, n)
			l := bj.r.lista
			b.ReportAllocs()
			b.ResetTimer()
			y := float32(0)
			for b.Loop() {
				y += 37
				if y > 30*AltoFilaConversacion() {
					y = 0
				}
				l.ScrollToOffset(y)
			}
		})
	}
}
