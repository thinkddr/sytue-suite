// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Las vistas planas: «Recientes» y «Destacados»
//
// `ResourceListView` de FileBrowser.tsx: la cabecera de sección con su glifo, y la lista
// de lo que el servicio devuelve con la ruta ya resuelta. Se abre, se descarga, se
// comparte y se pone o quita la estrella; renombrar, mover y borrar viven en «Mi
// unidad», con sus diálogos, y aquí no se ofrecen. «Recientes» pagina; «Destacados»
// llega entero.

// metaSeccion son el glifo y las CLAVES de cada vista plana (`SECTION_META`).
type metaSeccion struct {
	glifo                                                   string
	icono                                                   iconos.Icono
	titulo, subtitulo, error, vacioTitulo, vacioDescripcion string
}

var metaSecciones = map[Seccion]metaSeccion{
	Recientes: {
		glifo: glifoReloj, titulo: "recent.title", subtitulo: "recent.subtitle",
		error: "recent.errorTitle", vacioTitulo: "recent.emptyTitle", vacioDescripcion: "recent.emptyBody",
	},
	Favoritos: {
		icono: iconos.DriveEstrella, titulo: "favorites.title", subtitulo: "favorites.subtitle",
		error: "favorites.errorTitle", vacioTitulo: "favorites.emptyTitle", vacioDescripcion: "favorites.emptyBody",
	},
}

// VistaPlana es «Recientes» o «Destacados».
type VistaPlana struct {
	tr  *i18n.Translator
	loc i18n.Locale

	Seccion   Seccion
	Fase      Fase
	Elementos []Elemento
	// Favoritos son los ids con estrella; es el MISMO mapa de la pantalla.
	Favoritos map[string]bool
	// HayMas y CargandoMas: la paginación de «Recientes».
	HayMas, CargandoMas bool
	Desconectado        bool
	Ventana             float32

	AlAbrir      func(Elemento)
	AlEstrella   func(el Elemento, quiere bool)
	AlMenu       func(el Elemento, disparador fyne.CanvasObject)
	AlReintentar func()
	AlCargarMas  func()

	desplazable *container.Scroll
}

// NuevaVistaPlana crea la vista de una sección en su estado inicial: cargando.
func NuevaVistaPlana(tr *i18n.Translator, s Seccion, favoritos map[string]bool) *VistaPlana {
	return &VistaPlana{tr: tr, loc: tr.Locale(), Seccion: s, Favoritos: favoritos}
}

func (p *VistaPlana) t(clave string) string { return p.tr.T(clave, nil) }

// SinRed apaga la estrella y el «⋯» se queda con la descarga, como el `offline` de la web.
func (p *VistaPlana) SinRed() bool { return p.Fase == SinConexion || p.Desconectado }

// cabeceraSeccion es el `SectionHeader` con el glifo de 18 de la sección.
func cabeceraSeccion(tr *i18n.Translator, titulo, subtitulo string, glifo *iconos.Dibujo) *ui.CabeceraApp {
	cab := ui.NuevaCabeceraApp(tr.T(titulo, nil))
	cab.Subtitulo = tr.T(subtitulo, nil)
	cab.Icono = glifo.Objeto()
	return cab
}

// glifoSeccion pinta el glifo de la cabecera: 18 y `text-muted` (la caja del icono de
// SectionHeader es `text-text-muted`).
func glifoSeccion(cuerpo string, icono iconos.Icono, v tema.Variante) *iconos.Dibujo {
	if cuerpo != "" {
		return glifoCon(cuerpo, ui.GlifoMenu, tema.Color("text-muted", v))
	}
	return iconos.Trazo(icono, ui.GlifoMenu, "text-muted", v)
}

// Objeto arma la vista.
func (p *VistaPlana) Objeto(v tema.Variante) fyne.CanvasObject {
	m := metaSecciones[p.Seccion]
	cab := cabeceraSeccion(p.tr, m.titulo, m.subtitulo, glifoSeccion(m.glifo, m.icono, v))
	return container.New(Pila{Elastico: 1}, cab, p.cuerpo(v, m))
}

