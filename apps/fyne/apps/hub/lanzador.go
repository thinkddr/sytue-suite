// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/portal/src/pages/index.astro

package hub

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El lanzador: la portada de Sytue Hub (`Launcher.tsx` dentro de `index.astro`).
//
// Medido en producción a 1280×800 (`medidas/portal/inicio.ancho.json`), de arriba
// abajo y con la topbar ya descontada:
//
//	caja de contenido   x = 21, ancho 1238 (`max-w-[1280px] px-6 py-8`)
//	cabecera            1238 × 56, y = 28
//	sección de apps     1238 × 395,17 — rótulo 14 + `mb-4` 14 + rejilla 367,17
//	rejilla             5 columnas de 239,188 con `gap-3`, teselas de 115,39
//	«Hoy»               398,66 × 147,25
//	«Accesos rápidos»   398,67 × 432,5
//	«Reciente…»         818,3 × 413
//
// Entre bloques hay `mb-8` (28) y entre las tres tarjetas `gap-6` (21).
//
// Las tarjetas van en DOS columnas desde el PR #834 de la web: a la izquierda (1/3) «Hoy»
// y debajo «Accesos rápidos»; a la derecha (`lg:col-span-2`) lo reciente. Antes eran
// tres columnas con dos tarjetas —un hueco vacío de 420 × 433 a la derecha— y lo
// reciente bajaba a lo ancho, fuera de la primera pantalla. Medido en el frontal a
// 1280×800: Hoy en (21, 535), Accesos en (21, 688) y Reciente en (441, 535). Por debajo
// de `lg` todo va en una columna, en el mismo orden.
const (
	// MargenSeccion es el `mb-8` entre la cabecera, la rejilla y las tarjetas.
	MargenSeccion float32 = 8 * 0.25 * tema.Rem
	// MargenRotulo es el `mb-4` entre el rótulo «Aplicaciones» y la rejilla.
	MargenRotulo float32 = 4 * 0.25 * tema.Rem
	// HuecoRejilla es el `gap-3` de la rejilla de teselas.
	HuecoRejilla float32 = 3 * 0.25 * tema.Rem
	// HuecoTarjetas es el `gap-6` de la rejilla de tarjetas.
	HuecoTarjetas float32 = 6 * 0.25 * tema.Rem
	// AnchoAviso es el `max-w-3xl` del aviso de privacidad del pie.
	AnchoAviso float32 = 48 * tema.Rem
	// MargenAviso es su `pt-2`.
	MargenAviso float32 = 2 * 0.25 * tema.Rem
	// HuecoEsqueleto es el `gap-2` entre los dos esqueletos de la tarjeta «Hoy».
	HuecoEsqueleto float32 = 2 * 0.25 * tema.Rem
	// AltoParrafoHoy es el `py-3` del párrafo de estado de «Hoy».
	AltoParrafoHoy float32 = 3 * 0.25 * tema.Rem
)

// # La forma Marco: «Bienvenida y tablero»
//
// En Marco la portada es otra (las variantes `marco:` de `Launcher.tsx`): un saludo por
// la hora en vez de «Tus aplicaciones»; las apps en teselas compactas, tres columnas
// desde 1024; y «Hoy» y «Reciente» apiladas en una columna a su derecha. Los accesos
// rápidos dejan su tarjeta y pasan a píldoras bajo el saludo. En Carril, la de siempre.
//
// Las píldoras son cuatro y un «Más» que despliega las otras tres; «Buscar» no está,
// que ya lo lleva la barra. «Reciente» enseña las tres últimas. Y con seis apps o menos
// —dos filas—, en dos columnas, «Reciente» baja a la de las apps y al lado queda «Hoy».
//
// Medido por la web a 1280 (hoja de 1260, caja de 1212): columnas de 784 y 412 con 16
// entre ellas; teselas de 62 de alto con 10 de separación; «Hoy» y «Reciente» a 10.
const (
	// MargenYMarco es el `marco:py-5` de la página y MargenCabeceraMarco el
	// `marco:mb-3` de la cabecera.
	MargenYMarco        float32 = 5 * 0.25 * tema.Rem
	MargenCabeceraMarco float32 = 3 * 0.25 * tema.Rem
	// MargenPildoras es el `mt-4` de las píldoras bajo el saludo y HuecoPildoras su
	// `gap-2`, a lo ancho y entre líneas.
	MargenPildoras float32 = 4 * 0.25 * tema.Rem
	HuecoPildoras  float32 = 2 * 0.25 * tema.Rem
	// RecientesMarco son las entradas de «Reciente» que se pintan: `RECIENTES_EN_MARCO`
	// de `lib/reciente.ts`.
	RecientesMarco = 3
	// PocasApps es el `apps.length <= 6` con el que «Reciente» baja a la columna de las
	// apps: dos filas de tres.
	PocasApps = 6
	// HuecoColumnasMarco es el `marco:lg:gap-4` entre lo principal y el lado.
	HuecoColumnasMarco float32 = 4 * 0.25 * tema.Rem
	// FraccionPrincipalMarco y MinimoLadoMarco son el
	// `grid-cols-[minmax(0,1.9fr)_minmax(260px,1fr)]` del cuerpo.
	FraccionPrincipalMarco float32 = 1.9
	MinimoLadoMarco        float32 = 260
	// HuecoTeselasMarco es el `marco:gap-2.5` de la rejilla de apps y MinimoTeselaMarco
	// el `minmax(160px,1fr)` de sus columnas por debajo de 1024.
	HuecoTeselasMarco float32 = 2.5 * 0.25 * tema.Rem
	MinimoTeselaMarco float32 = 160
	// ColumnasTeselasMarco es el `marco:lg:grid-cols-3`.
	ColumnasTeselasMarco = 3
	// HuecoTarjetasMarco es el `marco:gap-2.5` entre «Hoy» y «Reciente».
	HuecoTarjetasMarco float32 = 2.5 * 0.25 * tema.Rem
)

// MomentoDelDia es `momentoDelDia` de la web: de 5 a 12, mañana; de 12 a 20, tarde; lo
// demás, noche. Devuelve el final de la clave `launcher.greeting.*`.
func MomentoDelDia(hora int) string {
	switch {
	case hora >= 5 && hora < 12:
		return "morning"
	case hora >= 12 && hora < 20:
		return "afternoon"
	}
	return "evening"
}

// columnasMarco reparte el cuerpo en lo principal y el lado: 1,9 a 1, con el lado en 260
// como poco, que es entonces lo que se le quita a lo principal.
func columnasMarco(ancho float32) (principal, lado float32) {
	libre := max(ancho-HuecoColumnasMarco, 0)
	lado = max(libre/(FraccionPrincipalMarco+1), MinimoLadoMarco)
	return max(libre-lado, 0), lado
}

