// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// LOS BANCOS DEL LIENZO: repintar un cuaderno de 100 y de 1.000 notas en columnas y en
// lista, en escritorio (1280×800) y en el teléfono (390×844), y una tecla del filtro. Son
// los de la medición del 2026-11-24 (docs/roadmap.md, «Rendimiento de las apps
// nativas»); se quedan para volver a medir.
//
//	go test -tags goolm -run '^$' -bench 'Notes' -benchmem ./apps/notes/

// notasDeBanco son n notas de todo tipo y largo, repartidas en las tres columnas.
func notasDeBanco(n int) []api.Note {
	ayer := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	out := make([]api.Note, 0, n)
	for i := range n {
		nota := api.Note{
			ID: fmt.Sprintf("n%04d", i), NotebookID: "c1", Kind: "text", Col: i % 3, Position: i,
			Title: fmt.Sprintf("Nota %d de la mudanza", i), UpdatedAt: ayer, CreatedAt: ayer,
		}
		switch i % 5 {
		case 0:
			nota.Content = "<p>Pedir tres presupuestos a empresas de mudanza y comparar precios.</p><ul><li>Cajas</li><li>Camión</li><li>Seguro</li></ul>"
		case 1:
			nota.Kind, nota.Checked = "todo", i%2 == 0
			nota.Title = fmt.Sprintf("Llamar al casero %d", i)
		case 2:
			nota.Content = "<p><b>Dirección nueva:</b> calle Mayor 3, segundo piso. " + strings.Repeat("Una frase más para que el párrafo parta en varias líneas. ", 1+i%4) + "</p>"
			nota.States = []api.NoteState{{TagID: "t1", StateID: "s1", Name: "Alta", TagName: "Prioridad", Color: "#b0413a"}}
		case 3:
			nota.Content = "<h2>Lista</h2><ol><li>Uno</li><li>Dos</li><li>Tres</li></ol><p>Fin.</p>"
			nota.Pinned = i%2 == 1
		default:
			nota.Content = "<p>Veinte grandes.</p>"
		}
		out = append(out, nota)
	}
	return out
}

// montarBanco arma la pantalla en una ventana de prueba con n notas en esa vista.
func montarBanco(b testing.TB, ancho, alto float32, vista string, n int) *Pantalla {
	b.Helper()
	w := test.NewWindow(nil)
	w.SetPadded(false)
	b.Cleanup(w.Close)
	w.Resize(fyne.NewSize(ancho, alto))
	tr, err := i18n.New("es", i18n.Common(), catalogos.Notes())
	if err != nil {
		b.Fatal(err)
	}
	p := NuevaPantalla(tr, "es", nil, nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(ancho, alto))
	c := DeCuaderno(api.Notebook{ID: "c1", Title: "Mudanza", Color: "#86198f", Layout: vista, NoteCount: n})
	p.cuadernos, p.activo = []Cuaderno{c}, c
	p.lateral.Fase = Exito
	p.notas, p.lienzo.Fase = DeNotas(notasDeBanco(n)), Exito
	p.etiquetas = []api.Tag{{ID: "t1", Name: "Prioridad", States: []api.TagState{{ID: "s1", Name: "Alta", Color: "#b0413a", Shortcut: "1"}}}}
	p.Repintar()
	return p
}

// BenchmarkNotesRepintar es `Repintar` con el estado sin cambiar: lo que cuesta cada
// cambio que no toca las notas (una imagen que llega, el foco, la barra).
func BenchmarkNotesRepintar(b *testing.B) {
	for _, pantalla := range []struct {
		nombre      string
		ancho, alto float32
	}{{"escritorio", 1280, 800}, {"movil", 390, 844}} {
		for _, vista := range []string{"columns", "list"} {
			for _, n := range []int{100, 1000} {
				b.Run(fmt.Sprintf("%s/%s/%d", pantalla.nombre, vista, n), func(b *testing.B) {
					p := montarBanco(b, pantalla.ancho, pantalla.alto, vista, n)
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						p.Repintar()
					}
				})
			}
		}
	}
}

