// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/portal/src/pages/notificaciones.astro

package hub

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La bandeja de avisos: `NotificationsInbox.tsx` dentro de `notificaciones.astro`. Es la
// vista COMPLETA de lo que el panel de la campana solo asoma —filtros, «marcar todas» y
// las reglas de silencio—, y es adonde lleva «Ver todas» desde cualquier app de la suite.
//
// Las medidas salen de las clases con el rem de 14; las piezas que repite (cabecera,
// píldora, conmutador, estado vacío y botón) son las ya medidas en producción en las
// otras pantallas. La bandeja en sí NO se ha medido contra producción: la sonda del
// portal no la recorre todavía.
const (
	// MaxAvisosBandeja es el `limit: 100` de la carga.
	MaxAvisosBandeja = 100
	// HuecoBandeja es el `flex flex-col gap-6` entre la barra, la lista y las reglas.
	HuecoBandeja float32 = 6 * 0.25 * tema.Rem
	// HuecoBarraAvisos es el `gap-3` entre los filtros y «Marcar todas».
	HuecoBarraAvisos float32 = 3 * 0.25 * tema.Rem

	// La fila: `flex items-start gap-3 rounded-md px-1 py-3`.
	MargenXAviso float32 = 1 * 0.25 * tema.Rem
	MargenYAviso float32 = 3 * 0.25 * tema.Rem
	HuecoAviso   float32 = 3 * 0.25 * tema.Rem
	// LadoMarcaAviso es el `<ProductIcon size={18} />`, con su `mt-0.5`.
	LadoMarcaAviso float32 = 18
	// SangriaMetaAviso es el `mt-1` de la línea «Mail · hace 5 min».
	SangriaMetaAviso float32 = 1 * 0.25 * tema.Rem
	// LadoPuntoSinLeer es el `h-2 w-2` del punto y SangriaPuntoAviso su `mt-2`.
	LadoPuntoSinLeer  float32 = 2 * 0.25 * tema.Rem
	SangriaPuntoAviso float32 = 2 * 0.25 * tema.Rem

	// El esqueleto: `Skeleton circle 18px` y dos barras de 14px y 12px con `gap-2`. Son
	// píxeles LITERALES en la web, no rem.
	AltoBarraEsqueleto1  float32 = 14
	AltoBarraEsqueleto2  float32 = 12
	FilasEsqueletoAvisos         = 4

	// Las reglas: `mt-2 border-t pt-6`, el `h2` de `text-base`, el párrafo con
	// `mt-1 mb-4` y los interruptores en `flex-col gap-3`.
	MargenReglas       float32 = 2 * 0.25 * tema.Rem
	RellenoReglas      float32 = 6 * 0.25 * tema.Rem
	SangriaTextoReglas float32 = 1 * 0.25 * tema.Rem
	MargenTextoReglas  float32 = 4 * 0.25 * tema.Rem
	HuecoReglas        float32 = 3 * 0.25 * tema.Rem
)

// OrigenesSilenciables son los seis productos con interruptor, en el orden de la web.
var OrigenesSilenciables = []string{"mail", "drive", "docs", "calendar", "contacts", "rooms"}

// Silenciado es el `silenciado` de la web: solo cuenta la regla del ORIGEN entero (sin
// `kind`). Las de un tipo concreto las pone quien las pone, no esta pantalla.
func Silenciado(reglas []api.HubNotificationPref, origen string) bool {
	for _, p := range reglas {
		if p.Source == origen && p.Kind == "" && p.Muted {
			return true
		}
	}
	return false
}

// ConRegla sustituye la regla del origen entero por la nueva, dejando las demás: el
// reductor optimista de `silenciar`.
func ConRegla(reglas []api.HubNotificationPref, origen string, silenciar bool) []api.HubNotificationPref {
	out := make([]api.HubNotificationPref, 0, len(reglas)+1)
	for _, p := range reglas {
		if p.Source != origen || p.Kind != "" {
			out = append(out, p)
		}
	}
	return append(out, api.HubNotificationPref{Source: origen, Muted: silenciar})
}

