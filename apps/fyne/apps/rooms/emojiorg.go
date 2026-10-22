// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"image"
	"image/color"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En la web: apps/frontend/apps/rooms/src/lib/customEmoji.ts y las dos secciones de
// abajo de EmojiPicker.tsx.
//
// Los emoji de la organización: un `:nombre:` que apunta a una imagen de Sytue Drive. La
// Sala pide el catálogo UNA vez al arrancar y lo deja en un registro de paquete, como el
// de módulo de la web: así lo consultan el cuerpo de un mensaje, las píldoras de
// reacción y el selector sin enhebrar la tabla por seis widgets.

// catalogoOrg es `customEmoji.ts`: el catálogo en su orden, el mapa nombre → fichero y
// quién da la imagen de un fichero (la Sala, con su caché). Solo se toca en el hilo de
// la interfaz.
var catalogoOrg struct {
	lista   []api.RoomsCustomEmoji
	fichero map[string]string
	imagen  func(fichero string) image.Image
}

// PonerCatalogoOrg es `setCustomEmoji`.
func PonerCatalogoOrg(es []api.RoomsCustomEmoji) {
	catalogoOrg.lista = es
	catalogoOrg.fichero = make(map[string]string, len(es))
	for _, e := range es {
		catalogoOrg.fichero[e.Name] = e.FileID
	}
}

// ficheroPropio es el fichero de `nombre` si es de la organización.
func ficheroPropio(nombre string) (string, bool) {
	f, ok := catalogoOrg.fichero[nombre]
	return f, ok
}

// imagenPropia es la imagen de un fichero del catálogo si ya ha llegado; si no, nil
// (y la pide).
func imagenPropia(fichero string) image.Image {
	if catalogoOrg.imagen == nil {
		return nil
	}
	return catalogoOrg.imagen(fichero)
}

var (
	// reEmojiPropio es `CUSTOM_EMOJI_RE`, y reSoloPropio el `loneEmoji` de MessageBody.
	reEmojiPropio = regexp.MustCompile(`:([a-z0-9_-]+):`)
	reSoloPropio  = regexp.MustCompile(`^:([a-z0-9_-]{1,64}):$`)
	// reNombrePropio es `EMOJI_NAME_RE`, el CHECK de la migración 0013.
	reNombrePropio = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
)

// soloPropio es el fichero de un cuerpo que es SOLO un emoji de la organización, que
// se pinta en grande.
func soloPropio(cuerpo string) (string, bool) {
	m := reSoloPropio.FindStringSubmatch(strings.TrimSpace(cuerpo))
	if m == nil {
		return "", false
	}
	return ficheroPropio(m[1])
}

// Las medidas. HOJA.
var (
	// AltoRejillaPropios es el `max-h-24` de su rejilla; LadoImagenPropio, el `h-5 w-5`
	// de la imagen dentro de su celda de `h-8 w-8`.
	AltoRejillaPropios = tema.Espacio(24)
	LadoImagenPropio   = tema.Espacio(5)
	// AltoSoloPropio es el `max-h-48` de un mensaje que es solo un emoji.
	AltoSoloPropio = tema.Espacio(48)
)

// EMOJI_IMG_CLASS: `mx-px h-[1.375em] w-[1.375em] align-[-0.3em]`, en em de la letra
// que lo rodea.
const (
	emEmojiPropio   = 1.375
	bajaEmojiPropio = 0.3
)

// anchoEmojiPropio es lo que ocupa en la línea, con su `mx-px`.
func anchoEmojiPropio(e tema.Escalon) float32 { return emEmojiPropio*e.Tam + 2 }

// yEmojiPropio es dónde va el borde de arriba de la imagen en una caja de línea que
// empieza en `y`: su base cae `0.3em` por debajo de la línea base del texto.
func yEmojiPropio(e tema.Escalon, p ui.Peso, y float32) float32 {
	return y + ui.LineaBase(e, p) + (bajaEmojiPropio-emEmojiPropio)*e.Tam
}

