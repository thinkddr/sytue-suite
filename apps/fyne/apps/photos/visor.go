// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image"
	"image/color"
	"math"
	"sync"
	"sync/atomic"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las medidas del visor, de `Visor()` en PhotosApp.tsx.
const (
	// La cabecera es `px-3 py-2` sobre una fila cuyo elemento más alto es un
	// `Button size="sm"` (`h-8`), más el `border-b`.
	RellenoVisorX = 3 * 0.25 * tema.Rem
	RellenoVisorY = 2 * 0.25 * tema.Rem
	HuecoVisor    = 2 * 0.25 * tema.Rem
	// HuecoMandos es el `gap-1` de la fila de mandos, que van en grupos con filetes.
	HuecoMandos    = 1 * 0.25 * tema.Rem
	LadoMandoVisor = 8 * 0.25 * tema.Rem
	// IconoMando es el `<Icon />` sin `size` que va dentro de cada `IconButton`: el 20
	// por defecto de Icon.tsx, no los 16 del botón.
	IconoMando    float32 = 20
	RellenoFicha          = 4 * 0.25 * tema.Rem   // `p-4` del aside
	AnchoFichaMin         = 240                   // FICHA_ANCHO.min
	AnchoFichaMax         = 720                   // FICHA_ANCHO.max
	AnchoFicha            = 340                   // FICHA_ANCHO.defecto
	AnchoAsaFicha         = 1.5 * 0.25 * tema.Rem // `w-1.5` (y `h-1.5` en móvil)
	AltoFichaMin          = 160                   // FICHA_ALTO.min
	AltoFichaMax          = 560                   // FICHA_ALTO.max
	AltoFicha             = 260                   // FICHA_ALTO.defecto
)

// Las dos formas del pase automático, de PhotosApp.tsx: son glifos RELLENOS y no
// están en el catálogo compartido.
const (
	glifoPlay  = "M7 5v14l12-7z"
	glifoPausa = "M7 5h4v14H7zm6 0h4v14h-4z"
)

// AltoCabeceraVisor es lo que ocupa la barra de mandos del visor, filete incluido.
func AltoCabeceraVisor() float32 {
	return ui.SueloTactil(LadoMandoVisor) + 2*RellenoVisorY + tema.Borde
}

// altoCabeceraVisor es la del visor a ese ancho de ventana: por debajo de `lg:` se parte
// en dos filas (el nombre y «Cerrar» arriba, los mandos debajo) con el `gap-2` entre
// ellas.
func altoCabeceraVisor(ventana float32) float32 {
	if ventana < ui.CorteLG {
		return AltoCabeceraVisor() + HuecoVisor + ui.SueloTactil(LadoMandoVisor)
	}
	return AltoCabeceraVisor()
}

// FondoLienzo es lo que se ve DETRÁS de la foto (fondoLienzo.ts): solo asoma en las
// transparentes, y por eso es una preferencia de la persona y no de la foto.
type FondoLienzo string

// Los cuatro, en el orden de FONDOS_DISPONIBLES.
const (
	FondoCrema  FondoLienzo = "crema"
	FondoBlanco FondoLienzo = "blanco"
	FondoOscuro FondoLienzo = "oscuro"
	FondoDamero FondoLienzo = "damero"
	// fondoSuelta no es de los que se eligen: es el del navegador cuando enseña una
	// imagen abierta sola (el documento de imagen de Chromium, `rgb(14, 14, 14)`), que es
	// lo que se ve al pulsar una foto del álbum compartido (VisorPublico).
	fondoSuelta FondoLienzo = "suelta"
)

// clave es el rótulo de cada fondo en el menú. Literal y no `"editor.bg_" + f`: una
// clave compuesta no la ve el guardián de i18n.
func (f FondoLienzo) clave() string {
	switch f {
	case FondoBlanco:
		return "editor.bg_blanco"
	case FondoOscuro:
		return "editor.bg_oscuro"
	case FondoDamero:
		return "editor.bg_damero"
	default:
		return "editor.bg_crema"
	}
}

// FondosLienzo son los cuatro, en el orden del menú.
var FondosLienzo = [...]FondoLienzo{FondoCrema, FondoBlanco, FondoOscuro, FondoDamero}

// colorFondo es el `backgroundColor` de `estilosFondoLienzo`: literales de la web, no
// tokens —son el color de un papel, no de la interfaz—.
func (f FondoLienzo) colorFondo() color.NRGBA {
	switch f {
	case FondoBlanco, FondoDamero:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case FondoOscuro:
		return color.NRGBA{R: 0x13, G: 0x16, B: 0x19, A: 255}
	case fondoSuelta:
		return color.NRGBA{R: 14, G: 14, B: 14, A: 255}
	case FondoCrema:
	}
	return color.NRGBA{R: 0xfa, G: 0xf9, B: 0xf6, A: 255}
}

// Visor son los datos del visor a pantalla completa. Los `Con…` son los `on…` que cada
// pantalla de la web le pasa o no: el álbum no ofrece borrar, «parecidas» no ofrece ni
// favorita ni describir.
type Visor struct {
	Foto api.Photo
	// Descripcion y Etiquetas son lo que Sytue AI vio en la foto.
	Descripcion string
	Etiquetas   []string
	// ConFicha enseña el panel de la derecha, y AnchoFicha lo mide (una preferencia de
	// ESTA pantalla en ESTE equipo, como en la web). En móvil va debajo y lo que se
	// ajusta es AltoFicha: compartir número daría una ficha de 600 de alto en un teléfono.
	ConFicha              bool
	AnchoFicha, AltoFicha float32
	// Pase es el pase automático, y TamanyoReal el zoom al 100 %, con Original, la foto
	// entera, si ya ha llegado.
	Pase        bool
	TamanyoReal bool
	Original    image.Image
	Fondo       FondoLienzo
	// Aviso es el `role="status"` de la cabecera.
	Aviso string
	// HayAnterior y HaySiguiente encienden las flechas.
	HayAnterior, HaySiguiente bool
	// Borrando enseña la banda de confirmación bajo la cabecera.
	Borrando bool
	// Girando y Describiendo inertizan su mando mientras dura la llamada.
	Girando, Describiendo bool

	ConFavorita, ConBorrar, ConDescribir bool
	// Accion es el mando extra de la cabecera («Quitar del álbum»).
	Accion fyne.CanvasObject

	// EditandoDesc cambia la descripción por un área de texto con Cancelar y Guardar, y
	// CampoDesc es esa área, que vive fuera por lo mismo que Pregunta. GuardandoDesc
	// inertiza los dos botones mientras dura la llamada.
	EditandoDesc, GuardandoDesc bool
	CampoDesc                   *AreaFicha

	// Pregunta es el campo de «Pregunta sobre esta foto», que vive fuera para no perder
	// lo escrito al repintar; Respuesta y Pensando, lo que dice debajo.
	Pregunta  *ui.Entrada
	Respuesta string
	Pensando  bool

	AlCerrar, AlAnterior, AlSiguiente func()
	AlFavorita, AlDescribir, AlGirar  func()
	AlPase, AlZoom, AlFicha           func()
	// AlZoomEn es el clic (o, con el dedo, el doble toque) en la foto: el tamaño real con
	// el punto pulsado —en fracciones de la foto— bajo el puntero, en `vista`.
	AlZoomEn func(fx, fy float32, vista fyne.Position)
	// Desplazamiento es por dónde va el recorrido a tamaño real, y AlDesplazar lo recibe:
	// el visor se rehace a cada cambio de estado y, sin él, volvía a la esquina.
	Desplazamiento                                  fyne.Position
	AlDesplazar                                     func(fyne.Position)
	AlBorrar, AlConfirmarBorrar, AlCancelarBorrar   func()
	AlParecidas, AlEditar, AlCerca, AlMapa, AlVerIA func()
	AlEditarDesc, AlCancelarDesc, AlGuardarDesc     func()
	AlDescargar                                     func(formato string)
	AlFondo                                         func(FondoLienzo)
	// AlMenuAlbumes pide la lista de álbumes (la web la pide al abrir el selector, no
	// al abrir el visor) y se la da a `mostrar` en el hilo de la interfaz.
	AlMenuAlbumes func(mostrar func([]api.Album))
	AlAnyadirA    func(api.Album)
	AlPreguntar   func(string)
	// AlAnchoFicha recibe el ancho nuevo de la ficha al arrastrar el asa o moverla con
	// las flechas; `guardar` es el final del gesto. AlAltoFicha, el alto en móvil.
	AlAnchoFicha, AlAltoFicha func(medida float32, guardar bool)
}

// PantallaVisor monta el visor. En la web es un `<dialog>` NATIVO a pantalla completa;
// aquí es una capa sobre la ventana entera, que es lo mismo sin el cromo del navegador.
func PantallaVisor(v Vista, d Visor) *fyne.Container {
	if d.AnchoFicha == 0 {
		d.AnchoFicha = AnchoFicha
	}
	if d.AltoFicha == 0 {
		d.AltoFicha = AltoFicha
	}
	if d.Fondo == "" {
		d.Fondo = FondoCrema
	}
	cabecera := cabeceraVisor(v, d)
	lienzo := NuevoLienzoVisor(v, d)
	zona := []fyne.CanvasObject{lienzo}
	anchoRotulo := float32(0)
	if d.Descripcion != "" && !d.TamanyoReal {
		r, ancho := rotuloDescripcion(v, d)
		// Sobre la foto, como las flechas: negro y blanco, sin desenfoque, sin borde y
		// sin sombra. Al 85 % un `backdrop-blur` no se ve y se paga en cada fotograma.
		zona = append(zona, r)
		anchoRotulo = ancho
	}
	var anterior, siguiente func()
	if d.HayAnterior {
		anterior = d.AlAnterior
	}
	if d.HaySiguiente {
		siguiente = d.AlSiguiente
	}
	if !d.TamanyoReal {
		// Encima de la foto: el toque amplía y el arrastre con el dedo pasa de foto
		// (`EscenarioVisor` de la web).
		zona = append([]fyne.CanvasObject{lienzo, NuevoDeslizaFoto(lienzo, anterior, siguiente, d.AlZoomEn)}, zona[1:]...)
	}
	centro := container.New(zonaVisor{lienzo: lienzo, real: d.TamanyoReal, ventana: v.Ventana, anchoRotulo: anchoRotulo}, zona...)
	cuerpo := escenario(v, centro, d.TamanyoReal, anterior, siguiente, d.Desplazamiento, d.AlDesplazar)
	var ficha, asa fyne.CanvasObject
	if d.ConFicha {
		ficha = FichaDelVisor(v, d)
		asa = NuevaAsaFicha(v, d)
	}
	var confirmar fyne.CanvasObject
	if d.Borrando && d.ConBorrar {
		confirmar = bandaBorrar(v, d)
	}
	fondo := canvas.NewRectangle(tema.Color("bg", v.Variante))
	filete := canvas.NewRectangle(tema.Color("border", v.Variante))
	piezas := []fyne.CanvasObject{fondo, cabecera, filete, cuerpo}
	for _, p := range []fyne.CanvasObject{confirmar, asa, ficha} {
		if p != nil {
			piezas = append(piezas, p)
		}
	}
	return container.New(visorLayout{d: d, ventana: v.Ventana}, piezas...)
}