// Visibles son los avisos que pasan los dos filtros: «sin leer» y el producto.
func Visibles(avisos []api.HubNotification, soloSinLeer bool, producto string) []api.HubNotification {
	var out []api.HubNotification
	for _, n := range avisos {
		if (!soloSinLeer || !n.Read) && (producto == "" || n.Source == producto) {
			out = append(out, n)
		}
	}
	return out
}

// Avisos es la pantalla `/notificaciones`.
type Avisos struct {
	pagina

	T *i18n.Translator
	// Ahora es la hora contra la que se dice «hace 5 min»; con el cero, la del reloj.
	Ahora time.Time

	Estado Estado
	Items  []api.HubNotification
	Reglas []api.HubNotificationPref

	// Los dos filtros son estado de la PANTALLA, como en la web: no piden nada al Hub.
	SoloSinLeer bool
	Producto    string

	// HayMas: quedan avisos más antiguos; AlCargarMas los pide («Cargar más»).
	HayMas      bool
	AlCargarMas func()

	AlAbrir       func(api.HubNotification)
	AlMarcarTodas func()
	AlSilenciar   func(origen string, silenciar bool)
	AlReintentar  func()
}

var _ fyne.Widget = (*Avisos)(nil)

// NuevaAvisos crea la pantalla.
func NuevaAvisos(t *i18n.Translator) *Avisos {
	a := &Avisos{T: t}
	a.ExtendBaseWidget(a)
	return a
}

// Resize recuerda el ancho: el alto depende de él (los avisos se reparten en líneas).
func (a *Avisos) Resize(s fyne.Size) {
	if a.recordar(s) {
		a.BaseWidget.Resize(s)
		a.Refresh()
		return
	}
	a.BaseWidget.Resize(s)
}

// AccessibilityRole: la pantalla agrupa.
func (a *Avisos) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

func (a *Avisos) ahora() time.Time {
	if a.Ahora.IsZero() {
		return time.Now()
	}
	return a.Ahora
}

// sinLeer son los no leídos de lo cargado: es lo que cuenta la píldora de la web, no el
// contador del servidor.
func (a *Avisos) sinLeer() int {
	n := 0
	for _, x := range a.Items {
		if !x.Read {
			n++
		}
	}
	return n
}

// CreateRenderer construye la pantalla. Los seis interruptores se crean UNA vez y
// sobreviven a los repintados: rehacerlos al pulsar uno le quitaba el foco a quien lo
// maneja con el teclado.
func (a *Avisos) CreateRenderer() fyne.WidgetRenderer {
	r := &avisosRenderer{a: a, fondo: ui.NuevoFondoSuite(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))}
	for _, clave := range OrigenesSilenciables {
		nombre := clave
		if p, ok := ProductoDe(clave); ok {
			nombre = p.Nombre
		}
		c := ui.NuevoConmutador(nombre, func(recibir bool) {
			if a.AlSilenciar != nil {
				a.AlSilenciar(clave, !recibir) // el interruptor dice «recibir»; el Hub guarda «silenciado»
			}
		})
		r.conmutadores = append(r.conmutadores, c)
	}
	r.rehacer()
	return r
}

type avisosRenderer struct {
	a *Avisos

	fondo     *ui.FondoSuite
	cabecera  *cabecera
	filtros   []*Pildora
	marcar    *ui.Boton
	esqueleto []fyne.CanvasObject
	aviso     *ui.Vacio
	filas     []*FilaAviso
	filetes   []*canvas.Rectangle
	mas       *ui.Boton

	fileteReglas *canvas.Rectangle
	tituloReglas *canvas.Text
	textoReglas  *ui.Parrafo
	conmutadores []*ui.Conmutador

	objetos []fyne.CanvasObject
}

