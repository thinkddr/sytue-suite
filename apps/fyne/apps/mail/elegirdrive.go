// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"image/color"
	"net/http"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Adjuntar desde Drive» (`DrivePickerModal.tsx`)
//
// Se navega Mi unidad con sus migas y se elige un fichero, que va al correo como
// ENLACE (`{drive}/files/{id}`): el permiso lo sigue aplicando Drive a quien lo abra.
// Las carpetas llegan enteras en la primera página y los ficheros, por páginas.

// ExploradorDrive es lo que el selector le pide a Drive. Lo cumple *api.Drive.
type ExploradorDrive interface {
	Root(ctx context.Context) (api.Folder, error)
	Children(ctx context.Context, carpeta, cursor string) (api.Children, error)
}

var _ ExploradorDrive = (*api.Drive)(nil)

// EnlaceDrive es un adjunto de Drive: su nombre y el enlace que va en el correo.
type EnlaceDrive struct{ Nombre, URL string }

// EnlaceDeFichero es el `link` del BFF: la URL pública de Drive y el id del fichero.
func EnlaceDeFichero(drive, id string) string {
	return strings.TrimSuffix(drive, "/") + "/files/" + url.PathEscape(id)
}

// TextoConDrive es `composedTextForDraft`: el cuerpo y, si hay adjuntos de Drive, su
// encabezado y un «nombre: enlace» por línea.
func TextoConDrive(t *i18n.Translator, cuerpo string, enlaces []EnlaceDrive) string {
	if len(enlaces) == 0 {
		return cuerpo
	}
	lineas := make([]string, 0, len(enlaces))
	for _, e := range enlaces {
		lineas = append(lineas, e.Nombre+": "+e.URL)
	}
	return cuerpo + "\n\n" + t.T("compose.driveAttachmentsHeading", nil) + "\n" + strings.Join(lineas, "\n")
}

// HTMLConDrive es `driveLinksHtml`: el cuerpo con formato y, si hay adjuntos de Drive,
// su encabezado en un párrafo y un enlace por elemento de lista.
func HTMLConDrive(t *i18n.Translator, html string, enlaces []EnlaceDrive) string {
	if len(enlaces) == 0 {
		return html
	}
	var b strings.Builder
	b.WriteString(html + "<p>" + htmltext.Escape(t.T("compose.driveAttachmentsHeading", nil)) + "</p><ul>")
	for _, e := range enlaces {
		b.WriteString(`<li><a href="` + strings.ReplaceAll(e.URL, `"`, "%22") + `">` + htmltext.Escape(e.Nombre) + "</a></li>")
	}
	b.WriteString("</ul>")
	return b.String()
}

// Medidas del selector, de la web.
const (
	altoListaDrive  float32 = 72 * 0.25 * tema.Rem // `h-72`
	margenXFilaDrv  float32 = 3 * 0.25 * tema.Rem  // `px-3`
	margenYFilaDrv  float32 = 2 * 0.25 * tema.Rem  // `py-2`
	huecoFilaDrv    float32 = 2.5 * 0.25 * tema.Rem
	glifoFilaDrv    float32 = 16
	galonFilaDrv    float32 = 14
	huecoMigas      float32 = 0.5 * 0.25 * tema.Rem
	aireMigas       float32 = 2 * 0.25 * tema.Rem // `mb-2`
	airePieSelector float32 = 2 * 0.25 * tema.Rem // `mt-2`
)

type selectorDrive struct {
	b        *Buzon
	d        *ui.Dialogo
	caja     *fyne.Container
	ancho    float32
	alElegir func(EnlaceDrive)

	// ruta son las carpetas desde Mi unidad hasta la que se ve.
	ruta        []api.Folder
	carpetas    []api.Folder
	ficheros    []api.File
	cursor      string
	estado      Estado
	sinSesion   bool
	cargandoMas bool
}

// elegirDeDrive abre el selector sobre el redactor. Elegir añade el enlace (sin
// repetirlo), cierra y lo dice.
func (b *Buzon) elegirDeDrive(rd *redaccion) *selectorDrive {
	return b.verSelectorDrive(func(e EnlaceDrive) {
		rd.r.AdjuntarDrive(e)
		b.avisar(ui.AvisoExito, "compose.driveAttachmentAdded", nil)
	})
}

