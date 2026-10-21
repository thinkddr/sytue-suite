// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Compartido conmigo»
//
// `SharedWithMeView`: las raíces que otras personas han compartido contigo, con quién y
// con qué permiso, y la navegación DENTRO de una carpeta compartida. El servicio navega
// por (raíz, subcamino) y no por id —la verja es del share, no del nodo—, y un share
// revocado mientras se mira contesta 404 sin decir que exista: eso es «ya no tienes
// acceso», con su botón para volver.
//
// Abrir un fichero lo DESCARGA: la web lo previsualiza en su visor, y las apps nativas
// de cada formato abren ficheros de TU unidad por id, no de la de otra persona.
//
// ponytail: sin vista previa. Llega con el visor de texto de Drive, el día que lo haya.

// VistaCompartido es la sección «Compartido conmigo».
type VistaCompartido struct {
	tr  *i18n.Translator
	loc i18n.Locale

	// Fase y Raices son el listado de raíces.
	Fase   Fase
	Raices []api.Compartido
	// Raiz es la carpeta compartida abierta; nil, se ven las raíces.
	Raiz *api.Compartido
	// Subcamino es dónde se está dentro de ella, relativo a la raíz; vacío, la raíz.
	Subcamino string
	// FaseCarpeta y Carpeta son lo de dentro. Denegada: el share ya no vale.
	FaseCarpeta Fase
	Denegada    bool
	Carpeta     []api.Compartido
	// Permiso es el de la carpeta abierta («view» o «edit»), que manda el servicio.
	Permiso      string
	Subiendo     bool
	Desconectado bool
	Ventana      float32

	AlAbrirRaiz         func(api.Compartido)
	AlAbrir             func(api.Compartido)
	AlDescargar         func(raiz, subcamino, nombre string)
	AlVolver            func()
	AlIr                func(subcamino string)
	AlSubir             func()
	AlReintentar        func()
	AlReintentarCarpeta func()

	// Peticion es la franja de «Pedir acceso de edición» de la raíz abierta en lectura
	// (secciones.go la engancha al abrirla).
	Peticion peticion.Estado

	desplazable *container.Scroll
}

// NuevaVistaCompartido crea la vista en su estado inicial: cargando.
func NuevaVistaCompartido(tr *i18n.Translator) *VistaCompartido {
	return &VistaCompartido{tr: tr, loc: tr.Locale()}
}

func (c *VistaCompartido) t(clave string, v ...i18n.Vars) string {
	var vars i18n.Vars
	if len(v) > 0 {
		vars = v[0]
	}
	return c.tr.T(clave, vars)
}

// PuedeEditar es el `canEdit` de la web: el permiso de edición y red para usarlo.
func (c *VistaCompartido) PuedeEditar() bool {
	permiso := c.Permiso
	if permiso == "" && c.Raiz != nil {
		permiso = c.Raiz.Permission
	}
	return permiso == "edit" && !c.Desconectado && c.Fase != SinConexion
}

// Objeto arma la vista: cabecera, barra de navegación si se está dentro, y cuerpo.
func (c *VistaCompartido) Objeto(v tema.Variante) fyne.CanvasObject {
	cab := cabeceraSeccion(c.tr, "shared.title", "shared.subtitle", glifoSeccion(glifoUsuarios, iconos.Ninguno, v))
	if c.Raiz == nil {
		return container.New(Pila{Elastico: 1}, cab, c.cuerpoRaices(v))
	}
	if f := c.franja(); f != nil {
		return container.New(Pila{Elastico: 3}, cab, c.barra(v), f, c.cuerpoCarpeta(v))
	}
	return container.New(Pila{Elastico: 2}, cab, c.barra(v), c.cuerpoCarpeta(v))
}

// franja es la de solo lectura de la raíz abierta: `FranjaPeticion` en la web.
func (c *VistaCompartido) franja() fyne.CanvasObject {
	if c.Raiz == nil || c.PuedeEditar() || c.Desconectado {
		return nil
	}
	return peticion.Franja(&c.Peticion, true, c.t("readOnly.sharedWithYou", i18n.Vars{"name": c.Raiz.Name}),
		func(k string, v i18n.Vars) string { return c.tr.T(k, v) })
}

