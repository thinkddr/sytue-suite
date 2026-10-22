// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Unidades de equipo
//
// `TeamDrives.tsx`. Dos pantallas:
//
//   - La LISTA es una página de ajustes (`SettingsShell`: `max-w-5xl px-4 py-6 sm:px-6`,
//     título `sytue-h2`, la frase que separa esto de «Mi unidad» y «Nueva unidad» a la
//     derecha) con una sección en tarjeta: las unidades de las que se es miembro, cada
//     una con lo que su rol PERMITE HACER, su distintivo de rol y «Abrir».
//   - El DETALLE de una unidad: la cabecera con «← Unidades», la ruta, el rol y las
//     acciones que el rol permite (editar: nueva carpeta y subir; gestionar: miembros,
//     renombrar y archivar), y el contenido de la carpeta.
//
// ponytail: un fichero se abre con la vista previa de Drive si es texto o WebP, y si no
// se descarga. La web lo previsualiza TODO en su visor (PDF, ofimática), y en nativo
// cada formato tiene su app, que abre ficheros de «Mi unidad» por id: el contenido de
// una unidad está aislado por unidad y esas apps aún no saben pedirlo por ahí.

// Medidas de las unidades de equipo.
const (
	// AnchoPaginaEquipos es el `max-w-5xl` con el rem de 14.
	AnchoPaginaEquipos float32 = 64 * tema.Rem
	// RellenoYPagina es el `py-6` de SettingsShell y SeparaSecciones su `mt-6`.
	RellenoYPagina  float32 = 6 * 0.25 * tema.Rem
	SeparaSecciones float32 = 6 * 0.25 * tema.Rem
	// AnchoProsa es el `max-w-2xl` de las descripciones.
	AnchoProsa float32 = 42 * tema.Rem
	// RellenoYFilaUnidad es el `py-2.5` de una unidad de la lista.
	RellenoYFilaUnidad float32 = 2.5 * 0.25 * tema.Rem
	// LadoGlifoEquipo es el `size={22}` de los glifos y CajaGlifoEquipo su `h-9 w-9`.
	LadoGlifoEquipo float32 = 22
	CajaGlifoEquipo float32 = 9 * 0.25 * tema.Rem
	// LadoAccionEquipo es el `p-1.5` alrededor de un glifo de 18.
	LadoAccionEquipo float32 = 18 + 2*1.5*0.25*tema.Rem
)

// VistaEquipos es la sección «Unidades de equipo».
type VistaEquipos struct {
	tr  *i18n.Translator
	loc i18n.Locale

	// La lista.
	Fase     Fase
	Unidades []api.UnidadEquipo

	// El detalle: la unidad abierta (nil, se ve la lista), la pila de carpetas por las
	// que se ha entrado —sin la raíz— y lo de la carpeta de ahora.
	Abierta     *api.UnidadEquipo
	Pila        []Elemento
	FaseDetalle Fase
	Denegada    bool
	Contenido   []Elemento
	HayMas      bool
	CargandoMas bool
	Ocupado     bool
	Subiendo    bool

	Desconectado bool
	Ventana      float32

	AlAbrirUnidad       func(api.UnidadEquipo)
	AlNueva             func()
	AlReintentar        func()
	AlVolver            func()
	AlIr                func(nivel int) // -1 es la raíz de la unidad
	AlAbrir             func(Elemento)
	AlDescargar         func(Elemento)
	AlBorrar            func(Elemento)
	AlNuevaCarpeta      func()
	AlSubir             func()
	AlMiembros          func()
	AlRenombrar         func()
	AlArchivar          func()
	AlReintentarDetalle func()
	AlCargarMas         func()

	desplazable, desplazableDetalle *container.Scroll
	// cursorUnidad es el de la página siguiente de la carpeta de la unidad.
	cursorUnidad string
}

// NuevaVistaEquipos crea la sección en su estado inicial: cargando.
func NuevaVistaEquipos(tr *i18n.Translator) *VistaEquipos {
	return &VistaEquipos{tr: tr, loc: tr.Locale()}
}

