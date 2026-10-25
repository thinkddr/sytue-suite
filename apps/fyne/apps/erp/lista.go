// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"errors"
	"image/color"
	"math"
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
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La lista de un modelo (`Lista` de ErpApp.tsx)
//
// UNA lista para los treinta y tres módulos: las columnas son `modelo.lista` y los
// rótulos los trae el catálogo ya traducidos. Aquí no hay el nombre de ningún módulo.
//
// Medido en el arnés del frontal (ErpApp con el servicio de mentira, 1280×800, rem 14,
// 2026-09-23) con cuatro facturas; y lo que no tiene filas, en producción el 22-09:
//
//	main            x 260 → 1280                                   (medido)
//	contenido       x 343, w 854  (`max-w-5xl` = 896, `px-6`)       (medido)
//	cabecera        FILA QUE SALTA: título arriba, barra debajo si no caben (medido)
//	buscador        w 154 (`w-44`), h 28,25                         (medido)
//	vista           grupo 135,8×29,5 (Tabla 59,03, Tarjetas 71,27)   (medido)
//	píldora estado  h 24 (`min-h-[24px]`), `px-3 py-1`, `text-xs`   (medido)
//	aviso rojo      `mx-6 mt-4`, `px-4 py-3`                        (medido)
//	fila de tabla   42,5 con su filete (ver `tabla.go`)             (medido)

// Medidas del cuerpo de la lista.
const (
	// AnchoContenidoLista es `max-w-5xl` con el rem de 14. Medido con su `px-6`: 854.
	AnchoContenidoLista float32 = 64 * tema.Rem
	// MargenLista es el `px-6`/`py-6` del contenido.
	MargenLista float32 = 6 * 0.25 * tema.Rem
	// SeparaBloques es el `space-y-4` entre cabecera, filtros, cuerpo y pie.
	SeparaBloques float32 = 4 * 0.25 * tema.Rem
	// AnchoBuscador es el `w-44` del campo de búsqueda. Medido: 154.
	AnchoBuscador float32 = 11 * tema.Rem
	// PildoraPX y PildoraPY son el `px-3 py-1` de una píldora de filtro.
	PildoraPX float32 = 3 * 0.25 * tema.Rem
	PildoraPY float32 = 1 * 0.25 * tema.Rem
	// AvisoPX y AvisoPY son el `px-4 py-3` de la banda roja de error, y AvisoMT su
	// `mt-4` sobre `main`.
	AvisoPX float32 = 4 * 0.25 * tema.Rem
	AvisoPY float32 = 3 * 0.25 * tema.Rem
	AvisoMT float32 = 4 * 0.25 * tema.Rem
	// IlustracionVacio es el `size={32}` del icono del estado vacío.
	IlustracionVacio float32 = 32
	// HuecoCabeceraFila es el `gap-3` de la cabecera, entre el título y la barra (en
	// las dos direcciones), y HuecoCabecera el `gap-2` de dentro de cada uno.
	HuecoCabeceraFila float32 = 3 * 0.25 * tema.Rem
	HuecoCabecera     float32 = 2 * 0.25 * tema.Rem
	// PieCabecera es el `pb-3` de la cabecera, antes de su filete.
	PieCabecera float32 = 3 * 0.25 * tema.Rem
)

// AltoPildora es `py-1` sobre la caja de `text-xs` —21, medido— con el suelo de
// `min-h-[24px]` que la web le puso en el PR #841: 24.
var AltoPildora = max(tema.TextoXS.Linea+2*PildoraPY, ui.LadoIconoSM)

// AltoBuscador es 28,25: `py-1.5` sobre la caja de `text-sm`, con el borde por dentro.
// Medido.
var AltoBuscador = CampoBuscadorLista.Alto()

// AltoCabeceraLista es la cabecera en UNA fila: la pieza más alta (el grupo de vista,
// 29,5), el `pb-3` y el filete. Era 39,75 en producción el 22-09, con los botones de
// vista a 21 de alto antes del PR #841.
var AltoCabeceraLista = AltoGrupoVista + PieCabecera + tema.Borde

// AltoBandaAviso es 36,75: `py-3` sobre la caja de `text-sm`.
var AltoBandaAviso = tema.TextoSM.Linea + 2*AvisoPY

