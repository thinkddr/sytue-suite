// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"image"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El visor vivo: `Visor()` de PhotosApp.tsx con el estado que allí guardan sus
// `useState`. Se monta como una capa sobre la ventana ENTERA —la barra de la suite
// incluida—, que es lo que hace el `<dialog>` con `showModal()`: lo de debajo queda
// inerte y Escape lo cierra.

// DuracionPase es el `setInterval(onNext, 4000)` del pase automático.
const DuracionPase = 4 * time.Second

// Las claves de las preferencias del visor en este equipo. La del fondo es la misma que
// la web guarda en `localStorage`; la del ancho lleva guiones bajos y no los puntos de
// `sytue.fotos.ficha.ancho` porque con puntos tiene forma de clave de catálogo, y el
// guardián de i18n la tomaría por una.
const (
	prefAnchoFicha = "sytue_fotos_ficha_ancho"
	prefAltoFicha  = "sytue_fotos_ficha_alto"
	prefFondo      = "sytue_photos_canvas_bg"
)

// origenVisor es de qué lista salió la foto abierta: decide qué mandos ofrece (la web
// los enciende por los `on…` que le pasa cada pantalla) y por dónde se pasa de foto.
type origenVisor uint8

const (
	desdeGaleria origenVisor = iota
	desdeAlbum
	desdeEncontradas
	// desdeSuelta es una foto que llegó sola, por un enlace de Drive.
	desdeSuelta
)

type estadoVisor struct {
	a      *App
	origen origenVisor
	i      int
	suelta []api.Photo

	conFicha, borrando, pase, tamanyoReal bool
	girando, describiendo, pensando       bool
	editandoDesc, guardandoDesc           bool
	aviso, respuesta                      string
	campoDesc                             *AreaFicha
	// lados son los de la foto tras girarla: se intercambian aquí para que la caja
	// no deje bandas mientras llega la variante nueva.
	lados map[string][2]int

	pregunta *ui.Entrada
	capa     *capaVisor
	// desplazamiento es por dónde va el recorrido a tamaño real.
	desplazamiento fyne.Position
	parar          chan struct{}
	// original es la foto entera para el tamaño real, de `pidiendo` mientras llega.
	original *fotoOriginal
	pidiendo string
}

// fotoOriginal es la foto a su tamaño, decodificada, con la huella de la receta con la
// que salió: tras girarla ya no vale.
type fotoOriginal struct {
	id, huella string
	img        image.Image
}

func (e *estadoVisor) lista() []api.Photo {
	switch e.origen {
	case desdeGaleria:
		return e.a.g.fotos
	case desdeAlbum:
		return e.a.uno.fotos
	case desdeEncontradas:
		return e.a.enc.fotos
	case desdeSuelta:
	}
	return e.suelta
}

// foto es la que se enseña, con sus lados de después de girarla.
func (e *estadoVisor) foto() (api.Photo, bool) {
	l := e.lista()
	if e.i < 0 || e.i >= len(l) {
		return api.Photo{}, false
	}
	f := l[e.i]
	if ld, ok := e.lados[f.ID]; ok {
		f.Width, f.Height = ld[0], ld[1]
	}
	return f, true
}

// haySiguiente: en la galería, la última foto cargada no es la última si quedan
// páginas por pedir.
func (e *estadoVisor) haySiguiente() bool {
	return e.i < len(e.lista())-1 || (e.origen == desdeGaleria && !e.a.g.fin)
}

// nuevaFoto olvida lo que es de UNA foto: la confirmación de borrar, el tamaño real y
// la pregunta. La ficha, el pase y el aviso se quedan, como en la web.
func (e *estadoVisor) nuevaFoto() {
	if e.a != nil {
		if g := e.a.v.TecaGrande; g != nil && g != e.a.v.Teca {
			g.Cancelar()
		}
	}
	e.borrando, e.tamanyoReal, e.respuesta, e.pensando = false, false, "", false
	e.desplazamiento = fyne.Position{}
	e.original, e.pidiendo = nil, "" // una foto de 48 Mpx son 72 MB decodificada
	e.editandoDesc = false
	e.pregunta.PonerTexto("")
}