func (q *VistaEquipos) t(k string, v ...i18n.Vars) string {
	var vars i18n.Vars
	if len(v) > 0 {
		vars = v[0]
	}
	return q.tr.T(k, vars)
}

// Las claves de cada rol: su nombre, lo que PERMITE HACER en segunda persona (el propio)
// y en tercera (al elegírselo a otra persona). Dos claves y no un reemplazo de texto:
// cambiar la persona de un verbo con una sustitución solo funciona en español.
var (
	claveRol     = map[string]string{api.RolVer: "team.role.viewer", api.RolEditar: "team.role.editor", api.RolGestionar: "team.role.manager"}
	clavePuedes  = map[string]string{api.RolVer: "team.roleCan.viewer", api.RolEditar: "team.roleCan.editor", api.RolGestionar: "team.roleCan.manager"}
	clavePodra   = map[string]string{api.RolVer: "team.roleWill.viewer", api.RolEditar: "team.roleWill.editor", api.RolGestionar: "team.roleWill.manager"}
	rolesEquipos = []string{api.RolVer, api.RolEditar, api.RolGestionar}
)

// insigniaRol es el `RoleBadge`: el gestor en acento, el editor apagado, el lector más.
func (q *VistaEquipos) insigniaRol(rol string) *Insignia {
	i := &Insignia{Texto: q.t(claveRol[rol]), Tinta: "text-subtle"}
	switch rol {
	case api.RolGestionar:
		i.Acento = true
	case api.RolEditar:
		i.Tinta = "text-muted"
	}
	return i
}

// insigniaArchivada es el `ArchivedBadge`: el candado de 12 y «Archivada».
func (q *VistaEquipos) insigniaArchivada() *Insignia {
	return &Insignia{Texto: q.t("team.archivedBadge"), Glifo: glifoCandado, Tinta: "text-subtle"}
}

// SinRed apaga las mutaciones: sin conexión se ve lo cargado y no se escribe.
func (q *VistaEquipos) SinRed() bool { return q.Fase == SinConexion || q.Desconectado }

// Objeto arma la pantalla que toque.
func (q *VistaEquipos) Objeto(v tema.Variante) fyne.CanvasObject {
	if q.Abierta != nil {
		return q.detalle(v)
	}
	return q.lista(v)
}

// ---- La lista ----

func (q *VistaEquipos) lista(v tema.Variante) fyne.CanvasObject {
	titulo := ui.NuevoParrafo(q.t("team.title"), ui.EscalonTitulo(2), ui.PesoTitulo(2), "text", v)
	relajado := tema.Escalon{Tam: tema.TextoBase.Tam, Linea: 1.625 * tema.TextoBase.Tam}
	desc := ui.NuevoParrafo(q.t("team.description"), relajado, ui.Normal, "text-muted", v)
	nueva := ui.NuevoBoton(q.t("team.new"), q.AlNueva).Con(ui.Primario, ui.TallaMD).ConIcono(iconos.Mas, 16)
	nueva.Inerte = q.SinRed()

	secTitulo := ui.NuevoParrafo(q.t("team.listTitle"), ui.EscalonTitulo(4), ui.PesoTitulo(4), "text", v)
	secDesc := ui.NuevoParrafo(q.t("team.listDescription"), tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}, ui.Normal, "text-muted", v)
	var ancho float32
	cuerpoSeccion := container.New(&pilaSeparada{separas: []float32{0, tema.Espacio(1), tema.Espacio(4)}, ancho: &ancho},
		secTitulo, secDesc, q.contenidoLista(v, &ancho))
	tarjeta := ui.NuevaTarjeta("", cuerpoSeccion)

	p := &paginaAjustes{ventana: q.Ventana, titulo: titulo, desc: desc, accion: nueva, seccion: tarjeta, anchoSeccion: &ancho}
	pagina := container.New(p, titulo, desc, nueva, tarjeta)
	if q.desplazable == nil {
		q.desplazable = container.NewVScroll(pagina)
	} else {
		q.desplazable.Content = pagina
		q.desplazable.Refresh()
	}
	return container.NewStack(canvas.NewRectangle(tema.Color("bg", v)), q.desplazable)
}

