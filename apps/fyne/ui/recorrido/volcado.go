// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recorrido

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
)

// Pintado es un trazo de color que el lienzo pinta: el `getComputedStyle` de Fyne. Lo
// que `test/e2e/medidas-web.mjs` saca del DOM, sacado del árbol de objetos, para poner
// los dos inventarios uno al lado del otro con el MISMO criterio.
type Pintado struct {
	// Prop es «fondo», «borde» o «texto», como en la sonda de la web.
	Prop string `json:"prop"`
	// Color en hex, con alfa si no es opaco.
	Color string `json:"color"`
	// Dueno son los tres widgets más cercanos que lo contienen, del más próximo al más
	// lejano: con eso se sabe de qué pieza es sin adivinar.
	Dueno []string `json:"dueno"`
	// X, Y, W, H es la caja en el lienzo.
	X, Y, W, H float32
}

// Volcar escribe en `ruta` todo lo que se pinta con color en el lienzo, capas encima
// incluidas. Solo los objetos visibles y con área.
func Volcar(c fyne.Canvas, ruta string) error {
	v := &volcado{img: c.Capture()}
	v.ir(c.Content(), fyne.Position{}, nil)
	for _, o := range c.Overlays().List() {
		v.ir(o, fyne.Position{}, []string{"capa"})
	}
	return escribir(ruta, v.out)
}

// volcado es el recorrido de Volcar: la captura, para leer la tinta de los iconos, y lo
// apuntado.
type volcado struct {
	img image.Image
	out []Pintado
}

func (v *volcado) ir(o fyne.CanvasObject, base fyne.Position, duenos []string) {
	if o == nil || !o.Visible() {
		return
	}
	st := base.Add(o.Position())
	if _, linea := o.(*canvas.Line); !linea && (o.Size().Width <= 0 || o.Size().Height <= 0) {
		return
	}
	v.pintados(o, st, duenos)
	if _, es := o.(fyne.Widget); es {
		duenos = append(duenos[:len(duenos):len(duenos)], nombre(o))
	}
	for _, h := range hijos(o) {
		v.ir(h, st, duenos)
	}
}

// pintados apunta los colores del objeto: relleno, trazo, texto o la tinta de un icono.
func (v *volcado) pintados(o fyne.CanvasObject, st fyne.Position, duenos []string) {
	switch c := o.(type) {
	case *canvas.Rectangle:
		v.anota("fondo", c.FillColor, st, o, duenos)
		if c.StrokeWidth > 0 {
			v.anota("borde", c.StrokeColor, st, o, duenos)
		}
	case *canvas.Circle:
		v.anota("fondo", c.FillColor, st, o, duenos)
		if c.StrokeWidth > 0 {
			v.anota("borde", c.StrokeColor, st, o, duenos)
		}
	case *canvas.Line:
		v.anota("borde", c.StrokeColor, st, o, duenos)
	case *canvas.Text:
		v.anota("texto", c.Color, st, o, duenos)
	case *canvas.Raster:
		if v.img == nil {
			return
		}
		// Los iconos son mapas de bits teñidos: el color no está en el objeto, así que
		// se apunta la caja y la tinta se lee de la captura (ver `tintaDe`).
		if t := o.Size(); t.Width <= 32 && t.Height <= 32 {
			if n, ok := tintaDe(v.img, st, t); ok {
				v.anota("icono", n, st, o, duenos)
			}
		}
	}
}

func (v *volcado) anota(prop string, col color.Color, st fyne.Position, o fyne.CanvasObject, duenos []string) {
	n := nrgba(col)
	if n.A == 0 {
		return
	}
	d := duenos
	if len(d) > 3 {
		d = d[len(d)-3:]
	}
	inv := make([]string, len(d))
	for i := range d {
		inv[i] = d[len(d)-1-i]
	}
	tam := o.Size()
	v.out = append(v.out, Pintado{Prop: prop, Color: hex(n), Dueno: inv, X: st.X, Y: st.Y, W: tam.Width, H: tam.Height})
}

// nrgba es el color en NRGBA; nil es transparente.
func nrgba(c color.Color) color.NRGBA {
	if c == nil {
		return color.NRGBA{}
	}
	n, _ := color.NRGBAModel.Convert(c).(color.NRGBA)
	return n
}

