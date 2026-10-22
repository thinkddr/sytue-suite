// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/fuentes"
)

// SelectorAvatar es AvatarPicker.tsx: el avatar con el botón de la cámara en su esquina,
// que despliega los colores predefinidos, «Subir una foto…» y, si la hay, «Quitar foto».
//
//	botón   `absolute -right-1 -bottom-1 h-7 w-7 rounded-full border-2 border-surface  hoja
//	        bg-surface text-text-muted shadow-xs`, la cámara de 14 (medido: 24,5)
//	menú    `w-56 p-2.5 flex-col gap-2`: «Colores predefinidos» `text-xs font-medium   hoja
//	        text-muted px-0.5`, la rejilla `grid-cols-6 gap-1.5` de círculos `h-7 w-7`
//	        con `ring-1 ring-inset` del borde, un `hr my-0.5` y las dos acciones
//	        `px-2 py-1.5 text-sm rounded-md`, la de quitar en `danger-text`
//
// Qué hace cada una lo decide la app: aquí solo se elige.
type SelectorAvatar struct {
	Pulsable

	Avatar *Avatar
	// TieneFoto cambia el rótulo del botón y enseña «Quitar foto».
	TieneFoto bool
	// Los rótulos, YA traducidos.
	RotuloCambiar, RotuloAnadir, RotuloColores, RotuloSubir, RotuloQuitar string

	// AlElegirColor recibe el token del color elegido (`avatar-1`…`avatar-6`).
	AlElegirColor func(token string)
	// AlSubir abre el selector de ficheros. nil: «Subir una foto…» sale apagada.
	AlSubir func()
	// AlQuitar quita la foto.
	AlQuitar func()
}

// ColoresAvatar son los seis de PRESET_AVATAR_COLORS, por token.
var ColoresAvatar = []string{"avatar-1", "avatar-2", "avatar-3", "avatar-4", "avatar-5", "avatar-6"}

// Las medidas del selector.
var (
	LadoBotonCamara   = tema.Espacio(7)   // h-7 w-7
	FueraBotonCamara  = tema.Espacio(1)   // -right-1 -bottom-1
	anchoMenuAvatar   = tema.Espacio(56)  // w-56
	rellenoMenuAvatar = tema.Espacio(2.5) // p-2.5
	huecoMenuAvatar   = tema.Espacio(2)   // gap-2
	huecoColores      = tema.Espacio(1.5) // gap-1.5
)

// LadoIconoCamara es el `<Icon name="camera" size={14}>`.
const LadoIconoCamara float32 = 14

// NuevoSelectorAvatar monta el selector sobre un avatar ya hecho.
func NuevoSelectorAvatar(av *Avatar) *SelectorAvatar {
	s := &SelectorAvatar{Avatar: av}
	s.AlPulsar = s.abrir
	s.ExtendBaseWidget(s)
	return s
}

// AccessibilityLabel es «Cambiar foto» o «Añadir foto», que es lo que hace el botón.
func (s *SelectorAvatar) AccessibilityLabel() string {
	if s.TieneFoto {
		return s.RotuloCambiar
	}
	return s.RotuloAnadir
}

// Tapped abre el menú solo si el toque cae en el botón de la cámara: el avatar no se
// pulsa, como en la web.
func (s *SelectorAvatar) Tapped(e *fyne.PointEvent) {
	lado := MedidaAvatar(s.Avatar.Talla).Width
	x0 := lado + FueraBotonCamara - LadoBotonCamara
	if EsTactil() {
		// Con el dedo, el blanco es de 44 (`pointer-coarse:after:-inset-2`): 8 más
		// por cada lado del botón de 28, hacia dentro del avatar.
		x0 -= (SueloTactilBarra - LadoBotonCamara) / 2
	}
	if e == nil || (e.Position.X >= x0 && e.Position.Y >= x0) {
		s.Pulsable.Tapped(e)
	}
}