// contenidoLista es lo que va dentro de la tarjeta de la sección.
func (q *VistaEquipos) contenidoLista(v tema.Variante, ancho *float32) fyne.CanvasObject {
	switch q.Fase {
	case Cargando:
		return esqueletoFilas()
	case Vacia:
		x := ui.NuevoVacio(q.t("team.empty.title"), q.t("team.empty.body"))
		x.Ilustracion = ilustracion(iconos.VacioDrive, v)
		b := ui.NuevoBoton(q.t("team.empty.cta"), q.AlNueva).Con(ui.Primario, ui.TallaSM).ConIcono(iconos.Mas, 16)
		b.Inerte = q.SinRed()
		x.Accion = b
		return vacioEnLinea(x)
	case SinSesion:
		x := ui.NuevoVacio(q.t("team.error.sessionTitle"), q.t("team.error.sessionBody"))
		x.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto()
		x.Accion = ui.NuevoBoton(q.t("team.error.sessionCta"), q.AlReintentar).Con(ui.Primario, ui.TallaSM)
		return vacioEnLinea(x)
	case Fallo:
		x := ui.NuevoVacio(q.t("team.error.listTitle"), q.t("team.error.network"))
		x.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto()
		x.Accion = ui.NuevoBoton(q.t("team.retry"), q.AlReintentar).Con(ui.Secundario, ui.TallaSM)
		return vacioEnLinea(x)
	case Prohibida, SinConexion:
		return vacioEnLinea(problemaVista(q.tr, q.Fase, "team.error.listTitle", q.AlReintentar, v))
	case Exito:
	}
	filas := make([]fyne.CanvasObject, 0, len(q.Unidades))
	for i, u := range q.Unidades {
		unidad := u
		f := NuevaFilaPlana(u.Name)
		f.Glifo = glifoCon(glifoUsuarios, LadoGlifoEquipo, tema.Color("text-muted", v))
		f.LadoGlifo, f.CajaGlifo = LadoGlifoEquipo, CajaGlifoEquipo
		f.Meta = q.t(clavePuedes[u.Role])
		// `sytue-meta mt-0.5 block`: lo que se puede hacer pasa de línea en un móvil en
		// vez de cortarse. La fila va dentro del borde de la lista (`conMargen`).
		f.MetaParte, f.AnchoDe = true, func() float32 { return *ancho - 2*tema.Borde }
		f.RellenoY, f.Filete = RellenoYFilaUnidad, FileteArriba
		if i == 0 {
			f.Filete = SinFilete
		}
		if u.ArchivedAt != nil {
			f.Insignias = append(f.Insignias, q.insigniaArchivada())
		}
		f.Insignias = append(f.Insignias, q.insigniaRol(u.Role))
		abrir := func(fyne.CanvasObject) {
			if q.AlAbrirUnidad != nil {
				q.AlAbrirUnidad(unidad)
			}
		}
		f.AlAbrir = func() { abrir(nil) }
		f.Botones = []BotonFila{{Texto: q.t("team.open"), Rotulo: q.t("team.openTitle", i18n.Vars{"name": u.Name}), AlPulsar: abrir}}
		filas = append(filas, f)
	}
	borde := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
	return container.New(&conMargen{m: tema.Borde}, append(borde.Objetos(), container.New(ui.Columna{}, filas...))...)
}

// FilasEsqueletoEquipos son las seis filas de carga de la lista.
const FilasEsqueletoEquipos = 6

// esqueleto es el `TeamDriveListSkeleton`: la caja de 36, una raya de `0.85rem` con un
// ancho que varía por fila y otra de 60 × `1.1rem` al final, en filas de `px-3 py-2.5`
// dentro de un `px-3`.
func esqueletoFilas() fyne.CanvasObject {
	e := &esqueletoEquipos{}
	hijos := make([]fyne.CanvasObject, 0, 3*FilasEsqueletoEquipos)
	for i := range FilasEsqueletoEquipos {
		f := [3]*ui.Esqueleto{
			ui.NuevoEsqueleto(36, 36),
			ui.NuevoEsqueleto(0, 0.85*tema.Rem),
			ui.NuevoEsqueleto(60, 1.1*tema.Rem),
		}
		e.filas = append(e.filas, f)
		e.pct = append(e.pct, float32(40+(i*13)%40)/100)
		hijos = append(hijos, f[0], f[1], f[2])
	}
	return container.New(e, hijos...)
}