func (b *Buzon) verSelectorDrive(alElegir func(EnlaceDrive)) *selectorDrive {
	lienzo := b.P.Lienzo()
	if lienzo == nil || b.Explorador == nil {
		return nil
	}
	t := b.P.T
	g := &selectorDrive{b: b, ancho: AnchoCuerpoDialogo(ui.DialogoMD, lienzo.Size().Width), alElegir: alElegir}
	g.caja = container.New(ui.Columna{})
	cancelar := ui.NuevoBoton(t.T("common.cancel", nil), nil).Con(ui.Fantasma, ui.TallaMD)
	g.d = ui.NuevoDialogo(t.T("drive.title", nil), g.caja).Con(ui.DialogoMD, cancelar)
	cancelar.AlPulsar = g.d.Cerrar
	g.d.RotuloCerrar = t.T("ui.close", nil)
	g.d.Mostrar(lienzo)
	g.abrir(nil)
	return g
}

// reentrar es «Volver a iniciar sesión»: la sesión de la suite abre Sytue ID y, con
// la sesión renovada, se vuelve a pedir la carpeta donde se estaba.
func (g *selectorDrive) reentrar() {
	g.estado = Cargando
	g.pintar()
	pedir(g.b, func(ctx context.Context) (struct{}, error) { return struct{}{}, g.b.Entrar(ctx) },
		func(_ struct{}, err error) {
			if err != nil {
				g.estado = Error
				g.pintar()
				return
			}
			g.abrir(g.ruta)
		})
}

// abrir enseña una carpeta: nil es Mi unidad, que primero hay que pedir.
func (g *selectorDrive) abrir(hasta []api.Folder) {
	g.ruta, g.carpetas, g.ficheros, g.cursor = hasta, nil, nil, ""
	g.estado, g.sinSesion = Cargando, false
	g.pintar()
	pedir(g.b, func(ctx context.Context) (paginaDrive, error) {
		r := paginaDrive{ruta: hasta}
		if len(r.ruta) == 0 {
			raiz, err := g.b.Explorador.Root(ctx)
			if err != nil {
				return r, err
			}
			r.ruta = []api.Folder{raiz}
		}
		var err error
		r.hijos, err = g.b.Explorador.Children(ctx, r.ruta[len(r.ruta)-1].ID, "")
		return r, err
	}, func(r paginaDrive, err error) {
		if err != nil {
			var e *api.Error
			g.estado, g.sinSesion = Error, errors.As(err, &e) && e.Status == http.StatusUnauthorized
			g.pintar()
			return
		}
		g.ruta, g.carpetas, g.ficheros, g.cursor = r.ruta, r.hijos.Folders, r.hijos.Files, r.hijos.NextCursor
		g.estado = Listo
		g.pintar()
	})
}

// paginaDrive es una carpeta abierta: la ruta hasta ella y su primera página.
type paginaDrive struct {
	ruta  []api.Folder
	hijos api.Children
}

// masFicheros es `loadMore`: la página siguiente, a continuación.
func (g *selectorDrive) masFicheros() {
	if g.cursor == "" || g.cargandoMas || len(g.ruta) == 0 {
		return
	}
	g.cargandoMas = true
	g.pintar()
	carpeta, cursor := g.ruta[len(g.ruta)-1].ID, g.cursor
	pedir(g.b, func(ctx context.Context) (api.Children, error) {
		return g.b.Explorador.Children(ctx, carpeta, cursor)
	}, func(h api.Children, err error) {
		g.cargandoMas = false
		if err != nil {
			g.b.avisar(ui.AvisoPeligro, "drive.loadMoreFailed", nil)
		} else {
			g.carpetas = append(g.carpetas, h.Folders...)
			g.ficheros = append(g.ficheros, h.Files...)
			g.cursor = h.NextCursor
		}
		g.pintar()
	})
}

func (g *selectorDrive) elegir(f api.File) {
	if g.b.URLDrive == "" {
		return // sin la URL pública no hay enlace que poner (`disabled={!it.link}`)
	}
	g.d.Cerrar()
	g.alElegir(EnlaceDrive{Nombre: f.Name, URL: EnlaceDeFichero(g.b.URLDrive, f.ID)})
}