// Lista es la pantalla de un modelo. Es ESTADO, no vista: quien la usa rellena sus
// campos y pide el objeto.
type Lista struct {
	tr  *i18n.Translator
	loc i18n.Locale

	// Modelo es el que se está mirando. Sin él no hay lista que pintar.
	Modelo Modelo
	// Pagina es lo que contestó el servicio.
	Pagina Pagina
	// Fase es en cuál de los seis estados está.
	Fase Fase
	// Aviso es lo que dijo el servicio cuando falló. Se pinta en la banda roja de
	// arriba, que en la web convive con el cuerpo en vez de sustituirlo.
	Aviso string

	// Busqueda, Estado, Papelera y Orden son los filtros de ahora.
	Busqueda string
	Estado   string
	Papelera bool
	Orden    Orden

	// SoloLectura es no tener escritura en el módulo: sin «Nuevo» ni acciones.
	SoloLectura bool
	// HayMas dice si el servicio dejó un cursor: entonces se ofrece «Ver más».
	HayMas bool
	// CargandoMas es el `loading` de «Ver más» mientras llega la página siguiente.
	CargandoMas bool
	// Total es el recuento del servidor (`/estadisticas`). Negativo: no se sabe, y
	// entonces se cuenta lo que hay a la vista, como hace la web.
	Total int
	// SumaDinero es el acumulado del servidor, en céntimos. Cero: no se enseña.
	SumaDinero int64
	// PorEstado es el recuento por estado, para los números de las píldoras.
	PorEstado map[string]int
	// Ventana es el ancho de la ventana, que decide el reparto del contenido.
	Ventana float32
	// Ocupado es lo que se lleva de ancho la ficha abierta a la derecha.
	Ocupado float32
	// Marcadas son las filas elegidas para las acciones en lote, por id.
	Marcadas map[string]bool
	// Tarjetas es la vista de tarjetas en lugar de la tabla (`vistaModo`).
	Tarjetas bool
	// EnLote es el `ejecutandoLote`: una acción en lote en curso.
	EnLote bool

	AlBuscar     func(string)
	AlEstado     func(string)
	AlPapelera   func(bool)
	AlOrdenar    func(Orden)
	AlAbrir      func(Registro)
	AlMas        func()
	AlReintentar func()
	// AlLimpiar es «Restablecer vista»: quita el estado, la búsqueda y la papelera.
	AlLimpiar func()
	// AlImportar abre el selector de ficheros para importar un CSV (o facturas); nil, no
	// hay selector del sistema y no se pinta. Importando es la carga en curso.
	AlImportar func()
	Importando bool
	// AlNueva abre la ficha en blanco; AlDuplicar, la copia de una fila.
	AlNueva    func()
	AlDuplicar func(Registro)
	// AlMarcar alterna una fila; AlMarcarTodas, todas o ninguna; AlDesmarcar vacía.
	AlMarcar      func(id string)
	AlMarcarTodas func()
	AlDesmarcar   func()
	// AlLote aplica «estado» (con su estado) o «borrar» a las marcadas.
	AlLote func(accion, estado string)
	// AlVista cambia entre tabla y tarjetas.
	AlVista func(tarjetas bool)
	// AlExportar baja el CSV: de la lista filtrada, o de `ids` si los hay.
	AlExportar func(ids []string)

	// buscador es el MISMO campo de una pintada a otra. La pantalla rehace el árbol
	// cada vez que cambia algo, y un campo nuevo en cada repintado se llevaba el foco
	// a la segunda letra: se escribía «a», llegaba la lista, y la «c» ya no entraba.
	buscador *ui.Entrada
}

// Orden es por qué columna se ordena y en qué sentido. El cero —clave vacía— es el
// orden por defecto del servicio (fecha de creación), que es el único que va por
// índice: medido en el servicio, ordenar por un campo del jsonb cuesta 20 ms contra
// 0,6 ms con 20.000 filas.
type Orden struct {
	Campo string
	Asc   bool
}

// NuevaLista crea la pantalla en su estado inicial: cargando.
func NuevaLista(tr *i18n.Translator) *Lista {
	l := &Lista{tr: tr, loc: tr.Locale(), Total: -1, Marcadas: map[string]bool{}}
	l.buscador = ui.NuevaEntrada(l.t("list.searchPlaceholder")).ConPropia(CampoBuscadorLista)
	l.buscador.Ancho = AnchoBuscador
	l.buscador.AlCambiar(func(q string) {
		if l.AlBuscar != nil {
			l.AlBuscar(q)
		}
	})
	return l
}