type esqueletoEquipos struct {
	filas [][3]*ui.Esqueleto
	pct   []float32
}

func (e *esqueletoEquipos) alto() float32 { return 2*RellenoYFilaUnidad + 36 }

func (e *esqueletoEquipos) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, float32(len(e.filas))*e.alto())
}

func (e *esqueletoEquipos) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	x := 2 * CanalonLista
	util := max(t.Width-4*CanalonLista, 0)
	for i, f := range e.filas {
		y := float32(i) * e.alto()
		c := func(h float32) float32 { return y + (e.alto()-h)/2 }
		f[0].Move(fyne.NewPos(x, c(36)))
		f[0].Resize(fyne.NewSize(36, 36))
		f[1].Move(fyne.NewPos(x+36+HuecoFilaPlana, c(0.85*tema.Rem)))
		f[1].Resize(fyne.NewSize(max(util-36-HuecoFilaPlana, 0)*e.pct[i], 0.85*tema.Rem))
		f[2].Move(fyne.NewPos(x+util-60, c(1.1*tema.Rem)))
		f[2].Resize(fyne.NewSize(60, 1.1*tema.Rem))
	}
}

// vacioEnLinea es un EmptyState dentro de una tarjeta: el alto que pida al ancho que le
// den, sin centrarse en toda la pantalla.
func vacioEnLinea(x *ui.Vacio) fyne.CanvasObject { return container.New(&vacioAncho{x: x}, x) }

type vacioAncho struct {
	x     *ui.Vacio
	ancho float32
}

func (a *vacioAncho) MinSize([]fyne.CanvasObject) fyne.Size {
	ancho := a.ancho
	if ancho <= 0 {
		ancho = 40 * tema.Rem
	}
	return fyne.NewSize(0, a.x.MedidaVacio(ancho))
}

func (a *vacioAncho) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	a.ancho = t.Width
	a.x.Move(fyne.NewPos(0, 0))
	a.x.Resize(fyne.NewSize(t.Width, a.x.MedidaVacio(t.Width)))
}

// paginaAjustes es `SettingsShell` con una sección: el título y su descripción a la
// izquierda, la acción a la derecha, y la tarjeta debajo con `mt-6`.
type paginaAjustes struct {
	ventana      float32
	titulo, desc *ui.Parrafo
	accion       fyne.CanvasObject
	seccion      *ui.Tarjeta
	anchoSeccion *float32
}

func (p *paginaAjustes) reparto(ancho float32) (izq, util float32) {
	canalon := CanalonCabecera(p.ventana)
	caja := min(ancho, AnchoPaginaEquipos)
	return (ancho-caja)/2 + canalon, max(caja-2*canalon, 0)
}

func (p *paginaAjustes) cabecera(util float32) (textoAncho, alto float32) {
	a := p.accion.MinSize()
	textoAncho = max(util-a.Width-tema.Espacio(4), 0)
	prosa := min(textoAncho, AnchoProsa)
	alto = max(ui.AltoParrafo(p.titulo, textoAncho)+tema.Espacio(1.5)+ui.AltoParrafo(p.desc, prosa), a.Height)
	return textoAncho, alto
}

func (p *paginaAjustes) altoSeccion(util float32) float32 {
	*p.anchoSeccion = max(util-2*(tema.Espacio(4)+tema.Borde), 0)
	return ui.MedidaTarjeta("", p.seccion.Cuerpo).Height
}

func (p *paginaAjustes) MinSize([]fyne.CanvasObject) fyne.Size {
	_, util := p.reparto(max(p.ventana-tema.AnchoLateral, 1))
	_, alto := p.cabecera(util)
	return fyne.NewSize(0, 2*RellenoYPagina+alto+SeparaSecciones+p.altoSeccion(util))
}