// abrirVisor abre la foto `i` de esa lista.
func (a *App) abrirVisor(origen origenVisor, i int, suelta ...api.Photo) {
	if a.vis != nil {
		a.vis.origen, a.vis.i, a.vis.suelta = origen, i, suelta
		a.vis.nuevaFoto()
		a.pintarVisor()
		return
	}
	e := &estadoVisor{a: a, origen: origen, i: i, suelta: suelta, lados: map[string][2]int{}}
	e.pregunta = ui.NuevaEntrada(a.v.Traduce("viewer.askPlaceholder"))
	e.pregunta.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			a.preguntarFoto(e.pregunta.Texto())
			return true
		}
		return false
	}
	e.capa = nuevaCapaVisor(a.cerrarVisor)
	a.vis = e
	a.pintarVisor()
	if a.w != nil {
		c := a.w.Canvas()
		c.Overlays().Add(e.capa)
		e.capa.Resize(c.Size())
		// Sin nada enfocado: así las flechas, Espacio, «i» y «z» llegan al visor.
		c.Unfocus()
	}
}

// abrirEnGaleria abre una foto de la línea de tiempo por su id.
func (a *App) abrirEnGaleria(id string) {
	if i := posicion(a.g.fotos, id); i >= 0 {
		a.abrirVisor(desdeGaleria, i)
	}
}

// AbrirFoto abre una foto suelta en el visor: la que llega por un enlace de Drive.
func (a *App) AbrirFoto(f api.Photo) { a.abrirVisor(desdeSuelta, 0, f) }

// cerrarVisor retira la capa y para el pase.
func (a *App) cerrarVisor() {
	e := a.vis
	if e == nil {
		return
	}
	a.pararPase()
	if g := a.v.TecaGrande; g != nil && g != a.v.Teca {
		g.Cancelar()
	}
	a.vis = nil
	if a.w != nil {
		a.w.Canvas().Overlays().Remove(e.capa)
	}
}

// pintarVisor rehace el visor con su estado.
func (a *App) pintarVisor() {
	a.v.Variante = varianteActual()
	e := a.vis
	if g := a.v.TecaGrande; g != nil && g != a.v.Teca {
		g.Olvidar() // el lienzo de antes ya no se ve
	}
	f, ok := e.foto()
	if !ok {
		a.cerrarVisor()
		return
	}
	d := Visor{
		Foto: f, Descripcion: f.Description, Etiquetas: f.Tags,
		ConFicha: e.conFicha, AnchoFicha: a.medidaFicha(prefAnchoFicha, AnchoFicha, AnchoFichaMin, AnchoFichaMax),
		AltoFicha: a.medidaFicha(prefAltoFicha, AltoFicha, AltoFichaMin, AltoFichaMax), Fondo: a.fondoLienzo(),
		Pase: e.pase, TamanyoReal: e.tamanyoReal, Original: e.originalDe(f.ID, a.Huella(f.ID)), Aviso: e.aviso,
		HayAnterior: e.i > 0, HaySiguiente: e.haySiguiente(),
		Borrando: e.borrando, Girando: e.girando, Describiendo: e.describiendo,
		ConFavorita: e.origen == desdeGaleria, ConBorrar: e.origen == desdeGaleria,
		ConDescribir: e.origen == desdeGaleria || e.origen == desdeAlbum,
		Pregunta:     e.pregunta, Respuesta: e.respuesta, Pensando: e.pensando,
		EditandoDesc: e.editandoDesc, GuardandoDesc: e.guardandoDesc, CampoDesc: e.campoDesc,

		AlCerrar:          a.cerrarVisor,
		AlAnterior:        a.anterior,
		AlSiguiente:       a.siguiente,
		AlFavorita:        func() { a.cambiarFavorita(f, !f.Favorite) },
		AlDescribir:       a.describirFoto,
		AlGirar:           a.girar,
		AlPase:            a.alternarPase,
		AlZoom:            a.alternarTamanyoReal,
		AlZoomEn:          a.tamanyoRealEn,
		Desplazamiento:    e.desplazamiento,
		AlDesplazar:       func(p fyne.Position) { e.desplazamiento = p },
		AlFicha:           func() { e.conFicha = !e.conFicha; a.pintarVisor() },
		AlVerIA:           func() { e.conFicha = true; a.pintarVisor() },
		AlBorrar:          func() { e.borrando = true; a.pintarVisor() },
		AlCancelarBorrar:  func() { e.borrando = false; a.pintarVisor() },
		AlConfirmarBorrar: func() { a.tirarDelVisor(f) },
		AlParecidas:       func() { a.AbrirParecidas([]string{f.ID}) },
		AlCerca:           func() { a.AbrirCerca(f.ID) },
		AlDescargar:       func(formato string) { a.descargar(f, formato) },
		AlFondo:           a.ponerFondo,
		AlMenuAlbumes:     a.conAlbumes,
		AlAnyadirA:        func(al api.Album) { a.anyadirFotoA(f, al) },
		AlPreguntar:       a.preguntarFoto,
		AlAnchoFicha:      func(m float32, _ bool) { a.ponerMedidaFicha(prefAnchoFicha, m, AnchoFichaMin, AnchoFichaMax) },
		AlAltoFicha:       func(m float32, _ bool) { a.ponerMedidaFicha(prefAltoFicha, m, AltoFichaMin, AltoFichaMax) },
		AlEditarDesc:      a.editarDescripcion,
		AlCancelarDesc:    func() { e.editandoDesc = false; a.pintarVisor() },
		AlGuardarDesc:     a.guardarDescripcion,
	}
	d.AlEditar = func() { a.AbrirEditor(f, "") }
	if l := f.Location; l != nil && a.AbrirWeb != nil {
		// El mapa es un ENLACE y no un mapa incrustado: así la posición de la foto
		// solo sale de aquí si se pulsa.
		lat, lon := strconv.FormatFloat(l.Lat, 'f', -1, 64), strconv.FormatFloat(l.Lon, 'f', -1, 64)
		d.AlMapa = func() {
			a.AbrirWeb("https://www.openstreetmap.org/?mlat=" + lat + "&mlon=" + lon + "#map=15/" + lat + "/" + lon)
		}
	}
	if e.origen == desdeAlbum {
		d.Accion = botonQuitarDelAlbum(a.v, func() { a.quitarDelAlbum(f) })
	}
	e.capa.poner(PantallaVisor(a.v, d))
	if g := a.v.TecaGrande; g != nil {
		l := e.lista()
		for _, j := range [...]int{e.i - 1, e.i + 1} {
			if j >= 0 && j < len(l) && a.Huella(l[j].ID) == "" {
				g.Precargar(l[j].ID)
			}
		}
	}
}