func (c *VistaCompartido) cuerpoRaices(v tema.Variante) fyne.CanvasObject {
	switch c.Fase {
	case Cargando:
		return (&Explorador{tr: c.tr, loc: c.loc, Ventana: c.Ventana}).esqueletoCuerpo(v)
	case Vacia:
		x := ui.NuevoVacio(c.t("shared.emptyTitle"), c.t("shared.emptyBody"))
		x.Ilustracion = ilustracion(iconos.VacioDrive, v)
		return centrar(x, v)
	case Fallo, SinSesion, Prohibida, SinConexion:
		return centrar(problemaVista(c.tr, c.Fase, "shared.errorTitle", c.AlReintentar, v), v)
	case Exito:
	}
	filas := make([]fyne.CanvasObject, 0, len(c.Raices))
	for _, r := range c.Raices {
		filas = append(filas, c.filaRaiz(r))
	}
	return c.lista(v, filas)
}

func (c *VistaCompartido) lista(v tema.Variante, filas []fyne.CanvasObject) fyne.CanvasObject {
	contenido := container.New(listaPlana{}, filas...)
	if c.desplazable == nil {
		c.desplazable = container.NewVScroll(contenido)
	} else {
		c.desplazable.Content = contenido
		c.desplazable.Refresh()
	}
	return container.NewStack(canvas.NewRectangle(tema.Color("bg", v)), c.desplazable)
}

// insignia es el `PermBadge` de un permiso.
func (c *VistaCompartido) insignia(permiso string) *Insignia {
	if permiso == "edit" {
		return &Insignia{Texto: c.t("shared.perm.edit"), Glifo: glifoLapiz, Acento: true}
	}
	return &Insignia{Texto: c.t("shared.perm.view")}
}

// botonDescargar es el `h-8 w-8` con el glifo de descarga de las filas de ficheros.
func (c *VistaCompartido) botonDescargar(nombre string, bajar func()) BotonFila {
	return BotonFila{
		Cuerpo: glifoDescargar, Tinta: "text-muted", TintaEncima: "text",
		Rotulo:   c.t("action.downloadName", i18n.Vars{"name": nombre}),
		AlPulsar: func(fyne.CanvasObject) { bajar() },
	}
}

// filaRaiz es una raíz compartida: el nombre abre, la meta dice qué es, dónde estaba y
// quién la compartió, y a la derecha el permiso y, si es un fichero, la descarga.
func (c *VistaCompartido) filaRaiz(r api.Compartido) *FilaPlana {
	el := deCompartido(r)
	f := NuevaFilaPlana(r.Name)
	f.Extension, f.Carpeta = Extension(r.Name), r.IsDir
	// Sin fecha: la raíz dice qué es, de dónde es y quién la compartió.
	f.Meta = metaPlana(c.tr, c.loc, Elemento{Carpeta: r.IsDir, Tamano: el.Tamano, Ruta: r.Path}, true)
	if r.SharedBy != "" {
		f.Meta += " · " + c.t("shared.sharedBy", i18n.Vars{"name": r.SharedBy})
	}
	raiz := r
	f.AlAbrir = func() {
		if c.AlAbrirRaiz != nil {
			c.AlAbrirRaiz(raiz)
		}
	}
	f.Insignias = []*Insignia{c.insignia(r.Permission)}
	if !r.IsDir && r.ID != "" {
		f.Botones = []BotonFila{c.botonDescargar(r.Name, func() {
			if c.AlDescargar != nil {
				c.AlDescargar(raiz.ID, "", raiz.Name)
			}
		})}
	}
	return f
}

// metaCompartido es la de lo que hay dentro: sin ubicación, que ya la dicen las migas.
func (c *VistaCompartido) metaCompartido(x api.Compartido) string {
	return metaPlana(c.tr, c.loc, deCompartido(x), false)
}

// deCompartido lo convierte en un Elemento para reutilizar tamaño, fecha y tesela.
func deCompartido(x api.Compartido) Elemento {
	e := DeFichero(x.File)
	if x.IsDir {
		e.Carpeta, e.Tamano = true, -1
	}
	return e
}