func (p *paginaAjustes) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	izq, util := p.reparto(t.Width)
	textoAncho, alto := p.cabecera(util)
	y := RellenoYPagina
	p.titulo.Move(fyne.NewPos(izq, y))
	p.titulo.Resize(fyne.NewSize(textoAncho, ui.AltoParrafo(p.titulo, textoAncho)))
	prosa := min(textoAncho, AnchoProsa)
	p.desc.Move(fyne.NewPos(izq, y+ui.AltoParrafo(p.titulo, textoAncho)+tema.Espacio(1.5)))
	p.desc.Resize(fyne.NewSize(prosa, ui.AltoParrafo(p.desc, prosa)))
	a := p.accion.MinSize()
	p.accion.Move(fyne.NewPos(izq+util-a.Width, y))
	p.accion.Resize(a)
	y += alto + SeparaSecciones
	p.seccion.Move(fyne.NewPos(izq, y))
	p.seccion.Resize(fyne.NewSize(util, p.altoSeccion(util)))
}

// ---- El detalle de una unidad ----

func (q *VistaEquipos) detalle(v tema.Variante) fyne.CanvasObject {
	return container.New(Pila{Elastico: 1}, q.cabeceraDetalle(v), q.cuerpoDetalle(v))
}

// cabeceraDetalle es la cabecera de la unidad: `bg-surface border-b px-4 py-2.5
// sm:px-6 gap-x-3`, con «← Unidades», la ruta y las acciones que el rol permite.
func (q *VistaEquipos) cabeceraDetalle(v tema.Variante) fyne.CanvasObject {
	u := q.Abierta
	volver := nuevoBotonTexto(q.t("team.back"), tema.TextoSM, ui.Medio, q.AlVolver)
	volver.icono, volver.ladoIcono, volver.hueco = iconos.ChevronIzquierda, 14, tema.Espacio(1)
	volver.margenX, volver.alto = tema.Espacio(2), tema.TextoSM.Linea+2*tema.Espacio(1)
	volver.tinta, volver.fondoEncima = "text", "surface-sunken"
	galon := iconos.TrazoConTinta(iconos.ChevronDerecha, ui.LadoGalonMiga, medioTexto(v))

	nombres := make([]string, 0, 1+len(q.Pila))
	nombres = append(nombres, u.Name)
	for _, c := range q.Pila {
		nombres = append(nombres, c.Nombre)
	}
	migas := ui.NuevasMigas(q.t("team.breadcrumb"), nombres, func(i int) {
		if q.AlIr != nil {
			q.AlIr(i - 1)
		}
	})

	var acciones []fyne.CanvasObject
	if u.ArchivedAt != nil {
		acciones = append(acciones, nuevaInsigniaSuelta(q.insigniaArchivada(), v))
	}
	acciones = append(acciones, nuevaInsigniaSuelta(q.insigniaRol(u.Role), v))
	inerte := q.Ocupado || q.SinRed()
	if u.PuedeEscribir() {
		nueva := nuevoBotonTexto(q.t("team.newFolder"), tema.TextoSM, ui.SemiNegrita, q.AlNuevaCarpeta)
		nueva.margenX, nueva.alto, nueva.borde = tema.Espacio(3), AltoControlCromo, true
		nueva.tinta, nueva.fondoEncima, nueva.tokenBorde = "text", "surface-sunken", "border-strong"
		nueva.Inerte = inerte
		subir := ui.NuevoBoton(q.t("team.upload"), q.AlSubir).Con(ui.Primario, ui.TallaSM).ConIcono(iconos.Mas, 16)
		subir.Inerte, subir.Cargando = inerte && !q.Subiendo, q.Subiendo
		acciones = append(acciones, nueva, subir)
	}
	if u.Role == api.RolGestionar {
		miembros := ui.NuevoBoton(q.t("team.members"), q.AlMiembros).Con(ui.Secundario, ui.TallaSM)
		renombrar := ui.NuevoBoton(q.t("team.rename"), q.AlRenombrar).Con(ui.Fantasma, ui.TallaSM)
		clave := "team.archive"
		if u.ArchivedAt != nil {
			clave = "team.unarchive"
		}
		archivar := ui.NuevoBoton(q.t(clave), q.AlArchivar).Con(ui.Fantasma, ui.TallaSM)
		for _, b := range []*ui.Boton{miembros, renombrar, archivar} {
			b.Inerte = inerte
			acciones = append(acciones, b)
		}
	}
	c := &cabeceraEquipo{volver: volver, galon: galon, migas: migas, acciones: acciones, ventana: q.Ventana}
	hijos := make([]fyne.CanvasObject, 0, 5+len(acciones))
	hijos = append(hijos, canvas.NewRectangle(tema.Color("surface", v)), canvas.NewRectangle(tema.Color("border", v)),
		volver, galon.Objeto(), migas)
	return container.New(c, append(hijos, acciones...)...)
}

