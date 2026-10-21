// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image"
	"image/color"
	"math"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Las variantes que sirve syphotos. La cuadrícula usa la primera y el visor la última.
const (
	VarianteMiniatura = "miniatura"
	VarianteGrande    = "grande"
)

// Las medidas de la tesela, de `Miniatura()` en PhotosApp.tsx. Son literales de la
// web: `rounded-sm` es 4, el círculo de selección es `h-6 w-6`, el corazón lleva `p-1`
// y los dos van a `1` de cada borde.
var radioTesela = tema.RadioSM

const (
	ladoMarca    = 6 * 0.25 * tema.Rem
	margenMarca  = 1 * 0.25 * tema.Rem
	iconoTesela  = float32(16)
	rellenoCoraz = 1 * 0.25 * tema.Rem
	rellenoAprox = 1 * 0.25 * tema.Rem
	textoAproxPx = float32(10) // `text-[10px]`, literal en la web
)

// text-[10px] cambia el tamaño, pero hereda la línea del tema; no lleva leading.
var lineaAproxPx = tema.TextoRaiz.Linea

// Miniatura es la tesela de las filas justificadas: la foto en la caja que le da la
// fila —con su proporción, así que no se recorta salvo que sea extrema— y, encima, los
// mandos que la web enseña al pasar por encima.
//
// Lleva cáscara de widget —y no es un contenedor— porque RECIBE EL CLIC y SE ANUNCIA:
// en la web es un `<button aria-label="Abrir …">`, o con `aria-pressed` cuando se está
// seleccionando. Un árbol de primitivas sueltas es decorado mudo.
type Miniatura struct {
	widget.BaseWidget

	Foto api.Photo
	// Rotulo es el `aria-label`: «Abrir X» o «Seleccionar X», ya traducido.
	Rotulo string
	// RotuloFavorita es el del corazón; vacío, no hay corazón (que es lo que pasa
	// cuando la web no le pasa `onFavorite`).
	RotuloFavorita string
	// Seleccionando es el modo selección: la tesela marca en vez de abrir.
	Seleccionando bool
	Seleccionada  bool
	// SinMarcas es la foto sola, sin la marca de fecha aproximada: la portada de un
	// álbum, las de «repetidas» y las de la cabecera de «parecidas», que en la web son
	// un `<img>` suelto y no la tesela de la galería.
	SinMarcas bool
	// Atenuada es el `opacity-70` de las fotos de la papelera.
	Atenuada bool
	// Filete es el `outline` de 1 px de `.filas > li img`: el borde sutil del tema
	// sobre la foto, para que una clara no desaparezca contra el fondo. Lo llevan las
	// teselas de las filas justificadas, no una portada ni una referencia.
	Filete bool

	AlPulsar   func()
	AlFavorita func()

	teca *Fototeca
	// muPintar serializa el `Refresh()` ENTERO. No es la misma precaución que
	// proteger los datos: `BaseWidget.Refresh` escribe `themeCache` sin sincronizar
	// (widget.go:125 de Fyne v2.8.1) y vuelve a entrar por `Objects()` y `Layout()`,
	// así que dos rutinas refrescando la misma tesela es carrera aunque no compartan
	// un solo campo. Es la misma que se llevó por delante `ui/aviso.go` (PR #771).
	//
	// Aquí hace falta de verdad y no por precaución: las miniaturas se preparan en
	// SEGUNDO PLANO y avisan al llegar, que es exactamente ese patrón. Y `fyne.Do` no
	// marshalla nada en el driver de pruebas —`DoFromGoroutine` llama a `f()` tal
	// cual—, así que sin esto el guardián lo vería y el binario no.
	muPintar         sync.Mutex
	encima, enfocado bool
	// anchoCSS es el ancho de la caja en px de CSS, que el GENERADOR necesita para
	// llevar el radio a píxeles de dispositivo. Va atómico y no bajo cerrojo porque
	// lo escribe el hilo que coloca y lo lee el que pinta, que en un binario de
	// verdad son dos.
	anchoCSS atomic.Uint32
}