func (a *App) anterior() {
	if e := a.vis; e != nil && e.i > 0 {
		e.i--
		e.nuevaFoto()
		a.pintarVisor()
	}
}

func (a *App) siguiente() {
	e := a.vis
	if e == nil {
		return
	}
	switch {
	case e.i < len(e.lista())-1:
		e.i++
		e.nuevaFoto()
		a.pintarVisor()
	case e.origen == desdeGaleria && !a.g.fin:
		a.g.siguienteAlLlegar = true
		a.cargarMas()
	case e.pase:
		// El pase se para solo en la última foto.
		a.pararPase()
		a.pintarVisor()
	}
}

// teclaVisor son los atajos del `onKeyDown` del `<dialog>`.
func (a *App) teclaVisor(k *fyne.KeyEvent) {
	e := a.vis
	switch k.Name {
	case fyne.KeyLeft:
		a.anterior()
	case fyne.KeyRight:
		a.siguiente()
	case fyne.KeyEscape:
		a.cerrarVisor()
	case fyne.KeyDelete, fyne.KeyBackspace:
		if e.origen == desdeGaleria {
			e.borrando = true
			a.pintarVisor()
		}
	default:
	}
}

// runaVisor: Espacio, «i» y «z». El Espacio llega como runa, no como tecla.
func (a *App) runaVisor(r rune) {
	e := a.vis
	switch r {
	case ' ':
		a.alternarPase()
	case 'i', 'I':
		e.conFicha = !e.conFicha
		a.pintarVisor()
	case 'z', 'Z':
		a.alternarTamanyoReal()
	}
}

// alternarTamanyoReal es la «z» y el botón: la foto a su tamaño con su centro en el
// centro del hueco.
func (a *App) alternarTamanyoReal() { a.tamanyoRealEn(0.5, 0.5, fyne.Position{}) }