func (g *selectorDrive) pintar() {
	t, l := g.b.P.T, g.b.P.Locale
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())

	// Las migas: «Mi unidad» y cada carpeta, con el galón de 14 entre medias.
	// Los segmentos de `ui` con el estilo de diálogo, pero sueltos y en `flex-wrap`: la
	// ruta del selector no se estrecha como la del explorador, parte de línea, y la
	// carpeta actual va en `font-medium` como las demás (ninguna lleva `aria-current`
	// en semi-negrita aquí).
	miga := func(rotulo string, ir func()) fyne.CanvasObject {
		m := ui.NuevaMiga(rotulo, ir)
		m.Estilo = ui.MigasEnDialogo
		return m
	}
	migas := []fyne.CanvasObject{miga(t.T("drive.myDrive", nil), func() { g.abrir(nil) })}
	for i := 1; i < len(g.ruta); i++ {
		hasta := g.ruta[:i+1]
		migas = append(migas, container.New(fijo{galonFilaDrv, galonFilaDrv}, iconos.Trazo(iconos.ChevronDerecha, galonFilaDrv, "text-subtle", v).Objeto()),
			miga(g.ruta[i].Name, func() { g.abrir(append([]api.Folder(nil), hasta...)) }))
	}

	var dentro fyne.CanvasObject
	centrado := func(o fyne.CanvasObject) fyne.CanvasObject { return container.NewCenter(o) }
	switch {
	case g.estado == Cargando:
		rueda := ui.NuevaRueda(20, "text-subtle", v)
		rueda.Rotulo = t.T("drive.loadingFiles", nil)
		dentro = centrado(rueda)
	case g.estado == Error:
		titulo, desc := t.T("drive.errorTitle", nil), t.T("drive.errorDescription", nil)
		if g.sinSesion {
			titulo, desc = t.T("drive.sessionExpiredTitle", nil), t.T("drive.sessionExpiredDescription", nil)
		}
		vacio := ui.NuevoVacio(titulo, desc)
		vacio.Ilustracion = iconos.Trazo(iconos.Aviso, 36, "danger-text", v).Objeto()
		switch {
		case !g.sinSesion:
			vacio.Accion = ui.NuevoBoton(t.T("common.retry", nil), func() { g.abrir(g.ruta) }).Con(ui.Secundario, ui.TallaSM)
		case g.b.Entrar != nil:
			// La web recarga la página, que es volver a pasar por Sytue ID; aquí es
			// la sesión de la suite, y al volver se sigue en la misma carpeta.
			vacio.Accion = ui.NuevoBoton(t.T("drive.signInAgain", nil), g.reentrar).Con(ui.Primario, ui.TallaSM)
		}
		dentro = container.New(margen(tema.Espacio(4)), centrado(vacio))
	case len(g.carpetas)+len(g.ficheros) == 0:
		dentro = centrado(ui.NuevoRotulo(t.T("drive.emptyFolder", nil), tema.TextoBase, ui.Normal, "text-muted"))
	default:
		filas := make([]fyne.CanvasObject, 0, len(g.carpetas)+len(g.ficheros)+1)
		for _, c := range g.carpetas {
			hasta := append(append([]api.Folder(nil), g.ruta...), c)
			filas = append(filas, nuevaFilaDrive(Glifo(CarpetaD, glifoFilaDrv, "primary-text", v), c.Name, ui.Medio,
				container.New(fijo{galonFilaDrv, galonFilaDrv}, iconos.Trazo(iconos.ChevronDerecha, galonFilaDrv, "text-subtle", v).Objeto()),
				func() { g.abrir(hasta) }))
		}
		for _, f := range g.ficheros {
			tam := ui.NuevoRotulo(i18n.FormatBytes(l, float64(f.SizeBytes)), tema.TextoXS, ui.Normal, "text-subtle")
			filas = append(filas, nuevaFilaDrive(iconos.Trazo(iconos.Enlace, glifoFilaDrv, "text-muted", v), f.Name, ui.Normal,
				tam, func() { g.elegir(f) }))
		}
		if g.cursor != "" {
			mas := ui.NuevoBoton(t.T("common.loadMore", nil), g.masFicheros).Con(ui.Secundario, ui.TallaSM)
			mas.Cargando = g.cargandoMas
			filas = append(filas, container.New(ui.Margen{Arr: tema.Espacio(3), Abj: tema.Espacio(3)}, container.NewCenter(mas)))
		}
		dentro = container.NewVScroll(container.New(ui.Columna{}, filas...))
	}
	lista := enCajaDe(tema.Color("surface-raised", v), v, container.New(fijo{alto: altoListaDrive - 2*tema.Borde}, dentro))

	g.caja.Objects = []fyne.CanvasObject{
		container.New(ui.Columna{Hueco: aireMigas}, container.New(flujo{hueco: huecoMigas, ancho: g.ancho}, migas...), lista),
		container.New(ui.Margen{Arr: airePieSelector},
			ui.NuevoParrafo(t.T("drive.footer", nil), tema.TextoSM, ui.Normal, "text-subtle", v).A(g.ancho)),
	}
	g.caja.Refresh()
	if g.d != nil {
		g.d.Recolocar() // el diálogo se vuelve a medir con su contenido nuevo
	}
}

