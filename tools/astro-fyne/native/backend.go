// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package webui

import (
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// Backend supplies font shaping, boxes and editing to the generated native tree.
// The public default only depends on Fyne. A host with a measured font renderer may
// inject it without copying its design system into generated pages.
type Backend interface {
	Defaults(Node) Style
	Validate(Style, bool) error
	Measure(string, Style) fyne.Size
	Text(string, Style) *canvas.Text
	PlaceText(*canvas.Text, Style, float32, float32)
	Box(Style) Box
	Editor(bool, Style, func(string)) Editor
}

// Box paints a filled border and background rather than a centered stroke.
type Box interface {
	Objects() []fyne.CanvasObject
	Resize(fyne.Size)
}

// Editor is a native input engine, reused across generated state updates.
type Editor interface {
	Object() fyne.CanvasObject
	Text() string
	SetText(string)
	SetPlaceholder(string)
	SetDisabled(bool)
	SetStyle(Style)
	Layout(fyne.Position, fyne.Size)
	FocusGained()
	FocusLost()
	TypedRune(rune)
	TypedKey(*fyne.KeyEvent)
	TypedShortcut(fyne.Shortcut)
}

// Font selects a bundled font file. Measured text must have an explicit resource;
// substituting an operating system font would make a visual guarantee meaningless.
type Font struct {
	Weight int
	Italic bool
}

// FyneBackend is the independent public backend. Fonts belong to the host, which
// must supply licensed resources matching its browser font files. The default theme
// font is used for unmeasured prototypes only. Upstream Fyne's painter is not assumed
// to match Chromium; the visual gate decides whether a captured state passes.
type FyneBackend struct {
	Fonts map[string]map[Font]fyne.Resource
}

func (b FyneBackend) Defaults(n Node) Style {
	s := Style{FontSize: 14, LineHeight: 20, FontWeight: 400, Color: "#11232b", Opacity: 1}
	switch n.Kind {
	case "button":
		s.Height, s.PaddingLeft, s.PaddingRight, s.Radius, s.FontWeight, s.TextAlign = 36, 16, 16, 4, 600, "center"
		s.Background, s.Color = "#1769aa", "#ffffff"
		switch n.Variant {
		case "secondary":
			s.Background, s.Color, s.BorderColor, s.BorderWidth = "#ffffff", "#11232b", "#d1d5db", 1
		case "ghost":
			s.Background, s.Color = "transparent", "#11232b"
		case "danger":
			s.Background = "#b91c1c"
		}
	case "input", "textarea":
		s.Width, s.Height, s.PaddingLeft, s.PaddingRight, s.Radius = 200, 36, 12, 12, 4
		s.Background, s.BorderColor, s.BorderWidth = "#ffffff", "#d1d5db", 1
		if n.Kind == "textarea" {
			s.Height = 80
		}
	}
	return s
}

func family(s Style) string {
	return strings.Trim(strings.TrimSpace(strings.Split(s.FontFamily, ",")[0]), "\"'")
}
func (b FyneBackend) font(s Style) fyne.Resource {
	weight := int(s.FontWeight)
	if weight == 0 {
		weight = 400
	}
	for name, faces := range b.Fonts {
		if strings.EqualFold(name, family(s)) {
			return faces[Font{weight, s.FontStyle == "italic"}]
		}
	}
	return nil
}
func (b FyneBackend) Validate(s Style, text bool) error {
	if s.Measured && text && (s.FontSize <= 0 || s.LineHeight <= 0) {
		return fmt.Errorf("measured text requires explicit positive font size and line height")
	}
	if s.Measured && text && b.font(s) == nil {
		return fmt.Errorf("measured font %q weight %v style %q requires an explicit matching font resource", s.FontFamily, s.FontWeight, s.FontStyle)
	}
	return nil
}
func (b FyneBackend) textStyle(s Style) fyne.TextStyle {
	if b.font(s) != nil {
		return fyne.TextStyle{}
	}
	return fyne.TextStyle{Bold: s.FontWeight >= 600, Italic: s.FontStyle == "italic"}
}
func (b FyneBackend) Measure(text string, s Style) fyne.Size {
	size, _ := fyne.CurrentApp().Driver().RenderedTextSize(text, s.FontSize, b.textStyle(s), b.font(s))
	return fyne.NewSize(size.Width, s.LineHeight)
}
func (b FyneBackend) Text(text string, s Style) *canvas.Text {
	t := canvas.NewText(text, parseColor(s.Color))
	t.TextSize, t.FontSource, t.TextStyle = s.FontSize, b.font(s), b.textStyle(s)
	return t
}
func (b FyneBackend) PlaceText(t *canvas.Text, s Style, x, y float32) {
	t.FontSource, t.TextStyle = b.font(s), b.textStyle(s)
	font, baseline := fyne.CurrentApp().Driver().RenderedTextSize("H", s.FontSize, t.TextStyle, t.FontSource)
	// Blink's rounded ascent/descent and integer half-leading. The independent
	// painter may still quantize glyph origins differently; only a pixel diff proves
	// equivalence at the requested device scale.
	a := float32(math.Round(float64(baseline)))
	d := float32(math.Round(float64(font.Height - baseline)))
	lineBaseline := float32(math.Floor(float64(s.LineHeight-a-d)/2)) + a
	t.Move(fyne.NewPos(x, y+lineBaseline-baseline))
	t.Resize(t.MinSize())
}
func (b FyneBackend) Box(s Style) Box {
	box := &primitiveBox{fill: rounded(parseColor(s.Background), s.Radius), width: s.BorderWidth, radius: s.Radius, color: parseColor(s.BorderColor)}
	if box.width > 0 && box.color.A > 0 {
		box.border = canvas.NewRaster(box.paintBorder)
	}
	return box
}
func rounded(c color.NRGBA, radius float32) *canvas.Rectangle {
	r := canvas.NewRectangle(c)
	r.CornerRadius = radius
	return r
}

// The border is its own transparent raster with a hole. A centered stroke changes
// coverage, while a filled outer rectangle leaks through translucent interiors.
// Background and border therefore remain separate native canvas primitives.
type primitiveBox struct {
	fill          *canvas.Rectangle
	border        *canvas.Raster
	width, radius float32
	color         color.NRGBA
	size          fyne.Size
}

func (b *primitiveBox) Objects() []fyne.CanvasObject {
	if b.border == nil {
		return []fyne.CanvasObject{b.fill}
	}
	return []fyne.CanvasObject{b.fill, b.border}
}
func (b *primitiveBox) Resize(size fyne.Size) {
	b.size = fyne.NewSize(max(size.Width, 0), max(size.Height, 0))
	b.fill.Resize(b.size)
	if b.border != nil {
		b.border.Resize(b.size)
		b.border.Refresh()
	}
}
func (b *primitiveBox) paintBorder(width, height int) image.Image {
	out := image.NewNRGBA(image.Rect(0, 0, max(width, 0), max(height, 0)))
	if width <= 0 || height <= 0 || b.size.Width <= 0 || b.size.Height <= 0 {
		return out
	}
	w, h := float64(b.size.Width), float64(b.size.Height)
	stroke := float64(b.width)
	sx, sy := float64(width)/w, float64(height)/h
	scale := math.Min(sx, sy)
	outerRadius := math.Min(float64(b.radius), math.Min(w, h)/2)
	innerWidth, innerHeight := w-2*stroke, h-2*stroke
	innerRadius := math.Min(math.Max(float64(b.radius)-stroke, 0), math.Max(math.Min(innerWidth, innerHeight)/2, 0))
	for y := range height {
		for x := range width {
			px, py := (float64(x)+0.5)/sx, (float64(y)+0.5)/sy
			outer := coverage(roundDistance(px, py, w, h, outerRadius), scale)
			inner := float64(0)
			if innerWidth > 0 && innerHeight > 0 {
				inner = coverage(roundDistance(px-stroke, py-stroke, innerWidth, innerHeight, innerRadius), scale)
			}
			alpha := uint8(math.Round(math.Max(outer-inner, 0) * float64(b.color.A)))
			out.SetNRGBA(x, y, color.NRGBA{b.color.R, b.color.G, b.color.B, alpha})
		}
	}
	return out
}
func roundDistance(x, y, w, h, r float64) float64 {
	qx, qy := math.Abs(x-w/2)-(w/2-r), math.Abs(y-h/2)-(h/2-r)
	return math.Min(math.Max(qx, qy), 0) + math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) - r
}
func coverage(distance, scale float64) float64 { return math.Max(0, math.Min(1, 0.5-distance*scale)) }