func (p *VistaPlana) cuerpo(v tema.Variante, m metaSeccion) fyne.CanvasObject {
	switch p.Fase {
	case Cargando:
		return (&Explorador{tr: p.tr, loc: p.loc, Ventana: p.Ventana}).esqueletoCuerpo(v)
	case Vacia:
		x := ui.NuevoVacio(p.t(m.vacioTitulo), p.t(m.vacioDescripcion))
		x.Ilustracion = ilustracion(iconos.VacioDrive, v)
		return centrar(x, v)
	case Fallo, SinSesion, Prohibida, SinConexion:
		return centrar(problemaVista(p.tr, p.Fase, m.error, p.AlReintentar, v), v)
	case Exito:
	}
	filas := make([]fyne.CanvasObject, 0, len(p.Elementos)+1)
	for _, el := range p.Elementos {
		filas = append(filas, p.fila(el))
	}
	if p.HayMas {
		filas = append(filas, pieCargarMas(p.tr, p.CargandoMas, p.AlCargarMas, v))
	}
	return p.lista(v, filas)
}

// lista mete las filas en el `px-3 pb-6` desplazable, y pide la página siguiente al
// acercarse al final: el `IntersectionObserver` de la web con su `rootMargin: 200px`.
func (p *VistaPlana) lista(v tema.Variante, filas []fyne.CanvasObject) fyne.CanvasObject {
	contenido := container.New(listaPlana{}, filas...)
	if p.desplazable == nil {
		p.desplazable = container.NewVScroll(contenido)
		p.desplazable.OnScrolled = func(pos fyne.Position) {
			if !p.HayMas || p.CargandoMas || p.AlCargarMas == nil {
				return
			}
			if pos.Y+p.desplazable.Size().Height >= p.desplazable.Content.MinSize().Height-MargenCargarMas {
				p.AlCargarMas()
			}
		}
	} else {
		p.desplazable.Content = contenido
		p.desplazable.Refresh()
	}
	return container.NewStack(canvas.NewRectangle(tema.Color("bg", v)), p.desplazable)
}

// MargenCargarMas es el `rootMargin: '200px'` del observador de «Recientes».
const MargenCargarMas float32 = 200

// fila es una fila de «Recientes» o «Destacados»: la estrella y el «⋯» siempre a la vista.
func (p *VistaPlana) fila(el Elemento) *FilaPlana {
	f := NuevaFilaPlana(el.Nombre)
	f.Extension, f.Carpeta = Extension(el.Nombre), el.Carpeta
	f.Meta = metaPlana(p.tr, p.loc, el, true)
	elemento := el
	if !el.Carpeta && p.AlAbrir != nil {
		f.AlAbrir = func() { p.AlAbrir(elemento) }
	}
	marcada := p.Favoritos[el.ID]
	estrella := BotonFila{Icono: iconos.DriveEstrella, Tinta: "text-subtle", Inerte: p.SinRed() || el.ID == ""}
	estrella.Rotulo = p.tr.T("files.star.add", i18n.Vars{"name": el.Nombre})
	if marcada {
		estrella.Tinta = "primary-text"
		estrella.Rotulo = p.tr.T("files.star.remove", i18n.Vars{"name": el.Nombre})
	}
	estrella.AlPulsar = func(fyne.CanvasObject) {
		if p.AlEstrella != nil {
			p.AlEstrella(elemento, !marcada)
		}
	}
	mas := BotonFila{
		Cuerpo: glifoMas, Tinta: "text-muted", TintaEncima: "text",
		Rotulo: p.tr.T("action.menu", i18n.Vars{"name": el.Nombre}),
	}
	mas.AlPulsar = func(d fyne.CanvasObject) {
		if p.AlMenu != nil {
			p.AlMenu(elemento, d)
		}
	}
	f.Botones = []BotonFila{estrella, mas}
	return f
}