// abrir despliega el menú colgando del botón de la cámara.
func (s *SelectorAvatar) abrir() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var m *Menu
	colores := make([]fyne.CanvasObject, 0, len(ColoresAvatar))
	for _, token := range ColoresAvatar {
		tok := token
		c := &muestraAvatar{Token: tok}
		c.Nombre = s.RotuloColores
		c.AlPulsar = func() {
			m.Cerrar()
			if s.AlElegirColor != nil {
				s.AlElegirColor(tok)
			}
		}
		c.ExtendBaseWidget(c)
		colores = append(colores, c)
	}
	subir := NuevoElementoMenu(s.RotuloSubir, iconos.Ninguno, s.AlSubir)
	subir.Denso, subir.Inerte = true, s.AlSubir == nil
	hijos := []fyne.CanvasObject{
		container.New(Margen{Izq: tema.Espacio(0.5), Der: tema.Espacio(0.5)},
			NuevoRotulo(s.RotuloColores, tema.TextoXS, Medio, "text-muted")),
		container.New(Fila{Hueco: huecoColores}, colores...),
		fileteMenuAvatar(tema.Color("border", v)),
		subir,
	}
	if s.TieneFoto && s.AlQuitar != nil {
		quitar := NuevoElementoMenu(s.RotuloQuitar, iconos.Ninguno, s.AlQuitar)
		quitar.Denso, quitar.Peligro = true, true
		hijos = append(hijos, quitar)
	}
	dentro := container.New(Margen{Izq: rellenoMenuAvatar, Arr: rellenoMenuAvatar, Der: rellenoMenuAvatar, Abj: rellenoMenuAvatar},
		container.New(&anchoFijo{Ancho: anchoMenuAvatar - 2*rellenoMenuAvatar}, container.New(Columna{Hueco: huecoMenuAvatar}, hijos...)))
	m = NuevoMenu(dentro)
	for _, h := range hijos {
		if e, es := h.(*ElementoMenu); es {
			e.menu = m
		}
	}
	m.Rotulo, m.AlInicio = s.AccessibilityLabel(), true
	m.Mostrar(s)
}

// fileteMenuAvatar es el `hr my-0.5`.
func fileteMenuAvatar(c color.NRGBA) fyne.CanvasObject {
	r := canvas.NewRectangle(c)
	r.SetMinSize(fyne.NewSize(0, tema.Borde))
	return container.New(Margen{Arr: tema.Espacio(0.5), Abj: tema.Espacio(0.5)}, r)
}

// anchoFijo le da a su hijo ese ancho: el `w-56` del contenido del menú.
type anchoFijo struct{ Ancho float32 }

func (a *anchoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var alto float32
	for _, o := range objs {
		alto = max(alto, o.MinSize().Height)
	}
	return fyne.NewSize(a.Ancho, alto)
}

func (a *anchoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// CreateRenderer compone el avatar, la sombra y el botón. Termina en Refresh.
func (s *SelectorAvatar) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &selectorAvatarRenderer{
		s:      s,
		sombra: NuevaSombra(tema.SombraXS, tema.Forma("radius-control")),
		// `border-2 border-surface` y `bg-surface`: una caja con la esquina del botón.
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		camara: iconos.Trazo(iconos.Camara, LadoIconoCamara, "text-muted", v),
		aro:    NuevoAro(tema.Forma("radius-control"), SinHueco, v),
	}
	r.Refresh()
	return r
}

type selectorAvatarRenderer struct {
	s      *SelectorAvatar
	sombra *Sombra
	fondo  *canvas.Rectangle
	camara *iconos.Dibujo
	aro    *Aro
}

func (r *selectorAvatarRenderer) MinSize() fyne.Size { return MedidaAvatar(r.s.Avatar.Talla) }

func (r *selectorAvatarRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.s.Avatar, r.sombra.Objeto()},
		r.aro.Antes(r.fondo, r.camara.Objeto())...)
}

func (r *selectorAvatarRenderer) Destroy() {}

func (r *selectorAvatarRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	lado := MedidaAvatar(r.s.Avatar.Talla)
	r.s.Avatar.Move(fyne.NewPos(0, 0))
	r.s.Avatar.Resize(lado)
	pos := fyne.NewPos(lado.Width+FueraBotonCamara-LadoBotonCamara, lado.Height+FueraBotonCamara-LadoBotonCamara)
	tam := fyne.NewSize(LadoBotonCamara, LadoBotonCamara)
	r.sombra.Ajustar(pos, tam, v)
	r.fondo.Move(pos)
	r.fondo.Resize(tam)
	r.aro.ColocarEn(pos, tam)
	r.camara.Colocar(pos.X+(tam.Width-LadoIconoCamara)/2, pos.Y+(tam.Height-LadoIconoCamara)/2, LadoIconoCamara)
}