// movil dice si la ventana está por debajo de `md:`, donde la ficha va debajo.
func movil(ventana float32) bool { return ventana < ui.CorteMD }

// cabeceraVisor es el `<header>`: el nombre, el aviso empujado a la derecha, los mandos
// y «Cerrar».
func cabeceraVisor(v Vista, d Visor) fyne.CanvasObject {
	nombre := ParrafoEn(d.Foto.Name, tema.TextoSM, ui.Medio, "text", v.Variante)
	aviso := ParrafoEn(d.Aviso, tema.TextoXS, ui.Normal, "text-muted", v.Variante)
	mandos := mandosVisor(v, d)
	// Cerrar es el aspa de cualquier diálogo de la casa, en la talla de los demás mandos.
	cerrar := mandoVisor(v, iconos.Cerrar, "viewer.close", d.AlCerrar)
	if v.Ventana >= ui.CorteLG {
		// `gap-2` entre el nombre, el aviso, el grupo de mandos y cerrar; dentro del
		// grupo, `gap-1`.
		grupo := container.New(ui.FilaAuto{Hueco: HuecoMandos}, mandos...)
		return container.New(ui.FilaAuto{Hueco: HuecoVisor, Derecha: 1}, nombre, aviso, grupo, cerrar)
	}
	// Por debajo de `lg:` la barra NO cabe en una línea: el nombre y cerrar arriba, y los
	// mandos en su propia fila,
	// que se desplaza de lado. La máscara de la derecha avisa de que hay más fuera de
	// vista, que en iOS no lo insinúa ninguna barra.
	fila := container.New(ui.FilaAuto{Hueco: HuecoMandos}, mandos...)
	// Sin barra (`[scrollbar-width:none]`): la máscara ya dice que hay más.
	desliza := container.NewThemeOverride(container.NewHScroll(fila), tema.SinBarras{})
	mascara := canvas.NewHorizontalGradient(conAlfa(tema.Color("bg", v.Variante), 0), tema.Color("bg", v.Variante))
	return container.New(cabeceraPartida{nombre: nombre, aviso: aviso, textoNombre: d.Foto.Name, textoAviso: d.Aviso},
		nombre, aviso, cerrar, desliza, mascara)
}

// cabeceraPartida es la cabecera del visor en dos filas. Arriba, el nombre en `min-w-0
// flex-1 truncate`, el aviso en `ml-auto truncate` y «Cerrar»; abajo, los mandos a todo
// el ancho con los últimos 28 px desvaneciéndose.
type cabeceraPartida struct {
	nombre, aviso *canvas.Text
	// Los textos enteros: Truncar escribe en el `canvas.Text`, y recolocar partiendo de
	// lo ya truncado iría comiéndose letras.
	textoNombre, textoAviso string
}

func (cabeceraPartida) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, 2*ui.SueloTactil(LadoMandoVisor)+HuecoVisor)
}

func (c cabeceraPartida) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	nombre, aviso := c.nombre, c.aviso
	cerrar, desliza, mascara := objs[2], objs[3], objs[4]
	lado := ui.SueloTactil(LadoMandoVisor)
	mc := cerrar.MinSize()
	cerrar.Move(fyne.NewPos(max(t.Width-mc.Width, 0), (lado-mc.Height)/2))
	cerrar.Resize(mc)
	libre := max(t.Width-mc.Width-2*HuecoVisor, 0)
	anchoAviso := min(ui.Medir(c.textoAviso, tema.TextoXS, ui.Normal).Width, libre)
	anchoNombre := ui.Truncar(nombre, c.textoNombre, tema.TextoSM, ui.Medio, libre-anchoAviso)
	ui.ColocarTexto(nombre, tema.TextoSM, 0, (lado-tema.TextoSM.Linea)/2)
	ui.Truncar(aviso, c.textoAviso, tema.TextoXS, ui.Normal, anchoAviso)
	ui.ColocarTexto(aviso, tema.TextoXS, max(t.Width-mc.Width-HuecoVisor-anchoAviso, anchoNombre+HuecoVisor),
		(lado-tema.TextoXS.Linea)/2)
	y := lado + HuecoVisor
	desliza.Move(fyne.NewPos(0, y))
	desliza.Resize(fyne.NewSize(t.Width, lado))
	const fundido = 28
	mascara.Move(fyne.NewPos(max(t.Width-fundido, 0), y))
	mascara.Resize(fyne.NewSize(min(fundido, t.Width), lado))
}

// mandosVisor son los mandos de la cabecera: UNA sola talla (el `IconButton size="lg"`,
// 32) y cuatro grupos separados por filetes. La acción extra; lo que es de ESTA FOTO
// (favorita, álbum, descargar, parecidas); lo que la CAMBIA (describir, girar); cómo se
// MIRA (pase, tamaño real, ficha, fondo); aparte, lo que la tira; y al final «Editar»,
// el único con rótulo. Eran tres rótulos de texto, nueve iconos de 24 y una papelera
// de 28: cuatro formas de botón en una fila.
func mandosVisor(v Vista, d Visor) []fyne.CanvasObject {
	var hijos []fyne.CanvasObject
	if d.Accion != nil {
		hijos = append(hijos, d.Accion, filete(v))
	}
	if d.ConFavorita {
		b := mandoVisor(v, iconos.Corazon, claveFavorita(d.Foto), d.AlFavorita)
		b.Pulsado = d.Foto.Favorite
		if d.Foto.Favorite {
			b.Glifo = iconos.TrazoLleno(iconos.Corazon, IconoMando, "text", v.Variante)
		}
		hijos = append(hijos, b)
	}
	var album *ui.BotonIcono
	album = mandoVisor(v, iconos.Album, "albums.addTo", func() {
		if d.AlMenuAlbumes == nil {
			return
		}
		d.AlMenuAlbumes(func(lista []api.Album) {
			elems := make([]fyne.CanvasObject, 0, len(lista))
			for _, a := range lista {
				elems = append(elems, ui.NuevoElementoMenu(a.Rotulo(), iconos.Ninguno, func() {
					if d.AlAnyadirA != nil {
						d.AlAnyadirA(a)
					}
				}))
			}
			mostrarMenu(v.Traduce("albums.addTo"), album, elems)
		})
	})
	hijos = append(hijos, album, mandoDescargar(v, d),
		mandoVisor(v, iconos.Imagen, "viewer.similar", d.AlParecidas), filete(v))

	if d.ConDescribir {
		b := mandoVisor(v, iconos.Texto, "viewer.describe", d.AlDescribir)
		b.Inerte = d.Describiendo
		hijos = append(hijos, b)
	}
	girar := mandoVisor(v, iconos.Rotar, "viewer.rotate", d.AlGirar)
	girar.Inerte = d.Girando
	hijos = append(hijos, girar, filete(v))

	clavePase, glifo := "viewer.slideshow", glifoPlay
	if d.Pase {
		clavePase, glifo = "viewer.slideshowStop", glifoPausa
	}
	pase := mandoVisor(v, iconos.Ninguno, clavePase, d.AlPase)
	pase.Glifo, pase.TamIcono = iconos.RellenoDeRuta(glifo, iconos.TallaSM, tema.Color("text-muted", v.Variante)), iconos.TallaSM
	pase.Pulsado, pase.Inerte = d.Pase, !d.HaySiguiente && !d.Pase

	zoom := mandoVisor(v, iconos.Buscar, claveZoom(d), d.AlZoom)
	zoom.Pulsado = d.TamanyoReal
	info := mandoVisor(v, iconos.Informacion, "viewer.info", d.AlFicha)
	info.Pulsado = d.ConFicha
	hijos = append(hijos, pase, zoom, info, mandoFondo(v, d))

	if d.ConBorrar {
		b := mandoVisor(v, iconos.Papelera, "viewer.delete", d.AlBorrar)
		b.FondoEncima, b.TrazoEncima = "bg-subtle", "danger-text" // `tone="danger"`
		hijos = append(hijos, filete(v), b)
	}
	// «Editar», el único con rótulo: `max-md:sr-only`, así que por debajo de 768 solo
	// queda el icono y el texto pasa a ser el nombre accesible.
	editar := ui.NuevoBoton(v.Traduce("viewer.edit"), d.AlEditar).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Lapiz, 0)
	if v.Ventana < ui.CorteMD {
		editar.Rotulo, editar.Texto = editar.Texto, ""
	}
	hijos = append(hijos, filete(v), editar)
	for _, h := range hijos {
		if b, ok := h.(*ui.BotonIcono); ok && b.Pulsado {
			b.Trazo = "text" // `bg-selected text-text`
		}
	}
	return hijos
}

// mandoDescargar es el desplegable de descargar: JPEG para compartir, PNG sin pérdida
// para seguir editando fuera.
func mandoDescargar(v Vista, d Visor) *ui.BotonIcono {
	var descargar *ui.BotonIcono
	descargar = mandoVisor(v, iconos.Descargar, "viewer.download", func() {
		elems := make([]fyne.CanvasObject, 0, 2)
		for _, f := range [...]struct{ formato, clave string }{{"jpeg", "viewer.downloadJpeg"}, {"png", "viewer.downloadPng"}} {
			elems = append(elems, ui.NuevoElementoMenu(v.Traduce(f.clave), iconos.Ninguno, func() {
				if d.AlDescargar != nil {
					d.AlDescargar(f.formato)
				}
			}))
		}
		mostrarMenu(v.Traduce("viewer.download"), descargar, elems)
	})
	return descargar
}