var (
	_ fyne.Widget       = (*Miniatura)(nil)
	_ fyne.Tappable     = (*Miniatura)(nil)
	_ fyne.Focusable    = (*Miniatura)(nil)
	_ fyne.Accessible   = (*Miniatura)(nil)
	_ desktop.Hoverable = (*Miniatura)(nil)
)

// NuevaMiniatura crea la tesela de esa foto contra esa caché.
func NuevaMiniatura(f api.Photo, teca *Fototeca) *Miniatura {
	m := &Miniatura{Foto: f, teca: teca}
	m.ExtendBaseWidget(m)
	if teca != nil {
		teca.apuntar(f.ID, m.Refrescar)
	}
	return m
}

// AccessibilityRole: es un botón, como en la web.
func (m *Miniatura) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// AccessibilityLabel es el `aria-label` del botón.
func (m *Miniatura) AccessibilityLabel() string { return m.Rotulo }

// ladoToqueCorazon es el objetivo del corazón: el botón de la web lleva `p-1` alrededor
// del disco de 24 y va pegado a la esquina (`bottom-0 left-0`), así que mide 32.
//
// 32 y no 44: en un teléfono de 390 la fila más baja mide 98, el centro de la foto queda
// a 49 del borde y un objetivo de 44 llegaría a 5 px de él; en uno de 320 la fila mide
// 79 y lo pisaría. Quien toca la foto para abrirla la marcaría.
const ladoToqueCorazon = 8 * 0.25 * tema.Rem

// sinHover dice si aquí no hay «pasar por encima» (un teléfono): el corazón va entonces
// SIEMPRE a la vista, que es el `pointer-coarse:opacity-100` de la web. Un botón que no
// se ve y se pulsa le roba el toque a la foto. Es una variable por las pruebas.
var sinHover = ui.EsMovil

// corazonVisible dice si el corazón se ve: siempre en una favorita, al pasar por encima
// en las demás —o siempre, donde no hay puntero que pase— y nunca mientras se selecciona.
func (m *Miniatura) corazonVisible() bool {
	return m.RotuloFavorita != "" && !m.Seleccionando && (m.Foto.Favorite || m.encima || sinHover())
}

// Tapped abre la foto, o la marca si se está seleccionando. Quién es cada cosa lo
// decide quien monta la tesela, igual que en la web. Y si el toque cae en el corazón
// —el cuadrado de 32 de la esquina de abajo a la izquierda, cuando se ve—, marca o
// desmarca la favorita: en la web es un `<button>` encima del de la foto.
//
// Sin posición (Espacio o Intro) es la foto: el corazón de la web tiene su propia parada
// de tabulador, y aquí la favorita se cambia con el teclado desde el visor.
func (m *Miniatura) Tapped(ev *fyne.PointEvent) {
	if ev != nil && m.AlFavorita != nil && m.corazonVisible() &&
		ev.Position.X <= ladoToqueCorazon && ev.Position.Y >= m.Size().Height-ladoToqueCorazon {
		m.AlFavorita()
		return
	}
	if m.AlPulsar != nil {
		m.AlPulsar()
	}
}

// Refrescar es el Refresh de la tesela SERIALIZADO. Todo lo que repinte —el puntero,
// el foco y la miniatura que acaba de llegar— pasa por aquí.
func (m *Miniatura) Refrescar() {
	m.muPintar.Lock()
	defer m.muPintar.Unlock()
	m.Refresh()
}

// MouseIn y MouseOut son el `group-hover`: el corazón de una foto que NO es favorita, y
// la marca de fecha aproximada, solo se ven al pasar por encima.
func (m *Miniatura) MouseIn(*desktop.MouseEvent)    { m.encima = true; m.Refrescar() }
func (m *Miniatura) MouseMoved(*desktop.MouseEvent) {}
func (m *Miniatura) MouseOut()                      { m.encima = false; m.Refrescar() }