// textosAvisos son las claves de sus dos estados propios.
func textosAvisos(soloSinLeer bool) Textos {
	if soloSinLeer {
		return Textos{Vacio: "inbox.emptyUnreadTitle", VacioCuerpo: "inbox.emptyUnreadBody"}
	}
	return Textos{Vacio: "inbox.emptyTitle", VacioCuerpo: "inbox.emptyBody"}
}

func (r *avisosRenderer) rehacer() {
	a, t := r.a, r.a.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	_, ancho := CajaContenido(a.Ancho())

	r.fondo.PonerVariante(v)
	r.cabecera = nuevaCabecera(v, t.T("inbox.overline", nil), t.T("inbox.heading", nil), t.T("inbox.intro", nil))
	r.cabecera.preparar(v, ancho)
	r.hacerBarra(t)
	r.esqueleto, r.aviso, r.filas, r.filetes, r.mas = nil, nil, nil, nil, nil

	visibles := Visibles(a.Items, a.SoloSinLeer, a.Producto)
	switch {
	case a.Estado == Cargando:
		for range FilasEsqueletoAvisos {
			r.esqueleto = append(r.esqueleto, ui.NuevoEsqueletoRedondo(LadoMarcaAviso),
				ui.NuevoEsqueleto(0, AltoBarraEsqueleto1), ui.NuevoEsqueleto(0, AltoBarraEsqueleto2))
		}
		r.filetes = Divisores(v, FilasEsqueletoAvisos)
	case a.Estado == Fallo:
		// Sin icono y con el botón en su talla de siempre: es el EmptyState de la
		// bandeja, no el `Aviso` de las otras pantallas.
		r.aviso = ui.NuevoVacio(t.T("inbox.errorTitle", nil), t.T("inbox.errorBody", nil))
		if a.AlReintentar != nil {
			r.aviso.Accion = ui.NuevoBoton(t.T("inbox.retry", nil), a.AlReintentar).Con(ui.Secundario, ui.TallaMD)
		}
	case a.Estado == SinConexion || a.Estado == Prohibido:
		r.aviso = Aviso(t, v, a.Estado, textosAvisos(false), a.AlReintentar)
	case len(visibles) == 0:
		textos := textosAvisos(a.SoloSinLeer)
		if a.HayMas {
			// Con más detrás, «nada que leer» o «estás al día» serían falsos: solo no hay
			// entre lo cargado, y «Cargar más» trae lo siguiente. Como en la web.
			textos.Vacio, textos.VacioCuerpo = "notifications.noneLoaded", ""
		}
		r.aviso = Aviso(t, v, SinNada, textos, nil)
	default:
		for _, n := range visibles {
			r.filas = append(r.filas, r.filaDe(n, ancho))
		}
		r.filetes = Divisores(v, len(r.filas))
	}
	// «Cargar más» también con un filtro que no deja ver nada: lo que busca puede estar
	// detrás. Como en la web.
	if a.Estado == Listo && a.HayMas && a.AlCargarMas != nil && len(a.Items) > 0 {
		r.mas = ui.NuevoBoton(t.T("notifications.loadMore", nil), a.AlCargarMas).Con(ui.Secundario, ui.TallaSM)
	}

	r.fileteReglas = canvas.NewRectangle(tema.Color("border", v))
	r.tituloReglas = ui.Texto(t.T("inbox.prefsTitle", nil), tema.TextoBase, ui.SemiNegrita, tema.Color("text", v))
	r.textoReglas = ui.NuevoParrafo(t.T("inbox.prefsBody", nil), tema.TextoSM, ui.Normal, "text-muted", v).
		A(min(ancho, AnchoIntro))
	for i, c := range r.conmutadores {
		c.Encendido = !Silenciado(a.Reglas, OrigenesSilenciables[i])
		c.Refresh()
	}
	r.recomponer()
}