func (c *VistaCompartido) cuerpoCarpeta(v tema.Variante) fyne.CanvasObject {
	switch {
	case c.FaseCarpeta == Cargando:
		return (&Explorador{tr: c.tr, loc: c.loc, Ventana: c.Ventana}).esqueletoCuerpo(v)
	case c.Denegada:
		x := ui.NuevoVacio(c.t("shared.deniedTitle"), c.t("shared.deniedBody"))
		x.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto()
		x.Accion = ui.NuevoBoton(c.t("shared.deniedCta"), c.AlVolver).Con(ui.Secundario, ui.TallaSM)
		return centrar(x, v)
	case c.FaseCarpeta == Vacia:
		clave := "shared.emptyFolderView"
		if c.PuedeEditar() {
			clave = "shared.emptyFolderEdit"
		}
		x := ui.NuevoVacio(c.t("team.folder.emptyTitle"), c.t(clave))
		x.Ilustracion = iconos.Trazo(iconos.DriveCarpeta, 40, "text-subtle", v).Objeto()
		return centrar(x, v)
	case c.FaseCarpeta != Exito:
		return centrar(problemaVista(c.tr, c.FaseCarpeta, "shared.folderErrorTitle", c.AlReintentarCarpeta, v), v)
	}
	filas := make([]fyne.CanvasObject, 0, len(c.Carpeta))
	for _, x := range c.Carpeta {
		item := x
		f := NuevaFilaPlana(x.Name)
		f.Extension, f.Carpeta = Extension(x.Name), x.IsDir
		f.Meta = c.metaCompartido(x)
		f.AlAbrir = func() {
			if c.AlAbrir != nil {
				c.AlAbrir(item)
			}
		}
		if !x.IsDir && c.Raiz != nil {
			raiz := c.Raiz.ID
			f.Botones = []BotonFila{c.botonDescargar(x.Name, func() {
				if c.AlDescargar != nil {
					c.AlDescargar(raiz, item.Path, item.Name)
				}
			})}
		}
		filas = append(filas, f)
	}
	return c.lista(v, filas)
}

// ---- La barra de dentro de una carpeta compartida ----

// Medidas de la barra: `border-b bg-bg-subtle px-4 py-2 sm:px-6`, `gap-x-2`.
const (
	RellenoYBarraCompartido float32 = 2 * 0.25 * tema.Rem
	HuecoBarraCompartido    float32 = 2 * 0.25 * tema.Rem
)

// barra es la franja de navegación: «← Compartido conmigo», la ruta desde la raíz y, a
// la derecha, el permiso y «Subir aquí» si se puede editar.
func (c *VistaCompartido) barra(v tema.Variante) fyne.CanvasObject {
	volver := nuevoBotonTexto(c.t("shared.title"), tema.TextoSM, ui.Medio, c.AlVolver)
	volver.icono, volver.ladoIcono, volver.hueco = iconos.ChevronIzquierda, 14, tema.Espacio(1)
	volver.margenX, volver.alto = tema.Espacio(2), tema.TextoSM.Linea+2*tema.Espacio(1)

	nombres := append([]string{c.Raiz.Name}, strings.FieldsFunc(c.Subcamino, func(r rune) bool { return r == '/' })...)
	migas := ui.NuevasMigas(c.t("files.breadcrumb"), nombres, func(i int) {
		if c.AlIr != nil {
			c.AlIr(strings.Join(nombres[1:i+1], "/"))
		}
	})
	migas.Estilo = ui.MigasEnBarra
	galon := iconos.Trazo(iconos.ChevronDerecha, ui.LadoGalonMiga, "text-subtle", v)

	permiso := c.Permiso
	if permiso == "" {
		permiso = c.Raiz.Permission
	}
	insignia := nuevaInsigniaSuelta(c.insignia(permiso), v)

	b := &barraCompartido{volver: volver, galon: galon, migas: migas, insignia: insignia, ventana: c.Ventana}
	hijos := []fyne.CanvasObject{
		canvas.NewRectangle(tema.Color("bg-subtle", v)), canvas.NewRectangle(tema.Color("border", v)),
		volver, galon.Objeto(), migas, insignia,
	}
	if c.PuedeEditar() {
		subir := nuevoBotonTexto(c.t("shared.uploadHere"), tema.TextoSM, ui.SemiNegrita, c.AlSubir)
		subir.cuerpo, subir.ladoIcono, subir.hueco = glifoSubir, 16, tema.Espacio(2)
		subir.margenX, subir.alto, subir.borde = tema.Espacio(3), AltoControlCromo, true
		subir.tinta = "text"
		subir.cargando = c.Subiendo
		b.subir = subir
		hijos = append(hijos, subir)
	}
	return container.New(b, hijos...)
}