// tamanyoRealEn es el clic en la foto: la foto a su tamaño, pintada con el ORIGINAL que se
// pide la primera vez, con el punto pulsado (en fracciones de la foto) bajo el puntero
// (`vista`, en el hueco). Sin punto —la «z»—, el centro en el centro. Abría siempre por la
// esquina de arriba a la izquierda (`ancla` en PhotosApp.tsx).
func (a *App) tamanyoRealEn(fx, fy float32, vista fyne.Position) {
	e := a.vis
	if !e.tamanyoReal {
		f, _ := e.foto()
		hueco := a.huecoVisor()
		if vista == (fyne.Position{}) {
			vista = fyne.NewPos(hueco.Width/2, hueco.Height/2)
		}
		e.desplazamiento = fyne.NewPos(max(fx*float32(f.Width)-vista.X, 0), max(fy*float32(f.Height)-vista.Y, 0))
	}
	e.tamanyoReal = !e.tamanyoReal
	a.pintarVisor()
	if f, ok := e.foto(); ok && e.tamanyoReal {
		a.pedirOriginal(f)
	}
}

func (e *estadoVisor) originalDe(id, huella string) image.Image {
	if o := e.original; o != nil && o.id == id && o.huella == huella {
		return o.img
	}
	return nil
}

// pedirOriginal trae la foto entera con su receta —la de «Descargar», en su formato— y
// la decodifica fuera del hilo de la interfaz.
func (a *App) pedirOriginal(f api.Photo) {
	e := a.vis
	huella := a.Huella(f.ID)
	if a.cli == nil || e.originalDe(f.ID, huella) != nil || e.pidiendo == f.ID {
		return
	}
	e.pidiendo = f.ID
	formato := "jpeg"
	if strings.Contains(f.ContentType, "png") {
		formato = "png"
	}
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := context.WithTimeout(vida, 2*time.Minute)
		defer fin()
		var img image.Image
		r, err := a.cli.Export(c, f.ID, formato)
		if err == nil {
			img, _, err = image.Decode(r)
			_ = r.Close()
		}
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if a.vis != e || e.pidiendo != f.ID {
				return
			}
			e.pidiendo = ""
			if err == nil {
				e.original = &fotoOriginal{id: f.ID, huella: huella, img: img}
				a.pintarVisor()
			}
		})
	})
}

// alternarPase enciende o apaga el pase de cuatro segundos por foto.
func (a *App) alternarPase() {
	e := a.vis
	if e.pase {
		a.pararPase()
		a.pintarVisor()
		return
	}
	if !e.haySiguiente() {
		return
	}
	e.pase = true
	parar := make(chan struct{})
	e.parar = parar
	gen := a.gen
	go func() {
		t := time.NewTicker(DuracionPase)
		defer t.Stop()
		for {
			select {
			case <-parar:
				return
			case <-t.C:
				a.hilo(gen, a.siguiente)
			}
		}
	}()
	a.pintarVisor()
}

func (a *App) pararPase() {
	if e := a.vis; e != nil && e.pase {
		e.pase = false
		close(e.parar)
	}
}

// tirarDelVisor manda la foto a la papelera y cierra el visor.
func (a *App) tirarDelVisor(f api.Photo) {
	if a.cli == nil {
		return
	}
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		err := a.cli.Trash(c, f.ID)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if err != nil {
				a.g.error = a.textoDeFallo(err)
			} else {
				a.g.fotos = quitarFoto(a.g.fotos, f.ID)
				a.cerrarVisor()
			}
			a.pintar()
		})
	})
}

// girar añade un paso «giro» a la receta. La variante nueva se pide con la huella que
// devuelve el servicio (ver Huella).
func (a *App) girar() {
	e := a.vis
	f, ok := e.foto()
	if !ok || e.girando || a.cli == nil {
		return
	}
	e.girando = true
	a.pintarVisor()
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		huella, err := a.cli.Rotar(c, f.ID)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if a.vis != e {
				return
			}
			e.girando = false
			if err != nil {
				e.aviso = a.v.Traduce("viewer.rotateFailed")
			} else {
				a.huellas.Store(f.ID, huella)
				e.lados[f.ID] = [2]int{f.Height, f.Width}
				if a.v.TecaGrande != nil {
					a.v.TecaGrande.Tirar(f.ID)
				}
				e.aviso = a.v.Traduce("viewer.rotated")
			}
			a.pintarVisor()
		})
	})
}

// Huella es la de la receta de una foto girada en esta sesión, para pedir su variante
// nueva (`urlDeVarianteCon`). Vacía, la de siempre. Se lee desde las goroutines de la
// Fototeca, y por eso va en un sync.Map.
func (a *App) Huella(id string) string {
	h, _ := a.huellas.Load(id)
	s, _ := h.(string)
	return s
}

