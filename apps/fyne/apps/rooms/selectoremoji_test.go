// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/go-text/typesetting/font"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/fuentes"
)

// La cobertura se comprueba sobre el TTF distribuido, sin fontconfig, fuentes del
// SO ni el fallback antiguo de Fyne. La tabla EmojiOneColor no cubre 🤪🤯🥱🤍.
func TestLaFuenteEmojiCubreTodoElCatalogoConImagenesDeColor(t *testing.T) {
	cara, err := font.ParseTTF(bytes.NewReader(fuentes.Emoji()))
	if err != nil {
		t.Fatal(err)
	}
	if len(entradasEmoji) < 100 {
		t.Fatal("la prueba no recorrió el catálogo completo")
	}
	for _, e := range entradasEmoji {
		for _, r := range e.Glifo {
			if r == '\ufe0f' || r == '\u200d' {
				continue // selectores de presentación y unión, no glifos independientes
			}
			g, ok := cara.NominalGlyph(r)
			if !ok || g == 0 {
				t.Errorf(":%s: carece de glifo %U en la fuente embebida", e.Nombres[0], r)
				continue
			}
			bitmap, ok := cara.GlyphData(g).(font.GlyphBitmap)
			if !ok || bitmap.Format != font.PNG || len(bitmap.Data) == 0 {
				t.Errorf(":%s: %U no tiene un bitmap PNG que el pintor pueda dibujar", e.Nombres[0], r)
			}
		}
	}
}

// No basta con tener el archivo: la celda debe usarlo al medir y pintar. Se
// comprueban píxeles distintos y color real para no aceptar cinco tofu iguales.
func TestLasCeldasEmojiPintanColorYNoElGlifoAusente(t *testing.T) {
	vistos := map[[32]byte]string{}
	for _, nombre := range []string{"smile", "zany", "exploding_head", "yawning", "white_heart"} {
		t.Run(nombre, func(t *testing.T) {
			s := NuevoSelectorEmoji(traductor(t), nil)
			var c *celdaEmoji
			for i, e := range s.resultados {
				for _, n := range e.Nombres {
					if n == nombre {
						c = s.celdas[i]
					}
				}
			}
			if c == nil {
				t.Fatalf("falta :%s: en el catálogo", nombre)
			}
			s.activo = -1 // el fondo seleccionado no puede simular tinta ni color del glifo
			// Se captura la celda aislada, no en las coordenadas de su fila/columna.
			c.Move(fyne.Position{})
			c.Resize(fyne.NewSize(32, 32))
			r := test.WidgetRenderer(c).(*celdaEmojiRenderer)
			r.Refresh()
			if r.texto.FontSource != tema.Emoji() {
				t.Fatal("la celda sigue usando Figtree o el fallback del sistema")
			}
			img := pinta(t, 32, 32, c)
			if n := tinta(img, color.NRGBA{R: 255, G: 255, B: 255, A: 255}); n < 20 {
				t.Fatalf("el emoji no tiene tinta: %d píxeles", n)
			}
			datos := make([]byte, 0, 4*img.Bounds().Dx()*img.Bounds().Dy())
			coloreados := 0
			for y := range img.Bounds().Dy() {
				for x := range img.Bounds().Dx() {
					p := px(img, x, y)
					datos = append(datos, p.R, p.G, p.B, p.A)
					if max(int(p.R), int(p.G), int(p.B))-min(int(p.R), int(p.G), int(p.B)) > 20 {
						coloreados++
					}
				}
			}
			if nombre != "white_heart" && coloreados < 20 {
				t.Errorf("el emoji perdió su color: %d píxeles coloreados", coloreados)
			}
			hash := sha256.Sum256(datos)
			if otro := vistos[hash]; otro != "" {
				t.Errorf("%s y %s pintan lo mismo: probable glifo ausente", nombre, otro)
			}
			vistos[hash] = nombre
			guardarEmoji(t, nombre, img)
		})
	}
}

// La ficha de iPad mostraba «� 2» aunque el selector ya pintaba 👍: la píldora
// todavía medía y dibujaba el glifo con Figtree. El contador y los shortcodes
// desconocidos conservan su cara de texto; solo el emoji usa la fuente de color.
func TestLaReaccionUnicodeUsaLaFuenteEmojiAlMedirYPintar(t *testing.T) {
	p := NuevaReaccion(traductor(t), Reaccion{Emoji: "👍", Veces: 2}, nil)
	m := p.MinSize()
	p.Resize(m)
	r := test.WidgetRenderer(p).(*reaccionRenderer)
	r.Layout(m)
	if r.emoji.Text != "👍" || r.emoji.FontSource != tema.Emoji() {
		t.Fatal("la reacción alteró el Unicode o sigue usando Figtree")
	}
	if r.veces.FontSource != ui.Medio.Cara() {
		t.Fatal("el contador perdió la fuente de texto")
	}
	anchoEmoji := ui.MedirConCara("👍", tema.TextoXS, tema.Emoji()).Width
	ancho := 2*CanalonReaccion + 2*tema.Borde + HuecoReaccion + anchoEmoji + ui.Medir("2", tema.TextoXS, ui.Medio).Width
	if m.Width != ancho || r.veces.Position().X != CanalonReaccion+tema.Borde+anchoEmoji+HuecoReaccion {
		t.Fatalf("la caja y el contador no reservan el ancho del emoji: %v, contador=%v, glifo=%v", m, r.veces.Position(), anchoEmoji)
	}
	img := pinta(t, m.Width, m.Height, p)
	coloreados := 0
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			c := px(img, x, y)
			// El pulgar es amarillo. El papel, el filete y el contador no pueden
			// simular este color aunque el tema use una tinta azulada.
			if int(c.R) > int(c.B)+35 && int(c.G) > int(c.B)+20 {
				coloreados++
				if float32(x)/escala >= r.veces.Position().X {
					t.Fatal("el glifo de color invade el contador")
				}
			}
		}
	}
	if coloreados < 20 {
		t.Fatalf("el pulgar no se pintó en color: %d píxeles", coloreados)
	}
	guardarEmoji(t, "reaccion-thumbsup", img)

	const desconocido = ":emoji_no_registrado:"
	p = NuevaReaccion(traductor(t), Reaccion{Emoji: desconocido, Veces: 2}, nil)
	r = test.WidgetRenderer(p).(*reaccionRenderer)
	if r.emoji.Text != desconocido || r.emoji.FontSource != ui.Medio.Cara() || anchoEmojiReaccion(desconocido) != ui.Medir(desconocido, tema.TextoXS, ui.Medio).Width {
		t.Fatal("un shortcode desconocido dejó de ser texto legible")
	}
}