func (r *selectorAvatarRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	radio := tema.Forma("radius-control")
	r.fondo.CornerRadius = radio
	r.sombra.CambiarRadio(radio)
	r.aro.CambiarRadio(radio)
	tinta := "text-muted"
	if r.s.Encima() {
		tinta = "text"
	}
	r.camara.Tenir(tinta, v)
	r.aro.Pintar(v)
	r.aro.Ver(r.s.Enfocado())
	r.fondo.Refresh()
	r.s.Avatar.Refresh()
	r.Layout(r.s.Size())
}

// muestraAvatar es un círculo de color de la rejilla: `h-7 w-7 rounded-full ring-1
// ring-inset ring-border`.
type muestraAvatar struct {
	Pulsable
	Token string
}

func (m *muestraAvatar) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &muestraRenderer{
		m:       m,
		anillo:  canvas.NewCircle(tema.Color("border", v)),
		relleno: canvas.NewCircle(tema.Color(m.Token, v)),
		aro:     NuevoAro(LadoBotonCamara/2, SinHueco, v),
	}
	r.Refresh()
	return r
}

type muestraRenderer struct {
	m               *muestraAvatar
	anillo, relleno *canvas.Circle
	aro             *Aro
}

func (r *muestraRenderer) MinSize() fyne.Size {
	return fyne.NewSize(LadoBotonCamara, LadoBotonCamara)
}

func (r *muestraRenderer) Objects() []fyne.CanvasObject {
	return r.aro.Antes(r.relleno, r.anillo)
}
func (r *muestraRenderer) Destroy() {}

func (r *muestraRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.relleno.Resize(t)
	// El trazo va centrado en el contorno: encogido medio píxel por lado, queda entero
	// por dentro, que es el `ring-inset`.
	r.anillo.Move(fyne.NewPos(tema.Borde/2, tema.Borde/2))
	r.anillo.Resize(fyne.NewSize(t.Width-tema.Borde, t.Height-tema.Borde))
}

func (r *muestraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.relleno.FillColor = tema.Color(r.m.Token, v)
	// `ring-1 ring-inset`: un trazo de 1 por DENTRO del círculo.
	r.anillo.FillColor = color.NRGBA{}
	r.anillo.StrokeColor, r.anillo.StrokeWidth = tema.Color("border", v), tema.Borde
	r.aro.Pintar(v)
	r.aro.Ver(r.m.Enfocado())
	r.relleno.Refresh()
	r.anillo.Refresh()
}

// ---- El avatar predefinido ----

// LadoAvatarPredefinido es AVATAR_SIDE: el cuadrado de 256 que se sube como foto.
const LadoAvatarPredefinido = 256

// AvatarPredefinido es `presetAvatarDataUri`: las iniciales en blanco, semi-negritas al
// 40 % del lado, sobre ese color, como PNG. Es la misma clase de imagen que una foto
// subida, para que el resto del circuito no tenga que distinguirlas.
//
// ponytail: la web lo escribe con `system-ui`, que es una letra distinta en cada
// sistema; aquí va en Figtree, la de la suite.
func AvatarPredefinido(nombre string, fondo color.NRGBA) ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, LadoAvatarPredefinido, LadoAvatarPredefinido))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: fondo}, image.Point{}, draw.Src)
	fuente, err := opentype.Parse(fuentes.Leer("Figtree-SemiBold.ttf"))
	if err != nil {
		return nil, err
	}
	cara, err := opentype.NewFace(fuente, &opentype.FaceOptions{
		Size: 0.4 * LadoAvatarPredefinido, DPI: 72, Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cara.Close() }()
	texto := Iniciales(nombre)
	d := &font.Drawer{Dst: img, Src: image.White, Face: cara}
	ancho := d.MeasureString(texto)
	m := cara.Metrics()
	// `textBaseline = 'middle'` y 2 px más abajo: el centro de la caja de la letra
	// en la mitad del cuadrado.
	medio := fixed.I(LadoAvatarPredefinido/2 + 2)
	d.Dot = fixed.Point26_6{
		X: (fixed.I(LadoAvatarPredefinido) - ancho) / 2,
		Y: medio + (m.Ascent-m.Descent)/2,
	}
	d.DrawString(texto)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