// mandoFondo es el desplegable del fondo del lienzo, pulsado si no es el crema.
func mandoFondo(v Vista, d Visor) *ui.BotonIcono {
	var fondo *ui.BotonIcono
	fondo = mandoVisor(v, iconos.Rejilla, "viewer.canvasBackground", func() {
		elems := make([]fyne.CanvasObject, 0, len(FondosLienzo))
		for _, f := range FondosLienzo {
			e := ui.NuevoElementoMenu(v.Traduce(f.clave()), iconos.Ninguno, func() {
				if d.AlFondo != nil {
					d.AlFondo(f)
				}
			})
			e.Glifo, e.TamIcono = muestraFondo(f, LadoGlifoFondo, 6), LadoGlifoFondo
			// El `<Icon name="check" size={14} class="text-primary ml-auto">` de la
			// elegida. ponytail: el tic de ElementoMenu mide 16 y el de aquí 14; el
			// panel sale 2 px más ancho hasta que el tic acepte talla.
			e.Marcada, e.TintaTic = f == d.Fondo, "primary"
			elems = append(elems, e)
		}
		mostrarMenu(v.Traduce("viewer.canvasBackground"), fondo, elems)
	})
	fondo.Pulsado = d.Fondo != FondoCrema
	return fondo
}

// LadoGlifoFondo es el `h-3 w-3` de la muestra de cada fondo en su menú.
const LadoGlifoFondo = 3 * 0.25 * tema.Rem

// mandoVisor es un `IconButton size="lg"`: 32 de lado con el glifo de 20.
func mandoVisor(v Vista, ic iconos.Icono, clave string, al func()) *ui.BotonIcono {
	b := ui.NuevoBotonIcono(ic, v.Traduce(clave), al)
	b.Lado, b.TamIcono = ui.LadoIconoLG, IconoMando
	return b
}

// mostrarMenu cuelga un `Dropdown align="end"` del disparador.
func mostrarMenu(rotulo string, disparador fyne.CanvasObject, elems []fyne.CanvasObject) {
	m := ui.NuevoMenu(elems...)
	m.Rotulo = rotulo
	m.Mostrar(disparador)
}

func claveFavorita(f api.Photo) string {
	if f.Favorite {
		return "gallery.unfavorite"
	}
	return "gallery.favorite"
}

func claveZoom(d Visor) string {
	if d.TamanyoReal {
		return "viewer.zoomOut"
	}
	return "viewer.zoomIn"
}

// visorLayout reparte la ventana: cabecera arriba con su filete, la banda de borrar,
// la ficha a la derecha con su asa, y la foto en lo que queda.
type visorLayout struct {
	d       Visor
	ventana float32
}

var _ fyne.Layout = visorLayout{}

func (l visorLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (l visorLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 4 {
		return
	}
	objs[0].Resize(t) // fondo
	alto := altoCabeceraVisor(l.ventana)
	objs[1].Move(fyne.NewPos(RellenoVisorX, RellenoVisorY))
	objs[1].Resize(fyne.NewSize(max(t.Width-2*RellenoVisorX, 0), alto-2*RellenoVisorY-tema.Borde))
	objs[2].Move(fyne.NewPos(0, alto-tema.Borde))
	objs[2].Resize(fyne.NewSize(t.Width, tema.Borde))

	y, i := alto, 4
	if l.d.Borrando && l.d.ConBorrar && len(objs) > i {
		// `mx-3 mb-2`: sin margen arriba, pegada al filete.
		h := ui.AltoA(objs[i], max(t.Width-2*RellenoVisorX, 0))
		objs[i].Move(fyne.NewPos(RellenoVisorX, y))
		objs[i].Resize(fyne.NewSize(max(t.Width-2*RellenoVisorX, 0), h))
		y += h + RellenoVisorY
		i++
	}
	anchoFicha, altoFicha := float32(0), float32(0)
	switch {
	case l.d.ConFicha && len(objs) > i+1 && movil(l.ventana):
		// `max-md:flex-col`: el asa y la ficha DEBAJO de la foto, a todo el ancho.
		alto := medidaFichaVisible(l.d.AltoFicha, AltoFichaMin, AltoFichaMax, t.Height-y)
		altoFicha = alto + AnchoAsaFicha
		objs[i].Move(fyne.NewPos(0, t.Height-altoFicha))
		objs[i].Resize(fyne.NewSize(t.Width, AnchoAsaFicha))
		objs[i+1].Move(fyne.NewPos(0, t.Height-alto))
		objs[i+1].Resize(fyne.NewSize(t.Width, alto))
	case l.d.ConFicha && len(objs) > i+1:
		ancho := medidaFichaVisible(l.d.AnchoFicha, AnchoFichaMin, AnchoFichaMax, t.Width)
		anchoFicha = ancho + AnchoAsaFicha
		objs[i].Move(fyne.NewPos(t.Width-anchoFicha, y))
		objs[i].Resize(fyne.NewSize(AnchoAsaFicha, max(t.Height-y, 0)))
		objs[i+1].Move(fyne.NewPos(t.Width-ancho, y))
		objs[i+1].Resize(fyne.NewSize(ancho, max(t.Height-y, 0)))
	}
	objs[3].Move(fyne.NewPos(0, y))
	objs[3].Resize(fyne.NewSize(max(t.Width-anchoFicha, 0), max(t.Height-y-altoFicha, 0)))
}

// medidaFichaVisible conserva la preferencia, pero deja al menos el 40 % del
// cuerpo para la foto, como el panel inferior del editor. Al girar puede ceder
// por debajo del mínimo de arrastre: el resto de la ficha sigue en su scroll.
func medidaFichaVisible(pedido, minimo, maximo, disponible float32) float32 {
	return min(max(pedido, minimo), maximo, max(0.6*disponible-AnchoAsaFicha, 0))
}

// bandaBorrar es la confirmación que va DENTRO del visor: `border-border bg-surface
// rounded-md border px-3 py-2 text-sm`.
func bandaBorrar(v Vista, d Visor) fyne.CanvasObject {
	// `flex flex-wrap gap-2` con el texto en `min-w-0 flex-1`: en un teléfono el texto se
	// estrecha y se reparte en varias líneas junto a los dos botones.
	fila := container.New(ui.FilaAuto{Hueco: HuecoVisor, Derecha: 3, Partir: true, SaltoFila: HuecoVisor, Elastico: 1},
		ui.NuevoParrafo(v.Traduce("viewer.deleteConfirm"), tema.TextoSM, ui.Normal, "text", v.Variante),
		ui.NuevoBoton(v.Traduce("viewer.delete"), d.AlConfirmarBorrar).Con(ui.Peligro, ui.TallaSM),
		ui.NuevoBoton(v.Traduce("common.cancel"), d.AlCancelarBorrar).Con(ui.Fantasma, ui.TallaSM),
	)
	return recuadro(v, "surface", "border", tema.RadioMD,
		ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(2), Abj: tema.Espacio(2)}, fila)
}

// ---- La zona de la foto: el lienzo, las flechas y el rótulo de la descripción ----

// zonaVisor centra la foto (`items-center justify-center`) o, a tamaño real, la deja
// en su medida para que el desplazamiento la recorra. Las flechas van a `left-3` y
// `right-3` centradas en alto, y el rótulo a `bottom-4` centrado.
type zonaVisor struct {
	lienzo  *LienzoVisor
	real    bool
	ventana float32
	// anchoRotulo es el ancho de contenido del rótulo de la descripción.
	anchoRotulo float32
}

var _ fyne.Layout = zonaVisor{}

func (z zonaVisor) MinSize([]fyne.CanvasObject) fyne.Size {
	if z.real {
		return z.lienzo.MedidaReal()
	}
	return fyne.NewSize(0, 0)
}

func (z zonaVisor) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		switch p := o.(type) {
		case *LienzoVisor:
			p.Move(fyne.NewPos(0, 0))
			p.Resize(t)
		case *DeslizaFoto:
			p.Move(fyne.NewPos(0, 0))
			p.Resize(t)
		case *MandoSobreFoto:
			pos, tam := p.cajaEn(t, z.ventana)
			p.Move(pos)
			p.Resize(tam)
		default:
			// El rótulo de la descripción: `absolute left-1/2 -translate-x-1/2` con
			// `max-w-[90vw] md:max-w-xl`, a `bottom-4`. Un absoluto con `left: 50%` mide
			// lo que su contenido (shrink-to-fit) sin pasar de la MITAD del hueco, que
			// es lo que le queda a la derecha de su `left`: medido contra la web, 275 a
			// 1280 y 180 en un teléfono de 360.
			tope := float32(36 * tema.Rem)
			if z.ventana < ui.CorteMD {
				tope = 0.9 * z.ventana
			}
			ancho := min(z.anchoRotulo, tope, t.Width/2)
			h := ui.AltoA(o, ancho)
			pos := fyne.NewPos((t.Width-ancho)/2, t.Height-tema.Espacio(4)-h)
			o.Move(pos)
			o.Resize(fyne.NewSize(ancho, h))
			if fd := desenfocadoEn(o); fd != nil {
				fd.Colocar(pos, ancho) // el lienzo empieza donde la zona
			}
		}
	}
}

// escenario pone las flechas sobre lo que enseña la foto. Van FUERA de lo que se
// desplaza: a tamaño real la foto se recorre (`overflow-auto`) y ellas se quedan en su
// sitio, como en `EscenarioVisor`.
func escenario(v Vista, foto fyne.CanvasObject, aTamanyoReal bool, anterior, siguiente func(), desde fyne.Position, alDesplazar func(fyne.Position)) fyne.CanvasObject {
	if aTamanyoReal {
		s := container.NewScroll(foto)
		s.Offset, s.OnScrolled = desde, alDesplazar
		foto = s
	}
	return container.New(escenarioLayout{ventana: v.Ventana}, append([]fyne.CanvasObject{foto}, flechasVisor(v, anterior, siguiente)...)...)
}