// hacerBarra construye los filtros y «Marcar todas», que solo sale si hay algo sin leer.
func (r *avisosRenderer) hacerBarra(t *i18n.Translator) {
	a := r.a
	sinLeer := a.sinLeer()
	r.filtros = []*Pildora{
		NuevaPildora(t.T("inbox.all", nil), !a.SoloSinLeer, func() { a.SoloSinLeer = false; a.Refresh() }),
		NuevaPildora(t.T("inbox.unreadFilter", i18n.Vars{"count": sinLeer}), a.SoloSinLeer,
			func() { a.SoloSinLeer = true; a.Refresh() }),
	}
	// Los productos que HAN avisado de algo: ofrecer un filtro vacío es ruido.
	visto := map[string]bool{}
	for _, n := range a.Items {
		if visto[n.Source] {
			continue
		}
		visto[n.Source] = true
		clave := n.Source
		rotulo := clave
		p, ok := ProductoDe(clave)
		if ok {
			rotulo = p.NombreCorto()
		}
		pl := NuevaPildora(rotulo, a.Producto == clave, func() {
			if a.Producto == clave {
				a.Producto = ""
			} else {
				a.Producto = clave
			}
			a.Refresh()
		})
		if ok {
			pl.Marca, pl.ConMarca = p.Marca, true
		}
		r.filtros = append(r.filtros, pl)
	}
	r.marcar = nil
	if sinLeer > 0 {
		r.marcar = ui.NuevoBoton(t.T("inbox.markAll", nil), func() {
			if a.AlMarcarTodas != nil {
				a.AlMarcarTodas()
			}
		}).Con(ui.Secundario, ui.TallaMD)
	}
}

func (r *avisosRenderer) filaDe(n api.HubNotification, ancho float32) *FilaAviso {
	a := r.a
	origen := n.Source
	p, ok := ProductoDe(n.Source)
	if ok {
		origen = p.NombreCorto()
	}
	// El título en el idioma de la app y el cuerpo con la hora local: los mismos que la
	// campana de la barra y que la bandeja de la web.
	f := NuevaFilaAviso(ui.TituloAviso(a.T, n), ui.CuerpoAviso(a.T.Locale(), n), ui.ProcedenciaAviso(a.T, n)+origen+" · "+i18n.FormatTimeAgo(a.T.Locale(), n.CreatedAt, a.ahora()),
		func() {
			if a.AlAbrir != nil {
				a.AlAbrir(n)
			}
		})
	f.Conocido, f.Marca = ok, p.Marca
	f.SinLeer, f.Enlace, f.Ancho = !n.Read, n.URL != "", ancho
	f.RotuloPunto = a.T.T("inbox.unreadDot", nil)
	return f
}

func (r *avisosRenderer) recomponer() {
	r.objetos = []fyne.CanvasObject{r.fondo}
	r.objetos = append(r.objetos, r.cabecera.objetos()...)
	for _, p := range r.filtros {
		r.objetos = append(r.objetos, p)
	}
	if r.marcar != nil {
		r.objetos = append(r.objetos, r.marcar)
	}
	r.objetos = append(r.objetos, r.esqueleto...)
	if r.aviso != nil {
		r.objetos = append(r.objetos, r.aviso)
	}
	for _, f := range r.filas {
		r.objetos = append(r.objetos, f)
	}
	for _, f := range r.filetes {
		r.objetos = append(r.objetos, f)
	}
	if r.mas != nil {
		r.objetos = append(r.objetos, r.mas)
	}
	r.objetos = append(r.objetos, r.fileteReglas, r.tituloReglas, r.textoReglas)
	for _, c := range r.conmutadores {
		r.objetos = append(r.objetos, c)
	}
}

func (r *avisosRenderer) MinSize() fyne.Size { return fyne.NewSize(0, r.alto(r.a.Ancho())) }

func (r *avisosRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *avisosRenderer) Destroy()                     {}

// Refresh rehace la pantalla con los datos de ahora (ver el comentario del lanzador).
func (r *avisosRenderer) Refresh() {
	r.rehacer()
	r.Layout(r.a.Size())
	canvas.Refresh(r.a)
}

