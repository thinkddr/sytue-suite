// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// buscarCarga recorre lo que pinta una pantalla y dice qué indicador de carga lleva:
// los esqueletos (Ocupado) y las ruedas sueltas.
func buscarCarga(o fyne.CanvasObject) (ocupados []*Ocupado, ruedas int) {
	var andar func(fyne.CanvasObject)
	andar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case nil:
			return
		case *Ocupado:
			ocupados = append(ocupados, x)
			andar(x.Contenido)
		case *ui.Rueda:
			ruedas++
		case *fyne.Container:
			for _, h := range x.Objects {
				andar(h)
			}
		case *container.Scroll:
			andar(x.Content)
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				andar(h)
			}
		}
	}
	andar(o)
	return ocupados, ruedas
}

// TestLaPrimeraCargaEnseñaElEsqueletoYNoLaRueda: las pantallas de Fotos, en su primera
// carga, enseñan un esqueleto con la forma de lo que va a llegar, uno solo (un único
// «Cargando»), y ninguna rueda suelta.
func TestLaPrimeraCargaEnseñaElEsqueletoYNoLaRueda(t *testing.T) {
	fotos := fotosDePrueba(21)
	base := vistaDePrueba(t, nil)
	base.Estado = Cargando
	pantallas := pantallasDePrueba(fotos, base)
	pantallas["compartidos"] = func(v Vista) fyne.CanvasObject { return PantallaCompartidos(v, Compartidos{}) }
	pantallas["compartido"] = func(v Vista) fyne.CanvasObject {
		return PantallaCompartido(v, CompartidoAbierto{Titulo: "Verano"})
	}
	for _, nombre := range []string{
		"cuadricula", "favoritas", "albumes", "album", "papelera", "repetidas",
		"parecidas", "cerca", "publica", "compartidos", "compartido",
	} {
		t.Run(nombre, func(t *testing.T) {
			var ocupados []*Ocupado
			var ruedas int
			fyne.DoAndWait(func() {
				o := pantallas[nombre](base)
				o.Resize(fyne.NewSize(ventanaAncho, ventanaAlto))
				ocupados, ruedas = buscarCarga(o)
			})
			if len(ocupados) != 1 {
				t.Fatalf("lleva %d esqueletos y tiene que llevar uno", len(ocupados))
			}
			if ruedas != 0 {
				t.Errorf("sigue con %d ruedas en la primera carga", ruedas)
			}
			if got := ocupados[0].AccessibilityLabel(); got != "Cargando" {
				t.Errorf("el esqueleto se anuncia como %q y no como «Cargando»", got)
			}
		})
	}
}

// TestLaRuedaQuedaParaLasPaginasSiguientes: con fotos a la vista y otra página en
// camino, la galería sigue con la rueda del centinela y sin esqueleto.
func TestLaRuedaQuedaParaLasPaginasSiguientes(t *testing.T) {
	fotos := fotosDePrueba(6)
	v := vistaDePrueba(t, nil)
	var ocupados []*Ocupado
	var ruedas int
	fyne.DoAndWait(func() {
		ocupados, ruedas = buscarCarga(PantallaGaleria(v, Galeria{Grupos: PorMes(fotos, v.T), Cargando: true}))
	})
	if len(ocupados) != 0 || ruedas != 1 {
		t.Errorf("con fotos y otra página en camino: %d esqueletos y %d ruedas; tiene que ser 0 y 1", len(ocupados), ruedas)
	}
}