type barraCompartido struct {
	volver   *botonTexto
	galon    *iconos.Dibujo
	migas    *ui.Migas
	insignia fyne.CanvasObject
	subir    *botonTexto
	ventana  float32
}

var _ fyne.Layout = (*barraCompartido)(nil)

func (b *barraCompartido) linea() float32 {
	if b.subir != nil {
		return AltoControlCromo
	}
	return max(b.volver.MinSize().Height, b.insignia.MinSize().Height)
}

func (b *barraCompartido) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, 2*RellenoYBarraCompartido+b.linea()+tema.Borde)
}

// Layout pone todo en una línea centrada.
//
// ponytail: sin `flex-wrap`. En un móvil la web parte la barra en dos líneas; aquí la
// ruta se estrecha y recorta, igual que la cabecera del explorador.
func (b *barraCompartido) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	objs[1].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	canalon := CanalonCabecera(b.ventana)
	linea := b.linea()
	centrar := func(o fyne.CanvasObject, x, ancho float32) {
		alto := o.MinSize().Height
		o.Move(fyne.NewPos(x, RellenoYBarraCompartido+(linea-alto)/2))
		o.Resize(fyne.NewSize(ancho, alto))
	}
	derecha := t.Width - canalon
	if b.subir != nil {
		m := b.subir.MinSize()
		centrar(b.subir, derecha-m.Width, m.Width)
		derecha -= m.Width + HuecoAcciones
	}
	m := b.insignia.MinSize()
	centrar(b.insignia, derecha-m.Width, m.Width)
	derecha -= m.Width + HuecoBarraCompartido

	x := canalon
	mv := b.volver.MinSize()
	centrar(b.volver, x, mv.Width)
	x += mv.Width + HuecoBarraCompartido
	b.galon.Colocar(x, RellenoYBarraCompartido+(linea-ui.LadoGalonMiga)/2, ui.LadoGalonMiga)
	x += ui.LadoGalonMiga + 0.5*0.25*tema.Rem
	centrar(b.migas, x, min(b.migas.MinSize().Width, max(derecha-x, 0)))
}

// nuevaInsigniaSuelta es la insignia fuera de una fila: la barra de dentro de una
// carpeta la lleva junto a «Subir aquí», y la cabecera de una unidad de equipo, el rol.
func nuevaInsigniaSuelta(i *Insignia, v tema.Variante) fyne.CanvasObject {
	p := nuevaPildora(i, v)
	return container.New(&insigniaSuelta{p: p}, p.objetos()...)
}

type insigniaSuelta struct{ p *pildora }

var _ fyne.Layout = (*insigniaSuelta)(nil)

func (s *insigniaSuelta) MinSize([]fyne.CanvasObject) fyne.Size { return MedidaInsignia(s.p.i) }

func (s *insigniaSuelta) Layout([]fyne.CanvasObject, fyne.Size) { s.p.colocar(fyne.NewPos(0, 0)) }

// botonTexto es un botón de texto escrito a mano en la web, sin `Button`: «← Compartido
// conmigo» (`px-2 py-1 text-sm font-medium`, apagado y con `hover:bg-bg`) y «Subir aquí»
// (`h-8 px-3 border text-sm font-semibold`, con su glifo o la rueda mientras sube).
// Pintan distinto que un `ui.Boton` —sin relleno propio sobre la barra `bg-subtle`, con
// el hover en `bg`— y por eso no lo son.
type botonTexto struct {
	ui.Pulsable
	texto string
	esc   tema.Escalon
	peso  ui.Peso
	// icono (del catálogo) o cuerpo (glifo de Drive), delante del texto.
	icono     iconos.Icono
	cuerpo    string
	ladoIcono float32
	hueco     float32
	margenX   float32
	alto      float32
	borde     bool
	// tinta en reposo; vacío es `text-muted`, que pasa a `text` al pasar por encima.
	tinta    string
	cargando bool
	// fondoEncima es el velo al pasar (vacío: `bg`) y tokenBorde el del borde (vacío:
	// `border`).
	fondoEncima, tokenBorde string
	// tintaEncima es la del texto y el glifo al pasar por encima (vacío: `text`).
	tintaEncima string
}