// AltoFilaEsqueleto es una fila de carga: `py-3` y la columna de dos barras con `gap-2`.
func AltoFilaEsqueleto() float32 {
	return 2*MargenYAviso + max(LadoMarcaAviso, AltoBarraEsqueleto1+HuecoFiltros+AltoBarraEsqueleto2)
}

// alto es lo que ocupa la pantalla entera. Es la ÚNICA cuenta: Layout recorre lo mismo.
func (r *avisosRenderer) alto(ventana float32) float32 {
	x, ancho := CajaContenido(ventana)
	y := MargenY + r.cabecera.alto(ancho) + MargenCabeceraActividad
	y += r.barra(x, y, ancho, false) + HuecoBandeja
	y += r.altoLista(ancho) + HuecoBandeja
	return y + r.altoReglas() + MargenY
}

func (r *avisosRenderer) altoLista(ancho float32) float32 {
	var extra float32
	if r.mas != nil {
		extra = HuecoBandeja + r.mas.MinSize().Height // `flex-col gap-6` hasta el botón
	}
	switch {
	case len(r.esqueleto) > 0:
		return FilasEsqueletoAvisos*AltoFilaEsqueleto() + (FilasEsqueletoAvisos-1)*tema.Borde
	case r.aviso != nil:
		return r.aviso.MedidaVacio(ancho) + extra
	}
	var alto float32
	for i, f := range r.filas {
		if i > 0 {
			alto += tema.Borde // `divide-y`
		}
		alto += AltoFilaAviso(f, ancho)
	}
	return alto + extra
}

func (r *avisosRenderer) altoReglas() float32 {
	alto := MargenReglas + tema.Borde + RellenoReglas + tema.TextoBase.Linea + SangriaTextoReglas
	alto += ui.AltoParrafo(r.textoReglas, r.textoReglas.Ancho) + MargenTextoReglas
	for i, c := range r.conmutadores {
		if i > 0 {
			alto += HuecoReglas
		}
		alto += c.MinSize().Height
	}
	return alto
}

// barra coloca los filtros y el botón (`flex flex-wrap items-center justify-between
// gap-3`, con los filtros en su propio `flex-wrap gap-2`) y devuelve el alto. Con `mover`
// a false solo mide: así `alto` y `Layout` no pueden discrepar.
//
// Es lo que hace el navegador: si la fila de filtros y el botón caben, van en una línea
// y centrados en vertical; si no, el botón baja a la siguiente, a la izquierda, y los
// filtros se reparten en todo el ancho.
func (r *avisosRenderer) barra(x, y, ancho float32, mover bool) float32 {
	altoPildora := MedidaPildora("").Height
	var natural float32
	for i, p := range r.filtros {
		if i > 0 {
			natural += HuecoFiltros
		}
		natural += p.MinSize().Width
	}
	var boton fyne.Size
	if r.marcar != nil {
		boton = r.marcar.MinSize()
	}
	enLinea := r.marcar == nil || natural+HuecoBarraAvisos+boton.Width <= ancho

	// Los filtros, repartidos en filas dentro del ancho.
	filas := 1
	px, py := x, float32(0)
	sitios := make([]fyne.Position, len(r.filtros))
	for i, p := range r.filtros {
		w := p.MinSize().Width
		if px > x && px+w > x+ancho {
			px, py = x, py+altoPildora+HuecoFiltros
			filas++
		}
		sitios[i] = fyne.NewPos(px, py)
		px += w + HuecoFiltros
	}
	altoNav := float32(filas)*altoPildora + float32(filas-1)*HuecoFiltros

	navY, botonPos := y, fyne.NewPos(x, y+altoNav+HuecoBarraAvisos)
	total := altoNav
	if r.marcar != nil {
		total = altoNav + HuecoBarraAvisos + boton.Height
	}
	if enLinea {
		total = max(altoNav, boton.Height)
		navY = y + (total-altoNav)/2
		botonPos = fyne.NewPos(x+ancho-boton.Width, y+(total-boton.Height)/2)
	}
	if mover {
		for i, p := range r.filtros {
			p.Move(fyne.NewPos(sitios[i].X, navY+sitios[i].Y))
			p.Resize(p.MinSize())
		}
		if r.marcar != nil {
			r.marcar.Move(botonPos)
			r.marcar.Resize(boton)
		}
	}
	return total
}