// escenarioLayout: lo primero llena el hueco y las flechas van a su caja.
type escenarioLayout struct{ ventana float32 }

func (escenarioLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (l escenarioLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for i, o := range objs {
		if f, ok := o.(*MandoSobreFoto); ok && i > 0 {
			pos, tam := f.cajaEn(t, l.ventana)
			f.Move(pos)
			f.Resize(tam)
			continue
		}
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// ---- El asa de la ficha ----

// AsaFicha es el `role="separator"` que estira la ficha: se arrastra, y con el foco
// responde a las flechas (16 px, 48 con Mayúsculas), porque un panel que solo se ajusta
// arrastrando no lo ajusta quien no puede arrastrar.
type AsaFicha struct {
	ui.Pulsable
	// Vertical es el asa del móvil (`aria-orientation="horizontal"`): va encima de la
	// ficha y ajusta su ALTO.
	Vertical bool
	ancho    float32
	desde    float32
	alMover  func(float32, bool)
	fondo    *canvas.Rectangle
	// mayus es Mayúsculas pulsada: las flechas mueven 48 y no 16, como `e.shiftKey`.
	mayus bool
}

var (
	_ fyne.Widget     = (*AsaFicha)(nil)
	_ fyne.Draggable  = (*AsaFicha)(nil)
	_ desktop.Keyable = (*AsaFicha)(nil)
)

// NuevaAsaFicha crea el asa con el ancho que tiene ahora la ficha.
func NuevaAsaFicha(v Vista, d Visor) *AsaFicha {
	a := &AsaFicha{ancho: d.AnchoFicha, alMover: d.AlAnchoFicha}
	if movil(v.Ventana) {
		a.Vertical, a.ancho, a.alMover = true, d.AltoFicha, d.AlAltoFicha
	}
	a.Nombre = v.Traduce("viewer.infoResize")
	a.ExtendBaseWidget(a)
	return a
}

// Dragged mueve el borde: la ficha está a la derecha, así que arrastrar hacia la
// izquierda la hace CRECER.
func (a *AsaFicha) Dragged(e *fyne.DragEvent) {
	if a.Vertical {
		a.desde -= e.Dragged.DY // abajo, arrastrar hacia ARRIBA la hace crecer
	} else {
		a.desde -= e.Dragged.DX
	}
	if a.alMover != nil {
		a.alMover(a.ancho+a.desde, false)
	}
}

// DragEnd guarda la medida.
func (a *AsaFicha) DragEnd() {
	if a.alMover != nil {
		a.alMover(a.ancho+a.desde, true)
	}
	a.ancho, a.desde = a.ancho+a.desde, 0
}

// TypedKey es el `onKeyDown` del separador.
func (a *AsaFicha) TypedKey(e *fyne.KeyEvent) {
	paso := float32(16)
	if a.mayus {
		paso = 48
	}
	crece, mengua := fyne.KeyLeft, fyne.KeyRight
	if a.Vertical {
		crece, mengua = fyne.KeyUp, fyne.KeyDown
	}
	switch e.Name {
	case crece:
	case mengua:
		paso = -paso
	default:
		a.Pulsable.TypedKey(e)
		return
	}
	if a.alMover != nil {
		a.alMover(a.ancho+paso, true)
	}
}

// KeyDown y KeyUp siguen a Mayúsculas: `fyne.KeyEvent` no trae los modificadores.
func (a *AsaFicha) KeyDown(e *fyne.KeyEvent) {
	if e.Name == desktop.KeyShiftLeft || e.Name == desktop.KeyShiftRight {
		a.mayus = true
	}
}

// KeyUp suelta Mayúsculas.
func (a *AsaFicha) KeyUp(e *fyne.KeyEvent) {
	if e.Name == desktop.KeyShiftLeft || e.Name == desktop.KeyShiftRight {
		a.mayus = false
	}
}

// Cursor es `cursor-col-resize`.
func (a *AsaFicha) Cursor() desktop.Cursor {
	if a.Vertical {
		return desktop.VResizeCursor
	}
	return desktop.HResizeCursor
}

// CreateRenderer: `bg-border/40 hover:bg-primary/40`.
func (a *AsaFicha) CreateRenderer() fyne.WidgetRenderer {
	a.fondo = canvas.NewRectangle(color.NRGBA{})
	r := &asaRenderer{a: a}
	r.Refresh()
	return r
}

type asaRenderer struct{ a *AsaFicha }

func (r *asaRenderer) MinSize() fyne.Size {
	if r.a.Vertical {
		return fyne.NewSize(0, AnchoAsaFicha)
	}
	return fyne.NewSize(AnchoAsaFicha, 0)
}
func (r *asaRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.a.fondo} }
func (r *asaRenderer) Destroy()                     {}
func (r *asaRenderer) Layout(t fyne.Size)           { r.a.fondo.Resize(t) }
func (r *asaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c := conAlfa(tema.Color("border", v), 0.4)
	if r.a.Encima() || r.a.Enfocado() {
		c = conAlfa(tema.Color("primary", v), 0.4)
	}
	r.a.fondo.FillColor = c
	r.a.fondo.Refresh()
}

// ---- La ficha ----

// FichaDelVisor es el `<aside>` con `space-y-4 p-4 border-l`: la sección destacada de
// la descripción, los detalles técnicos y la pregunta.
func FichaDelVisor(v Vista, d Visor) fyne.CanvasObject {
	tecnica := fichaTecnica(v, d)
	bloques := make([]ui.Bloque, 0, 2+len(tecnica)+5)
	bloques = append(bloques,
		ui.Bloque{Obj: seccionDescripcion(v, d), Margen: tema.Espacio(4)},
		// `pt-1`: el rótulo técnico baja un poco más que el `space-y-4`.
		ui.Bloque{Obj: rotuloFicha(v.Traduce("viewer.infoTechnical"), ui.Medio, "text-muted", v, tema.Espacio(1)), Margen: tema.Espacio(4)},
	)
	for _, f := range tecnica {
		bloques = append(bloques, ui.Bloque{Obj: f, Margen: tema.Espacio(2)})
	}
	bloques[len(bloques)-1].Margen = tema.Espacio(4)
	bloques = append(bloques, preguntaFicha(v, d)...)
	// El `p-4` va DENTRO del desplazamiento, como el `overflow-y-auto` del aside, y el
	// alto se cuenta al ancho de verdad de la ficha: sin él, el desplazamiento mediría
	// los párrafos partidos a su ancho mínimo y dejaría un hueco al final.
	ancho := max(medidaFichaVisible(d.AnchoFicha, AnchoFichaMin, AnchoFichaMax, v.Ventana)-tema.Borde-2*RellenoFicha, 0)
	if movil(v.Ventana) {
		ancho = max(v.Ventana-2*RellenoFicha, 0) // `max-md:border-t max-md:border-l-0`
	}
	cuerpo := container.New(ui.Margen{Izq: RellenoFicha, Der: RellenoFicha, Arr: RellenoFicha, Abj: RellenoFicha},
		ui.AlAnchoFluido(ancho, ui.PilaBloques(bloques...)))
	return container.New(fichaLayout{arriba: movil(v.Ventana)}, canvas.NewRectangle(tema.Color("border", v.Variante)), container.NewVScroll(cuerpo))
}

// seccionDescripcion es la `<section class="bg-bg-subtle border-border rounded-lg
// border p-3">`: el título con su icono y el botón de describir, la pista y la
// descripción (o el vacío con su botón).
func seccionDescripcion(v Vista, d Visor) fyne.CanvasObject {
	titulo := container.New(ui.Fila{Hueco: tema.Espacio(1.5)},
		iconos.Trazo(iconos.Texto, iconos.TallaXS, "primary", v.Variante).Objeto(),
		rotuloFicha(v.Traduce("viewer.infoDescriptionTitle"), ui.SemiNegrita, "text", v, 0))
	cabecera := []fyne.CanvasObject{titulo}
	if d.ConDescribir && !d.EditandoDesc {
		clave := "viewer.describe"
		switch {
		case d.Describiendo:
			clave = "viewer.describing"
		case d.Descripcion != "":
			clave = "viewer.describeAgain"
		}
		// Los dos mandos de la casa: `Button variant="ghost" size="xs"` (con su rueda
		// mientras describe) e `IconButton size="sm"` con el lápiz de 14. Eran dos
		// botones escritos a mano.
		e := ui.NuevoBoton(v.Traduce(clave), d.AlDescribir).Con(ui.Fantasma, ui.TallaXS)
		e.Cargando = d.Describiendo
		mandos := []fyne.CanvasObject{e}
		if d.Descripcion != "" {
			lapiz := ui.NuevoBotonIcono(iconos.Lapiz, v.Traduce("viewer.editDescription"), d.AlEditarDesc)
			lapiz.Lado, lapiz.TamIcono = ui.LadoIconoSM, 14
			mandos = append([]fyne.CanvasObject{lapiz}, mandos...)
		}
		cabecera = append(cabecera, container.New(ui.Fila{Hueco: tema.Espacio(1)}, mandos...))
	}
	bloques := []ui.Bloque{
		{Obj: container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 1}, cabecera...), Margen: tema.Espacio(1.5)},
		bloqueLargo(ui.NuevoParrafo(v.Traduce("viewer.infoDescriptionHint"),
			escalon11, ui.Normal, "text-muted", v.Variante), tema.Espacio(2.5)),
	}
	switch {
	case d.EditandoDesc && d.CampoDesc != nil:
		// `space-y-2`: el área de tres filas y, a la derecha, Cancelar y Guardar.
		clave := "viewer.saveDescription"
		if d.GuardandoDesc {
			clave = "viewer.savingDescription"
		}
		cancelar := ui.NuevoBoton(v.Traduce("common.cancel"), d.AlCancelarDesc).Con(ui.Fantasma, ui.TallaSM)
		guardar := ui.NuevoBoton(v.Traduce(clave), d.AlGuardarDesc).Con(ui.Primario, ui.TallaSM)
		cancelar.Inerte, guardar.Inerte = d.GuardandoDesc, d.GuardandoDesc
		bloques = append(bloques,
			ui.Bloque{Obj: d.CampoDesc, Margen: tema.Espacio(2)},
			ui.Bloque{Obj: container.New(ui.FilaAuto{Hueco: tema.Espacio(2)}, cancelar, guardar)})
	case d.Descripcion != "":
		// `bg-bg/70 border-border/60 rounded border p-2.5`.
		texto := ui.NuevoParrafo(d.Descripcion, textoFicha(v), ui.Normal, "text", v.Variante)
		debajo := tema.Color("bg-subtle", v.Variante)
		caja := recuadroCon(sobre(conAlfa(tema.Color("bg", v.Variante), 0.7), debajo),
			sobre(conAlfa(tema.Color("border", v.Variante), 0.6), debajo),
			tema.RadioSM, ui.Margen{Izq: tema.Espacio(2.5), Der: tema.Espacio(2.5), Arr: tema.Espacio(2.5), Abj: tema.Espacio(2.5)}, texto)
		bloques = append(bloques, ui.Bloque{Obj: caja})
		if len(d.Etiquetas) > 0 {
			bloques[len(bloques)-1].Margen = tema.Espacio(2) + tema.Espacio(0.5)
			bloques = append(bloques, ui.Bloque{Obj: etiquetasFicha(v, d.Etiquetas)})
		}
	default:
		vacio := []ui.Bloque{{Obj: ParrafoEn(v.Traduce("viewer.infoDescriptionEmpty"), tema.TextoXS, ui.Normal, "text-muted", v.Variante), Margen: tema.Espacio(2)}}
		if d.ConDescribir {
			clave := "viewer.describe"
			if d.Describiendo {
				clave = "viewer.describing"
			}
			b := ui.NuevoBoton(v.Traduce(clave), d.AlDescribir).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Texto, 0)
			b.Inerte = d.Describiendo
			vacio = append(vacio, ui.Bloque{Obj: b})
		}
		// `py-2 text-center`.
		bloques = append(bloques, ui.Bloque{Obj: container.New(ui.Margen{Arr: tema.Espacio(2), Abj: tema.Espacio(2)},
			container.New(centradoH{}, ui.PilaBloques(vacio...)))})
	}
	return recuadro(v, "bg-subtle", "border", tema.RadioLG,
		ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(3), Abj: tema.Espacio(3)}, ui.PilaBloques(bloques...))
}