// medioTexto es el `text-text/50` del galón.
func medioTexto(v tema.Variante) color.NRGBA {
	c := tema.Color("text", v)
	c.A /= 2
	return c
}

// cabeceraEquipo coloca la cabecera del detalle en una línea de 28.
//
// ponytail: sin `flex-wrap`, como la del explorador: en un móvil la ruta se estrecha.
type cabeceraEquipo struct {
	volver   *botonTexto
	galon    *iconos.Dibujo
	migas    *ui.Migas
	acciones []fyne.CanvasObject
	ventana  float32
}

// AltoCabeceraEquipo es `py-2.5` a cada lado de la línea de 28 y el `border-b`.
var AltoCabeceraEquipo = 2*RellenoYCabecera + AltoControlCromo + tema.Borde

func (c *cabeceraEquipo) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, AltoCabeceraEquipo)
}

func (c *cabeceraEquipo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	objs[1].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	canalon := CanalonCabecera(c.ventana)
	linea := AltoControlCromo
	poner := func(o fyne.CanvasObject, x, ancho float32) {
		h := o.MinSize().Height
		o.Move(fyne.NewPos(x, RellenoYCabecera+(linea-h)/2))
		o.Resize(fyne.NewSize(ancho, h))
	}
	derecha := t.Width - canalon
	for i := len(c.acciones) - 1; i >= 0; i-- {
		m := c.acciones[i].MinSize()
		poner(c.acciones[i], derecha-m.Width, m.Width)
		derecha -= m.Width + HuecoAcciones
	}
	x := canalon
	mv := c.volver.MinSize()
	poner(c.volver, x, mv.Width)
	x += mv.Width + HuecoCabecera
	c.galon.Colocar(x, RellenoYCabecera+(linea-ui.LadoGalonMiga)/2, ui.LadoGalonMiga)
	x += ui.LadoGalonMiga + 0.5*0.25*tema.Rem
	poner(c.migas, x, min(c.migas.MinSize().Width, max(derecha-HuecoCabecera+HuecoAcciones-x, 0)))
}

// cuerpoDetalle es lo de la carpeta, o su estado.
func (q *VistaEquipos) cuerpoDetalle(v tema.Variante) fyne.CanvasObject {
	escribe := q.Abierta.PuedeEscribir() && !q.SinRed()
	switch {
	case q.FaseDetalle == Cargando:
		return container.NewStack(canvas.NewRectangle(tema.Color("bg", v)), container.NewVScroll(esqueletoFilas()))
	case q.Denegada:
		x := ui.NuevoVacio(q.t("team.detail.deniedTitle"), q.t("team.detail.deniedBody"))
		x.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto()
		x.Accion = ui.NuevoBoton(q.t("team.detail.backCta"), q.AlVolver).Con(ui.Secundario, ui.TallaSM)
		return centrar(x, v)
	case q.FaseDetalle == Vacia:
		clave := "team.folder.emptyReadonly"
		if escribe {
			clave = "team.folder.emptyWritable"
		}
		x := ui.NuevoVacio(q.t("team.folder.emptyTitle"), q.t(clave))
		x.Ilustracion = ilustracion(iconos.VacioDrive, v)
		if escribe {
			x.Accion = ui.NuevoBoton(q.t("team.folder.uploadCta"), q.AlSubir).Con(ui.Primario, ui.TallaSM).ConIcono(iconos.Mas, 16)
		}
		return centrar(x, v)
	case q.FaseDetalle == Fallo:
		x := ui.NuevoVacio(q.t("team.detail.loadTitle"), q.t("team.error.network"))
		x.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto()
		x.Accion = ui.NuevoBoton(q.t("team.retry"), q.AlReintentarDetalle).Con(ui.Secundario, ui.TallaSM)
		return centrar(x, v)
	case q.FaseDetalle != Exito:
		return centrar(problemaVista(q.tr, q.FaseDetalle, "team.detail.loadTitle", q.AlReintentarDetalle, v), v)
	}
	filas := make([]fyne.CanvasObject, 0, len(q.Contenido)+1)
	for _, el := range q.Contenido {
		filas = append(filas, q.filaContenido(el, escribe, v))
	}
	lista := container.New(&listaEquipo{}, filas...)
	var contenido fyne.CanvasObject = lista
	if q.HayMas {
		mas := ui.NuevoBoton(q.t("team.loadMore"), q.AlCargarMas).Con(ui.Secundario, ui.TallaSM)
		mas.Cargando = q.CargandoMas
		contenido = container.New(&pilaSeparada{separas: []float32{0, tema.Espacio(4)}}, lista,
			container.New(&altoFijo{alto: mas.MinSize().Height + tema.Espacio(4)}, container.NewCenter(mas)))
	}
	if q.desplazableDetalle == nil {
		q.desplazableDetalle = container.NewVScroll(contenido)
	} else {
		q.desplazableDetalle.Content = contenido
		q.desplazableDetalle.Refresh()
	}
	return container.NewStack(canvas.NewRectangle(tema.Color("bg", v)), q.desplazableDetalle)
}