// Layout recorre lo mismo que `alto`, en el mismo orden.
func (r *avisosRenderer) Layout(t fyne.Size) {
	if t.Width <= 0 {
		return
	}
	r.fondo.Resize(fyne.NewSize(t.Width, max(t.Height, r.alto(t.Width))))
	x, ancho := CajaContenido(t.Width)
	y := r.cabecera.colocar(x) + MargenCabeceraActividad
	y += r.barra(x, y, ancho, true) + HuecoBandeja

	switch {
	case len(r.esqueleto) > 0:
		r.colocarEsqueleto(x, y, ancho)
	case r.aviso != nil:
		r.aviso.Move(fyne.NewPos(x, y))
		r.aviso.Resize(fyne.NewSize(ancho, r.aviso.MedidaVacio(ancho)))
	}
	fy := y
	for i, f := range r.filas {
		if i > 0 {
			ColocarDivisor(r.filetes[i-1], x, fy, ancho)
			fy += tema.Borde
		}
		h := AltoFilaAviso(f, ancho)
		f.Move(fyne.NewPos(x, fy))
		f.Resize(fyne.NewSize(ancho, h))
		fy += h
	}
	if r.mas != nil {
		if r.aviso != nil {
			fy += r.aviso.MedidaVacio(ancho)
		}
		r.mas.Move(fyne.NewPos(x, fy+HuecoBandeja))
		r.mas.Resize(r.mas.MinSize())
	}
	y += r.altoLista(ancho) + HuecoBandeja
	r.colocarReglas(x, y, ancho)
}

// colocarEsqueleto pone las cuatro filas de carga: el círculo centrado y las dos barras
// al 45 % y al 70 % de la columna de texto.
func (r *avisosRenderer) colocarEsqueleto(x, y, ancho float32) {
	columna := ancho - 2*MargenXAviso - LadoMarcaAviso - HuecoAviso
	cx := x + MargenXAviso + LadoMarcaAviso + HuecoAviso
	altoCol := AltoBarraEsqueleto1 + HuecoFiltros + AltoBarraEsqueleto2
	for i := range FilasEsqueletoAvisos {
		fy := y + float32(i)*(AltoFilaEsqueleto()+tema.Borde)
		if i > 0 {
			ColocarDivisor(r.filetes[i-1], x, fy-tema.Borde, ancho)
		}
		interior := fy + MargenYAviso + (max(LadoMarcaAviso, altoCol)-altoCol)/2
		circulo, b1, b2 := r.esqueleto[3*i], r.esqueleto[3*i+1], r.esqueleto[3*i+2]
		circulo.Move(fyne.NewPos(x+MargenXAviso, fy+MargenYAviso+(max(LadoMarcaAviso, altoCol)-LadoMarcaAviso)/2))
		circulo.Resize(fyne.NewSize(LadoMarcaAviso, LadoMarcaAviso))
		b1.Move(fyne.NewPos(cx, interior))
		b1.Resize(fyne.NewSize(columna*0.45, AltoBarraEsqueleto1))
		b2.Move(fyne.NewPos(cx, interior+AltoBarraEsqueleto1+HuecoFiltros))
		b2.Resize(fyne.NewSize(columna*0.7, AltoBarraEsqueleto2))
	}
}

func (r *avisosRenderer) colocarReglas(x, y, ancho float32) {
	y += MargenReglas
	r.fileteReglas.Move(fyne.NewPos(x, y))
	r.fileteReglas.Resize(fyne.NewSize(ancho, tema.Borde))
	y += tema.Borde + RellenoReglas
	ui.ColocarTexto(r.tituloReglas, tema.TextoBase, x, y)
	y += tema.TextoBase.Linea + SangriaTextoReglas
	alto := ui.AltoParrafo(r.textoReglas, r.textoReglas.Ancho)
	r.textoReglas.Move(fyne.NewPos(x, y))
	r.textoReglas.Resize(fyne.NewSize(r.textoReglas.Ancho, alto))
	y += alto + MargenTextoReglas
	for _, c := range r.conmutadores {
		m := c.MinSize()
		c.Move(fyne.NewPos(x, y))
		c.Resize(m)
		y += m.Height + HuecoReglas
	}
}