// FocusGained enciende el aro. Fyne solo da el foco por teclado, así que esto es el
// `:focus-visible` de la web y no el `:focus`.
func (m *Miniatura) FocusGained() { m.enfocado = true; m.Refrescar() }
func (m *Miniatura) FocusLost()   { m.enfocado = false; m.Refrescar() }

// TypedRune activa con Espacio y TypedKey con Intro, las dos teclas de un `<button>`.
func (m *Miniatura) TypedRune(r rune) {
	if r == ' ' {
		m.Tapped(nil)
	}
}

// TypedKey activa con Intro.
func (m *Miniatura) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(m, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		m.Tapped(nil)
	}
}

// MedidaMiniatura es el mínimo de la tesela SUELTA (una portada, una referencia): un
// cuadrado de 9rem. En las filas justificadas la caja se la da la fila.
func MedidaMiniatura() fyne.Size { return fyne.NewSize(9*tema.Rem, 9*tema.Rem) }

// CreateRenderer arma la tesela.
func (m *Miniatura) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &miniaturaRenderer{m: m}
	// EL HUECO SE RESERVA ANTES DE QUE LLEGUE LA IMAGEN: el fondo `bg-bg-subtle` con
	// el radio de la tesela. Sin esto la cuadrícula da saltos al desplazarse, que es
	// el defecto más visible de una galería.
	r.fondo = ui.NuevaCaja(tema.Color("bg-subtle", v), color.NRGBA{}, radioTesela)
	r.foto = canvas.NewRaster(r.generar)
	r.foto.ScaleMode = canvas.ImageScalePixels
	// El filete va ENCIMA de la foto y sin relleno: es el token crudo —su 6 % de
	// tinta—, no el aplanado sobre `surface`, porque lo de debajo es la foto.
	r.filete = canvas.NewRectangle(color.NRGBA{})
	r.filete.StrokeWidth, r.filete.CornerRadius = tema.Borde, radioTesela
	// `ring-2`: dos píxeles POR FUERA de la caja, no un borde. Es la misma pieza que
	// el anillo de foco con otro token, que es literalmente lo que hace la hoja.
	r.anillo = ui.NuevoAroCon("primary", radioTesela, ui.SinHueco, v)
	// `sytue-focus-ring-inset`: el aro lleva hueco para que se distinga del anillo de
	// una foto marcada, que es del mismo azul.
	r.aro = ui.NuevoAro(radioTesela, ui.ConHueco, v)
	r.marca = ui.NuevaCaja(negro50, blanco, tema.RadioPill)
	r.tic = iconos.Trazo(iconos.Tic, iconoTesela, "on-primary", v)
	r.fondoCoraz = ui.NuevaCaja(negro60, color.NRGBA{}, tema.Forma("radius-control"))
	r.corazon = iconos.TrazoConTinta(iconos.Corazon, iconoTesela, blanco)
	r.fondoAprox = ui.NuevaCaja(tema.Color(fondoAprox, v), color.NRGBA{}, tema.RadioSM)
	r.aprox = ui.Texto("~", escalonAprox, ui.Normal, tema.Color("text-subtle", v))
	r.rehacer()
	r.Refresh() // regla 7 de ui.go: sin esto la foto marcada nace sin su 70 %
	return r
}

var escalonAprox = tema.Escalon{Tam: textoAproxPx, Linea: lineaAproxPx}

// Los colores de lo que va SOBRE UNA FOTO, que en la web NO son tokens sino literales
// (`SOBRE_FOTO` en PhotosApp.tsx y el tono `media` de IconButton.tsx): lo de debajo no
// cambia con el tema. Negro al 60 % con la tinta en blanco da 5,7:1 sobre una foto
// blanca, el peor caso; con el 30 % y el 40 % de antes, 2,1:1 y 2,9:1. El círculo de
// selección sin marcar va al 50 % (`bg-black/50`). Todo lo demás de esta pantalla sí va
// por `tema.Color`.
var (
	negro50 = color.NRGBA{R: 0, G: 0, B: 0, A: 128} // 0,50 × 255 = 127,5 → 128
	negro60 = color.NRGBA{R: 0, G: 0, B: 0, A: 153} // 0,60 × 255
	negro75 = color.NRGBA{R: 0, G: 0, B: 0, A: 191} // `hover:bg-black/75`
	blanco  = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
)