func nuevoBotonTexto(texto string, e tema.Escalon, p ui.Peso, alPulsar func()) *botonTexto {
	b := &botonTexto{texto: texto, esc: e, peso: p}
	b.Nombre, b.AlPulsar = texto, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// MinSize es el texto con su relleno y su glifo, y el borde si lo lleva.
func (b *botonTexto) MinSize() fyne.Size {
	ancho := ui.Medir(b.texto, b.esc, b.peso).Width + 2*b.margenX
	if b.icono != iconos.Ninguno || b.cuerpo != "" {
		ancho += b.ladoIcono + b.hueco
	}
	if b.borde {
		ancho += 2 * tema.Borde
	}
	return fyne.NewSize(ancho, b.alto)
}

func (b *botonTexto) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonTextoRenderer{
		b:     b,
		aro:   ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
		caja:  ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioMD),
		texto: ui.Texto(b.texto, b.esc, b.peso, tema.Color("text-muted", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	switch {
	case b.cuerpo != "":
		r.glifo = glifoCon(b.cuerpo, b.ladoIcono, tema.Color("text", v))
	case b.icono != iconos.Ninguno:
		r.glifo = iconos.Trazo(b.icono, b.ladoIcono, "text-muted", v)
	}
	if r.glifo != nil {
		r.objetos = append(r.objetos, r.glifo.Objeto())
	}
	if b.cargando {
		r.rueda = ui.NuevaRueda(b.ladoIcono, "text", v)
		r.objetos = append(r.objetos, r.rueda)
	}
	r.objetos = append(r.objetos, r.texto)
	r.Refresh()
	return r
}

type botonTextoRenderer struct {
	b       *botonTexto
	aro     *ui.Aro
	caja    *ui.Caja
	glifo   *iconos.Dibujo
	rueda   *ui.Rueda
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *botonTextoRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonTextoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonTextoRenderer) Destroy()                     {}

func (r *botonTextoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	x := r.b.margenX
	if r.b.borde {
		x += tema.Borde
	}
	y := (t.Height - r.b.ladoIcono) / 2
	if r.glifo != nil {
		r.glifo.Colocar(x, y, r.b.ladoIcono)
	}
	if r.rueda != nil {
		r.rueda.Move(fyne.NewPos(x, y))
		r.rueda.Resize(fyne.NewSize(r.b.ladoIcono, r.b.ladoIcono))
	}
	if r.glifo != nil || r.rueda != nil {
		x += r.b.ladoIcono + r.b.hueco
	}
	ui.ColocarTexto(r.texto, r.b.esc, x, (t.Height-r.b.esc.Linea)/2)
}

func (r *botonTextoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := r.b
	tinta, fondo, borde := "text-muted", color.NRGBA{}, color.NRGBA{}
	if b.tinta != "" {
		tinta = b.tinta
	}
	if b.Encima() && !b.cargando {
		velo := b.fondoEncima
		if velo == "" {
			velo = "bg"
		}
		tinta, fondo = "text", tema.Color(velo, v)
		if b.tintaEncima != "" {
			tinta = b.tintaEncima
		}
	}
	if b.borde {
		token := b.tokenBorde
		if token == "" {
			token = "border"
		}
		borde = tema.Color(token, v)
	}
	c := tema.Color(tinta, v)
	if b.cargando { // `disabled:opacity-60`
		c.A = uint8(float32(c.A) * 0.6)
	}
	r.caja.Pintar(fondo, borde)
	r.texto.Color = c
	if r.glifo != nil {
		r.glifo.TenirCon(c)
		r.glifo.Ver(!b.cargando)
	}
	r.aro.Pintar(v)
	r.aro.Ver(b.Enfocado())
	for _, o := range r.objetos {
		o.Refresh()
	}
}