// escalon11 es el `text-[11px]` de la pista y de las etiquetas: el tamaño es literal y
// la línea la HEREDA, que dentro de la ficha es la del texto raíz (19,25).
var escalon11 = tema.Escalon{Tam: 11, Linea: tema.TextoRaiz.Linea}

// textoFicha es el `text-xs sm:text-sm` de la descripción, con `leading-relaxed`.
func textoFicha(v Vista) tema.Escalon {
	e := tema.TextoXS
	if v.Ventana >= ui.CorteSM {
		e = tema.TextoSM
	}
	return tema.Escalon{Tam: e.Tam, Linea: 1.625 * e.Tam}
}

// etiquetasFicha son las `#etiqueta` en píldora: `bg-surface border rounded-full px-2
// py-0.5 text-[11px]`, con `flex-wrap gap-1`.
func etiquetasFicha(v Vista, etiquetas []string) fyne.CanvasObject {
	hijos := make([]fyne.CanvasObject, len(etiquetas))
	for i, e := range etiquetas {
		hijos[i] = recuadro(v, "surface", "border", tema.RadioPill,
			ui.Margen{Izq: tema.Espacio(2), Der: tema.Espacio(2), Arr: tema.Espacio(0.5), Abj: tema.Espacio(0.5)},
			ParrafoEn("#"+e, escalon11, ui.Normal, "text-muted", v.Variante))
	}
	return container.New(ui.Fila{Hueco: tema.Espacio(1)}, hijos...)
}

// rotuloFicha es un `text-xs tracking-wider uppercase` con su peso y su tinta, más un
// relleno arriba (`pt-1` en el técnico).
func rotuloFicha(s string, p ui.Peso, token string, v Vista, arriba float32) fyne.CanvasObject {
	vs := ui.NuevoVersalCon(s, tema.TextoXS, p, tema.Color(token, v.Variante), ui.TrackingWider)
	c := &cajaVersal{vs: vs, arriba: arriba}
	return c.contenedor()
}

// fichaTecnica son las filas del `<dl>`: fecha, cámara, objetivo, lugar, dimensiones y
// peso, en el orden de la web y saltándose las que la foto no tiene.
func fichaTecnica(v Vista, d Visor) []fyne.CanvasObject {
	clave := "viewer.infoUploaded"
	if d.Foto.TakenFromEXIF {
		clave = "viewer.infoTaken"
	}
	// El `<dd>` es `min-w-0 break-words`: el valor se reparte en líneas si no cabe (la
	// fecha larga lo hace a 340 de ficha).
	valor := func(s string) fyne.CanvasObject {
		return ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, "text", v.Variante)
	}
	type fila struct {
		clave string
		valor fyne.CanvasObject
	}
	// `dateStyle: 'long', timeStyle: 'short'` y en la hora de ESTE equipo, como
	// `toLocaleString` en el navegador.
	filas := []fila{{v.Traduce(clave), valor(i18n.FormatDateTimeLong(v.T.Locale(), d.Foto.TakenAt.Local()))}}
	if d.Foto.Camera != "" {
		filas = append(filas, fila{v.Traduce("viewer.infoCamera"), valor(d.Foto.Camera)})
	}
	if d.Foto.Lens != "" {
		filas = append(filas, fila{v.Traduce("viewer.infoLens"), valor(d.Foto.Lens)})
	}
	if l := d.Foto.Location; l != nil {
		mapa := NuevoEnlace(v.Traduce("viewer.infoPlaceMap"), d.AlMapa)
		cerca := NuevoEnlace(v.Traduce("viewer.infoPlaceNearby"), d.AlCerca)
		mapa.Subrayado, mapa.Tinta, cerca.Subrayado, cerca.Tinta = true, "text", true, "text"
		filas = append(filas, fila{v.Traduce("viewer.infoPlace"), container.New(ui.Fila{Hueco: espacioTexto(v)},
			ParrafoEn(Coordenadas(v, *l), tema.TextoSM, ui.Normal, "text", v.Variante), mapa, cerca)})
	}
	filas = append(filas,
		fila{v.Traduce("viewer.infoDimensions"), valor(fmtLados(d.Foto.Width, d.Foto.Height))},
		fila{v.Traduce("viewer.infoSize"), valor(i18n.FormatBytes(v.T.Locale(), float64(d.Foto.SizeBytes)))},
	)
	// Una REJILLA y no filas sueltas: la columna de las claves mide lo que la más ancha
	// de todas, igual en cada fila.
	columna := float32(0)
	for _, f := range filas {
		columna = max(columna, ui.Medir(f.clave, tema.TextoSM, ui.Normal).Width)
	}
	objs := make([]fyne.CanvasObject, len(filas))
	for i, f := range filas {
		objs[i] = container.New(filaClaveValor{columna: columna},
			ui.NuevoParrafo(f.clave, tema.TextoSM, ui.Normal, "text-muted", v.Variante), f.valor)
	}
	return objs
}

// espacioTexto es el ancho de un espacio en `text-sm`: los tres trozos del lugar van
// separados por `{' '}` en la web, no por un `gap`.
func espacioTexto(Vista) float32 { return ui.Medir(" ", tema.TextoSM, ui.Normal).Width }

// Coordenadas es el «41,3851, 2,1734» de un lugar: cuatro decimales —unos once metros,
// lo que de verdad sabe un GPS de móvil— y con el `toFixed(4)` de la web, que usa el
// PUNTO decimal en cualquier idioma.
func Coordenadas(_ Vista, l api.PhotoPlace) string {
	return i18n.FormatFixed("en", l.Lat, 4) + ", " + i18n.FormatFixed("en", l.Lon, 4)
}

// filaClaveValor es una fila de `grid-cols-[minmax(0,auto)_minmax(9rem,1fr)] gap-x-4`:
// la clave en la columna de todas —que encoge si hace falta para dejarle al valor sus
// `9rem`— y el valor en lo que quede. Cada celda se reparte en líneas a su ancho.
type filaClaveValor struct{ columna float32 }

func (f filaClaveValor) anchos(total float32) (clave, valor float32) {
	clave = min(f.columna, max(total-tema.Espacio(4)-9*tema.Rem, 0))
	return clave, max(total-clave-tema.Espacio(4), 0)
}

func (f filaClaveValor) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(f.columna+tema.Espacio(4)+9*tema.Rem, tema.TextoSM.Linea)
}

// AltoCon es el de la celda más alta a ese ancho.
func (f filaClaveValor) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	c, v := f.anchos(ancho)
	return max(ui.AltoA(objs[0], c), ui.AltoA(objs[1], v), tema.TextoSM.Linea)
}

func (f filaClaveValor) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	c, v := f.anchos(t.Width)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(c, ui.AltoA(objs[0], c)))
	objs[1].Move(fyne.NewPos(c+tema.Espacio(4), 0))
	objs[1].Resize(fyne.NewSize(v, ui.AltoA(objs[1], v)))
}

// preguntaFicha es el `<form>` de «Pregunta sobre esta foto»: `mt-4 border-t pt-4`, el
// rótulo, el campo con su botón, la respuesta y el aviso de que no se guarda.
func preguntaFicha(v Vista, d Visor) []ui.Bloque {
	campo := d.Pregunta
	if campo == nil {
		campo = ui.NuevaEntrada(v.Traduce("viewer.askPlaceholder"))
	}
	enviar := ui.NuevoBoton(v.Traduce("viewer.askSend"), func() {
		if d.AlPreguntar != nil {
			d.AlPreguntar(campo.Texto())
		}
	}).Con(ui.Secundario, ui.TallaMD)
	enviar.Inerte = d.Pensando
	respuesta := d.Respuesta
	if d.Pensando {
		respuesta = v.Traduce("viewer.describing")
	}
	filete := canvas.NewRectangle(tema.Color("border", v.Variante))
	bloques := []ui.Bloque{
		// El `mt-4` del form NO se suma al `space-y-4` del aside: en Tailwind 4 el
		// `space-y` es un margen de ABAJO del hermano de arriba, y los dos márgenes se
		// funden. Medido contra la web: 13,5 del final de la rejilla al filete.
		{Obj: filete, Margen: tema.Espacio(4), Alto: func(float32) float32 { return tema.Borde }},
		{Obj: ParrafoEn(v.Traduce("viewer.ask"), tema.TextoSM, ui.Normal, "text-muted", v.Variante), Margen: tema.Espacio(1)},
		{Obj: container.New(filaConCampoElastico{Hueco: tema.Espacio(2)}, campo, enviar), Margen: tema.Espacio(2)},
	}
	// El `<p role="status">` vacío no ocupa nada y sus márgenes `mt-2` se funden con el
	// del aviso de debajo (colapso de márgenes): medido contra la web, del campo al
	// aviso van 7 y no 29,75.
	if respuesta != "" {
		bloques = append(bloques, bloqueLargo(ui.NuevoParrafo(respuesta, tema.TextoSM, ui.Normal, "text-muted", v.Variante), tema.Espacio(2)))
	}
	return append(bloques, bloqueLargo(ui.NuevoParrafo(v.Traduce("viewer.askNotSaved"),
		tema.TextoXS, ui.Normal, "text-muted", v.Variante), 0))
}