// fondoAprox es el `bg-surface-raised` de la marca «~»: opaca en los tres temas.
// Era `bg-surface/80`, casi transparente en el traslúcido.
const fondoAprox = "surface-raised"

// conAlfa es el token con otra opacidad (`tokens.ConAlfa`).
func conAlfa(c color.NRGBA, f float32) color.NRGBA { return tokens.ConAlfa(c, float64(f)) }

type miniaturaRenderer struct {
	m *Miniatura

	fondo      *ui.Caja
	foto       *canvas.Raster
	filete     *canvas.Rectangle
	anillo     *ui.Aro
	aro        *ui.Aro
	marca      *ui.Caja
	tic        *iconos.Dibujo
	fondoCoraz *ui.Caja
	corazon    *iconos.Dibujo
	fondoAprox *ui.Caja
	aprox      *canvas.Text
	objetos    []fyne.CanvasObject
}

// rehacer arma la lista en el orden en que se pinta: los aros al fondo —la caja es
// opaca y les tapa lo que se solapa—, luego la foto y encima los mandos.
func (r *miniaturaRenderer) rehacer() {
	r.objetos = append([]fyne.CanvasObject{}, r.anillo.Objetos()...)
	r.objetos = append(r.objetos, r.aro.Objetos()...)
	r.objetos = append(r.objetos, r.fondo.Objetos()...)
	r.objetos = append(r.objetos, r.foto, r.filete)
	r.objetos = append(r.objetos, r.marca.Objetos()...)
	r.objetos = append(r.objetos, r.tic.Objeto())
	r.objetos = append(r.objetos, r.fondoCoraz.Objetos()...)
	r.objetos = append(r.objetos, r.corazon.Objeto())
	r.objetos = append(r.objetos, r.fondoAprox.Objetos()...)
	r.objetos = append(r.objetos, r.aprox)
}

// generar es lo que Fyne le pide al raster: el mapa de bits en píxeles de DISPOSITIVO.
// Aquí no se decodifica nada salvo la primera vez de cada tamaño; lo hace la Fototeca.
func (r *miniaturaRenderer) generar(w, h int) image.Image {
	vacio := image.NewRGBA(image.Rect(0, 0, w, h))
	if r.m.teca == nil {
		return vacio
	}
	// El radio va en píxeles de DISPOSITIVO: la caja se declara en px de CSS y el
	// raster se pide en los del dispositivo, así que se lleva con la misma razón. El
	// ancho de CSS se lee del atómico y NO de `r.foto.Size()`: esto corre en el hilo
	// que pinta y aquello lo escribe el que coloca.
	radio := float32(radioTesela)
	if caja := math.Float32frombits(r.m.anchoCSS.Load()); caja > 0 {
		radio *= float32(w) / caja
	}
	if m := r.m.teca.Tesela(r.m.Foto.ID, w, h, radio); m != nil {
		return m
	}
	return vacio
}

func (r *miniaturaRenderer) MinSize() fyne.Size           { return MedidaMiniatura() }
func (r *miniaturaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *miniaturaRenderer) Destroy()                     {}