// tamSoloPropio es el `max-h-48 max-w-full` de la imagen a su tamaño natural.
func tamSoloPropio(img image.Image, ancho float32) fyne.Size {
	if img == nil {
		return fyne.NewSize(0, 0) // `loading="lazy"`: no ocupa hasta que llega
	}
	b := img.Bounds()
	w, h := float32(b.Dx()), float32(b.Dy())
	if h > AltoSoloPropio {
		w, h = w*AltoSoloPropio/h, AltoSoloPropio
	}
	if ancho > 0 && w > ancho {
		w, h = ancho, h*ancho/w
	}
	return fyne.NewSize(w, h)
}

// BuscarPropios es el `customMatches` del selector: los que CONTIENEN la consulta; sin
// ella, todos.
func BuscarPropios(q string) []api.RoomsCustomEmoji {
	q = strings.ToLower(strings.TrimSpace(q))
	var out []api.RoomsCustomEmoji
	for _, e := range catalogoOrg.lista {
		if buscar.Contiene(e.Name, q) {
			out = append(out, e)
		}
	}
	return out
}

// ---- En el selector ----

// seccionPropios es el `border-t mt-2 pt-2` con su rótulo y su rejilla, o nada si no
// casa ninguno.
func (s *SelectorEmoji) seccionPropios(v tema.Variante) []fyne.CanvasObject {
	ps := BuscarPropios(s.caja.Texto())
	s.propias = s.propias[:0]
	s.desplazaPropios = nil
	if len(ps) == 0 {
		return nil
	}
	celdas := make([]fyne.CanvasObject, 0, len(ps))
	for _, p := range ps {
		c := &celdaPropia{s: s, propio: p}
		c.Nombre, c.AlPulsar = p.Name, func() {
			if s.AlElegir != nil {
				s.AlElegir(":" + p.Name + ":")
			}
		}
		c.ExtendBaseWidget(c)
		s.propias = append(s.propias, c)
		celdas = append(celdas, c)
	}
	rejilla := container.New(&rejillaEmoji{}, celdas...)
	desplaza := container.NewVScroll(rejilla)
	s.desplazaPropios = desplaza
	desplaza.SetMinSize(fyne.NewSize(0, min(rejillaEmoji{}.MinSize(celdas).Height, AltoRejillaPropios)))
	rotulo := ui.NuevoRotulo(s.T.T("emoji.custom", nil), escalonMeta, ui.Medio, "text-subtle")
	return append(fileteSelector(v),
		container.NewBorder(nil, nil, sangria(tema.Espacio(1)), nil, rotulo), hueco(tema.Espacio(1)), desplaza)
}

// fileteSelector es el `border-border mt-2 border-t pt-2` que abre una sección.
func fileteSelector(v tema.Variante) []fyne.CanvasObject {
	filete := canvas.NewRectangle(tema.Color("border", v))
	filete.SetMinSize(fyne.NewSize(0, tema.Borde))
	return []fyne.CanvasObject{hueco(tema.Espacio(2)), filete, hueco(tema.Espacio(2))}
}

// ActualizarImagenes repinta las celdas cuando llega una imagen.
func (s *SelectorEmoji) ActualizarImagenes() {
	for _, c := range s.propias {
		c.Refresh()
	}
}

// celdaPropia es un emoji de la organización en la rejilla: su imagen en `h-5 w-5
// object-contain` y `hover:bg-bg-subtle`. No entra en el recorrido de las flechas, que
// es el de la web.
type celdaPropia struct {
	ui.Pulsable
	s      *SelectorEmoji
	propio api.RoomsCustomEmoji
}

func (c *celdaPropia) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &celdaPropiaRenderer{
		c:     c,
		aro:   ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		fondo: ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioSM),
		img:   canvas.NewImageFromImage(nil),
	}
	r.img.FillMode = canvas.ImageFillContain
	r.objetos = append(append(r.aro.Objetos(), r.fondo.Objetos()...), r.img)
	r.Refresh()
	return r
}

type celdaPropiaRenderer struct {
	c       *celdaPropia
	aro     *ui.Aro
	fondo   *ui.Caja
	img     *canvas.Image
	objetos []fyne.CanvasObject
}

func (r *celdaPropiaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(LadoCeldaEmoji, LadoCeldaEmoji)
}
func (r *celdaPropiaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *celdaPropiaRenderer) Destroy()                     {}

func (r *celdaPropiaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Redimensionar(t)
	r.img.Move(fyne.NewPos((t.Width-LadoImagenPropio)/2, (t.Height-LadoImagenPropio)/2))
	r.img.Resize(fyne.NewSize(LadoImagenPropio, LadoImagenPropio))
}