// metaPlana es la línea de debajo del nombre: «Carpeta» o el tamaño, la fecha si la
// hay y, si se pide, la carpeta donde vive (`locationOf`).
func metaPlana(tr *i18n.Translator, l i18n.Locale, el Elemento, conUbicacion bool) string {
	meta := Tamano(l, el.Tamano)
	if el.Carpeta {
		meta = tr.T("files.folder", nil)
	}
	if f := Fecha(l, el.Modificado); f != "" {
		meta += " · " + f
	}
	if conUbicacion {
		meta += " · " + UbicacionDe(tr, el.Ruta)
	}
	return meta
}

// UbicacionDe es `locationOf`: el nombre de la carpeta que contiene la ruta, o «Mi
// unidad» si cuelga de la raíz.
func UbicacionDe(tr *i18n.Translator, ruta string) string {
	var partes []string
	for _, s := range strings.Split(ruta, "/") {
		if s != "" {
			partes = append(partes, s)
		}
	}
	if len(partes) <= 1 {
		return tr.T("nav.myDrive", nil)
	}
	if d, err := url.PathUnescape(partes[len(partes)-2]); err == nil {
		return d
	}
	return partes[len(partes)-2]
}

// problemaVista son los estados de error de una vista plana: la sesión caducada con su
// botón de entrar, el permiso, la falta de red, y el fallo con su reintento.
func problemaVista(tr *i18n.Translator, f Fase, claveError string, reintentar func(), v tema.Variante) *ui.Vacio {
	var x *ui.Vacio
	switch f {
	case SinSesion:
		x = ui.NuevoVacio(tr.T("files.error.sessionTitle", nil), tr.T("files.error.sessionBody", nil))
		x.Accion = ui.NuevoBoton(tr.T("files.error.sessionCta", nil), reintentar).Con(ui.Primario, ui.TallaSM)
	case Prohibida:
		x = ui.NuevoVacio(tr.T("forbidden.title", nil), tr.T("forbidden.description", nil))
	case SinConexion:
		x = ui.NuevoVacio(tr.T("offline.status", nil), tr.T("offline.description", nil))
		x.Ilustracion = iconos.Trazo(iconos.Nube, 40, "text-subtle", v).Objeto()
		return x
	case Cargando, Exito, Vacia, Fallo:
		x = ui.NuevoVacio(tr.T(claveError, nil), tr.T("files.error.network", nil))
		x.Accion = ui.NuevoBoton(tr.T("files.retry", nil), reintentar).Con(ui.Secundario, ui.TallaSM)
	}
	x.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto()
	return x
}

// pieCargarMas es el centinela del final: `flex justify-center px-4 py-4` con «Cargar
// más» (fantasma, sm) o la rueda de 16 y «Cargando más…» mientras llega. El botón existe
// aunque la lista pida sola la página siguiente: con el teclado no hay desplazamiento que
// la dispare.
func pieCargarMas(tr *i18n.Translator, cargando bool, alPulsar func(), v tema.Variante) fyne.CanvasObject {
	var dentro fyne.CanvasObject
	if cargando {
		r := ui.NuevaRueda(LadoRuedaSubida, "text-subtle", v)
		r.Rotulo = tr.T("files.loadingMore", nil)
		texto := ui.Texto(tr.T("files.loadingMore", nil), tema.TextoSM, ui.Normal, tema.Color("text-subtle", v))
		dentro = container.New(ui.Fila{Hueco: tema.Espacio(2)}, r, texto)
	} else {
		dentro = ui.NuevoBoton(tr.T("files.loadMore", nil), alPulsar).Con(ui.Fantasma, ui.TallaSM)
	}
	return container.New(ui.Margen{Izq: tema.Espacio(4), Arr: tema.Espacio(4), Der: tema.Espacio(4), Abj: tema.Espacio(4)},
		container.NewCenter(dentro))
}

// listaPlana apila las filas dentro del `px-3 pb-6` de la lista.
type listaPlana struct{}

var _ fyne.Layout = listaPlana{}

func (listaPlana) MinSize(objs []fyne.CanvasObject) fyne.Size {
	alto := PieLista
	for _, o := range objs {
		alto += o.MinSize().Height
	}
	return fyne.NewSize(0, alto)
}