// clave identifica una lista por lo que la acota: modelo, búsqueda, estado, orden y
// papelera. Es la clave de la caché y la que dice si una respuesta que llega tarde
// sigue siendo la de lo que se está mirando. El modelo va el primero y separado por
// «|», que no puede aparecer en una clave del catálogo (`claveDeModelo`).
func (l *Lista) clave() string {
	orden := ""
	if l.Orden.Campo != "" {
		orden = l.Orden.Campo + ":" + strconv.FormatBool(l.Orden.Asc)
	}
	papelera := ""
	if l.Papelera {
		papelera = "p"
	}
	return strings.Join([]string{l.Modelo.Clave, l.Busqueda, l.Estado, orden, papelera}, "|")
}

// claveDeModelo es el modelo de una clave de `Lista.clave`.
func claveDeModelo(clave string) string {
	m, _, _ := strings.Cut(clave, "|")
	return m
}

// limpiarBuscador vacía el campo sin avisar: quien lo llama ya ha puesto `Busqueda` a
// cero, y el aviso del campo lo ignora por igual.
func (l *Lista) limpiarBuscador() {
	if l.buscador.Texto() != "" {
		l.buscador.PonerTexto("")
	}
}

// enfocarBuscador es la tecla «/» de la web: lleva el foco al buscador de la lista.
func (l *Lista) enfocarBuscador(c fyne.Canvas) { c.Focus(l.buscador) }

// filtrada dice si hay algo que «Restablecer vista» pueda quitar.
func (l *Lista) filtrada() bool { return l.Estado != "" || l.Busqueda != "" || l.Papelera }

// importe es el de la insignia azul de la cabecera: el acumulado del servidor si lo
// dio, y si no la suma de lo que hay a la vista en la primera columna de dinero de la
// lista, que es lo que hace la web. Sin columna de dinero o sin filas, no hay insignia.
func (l *Lista) importe() (int64, bool) {
	if l.SumaDinero > 0 {
		return l.SumaDinero, true
	}
	rs := l.registros()
	if len(rs) == 0 {
		return 0, false
	}
	for _, c := range l.Modelo.Campos {
		if c.Tipo != api.ERPDinero || !slices.Contains(l.Modelo.Lista, c.Clave) {
			continue
		}
		var suma int64
		for _, r := range rs {
			// Solo lo que es número: `typeof val === 'number'` en la web.
			if v, ok := r.Data[c.Clave].(float64); ok {
				suma += int64(math.Round(v))
			}
		}
		return suma, true
	}
	return 0, false
}

func (l *Lista) t(clave string, vars ...i18n.Vars) string {
	var v i18n.Vars
	if len(vars) > 0 {
		v = vars[0]
	}
	return l.tr.T(clave, v)
}

// registros es lo que hay a la vista.
func (l *Lista) registros() []Registro { return l.Pagina.Registros }

// cuenta es el número de la insignia de la cabecera: el del servidor si lo hay, y si no
// lo que se ve. Es lo que hace la web, y la diferencia importa: «50 registros cargados»
// con 4.000 en la base es una frase distinta de «4.000».
func (l *Lista) cuenta() int {
	if l.Total >= 0 {
		return l.Total
	}
	return len(l.registros())
}

// Siguiente es el orden que toca al pulsar la cabecera de `campo`: primero descendente,
// luego ascendente y luego ninguno, que es el ciclo de la web.
func (o Orden) Siguiente(campo string) Orden {
	switch {
	case o.Campo != campo:
		return Orden{Campo: campo}
	case !o.Asc:
		return Orden{Campo: campo, Asc: true}
	}
	return Orden{}
}

// tonosEstado traduce el tono que nombra `Tono` —que es `tonoDeEstado` de la web— al
// de la insignia del catálogo. Los nombres son los de la web y no se inventan aquí.
var tonosEstado = map[string]ui.Tono{
	"success": ui.InsigniaExito,
	"info":    ui.InsigniaInfo,
	"danger":  ui.InsigniaPeligro,
}

// esMovil es la ventana por debajo de `md` (768): la tabla se esconde y la lista va
// en filas de resumen (`max-md:hidden` / `md:hidden`).
func (l *Lista) esMovil() bool { return l.Ventana > 0 && l.Ventana < ui.Estrecho }

// haySM es la ventana desde `sm` (640), donde aparecen el grupo de vista y el rótulo
// de «Exportar CSV». Los cortes miran la VENTANA, no esta caja.
func (l *Lista) haySM() bool { return l.Ventana == 0 || l.Ventana >= ui.CorteSM }