func escribir(ruta string, datos any) error {
	b, err := json.Marshal(datos)
	if err != nil {
		return err
	}
	return os.WriteFile(ruta, b, 0o600) //nolint:gosec // la ruta la pone quien lanza el test
}

// tintaDe es la tinta de un icono en la captura: el color que más se aleja del fondo de
// su caja y que se repite, que en un trazo de 1,5–2 px a ×1 son los píxeles del centro,
// ya sin antialias. ok es falso si la caja es lisa.
func tintaDe(img image.Image, p fyne.Position, t fyne.Size) (color.NRGBA, bool) {
	r := image.Rect(int(p.X+0.5), int(p.Y+0.5), int(p.X+t.Width+0.5), int(p.Y+t.Height+0.5)).Intersect(img.Bounds())
	cuenta := map[color.NRGBA]int{}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cuenta[nrgba(img.At(x, y))]++
		}
	}
	var fondo color.NRGBA
	for c, n := range cuenta {
		if n > cuenta[fondo] {
			fondo = c
		}
	}
	dist := func(a, b color.NRGBA) int {
		d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
		return d(a.R, b.R) + d(a.G, b.G) + d(a.B, b.B)
	}
	var tinta color.NRGBA
	lejos := 0
	for c, n := range cuenta {
		if d := dist(c, fondo); n >= 2 && d > lejos {
			tinta, lejos = c, d
		}
	}
	return tinta, lejos > 24
}

func hex(c color.NRGBA) string {
	const d = "0123456789abcdef"
	b := []byte{'#', d[c.R>>4], d[c.R&15], d[c.G>>4], d[c.G&15], d[c.B>>4], d[c.B&15]}
	if c.A != 0xff {
		b = append(b, d[c.A>>4], d[c.A&15])
	}
	return string(b)
}

// Encima es lo que cambia de color en un widget al pasar el ratón: el `hover:` de la web.
type Encima struct {
	Tipo   string   `json:"tipo"`
	Texto  string   `json:"texto"`
	Nuevos []string `json:"nuevos"` // colores que aparecen al entrar el ratón
	Idos   []string `json:"idos"`   // y los que se van
}

// VolcarEncima pasa el ratón por cada widget que lo atiende —el primero de cada tipo— y
// apunta qué colores cambian, ya acabada la transición. Sin posición ni capas: solo el
// subárbol del widget.
func VolcarEncima(c fyne.Canvas, ruta string) error {
	var out []Encima
	vistos := map[string]bool{}
	var ir func(o fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		if o == nil || !o.Visible() {
			return
		}
		if h, es := o.(desktop.Hoverable); es && !vistos[nombre(o)] {
			vistos[nombre(o)] = true
			if e := encima(o, h); len(e.Nuevos)+len(e.Idos) > 0 {
				out = append(out, e)
			}
		}
		for _, h := range hijos(o) {
			ir(h)
		}
	}
	ir(c.Content())
	return escribir(ruta, out)
}

// encima es lo que cambia en `o` entre el reposo y el ratón encima.
func encima(o fyne.CanvasObject, h desktop.Hoverable) Encima {
	antes := colores(o)
	h.MouseIn(&desktop.MouseEvent{})
	esperar(300 * time.Millisecond) // las transiciones de color duran 150
	despues := colores(o)
	h.MouseOut()
	esperar(300 * time.Millisecond)
	e := Encima{Tipo: nombre(o), Texto: describir(o)}
	for col := range despues {
		if !antes[col] {
			e.Nuevos = append(e.Nuevos, col)
		}
	}
	for col := range antes {
		if !despues[col] {
			e.Idos = append(e.Idos, col)
		}
	}
	return e
}

// colores son los «prop color» que pinta el subárbol de o: el mismo recorrido que
// Volcar, sin captura (y por eso sin la tinta de los iconos).
func colores(o fyne.CanvasObject) map[string]bool {
	v := &volcado{}
	v.ir(o, fyne.Position{}, nil)
	m := map[string]bool{}
	for _, p := range v.out {
		m[p.Prop+" "+p.Color] = true
	}
	return m
}