// filaContenido es un elemento de la unidad: el glifo de 22 en su caja de 31,5 (carpeta
// en `primary-text`, fichero en `text-muted`), el nombre, «Carpeta» o el tamaño, y la
// descarga y el borrado, que solo aparecen al pasar por encima.
func (q *VistaEquipos) filaContenido(el Elemento, escribe bool, v tema.Variante) *FilaPlana {
	elemento := el
	f := NuevaFilaPlana(el.Nombre)
	icono, tinta := iconos.DriveFichero, "text-muted"
	if el.Carpeta {
		icono, tinta = iconos.DriveCarpeta, "primary-text"
	}
	f.Glifo = iconos.Trazo(icono, LadoGlifoEquipo, tinta, v)
	f.LadoGlifo, f.CajaGlifo, f.TintaGlifo = LadoGlifoEquipo, CajaGlifoEquipo, tinta
	f.Meta = Tamano(q.loc, el.Tamano)
	if el.Carpeta {
		f.Meta = q.t("team.folder.label")
	}
	f.Grueso = punteroGrueso()
	f.AlAbrir = func() {
		if q.AlAbrir != nil {
			q.AlAbrir(elemento)
		}
	}
	if !el.Carpeta {
		f.Botones = append(f.Botones, BotonFila{
			Cuerpo: glifoDescargar, Tinta: "text-muted", TintaEncima: "text", FondoEncima: "bg",
			Lado: LadoAccionEquipo, LadoGlifo: 18, SoloAlPasar: true,
			Rotulo:   q.t("team.item.download", i18n.Vars{"name": el.Nombre}),
			AlPulsar: func(fyne.CanvasObject) { q.AlDescargar(elemento) },
		})
	}
	if escribe {
		f.Botones = append(f.Botones, BotonFila{
			Cuerpo: glifoPapelera, Tinta: "text-muted", TintaEncima: "danger-text", FondoEncima: "danger-bg",
			Lado: LadoAccionEquipo, LadoGlifo: 18, SoloAlPasar: true, Inerte: q.Ocupado,
			Rotulo:   q.t("team.item.delete", i18n.Vars{"name": el.Nombre}),
			AlPulsar: func(fyne.CanvasObject) { q.AlBorrar(elemento) },
		})
	}
	return f
}

// listaEquipo es el `px-3 pb-2` de la lista de una unidad.
type listaEquipo struct{}

func (listaEquipo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	alto := tema.Espacio(2)
	for _, o := range objs {
		alto += o.MinSize().Height
	}
	return fyne.NewSize(0, alto)
}

func (listaEquipo) Layout(objs []fyne.CanvasObject, t fyne.Size) { listaPlana{}.Layout(objs, t) }