// ParseColor understands the solid color serializations accepted by capture. The
// independent runtime has no token sheet or private design-system dependency.
func ParseColor(value string) (color.NRGBA, error) {
	s := strings.ToLower(strings.TrimSpace(value))
	switch s {
	case "", "transparent":
		return color.NRGBA{}, nil
	case "white":
		return color.NRGBA{255, 255, 255, 255}, nil
	case "black":
		return color.NRGBA{0, 0, 0, 255}, nil
	}
	if strings.HasPrefix(s, "#") {
		hexValue := s[1:]
		if len(hexValue) == 3 || len(hexValue) == 4 {
			var expanded strings.Builder
			for _, c := range hexValue {
				expanded.WriteRune(c)
				expanded.WriteRune(c)
			}
			hexValue = expanded.String()
		}
		if len(hexValue) == 6 || len(hexValue) == 8 {
			bytes, err := hex.DecodeString(hexValue)
			if err == nil {
				c := color.NRGBA{bytes[0], bytes[1], bytes[2], 255}
				if len(bytes) == 4 {
					c.A = bytes[3]
				}
				return c, nil
			}
		}
		return color.NRGBA{}, fmt.Errorf("unsupported hex color %q", value)
	}
	if (strings.HasPrefix(s, "rgb(") || strings.HasPrefix(s, "rgba(")) && strings.HasSuffix(s, ")") {
		fields := strings.FieldsFunc(s[strings.IndexByte(s, '(')+1:len(s)-1], func(c rune) bool { return c == ',' || c == ' ' || c == '/' })
		if len(fields) == 3 || len(fields) == 4 {
			channels := [4]uint8{0, 0, 0, 255}
			for i, field := range fields {
				percent := strings.HasSuffix(field, "%")
				n, err := strconv.ParseFloat(strings.TrimSuffix(field, "%"), 64)
				if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
					return color.NRGBA{}, fmt.Errorf("invalid color %q", value)
				}
				limit := float64(255)
				if i == 3 {
					limit = 1
				}
				if percent {
					limit = 100
				}
				if n < 0 || n > limit {
					return color.NRGBA{}, fmt.Errorf("color channel outside range in %q", value)
				}
				channels[i] = uint8(math.Round(n / limit * 255))
			}
			return color.NRGBA{channels[0], channels[1], channels[2], channels[3]}, nil
		}
	}
	return color.NRGBA{}, fmt.Errorf("unsupported solid color %q", value)
}
func parseColor(s string) color.NRGBA { c, _ := ParseColor(s); return c }