func (r *celdaPropiaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.c.Encima() {
		fondo = tema.Color("bg-subtle", v)
	}
	r.fondo.Pintar(fondo, color.NRGBA{})
	r.aro.Ver(r.c.Enfocado())
	r.aro.Pintar(v)
	if img := imagenPropia(r.c.propio.FileID); img != nil && r.img.Image != img {
		r.img.Image = img
		r.img.Refresh()
	}
	r.Layout(r.c.Size())
}

// ---- Subir uno ----

// MaxBytesEmoji es `MAX_EMOJI_BYTES`.
const MaxBytesEmoji = 512 * 1024

// subidaPropia es el estado del formulario de subir: el nombre, la imagen elegida, el
// error y si está subiendo.
type subidaPropia struct {
	abierta, subiendo bool
	nombre            *ui.Entrada
	ruta, fallo       string
}

// seccionSubir es el `border-t mt-2 pt-2` del final: «+ Añadir emoji personalizado» o,
// abierto, el nombre, la imagen, la pista o el error y «Subir» / «Cancelar».
//
// DECISIÓN: el `<input type="file">` de la web es aquí un botón secundario con el nombre
// del fichero elegido al lado, que abre el selector del sistema.
func (s *SelectorEmoji) seccionSubir(v tema.Variante) []fyne.CanvasObject {
	t, sp := s.T, &s.subida
	if !sp.abierta {
		abrir := &botonSubirPropio{texto: t.T("emoji.uploadCustom", nil)}
		abrir.Nombre, abrir.AlPulsar = abrir.texto, func() {
			sp.abierta = true
			s.montar()
		}
		abrir.ExtendBaseWidget(abrir)
		return append(fileteSelector(v), abrir)
	}
	if sp.nombre == nil {
		sp.nombre = ui.NuevaEntrada(t.T("emoji.namePlaceholder", nil))
		sp.nombre.Rotulo = t.T("emoji.nameLabel", nil)
		sp.nombre.Propia = &ui.CampoPropio{MargenX: tema.Espacio(2), MargenY: tema.Espacio(1), Texto: tema.TextoSM, Borde: "border", Relleno: "bg"}
		sp.nombre.AlCambiar(func(string) {
			if sp.fallo != "" {
				sp.fallo = ""
				s.montar()
			}
		})
	}
	elegir := ui.NuevoBoton(t.T("emoji.imageLabel", nil), func() {
		if s.AlElegirImagen != nil {
			s.AlElegirImagen(func(ruta string) {
				sp.ruta, sp.fallo = ruta, ""
				s.montar()
			})
		}
	}).Con(ui.Secundario, ui.TallaSM)
	elegir.Inerte = sp.subiendo
	elegido := ""
	if sp.ruta != "" {
		elegido = filepath.Base(sp.ruta)
	}
	fichero := ui.NuevoRotulo(elegido, tema.TextoXS, ui.Normal, "text-muted")
	var nota fyne.CanvasObject
	if sp.fallo != "" {
		aviso := iconos.Trazo(iconos.Aviso, 11, "danger-text", v)
		aviso.Colocar(0, 0, 11)
		nota = container.New(filaCentradaRooms{hueco: tema.Espacio(1)},
			container.New(tamFijo{fyne.NewSize(11, 11)}, aviso.Objeto()),
			ui.NuevoRotulo(sp.fallo, tema.TextoXS, ui.Normal, "danger-text"))
	} else {
		nota = ui.NuevoRotulo(t.T("emoji.imageHint", i18n.Vars{"max": strconv.Itoa(MaxBytesEmoji / 1024)}), escalonMeta, ui.Normal, "text-subtle")
	}
	subirRot := t.T("action.upload", nil)
	if sp.subiendo {
		subirRot = t.T("state.uploading", nil)
	}
	subir := ui.NuevoBoton(subirRot, s.subirPropio).Con(ui.Primario, ui.TallaSM)
	cancelar := ui.NuevoBoton(t.T("action.cancel", nil), func() {
		sp.abierta, sp.fallo = false, ""
		s.montar()
	}).Con(ui.Secundario, ui.TallaSM)
	subir.Inerte, cancelar.Inerte = sp.subiendo, sp.subiendo
	form := container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, sp.nombre,
		container.New(filaCentradaRooms{hueco: tema.Espacio(2)}, elegir, fichero), nota,
		container.New(filaCentradaRooms{hueco: tema.Espacio(1.5)}, subir, cancelar))
	return append(fileteSelector(v), form)
}