// anchoMain es lo que mide `main`: la ventana menos el carril de iconos y la separación
// de la hoja (que en móvil no hay) y menos la ficha abierta.
func (l *Lista) anchoMain() float32 {
	if l.Ventana == 0 {
		return AnchoContenidoLista
	}
	m := l.Ventana - l.Ocupado - ocupaCarcasa(l.Ventana)
	return max(m, 0)
}

// anchoUtil es lo que mide el contenido acotado: el `max-w-5xl` centrado en `main`,
// menos su `px-6`.
func (l *Lista) anchoUtil() float32 {
	return max(min(l.anchoMain(), AnchoContenidoLista)-2*MargenLista, 0)
}

// Objeto arma la pantalla entera —la banda de aviso, la cabecera, la barra de lote,
// los filtros, el cuerpo y el pie— dentro del `max-w-5xl` centrado, con el alto que
// ocupa de verdad para que la desplace quien la monte.
func (l *Lista) Objeto(v tema.Variante) fyne.CanvasObject {
	ancho := l.anchoUtil()
	p := &pagina{}
	hijos := []fyne.CanvasObject{canvas.NewRectangle(color.NRGBA{})}
	if a := l.bandaAviso(v); a != nil {
		p.banda = a
		hijos = append(hijos, a)
	}
	cab, altoCab := l.cabecera(v, ancho)
	p.bloques = append(p.bloques, bloque{cab, altoCab, 0})
	if lote, alto := l.barraLote(v, ancho); lote != nil {
		p.bloques = append(p.bloques, bloque{lote, alto, 0})
	}
	fil, altoFil := l.filtros(v, ancho)
	p.bloques = append(p.bloques, bloque{fil, altoFil, 0})
	cuerpo, altoCuerpo := l.cuerpo(v, ancho)
	p.bloques = append(p.bloques, bloque{cuerpo, altoCuerpo, 0})
	if pie, alto := l.pie(v); pie != nil {
		p.bloques = append(p.bloques, bloque{pie, alto, 0})
	}
	for _, b := range p.bloques {
		hijos = append(hijos, b.obj)
	}
	return container.New(p, hijos...)
}

// bandaAviso es el `<p class="bg-danger-bg text-danger-text mx-6 mt-4">` de arriba de
// `main`.
//
// Va SOBRE el cuerpo y no en su lugar: es lo que hace la web, y es correcto —con la
// lista ya pintada, un fallo al refrescar no debería borrar lo que sigue siendo cierto
// hasta hace un momento—. Con nil, no hay nada que avisar.
func (l *Lista) bandaAviso(v tema.Variante) fyne.CanvasObject {
	texto := l.Aviso
	if texto == "" {
		if l.Fase == Prohibida {
			// Sin banda: el estado vacío de debajo ya lo dice, y la banda repetía el
			// respaldo en inglés del servicio (web, PR #840).
			return nil
		}
		texto = textoDeFase(l.Fase, l.tr)
	}
	if texto == "" {
		return nil
	}
	b := &banda{
		caja:     ui.NuevaCaja(tema.Color("danger-bg", v), color.NRGBA{}, tema.RadioMD),
		texto:    ui.Texto(texto, tema.TextoSM, ui.Normal, tema.Color("danger-text", v)),
		completo: texto,
	}
	return container.New(b, append(b.caja.Objetos(), b.texto)...)
}

// FaseDeLista es la fase de una lista con el 403 de la web (`esSinPermiso`): solo el
// código `sin_permiso` es el candado de «No tienes acceso»; otro 403 —el rol, la
// organización personal— es un fallo, y la banda dice lo que contestó el servicio.
// Devuelve la fase y el texto de la banda, vacío si no lo lleva.
func FaseDeLista(err error, hayRed bool, n int, tr *i18n.Translator) (Fase, string) {
	f := FaseDe(err, hayRed, n)
	var e *api.Error
	if f == Prohibida && errors.As(err, &e) && e.Code() != "sin_permiso" {
		return Fallo, MensajeDeError(err, tr)
	}
	return f, ""
}

// textoDeFase es lo que dice la banda roja en cada fase de fallo; vacío en las demás.
func textoDeFase(f Fase, tr *i18n.Translator) string {
	switch f {
	case Prohibida:
		return tr.T("error.sinPermiso", nil)
	case SinConexion:
		return tr.T("error.offline", nil)
	// `SinSesion` va con `Fallo`: la web del ERP no distingue el 401 (`erp.Fase`).
	case Fallo, SinSesion:
		return tr.T("error.generic", nil)
	case Cargando, Exito, Vacia:
	}
	return ""
}

