// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

// Package sytue is the optional host adapter. The independent compiler/runtime
// does not import it; the Sytue application supplies its existing measured widgets
// and patched font shaping explicitly. Existing proprietary UI code stays in ui.
package sytue

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	webui "github.com/thinkddr/astro-fyne/native"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Backend reuses Sytue's measured text placement, fonts and boxes. The shared
// renderer-only editing engine leaves focus and HTML commits on the generated View.
type Backend struct{}

var _ webui.Backend = Backend{}

func (Backend) Defaults(n webui.Node) webui.Style {
	s := (webui.FyneBackend{}).Defaults(n)
	s.FontFamily, s.FontSize, s.LineHeight = "Figtree", tema.TextoBase.Tam, tema.TextoBase.Linea
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	colorToken := "text"
	if n.Kind == "button" {
		s.Height, s.Radius = tema.Espacio(9), tema.RadioMD
		s.PaddingLeft, s.PaddingRight = tema.Espacio(4), tema.Espacio(4)
		background := "primary"
		colorToken = "on-primary"
		switch n.Variant {
		case "secondary":
			background, colorToken = "surface", "text"
		case "ghost":
			background, colorToken = "", "text"
		case "danger":
			// Blanco: `on-primary` es el rótulo del primario y en oscuro es casi negro.
			background, colorToken = "danger", "neutral-0"
		}
		if background == "" {
			s.Background = "transparent"
		} else {
			s.Background = css(tema.Color(background, v))
		}
		if n.Variant == "secondary" {
			s.BorderColor = css(tema.Color("border-strong", v))
		}
	}
	if n.Kind == "input" || n.Kind == "textarea" {
		s.Background, s.BorderColor, s.Radius = css(tema.Color("surface", v)), css(tema.Color("border-strong", v)), tema.RadioMD
		s.PaddingLeft, s.PaddingRight = tema.Espacio(3), tema.Espacio(3)
		if n.Kind == "input" {
			s.Height = tema.Espacio(9)
		}
	}
	s.Color = css(tema.Color(colorToken, v))
	return s
}

func (Backend) Validate(s webui.Style, text bool) error {
	if !text {
		return nil
	}
	if s.Measured && (s.FontSize <= 0 || s.LineHeight <= 0) {
		return fmt.Errorf("measured text requires explicit positive font size and line height")
	}
	family := firstFamily(s)
	if family != "Figtree" && family != "JetBrains Mono" {
		return fmt.Errorf("sytue backend has no font resource for %q", family)
	}
	if s.FontWeight != 400 && s.FontWeight != 500 && s.FontWeight != 600 && s.FontWeight != 700 {
		return fmt.Errorf("sytue backend has no font weight %v", s.FontWeight)
	}
	if s.FontStyle == "italic" && s.FontWeight != 400 {
		return fmt.Errorf("sytue backend currently supports regular italic only")
	}
	if family == "JetBrains Mono" && (s.FontWeight != 400 || s.FontStyle == "italic") {
		return fmt.Errorf("sytue backend only supplies regular monospace")
	}
	return nil
}

func escalon(s webui.Style) tema.Escalon { return tema.Escalon{Tam: s.FontSize, Linea: s.LineHeight} }

func firstFamily(s webui.Style) string {
	return strings.Trim(strings.TrimSpace(strings.Split(s.FontFamily, ",")[0]), "\"'")
}

func peso(s webui.Style) ui.Peso {
	if firstFamily(s) == "JetBrains Mono" {
		return ui.Mono
	}
	if s.FontStyle == "italic" {
		return ui.Cursiva
	}
	switch s.FontWeight {
	case 500:
		return ui.Medio
	case 600:
		return ui.SemiNegrita
	case 700:
		return ui.Negrita
	default:
		return ui.Normal
	}
}

func (Backend) Measure(text string, s webui.Style) fyne.Size {
	return ui.Medir(text, escalon(s), peso(s))
}

func (Backend) Text(text string, s webui.Style) *canvas.Text {
	c, _ := webui.ParseColor(s.Color)
	return ui.Texto(text, escalon(s), peso(s), c)
}

func (Backend) PlaceText(t *canvas.Text, s webui.Style, x, y float32) {
	t.FontSource = peso(s).Cara()
	ui.ColocarTexto(t, escalon(s), x, y)
}

func (Backend) Box(s webui.Style) webui.Box {
	background, _ := webui.ParseColor(s.Background)
	border, _ := webui.ParseColor(s.BorderColor)
	if background.A != 255 && s.BorderWidth > 0 {
		return (webui.FyneBackend{}).Box(s)
	}
	if s.BorderWidth == 0 {
		border.A = 0
	}
	return suiteBox{ui.NuevaCaja(background, border, s.Radius).ConBorde(s.BorderWidth)}
}

type suiteBox struct{ box *ui.Caja }

func (b suiteBox) Objects() []fyne.CanvasObject { return b.box.Objetos() }
func (b suiteBox) Resize(size fyne.Size)        { b.box.Redimensionar(size) }

func (Backend) Editor(multiline bool, s webui.Style, onChange func(string)) webui.Editor {
	return webui.NewEditor(Backend{}, multiline, s, onChange)
}

func css(c color.NRGBA) string {
	return fmt.Sprintf("rgba(%d, %d, %d, %g)", c.R, c.G, c.B, float64(c.A)/255)
}