func TestElSelectorEmojiNoAbreElTecladoConElDedo(t *testing.T) {
	antes := tactilEnEmojis
	t.Cleanup(func() { tactilEnEmojis = antes })
	for _, tactil := range []bool{false, true} {
		t.Run(fmt.Sprint(tactil), func(t *testing.T) {
			tactilEnEmojis = func() bool { return tactil }
			sa, _, w := salaEn(t, servicioConDos(), 320)
			sa.Arrancar(context.Background())
			sa.AbrirEmojis(sa.P.Canal.Compositor.BotonEmoji(), func(string) {})
			if got := w.Canvas().Focused() == fyne.Focusable(sa.emojis.Caja()); got == tactil {
				t.Errorf("táctil=%v: foco en buscador=%v", tactil, got)
			}
			w.Canvas().Overlays().Remove(w.Canvas().Overlays().Top())
		})
	}
}

func TestElSelectorEmojiConPocaAlturaTieneUnSoloDesplazador(t *testing.T) {
	s := NuevoSelectorEmoji(traductor(t), nil)
	s.Resize(fyne.NewSize(294, 110))
	r := test.WidgetRenderer(s).(*selectorEmojiRenderer)
	r.Layout(s.Size())
	if !r.recortado || len(r.objetos) != 1 || r.objetos[0] != r.desplaza {
		t.Fatal("el menú recortado no entrega un único desplazador al selector")
	}
	for _, o := range r.entero.Objects {
		if o == s.desplaza || o == s.desplazaPropios {
			t.Fatal("una rejilla desplazable queda anidada bajo el scroll del panel")
		}
	}
	s.Activar(len(s.resultados) - 1)
	fin := s.rejilla.Position().Y + s.celdas[len(s.celdas)-1].Position().Y + LadoCeldaEmoji - r.desplaza.Offset.Y
	if r.desplaza.Offset.Y <= 0 || fin < 0 || fin > r.desplaza.Size().Height+0.01 {
		t.Errorf("el último emoji queda fuera del alto visible: fin=%v, scroll=%v, alto=%v", fin, r.desplaza.Offset, r.desplaza.Size())
	}
}

func TestElSelectorEmojiMovilSeAjustaAlMenu(t *testing.T) {
	for _, ancho := range []float32{320, 375, 390} {
		for _, alto := range []float32{180, 640} {
			nombre := fmt.Sprintf("%gx%g", ancho, alto)
			t.Run(nombre, func(t *testing.T) {
				boton := ui.NuevoBoton("Emoji", nil)
				w := test.NewTempWindow(t, container.NewWithoutLayout(boton))
				w.SetPadded(false)
				w.Resize(fyne.NewSize(ancho, alto))
				boton.Move(fyne.NewPos(20, alto-50))
				boton.Resize(boton.MinSize())
				s := NuevoSelectorEmoji(traductor(t), nil)
				m := ui.NuevoMenu(s)
				m.ContenidoAjustable = true
				m.Mostrar(boton)
				t.Cleanup(m.Cerrar)
				if m.Position().X < 0 || m.Position().Y < 0 || m.Position().X+m.Size().Width > ancho || m.Position().Y+m.Size().Height > alto {
					t.Fatalf("el selector sale del área visible: %v/%v", m.Position(), m.Size())
				}
				if s.Size().Height >= m.Size().Height {
					t.Fatal("el selector conserva su mínimo de escritorio bajo el borde del menú")
				}
				if s.rejilla.MinSize().Width > s.Size().Width {
					t.Fatal("las ocho columnas quedan cortadas a la derecha")
				}
				guardarEmoji(t, "selector-"+nombre, w.Canvas().Capture())
			})
		}
	}
}

func guardarEmoji(t *testing.T, nombre string, img image.Image) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURA_EMOJIS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, nombre+".png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, img)
	if cerrar := f.Close(); err != nil || cerrar != nil {
		t.Fatalf("guardar emoji: %v / %v", err, cerrar)
	}
}