// errSinCatalogo es subir sin Rooms que sirva el catálogo o sin Drive.
var errSinCatalogo = errors.New("sin catálogo de la organización")

// validarPropio es la parte de `submitUpload` que no sube: el nombre, que no exista, que
// haya imagen, que sea una imagen y su tamaño. Devuelve la clave del error y sus
// variables, o "".
func validarPropio(nombre, ruta string) (string, i18n.Vars) {
	switch {
	case !reNombrePropio.MatchString(nombre):
		return "emoji.badName", nil
	case slices.ContainsFunc(catalogoOrg.lista, func(e api.RoomsCustomEmoji) bool { return e.Name == nombre }):
		return "emoji.exists", i18n.Vars{"name": nombre}
	case ruta == "":
		return "emoji.pickImage", nil
	case !strings.HasPrefix(mime.TypeByExtension(strings.ToLower(filepath.Ext(ruta))), "image/"):
		return "emoji.mustBeImage", nil
	}
	if info, err := os.Stat(ruta); err == nil && info.Size() > MaxBytesEmoji {
		return "emoji.tooLarge", i18n.Vars{"max": strconv.Itoa(MaxBytesEmoji / 1024)}
	}
	return "", nil
}

// subirPropio es `submitUpload`.
func (s *SelectorEmoji) subirPropio() {
	sp := &s.subida
	if sp.subiendo || sp.nombre == nil {
		return
	}
	nombre := strings.ToLower(strings.TrimSpace(sp.nombre.Texto()))
	if clave, vars := validarPropio(nombre, sp.ruta); clave != "" {
		sp.fallo = s.T.T(clave, vars)
		s.montar()
		return
	}
	if s.AlSubir == nil {
		return
	}
	sp.subiendo, sp.fallo = true, ""
	s.montar()
	s.AlSubir(nombre, sp.ruta, func(err error) {
		sp.subiendo = false
		if err != nil {
			sp.fallo = s.T.T("emoji.uploadFailed", nil)
		} else {
			sp.abierta, sp.ruta = false, ""
			sp.nombre.PonerTexto("")
		}
		s.montar()
	})
}

// botonSubirPropio es el `flex w-full items-center gap-1.5 rounded-control px-1 py-1 text-xs
// font-medium text-text-muted hover:bg-bg-subtle hover:text-text` con su «+» en `text-sm
// font-semibold leading-none`.
type botonSubirPropio struct {
	ui.Pulsable
	texto string
}

func (b *botonSubirPropio) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonSubirRenderer{
		b:     b,
		aro:   ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v),
		fondo: ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-control")),
		mas:   ui.Texto("+", tema.TextoSM, ui.SemiNegrita, color.NRGBA{}),
		texto: ui.Texto(b.texto, tema.TextoXS, ui.Medio, color.NRGBA{}),
	}
	r.objetos = append(append(r.aro.Objetos(), r.fondo.Objetos()...), r.mas, r.texto)
	r.Refresh()
	return r
}

type botonSubirRenderer struct {
	b          *botonSubirPropio
	aro        *ui.Aro
	fondo      *ui.Caja
	mas, texto *canvas.Text
	objetos    []fyne.CanvasObject
}

// El «+» lleva `leading-none`: su caja de línea es su tamaño.
func (r *botonSubirRenderer) linea() float32 { return max(tema.TextoXS.Linea, tema.TextoSM.Tam) }

func (r *botonSubirRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, r.linea()+2*tema.Espacio(1))
}
func (r *botonSubirRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonSubirRenderer) Destroy()                     {}