// ---- La fila de aviso ----

// FilaAviso es un aviso de la bandeja. A diferencia de las filas de la actividad, el
// título y el cuerpo NO se truncan: son `block` y se reparten en las líneas que hagan
// falta, así que el alto depende del ancho (AltoFilaAviso).
type FilaAviso struct {
	ui.Pulsable

	Titulo, Cuerpo, Meta string
	Marca                iconos.Producto
	Conocido             bool
	// SinLeer pone el título en `font-semibold` y el punto de la derecha.
	SinLeer bool
	// Enlace es que el aviso lleva a algún sitio: en la web es un `<a>`, y si no, un
	// `<button>` que solo lo marca como leído.
	Enlace bool
	// RotuloPunto es el `aria-label` del punto («Sin leer»).
	RotuloPunto string
	// Ancho es el de la lista, que es el que decide en cuántas líneas cae el texto.
	Ancho float32
}

var _ fyne.Widget = (*FilaAviso)(nil)

// NuevaFilaAviso crea la fila.
func NuevaFilaAviso(titulo, cuerpo, meta string, alPulsar func()) *FilaAviso {
	f := &FilaAviso{Titulo: titulo, Cuerpo: cuerpo, Meta: meta}
	f.Nombre = titulo
	f.AlPulsar = alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityRole: enlace si lleva a algún sitio, botón si solo marca.
func (f *FilaAviso) AccessibilityRole() fyne.AccessibleRole {
	if f.Enlace {
		return fyne.AccessibleRoleLink
	}
	return fyne.AccessibleRoleButton
}

// AccessibilityLabel es lo que se lee de corrido en la web: título, cuerpo, origen y
// cuándo, y el «Sin leer» del punto.
func (f *FilaAviso) AccessibilityLabel() string {
	s := f.Titulo
	for _, parte := range []string{f.Cuerpo, f.Meta} {
		if parte != "" {
			s += ". " + parte
		}
	}
	if f.SinLeer && f.RotuloPunto != "" {
		s += ". " + f.RotuloPunto
	}
	return s
}

func (f *FilaAviso) peso() ui.Peso {
	if f.SinLeer {
		return ui.SemiNegrita
	}
	return ui.Medio
}

// anchoTextoAviso es lo que le queda a la columna de texto en una fila de ese ancho.
func anchoTextoAviso(ancho float32, sinLeer bool) float32 {
	cabe := ancho - 2*MargenXAviso - LadoMarcaAviso - HuecoAviso
	if sinLeer {
		cabe -= HuecoAviso + LadoPuntoSinLeer
	}
	return max(cabe, 1)
}

// AltoFilaAviso es lo que ocupa una fila a ese ancho: `py-3` y lo más alto de la marca
// (con su `mt-0.5`), la columna de texto y el punto (con su `mt-2`).
func AltoFilaAviso(f *FilaAviso, ancho float32) float32 {
	cabe := anchoTextoAviso(ancho, f.SinLeer)
	texto := float32(len(ui.Repartir(f.Titulo, tema.TextoSM, f.peso(), cabe))) * tema.TextoSM.Linea
	if f.Cuerpo != "" {
		texto += SangriaResumen + float32(len(ui.Repartir(f.Cuerpo, tema.TextoSM, ui.Normal, cabe)))*tema.TextoSM.Linea
	}
	texto += SangriaMetaAviso + tema.TextoXS.Linea
	lado := max(texto, SangriaMarca+LadoMarcaAviso)
	if f.SinLeer {
		lado = max(lado, SangriaPuntoAviso+LadoPuntoSinLeer)
	}
	return 2*MargenYAviso + lado
}

// CreateRenderer compone la fila. Termina en Refresh (regla 7 de `ui.go`).
func (f *FilaAviso) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cabe := anchoTextoAviso(f.Ancho, f.SinLeer)
	r := &filaAvisoRenderer{
		f:      f,
		fondo:  canvas.NewRectangle(color.NRGBA{}),
		aro:    ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
		titulo: ui.NuevoParrafo(f.Titulo, tema.TextoSM, f.peso(), "text", v).A(cabe),
		meta:   ui.Texto(f.Meta, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.fondo.CornerRadius = tema.RadioMD
	r.objetos = append(r.aro.Objetos(), r.fondo)
	if f.Conocido {
		r.marca = iconos.Marca(f.Marca, LadoMarcaAviso, v)
		r.objetos = append(r.objetos, r.marca.Objeto())
	}
	r.objetos = append(r.objetos, r.titulo)
	if f.Cuerpo != "" {
		r.cuerpo = ui.NuevoParrafo(f.Cuerpo, tema.TextoSM, ui.Normal, "text-muted", v).A(cabe)
		r.objetos = append(r.objetos, r.cuerpo)
	}
	r.objetos = append(r.objetos, r.meta)
	if f.SinLeer {
		r.punto = canvas.NewCircle(tema.Color("primary", v))
		r.objetos = append(r.objetos, r.punto)
	}
	r.Refresh()
	return r
}

// fondo es `hover:bg-chrome-hover`, como la fila de la actividad.
func (f *FilaAviso) fondo(v tema.Variante) color.NRGBA {
	if f.Encima() || f.Enfocado() {
		return tema.Color("chrome-hover", v)
	}
	return color.NRGBA{}
}

type filaAvisoRenderer struct {
	f              *FilaAviso
	fondo          *canvas.Rectangle
	aro            *ui.Aro
	marca          *iconos.Dibujo
	titulo, cuerpo *ui.Parrafo
	meta           *canvas.Text
	punto          *canvas.Circle
	objetos        []fyne.CanvasObject
}

func (r *filaAvisoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, AltoFilaAviso(r.f, r.f.Ancho))
}

func (r *filaAvisoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaAvisoRenderer) Destroy()                     {}

// Layout coloca marca, columna de texto y punto, todo arriba (`items-start`).
func (r *filaAvisoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Resize(t)
	x, y := MargenXAviso, MargenYAviso
	if r.marca != nil {
		r.marca.Colocar(x, y+SangriaMarca, LadoMarcaAviso)
	}
	x += LadoMarcaAviso + HuecoAviso
	cabe := anchoTextoAviso(t.Width, r.f.SinLeer)
	alto := ui.AltoParrafo(r.titulo, cabe)
	r.titulo.Move(fyne.NewPos(x, y))
	r.titulo.Resize(fyne.NewSize(cabe, alto))
	y += alto
	if r.cuerpo != nil {
		y += SangriaResumen
		alto = ui.AltoParrafo(r.cuerpo, cabe)
		r.cuerpo.Move(fyne.NewPos(x, y))
		r.cuerpo.Resize(fyne.NewSize(cabe, alto))
		y += alto
	}
	ui.Truncar(r.meta, r.f.Meta, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(r.meta, tema.TextoXS, x, y+SangriaMetaAviso)
	if r.punto != nil {
		r.punto.Move(fyne.NewPos(t.Width-MargenXAviso-LadoPuntoSinLeer, MargenYAviso+SangriaPuntoAviso))
		r.punto.Resize(fyne.NewSize(LadoPuntoSinLeer, LadoPuntoSinLeer))
	}
}

// Refresh repinta con el estado del puntero y del foco.
func (r *filaAvisoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = r.f.fondo(v)
	r.fondo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.meta.Color = tema.Color("text-subtle", v)
	r.meta.Refresh()
	if r.punto != nil {
		r.punto.FillColor = tema.Color("primary", v)
		r.punto.Refresh()
	}
}