// Cortes de la rejilla de teselas y de la de tarjetas, tal cual están en el JSX.
var (
	cortesApps     = ui.Cortes{2, 3, 0, 5, 0, 0} // grid-cols-2 sm:grid-cols-3 lg:grid-cols-5
	cortesTarjetas = ui.Cortes{1, 0, 0, 3, 0, 0} // grid gap-6 lg:grid-cols-3
)

// EventoDeHoy es lo que la tarjeta «Hoy» enseña de un evento: lo mismo que `resumir`
// deja en el BFF de la web (`/api/hoy`).
type EventoDeHoy struct {
	ID        string
	Resumen   string
	Inicio    time.Time
	Fin       time.Time
	TodoElDia bool
}

// OrdenarEventos es el `ordenarEventos` de `lib/hoy.ts`: todo el día delante y el
// resto por hora de inicio. No muta la lista que recibe.
func OrdenarEventos(in []EventoDeHoy) []EventoDeHoy {
	out := append([]EventoDeHoy(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && antes(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func antes(a, b EventoDeHoy) bool {
	if a.TodoElDia != b.TodoElDia {
		return a.TodoElDia
	}
	return a.Inicio.Before(b.Inicio)
}

// LimitesDelDia es el `limitesDelDia` de `lib/hoy.ts`: de la medianoche local a la
// siguiente. Los pone el CLIENTE y no el servidor, porque es quien sabe en qué zona
// horaria vive la persona.
func LimitesDelDia(ahora time.Time) (desde, hasta time.Time) {
	a, m, d := ahora.Date()
	desde = time.Date(a, m, d, 0, 0, 0, 0, ahora.Location())
	return desde, desde.AddDate(0, 0, 1)
}

// HaTerminado es el `haTerminado` de la web: un evento ya pasado se apaga en la
// lista, pero no desaparece.
func HaTerminado(e EventoDeHoy, ahora time.Time) bool {
	return !e.TodoElDia && !e.Fin.IsZero() && e.Fin.Before(ahora)
}

// Agenda es la mitad izquierda de la tarjeta «Hoy». Las dos fuentes fallan por su
// lado —así lo hace el BFF de la web, con un `Promise.all` de dos `catch`—: un Mail
// caído no deja la agenda a oscuras.
type Agenda struct {
	Estado  Estado
	Eventos []EventoDeHoy
	// HayCorreo dice si el buzón contestó; con false no se pinta el pie.
	HayCorreo bool
	SinLeer   int
}

// Reciente es la tarjeta de abajo: la actividad cross-app que sirve el Hub.
type Reciente struct {
	Estado Estado
	Items  []api.HubHit
}

// Lanzador es la portada.
type Lanzador struct {
	pagina

	T *i18n.Translator
	// Ahora es la fecha del sobretítulo y la que decide qué evento ha pasado ya. Se
	// inyecta para que el guardián de píxeles no dependa del reloj.
	Ahora time.Time
	// Nombre es el de pila de quien ha entrado, para el saludo de la forma Marco. Sin
	// él, el saludo solo.
	Nombre string
	// Apps son los productos a enseñar, ya filtrados por lo desplegado y SIN el Hub,
	// que es donde ya estás.
	Apps []Producto
	// Apagados son las claves de producto que esta cuenta no puede usar (SyERP en
	// una cuenta personal).
	Apagados map[string]bool
	// Sustitutos son los productos que la organización ha dado a una aplicación de
	// terceros: la tesela lo dice en su descripción y abre ESA aplicación, y los
	// accesos rápidos y los enlaces de las tarjetas también (el `productMap` de la web).
	Sustitutos map[string]api.Sustituto
	// Conectadas son las aplicaciones de terceros activas de la organización: van en
	// su propia rejilla, «Conectado», debajo de la de productos.
	Conectadas []api.AppConectada
	// PuedeAdmin dice si se pinta la tesela de Sytue Admin: el `puedeAdmin` que la web
	// decide en el servidor. Arranca en falso, como allí: una puerta que va a dar 403
	// no se ofrece, ni siquiera mientras se averigua.
	PuedeAdmin bool
	// These entries are independent of the paid Admin tile: personal free accounts
	// can connect storage, and Drive still authorizes availability on its API.
	PuedeConectarAlmacen bool
	PuedeConectarApps    bool
	AlConectarAlmacen    func()
	AlConectarApps       func()

	Agenda   Agenda
	Reciente Reciente

	// AlAbrir salta a otro producto: es una acción del SISTEMA, no una pantalla.
	AlAbrir func(p Producto, ruta string)
	// AlIr navega dentro de esta aplicación.
	AlIr func(Destino)
	// AlAbrirURL abre una dirección de fuera (el enlace de descargas apunta al sitio
	// público, igual que en la web).
	AlAbrirURL func(string)
	// AlReintentar vuelve a pedir la actividad reciente.
	AlReintentar func()
}

var _ fyne.Widget = (*Lanzador)(nil)

// NuevoLanzador crea la portada.
func NuevoLanzador(t *i18n.Translator, ahora time.Time) *Lanzador {
	l := &Lanzador{T: t, Ahora: ahora, Apagados: map[string]bool{}}
	l.ExtendBaseWidget(l)
	return l
}

// Resize recuerda el ancho de ventana: el alto de la portada depende de él (cuántas
// columnas tiene la rejilla, y por tanto cuántas filas).
func (l *Lanzador) Resize(s fyne.Size) {
	if l.recordar(s) {
		l.BaseWidget.Resize(s)
		l.Refresh()
		return
	}
	l.BaseWidget.Resize(s)
}

// AccessibilityRole: la portada agrupa.
func (l *Lanzador) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// CreateRenderer construye la portada entera.
func (l *Lanzador) CreateRenderer() fyne.WidgetRenderer {
	r := &lanzadorRenderer{l: l, fondo: ui.NuevoFondoSuite(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))}
	r.rehacer()
	return r
}

type lanzadorRenderer struct {
	l *Lanzador
	v tema.Variante
	// La forma con que se construyeron los objetos. Fyne puede pedir Layout antes
	// de Refresh al cambiar el tema; Marco y Carril llevan tarjetas distintas.
	marco bool

	atajosConexiones fyne.CanvasObject
	fondo            *ui.FondoSuite
	cabecera         *cabecera
	rotulo           *ui.Versal
	teselas          []*TeselaApp
	// rotuloCon y conectadas son la rejilla de aplicaciones conectadas; sin ninguna,
	// la sección no existe.
	rotuloCon  *ui.Versal
	conectadas []*TeselaConectada
	accesos    []*FilaAcceso
	// pildoras son las acciones del día de Marco; tiraPildoras, ellas ya repartidas en
	// líneas al ancho de la página, y altoPildoras lo que ocupan.
	pildoras     []fyne.CanvasObject
	tiraPildoras fyne.CanvasObject
	altoPildoras float32
	// menu es el último que desplegó «Más»: lo mira el guardián.
	menu *ui.Menu

	hoy       *tarjeta
	tAccesos  *tarjeta
	filasHoy  []fyne.CanvasObject // filas de evento y, al final, el pie de correo
	textoHoy  *canvas.Text        // el párrafo de «no hay nada» / «no disponible»
	esqueleto []fyne.CanvasObject

	recientes  *tarjeta
	filas      []*FilaReciente
	filetesRec []*canvas.Rectangle
	filetesHoy []*canvas.Rectangle
	avisoRec   *ui.Vacio
	esqRec     []fyne.CanvasObject
	privacidad []*canvas.Text

	objetos []fyne.CanvasObject
}

func (r *lanzadorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, r.alto(r.l.Ancho()))
}

func (r *lanzadorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *lanzadorRenderer) Destroy()                     {}

// Refresh rehace la portada con los datos de ahora y la vuelve a colocar.
//
// Rehacer y no solo repintar: lo que cambia entre un estado y otro es CUÁNTOS objetos
// hay —cuatro esqueletos, seis filas o un estado vacío—, y eso no se arregla cambiando
// colores. Y Fyne solo llama a Layout cuando cambia el tamaño, así que hay que
// llamarlo aquí o la portada se queda con la disposición anterior.
func (r *lanzadorRenderer) Refresh() {
	r.rehacer()
	r.Layout(r.l.Size())
	canvas.Refresh(r.l)
}

//nolint:gocyclo // rehacer lays out launcher UI blocks with dense options.
func (r *lanzadorRenderer) rehacer() {
	l := r.l
	r.marco = tema.EsMarco()
	r.v = tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	v, t := r.v, l.T
	_, ancho := CajaContenido(l.Ancho())

	r.fondo.PonerVariante(v)
	r.cabecera = nuevaCabecera(v, i18n.FormatWeekdayDayMonth(t.Locale(), l.Ahora), l.titulo(), "")
	r.cabecera.preparar(v, ancho)
	r.atajosConexiones = nil
	var conexiones []ui.Bloque
	tarjeta := func(titulo, descripcion string, hacer func()) {
		c := ui.NuevaTarjetaEleccion(titulo, descripcion, hacer).A(ancho)
		c.Densa = true
		conexiones = append(conexiones, ui.Bloque{Obj: c, Margen: tema.Espacio(2), Alto: func(a float32) float32 { c.Ancho = a; return ui.AltoTarjetaEleccion(c, a) }})
	}
	if l.PuedeConectarAlmacen {
		tarjeta(t.T("storage.connectTitle", nil), t.T("storage.connectDescription", nil), l.AlConectarAlmacen)
	}
	if l.PuedeConectarApps {
		tarjeta(t.T("apps.connect", nil), "", l.AlConectarApps)
	}
	if len(conexiones) > 0 {
		r.atajosConexiones = ui.PilaBloques(conexiones...)
	}
	r.rotulo = ui.NuevoVersal(t.T("launcher.apps", nil), tema.TextoXS, ui.SemiNegrita,
		tema.Color("text-muted", v))

	r.teselas = r.teselasDe()
	r.rotuloCon, r.conectadas = nil, nil
	if len(l.Conectadas) > 0 {
		r.rotuloCon = ui.NuevoVersal(t.T("launcher.connected", nil), tema.TextoXS, ui.SemiNegrita,
			tema.Color("text-muted", v))
		for _, a := range l.Conectadas {
			href := a.Href
			rotulo := t.T("launcher.appAria", i18n.Vars{"name": a.Name, "description": a.Description})
			r.conectadas = append(r.conectadas, NuevaTeselaConectada(a, rotulo, func() {
				if l.AlAbrirURL != nil {
					l.AlAbrirURL(href)
				}
			}))
		}
	}
	// En Marco los accesos rápidos no llevan tarjeta (`marco:hidden`): son las píldoras
	// de bajo el saludo (`carril:hidden`).
	r.accesos, r.tAccesos, r.pildoras, r.tiraPildoras, r.altoPildoras = nil, nil, nil, nil, 0
	if tema.EsMarco() {
		r.pildoras = r.pildorasDe(v)
		r.tiraPildoras, r.altoPildoras = repartirPildoras(r.pildoras, ancho)
	} else {
		r.accesos = r.accesosDe()
		r.tAccesos = nuevaTarjeta(v, t.T("launcher.quickActions", nil), nil)
	}
	r.hacerHoy(v)
	r.hacerReciente(v, ancho)
	r.privacidad = nil
	for _, linea := range ui.Repartir(t.T("privacy.notice", nil), tema.TextoXS, ui.Normal,
		min(ancho-2*MargenX, AnchoAviso-2*MargenX)) {
		r.privacidad = append(r.privacidad,
			ui.Texto(linea, tema.TextoXS, ui.Normal, tema.Color("text-muted", v)))
	}

	r.objetos = []fyne.CanvasObject{r.fondo}
	r.objetos = append(r.objetos, r.cabecera.objetos()...)
	if r.tiraPildoras != nil {
		r.objetos = append(r.objetos, r.tiraPildoras)
	}
	if r.atajosConexiones != nil {
		r.objetos = append(r.objetos, r.atajosConexiones)
	}
	r.objetos = append(r.objetos, r.rotulo.Objetos()...)
	for _, x := range r.teselas {
		r.objetos = append(r.objetos, x)
	}
	if r.rotuloCon != nil {
		r.objetos = append(r.objetos, r.rotuloCon.Objetos()...)
	}
	for _, x := range r.conectadas {
		r.objetos = append(r.objetos, x)
	}
	r.objetos = append(r.objetos, r.hoy.objetos()...)
	r.objetos = append(r.objetos, r.esqueleto...)
	if r.textoHoy != nil {
		r.objetos = append(r.objetos, r.textoHoy)
	}
	r.objetos = append(r.objetos, r.filasHoy...)
	for _, f := range r.filetesHoy {
		r.objetos = append(r.objetos, f)
	}
	if r.tAccesos != nil {
		r.objetos = append(r.objetos, r.tAccesos.objetos()...)
	}
	for _, a := range r.accesos {
		r.objetos = append(r.objetos, a)
	}
	r.objetos = append(r.objetos, r.recientes.objetos()...)
	r.objetos = append(r.objetos, r.esqRec...)
	if r.avisoRec != nil {
		r.objetos = append(r.objetos, r.avisoRec)
	}
	for _, f := range r.filas {
		r.objetos = append(r.objetos, f)
	}
	for _, f := range r.filetesRec {
		r.objetos = append(r.objetos, f)
	}
	for _, p := range r.privacidad {
		r.objetos = append(r.objetos, p)
	}
}

// titulo es el H1 de la portada: en Carril, «Tus aplicaciones»; en Marco, el saludo por
// la hora LOCAL, con el nombre de pila si se sabe.
func (l *Lanzador) titulo() string {
	if !tema.EsMarco() {
		return l.T.T("launcher.heading", nil)
	}
	saludo := l.T.T("launcher.greeting."+MomentoDelDia(l.Ahora.Hour()), nil)
	if l.Nombre == "" {
		return saludo
	}
	return saludo + ", " + l.Nombre
}

// dosColumnas dice si el cuerpo de Marco va en dos columnas: desde 1024 de VENTANA
// (`marco:lg:`). La portada mide lo que la hoja, que a ese ancho se separa del borde por
// los dos lados.
func (l *Lanzador) dosColumnas() bool {
	return tema.EsMarco() && l.Ancho()+2*tema.Forma("sheet-gap") >= ui.CorteLG
}

// rejillaApps es la rejilla de teselas de la forma en vigor: en Carril, los cortes de
// siempre; en Marco, tres columnas desde 1024 y, por debajo, las que quepan de 160.
func (r *lanzadorRenderer) rejillaApps(ventana float32) ui.Rejilla {
	switch {
	case !tema.EsMarco():
		return ui.RejillaTarjetas(ventana, HuecoRejilla, cortesApps)
	case r.l.dosColumnas():
		return ui.Rejilla{Fijas: ColumnasTeselasMarco, Hueco: HuecoTeselasMarco}
	}
	return ui.Rejilla{Min: MinimoTeselaMarco, Hueco: HuecoTeselasMarco}
}

// teselasDe construye la rejilla de productos.
func (r *lanzadorRenderer) teselasDe() []*TeselaApp {
	l, t := r.l, r.l.T
	out := make([]*TeselaApp, 0, len(l.Apps))
	for _, p := range l.Apps {
		// El Hub no se lanza desde el Hub, y Sytue Admin solo a quien puede entrar.
		if p.Marca == iconos.One || (p.Marca == iconos.Console && !l.PuedeAdmin) {
			continue
		}
		desc := t.T(p.ClaveDescripcion(), nil)
		// Una tesela sustituida lleva el nombre de la aplicación en su descripción:
		// quien la pulsa tiene que saber ANTES que sale a otro producto.
		if sub, ok := l.Sustitutos[p.Marca.Clave()]; ok {
			desc += " · " + sub.Name
		}
		apagada := l.Apagados[p.Marca.Clave()]
		rotulo := t.T("launcher.appAria", i18n.Vars{"name": p.Nombre, "description": desc})
		if apagada {
			rotulo = p.Nombre + " — " + desc + " (" + t.T("launcher.disabled", nil) + ")"
		}
		tesela := NuevaTeselaApp(p, desc, rotulo, r.abrir(p))
		tesela.Deshabilitada = apagada
		tesela.Inerte = apagada
		if apagada {
			tesela.RotuloDeshabilitada = t.T("launcher.disabled", nil)
		}
		out = append(out, tesela)
	}
	return out
}

func (r *lanzadorRenderer) abrir(p Producto) func() {
	return func() { r.l.abrirProducto(p.Marca.Clave(), "") }
}

// abrirProducto salta a un producto por su clave o, si la organización se lo ha dado a
// una aplicación de terceros, a ESA aplicación, sin la ruta: la de Sytue no existe en
// la otra y pegarla daría un enlace roto.
func (l *Lanzador) abrirProducto(clave, ruta string) {
	if sub, ok := l.Sustitutos[clave]; ok {
		if l.AlAbrirURL != nil {
			l.AlAbrirURL(sub.Href)
		}
		return
	}
	if p, ok := ProductoDe(clave); ok && l.AlAbrir != nil {
		l.AlAbrir(p, ruta)
	}
}

// acciones son los ocho accesos rápidos, en el orden de `QUICK_ACTIONS`. De aquí salen
// la tarjeta de Carril y las píldoras de Marco: una sola lista, como en la web.
func (r *lanzadorRenderer) acciones() []accion {
	l := r.l
	ir := func(d Destino) func() {
		return func() {
			if l.AlIr != nil {
				l.AlIr(d)
			}
		}
	}
	saltar := func(clave, ruta string) func() {
		return func() { l.abrirProducto(clave, ruta) }
	}
	// El destino de cada uno es el de la web, ruta a ruta. `quick.download` apunta al
	// sitio público y no a la pantalla de descargas de esta app: en la web tampoco
	// van al mismo sitio, y la paridad es de pantallas, no de criterio.
	return []accion{
		{"quick.search", GlifoAcceso{Icono: iconos.Buscar}, ir(ABuscar)},
		{"quick.compose", GlifoAcceso{Ruta: GlifoCorreo}, saltar("mail", "/compose")},
		{"quick.newDoc", GlifoAcceso{Ruta: GlifoFichero}, saltar("docs", "")},
		{"quick.upload", GlifoAcceso{Ruta: GlifoSubir}, saltar("drive", "/?upload=1")},
		{"quick.newEvent", GlifoAcceso{Ruta: GlifoAgenda}, saltar("calendar", "/new")},
		{"quick.download", GlifoAcceso{Ruta: GlifoDescarga}, r.abrirWeb()},
		{"quick.devices", GlifoAcceso{Ruta: GlifoTelefono}, ir(ADispositivos)},
		{"quick.data", GlifoAcceso{Ruta: GlifoArchivo}, ir(ADatos)},
	}
}

// accesosDe son esos ocho en la tarjeta «Accesos rápidos» de Carril.
func (r *lanzadorRenderer) accesosDe() []*FilaAcceso {
	acciones := r.acciones()
	out := make([]*FilaAcceso, 0, len(acciones))
	for _, e := range acciones {
		f := NuevaFilaAcceso(r.l.T.T(e.clave, nil), e.glifo, e.pulsar)
		f.Tono = tonosAcceso[len(out)%len(tonosAcceso)]
		out = append(out, f)
	}
	return out
}

func (r *lanzadorRenderer) abrirWeb() func() {
	return func() {
		if r.l.AlAbrirURL != nil {
			r.l.AlAbrirURL("https://www." + api.ApexProd + "/downloads")
		}
	}
}

// hacerHoy monta la tarjeta «Hoy» con el estado en el que esté la agenda.
func (r *lanzadorRenderer) hacerHoy(v tema.Variante) {
	l, t := r.l, r.l.T
	abrir := enlace(t.T("today.openCalendar", nil), ui.Medio, func() { l.abrirProducto("calendar", "") })
	r.hoy = nuevaTarjeta(v, t.T("today.title", nil), nil, abrir)
	r.filasHoy, r.textoHoy, r.esqueleto = nil, nil, nil

	switch l.Agenda.Estado {
	case Cargando:
		// «filas Skeleton con la geometría real de la lista»: dos rótulos de
		// `0.875rem` al 70 % y al 55 %, en un `flex-col gap-2 p-4`.
		r.esqueleto = []fyne.CanvasObject{
			ui.NuevoEsqueleto(0, tema.TextoRaiz.Tam), ui.NuevoEsqueleto(0, tema.TextoRaiz.Tam),
		}
	case SinNada:
		r.textoHoy = ui.Texto(t.T("today.noEvents", nil), tema.TextoSM, ui.Normal,
			tema.Color("text-muted", v))
	case Fallo, SinConexion, Prohibido:
		// La web enseña UNA línea apagada y no un estado vacío entero: la tarjeta es
		// de 398 px y el correo de abajo tiene que seguir viéndose.
		r.textoHoy = ui.Texto(t.T("today.unavailable", nil), tema.TextoSM, ui.Normal,
			tema.Color("text-muted", v))
	case Listo:
		for _, e := range OrdenarEventos(l.Agenda.Eventos) {
			r.filasHoy = append(r.filasHoy, nuevaFilaEvento(v, t, e, l.Ahora))
		}
	}
	if l.Agenda.HayCorreo && l.Agenda.Estado != Cargando {
		// El pie lleva su propio `border-t`, así que no cuenta para el `divide-y`.
		r.filetesHoy = Divisores(v, len(r.filasHoy))
		r.filasHoy = append(r.filasHoy, r.pieCorreo(v))
		return
	}
	r.filetesHoy = Divisores(v, len(r.filasHoy))
}

// pieCorreo es la línea de abajo de «Hoy»: la marca de Mail y el número de no leídos.
func (r *lanzadorRenderer) pieCorreo(v tema.Variante) fyne.CanvasObject {
	t := r.l.T
	texto := t.T("today.inboxZero", nil)
	if r.l.Agenda.SinLeer > 0 {
		texto = t.T("today.unread", i18n.Vars{"count": r.l.Agenda.SinLeer})
	}
	f := NuevaFilaCorreo(texto, func() { r.l.abrirProducto("mail", "") })
	_ = v
	return f
}

// hacerReciente monta la tarjeta «Reciente en toda la suite».
func (r *lanzadorRenderer) hacerReciente(v tema.Variante, ancho float32) {
	l, t := r.l, r.l.T
	verTodo := enlace(t.T("recent.viewAll", nil)+" →", ui.SemiNegrita, func() {
		if l.AlIr != nil {
			l.AlIr(AActividad)
		}
	})
	r.recientes = nuevaTarjeta(v, t.T("launcher.recentAll", nil), nil, verTodo)
	r.filas, r.avisoRec, r.esqRec = nil, nil, nil

	switch l.Reciente.Estado {
	case Cargando:
		for i := range 4 {
			ancho1, ancho2 := float32(0.6), float32(0.35)
			if i%2 == 1 {
				ancho1, ancho2 = 0.45, 0.7
			}
			interior := AnchoInterior(ancho)
			r.esqRec = append(r.esqRec,
				ui.NuevoEsqueletoRedondo(10),
				ui.NuevoEsqueleto(interior*ancho1, tema.TextoRaiz.Tam),
				ui.NuevoEsqueleto(interior*ancho2, tema.TextoXS.Tam),
				ui.NuevoEsqueleto(3*tema.Rem, tema.TextoXS.Tam))
		}
	case SinNada:
		r.avisoRec = Aviso(t, v, SinNada, textosReciente, func() { l.abrirProducto("drive", "") })
	case Fallo, SinConexion, Prohibido:
		r.avisoRec = Aviso(t, v, l.Reciente.Estado, textosReciente, l.AlReintentar)
	case Listo:
		items := l.Reciente.Items
		if tema.EsMarco() { // `recientesVisibles`: el resto está en «Ver todo»
			items = items[:min(len(items), RecientesMarco)]
		}
		for _, it := range items {
			r.filas = append(r.filas, r.filaDe(it))
		}
	}
	r.filetesRec = Divisores(v, len(r.filas))
}

// textosReciente son las claves propias de la tarjeta de actividad. El vacío lleva su
// propia llamada a la acción («Empieza en Drive»), que la web pinta como enlace de
// verdad y no como botón que navega.
var textosReciente = Textos{
	Vacio:       "launcher.noActivity",
	VacioCuerpo: "recent.emptyBody",
	Accion:      "recent.emptyCta",
	Fallo:       "launcher.activityError",
	FalloCuerpo: "recent.hubDown",
}

func (r *lanzadorRenderer) filaDe(it api.HubHit) *FilaReciente {
	p, ok := ProductoDe(it.Source)
	url := it.URL
	f := NuevaFilaReciente(it.Title, ui.DetalleHit(r.l.T.Locale(), it), func() {
		if r.l.AlAbrirURL != nil {
			r.l.AlAbrirURL(url)
		}
	})
	f.Conocido = ok
	if ok {
		f.Marca, f.Origen = p.Marca, p.NombreCorto()
	} else {
		f.Origen = it.Source
	}
	if it.Timestamp != nil {
		f.Cuando = i18n.FormatTimeAgo(r.l.T.Locale(), *it.Timestamp, r.l.Ahora)
	}
	return f
}

// ---- Disposición ----

// alto es lo que ocupa la portada entera a ese ancho de ventana. Es la ÚNICA cuenta:
// `Layout` recorre lo mismo y el guardián compara contra ella.
func (r *lanzadorRenderer) alto(ventana float32) float32 {
	if r.marco != tema.EsMarco() {
		r.rehacer()
	}
	_, ancho := CajaContenido(ventana)
	r.medirCabecerasTarjetas(ventana, ancho)
	if tema.EsMarco() {
		return r.altoMarco(ventana, ancho)
	}
	y := MargenY
	y += r.cabecera.alto(ancho) + MargenSeccion
	if r.atajosConexiones != nil {
		y += ui.AltoA(r.atajosConexiones, ancho) + MargenSeccion
	}
	y += tema.TextoXS.Linea + MargenRotulo + r.altoRejilla(ventana, ancho) + MargenSeccion
	if len(r.conectadas) > 0 {
		y += tema.TextoXS.Linea + MargenRotulo + r.altoConectadas(ventana, ancho) + MargenSeccion
	}

	rt := ui.RejillaTarjetas(ventana, HuecoTarjetas, cortesTarjetas)
	altoHoy := r.hoy.alto(r.altoCuerpoHoy())
	altoAcc := r.tAccesos.alto(r.altoCuerpoAccesos())
	if rt.Columnas(ancho) == 1 {
		y += altoHoy + HuecoTarjetas + altoAcc + HuecoTarjetas + r.recientes.alto(r.altoCuerpoReciente(ancho))
	} else {
		anchoRec := ancho - rt.Paso(ancho)
		y += max(altoHoy+HuecoTarjetas+altoAcc, r.recientes.alto(r.altoCuerpoReciente(anchoRec)))
	}
	return y + r.altoPie()
}

// medirCabecerasTarjetas da a SectionHeader el ancho que tendrá su tarjeta antes de
// preguntar el alto. Con ancho cero medía una fila, pero al pintar en el teléfono
// envolvía las acciones: las últimas filas quedaban fuera de la superficie de Card.
func (r *lanzadorRenderer) medirCabecerasTarjetas(ventana, ancho float32) {
	hoy, accesos, reciente := ancho, ancho, ancho
	if tema.EsMarco() {
		if r.l.dosColumnas() {
			principal, lado := columnasMarco(ancho)
			hoy, reciente = lado, lado
			if r.pocas() {
				reciente = principal
			}
		}
	} else {
		rejilla := ui.RejillaTarjetas(ventana, HuecoTarjetas, cortesTarjetas)
		if rejilla.Columnas(ancho) > 1 {
			hoy = rejilla.AnchoColumna(ancho)
			accesos, reciente = hoy, ancho-rejilla.Paso(ancho)
		}
	}
	for _, c := range []struct {
		tarjeta *tarjeta
		ancho   float32
	}{{r.hoy, hoy}, {r.tAccesos, accesos}, {r.recientes, reciente}} {
		if c.tarjeta != nil {
			c.tarjeta.cabecera.Ancho = max(c.ancho-2*tema.Borde, 0)
		}
	}
}

// altoRejilla es lo que ocupa la rejilla de teselas: el alto de cada fila es el de su
// tesela más alta, como hace una fila implícita de CSS grid.
func (r *lanzadorRenderer) altoRejilla(ventana, ancho float32) float32 {
	rj := r.rejillaApps(ventana)
	cols := rj.Columnas(ancho)
	anchoCol := rj.AnchoColumna(ancho)
	var alto float32
	for i := 0; i < len(r.teselas); i += cols {
		if i > 0 {
			alto += rj.Hueco
		}
		alto += r.altoFila(i, cols, anchoCol)
	}
	return alto
}

func (r *lanzadorRenderer) altoFila(desde, cols int, anchoCol float32) float32 {
	if tema.EsMarco() {
		return AltoTeselaCompacta // una línea de descripción: todas miden lo mismo
	}
	var alto float32
	for i := desde; i < len(r.teselas) && i < desde+cols; i++ {
		alto = max(alto, AltoTesela(LineasDe(r.teselas[i].Descripcion, anchoCol)))
	}
	return alto
}

// filasConectadas son los altos de las filas de la rejilla de aplicaciones conectadas:
// cada una lo que su tesela más alta, como las de producto.
func (r *lanzadorRenderer) filasConectadas(ventana, ancho float32) (cols int, col float32, filas []float32) {
	rj := r.rejillaApps(ventana)
	cols, col = rj.Columnas(ancho), rj.AnchoColumna(ancho)
	for i := 0; i < len(r.conectadas); i += cols {
		var alto float32
		for c := i; c < len(r.conectadas) && c < i+cols; c++ {
			alto = max(alto, AltoTeselaConectada(LineasDe(r.conectadas[c].App.Description, col)))
		}
		filas = append(filas, alto)
	}
	return cols, col, filas
}

func (r *lanzadorRenderer) altoConectadas(ventana, ancho float32) float32 {
	_, _, filas := r.filasConectadas(ventana, ancho)
	alto := float32(max(len(filas)-1, 0)) * r.rejillaApps(ventana).Hueco
	for _, f := range filas {
		alto += f
	}
	return alto
}

func (r *lanzadorRenderer) colocarConectadas(x, y, ventana, ancho float32) float32 {
	rj := r.rejillaApps(ventana)
	cols, col, filas := r.filasConectadas(ventana, ancho)
	for f, alto := range filas {
		for c := range cols {
			i := f*cols + c
			if i >= len(r.conectadas) {
				break
			}
			r.conectadas[i].Move(fyne.NewPos(x+rj.X(ancho, c), y))
			r.conectadas[i].Resize(fyne.NewSize(col, alto))
		}
		y += alto + rj.Hueco
	}
	return y - rj.Hueco
}

// altoCuerpoHoy es lo que mide el interior de la tarjeta «Hoy»: las filas de evento
// (con su filete entre ellas) más el pie de correo, o el párrafo del estado.
func (r *lanzadorRenderer) altoCuerpoHoy() float32 {
	var alto float32
	if len(r.esqueleto) > 0 {
		return float32(len(r.esqueleto))*tema.TextoRaiz.Tam +
			float32(len(r.esqueleto)-1)*HuecoEsqueleto
	}
	if r.textoHoy != nil {
		alto += tema.TextoSM.Linea + 2*AltoParrafoHoy // `px-4 py-3`
	}
	for i, f := range r.filasHoy {
		if i > 0 {
			alto += tema.Borde // `divide-y`
		}
		alto += f.MinSize().Height
	}
	return alto
}

// altoCuerpoAccesos son las ocho filas con su `gap-1`. Solo en Carril: en Marco los
// accesos no llevan tarjeta.
func (r *lanzadorRenderer) altoCuerpoAccesos() float32 {
	if len(r.accesos) == 0 {
		return 0
	}
	return float32(len(r.accesos))*MedidaAcceso() + float32(len(r.accesos)-1)*SepAccesos
}

// altoCuerpoReciente es el interior de la tarjeta de actividad.
func (r *lanzadorRenderer) altoCuerpoReciente(ancho float32) float32 {
	switch {
	case len(r.esqRec) > 0:
		filas := len(r.esqRec) / 4
		return float32(filas)*MedidaReciente(true) + float32(filas-1)*tema.Borde
	case r.avisoRec != nil:
		return r.avisoRec.MedidaVacio(AnchoInterior(ancho))
	}
	var alto float32
	for i, f := range r.filas {
		if i > 0 {
			alto += tema.Borde
		}
		alto += f.MinSize().Height
	}
	return alto
}

// Layout coloca la portada. Recorre lo mismo que `alto`, en el mismo orden.
func (r *lanzadorRenderer) Layout(t fyne.Size) {
	if t.Width <= 0 {
		return
	}
	r.fondo.Resize(fyne.NewSize(t.Width, max(t.Height, r.alto(t.Width))))
	x, ancho := CajaContenido(t.Width)
	r.medirCabecerasTarjetas(t.Width, ancho)
	if tema.EsMarco() {
		r.colocarPie(x, r.colocarMarco(x, t.Width, ancho), ancho)
		return
	}
	y := r.cabecera.colocar(x) + MargenSeccion
	if r.atajosConexiones != nil {
		h := ui.AltoA(r.atajosConexiones, ancho)
		r.atajosConexiones.Move(fyne.NewPos(x, y))
		r.atajosConexiones.Resize(fyne.NewSize(ancho, h))
		y += h + MargenSeccion
	}
	r.rotulo.Colocar(x, y)
	y += tema.TextoXS.Linea + MargenRotulo
	y = r.colocarRejilla(x, y, t.Width, ancho) + MargenSeccion
	if len(r.conectadas) > 0 {
		r.rotuloCon.Colocar(x, y)
		y += tema.TextoXS.Linea + MargenRotulo
		y = r.colocarConectadas(x, y, t.Width, ancho) + MargenSeccion
	}

	rt := ui.RejillaTarjetas(t.Width, HuecoTarjetas, cortesTarjetas)
	altoHoy, altoAcc := r.altoCuerpoHoy(), r.altoCuerpoAccesos()
	if rt.Columnas(ancho) == 1 {
		r.hoy.colocar(x, y, ancho, altoHoy)
		r.colocarHoy(x, y, ancho)
		y += r.hoy.alto(altoHoy) + HuecoTarjetas
		r.colocarAccesos(x, y, ancho, altoAcc)
		y += r.tAccesos.alto(altoAcc) + HuecoTarjetas
		y = r.colocarReciente(x, y, ancho)
	} else {
		col := rt.AnchoColumna(ancho)
		r.hoy.colocar(x, y, col, altoHoy)
		r.colocarHoy(x, y, col)
		yAcc := y + r.hoy.alto(altoHoy) + HuecoTarjetas
		r.colocarAccesos(x, yAcc, col, altoAcc)
		fin := r.colocarReciente(x+rt.Paso(ancho), y, ancho-rt.Paso(ancho))
		y = max(yAcc+r.tAccesos.alto(altoAcc), fin)
	}

	r.colocarPie(x, y, ancho)
}

// colocarPie pone el aviso de privacidad, centrado, bajo el cuerpo que acaba en `y`.
func (r *lanzadorRenderer) colocarPie(x, y, ancho float32) {
	y += margenPagina() + MargenAviso
	for _, p := range r.privacidad {
		w := ui.Medir(p.Text, tema.TextoXS, ui.Normal).Width
		ui.ColocarTexto(p, tema.TextoXS, x+(ancho-w)/2, y)
		y += tema.TextoXS.Linea
	}
}

// altoPie es lo que ocupa ese aviso con sus márgenes.
func (r *lanzadorRenderer) altoPie() float32 {
	return margenPagina() + MargenAviso + float32(len(r.privacidad))*tema.TextoXS.Linea + MargenY
}

// margenPagina es el `py-8` de la página, que en Marco es `marco:py-5`. El `pb-8` del
// aviso de privacidad, que va fuera de ella, no cambia.
func margenPagina() float32 {
	if tema.EsMarco() {
		return MargenYMarco
	}
	return MargenY
}

// ---- La disposición de Marco ----

// pocas dice si «Reciente» baja a la columna de las apps: con seis o menos —dos filas de
// tres— y solo en dos columnas (`pocas && 'marco:lg:…'`). Al lado queda «Hoy».
func (r *lanzadorRenderer) pocas() bool {
	return r.l.dosColumnas() && len(r.teselas) <= PocasApps
}

// altoCabeceraMarco es el saludo con sus píldoras, del borde de la página al cuerpo.
func (r *lanzadorRenderer) altoCabeceraMarco(ancho float32) float32 {
	alto := MargenYMarco + r.cabecera.alto(ancho)
	if r.tiraPildoras != nil {
		alto += MargenPildoras + r.altoPildoras
	}
	if r.atajosConexiones != nil {
		alto += MargenPildoras + ui.AltoA(r.atajosConexiones, ancho)
	}
	return alto + MargenCabeceraMarco
}

// colocarCabeceraMarco lo coloca y devuelve dónde empieza el cuerpo.
func (r *lanzadorRenderer) colocarCabeceraMarco(x, ancho float32) float32 {
	y := r.cabecera.colocarEn(x, MargenYMarco)
	if r.tiraPildoras != nil {
		y += MargenPildoras
		r.tiraPildoras.Move(fyne.NewPos(x, y))
		r.tiraPildoras.Resize(fyne.NewSize(ancho, r.altoPildoras))
		y += r.altoPildoras
	}
	if r.atajosConexiones != nil {
		y += MargenPildoras
		h := ui.AltoA(r.atajosConexiones, ancho)
		r.atajosConexiones.Move(fyne.NewPos(x, y))
		r.atajosConexiones.Resize(fyne.NewSize(ancho, h))
		y += h
	}
	return y + MargenCabeceraMarco
}

// altoPrincipalMarco es lo que ocupa la columna de las apps a ese ancho: cada sección con
// su rótulo y `mb-8` entre ellas. La última lo pierde en dos columnas
// (`marco:lg:last:mb-0`) y lo conserva en una, donde separa las apps de «Hoy».
func (r *lanzadorRenderer) altoPrincipalMarco(ventana, ancho float32) float32 {
	alto := tema.TextoXS.Linea + MargenRotulo + r.altoRejilla(ventana, ancho)
	if len(r.conectadas) > 0 {
		alto += MargenSeccion + tema.TextoXS.Linea + MargenRotulo + r.altoConectadas(ventana, ancho)
	}
	if !r.l.dosColumnas() {
		alto += MargenSeccion
	}
	return alto
}

// altoMarco es `alto` en la forma Marco. `colocarMarco` recorre lo mismo.
//
// ponytail: con pocas apps «Hoy» abarca las dos filas de la rejilla (`row-span-2`), y si
// midiera más que las apps y «Reciente» juntas el navegador repartiría lo que sobra
// entre las dos filas, bajando «Reciente». Aquí «Reciente» se queda a 16 de las apps:
// hacen falta más de ocho eventos en el día para que se note.
func (r *lanzadorRenderer) altoMarco(ventana, ancho float32) float32 {
	y := r.altoCabeceraMarco(ancho)
	hoy := r.hoy.alto(r.altoCuerpoHoy())
	rec := func(a float32) float32 { return r.recientes.alto(r.altoCuerpoReciente(a)) }
	principal, lado := columnasMarco(ancho)
	switch {
	case r.pocas():
		y += max(r.altoPrincipalMarco(ventana, principal)+HuecoColumnasMarco+rec(principal), hoy)
	case r.l.dosColumnas():
		y += max(r.altoPrincipalMarco(ventana, principal), hoy+HuecoTarjetasMarco+rec(lado))
	default:
		y += r.altoPrincipalMarco(ventana, ancho) + hoy + HuecoTarjetasMarco + rec(ancho)
	}
	return y + r.altoPie()
}

// colocarMarco coloca la portada de Marco y devuelve dónde acaba su cuerpo: el saludo con
// sus píldoras y, debajo, las apps con «Hoy» y «Reciente» a su derecha desde 1024 —o
// «Reciente» bajo las apps si son pocas—, y en una columna por debajo.
func (r *lanzadorRenderer) colocarMarco(x, ventana, ancho float32) float32 {
	y := r.colocarCabeceraMarco(x, ancho)
	interior := r.altoCuerpoHoy()
	hoy := func(hx, hy, hw float32) float32 {
		r.hoy.colocar(hx, hy, hw, interior)
		r.colocarHoy(hx, hy, hw)
		return hy + r.hoy.alto(interior)
	}
	if !r.l.dosColumnas() {
		y = hoy(x, r.colocarPrincipalMarco(x, y, ventana, ancho), ancho)
		return r.colocarReciente(x, y+HuecoTarjetasMarco, ancho)
	}
	principal, lado := columnasMarco(ancho)
	xLado := x + principal + HuecoColumnasMarco
	fin := r.colocarPrincipalMarco(x, y, ventana, principal)
	finHoy := hoy(xLado, y, lado)
	if r.pocas() {
		return max(r.colocarReciente(x, fin+HuecoColumnasMarco, principal), finHoy)
	}
	return max(fin, r.colocarReciente(xLado, finHoy+HuecoTarjetasMarco, lado))
}

// colocarPrincipalMarco pone las apps y, si las hay, las conectadas; devuelve dónde
// acaba la columna, con el `mb-8` de la última sección solo en una columna.
func (r *lanzadorRenderer) colocarPrincipalMarco(x, y, ventana, ancho float32) float32 {
	r.rotulo.Colocar(x, y)
	y += tema.TextoXS.Linea + MargenRotulo
	y = r.colocarRejilla(x, y, ventana, ancho)
	if len(r.conectadas) > 0 {
		y += MargenSeccion
		r.rotuloCon.Colocar(x, y)
		y += tema.TextoXS.Linea + MargenRotulo
		y = r.colocarConectadas(x, y, ventana, ancho)
	}
	if !r.l.dosColumnas() {
		y += MargenSeccion
	}
	return y
}

func (r *lanzadorRenderer) colocarRejilla(x, y, ventana, ancho float32) float32 {
	rj := r.rejillaApps(ventana)
	cols := rj.Columnas(ancho)
	col := rj.AnchoColumna(ancho)
	for i := 0; i < len(r.teselas); i += cols {
		alto := r.altoFila(i, cols, col)
		for c := 0; c < cols && i+c < len(r.teselas); c++ {
			o := r.teselas[i+c]
			o.Move(fyne.NewPos(x+rj.X(ancho, c), y))
			o.Resize(fyne.NewSize(col, alto))
		}
		y += alto + rj.Hueco
	}
	if len(r.teselas) > 0 {
		y -= rj.Hueco
	}
	return y
}

// colocarHoy pone el interior de la tarjeta «Hoy».
func (r *lanzadorRenderer) colocarHoy(x, y, ancho float32) {
	ix := x + tema.Borde + MargenTarjeta
	iy := y + tema.Borde + r.hoy.altoCabecera() + MargenTarjeta
	iw := AnchoInterior(ancho)
	for _, e := range r.esqueleto {
		e.Move(fyne.NewPos(ix, iy))
		e.Resize(fyne.NewSize(iw*0.7, tema.TextoRaiz.Tam))
		iy += tema.TextoRaiz.Tam + HuecoEsqueleto
	}
	if len(r.esqueleto) == 2 {
		r.esqueleto[1].Resize(fyne.NewSize(iw*0.55, tema.TextoRaiz.Tam))
	}
	if r.textoHoy != nil {
		ui.ColocarTexto(r.textoHoy, tema.TextoSM, ix+MargenTarjeta, iy+AltoParrafoHoy)
		iy += tema.TextoSM.Linea + 2*AltoParrafoHoy
	}
	for i, f := range r.filasHoy {
		if i > 0 {
			if i-1 < len(r.filetesHoy) {
				ColocarDivisor(r.filetesHoy[i-1], ix, iy, iw)
			}
			iy += tema.Borde
		}
		h := f.MinSize().Height
		f.Move(fyne.NewPos(ix, iy))
		f.Resize(fyne.NewSize(iw, h))
		iy += h
	}
}

func (r *lanzadorRenderer) colocarAccesos(x, y, ancho, interior float32) {
	r.tAccesos.colocar(x, y, ancho, interior)
	ix := x + tema.Borde + MargenTarjeta
	iy := y + tema.Borde + r.tAccesos.altoCabecera() + MargenTarjeta
	iw := AnchoInterior(ancho)
	for _, a := range r.accesos {
		a.Move(fyne.NewPos(ix, iy))
		a.Resize(fyne.NewSize(iw, MedidaAcceso()))
		iy += MedidaAcceso() + SepAccesos
	}
}

func (r *lanzadorRenderer) colocarReciente(x, y, ancho float32) float32 {
	interior := r.altoCuerpoReciente(ancho)
	r.recientes.colocar(x, y, ancho, interior)
	ix := x + tema.Borde + MargenTarjeta
	iy := y + tema.Borde + r.recientes.altoCabecera() + MargenTarjeta
	iw := AnchoInterior(ancho)
	if r.avisoRec != nil {
		r.avisoRec.Move(fyne.NewPos(ix, iy))
		r.avisoRec.Resize(fyne.NewSize(iw, interior))
	}
	for i := 0; i+3 < len(r.esqRec); i += 4 {
		fy := iy + float32(i/4)*(MedidaReciente(true)+tema.Borde)
		r.esqRec[i].Move(fyne.NewPos(ix+MargenReciente, fy+altoReciente()+2))
		r.esqRec[i].Resize(fyne.NewSize(10, 10))
		cx := ix + MargenReciente + 10 + HuecoReciente
		r.esqRec[i+1].Move(fyne.NewPos(cx, fy+altoReciente()))
		r.esqRec[i+2].Move(fyne.NewPos(cx, fy+altoReciente()+tema.TextoRaiz.Linea))
		r.esqRec[i+3].Move(fyne.NewPos(ix+iw-MargenReciente-3*tema.Rem, fy+altoReciente()))
	}
	for i, f := range r.filas {
		if i > 0 {
			ColocarDivisor(r.filetesRec[i-1], ix, iy, iw)
			iy += tema.Borde
		}
		h := f.MinSize().Height
		f.Move(fyne.NewPos(ix, iy))
		f.Resize(fyne.NewSize(iw, h))
		iy += h
	}
	return y + r.recientes.alto(interior)
}