func (r *botonSubirRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Redimensionar(t)
	x := tema.Espacio(1)
	ui.ColocarTexto(r.mas, tema.Escalon{Tam: tema.TextoSM.Tam, Linea: tema.TextoSM.Tam}, x, (t.Height-tema.TextoSM.Tam)/2)
	x += ui.Medir("+", tema.TextoSM, ui.SemiNegrita).Width + tema.Espacio(1.5)
	ui.ColocarTexto(r.texto, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
}

func (r *botonSubirRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := color.NRGBA{}, tema.Color("text-muted", v)
	if r.b.Encima() {
		fondo, tinta = tema.Color("bg-subtle", v), tema.Color("text", v)
	}
	r.fondo.Pintar(fondo, color.NRGBA{})
	r.mas.Color, r.texto.Color = tinta, tinta
	r.mas.Refresh()
	r.texto.Refresh()
	r.aro.Ver(r.b.Enfocado())
	r.aro.Pintar(v)
	r.Layout(r.b.Size())
}

// ---- En la Sala ----

// cargarEmojisOrg es el `listCustomEmoji` del arranque. Degrada en silencio: sin él, el
// selector no tiene su sección y un `:nombre:` se lee como texto.
func (sa *Sala) cargarEmojisOrg() {
	ep, ok := sa.S.(EmojiPropio)
	if !ok {
		return
	}
	pedir(sa, ep.CustomEmoji, func(es []api.RoomsCustomEmoji, err error) {
		if err != nil {
			return
		}
		PonerCatalogoOrg(es)
		sa.repintarEmojisOrg()
	})
}

// repintarEmojisOrg lleva un cambio del catálogo o una imagen recién llegada a lo que
// los enseña: el selector abierto, el historial y el hilo.
func (sa *Sala) repintarEmojisOrg() {
	if sa.emojis != nil {
		sa.emojis.montar()
	}
	if sa.abierto != "" {
		sa.P.Canal.Historial.Refresh()
	}
	if sa.hilo != nil {
		sa.hilo.Refresh()
	}
}

// imagenEmoji es la imagen de un fichero del catálogo si ya está bajada; si no, pide
// su miniatura de Drive (UNA vez) y devuelve nil hasta que llegue.
//
// DECISIÓN (del dueño): la miniatura de Drive y no el original que pinta la web; a 19 px
// no se nota y es lo que ya baja «GIF».
//
// ponytail: un GIF sale en su primer fotograma (Fyne no lo anima).
func (sa *Sala) imagenEmoji(fichero string) image.Image {
	dr, ok := sa.Ficheros.(DriveOrganizacion)
	if img := sa.imagenesEmoji[fichero]; img != nil || !ok || sa.pidiendoEmoji[fichero] {
		return img
	}
	sa.pidiendoEmoji[fichero] = true
	pedir(sa, func(ctx context.Context) (image.Image, error) {
		rc, err := dr.Thumbnail(ctx, fichero)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rc.Close() }()
		img, _, err := image.Decode(rc)
		return img, err
	}, func(img image.Image, err error) {
		if err != nil || img == nil {
			return // se queda pedida: una imagen que no baja no se reintenta en cada repintado
		}
		sa.imagenesEmoji[fichero] = img
		if sa.gif != nil {
			sa.gif.Imagenes[fichero] = img
			sa.gif.Refresh()
		}
		sa.repintarEmojisOrg()
	})
	return nil
}

// subirEmojiOrg es el «Subir» del selector: la imagen a la raíz de Drive, el atajo, y el
// catálogo con el nuevo en su orden.
func (sa *Sala) subirEmojiOrg(nombre, ruta string, luego func(error)) {
	ep, ok := sa.S.(EmojiPropio)
	dr, conDrive := sa.Ficheros.(DriveOrganizacion)
	if !ok || !conDrive {
		luego(errSinCatalogo)
		return
	}
	tipo := mime.TypeByExtension(strings.ToLower(filepath.Ext(ruta)))
	pedir(sa, func(ctx context.Context) (api.RoomsCustomEmoji, error) {
		return subirImagenOrg(ctx, ep, dr, ruta, tipo, nombre)
	}, func(e api.RoomsCustomEmoji, err error) {
		if err == nil {
			lista := append(slices.DeleteFunc(slices.Clone(catalogoOrg.lista),
				func(x api.RoomsCustomEmoji) bool { return x.Name == e.Name }), e)
			slices.SortFunc(lista, func(a, b api.RoomsCustomEmoji) int { return strings.Compare(a.Name, b.Name) })
			PonerCatalogoOrg(lista)
			sa.repintarEmojisOrg()
		}
		luego(err)
	})
}