// bloqueLargo mete un párrafo que se reparte, con su alto dependiente del ancho.
func bloqueLargo(t *ui.Parrafo, margen float32) ui.Bloque {
	return ui.Bloque{
		Obj:    t,
		Margen: margen,
		Alto:   func(ancho float32) float32 { return ui.AltoParrafo(t, ancho) },
	}
}

// filaConCampoElastico es `flex gap-2` con el PRIMER hijo en `flex-1`: se lleva lo que
// sobre después de los demás. MaxCampo conserva un ancho preferido que puede
// encoger, como el `w-64 min-w-0` del formulario de crear un álbum.
type filaConCampoElastico struct{ Hueco, MaxCampo float32 }

var _ fyne.Layout = filaConCampoElastico{}

func (f filaConCampoElastico) MinSize(objs []fyne.CanvasObject) fyne.Size {
	ancho, alto := float32(0), float32(0)
	for i, o := range objs {
		if i > 0 {
			ancho += f.Hueco
		}
		m := o.MinSize()
		ancho += m.Width
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

func (f filaConCampoElastico) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	resto := float32(0)
	for _, o := range objs[1:] {
		resto += o.MinSize().Width + f.Hueco
	}
	// `flex` con `align-items: stretch`: el campo se estira al alto de la fila, que es
	// el del botón (28 y no los 24,75 del campo). Medido contra la web.
	x := max(t.Width-resto, 0)
	if f.MaxCampo > 0 {
		x = min(x, f.MaxCampo)
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(x, t.Height))
	x += f.Hueco
	for _, o := range objs[1:] {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + f.Hueco
	}
}

// fichaLayout: el filete de la izquierda (`border-l`) —o de arriba en móvil
// (`max-md:border-t`)— y el contenido con su `p-4`.
type fichaLayout struct{ arriba bool }

var _ fyne.Layout = fichaLayout{}

func (fichaLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (f fichaLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 2 {
		return
	}
	objs[0].Move(fyne.NewPos(0, 0))
	if f.arriba {
		objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
		objs[1].Move(fyne.NewPos(0, tema.Borde))
		objs[1].Resize(fyne.NewSize(t.Width, max(t.Height-tema.Borde, 0)))
		return
	}
	objs[0].Resize(fyne.NewSize(tema.Borde, t.Height))
	objs[1].Move(fyne.NewPos(tema.Borde, 0))
	objs[1].Resize(fyne.NewSize(max(t.Width-tema.Borde, 0), t.Height))
}

// ---- El rótulo de la descripción sobre la foto ----

// rotuloDescripcion es la `region` que la web pone sobre la foto cuando hay
// descripción: `bg-black/85 border-white/15 rounded-lg px-4 py-2.5 text-white`, con el
// icono en `text-primary`, el título en versalitas, «Ver en la ficha» y el texto a dos
// líneas (`line-clamp-2`).
//
// El `shadow-xl` y el `backdrop-blur-md` los pone PantallaVisor alrededor (conSombra y
// FondoDesenfocado).
//
// ponytail: el borde blanco al 15 % va DEBAJO del negro al 85 % (así pinta la caja de
// `ui`), y el interior sale hasta 6 niveles de 255 más claro sobre una foto negra; sobre
// una clara, lo mismo que la web. El `hover:line-clamp-none` tampoco: el texto entero
// está en la ficha, a un clic de «Ver en la ficha».
func rotuloDescripcion(v Vista, d Visor) (rotulo *fyne.Container, natural float32) {
	blanco := func(a float32) color.NRGBA { return color.NRGBA{R: 255, G: 255, B: 255, A: uint8(a*255 + 0.5)} }
	titulo := NuevoVersalRepartido(v.Traduce("viewer.infoDescriptionTitle"), escalon10, ui.Negrita, blanco(0.7), ui.TrackingWider)
	// `text-xs font-medium hover:underline`: no es el `text-sm` de los demás enlaces.
	ver := NuevoEnlace(v.Traduce("viewer.viewInInfo"), d.AlVerIA).Con(tema.TextoXS, ui.Medio)
	// Blanco y no `primary-text`: ese token es la tinta del tema para SUS superficies
	// —azul oscuro en el claro— y sobre este negro se quedaba en 2,2:1. `on-primary` es
	// el token que vale blanco en los tres temas (`Enlace` pinta por token).
	ver.Tinta, ver.SubrayadoEncima = "on-primary", true
	e := tema.TextoXS
	if v.Ventana >= ui.CorteSM {
		e = tema.TextoSM
	}
	texto := ui.NuevoParrafo(d.Descripcion, tema.Escalon{Tam: e.Tam, Linea: 1.375 * e.Tam}, ui.Normal, "text", v.Variante)
	texto.Lineas = 2
	texto.Tinta = blanco(0.95)
	// `flex items-center justify-between gap-2`: el título encoge y se parte en líneas
	// (en un teléfono, en dos) y «Ver en la ficha» es `shrink-0`.
	enlace := container.New(ui.Fila{Hueco: tema.Espacio(1)}, iconos.TrazoConTinta(iconos.Informacion, 12, blanco(1)).Objeto(), ver)
	pEnlace := PiezaFlex{Max: enlace.MinSize().Width, Min: enlace.MinSize().Width, Fija: true}
	cabecera := container.New(FilaEncoge{Hueco: tema.Espacio(2), Separar: true, Piezas: []PiezaFlex{titulo.Pieza(), pEnlace}},
		titulo, enlace)
	cuerpo := ui.PilaBloques(ui.Bloque{Obj: cabecera, Margen: tema.Espacio(0.5)}, ui.Bloque{Obj: texto})
	fila := container.New(filaIcono{hueco: tema.Espacio(2.5), arriba: tema.Espacio(0.5)},
		iconos.TrazoConTinta(iconos.Texto, 16, blanco(0.7)).Objeto(), cuerpo)
	// Su ancho de contenido (`max-content`): el relleno y el borde, el icono con su
	// hueco y lo más ancho de la cabecera en una fila y el texto en una línea.
	anchoCabecera := titulo.Pieza().Max + tema.Espacio(2) + pEnlace.Max
	anchoTexto := ui.Medir(d.Descripcion, e, ui.Normal).Width
	natural = 2*tema.Espacio(4) + 16 + tema.Espacio(2.5) + max(anchoCabecera, anchoTexto)
	return recuadroCon(color.NRGBA{A: 217}, color.NRGBA{}, tema.RadioLG,
		ui.Margen{Izq: tema.Espacio(4), Der: tema.Espacio(4), Arr: tema.Espacio(2.5), Abj: tema.Espacio(2.5)}, fila), natural
}

// desenfocadoEn es el FondoDesenfocado de una caja, puesto directamente o dentro de su
// sombra; nil si no lleva.
func desenfocadoEn(o fyne.CanvasObject) *FondoDesenfocado {
	c, ok := o.(*fyne.Container)
	if !ok {
		return nil
	}
	switch l := c.Layout.(type) {
	case capaDesenfocada:
		return l.fd
	case *sombraLayout:
		return desenfocadoEn(c.Objects[1])
	}
	return nil
}

// capaDesenfocada pone el FondoDesenfocado debajo de la caja, a su tamaño, y mide lo que
// la caja.
type capaDesenfocada struct{ fd *FondoDesenfocado }

func (c capaDesenfocada) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[1].MinSize() }

func (c capaDesenfocada) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return ui.AltoA(objs[1], ancho)
}

func (c capaDesenfocada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// escalon10 es el `text-[10px]` del título del rótulo; la línea la hereda.
var escalon10 = tema.Escalon{Tam: 10, Linea: tema.TextoRaiz.Linea}

// filaIcono es `flex items-start gap-*` con un icono delante (`mt-*`) y el resto en
// `flex-1`.
type filaIcono struct{ hueco, arriba float32 }

func (f filaIcono) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(a.Width+f.hueco+b.Width, max(a.Height+f.arriba, b.Height))
}

// AltoCon es el alto a un ancho dado: el del cuerpo, que es lo que se reparte.
func (f filaIcono) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	a := objs[0].MinSize()
	return max(a.Height+f.arriba, ui.AltoA(objs[1], ancho-a.Width-f.hueco))
}

func (f filaIcono) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, f.arriba))
	objs[0].Resize(a)
	objs[1].Move(fyne.NewPos(a.Width+f.hueco, 0))
	objs[1].Resize(fyne.NewSize(max(t.Width-a.Width-f.hueco, 0), t.Height))
}

// centradoH centra su único hijo en horizontal (`text-center` de un bloque).
type centradoH struct{}

func (centradoH) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (centradoH) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos((t.Width-m.Width)/2, 0))
	objs[0].Resize(fyne.NewSize(m.Width, t.Height))
}

// ---- Las flechas ----

// MandoSobreFoto es el `IconButton tone="media"` de la web: un botón negro al 60 % con
// el icono en blanco, para lo que se pulsa SOBRE una foto —las flechas del visor, el «⋯»
// de la papelera—.
//
// No es `ui.BotonIcono` porque sus colores van por token y estos no lo son: lo de
// debajo es una foto, que no cambia con el tema (ver negro60 en miniatura.go).
type MandoSobreFoto struct {
	ui.Pulsable
	Icono          iconos.Icono
	Lado, TamIcono float32
	// Izquierda dice, en el visor, a qué lado va la flecha.
	Izquierda bool
	// Franja alarga el objetivo con ratón hasta el canto de la ventana y 120 px por
	// encima y por debajo de la placa (el `::before` de `FlechaVisor`).
	Franja bool
}