// descargar exporta la foto con su receta aplicada y la deja en la carpeta de
// descargas, que es lo que hace el navegador con `Content-Disposition: attachment`.
func (a *App) descargar(f api.Photo, formato string) {
	if a.cli == nil || a.Guardar == nil {
		return
	}
	gen := a.gen
	a.avisarVisor(a.v.T.T("gallery.downloading", i18n.Vars{"count": 1}))
	a.enSesion(func(vida context.Context) {
		err := a.bajar(vida, f, formato)
		if err != nil {
			//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
			a.hilo(gen, func() { a.avisarVisor(a.textoDeFallo(err)) })
		}
	})
}

// bajar trae una exportación y la guarda. Corre FUERA del hilo de la interfaz.
func (a *App) bajar(vida context.Context, f api.Photo, formato string) error {
	c, fin := ctx(vida)
	defer fin()
	r, err := a.cli.Export(c, f.ID, formato)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	datos, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if err := vida.Err(); err != nil {
		return err
	}
	_, err = a.Guardar(nombreDescarga(f.Name, formato), datos)
	return err
}

// nombreDescarga es el nombre de la foto con la extensión del formato exportado.
func nombreDescarga(nombre, formato string) string {
	ext := ".jpg"
	if formato == "png" {
		ext = ".png"
	}
	return strings.TrimSuffix(nombre, path.Ext(nombre)) + ext
}

func (a *App) avisarVisor(s string) {
	if a.vis != nil {
		a.vis.aviso = s
		a.pintarVisor()
	}
}

// conAlbumes da la lista de álbumes a `mostrar`, pidiéndola si todavía no se tiene. La
// web la pide al abrir el selector, no al abrir el visor, para no pagar una petición por
// cada foto que se mira.
func (a *App) conAlbumes(mostrar func([]api.Album)) {
	if a.albumes != nil {
		mostrar(a.albumes)
		return
	}
	if a.cli == nil {
		return
	}
	gen, dir := a.gen, a.Directorio
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		lista, err := a.cli.Albums(c)
		if err != nil {
			lista = []api.Album{}
		}
		lista = append(lista, editablesConmigo(c, a.cli, dir)...)
		a.hilo(gen, func() {
			a.albumes = lista
			mostrar(lista)
		})
	})
}

// editablesConmigo son los álbumes que otras personas me han compartido PARA EDITAR,
// para «Añadir a un álbum…»: lo que se añade a uno se copia al Drive de su dueña.
func editablesConmigo(c context.Context, cli *api.Photos, dir *api.Directorio) []api.Album {
	compartidos, err := cli.AlbumesCompartidos(c)
	if err != nil {
		return nil
	}
	var editables []api.AlbumCompartido
	for _, al := range compartidos {
		if al.Permiso == api.NivelEditar {
			editables = append(editables, al)
		}
	}
	nombres := nombresDe(c, dir, editables)
	out := make([]api.Album, 0, len(editables))
	for _, al := range editables {
		por := nombres[al.Por]
		if por == "" {
			por = al.Por
		}
		out = append(out, api.Album{ID: al.ID, Title: al.Title, Count: al.Count, CompartidoPor: por})
	}
	return out
}

// anyadirFotoA mete la foto del visor en un álbum.
func (a *App) anyadirFotoA(f api.Photo, al api.Album) {
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		_, err := a.cli.AddToAlbum(c, al.ID, []string{f.ID})
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if err != nil {
				a.avisarVisor(a.textoDeFallo(err))
				return
			}
			a.avisarVisor(a.v.T.T("albums.added", i18n.Vars{"title": al.Title}))
		})
	})
}

// quitarDelAlbum es el mando extra del visor de un álbum.
func (a *App) quitarDelAlbum(f api.Photo) {
	gen, album := a.gen, a.uno.id
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		err := a.cli.RemoveFromAlbum(c, album, f.ID)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if err != nil {
				a.uno.error = a.textoDeFallo(err)
			} else {
				a.uno.fotos = quitarFoto(a.uno.fotos, f.ID)
				a.cerrarVisor()
			}
			a.pintar()
		})
	})
}

// ---- Las preferencias del visor en este equipo ----

func (a *App) prefs() fyne.Preferences {
	if app := fyne.CurrentApp(); app != nil {
		return app.Preferences()
	}
	return nil
}