// BenchmarkNotesFiltroTecla es una tecla en el filtro del cuaderno: lo que cuesta
// mientras se escribe (`tecla`) y, aparte, el repintado filtrado que llega al dejar de
// escribir (`repintado`).
func BenchmarkNotesFiltroTecla(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprintf("tecla/%d", n), func(b *testing.B) {
			p := montarBanco(b, 1280, 800, "columns", n)
			textos := []string{"m", "mu", "mud", "muda", "mudan"}
			b.ReportAllocs()
			b.ResetTimer()
			i := 0
			for b.Loop() {
				p.barra.AlFiltrar(textos[i%len(textos)])
				// El repintado llega al dejar de escribir (EsperaFiltro): se mide aparte.
				p.relojFiltro.Stop()
				i++
			}
		})
		b.Run(fmt.Sprintf("repintado/%d", n), func(b *testing.B) {
			p := montarBanco(b, 1280, 800, "columns", n)
			textos := []string{"m", "mu", "mud", "muda", "mudan"}
			b.ReportAllocs()
			b.ResetTimer()
			i := 0
			for b.Loop() {
				p.filtro = textos[i%len(textos)]
				p.Repintar()
				i++
			}
		})
	}
}

// colgadas son las tarjetas que las columnas del lienzo tienen en el árbol, y todas las
// que reparten.
func colgadas(p *Pantalla) (colgadas, todas int) {
	for _, cv := range p.lienzo.virtuales {
		colgadas += len(cv.c.Objects)
		todas += len(cv.todas)
	}
	return colgadas, todas
}

// TestSoloSeMontanLasTarjetasQueSeVen: con mil notas, en columnas y en lista, el árbol
// lleva solo las tarjetas de la vista y una vista más por cada lado; el desplazador mide
// lo mismo que con todas montadas, y al bajar entran las de abajo y salen las de arriba.
// La que tiene el foco se queda aunque deje de verse. VISTO EN ROJO con las columnas de
// antes (`columnaTarjetas`): las mil colgadas.
func TestSoloSeMontanLasTarjetasQueSeVen(t *testing.T) {
	for _, vista := range []string{"columns", "list"} {
		t.Run(vista, func(t *testing.T) {
			p := montarBanco(t, 1280, 800, vista, 1000)
			colg, todas := colgadas(p)
			if todas != 1000 || colg == 0 || colg > 150 {
				t.Fatalf("de %d tarjetas hay %d colgadas", todas, colg)
			}
			// El mínimo es el de todas a su ancho, colgadas o no.
			for _, cv := range p.lienzo.virtuales {
				var alto float32
				for i, tj := range cv.todas {
					_, w := anchoDe(tj, cv.ancho)
					if i > 0 {
						alto += cv.hueco
					}
					alto += MedidaTarjetaNota(tj, w).Height
				}
				if got := cv.c.MinSize().Height; got < alto-0.5 || got > alto+0.5 {
					t.Errorf("la columna mide %v y sus tarjetas suman %v", got, alto)
				}
			}
			primera := p.lienzo.virtuales[0].todas[0]
			if primera.fuera {
				t.Fatal("la primera tarjeta no está colgada")
			}
			primera.FocusGained()

			s := p.desplazador
			s.ScrollToOffset(fyne.NewPos(0, s.Content.MinSize().Height/2))
			s.OnScrolled(s.Offset)
			cv := p.lienzo.virtuales[0]
			medio := -1
			for i := range cv.todas {
				if cv.ys[i] >= s.Offset.Y {
					medio = i
					break
				}
			}
			if medio < 0 || cv.todas[medio].fuera {
				t.Fatalf("a media columna la tarjeta %d no está colgada", medio)
			}
			if primera.fuera {
				t.Error("la tarjeta con el foco se ha soltado al dejar de verse")
			}
			if segunda := cv.todas[1]; !segunda.fuera {
				t.Error("las tarjetas de arriba siguen colgadas lejos de la vista")
			}
			if colg2, _ := colgadas(p); colg2 > 150 {
				t.Errorf("tras bajar hay %d colgadas", colg2)
			}
		})
	}
}