// TestElEsqueletoDeLaGaleriaTieneSuAltoDeFila: las teselas del esqueleto van en el
// layout de la galería, con proporciones variadas, y cada fila mide al menos el alto de
// fila de la galería real a ese ancho (y todas las de una fila, lo mismo).
func TestElEsqueletoDeLaGaleriaTieneSuAltoDeFila(t *testing.T) {
	v := vistaDePrueba(t, nil)
	_, ancho := CajaGaleria(ventanaAncho)
	fyne.DoAndWait(func() {
		o := EsqueletoFilas(v, TeselasEsqueletoGaleria, true)
		pila, ok := o.Contenido.(*fyne.Container)
		if !ok || len(pila.Objects) != 2 {
			t.Fatalf("con `mes` tiene que ser la cabecera y las filas; es %T", o.Contenido)
		}
		filas := pila.Objects[1].(*fyne.Container)
		l, ok := filas.Layout.(Filas)
		if !ok {
			t.Fatalf("las teselas no van en el layout de la galería: %T", filas.Layout)
		}
		distintas := map[float32]bool{}
		for _, r := range l.Razones {
			distintas[r] = true
			if r < ProporcionMin || r > ProporcionMax {
				t.Errorf("la proporción %v se sale de las que admite la galería", r)
			}
		}
		if len(distintas) < 4 {
			t.Errorf("solo %d proporciones distintas: el esqueleto parecería una cuadrícula", len(distintas))
		}
		alto := ui.AltoA(o, ancho)
		cab := tema.Espacio(4) + tema.TextoSM.Linea
		if alto <= cab {
			t.Fatalf("el esqueleto mide %v: no hay filas", alto)
		}
		filas.Resize(fyne.NewSize(ancho, ui.AltoA(filas, ancho)))
		fila := AltoFila(ventanaAncho, ancho)
		primera := filas.Objects[0]
		for _, h := range filas.Objects {
			if _, es := h.(*ui.Esqueleto); !es {
				t.Fatalf("una tesela es %T y no un esqueleto", h)
			}
			if h.Size().Height+0.5 < fila {
				t.Errorf("una tesela mide %v de alto, menos que el alto de fila %v", h.Size().Height, fila)
			}
			if h.Position().Y == primera.Position().Y && h.Size().Height != primera.Size().Height {
				t.Errorf("dos teselas de la misma fila miden %v y %v", h.Size().Height, primera.Size().Height)
			}
		}
	})
}

// TestLasTarjetasDelEsqueletoSonLasDeLosAlbumes: misma tarjeta 4:3 y misma rejilla que
// «Álbumes», así que al llegar la lista nada se mueve.
func TestLasTarjetasDelEsqueletoSonLasDeLosAlbumes(t *testing.T) {
	v := vistaDePrueba(t, nil)
	album := api.Album{ID: "a1", Title: "Verano", Count: 3}
	fyne.DoAndWait(func() {
		esq := EsqueletoAlbumes(v, 4, tema.Espacio(6))
		rej, ok := esq.Contenido.(*fyne.Container)
		if !ok {
			t.Fatalf("el contenido es %T", esq.Contenido)
		}
		r, ok := rej.Layout.(ui.Rejilla)
		if !ok || r.Hueco != tema.Espacio(6) || r.Min != ui.RejillaAlbumes().Min {
			t.Fatalf("no es la rejilla de «Álbumes»: %#v", rej.Layout)
		}
		de := func(o fyne.CanvasObject) fyne.Size {
			c := o.(*fyne.Container)
			c.Resize(fyne.NewSize(200, ui.AltoA(c, 200)))
			c.Layout.Layout(c.Objects, c.Size())
			return c.Objects[0].Size()
		}
		verdad, gris := de(TarjetaAlbum(v, album, nil)), de(rej.Objects[0])
		if verdad != gris {
			t.Errorf("la portada del esqueleto mide %v y la de verdad %v", gris, verdad)
		}
		if gris.Height != gris.Width*3/4 {
			t.Errorf("la portada del esqueleto no es 4:3: %v", gris)
		}
		if a, b := ui.AltoA(rej.Objects[0], 200), ui.AltoA(TarjetaAlbum(v, album, nil), 200); a != b {
			t.Errorf("la tarjeta del esqueleto mide %v de alto y la de verdad %v", a, b)
		}
	})
}