func (listaPlana) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	y := float32(0)
	ancho := max(t.Width-2*CanalonLista, 0)
	for _, o := range objs {
		h := o.MinSize().Height
		o.Move(fyne.NewPos(CanalonLista, y))
		o.Resize(fyne.NewSize(ancho, h))
		y += h
	}
}

// ---- «Documentos» ----

// # «Documentos»
//
// `DocumentsView`: los documentos de Sytue Docs vistos desde Drive, porque un documento
// no vive en ninguna carpeta y lo creado con «Nuevo → Documento» desaparecía de la vista.
// De solo lectura a propósito: renombrar o borrar un documento son cosas de Docs.
//
// Su cabecera NO es `SectionHeader`: va escrita a mano con `px-4 py-3`, el título en
// `text-base font-semibold` y un buscador de `sm:w-64` que filtra EN EL SERVICIO, con el
// mismo rebote de 300 ms que el explorador.

// Documento es una fila de «Documentos».
type Documento struct {
	ID, Titulo string
	Tipo       api.DocKind
	Cambiado   time.Time
}

// VistaDocumentos es la sección «Documentos».
type VistaDocumentos struct {
	tr  *i18n.Translator
	loc i18n.Locale

	Fase       Fase
	Documentos []Documento
	Consulta   string
	Ventana    float32
	// Ahora es desde cuándo se cuenta el «hace 5 min»; se inyecta para el guardián.
	Ahora time.Time

	AlAbrir      func(Documento)
	AlBuscar     func(string)
	AlReintentar func()

	buscador    *ui.Entrada
	desplazable *container.Scroll
}

// NuevaVistaDocumentos crea la vista en su estado inicial: cargando.
func NuevaVistaDocumentos(tr *i18n.Translator) *VistaDocumentos {
	return &VistaDocumentos{tr: tr, loc: tr.Locale()}
}

// Medidas de la cabecera de «Documentos».
const (
	// RellenoXDocs y RellenoYDocs son el `px-4 py-3`, y HuecoDocs su `gap-3`.
	RellenoXDocs float32 = 4 * 0.25 * tema.Rem
	RellenoYDocs float32 = 3 * 0.25 * tema.Rem
	HuecoDocs    float32 = 3 * 0.25 * tema.Rem
	// AnchoBuscadorDocs es el `sm:w-64`.
	AnchoBuscadorDocs float32 = 64 * 0.25 * tema.Rem
)

// glifoDeTipo es el glifo del tipo de documento (los tres comparten encuadre).
func glifoDeTipo(k api.DocKind) iconos.Icono {
	switch k {
	case api.DocSheet:
		return iconos.DriveHoja
	case api.DocSlides:
		return iconos.DrivePresentacion
	case api.DocText:
	}
	return iconos.DriveDocumento
}

func claveDeTipo(k api.DocKind) string {
	switch k {
	case api.DocSheet:
		return "docs.kind.spreadsheet"
	case api.DocSlides:
		return "docs.kind.presentation"
	case api.DocText:
	}
	return "docs.kind.document"
}

// Objeto arma la vista.
func (d *VistaDocumentos) Objeto(v tema.Variante) fyne.CanvasObject {
	return container.New(Pila{Elastico: 1}, d.cabecera(v), d.cuerpo(v))
}