// --- La cabecera ------------------------------------------------------------

// cabecera es el `<header>`: a la izquierda el título con sus insignias y a la derecha
// la barra —buscador, vista, exportar y «Nuevo»—. Es UNA FILA QUE SALTA (`flex-wrap`):
// si no caben juntos, la barra baja a la línea siguiente y, dentro de cada uno, lo que
// no cabe también. Medido a 1280 sin ficha: la barra ya baja (título en y 183,25,
// buscador en 217,13) porque el título con sus dos insignias y la barra suman 1.069.
func (l *Lista) cabecera(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	c := &cabeceraLista{filete: canvas.NewRectangle(ui.BordeSobre("border", "bg", v))}
	hijos := []fyne.CanvasObject{c.filete}

	h4, peso := ui.EscalonTitulo(4), ui.PesoTitulo(4) // `sytue-h4`: el título es de la forma
	titulo := ui.Texto(l.Modelo.Etiqueta, h4, peso, tema.Color("text", v))
	c.izq = append(c.izq, pieza{titulo, ui.Medir(l.Modelo.Etiqueta, h4, peso).Width, h4.Linea, true})
	hijos = append(hijos, titulo)
	cuenta := l.t("list.recordsLoaded", i18n.Vars{"count": l.cuenta()})
	c.izq = append(c.izq, piezaDe(ui.NuevaInsignia(cuenta, ui.InsigniaNeutra), ui.MedidaInsignia(cuenta)))
	hijos = append(hijos, c.izq[len(c.izq)-1].obj)
	if suma, hay := l.importe(); hay {
		t := l.t("list.summaryTotal", i18n.Vars{"amount": i18n.FormatCurrency(l.loc, suma)})
		c.izq = append(c.izq, piezaDe(ui.NuevaInsignia(t, ui.InsigniaInfo), ui.MedidaInsignia(t)))
		hijos = append(hijos, c.izq[len(c.izq)-1].obj)
	}

	c.der = append(c.der, pieza{l.buscador, AnchoBuscador, AltoBuscador, false})
	hijos = append(hijos, l.buscador)
	if l.haySM() {
		g := NuevoGrupoVista(l.t("list.viewOptions"), l.t("list.viewTable"), l.t("list.viewCards"), l.Tarjetas,
			func(tarjetas bool) {
				if l.AlVista != nil {
					l.AlVista(tarjetas)
				}
			})
		c.der = append(c.der, piezaDe(g, g.MinSize()))
		hijos = append(hijos, g)
	}
	// «Exportar CSV» baja el fichero a la carpeta de descargas y lo abre, que es lo que
	// hace el `href` de la web. En el móvil, solo el icono.
	exportar := ui.NuevoBoton(l.textoSM("list.exportCsv"), l.exportar(nil)).Con(ui.Secundario, ui.TallaSM)
	exportar.ConIcono(iconos.Descargar, 14)
	exportar.MargenIcono = 1 * 0.25 * tema.Rem
	exportar.Rotulo = l.t("list.exportCsv")
	exportar.Inerte = l.AlExportar == nil
	c.der = append(c.der, piezaDe(exportar, exportar.MinSize()))
	hijos = append(hijos, exportar)
	// «Importar CSV» abre el selector de ficheros DEL SISTEMA. Donde no lo hay (el móvil,
	// un Linux sin zenity ni kdialog) no se pinta: un botón que no hace nada miente más
	// que uno que no está.
	if !l.SoloLectura && l.AlImportar != nil {
		clave := "list.import"
		if l.Modelo.Recibe {
			clave = "list.importInvoices"
		}
		imp := ui.NuevoBoton(l.textoSM(clave), l.AlImportar).Con(ui.Secundario, ui.TallaSM)
		imp.ConIcono(iconos.Subir, 14)
		imp.MargenIcono = 1 * 0.25 * tema.Rem
		imp.Rotulo, imp.Cargando = l.t(clave), l.Importando
		c.der = append(c.der, piezaDe(imp, imp.MinSize()))
		hijos = append(hijos, imp)
	}
	if !l.SoloLectura && l.AlNueva != nil {
		nuevo := ui.NuevoBoton(l.t("action.new"), l.AlNueva).Con(ui.Primario, ui.TallaSM)
		nuevo.ConIcono(iconos.Mas, 14)
		nuevo.MargenIcono = 1 * 0.25 * tema.Rem
		c.der = append(c.der, piezaDe(nuevo, nuevo.MinSize()))
		hijos = append(hijos, nuevo)
	}
	c.colocar(ancho)
	return container.New(c, hijos...), c.alto
}