// medidaFicha es el ancho (o el alto, en móvil) de la ficha recordado en este equipo,
// dentro de sus topes: el `leerMedida` de la web.
func (a *App) medidaFicha(clave string, defecto, lo, hi float32) float32 {
	if p := a.prefs(); p != nil {
		if n := float32(p.FloatWithFallback(clave, float64(defecto))); n > 0 {
			return min(max(n, lo), hi)
		}
	}
	return defecto
}

// ponerMedidaFicha fija la medida dentro de sus topes y la recuerda. Las preferencias de
// Fyne se guardan solas, así que no hace falta separar arrastrar de soltar como la web.
func (a *App) ponerMedidaFicha(clave string, m, lo, hi float32) {
	m = min(max(float32(int(m+0.5)), lo), hi)
	if p := a.prefs(); p != nil {
		p.SetFloat(clave, float64(m))
	}
	if a.vis != nil {
		a.pintarVisor()
	}
}

func (a *App) fondoLienzo() FondoLienzo {
	if p := a.prefs(); p != nil {
		f := FondoLienzo(p.StringWithFallback(prefFondo, string(FondoCrema)))
		for _, x := range FondosLienzo {
			if x == f {
				return f
			}
		}
	}
	return FondoCrema
}

func (a *App) ponerFondo(f FondoLienzo) {
	if p := a.prefs(); p != nil {
		p.SetString(prefFondo, string(f))
	}
	if a.vis != nil {
		a.pintarVisor()
	}
}

// ---- La capa ----

// capaVisor es la capa a pantalla completa. Es un widget —y no un contenedor— porque
// la pila de capas pregunta por `Descartable` para saber a quién cerrar con Escape.
type capaVisor struct {
	widget.BaseWidget
	dentro   fyne.CanvasObject
	alCerrar func()
}

var (
	_ fyne.Widget    = (*capaVisor)(nil)
	_ ui.Descartable = (*capaVisor)(nil)
)

func nuevaCapaVisor(alCerrar func()) *capaVisor {
	c := &capaVisor{alCerrar: alCerrar}
	c.ExtendBaseWidget(c)
	return c
}

// Descartar es el `onClose` del `<dialog>`.
func (c *capaVisor) Descartar() { c.alCerrar() }

func (c *capaVisor) poner(o fyne.CanvasObject) {
	c.dentro = o
	c.Refresh()
}

func (c *capaVisor) CreateRenderer() fyne.WidgetRenderer { return &capaVisorRenderer{c: c} }

type capaVisorRenderer struct{ c *capaVisor }

func (r *capaVisorRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 0) }
func (r *capaVisorRenderer) Objects() []fyne.CanvasObject {
	if r.c.dentro == nil {
		return nil
	}
	return []fyne.CanvasObject{r.c.dentro}
}
func (r *capaVisorRenderer) Destroy() {}
func (r *capaVisorRenderer) Layout(t fyne.Size) {
	if r.c.dentro != nil {
		r.c.dentro.Move(fyne.NewPos(0, 0))
		r.c.dentro.Resize(t)
	}
}

func (r *capaVisorRenderer) Refresh() {
	r.Layout(r.c.Size())
	if r.c.dentro != nil {
		r.c.dentro.Refresh()
	}
}

// huecoVisor es lo que mide el hueco de la foto en la ventana: sin la cabecera y, si está
// abierta, sin la ficha (a la derecha, o debajo en un teléfono). Es el reparto de
// visorLayout, sin la banda de borrar, que no cambia dónde cae el centro a lo ancho.
func (a *App) huecoVisor() fyne.Size {
	if a.w == nil || a.vis == nil {
		return fyne.NewSize(0, 0)
	}
	t := a.w.Canvas().Size()
	w, h := t.Width, t.Height-altoCabeceraVisor(a.v.Ventana)
	if a.vis.conFicha {
		if movil(a.v.Ventana) {
			h -= a.medidaFicha(prefAltoFicha, AltoFicha, AltoFichaMin, AltoFichaMax) + AnchoAsaFicha
		} else {
			w -= a.medidaFicha(prefAnchoFicha, AnchoFicha, AnchoFichaMin, AnchoFichaMax) + AnchoAsaFicha
		}
	}
	return fyne.NewSize(max(w, 0), max(h, 0))
}