func (d *VistaDocumentos) cabecera(v tema.Variante) fyne.CanvasObject {
	if d.buscador == nil {
		b := ui.NuevaEntrada(d.tr.T("docs.search", nil))
		b.PonerTexto(d.Consulta)
		b.AlCambiar(func(s string) {
			if d.AlBuscar != nil {
				d.AlBuscar(s)
			}
		})
		d.buscador = b
	}
	glifo := iconos.Trazo(iconos.DriveFichero, ui.GlifoMenu, "text-muted", v)
	titulo := ui.Texto(d.tr.T("docs.title", nil), tema.TextoBase, ui.SemiNegrita, tema.Color("text", v))
	sub := ui.Texto(d.tr.T("docs.subtitle", nil), tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
	c := &cabeceraDocs{glifo: glifo, titulo: titulo, sub: sub, buscador: d.buscador, ventana: d.Ventana}
	return container.New(c, canvas.NewRectangle(tema.Color("border", v)), glifo.Objeto(), titulo, sub, d.buscador)
}

// cabeceraDocs coloca la cabecera de «Documentos». Por debajo de `sm` el buscador es
// `w-full` y baja a su propia línea (`flex-wrap`).
type cabeceraDocs struct {
	glifo       *iconos.Dibujo
	titulo, sub *canvas.Text
	buscador    *ui.Entrada
	ventana     float32
}

var _ fyne.Layout = (*cabeceraDocs)(nil)

func (c *cabeceraDocs) estrecha() bool { return c.ventana > 0 && c.ventana < ui.CorteSM }

func (c *cabeceraDocs) MinSize([]fyne.CanvasObject) fyne.Size {
	texto := tema.TextoBase.Linea + tema.TextoSM.Linea
	campo := c.buscador.MinSize().Height
	if c.estrecha() {
		return fyne.NewSize(0, 2*RellenoYDocs+texto+HuecoDocs+campo+tema.Borde)
	}
	return fyne.NewSize(0, 2*RellenoYDocs+max(texto, campo)+tema.Borde)
}

func (c *cabeceraDocs) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
	texto := tema.TextoBase.Linea + tema.TextoSM.Linea
	campo := c.buscador.MinSize().Height
	linea := max(texto, campo)
	if c.estrecha() {
		linea = texto
	}
	x := RellenoXDocs
	c.glifo.Colocar(x, RellenoYDocs+(linea-ui.GlifoMenu)/2, ui.GlifoMenu)
	x += ui.GlifoMenu + HuecoDocs
	derecha := t.Width - RellenoXDocs
	if c.estrecha() {
		c.buscador.Move(fyne.NewPos(RellenoXDocs, RellenoYDocs+texto+HuecoDocs))
		c.buscador.Resize(fyne.NewSize(max(t.Width-2*RellenoXDocs, 0), campo))
	} else {
		c.buscador.Move(fyne.NewPos(derecha-AnchoBuscadorDocs, RellenoYDocs+(linea-campo)/2))
		c.buscador.Resize(fyne.NewSize(AnchoBuscadorDocs, campo))
		derecha -= AnchoBuscadorDocs + HuecoDocs
	}
	cabe := max(derecha-x, 0)
	y := RellenoYDocs + (linea-texto)/2
	ui.Truncar(c.titulo, c.titulo.Text, tema.TextoBase, ui.SemiNegrita, cabe)
	ui.ColocarTexto(c.titulo, tema.TextoBase, x, y)
	ui.Truncar(c.sub, c.sub.Text, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(c.sub, tema.TextoSM, x, y+tema.TextoBase.Linea)
}

// FilasEsqueletoDocs son las cinco filas del esqueleto de «Documentos».
const FilasEsqueletoDocs = 5

func (d *VistaDocumentos) cuerpo(v tema.Variante) fyne.CanvasObject {
	switch d.Fase {
	case Cargando:
		return d.esqueleto(v)
	case Vacia:
		titulo, desc := "docs.emptyTitle", "docs.emptyBody"
		if strings.TrimSpace(d.Consulta) != "" {
			titulo, desc = "docs.emptyQueryTitle", "docs.emptyQueryBody"
		}
		x := ui.NuevoVacio(d.tr.T(titulo, nil), d.tr.T(desc, nil))
		x.Ilustracion = ilustracion(iconos.VacioDrive, v)
		return centrar(x, v)
	case SinSesion:
		x := ui.NuevoVacio(d.tr.T("files.error.sessionTitle", nil), d.tr.T("docs.sessionBody", nil))
		x.Accion = ui.NuevoBoton(d.tr.T("files.error.sessionCta", nil), d.AlReintentar).Con(ui.Primario, ui.TallaSM)
		x.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto()
		return centrar(x, v)
	case Fallo:
		x := ui.NuevoVacio(d.tr.T("docs.errorTitle", nil), d.tr.T("docs.errorBody", nil))
		x.Accion = ui.NuevoBoton(d.tr.T("files.retry", nil), d.AlReintentar).Con(ui.Secundario, ui.TallaSM)
		x.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto()
		return centrar(x, v)
	case Prohibida, SinConexion:
		return centrar(problemaVista(d.tr, d.Fase, "docs.errorTitle", d.AlReintentar, v), v)
	case Exito:
	}
	filas := make([]fyne.CanvasObject, 0, len(d.Documentos))
	ahora := d.Ahora
	if ahora.IsZero() {
		ahora = time.Now()
	}
	for _, doc := range d.Documentos {
		f := NuevaFilaPlana(doc.Titulo)
		f.Glifo = iconos.Trazo(glifoDeTipo(doc.Tipo), LadoGlifoDoc, "text-muted", v)
		f.Meta, f.MetaDocs = d.tr.T(claveDeTipo(doc.Tipo), nil), true
		f.Derecha = i18n.FormatTimeAgo(d.loc, doc.Cambiado, ahora)
		documento := doc
		f.AlAbrir = func() {
			if d.AlAbrir != nil {
				d.AlAbrir(documento)
			}
		}
		filas = append(filas, f)
	}
	contenido := container.New(listaPlana{}, filas...)
	if d.desplazable == nil {
		d.desplazable = container.NewVScroll(contenido)
	} else {
		d.desplazable.Content = contenido
		d.desplazable.Refresh()
	}
	return container.NewStack(canvas.NewRectangle(tema.Color("bg", v)), d.desplazable)
}

// esqueleto son las cinco filas de carga: la caja de `h-9 w-9` (31,5) y una raya de
// `h-4 w-1/3`, dentro del `px-3` de la lista y del `px-3 py-2 gap-3` de cada fila.
func (d *VistaDocumentos) esqueleto(v tema.Variante) fyne.CanvasObject {
	hijos := make([]fyne.CanvasObject, 0, 1+2*FilasEsqueletoDocs)
	hijos = append(hijos, canvas.NewRectangle(tema.Color("bg", v)))
	filas := make([][2]*ui.Esqueleto, 0, FilasEsqueletoDocs)
	for range FilasEsqueletoDocs {
		caja, raya := ui.NuevoEsqueleto(LadoCajaEsqueletoDocs, LadoCajaEsqueletoDocs), ui.NuevoEsqueleto(0, tema.Espacio(4))
		filas = append(filas, [2]*ui.Esqueleto{caja, raya})
		hijos = append(hijos, caja, raya)
	}
	return container.New(&esqueletoDocs{filas: filas}, hijos...)
}

// LadoCajaEsqueletoDocs es el `h-9 w-9` de la caja del esqueleto: 31,5, no los 36 de la
// tesela de verdad. Por eso las filas de carga miden 46,5 y no 50.
const LadoCajaEsqueletoDocs = 9 * 0.25 * tema.Rem

type esqueletoDocs struct{ filas [][2]*ui.Esqueleto }

var _ fyne.Layout = (*esqueletoDocs)(nil)

func (e *esqueletoDocs) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (e *esqueletoDocs) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	x := 2 * CanalonLista
	// `w-1/3` del contenido de la fila: el ancho menos el `px-3` de la lista y el suyo.
	tercio := max(t.Width-4*CanalonLista, 0) / 3
	alto := 2*ui.AltoFila + LadoCajaEsqueletoDocs + tema.Borde
	for i, f := range e.filas {
		y := float32(i) * alto
		centro := func(h float32) float32 { return y + (alto-tema.Borde-h)/2 }
		f[0].Move(fyne.NewPos(x, centro(LadoCajaEsqueletoDocs)))
		f[0].Resize(fyne.NewSize(LadoCajaEsqueletoDocs, LadoCajaEsqueletoDocs))
		f[1].Move(fyne.NewPos(x+LadoCajaEsqueletoDocs+HuecoFilaPlana, centro(tema.Espacio(4))))
		f[1].Resize(fyne.NewSize(tercio, tema.Espacio(4)))
	}
}