func (l *Lista) exportar(ids []string) func() {
	return func() {
		if l.AlExportar != nil {
			l.AlExportar(ids)
		}
	}
}

// textoSM es un rótulo que en móvil desaparece (`hidden sm:inline`): el botón se queda
// en su icono.
func (l *Lista) textoSM(clave string) string {
	if !l.haySM() {
		return ""
	}
	return l.t(clave)
}

// --- La barra de acciones en lote -------------------------------------------

// Medidas de la barra de lote: `rounded-lg border px-4 py-2.5 shadow-sm`, con un botón
// `xs` (24) dentro. Medido: 43,5 de alto.
const (
	LotePX   float32 = 4 * 0.25 * tema.Rem
	LotePY   float32 = 2.5 * 0.25 * tema.Rem
	AltoLote float32 = ui.LadoIconoSM + 2*LotePY + 2*tema.Borde
)

// barraLote es la barra de «N seleccionados» que sale al marcar filas: deseleccionar,
// cambiar el estado de todas, exportarlas y mandarlas a la papelera.
func (l *Lista) barraLote(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	n := 0
	for _, r := range l.registros() {
		if l.Marcadas[r.ID] {
			n++
		}
	}
	if n == 0 {
		return nil, 0
	}
	b := &barraLote{
		caja:   ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), radioTarjeta()),
		sombra: ui.NuevaSombra(tema.SombraSM, radioTarjeta()), v: v,
	}
	hijos := []fyne.CanvasObject{b.sombra.Objeto()}
	hijos = append(hijos, b.caja.Objetos()...)
	cuenta := l.t("list.selectedCount", i18n.Vars{"n": strconv.Itoa(n)})
	b.izq = append(b.izq, piezaDe(ui.NuevaInsignia(cuenta, ui.InsigniaInfo), ui.MedidaInsignia(cuenta)))
	quitar := ui.NuevoBoton(l.t("list.deselect"), l.AlDesmarcar).Con(ui.Fantasma, ui.TallaXS)
	b.izq = append(b.izq, piezaDe(quitar, quitar.MinSize()))
	if len(l.Modelo.Estados) > 0 {
		rot := l.t("list.batchState") + ":"
		b.der = append(b.der, pieza{
			ui.Texto(rot, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
			ui.Medir(rot, tema.TextoXS, ui.Normal).Width, tema.TextoXS.Linea, true,
		})
		opciones := []string{l.t("relation.choose")}
		for _, e := range l.Modelo.Estados {
			opciones = append(opciones, e.Etiqueta)
		}
		sel := ui.NuevoSelector(opciones, func(i int) {
			if i > 0 && l.AlLote != nil {
				l.AlLote("estado", l.Modelo.Estados[i-1].Valor)
			}
		})
		// El `<select>` escrito a mano de la web: `bg-surface rounded px-2 py-1 text-xs`
		// y SIN `border` (la clase `border-border` solo pone el color), con la flecha del
		// navegador. Medido: 90×22.
		sel.Propio = selectorPropio(CampoLote, AltoSelectLote)
		sel.Ancho = AnchoSelectLote(opciones)
		if l.EnLote {
			sel.Inerte(true)
		}
		b.der = append(b.der, piezaDe(sel, sel.MinSize()))
		b.juntos = 2 // rótulo y selector van en su propio `gap-1`
	}
	var ids []string
	for _, r := range l.registros() {
		if l.Marcadas[r.ID] {
			ids = append(ids, r.ID)
		}
	}
	exportar := ui.NuevoBoton(l.t("list.exportCsv"), l.exportar(ids)).Con(ui.Secundario, ui.TallaXS)
	exportar.ConIcono(iconos.Descargar, 12)
	exportar.MargenIcono = 1 * 0.25 * tema.Rem
	exportar.Inerte = l.AlExportar == nil
	b.der = append(b.der, piezaDe(exportar, exportar.MinSize()))
	borrar := ui.NuevoBoton(l.t("list.batchDelete"), func() {
		if l.AlLote != nil {
			l.AlLote("borrar", "")
		}
	}).Con(ui.Fantasma, ui.TallaXS)
	borrar.Inerte = l.EnLote
	b.der = append(b.der, piezaDe(borrar, borrar.MinSize()))
	for _, p := range append(slices.Clone(b.izq), b.der...) {
		hijos = append(hijos, p.obj)
	}
	b.colocar(ancho)
	return container.New(b, hijos...), b.alto
}