var _ fyne.Widget = (*MandoSobreFoto)(nil)

// NuevoMandoSobreFoto crea el mando con su nombre accesible.
func NuevoMandoSobreFoto(ic iconos.Icono, rotulo string, lado, tamIcono float32, al func()) *MandoSobreFoto {
	f := &MandoSobreFoto{Icono: ic, Lado: lado, TamIcono: tamIcono}
	f.Nombre, f.AlPulsar = rotulo, al
	f.ExtendBaseWidget(f)
	return f
}

// Las flechas del visor (`FlechaVisor` de la web, en EscenarioVisor.tsx). Eran de 32 con
// un galón de 20 y sin borde: sobre una foto negra la placa medía 1:1 contra la foto.
const (
	// LadoFlecha es el `IconButton size="xl"`: 48, por encima del suelo táctil.
	LadoFlecha = 12 * 0.25 * tema.Rem
	// GlifoFlecha es el galón de 24.
	GlifoFlecha float32 = 24
	// MargenFlecha es el `sm:left-4`; MargenFlechaMovil, el `left-2` de un teléfono.
	MargenFlecha      = 4 * 0.25 * tema.Rem
	MargenFlechaMovil = 2 * 0.25 * tema.Rem
	// FranjaFlecha es lo que el objetivo crece por encima y por debajo de la placa con
	// ratón (`-inset-y-30`); a lo ancho llega hasta el canto (`-inset-x-4`).
	FranjaFlecha = 30 * 0.25 * tema.Rem
)

// flechasVisor son «anterior» y «siguiente», las que haya.
func flechasVisor(v Vista, anterior, siguiente func()) []fyne.CanvasObject {
	var out []fyne.CanvasObject
	if anterior != nil {
		out = append(out, NuevaFlechaVisor(v.Traduce("viewer.prev"), true, anterior))
	}
	if siguiente != nil {
		out = append(out, NuevaFlechaVisor(v.Traduce("viewer.next"), false, siguiente))
	}
	return out
}

// NuevaFlechaVisor es la flecha de un lado del visor: placa de 48 con el galón de 24, a
// `left-4`/`right-4` (8 por debajo de `sm:`) y centrada en alto. Con ratón el objetivo
// es una franja pegada al canto de la ventana (Franja).
func NuevaFlechaVisor(rotulo string, izquierda bool, al func()) *MandoSobreFoto {
	galon := iconos.ChevronDerecha
	if izquierda {
		galon = iconos.ChevronIzquierda
	}
	f := NuevoMandoSobreFoto(galon, rotulo, LadoFlecha, GlifoFlecha, al)
	f.Izquierda, f.Franja = izquierda, true
	return f
}

// cajaEn es dónde va el mando en una zona de tamaño t: la placa a su margen y centrada
// en alto, o, si es una franja y hay ratón, la franja entera desde el canto.
func (f *MandoSobreFoto) cajaEn(t fyne.Size, ventana float32) (fyne.Position, fyne.Size) {
	margen := float32(MargenFlecha)
	if ventana > 0 && ventana < ui.CorteSM {
		margen = MargenFlechaMovil
	}
	m := f.MinSize()
	tam, x := m, margen
	if f.Franja && !conDedo() {
		tam = fyne.NewSize(m.Width+2*margen, m.Height+2*FranjaFlecha)
		x = 0
	}
	if !f.Izquierda {
		x = t.Width - x - tam.Width
	}
	return fyne.NewPos(x, (t.Height-tam.Height)/2), tam
}

// fileteSobreFoto es el `border-white/40` del tono `media` compuesto sobre su `bg-black/60`
// (`background-clip: border-box`): gris 134 al 76 %, que sobre una foto negra da 102 —el
// borde a 3,7:1— y sobre una blanca 163.
var fileteSobreFoto = color.NRGBA{R: 134, G: 134, B: 134, A: 194}

// fileteSobreFotoEncima es el `hover:border-white/60` sobre el `hover:bg-black/75`.
var fileteSobreFotoEncima = color.NRGBA{R: 170, G: 170, B: 170, A: 230}

// CreateRenderer: la caja negra al 60 % con su filete, el icono blanco y el aro de foco,
// todo en la PLACA, que va centrada en la caja del mando (la franja es solo objetivo).
func (f *MandoSobreFoto) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &flechaRenderer{
		f: f, aro: ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v),
		caja:  ui.NuevaCaja(negro60, fileteSobreFoto, tema.Forma("radius-control")).ConTrazo(),
		glifo: iconos.TrazoConTinta(f.Icono, f.TamIcono, blanco),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.glifo.Objeto())
	r.Refresh()
	return r
}

type flechaRenderer struct {
	f       *MandoSobreFoto
	aro     *ui.Aro
	caja    *ui.Caja
	glifo   *iconos.Dibujo
	objetos []fyne.CanvasObject
}

// MinSize es la placa, con el suelo táctil de la web (`pointer-coarse:min-h-[44px]`) para
// las tallas que no llegan.
func (r *flechaRenderer) MinSize() fyne.Size {
	lado := r.f.Lado
	if conDedo() {
		lado = max(lado, ui.SueloTactilBarra)
	}
	return fyne.NewSize(lado, lado)
}
func (r *flechaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *flechaRenderer) Destroy()                     {}

// Layout pone la placa en el centro de la caja del mando.
func (r *flechaRenderer) Layout(t fyne.Size) {
	placa := r.MinSize()
	pos := fyne.NewPos((t.Width-placa.Width)/2, (t.Height-placa.Height)/2)
	r.aro.ColocarEn(pos, placa)
	r.caja.Redimensionar(placa)
	for _, o := range r.caja.Objetos() {
		o.Move(o.Position().Add(pos))
	}
	r.glifo.Colocar(pos.X+(placa.Width-r.f.TamIcono)/2, pos.Y+(placa.Height-r.f.TamIcono)/2, r.f.TamIcono)
}

func (r *flechaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	fondo, filete := negro60, fileteSobreFoto
	if r.f.Encima() { // `hover:bg-black/75 hover:border-white/60`
		fondo, filete = negro75, fileteSobreFotoEncima
	}
	r.caja.Pintar(fondo, filete)
	r.Layout(r.f.Size())
}

// ---- El lienzo: la foto grande ----

// LienzoVisor pinta la variante «grande» con `object-contain`, centrada, sobre el fondo
// que la persona haya elegido para el lienzo.
//
// Es otro `canvas.Raster` por lo mismo que la tesela, pero el ajuste es el CONTRARIO:
// aquí la foto entera tiene que verse, así que se encaja dentro de la caja y lo que
// sobra es fondo. La tesela recorta; el visor no.
type LienzoVisor struct {
	widget.BaseWidget

	Foto  api.Photo
	Fondo FondoLienzo
	// Real es el tamaño real (`max-h-none max-w-none`): la foto a su medida y no
	// encajada.
	Real bool
	// Original es la foto entera, ya decodificada, para el tamaño real; nil mientras
	// llega (y entonces se estira la «grande», que es lo que pinta la web siempre).
	Original image.Image
	// AlPulsar es el `onClick` del `<img>`: alterna el tamaño real.
	AlPulsar func()

	teca *Fototeca
	// toques cuenta el doble toque con el dedo.
	toques toques

	muPintar sync.Mutex
}

var (
	_ fyne.Widget   = (*LienzoVisor)(nil)
	_ fyne.Tappable = (*LienzoVisor)(nil)
)

// NuevoLienzoVisor crea el lienzo de esa foto.
func NuevoLienzoVisor(v Vista, d Visor) *LienzoVisor {
	// La variante GRANDE y no la miniatura: son dos cachés distintas porque son dos
	// costes distintos (una foto de 2048 px ocupa dieciséis veces más que una de 512)
	// y dos ciclos de vida — el visor enseña UNA y la galería cien.
	teca := v.TecaGrande
	if teca == nil {
		teca = v.Teca
	}
	l := &LienzoVisor{Foto: d.Foto, Fondo: d.Fondo, Real: d.TamanyoReal, Original: d.Original, AlPulsar: d.AlZoom, teca: teca}
	l.ExtendBaseWidget(l)
	if teca != nil {
		teca.apuntar(d.Foto.ID, l.Refrescar)
	}
	return l
}

// AccessibilityRole: es la imagen, y Fyne solo tiene cuatro roles.
func (l *LienzoVisor) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// AccessibilityLabel es el `alt`: la descripción si la hay, y si no el nombre, que es
// lo que hace `textoAlternativo()`.
func (l *LienzoVisor) AccessibilityLabel() string {
	if l.Foto.Description != "" {
		return l.Foto.Description
	}
	return l.Foto.Name
}

// Tapped alterna el tamaño real, como el clic en la foto de la web. Con el dedo, con
// doble toque: un toque suelto no hace nada (lo vigila el `onClickCapture` de la web).
func (l *LienzoVisor) Tapped(e *fyne.PointEvent) {
	if l.AlPulsar == nil || (conDedo() && !l.toques.doble(e)) {
		return
	}
	l.AlPulsar()
}

// cajaEncajada es dónde va una foto de w×h encajada en t, centrada y SIN ampliarla por
// encima de su tamaño: en la web la caja del `<img>` es la de la foto (`h-auto w-auto
// max-h-full max-w-full`), y una foto más pequeña que el hueco se queda en su medida.
func cajaEncajada(w, h float32, t fyne.Size) (fyne.Position, fyne.Size) {
	k := min(t.Width/w, t.Height/h, 1)
	caja := fyne.NewSize(w*k, h*k)
	return fyne.NewPos((t.Width-caja.Width)/2, (t.Height-caja.Height)/2), caja
}

// fraccionEn es el punto p del lienzo en fracciones de la foto encajada (0..1).
func (l *LienzoVisor) fraccionEn(p fyne.Position) (fx, fy float32) {
	w, h := float32(l.Foto.Width), float32(l.Foto.Height)
	if w <= 0 || h <= 0 {
		return 0.5, 0.5
	}
	pos, caja := cajaEncajada(w, h, l.Size())
	return min(max((p.X-pos.X)/caja.Width, 0), 1), min(max((p.Y-pos.Y)/caja.Height, 0), 1)
}