// Layout coloca la foto y sus mandos.
func (r *miniaturaRenderer) Layout(t fyne.Size) {
	m := r.m
	r.anillo.Colocar(t)
	r.anillo.Ver(m.Seleccionada)
	r.aro.Colocar(t)
	r.aro.Ver(m.enfocado)

	ui.MoverCaja(r.fondo, fyne.NewPos(0, 0), t)
	// `h-full w-full object-cover`: la foto llena la caja que le da la fila.
	r.foto.Move(fyne.NewPos(0, 0))
	r.foto.Resize(t)
	m.anchoCSS.Store(math.Float32bits(t.Width))
	r.filete.Move(fyne.NewPos(0, 0))
	r.filete.Resize(t)
	if m.Filete {
		r.filete.Show()
	} else {
		r.filete.Hide()
	}

	// El círculo de selección, arriba a la izquierda.
	ui.MoverCaja(r.marca, fyne.NewPos(margenMarca, margenMarca), fyne.NewSize(ladoMarca, ladoMarca))
	verCaja(r.marca, m.Seleccionando)
	sangria := (ladoMarca - iconoTesela) / 2
	r.tic.Colocar(margenMarca+sangria, margenMarca+sangria, iconoTesela)
	r.tic.Ver(m.Seleccionando && m.Seleccionada)

	// El corazón, abajo a la izquierda: siempre si es favorita, al pasar por encima si
	// no lo es, y nunca mientras se selecciona.
	verCorazon := m.corazonVisible()
	lado := iconoTesela + 2*rellenoCoraz
	y := t.Height - margenMarca - lado
	ui.MoverCaja(r.fondoCoraz, fyne.NewPos(margenMarca, y), fyne.NewSize(lado, lado))
	verCaja(r.fondoCoraz, verCorazon)
	r.corazon.Colocar(margenMarca+rellenoCoraz, y+rellenoCoraz, iconoTesela)
	r.corazon.Ver(verCorazon)

	// La marca de «fecha aproximada», abajo a la derecha, al pasar por encima o con el
	// foco dentro (`group-hover` y `group-focus-within`).
	//
	// Sin pista, como la web: su `title={t('gallery.noExifDate')}` va en un `<span>`
	// con `pointer-events-none`, que nunca es el blanco del puntero, así que el
	// navegador no lo enseña; lo que asoma ahí es el `title` del botón de debajo.
	verAprox := !m.Foto.TakenFromEXIF && (m.encima || m.enfocado) && !m.SinMarcas
	anchoAprox := ui.Medir("~", escalonAprox, ui.Normal).Width + 2*rellenoAprox
	xa := t.Width - margenMarca - anchoAprox
	ya := t.Height - margenMarca - lineaAproxPx
	ui.MoverCaja(r.fondoAprox, fyne.NewPos(xa, ya), fyne.NewSize(anchoAprox, lineaAproxPx))
	verCaja(r.fondoAprox, verAprox)
	ui.ColocarTexto(r.aprox, escalonAprox, xa+rellenoAprox, ya)
	if verAprox {
		r.aprox.Show()
	} else {
		r.aprox.Hide()
	}
}

// Refresh repinta lo que depende del estado y REHACE la disposición: marcar una foto no
// cambia el tamaño de la tesela, y Fyne solo llama a Layout cuando el tamaño cambia.
func (r *miniaturaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.Pintar(tema.Color("bg-subtle", v), color.NRGBA{})
	r.anillo.Pintar(v)
	r.aro.Pintar(v)
	// El círculo marcado va en `primary`; sin marcar, negro al 50 %. El corazón, el
	// mismo negro al 60 % marcada o no (`SOBRE_FOTO`).
	if r.m.Seleccionada {
		r.marca.Pintar(tema.Color("primary", v), blanco)
	} else {
		r.marca.Pintar(negro50, blanco)
	}
	r.fondoCoraz.Pintar(negro60, color.NRGBA{})
	r.fondoAprox.Pintar(tema.Color(fondoAprox, v), color.NRGBA{})
	r.filete.StrokeColor = tokens.Crudo("border-subtle", v)
	// `opacity-70`: la foto marcada deja ver el `bg-bg-subtle` de debajo, igual que
	// las de la papelera. Es la mitad de cómo se ve de un vistazo qué va en el lote.
	r.foto.Translucency = 0
	if r.m.Seleccionada || r.m.Atenuada {
		r.foto.Translucency = 0.3
	}
	r.Layout(r.m.Size())
	r.foto.Refresh()
}

// verCaja enseña o esconde una Caja entera (sus uno o dos rectángulos).
func verCaja(c *ui.Caja, si bool) {
	for _, o := range c.Objetos() {
		if si {
			o.Show()
			continue
		}
		o.Hide()
	}
}