// fijo le da al hijo un tamaño fijo: la caja `h-72` (sin ancho: el que haya) y el
// galón de 14, que como ráster no declara mínimo.
type fijo struct{ ancho, alto float32 }

func (f fijo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(f.ancho, f.alto) }

func (f fijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// ---- Una fila del selector y una miga ----

// filaDrive es `flex items-center gap-2.5 border-b border-border-subtle px-3 py-2`: el
// glifo, el nombre recortado y lo del final (el galón o el tamaño).
type filaDrive struct {
	ui.Pulsable
	glifo  *iconos.Dibujo
	fondo  *canvas.Rectangle
	nombre string
	peso   ui.Peso
	cola   fyne.CanvasObject
}

func nuevaFilaDrive(glifo *iconos.Dibujo, nombre string, peso ui.Peso, cola fyne.CanvasObject, alPulsar func()) *filaDrive {
	f := &filaDrive{glifo: glifo, nombre: nombre, peso: peso, cola: cola}
	f.Nombre, f.AlPulsar = nombre, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// MinSize: `py-2` y la caja de línea de `text-base`, más el filete de abajo.
func (f *filaDrive) MinSize() fyne.Size {
	return fyne.NewSize(0, 2*margenYFilaDrv+tema.TextoBase.Linea+tema.Borde)
}

func (f *filaDrive) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaDriveRenderer{
		f:      f,
		filete: canvas.NewRectangle(tema.Color("border-subtle", v)),
		texto:  ui.Texto(f.nombre, tema.TextoBase, f.peso, tema.Color("text", v)),
	}
	f.fondo = canvas.NewRectangle(color.NRGBA{})
	r.objetos = []fyne.CanvasObject{f.fondo, r.filete, f.glifo.Objeto(), r.texto, f.cola}
	r.Refresh()
	return r
}

type filaDriveRenderer struct {
	f       *filaDrive
	filete  *canvas.Rectangle
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *filaDriveRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filaDriveRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaDriveRenderer) Destroy()                     {}

// Refresh pinta `hover:bg-bg-subtle` (y el mismo velo con el foco: `focus-visible:bg-bg-subtle`).
func (r *filaDriveRenderer) Refresh() {
	r.f.fondo.FillColor = color.NRGBA{}
	if r.f.Encima() || r.f.Enfocado() {
		r.f.fondo.FillColor = tema.Color("bg-subtle", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	}
	r.f.fondo.Refresh()
	r.Layout(r.f.Size())
}

func (r *filaDriveRenderer) Layout(t fyne.Size) {
	alto := t.Height - tema.Borde
	r.f.fondo.Move(fyne.NewPos(0, 0))
	r.f.fondo.Resize(fyne.NewSize(t.Width, alto))
	r.filete.Move(fyne.NewPos(0, alto))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	r.f.glifo.Colocar(margenXFilaDrv, (alto-glifoFilaDrv)/2, glifoFilaDrv)
	mc := r.f.cola.MinSize()
	xc := t.Width - margenXFilaDrv - mc.Width
	r.f.cola.Move(fyne.NewPos(xc, (alto-mc.Height)/2))
	r.f.cola.Resize(mc)
	x := margenXFilaDrv + glifoFilaDrv + huecoFilaDrv
	ui.Truncar(r.texto, r.f.nombre, tema.TextoBase, r.f.peso, max(xc-huecoFilaDrv-x, 0))
	ui.ColocarTexto(r.texto, tema.TextoBase, x, (alto-tema.TextoBase.Linea)/2)
}