// Desplazar mueve la foto de lado mientras el dedo la arrastra (DeslizaFoto). Se mueve
// el widget entero y no se repinta: un `Refresh` del raster subiría a la GPU la foto
// entera en cada movimiento del dedo, y un `Move` solo cambia dónde se dibuja.
func (l *LienzoVisor) Desplazar(dx float32) { l.Move(fyne.NewPos(dx, l.Position().Y)) }

// Cursor es `cursor-zoom-in`/`cursor-zoom-out`. Fyne no trae la lupa, así que es la
// mano de lo que se puede pulsar.
func (l *LienzoVisor) Cursor() desktop.Cursor { return desktop.PointerCursor }

// Refrescar serializa el repintado, igual que en la tesela: la foto grande también
// llega en segundo plano.
func (l *LienzoVisor) Refrescar() {
	l.muPintar.Lock()
	defer l.muPintar.Unlock()
	l.Refresh()
}

// MedidaReal es lo que mide la foto a tamaño real: su ancho y su alto en px de CSS,
// que es como la web pinta el `<img>` (`width={foto.width}` sin tope).
//
// La web estira ahí la variante «grande» (2048 px): a ×2, una foto de 12 Mpx sale a 8064
// píxeles de ancho desde 2048. Aquí se pinta con el ORIGINAL en cuanto llega (Original),
// por teselas de LadoTeselaReal: un solo `canvas.Raster` de ese tamaño serían 195 MB por
// fotograma, y el pintor de GL no genera las teselas que quedan fuera de la vista.
//
// ponytail: las teselas que se han visto se quedan en la caché de texturas de Fyne hasta
// que caducan (un minuto sin pintarse); recorrer entera una foto de 48 Mpx a ×2 puede
// llegar a tenerla toda en la GPU un rato. Si hace falta, se sueltan al salir del
// tamaño real.
func (l *LienzoVisor) MedidaReal() fyne.Size {
	w, h := float32(l.Foto.Width), float32(l.Foto.Height)
	if w <= 0 || h <= 0 {
		return fyne.NewSize(0, 0)
	}
	return fyne.NewSize(w, h)
}

// LadoTeselaReal es el lado de cada tesela del tamaño real, en px de CSS: 1024 píxeles
// de dispositivo a ×2, 4 MB por textura.
const LadoTeselaReal = 512

// fuenteReal es de donde salen las teselas: el original si ya está, y si no la variante
// grande a su resolución (la que estira la web).
func (l *LienzoVisor) fuenteReal() image.Image {
	if l.Original != nil {
		return l.Original
	}
	if l.teca == nil || l.Foto.ID == "" {
		return nil
	}
	w, h := float32(l.Foto.Width), float32(l.Foto.Height)
	k := min(1, 2048/max(w, h))
	if m := l.teca.Tesela(l.Foto.ID, int(w*k+0.5), int(h*k+0.5), 0); m != nil {
		return m
	}
	if m := l.teca.Pintada(l.Foto.ID); m != nil {
		return m
	}
	return nil
}

// teselaReal genera la tesela (col, fila) del tamaño real a los píxeles que pida el
// pintor, llevando la fuente entera con una transformación afín: así las teselas
// casan sin costuras aunque la fuente no tenga un número entero de píxeles por tesela.
func (l *LienzoVisor) teselaReal(col, fila int) func(w, h int) image.Image {
	return func(w, h int) image.Image {
		dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
		src := l.fuenteReal()
		medida := l.MedidaReal()
		if src == nil || w <= 0 || medida.Width <= 0 {
			return dst
		}
		ancho := min(LadoTeselaReal, medida.Width-float32(col*LadoTeselaReal))
		k := float64(float32(w) / ancho) // px de dispositivo por px de CSS
		b := src.Bounds()
		sx, sy := float64(medida.Width)/float64(b.Dx())*k, float64(medida.Height)/float64(b.Dy())*k
		tx := -float64(b.Min.X)*sx - float64(col*LadoTeselaReal)*k
		ty := -float64(b.Min.Y)*sy - float64(fila*LadoTeselaReal)*k
		xdraw.ApproxBiLinear.Transform(dst, f64.Aff3{sx, 0, tx, 0, sy, ty}, src, b, xdraw.Src, nil)
		return dst
	}
}

// CreateRenderer arma el lienzo.
func (l *LienzoVisor) CreateRenderer() fyne.WidgetRenderer {
	r := &lienzoRenderer{l: l, fondo: canvas.NewRectangle(l.Fondo.colorFondo())}
	r.foto = canvas.NewRaster(r.generar)
	r.foto.ScaleMode = canvas.ImageScalePixels
	r.damero = canvas.NewRaster(r.pintarDamero)
	r.damero.ScaleMode = canvas.ImageScalePixels
	r.objetos = []fyne.CanvasObject{r.fondo, r.damero, r.foto}
	if medida := l.MedidaReal(); l.Real && medida.Width > 0 {
		r.foto.Hide()
		cols, filas := int(math.Ceil(float64(medida.Width/LadoTeselaReal))), int(math.Ceil(float64(medida.Height/LadoTeselaReal)))
		for f := range filas {
			for c := range cols {
				t := canvas.NewRaster(l.teselaReal(c, f))
				t.ScaleMode = canvas.ImageScalePixels
				r.teselas = append(r.teselas, t)
				r.objetos = append(r.objetos, t)
			}
		}
		r.cols = cols
	}
	r.Refresh()
	return r
}

type lienzoRenderer struct {
	l        *LienzoVisor
	fondo    *canvas.Rectangle
	damero   *canvas.Raster
	foto     *canvas.Raster
	anchoCSS atomic.Uint32
	// teselas son las del tamaño real, fila a fila, `cols` por fila.
	teselas []*canvas.Raster
	cols    int
	objetos []fyne.CanvasObject
}

// pintarDamero es el `repeating-conic-gradient(rgba(180,180,180,.35) 0% 25%,
// transparent 0% 50%)` de 16 px del fondo «damero»: cuadros de 8 px de CSS. El raster se
// pide en píxeles de DISPOSITIVO, así que el cuadro se escala con la razón entre los
// dos, que se lee del ancho de CSS que dejó Layout (atómico: lo escribe el hilo que
// coloca y lo lee el que pinta).
func (r *lienzoRenderer) pintarDamero(w, h int) image.Image {
	css := math.Float32frombits(r.anchoCSS.Load())
	if w == 0 || css <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, w, h))
	}
	return pintarDameroCon(w, h, 8*float32(w)/css)
}

// pintarDameroCon pinta el damero con cuadros de `paso` píxeles de dispositivo. Lo
// comparten el visor y el editor, que ponen el mismo fondo detrás de la foto.
func pintarDameroCon(w, h int, paso float32) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	gris := color.NRGBA{R: 180, G: 180, B: 180, A: 89} // 0,35 × 255
	for y := range h {
		for x := range w {
			// `repeating-conic-gradient(… 0% 25%, transparent 0% 50%)` pinta el cuarto de
			// ARRIBA A LA DERECHA de cada baldosa y, al repetirse, el de abajo a la
			// izquierda: los cuadros con columna y fila de distinta paridad.
			if (int(float32(x)/paso)+int(float32(y)/paso))%2 == 1 {
				m.SetNRGBA(x, y, gris)
			}
		}
	}
	return m
}

// generar pinta la foto encajada. La caja que se le pide al remuestreador es la de la
// FOTO dentro del hueco, no el hueco entero, así que `cover` sobre ella no recorta nada.
//
// Mientras se prepara la foto a un tamaño nuevo —al estirar la ficha o la ventana— se
// enseña la que ya hubiera a otro tamaño, que el raster estira: el hueco en blanco a
// cada píxel de arrastre sería un parpadeo, y el navegador tampoco lo hace.
func (r *lienzoRenderer) generar(w, h int) image.Image {
	l := r.l
	if l.teca != nil && l.Foto.ID != "" {
		if m := l.teca.Tesela(l.Foto.ID, w, h, 0); m != nil {
			return m
		}
		if m := l.teca.Pintada(l.Foto.ID); m != nil {
			return m
		}
	}
	return image.NewRGBA(image.Rect(0, 0, w, h))
}

func (r *lienzoRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *lienzoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *lienzoRenderer) Destroy()                     {}

func (r *lienzoRenderer) Refresh() {
	r.fondo.FillColor = r.l.Fondo.colorFondo()
	r.fondo.Refresh()
	if r.l.Fondo == FondoDamero {
		r.damero.Show()
	} else {
		r.damero.Hide()
	}
	r.Layout(r.l.Size())
	r.foto.Refresh()
	for _, t := range r.teselas {
		t.Refresh()
	}
}

// Layout encaja la foto: `max-h-full max-w-full object-contain`, centrada; o a su
// tamaño real, arriba a la izquierda, para que el desplazamiento la recorra. El fondo
// va solo DETRÁS de la foto, que es donde lo pone el `style` del `<img>`.
func (r *lienzoRenderer) Layout(t fyne.Size) {
	w, h := float32(r.l.Foto.Width), float32(r.l.Foto.Height)
	var pos fyne.Position
	caja := t
	switch {
	case w <= 0 || h <= 0:
	case r.l.Real:
		caja = r.l.MedidaReal()
		pos = fyne.NewPos(max((t.Width-caja.Width)/2, 0), max((t.Height-caja.Height)/2, 0))
	default:
		pos, caja = cajaEncajada(w, h, t)
	}
	for _, o := range []fyne.CanvasObject{r.fondo, r.damero, r.foto} {
		o.Move(pos)
		o.Resize(caja)
	}
	for i, t := range r.teselas {
		x, y := float32(i%r.cols*LadoTeselaReal), float32(i/r.cols*LadoTeselaReal)
		t.Move(pos.AddXY(x, y))
		t.Resize(fyne.NewSize(min(LadoTeselaReal, caja.Width-x), min(LadoTeselaReal, caja.Height-y)))
	}
	r.anchoCSS.Store(math.Float32bits(caja.Width))
}