// --- Los filtros ------------------------------------------------------------

// filtros es la fila de píldoras: «Todos», los estados del modelo con su recuento y la
// papelera, separada por su `ml-2`; y «Restablecer vista» a la derecha. La fila salta
// cuando no cabe (`flex-wrap`), que en móvil es lo normal.
func (l *Lista) filtros(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	f := &filaFiltros{}
	var hijos []fyne.CanvasObject
	if len(l.Modelo.Estados) > 0 {
		rot := l.t("list.filterByStatus") + ":"
		f.piezas = append(f.piezas, pieza{
			ui.Texto(rot, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
			ui.Medir(rot, tema.TextoXS, ui.Normal).Width + MargenRotuloFil, tema.TextoXS.Linea, true,
		})

		todos := l.t("list.all")
		if l.Total >= 0 {
			todos = conCuenta(todos, l.Total)
		}
		f.piezas = append(f.piezas, piezaPildora(NuevaPildora(todos, l.Estado == "", func() { l.estado("") })))
		for _, e := range l.Modelo.Estados {
			est := e
			rotulo := est.Etiqueta
			if n, hay := l.PorEstado[est.Valor]; hay {
				rotulo = conCuenta(rotulo, n)
			}
			f.piezas = append(f.piezas,
				piezaPildora(NuevaPildora(rotulo, l.Estado == est.Valor, func() { l.estado(est.Valor) })))
		}
		f.papelera = len(f.piezas)
	}
	f.piezas = append(f.piezas, piezaPildora(NuevaPildora(l.t("list.trash"), l.Papelera, func() {
		if l.AlPapelera != nil {
			l.AlPapelera(!l.Papelera)
		}
	})))
	for _, p := range f.piezas {
		hijos = append(hijos, p.obj)
	}
	// «Restablecer vista», pegado a la derecha y solo con algo que quitar.
	if l.filtrada() && l.AlLimpiar != nil {
		f.limpiar = ui.NuevoBoton(l.t("action.resetView"), l.AlLimpiar).Con(ui.Fantasma, ui.TallaXS)
		hijos = append(hijos, f.limpiar)
	}
	f.colocar(ancho)
	return container.New(f, hijos...), f.alto
}

func piezaPildora(p *Pildora) pieza { return piezaDe(p, MedidaPildora(p)) }

func (l *Lista) estado(e string) {
	if l.AlEstado != nil {
		l.AlEstado(e)
	}
}

// conCuenta es el «Todos (42)» de la web: el rótulo y su número entre paréntesis. No se
// traduce porque en la web tampoco es una clave, es interpolación de plantilla.
func conCuenta(rotulo string, n int) string { return rotulo + " (" + contar(n) + ")" }

// --- El cuerpo --------------------------------------------------------------

// cuerpo es la tabla (o las tarjetas, o las filas de resumen en móvil), o el estado que
// toque. Devuelve también su alto: no ocupan lo mismo y el layout tiene que saberlo
// antes de colocar el pie.
//
// Un FALLO no tiene cuerpo propio, y es la web: la banda roja de arriba dice qué pasó
// y debajo queda la lista vacía («Aquí todavía no hay nada»). Solo el 403 tiene el
// suyo, el candado, desde el PR #840.
func (l *Lista) cuerpo(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	rs := l.registros()
	switch {
	case l.Fase == Cargando && len(rs) == 0:
		return centrado(ui.NuevaRueda(22, "text-muted", v), AltoEsqueleto), AltoEsqueleto
	case l.Fase == Prohibida:
		// Con un 403 `sin_permiso` la lista no está vacía, es que no se puede ver: el
		// estado vacío de la suite con el candado de 32 en `text-subtle`, las dos claves
		// del ERP y sin botón, porque no hay nada que crear. Otro 403 es un fallo con su
		// mensaje en la banda (`FaseDeLista`).
		x := ui.NuevoVacio(l.t("error.sinPermiso.titulo"), l.t("error.sinPermiso"))
		x.Ilustracion = iconos.Trazo(iconos.Candado, 32, "text-subtle", v).Objeto()
		return l.estadoVacio(x, ancho)
	case len(rs) == 0:
		return l.estadoVacio(l.vacio(v), ancho)
	case l.Tarjetas:
		return l.tarjetas(v)
	case l.esMovil():
		return l.filasMovil(v, ancho)
	}
	return l.tablaERP(v, ancho)
}

// vacio es `EmptyState` con el icono de edificio: «Aquí todavía no hay nada». Con un
// filtro puesto, la acción es quitarlo; sin filtro, «Nuevo» si se puede escribir.
func (l *Lista) vacio(v tema.Variante) *ui.Vacio {
	x := ui.NuevoVacio(l.t("list.empty"), "")
	// La papelera NO cuenta aquí, y es la web: vacía, la papelera dice lo mismo que una
	// lista vacía («Crea el primero con Nuevo») porque su condición mira solo estado y
	// búsqueda.
	switch {
	case l.Estado != "" || l.Busqueda != "":
		if l.AlLimpiar != nil {
			x.Accion = ui.NuevoBoton(l.t("action.resetView"), l.AlLimpiar).Con(ui.Secundario, ui.TallaSM)
		}
	default:
		x.Descripcion = l.t("list.emptyHint", i18n.Vars{"action": l.t("action.new")})
		if !l.SoloLectura && l.AlNueva != nil {
			x.Accion = ui.NuevoBoton(l.t("action.new"), l.AlNueva).Con(ui.Primario, ui.TallaSM)
		}
	}
	x.Ilustracion = iconos.Trazo(iconos.Edificio, IlustracionVacio, "text-subtle", v).Objeto()
	return x
}

// pie es «N a la vista» y, si el servicio dejó cursor, «Ver más».
func (l *Lista) pie(v tema.Variante) (fyne.CanvasObject, float32) {
	if len(l.registros()) == 0 {
		return nil, 0
	}
	visible := l.t("list.showing", i18n.Vars{"n": contar(len(l.registros()))})
	p := &pieLista{
		texto:    ui.Texto(visible, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		completo: visible,
	}
	hijos := []fyne.CanvasObject{p.texto}
	alto := tema.TextoXS.Linea
	if l.HayMas && l.AlMas != nil {
		p.mas = ui.NuevoBoton(l.t("action.more"), l.AlMas).Con(ui.Secundario, ui.TallaSM)
		p.mas.Cargando = l.CargandoMas
		hijos = append(hijos, p.mas)
		alto = max(alto, p.mas.MinSize().Height)
	}
	p.alto = alto
	return container.New(p, hijos...), alto + PiePT
}

// PiePT es el `mt-4` del `<footer>`: se SUMA al `space-y-4` porque el pie es el último
// hijo y su margen no se funde con nada.
const PiePT float32 = 4 * 0.25 * tema.Rem

// AltoEsqueleto es el `py-16` de la rueda de carga sobre su lado de 22.
const AltoEsqueleto float32 = 2*(16*0.25*tema.Rem) + 22

// estadoVacio envuelve un `ui.Vacio` con el alto que él mismo declara para este ancho:
// los estados no ocupan lo mismo y el layout tiene que saberlo antes de colocar el pie.
func (l *Lista) estadoVacio(x *ui.Vacio, ancho float32) (fyne.CanvasObject, float32) {
	alto := x.MedidaVacio(ancho)
	return centrado(x, alto), alto
}

// centrado pone un objeto en el centro horizontal de un bloque de alto fijo, que es lo
// que hace el `flex flex-col items-center` de la web.
func centrado(o fyne.CanvasObject, alto float32) fyne.CanvasObject {
	return container.New(&enElCentro{o: o, alto: alto}, o)
}

type enElCentro struct {
	o    fyne.CanvasObject
	alto float32
}

var _ fyne.Layout = (*enElCentro)(nil)

func (c *enElCentro) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(c.o.MinSize().Width, c.alto)
}

func (c *enElCentro) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	m := c.o.MinSize()
	// Un estado vacío se lleva TODO el ancho —su texto se reparte dentro—; una rueda,
	// solo el suyo y centrada.
	if _, esVacio := c.o.(*ui.Vacio); esVacio {
		c.o.Move(fyne.NewPos(0, 0))
		c.o.Resize(fyne.NewSize(t.Width, t.Height))
		return
	}
	c.o.Resize(m)
	c.o.Move(fyne.NewPos((t.Width-m.Width)/2, (t.Height-m.Height)/2))
}