// TestElMasNoExisteHastaQueHaceFalta: un cuaderno entero en reposo no lleva ni un «…»:
// solo su hueco. Pasar el ratón por una tarjeta lo crea en ella y en ninguna más.
func TestElMasNoExisteHastaQueHaceFalta(t *testing.T) {
	p := montarBanco(t, 1280, 800, "columns", 100)
	var montadas []*Tarjeta
	for _, cv := range p.lienzo.virtuales {
		for _, o := range cv.c.Objects {
			tj := o.(*Tarjeta)
			test.WidgetRenderer(tj)
			if tj.mas != nil {
				t.Fatalf("la tarjeta %s lleva «…» en reposo", tj.nota.ID)
			}
			montadas = append(montadas, tj)
		}
	}
	montadas[0].MouseIn(nil)
	for i, tj := range montadas {
		if (tj.mas != nil) != (i == 0) {
			t.Errorf("la tarjeta %d: «…» %v", i, tj.mas != nil)
		}
	}
}

// TestRepintarConservaElCarrilYLaBarra: sin cambios, el carril y la barra del cuaderno
// son los mismos; cambiar el título del cuaderno los rehace. VISTO EN ROJO con el carril
// rehecho en cada repintado.
func TestRepintarConservaElCarrilYLaBarra(t *testing.T) {
	p := montarBanco(t, 1280, 800, "columns", 10)
	carril, barra := p.lateralHecho, p.barraHecha
	p.Repintar()
	if p.lateralHecho != carril || p.barraHecha != barra {
		t.Fatal("un repintado sin cambios ha rehecho el carril o la barra")
	}
	p.notas = p.notas[:5]
	p.Repintar()
	if p.lateralHecho == carril {
		t.Error("el contador del cuaderno ha cambiado y el carril es el de antes")
	}
	if p.barraHecha != barra {
		t.Error("cinco notas menos no cambian la barra, y se ha rehecho")
	}
	carril = p.lateralHecho
	c := p.activo
	c.Titulo = "Mudanza a Cádiz"
	p.cuadernos, p.activo = []Cuaderno{c}, c
	p.Repintar()
	if p.lateralHecho == carril || p.barraHecha == barra {
		t.Error("otro título del cuaderno tenía que rehacer el carril y la barra")
	}
}

// TestRepintarSueltaLasTarjetasQueSeVan: la tarjeta que un repintado sustituye (su nota ha
// cambiado) o descuelga (el filtro la quita) suelta su renderer en el acto
// (ui.SoltarArboles), sin esperar el minuto de la caché de Fyne; la que sigue, no. VISTO
// EN ROJO con `soltarArboles` apagado: las dos seguían con su renderer.
func TestRepintarSueltaLasTarjetasQueSeVan(t *testing.T) {
	p := montarBanco(t, 1280, 800, "columns", 10)
	vieja, quitada, sigue := p.lienzo.vivas["n0001"], p.lienzo.vivas["n0002"], p.lienzo.vivas["n0003"]
	if vieja.render == nil || quitada.render == nil || sigue.render == nil {
		t.Fatal("las tarjetas no se han pintado")
	}
	c := p.notas[1].Crudo
	c.Title = "Cajas de cartón"
	p.notas[1] = DeNota(c)
	p.notas = append(p.notas[:2], p.notas[3:]...)
	p.Repintar()
	if p.lienzo.vivas["n0001"] == vieja {
		t.Fatal("la nota cambiada conserva su tarjeta")
	}
	if vieja.render != nil || quitada.render != nil {
		t.Error("la tarjeta sustituida o descolgada sigue con su renderer")
	}
	if sigue.render == nil || p.lienzo.vivas["n0003"] != sigue {
		t.Error("se ha soltado una tarjeta que sigue en el cuaderno")
	}
}
