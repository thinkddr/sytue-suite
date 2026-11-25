// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Punto de venta (`PuntoVenta.tsx`)
//
// El catálogo de productos con su búsqueda y sus categorías a la izquierda y, a la
// derecha, el ticket: la caja abierta, la cesta con sus cantidades, el IVA por tipo, el
// cliente y el cobro en efectivo (con su cambio) o con tarjeta. Por debajo de 1100 el
// ticket baja debajo del catálogo (`@media (max-width: 1100px)` de app.css).
//
// Las medidas son las de `.erp-tpv*` en app.css; los botones y campos del TPV tienen el
// suelo táctil de 44 y 48 que pone la hoja, más alto que el del resto del ERP, porque
// esta pantalla se usa de pie y con el dedo.
//
// El cobro es lo delicado. La cesta guarda el precio que se VIO; el servicio lo
// contrasta y confirma factura, asiento, cobro y sesión en una transacción. Si se
// pierde la respuesta no se sabe si la venta quedó registrada, así que la cesta se
// congela (`Incierto`) y «Reintentar» manda EXACTAMENTE el mismo cuerpo, con la misma
// operación: el servicio la reconoce y devuelve la factura que ya emitió.
//
// Como la web, el cobro enviado y sin confirmar se guarda ANTES de enviarlo en el almacén
// sin conexión de la suite, a nombre de la cuenta (emisor, persona y organización):
// cerrar la app con un cobro incierto ya no lo pierde, y al volver al punto de venta la
// cesta congelada reaparece con su misma operación (tpv_pendiente.go). Con la cesta se
// crea también un presupuesto («Crear presupuesto») y cada producto puede llevar su
// imagen, que se sube desde aquí (tpv_imagenes.go): la pieza ERP-09.

// Medidas del punto de venta.
const (
	// AnchoTPV es el `max-width: 100rem` de `.erp-tpv`.
	AnchoTPV float32 = 100 * tema.Rem
	// CorteTPV es el `max-width: 1100px` por debajo del cual el ticket baja; CorteTPVMovil,
	// el de 600, que junta el catálogo en dos columnas.
	CorteTPV      float32 = 1100
	CorteTPVMovil float32 = 600
	// HuecoTPV es el `gap` y el `padding` de `.erp-tpv-layout`, y el de dentro del ticket.
	HuecoTPV float32 = 1.25 * tema.Rem
	// AnchoTicketTPV es el `minmax(20rem, 22rem)` de su columna: con sitio, el máximo.
	AnchoTicketTPV float32 = 22 * tema.Rem
	// TactilTPV es el `min-height: max(44px, 2.75rem)` de botones y selects; CampoTPV, el
	// `max(48px, 3rem)` de los campos; CobrarTPV, el `3.5rem` del botón de cobrar.
	TactilTPV float32 = 44
	CampoTPV  float32 = 48
	CobrarTPV float32 = 3.5 * tema.Rem
	// TeselaTPVMin es el `minmax(10.5rem, 1fr)` de la rejilla, TeselaTPVGap su `gap` y
	// TeselaTPVAlto el `min-height: 10rem` de cada producto.
	TeselaTPVMin  float32 = 10.5 * tema.Rem
	TeselaTPVGap  float32 = 0.75 * tema.Rem
	TeselaTPVAlto float32 = 10 * tema.Rem
	// TeselaTPVPad es el `padding: 1rem` de su contenido y TeselaTPVNombreMY el
	// `margin-block: 0.875rem` del nombre.
	TeselaTPVPad      float32 = 1 * tema.Rem
	TeselaTPVNombreMY float32 = 0.875 * tema.Rem
	// LadoCuentaTPV es el cuadro de `2.5rem` con las unidades en la cesta.
	LadoCuentaTPV float32 = 2.5 * tema.Rem
	// AnchoBuscadorTPV es el `max-width: 25rem` del buscador.
	AnchoBuscadorTPV float32 = 25 * tema.Rem
	// MaxUnidadesTPV es el tope de una línea, el mismo que valida el servicio.
	MaxUnidadesTPV = 1000
	// MaxNombreCajaTPV es el `maxLength={100}` del nombre de la caja.
	MaxNombreCajaTPV = 100
	// LimiteClienteTPV: por encima de 400 € la factura simplificada pide cliente.
	// LimiteEfectivoTPV: un pago en efectivo debe ser inferior a 1.000 €.
	LimiteClienteTPV  int64 = 40000
	LimiteEfectivoTPV int64 = 100000
	// PaginaTPV es el `limite: 60` de cada página del catálogo.
	PaginaTPV = 60
	// SiglaIVA va tal cual en las dos superficies: es el nombre del impuesto.
	SiglaIVA = "IVA"
)

var (
	// Los escalones propios de `.erp-tpv*`.
	escalonNombreTPV = tema.Escalon{Tam: 1.0625 * tema.Rem, Linea: 1.35 * 1.0625 * tema.Rem}
	escalonPrecioTPV = tema.Escalon{Tam: 1.125 * tema.Rem, Linea: 1.5 * 1.125 * tema.Rem}
	escalonCuentaTPV = tema.Escalon{Tam: 1.25 * tema.Rem, Linea: 1.5 * 1.25 * tema.Rem}
	escalonTicketTPV = tema.Escalon{Tam: 1.25 * tema.Rem, Linea: 1.5 * 1.25 * tema.Rem}
	escalonTotalTPV  = tema.Escalon{Tam: 1.75 * tema.Rem, Linea: 1.5 * 1.75 * tema.Rem}
	escalonHechaTPV  = tema.Escalon{Tam: 1.4 * tema.Rem, Linea: 1.5 * 1.4 * tema.Rem}
	escalonGranTPV   = tema.Escalon{Tam: 2.5 * tema.Rem, Linea: 1.5 * 2.5 * tema.Rem}
	// escalonFinoTPV es el `0.8rem` de la fecha y del precio de una línea.
	escalonFinoTPV = tema.Escalon{Tam: 0.8 * tema.Rem, Linea: 1.5 * 0.8 * tema.Rem}
)

// tiposTPV son los tipos de IVA que puede llevar un producto vendible en el TPV.
var tiposTPV = []string{"21", "10", "5", "4"}

// LineaTPV es una línea de la cesta: el producto tal como se vio y sus unidades.
type LineaTPV struct {
	Producto Registro `json:"producto"`
	Cantidad int64    `json:"cantidad"`
}

// CuotaTPV es el desglose de un tipo de IVA de la cesta, en céntimos.
type CuotaTPV struct {
	Tipo      int
	Base, IVA int64
}

// TPV es el estado de la pantalla. Vive lo que la Pantalla: salir y volver conserva la
// cesta, y con ella un cobro todavía incierto.
type TPV struct {
	tr  *i18n.Translator
	loc i18n.Locale

	// Productos son los del catálogo cargados; Siguiente, el cursor de «Ver más».
	Productos []Registro
	Siguiente string
	// Sesiones son las cajas abiertas y Sesion la elegida.
	Sesiones []Registro
	Sesion   string
	// Busqueda es lo escrito en el buscador y Categoria la píldora elegida ("" todas).
	Busqueda, Categoria string
	Lineas              []LineaTPV
	// Cliente es el id del contacto y NombreCliente su nombre.
	Cliente, NombreCliente string
	// Medio es api.ERPMedioTarjeta o api.ERPMedioEfectivo; Recibido, lo que se teclea.
	Medio, Recibido string
	// Venta es el cobro hecho; nil, se está armando la cesta.
	Venta *api.ERPVentaTPV
	// Cargando: el catálogo viene de camino. Ocupado: una escritura espera al servicio.
	Cargando, Ocupado bool
	Fallo             string
	// Incierto: se envió un cobro y no se sabe cómo acabó. La cesta no se toca hasta
	// reintentarlo.
	Incierto bool
	// SoloLectura: no tiene los verbos para cobrar.
	SoloLectura bool
	// PuedeCrearProducto ofrece «Nuevo producto».
	PuedeCrearProducto bool
	// PuedePresupuesto ofrece «Crear presupuesto» con la cesta.
	PuedePresupuesto bool
	// Imagenes son las de los productos que la tienen, ya reducidas; EditarImagenes
	// enseña en cada producto el botón de subir la suya, que solo se ofrece con
	// PuedeEditarImagenes. SubiendoImagen es el producto cuya imagen va de camino.
	Imagenes                            map[string]image.Image
	EditarImagenes, PuedeEditarImagenes bool
	SubiendoImagen                      string
	Ventana                             float32

	// Pendientes es donde se guarda el cobro enviado y sin confirmar; nil, solo en
	// memoria (como antes).
	Pendientes AlmacenCobroTPV

	// Buscar es el de SelectorRelacion, para el cliente.
	Buscar func(ctx context.Context, modelo, q string) ([]Registro, error)

	AlBuscar        func(q string)
	AlMas           func()
	AlAbrirCaja     func(caja string)
	AlCobrar        func(c api.ERPCobroTPV)
	AlVerFactura    func(id string)
	AlImprimir      func(f Registro)
	AlCatalogo      func()
	AlNuevoProducto func()
	AlCambio        func()
	AlPresupuesto   func(p api.ERPPresupuestoTPV)
	AlSubirImagen   func(producto string)

	// operacion es el UUID del cobro que se está armando; se rehace al cambiar la cesta.
	operacion string
	// pendiente es el cuerpo enviado y no confirmado: el reintento lo manda tal cual.
	pendiente *api.ERPCobroTPV
	// recuperado: el envío en curso es un reintento.
	recuperado bool
	// ofertaOperacion es el UUID del presupuesto que se está armando, igual que
	// operacion para el cobro; restaurado, que ya se miró si había un cobro guardado.
	ofertaOperacion string
	restaurado      bool
	// adjuntosImagen es el adjunto de cada imagen ya bajada: no se vuelve a bajar.
	adjuntosImagen map[string]string

	buscador, caja, recibido *ui.Entrada
	cliente                  *SelectorRelacion
	sesiones                 *ui.Selector
}

// NuevoTPV crea la pantalla.
func NuevoTPV(tr *i18n.Translator) *TPV {
	t := &TPV{tr: tr, loc: tr.Locale(), Medio: api.ERPMedioTarjeta}
	t.buscador = ui.NuevaEntrada(t.t("list.searchPlaceholder"))
	t.buscador.Icono = iconos.Buscar
	t.buscador.AlCambiar(func(q string) {
		t.Busqueda = q
		if t.AlBuscar != nil {
			t.AlBuscar(q)
		}
	})
	t.caja = ui.NuevaEntrada("")
	t.caja.Rotulo = t.t("pos.register")
	t.recibido = ui.NuevaEntrada("")
	t.recibido.Rotulo = t.t("pos.received")
	t.recibido.AlCambiar(func(s string) {
		if t.Recibido == s {
			return
		}
		t.Recibido = s
		t.operacion = ""
		t.cambio()
	})
	t.cliente = NuevoSelectorRelacion(tr, "contacto", "", "", func(id, nombre string) {
		t.PonerCliente(id, nombre)
	})
	t.sesiones = &ui.Selector{Propio: selectorPropio(CampoFicha, TactilTPV)}
	t.sesiones.ExtendBaseWidget(t.sesiones)
	return t
}

func (t *TPV) t(clave string, vars ...i18n.Vars) string {
	if len(vars) > 0 {
		return t.tr.T(clave, vars[0])
	}
	return t.tr.T(clave, nil)
}

func (t *TPV) cambio() {
	if t.AlCambio != nil {
		t.AlCambio()
	}
}

// --- La cesta ---------------------------------------------------------------------

// PrecioTPV es el `precio_con_iva` de un producto en céntimos, si lo tiene y es entero.
func PrecioTPV(r Registro) (int64, bool) {
	switch v := r.Data["precio_con_iva"].(type) {
	case float64:
		if v == math.Trunc(v) && v >= 0 && v <= math.MaxInt32 {
			return int64(v), true
		}
	case json.Number:
		if n, err := v.Int64(); err == nil && n >= 0 {
			return n, true
		}
	case int64:
		return v, v >= 0
	case int:
		return int64(v), v >= 0
	}
	return 0, false
}

// tipoTPV es el `tipo_iva` del producto como lo guarda el servicio («21»).
func tipoTPV(r Registro) string {
	switch v := r.Data["tipo_iva"].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

// ListoTPV es si el producto se puede vender: precio y un tipo de IVA del TPV.
func ListoTPV(r Registro) bool {
	_, ok := PrecioTPV(r)
	return ok && slices.Contains(tiposTPV, tipoTPV(r))
}

func categoriaTPV(r Registro) string {
	s, _ := r.Data["categoria"].(string)
	return s
}

// NombreCajaTPV es el nombre de la caja que se abre: sin los blancos de los lados y con
// los 100 caracteres del `maxLength` del campo de la web. Solo blancos no es un nombre.
func NombreCajaTPV(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > MaxNombreCajaTPV {
		s = strings.TrimSpace(string(r[:MaxNombreCajaTPV]))
	}
	return s
}

// Editable: ni un cobro en vuelo ni uno incierto.
func (t *TPV) Editable() bool { return !t.Ocupado && !t.Incierto }

// Modificar suma (o resta) unidades de un producto. Cambiar la cesta es otra venta: la
// operación se rehace.
func (t *TPV) Modificar(p Registro, delta int64) {
	if !t.Editable() {
		return
	}
	t.operacion, t.ofertaOperacion = "", ""
	i := slices.IndexFunc(t.Lineas, func(l LineaTPV) bool { return l.Producto.ID == p.ID })
	if i < 0 {
		if delta > 0 {
			t.Lineas = append(t.Lineas, LineaTPV{Producto: p, Cantidad: 1})
		}
		return
	}
	t.Lineas[i] = LineaTPV{Producto: p, Cantidad: min(MaxUnidadesTPV, t.Lineas[i].Cantidad+delta)}
	t.Lineas = slices.DeleteFunc(t.Lineas, func(l LineaTPV) bool { return l.Cantidad <= 0 })
}

// Cantidad son las unidades de ese producto en la cesta.
func (t *TPV) Cantidad(id string) int64 {
	for _, l := range t.Lineas {
		if l.Producto.ID == id {
			return l.Cantidad
		}
	}
	return 0
}

// Unidades es la suma de unidades de la cesta.
func (t *TPV) Unidades() int64 {
	var n int64
	for _, l := range t.Lineas {
		n += l.Cantidad
	}
	return n
}

// Totales es `grupos` y `cuotas` de la web: el total con IVA y, por cada tipo en el orden
// en que entró en la cesta, su base y su cuota. Se agrupa ANTES de desglosar —como el
// servicio—, así que la suma de bases y cuotas cuadra siempre al céntimo con el total.
func (t *TPV) Totales() (total int64, cuotas []CuotaTPV) {
	var orden []int
	grupos := map[int]int64{}
	for _, l := range t.Lineas {
		precio, _ := PrecioTPV(l.Producto)
		tipo, _ := strconv.Atoi(tipoTPV(l.Producto))
		if _, ok := grupos[tipo]; !ok {
			orden = append(orden, tipo)
		}
		grupos[tipo] += precio * l.Cantidad
	}
	for _, tipo := range orden {
		base, iva := Desglosar(grupos[tipo], tipo)
		cuotas = append(cuotas, CuotaTPV{Tipo: tipo, Base: base, IVA: iva})
		total += grupos[tipo]
	}
	return total, cuotas
}

// efectivo es lo recibido en céntimos (0 si no es un importe).
func (t *TPV) efectivo() int64 {
	c, _ := ACentimos(t.Recibido)
	return c
}

// efectivoExcesivo: la ley no deja cobrar en efectivo 1.000 € o más.
func (t *TPV) efectivoExcesivo(total int64) bool {
	return t.Medio == api.ERPMedioEfectivo && total >= LimiteEfectivoTPV
}

// PonerCliente elige (o quita, con los dos vacíos) el cliente de la venta.
func (t *TPV) PonerCliente(id, nombre string) {
	if !t.Editable() {
		return
	}
	t.Cliente, t.NombreCliente, t.operacion, t.ofertaOperacion = id, nombre, "", ""
	t.cliente.Valor, t.cliente.Etiqueta = id, nombre
	t.cliente.Refresh()
	t.cambio()
}

// PonerMedio elige efectivo o tarjeta.
func (t *TPV) PonerMedio(m string) {
	if !t.Editable() || t.Medio == m {
		return
	}
	t.Medio, t.operacion = m, ""
	t.cambio()
}

// PonerSesion elige la caja.
func (t *TPV) PonerSesion(id string) {
	if !t.Editable() {
		return
	}
	t.Sesion, t.operacion = id, ""
}

// PrepararCobro es el cuerpo que se envía y, si no se puede cobrar, la clave del motivo.
// Con un cobro incierto devuelve EL MISMO cuerpo, que es lo que hace seguro reintentar.
func (t *TPV) PrepararCobro() (api.ERPCobroTPV, string) {
	total, _ := t.Totales()
	if t.pendiente != nil {
		return *t.pendiente, ""
	}
	if t.Sesion == "" || len(t.Lineas) == 0 || total <= 0 ||
		(t.Cliente == "" && total > LimiteClienteTPV) ||
		(t.Medio == api.ERPMedioEfectivo && t.efectivo() < total) {
		return api.ERPCobroTPV{}, "pos.review"
	}
	if t.efectivoExcesivo(total) {
		return api.ERPCobroTPV{}, "pos.cashLimit"
	}
	if t.operacion == "" {
		t.operacion = uuid.NewString()
	}
	recibido := total
	if t.Medio == api.ERPMedioEfectivo {
		recibido = t.efectivo()
	}
	c := api.ERPCobroTPV{Operacion: t.operacion, Sesion: t.Sesion, Cliente: t.Cliente, Medio: t.Medio, Recibido: recibido}
	for _, l := range t.Lineas {
		precio, _ := PrecioTPV(l.Producto)
		c.Lineas = append(c.Lineas, api.ERPLineaTPV{Producto: l.Producto.ID, Cantidad: l.Cantidad, Precio: precio, TipoIVA: tipoTPV(l.Producto)})
	}
	return c, ""
}

// Cobrar valida, congela la cesta y entrega el cuerpo a AlCobrar.
func (t *TPV) Cobrar() {
	if t.Ocupado || t.SoloLectura || t.AlCobrar == nil {
		return
	}
	c, motivo := t.PrepararCobro()
	if motivo != "" {
		t.Fallo = t.t(motivo)
		t.cambio()
		return
	}
	// Se guarda ANTES de enviar: si la respuesta se pierde y la app se cierra, al volver
	// se reintenta la misma operación. Si no se puede guardar, no se envía: otro cobro
	// distinto sin confirmar (de otra ventana) se recupera en vez de pisarlo.
	if err := t.guardarPendiente(c); err != nil {
		t.Fallo = t.t("pos.resume")
		if !errors.Is(err, ErrOtroCobroTPV) {
			t.Fallo = t.t("error.generic")
		}
		t.Restaurar()
		t.cambio()
		return
	}
	t.recuperado = t.Incierto
	t.pendiente, t.Incierto, t.Ocupado, t.Fallo = &c, true, true, ""
	t.cambio()
	t.AlCobrar(c)
}

// Cobrado recibe la respuesta del servicio. Solo un 400 o un 422 al PRIMER envío prueba
// que no se registró nada (el servicio deshizo la transacción): entonces la cesta se
// libera. Cualquier otro fallo —la red, un 5xx, un 401— o un rechazo de un reintento
// deja la misma operación lista para reintentar.
func (t *TPV) Cobrado(v api.ERPVentaTPV, err error, mensaje string) {
	t.Ocupado = false
	if err == nil {
		t.quitarPendiente()
		t.Venta, t.Incierto, t.pendiente, t.Fallo = &v, false, nil, ""
		return
	}
	t.Fallo = mensaje
	var e *api.Error
	if !t.recuperado && errors.As(err, &e) && (e.Status == http.StatusBadRequest || e.Status == http.StatusUnprocessableEntity) {
		// Si no se puede borrar lo guardado, se queda incierto: mejor reintentar la
		// misma operación que dejar guardado un cobro que la cesta ya no enseña.
		if t.quitarPendiente() {
			t.pendiente, t.Incierto = nil, false
		}
	}
}

// NuevaVenta vacía el ticket para la siguiente.
func (t *TPV) NuevaVenta() {
	if t.Ocupado {
		return
	}
	if !t.quitarPendiente() {
		t.Fallo = t.t("error.generic")
		t.cambio()
		return
	}
	t.Venta, t.pendiente, t.Incierto, t.Fallo = nil, nil, false, ""
	t.Lineas, t.operacion, t.ofertaOperacion, t.Recibido = nil, "", "", ""
	t.recibido.PonerTexto("")
	t.Cliente, t.NombreCliente = "", ""
	t.cliente.Valor, t.cliente.Etiqueta = "", ""
	t.cliente.Refresh()
	t.cambio()
}

// Categorias son las de los productos cargados, ordenadas.
func (t *TPV) Categorias() []string {
	var out []string
	for _, p := range t.Productos {
		if c := categoriaTPV(p); c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	return out
}

// visibles son los productos de la categoría elegida.
func (t *TPV) visibles() []Registro {
	if t.Categoria == "" {
		return t.Productos
	}
	var out []Registro
	for _, p := range t.Productos {
		if categoriaTPV(p) == t.Categoria {
			out = append(out, p)
		}
	}
	return out
}

func (t *TPV) dinero(c int64) string { return i18n.FormatCurrency(t.loc, c) }

// --- La vista ---------------------------------------------------------------------

func (t *TPV) ancho() bool { return t.Ventana == 0 || t.Ventana > CorteTPV }
func (t *TPV) movil() bool { return t.Ventana > 0 && t.Ventana <= CorteTPVMovil }
func (t *TPV) relleno() float32 {
	if t.movil() {
		return 0.875 * tema.Rem
	}
	return HuecoTPV
}

// Objeto arma la pantalla.
func (t *TPV) Objeto(v tema.Variante) fyne.CanvasObject {
	ancho := anchoUtilDe(t.Ventana, AnchoTPV)
	p := &pila{}
	cab, hc := t.cabecera(v, ancho)
	p.add(cab, hc, 0)
	if t.Fallo != "" {
		bnd := &banda{
			caja:     ui.NuevaCaja(tema.Color("danger-bg", v), sinColor, tema.RadioMD),
			texto:    ui.Texto(t.Fallo, tema.TextoSM, ui.Normal, tema.Color("danger-text", v)),
			completo: t.Fallo,
		}
		p.add(container.New(bnd, append(bnd.caja.Objetos(), bnd.texto)...), AltoBandaAviso, HuecoDos)
	}
	o, alto := t.disposicion(v, ancho)
	p.add(o, alto, 1.5*tema.Rem)
	todo, h := p.objeto()
	return enPagina(todo, h, AnchoTPV)
}

// cabecera es «Sytue ERP» en versalitas, el título y, a la derecha, «Nuevo producto» y
// «Catálogo», que abre la lista de productos.
func (t *TPV) cabecera(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	// `clamp(1.7rem, 2.5vw, 2.4rem)`.
	tam := min(max(1.7*tema.Rem, 0.025*t.Ventana), 2.4*tema.Rem)
	titulo := tema.Escalon{Tam: tam, Linea: 1.25 * tam}
	l := &lienzo{}
	sobre := ui.NuevoVersal("Sytue ERP", tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v))
	l.poner(sobre.Objeto(), 0, 0, ancho, tema.TextoXS.Linea)
	l.texto(ui.Texto(t.t("pos.title"), titulo, ui.SemiNegrita, tema.Color("text", v)), 0, tema.TextoXS.Linea)
	izq := tema.TextoXS.Linea + titulo.Linea
	var ps []pieza
	if t.PuedeCrearProducto && t.AlNuevoProducto != nil {
		b := ui.NuevoBoton(t.t("pos.newProduct"), t.AlNuevoProducto).Con(ui.Secundario, ui.TallaMD)
		ps = append(ps, piezaDe(b, fyne.NewSize(b.MinSize().Width, TactilTPV)))
	}
	if t.AlCatalogo != nil {
		b := ui.NuevoBoton(t.t("pos.catalogue"), t.AlCatalogo).Con(ui.Fantasma, ui.TallaMD)
		b.Icono = iconos.Rejilla
		ps = append(ps, piezaDe(b, fyne.NewSize(b.MinSize().Width, TactilTPV)))
	}
	// «Imágenes» del menú «Catálogo» de la web: enseña en cada producto el botón de
	// subir su imagen. Encendido, va lleno (el `aria-pressed`).
	if t.PuedeEditarImagenes && t.AlSubirImagen != nil {
		variante := ui.Fantasma
		if t.EditarImagenes {
			variante = ui.Secundario
		}
		b := ui.NuevoBoton(t.t("pos.images"), func() {
			t.EditarImagenes = !t.EditarImagenes
			t.cambio()
		}).Con(variante, ui.TallaMD)
		b.Icono = iconos.Camara
		ps = append(ps, piezaDe(b, fyne.NewSize(b.MinSize().Width, TactilTPV)))
	}
	if len(ps) == 0 {
		return l.objeto(izq)
	}
	an, al := medidasDe(ps)
	pos, ocupa := Envolver(an, al, HuecoDos, HuecoDos, ancho)
	// `flex-wrap justify-between items-center`: los botones a la derecha si caben.
	if ocupa.Width+1*tema.Rem+ui.Medir(t.t("pos.title"), titulo, ui.SemiNegrita).Width <= ancho {
		y := (izq - ocupa.Height) / 2
		l.piezas(ps, pos, ancho-ocupa.Width, y)
		return l.objeto(max(izq, ocupa.Height))
	}
	l.piezas(ps, pos, 0, izq+1*tema.Rem)
	return l.objeto(izq + 1*tema.Rem + ocupa.Height)
}

// disposicion es `.erp-tpv-layout`: la caja `bg-subtle` con el catálogo y el ticket, lado
// a lado en ancho y uno debajo del otro por debajo del corte.
func (t *TPV) disposicion(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	pad := t.relleno()
	dentro := ancho - 2*pad
	l := &lienzo{}
	l.caja(ui.NuevaCaja(tema.Color("bg-subtle", v), sinColor, tema.RadioXL), 0, 0)
	var alto float32
	if t.ancho() {
		wc := max(dentro-AnchoTicketTPV-HuecoTPV, 0)
		oc, hc := t.catalogo(v, wc)
		ot, ht := t.ticket(v, AnchoTicketTPV)
		l.poner(oc, pad, pad, wc, hc)
		l.poner(ot, pad+wc+HuecoTPV, pad, AnchoTicketTPV, ht)
		alto = max(hc, ht) + 2*pad
	} else {
		hueco := HuecoTPV
		if t.movil() {
			hueco = 1.5 * tema.Rem
		}
		oc, hc := t.catalogo(v, dentro)
		wt := min(dentro, 40*tema.Rem) // `max-width: 40rem`
		ot, ht := t.ticket(v, wt)
		l.poner(oc, pad, pad, dentro, hc)
		l.poner(ot, pad, pad+hc+hueco, wt, ht)
		alto = hc + hueco + ht + 2*pad
	}
	l.puestos[0].w, l.puestos[0].h = ancho, alto
	return l.objeto(alto)
}

// catalogo es `.erp-tpv-catalogue`: el buscador, las categorías, la rejilla, «Ver más» y
// la nota del IVA incluido, separados por `gap: 1rem`.
func (t *TPV) catalogo(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	var gap float32 = 1 * tema.Rem
	add := func(o fyne.CanvasObject, alto float32) {
		arriba := gap
		if len(p.bloques) == 0 {
			arriba = 0
		}
		p.add(o, alto, arriba)
	}
	if t.buscador.Texto() != t.Busqueda {
		t.buscador.PonerTexto(t.Busqueda)
	}
	wb := min(ancho, AnchoBuscadorTPV)
	t.buscador.Ancho = wb
	lb := &lienzo{}
	lb.poner(t.buscador, 0, 0, wb, TactilTPV)
	add(lb.objeto(TactilTPV))
	if cats := t.Categorias(); len(cats) > 0 {
		add(t.categorias(cats, ancho))
	}
	productos := t.visibles()
	switch {
	case t.Cargando && len(t.Productos) == 0:
		alto := 24 + 2*CargandoOblPY
		add(centrado(ui.NuevaRueda(24, "text-muted", v), alto), alto)
	case len(productos) == 0:
		// `py-12`.
		alto := tema.TextoSM.Linea + 2*12*0.25*tema.Rem
		x := ui.Texto(t.t("relation.empty"), tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
		lv := &lienzo{}
		lv.texto(x, 0, 12*0.25*tema.Rem)
		add(lv.objeto(alto))
	default:
		add(t.rejilla(productos, ancho))
	}
	if t.Siguiente != "" && t.AlMas != nil {
		b := ui.NuevoBoton(t.t("action.more"), t.AlMas).Con(ui.Secundario, ui.TallaMD)
		b.Inerte = t.Cargando
		lm := &lienzo{}
		lm.poner(b, 0, 0, b.MinSize().Width, TactilTPV)
		add(lm.objeto(TactilTPV))
	}
	nota := ui.Texto(t.t("pos.taxIncluded"), tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
	add(textoSolo(nota), tema.TextoSM.Linea)
	return p.objeto()
}

// categorias es `.erp-tpv-categories`: «Todos» y una por categoría; la elegida, llena.
func (t *TPV) categorias(cats []string, ancho float32) (fyne.CanvasObject, float32) {
	boton := func(rotulo, valor string) pieza {
		variante := ui.Fantasma
		if t.Categoria == valor {
			variante = ui.Primario
		}
		b := ui.NuevoBoton(rotulo, func() {
			t.Categoria = valor
			t.cambio()
		}).Con(variante, ui.TallaMD)
		return piezaDe(b, fyne.NewSize(b.MinSize().Width, TactilTPV))
	}
	ps := make([]pieza, 0, 1+len(cats))
	ps = append(ps, boton(t.t("list.all"), ""))
	for _, c := range cats {
		ps = append(ps, boton(c, c))
	}
	an, al := medidasDe(ps)
	pos, ocupa := Envolver(an, al, HuecoDos, HuecoDos, ancho)
	l := &lienzo{}
	l.piezas(ps, pos, 0, 0)
	return l.objeto(ocupa.Height)
}

// ColumnasTPV es `repeat(auto-fill, minmax(min(100%, 10.5rem), 1fr))` —dos fijas en el
// teléfono— y el ancho de cada una.
func ColumnasTPV(ancho float32, movil bool) (int, float32) {
	gap := TeselaTPVGap
	if movil {
		gap = 0.625 * tema.Rem
		return 2, max((ancho-gap)/2, 0)
	}
	n := max(int((ancho+gap)/(TeselaTPVMin+gap)), 1)
	return n, max((ancho-gap*float32(n-1))/float32(n), 0)
}

// rejilla coloca las teselas por filas; cada fila mide lo que la más alta (`stretch`).
func (t *TPV) rejilla(productos []Registro, ancho float32) (fyne.CanvasObject, float32) {
	n, w := ColumnasTPV(ancho, t.movil())
	gap := TeselaTPVGap
	if t.movil() {
		gap = 0.625 * tema.Rem
	}
	cats := t.Categorias()
	tonos := ui.TonosTarjeta()
	l := &lienzo{}
	var y float32
	for i := 0; i < len(productos); i += n {
		fila := productos[i:min(i+n, len(productos))]
		teselas := make([]*TeselaTPV, len(fila))
		var alto float32
		for j, p := range fila {
			precio, _ := PrecioTPV(p)
			listo := ListoTPV(p)
			te := NuevaTeselaTPV(p, t.Cantidad(p.ID), func() { t.Modificar(p, 1); t.cambio() })
			te.Movil = t.movil()
			te.Imagen = t.Imagenes[p.ID]
			te.Tono = tonos[max(0, slices.Index(cats, categoriaTPV(p)))%len(tonos)]
			te.Precio = t.t("pos.configure")
			if listo {
				te.Precio = t.dinero(precio)
			}
			te.Inerte = !listo || !t.Editable()
			teselas[j] = te
			alto = max(alto, te.AltoPara(w))
		}
		for j, te := range teselas {
			x := float32(j) * (w + gap)
			l.poner(te, x, y, w, alto)
			// El botón de la cámara, arriba a la derecha (`.erp-tpv-product-image-action`).
			if t.EditarImagenes && t.PuedeEditarImagenes && t.AlSubirImagen != nil {
				id := fila[j].ID
				nombre := NombreDe(fila[j])
				b := ui.NuevoBoton("", func() { t.AlSubirImagen(id) }).Con(ui.Secundario, ui.TallaSM)
				b.Icono, b.SoloIcono = iconos.Camara, true
				b.Rotulo = t.t("pos.imageUpload", i18n.Vars{"name": nombre})
				b.Cargando = t.SubiendoImagen == id
				b.Inerte = t.SubiendoImagen != ""
				lado := max(b.MinSize().Width, TactilTPV)
				l.poner(b, x+w-0.75*tema.Rem-lado, y+0.75*tema.Rem, lado, TactilTPV)
			}
		}
		y += alto + gap
	}
	return l.objeto(max(y-gap, 0))
}

// ticket es `.erp-tpv-receipt`: la venta en curso o la venta hecha.
func (t *TPV) ticket(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	l.caja(ui.NuevaCaja(tema.Color("surface", v), tema.Color("border-subtle", v), tema.RadioXL).ConTrazo(), 0, 0)
	dentro := ancho - 2*HuecoTPV
	var o fyne.CanvasObject
	var h float32
	if t.Venta != nil {
		o, h = t.hecha(v, dentro)
	} else {
		o, h = t.enCurso(v, dentro)
	}
	l.poner(o, HuecoTPV, HuecoTPV, dentro, h)
	alto := h + 2*HuecoTPV
	l.puestos[0].w, l.puestos[0].h = ancho, alto
	return l.objeto(alto)
}

// enCurso es el ticket mientras se arma la venta.
func (t *TPV) enCurso(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	// La cabecera: «Venta actual» y la fecha, con la raya debajo.
	tit := ui.Texto(t.t("pos.sale"), escalonTicketTPV, ui.SemiNegrita, tema.Color("text", v))
	fecha := i18n.FormatDateShortMonth(t.loc, time.Now())
	tf := ui.Texto(fecha, escalonFinoTPV, ui.Normal, tema.Color("text-muted", v))
	l.texto(tit, 0, 0)
	wf := ui.Medir(fecha, escalonFinoTPV, ui.Normal).Width
	l.texto(tf, ancho-wf, escalonTicketTPV.Linea-escalonFinoTPV.Linea)
	y := escalonTicketTPV.Linea + 1*tema.Rem
	l.poner(canvas.NewRectangle(tema.Color("border", v)), 0, y, ancho, tema.Borde)
	y += tema.Borde + 1*tema.Rem
	o, h := t.cajaTPV(v, ancho)
	l.poner(o, 0, y, ancho, h)
	y += h + 1*tema.Rem
	o, h = t.lineas(v, ancho)
	l.poner(o, 0, y, ancho, h)
	y += h + 1*tema.Rem
	// «Crear presupuesto», con la cesta y quien puede crearlos (ERP-09).
	if t.PuedePresupuesto && t.AlPresupuesto != nil && len(t.Lineas) > 0 {
		b := ui.NuevoBoton(t.t("pos.quote"), t.CrearPresupuesto).Con(ui.Secundario, ui.TallaMD)
		b.Inerte = !t.Editable()
		l.poner(b, 0, y, b.MinSize().Width, TactilTPV)
		y += TactilTPV + 1*tema.Rem
	}
	o, h = t.totales(v, ancho)
	l.poner(o, 0, y+0.25*tema.Rem, ancho, h)
	y += 0.25*tema.Rem + h + 1.5*tema.Rem
	o, h = t.pago(v, ancho)
	l.poner(o, 0, y, ancho, h)
	return l.objeto(y + h)
}

// cajaTPV es «Abrir caja» si no hay ninguna abierta, o el selector de la caja.
func (t *TPV) cajaTPV(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	rot := ui.Texto(t.t("pos.register"), tema.TextoBase, ui.Medio, tema.Color("text", v))
	l.texto(rot, 0, 0)
	y := tema.TextoBase.Linea + HuecoDos
	if len(t.Sesiones) == 0 && !t.SoloLectura {
		t.caja.Ancho = ancho
		l.poner(t.caja, 0, y, ancho, CampoTPV)
		y += CampoTPV + HuecoDos
		b := ui.NuevoBoton(t.t("pos.openRegister"), func() {
			if nombre := NombreCajaTPV(t.caja.Texto()); nombre != "" && t.AlAbrirCaja != nil && !t.Ocupado {
				t.AlAbrirCaja(nombre)
			}
		}).Con(ui.Primario, ui.TallaMD)
		b.Inerte = t.Ocupado
		l.poner(b, 0, y, b.MinSize().Width, TactilTPV)
		return l.objeto(y + TactilTPV)
	}
	ops := make([]string, len(t.Sesiones))
	elegida := -1
	for i, s := range t.Sesiones {
		caja, _ := s.Data["caja"].(string)
		ops[i] = caja + " · " + s.Ref
		if s.ID == t.Sesion {
			elegida = i
		}
	}
	sesiones := t.Sesiones
	t.sesiones.Opciones, t.sesiones.Elegida, t.sesiones.Rotulo, t.sesiones.Ancho = ops, elegida, t.t("pos.register"), ancho
	t.sesiones.AlElegir = func(i int) {
		if i >= 0 && i < len(sesiones) {
			t.PonerSesion(sesiones[i].ID)
			t.cambio()
		}
	}
	t.sesiones.Inerte(!t.Editable())
	l.poner(t.sesiones, 0, y, ancho, TactilTPV)
	return l.objeto(y + TactilTPV)
}

// lineas es `.erp-tpv-lines`: «Añade productos» o cada línea con su nombre, su precio y
// tipo, el mando de unidades y el importe.
func (t *TPV) lineas(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	if len(t.Lineas) == 0 {
		// `padding-block: 2rem; text-align: center`.
		s := t.t("pos.addProducts")
		x := ui.Texto(s, tema.TextoBase, ui.Normal, tema.Color("text-muted", v))
		l.texto(x, max((ancho-ui.Medir(s, tema.TextoBase, ui.Normal).Width)/2, 0), 2*tema.Rem)
		return l.objeto(tema.TextoBase.Linea + 4*tema.Rem)
	}
	var y float32
	for i, ln := range t.Lineas {
		if i > 0 {
			y += HuecoTPV
		}
		producto := ln.Producto
		nombre := NombreDe(producto)
		par := ui.NuevoParrafo(nombre, tema.TextoBase, ui.SemiNegrita, "text", v)
		hn := ui.AltoParrafo(par, ancho)
		l.poner(par, 0, y, ancho, hn)
		y += hn + HuecoUno
		precio, _ := PrecioTPV(producto)
		detalle := t.dinero(precio) + " · " + tipoTPV(producto) + "% " + SiglaIVA
		l.texto(ui.Texto(detalle, escalonFinoTPV, ui.Normal, tema.Color("text-muted", v)), 0, y)
		y += escalonFinoTPV.Linea + HuecoDos
		// El mando: − cifra +, sobre el primario.
		l.poner(cajaSuelta(ui.NuevaCaja(tema.Color("primary", v), tema.Color("primary", v), tema.RadioMD)),
			0, y, 2*TactilTPV+2.75*tema.Rem, TactilTPV)
		menos := ui.NuevoBoton("−", func() { t.Modificar(producto, -1); t.cambio() }).Con(ui.Primario, ui.TallaLG)
		menos.Rotulo, menos.Inerte = t.t("pos.removeUnit", i18n.Vars{"name": nombre}), !t.Editable()
		mas := ui.NuevoBoton("+", func() { t.Modificar(producto, 1); t.cambio() }).Con(ui.Primario, ui.TallaLG)
		mas.Rotulo, mas.Inerte = t.t("pos.addUnit", i18n.Vars{"name": nombre}), !t.Editable()
		l.poner(menos, 0, y, TactilTPV, TactilTPV)
		cifra := strconv.FormatInt(ln.Cantidad, 10)
		wc := ui.Medir(cifra, tema.TextoBase, ui.Negrita).Width
		l.texto(ui.Texto(cifra, tema.TextoBase, ui.Negrita, tema.Color("on-primary", v)),
			TactilTPV+(2.75*tema.Rem-wc)/2, y+(TactilTPV-tema.TextoBase.Linea)/2)
		l.poner(mas, TactilTPV+2.75*tema.Rem, y, TactilTPV, TactilTPV)
		imp, wi := textoTabular(t.dinero(precio*ln.Cantidad), tema.TextoBase, ui.SemiNegrita, tema.Color("text", v))
		l.poner(imp, ancho-wi, y+(TactilTPV-tema.TextoBase.Linea)/2, wi, tema.TextoBase.Linea)
		y += TactilTPV
	}
	// `max-height: 16rem; overflow-y: auto`: una cesta larga no empuja el cobro fuera.
	o, alto := l.objeto(y)
	if tope := float32(16 * tema.Rem); alto > tope {
		d := container.NewVScroll(o)
		lc := &lienzo{}
		lc.poner(d, 0, 0, ancho, tope)
		return lc.objeto(tope)
	}
	return o, alto
}

// totales es `.erp-tpv-totals`: el IVA de cada tipo y el total, entre dos rayas.
func (t *TPV) totales(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	total, cuotas := t.Totales()
	l := &lienzo{}
	l.poner(canvas.NewRectangle(tema.Color("border", v)), 0, 0, ancho, tema.Borde)
	y := tema.Borde + HuecoTPV
	fila := func(izq, der string, e tema.Escalon, peso ui.Peso) {
		// Las dos mitades por el mismo camino, para que compartan línea base.
		oi, wi := textoTabular(izq, e, peso, tema.Color("text", v))
		l.poner(oi, 0, y, wi, e.Linea)
		o, w := textoTabular(der, e, peso, tema.Color("text", v))
		l.poner(o, ancho-w, y, w, e.Linea)
		y += e.Linea + HuecoDos
	}
	for _, c := range cuotas {
		fila(SiglaIVA+" "+strconv.Itoa(c.Tipo)+"%", t.dinero(c.IVA), tema.TextoBase, ui.Normal)
	}
	fila(t.t("pos.total"), t.dinero(total), escalonTotalTPV, ui.SemiNegrita)
	y += HuecoTPV - HuecoDos
	l.poner(canvas.NewRectangle(tema.Color("border", v)), 0, y, ancho, tema.Borde)
	return l.objeto(y + tema.Borde)
}

// pago es `.erp-tpv-payment`: el cliente, la forma de cobro, lo recibido y el botón.
func (t *TPV) pago(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	total, _ := t.Totales()
	editable := t.Editable()
	l := &lienzo{}
	leyenda := func(s string, y float32) float32 {
		l.texto(ui.Texto(s, tema.TextoBase, ui.Medio, tema.Color("text", v)), 0, y)
		return y + tema.TextoBase.Linea + HuecoDos
	}
	y := leyenda(t.t("pos.customer"), 0)
	t.cliente.Buscar, t.cliente.Inerte = t.Buscar, !editable
	alt := max(t.cliente.MinSize().Height, CampoTPV)
	l.poner(t.cliente, 0, y, ancho, alt)
	y += alt + HuecoUno
	if t.Cliente != "" {
		q := NuevoPulsador(t.t("list.clearFilters"), tema.TextoSM, ui.Normal, func() { t.PonerCliente("", "") })
		q.Subrayado, q.Token, q.TokenEncima, q.Radio = true, "text-muted", "text", tema.RadioSM
		q.Inerte = !editable
		mq := q.MinSize()
		l.poner(q, 0, y, mq.Width, mq.Height)
		y += mq.Height + HuecoUno
	}
	pista := ui.NuevoParrafo(t.t("pos.customerHint"), tema.TextoXS, ui.Normal, "text-muted", v)
	hp := ui.AltoParrafo(pista, ancho)
	l.poner(pista, 0, y, ancho, hp)
	y += hp + 1*tema.Rem
	// La forma de cobro: dos cajas lado a lado (`flex: 1; gap: 0.75rem`).
	y = leyenda(t.t("pos.payment"), y)
	wm := (ancho - TeselaTPVGap) / 2
	for i, m := range []string{api.ERPMedioTarjeta, api.ERPMedioEfectivo} {
		medio := m
		rot := t.t("pos.card")
		if m == api.ERPMedioEfectivo {
			rot = t.t("pos.cash")
		}
		elegido := t.Medio == m
		borde, fondo := tema.Color("border-strong", v), tema.Color("surface", v)
		if elegido {
			borde, fondo = tema.Color("primary", v), tema.Color("selected", v)
		}
		x := float32(i) * (wm + TeselaTPVGap)
		alto := TactilTPV + 2*0.75*tema.Rem - 2*HuecoDos
		l.poner(cajaSuelta(ui.NuevaCaja(fondo, borde, tema.RadioMD).ConTrazo()), x, y, wm, alto)
		r := ui.NuevoRadio(rot, func(bool) { t.PonerMedio(medio) })
		r.Marcada = elegido
		if !editable {
			r.Disable()
		}
		mr := r.MinSize()
		l.poner(r, x+0.75*tema.Rem, y+(alto-mr.Height)/2, min(mr.Width, wm-1.5*tema.Rem), mr.Height)
		if i == 1 {
			y += alto + 1*tema.Rem
		}
	}
	if t.Medio == api.ERPMedioEfectivo {
		y = leyenda(t.t("pos.received"), y)
		if t.recibido.Texto() != t.Recibido {
			t.recibido.PonerTexto(t.Recibido)
		}
		t.recibido.Ancho = ancho
		t.recibido.Inerte(!editable)
		l.poner(t.recibido, 0, y, ancho, CampoTPV)
		y += CampoTPV + HuecoDos
		cambio := t.t("pos.change") + ": " + t.dinero(max(0, t.efectivo()-total))
		l.texto(ui.Texto(cambio, tema.TextoBase, ui.Medio, tema.Color("text", v)), 0, y)
		y += tema.TextoBase.Linea + 1*tema.Rem
	} else {
		par := ui.NuevoParrafo(t.t("pos.terminalHint"), tema.TextoXS, ui.Normal, "text-muted", v)
		h := ui.AltoParrafo(par, ancho)
		l.poner(par, 0, y, ancho, h)
		y += h + 1*tema.Rem
	}
	aviso := func(clave, token string) {
		par := ui.NuevoParrafo(t.t(clave), tema.TextoSM, ui.Normal, token, v)
		h := ui.AltoParrafo(par, ancho)
		l.poner(par, 0, y, ancho, h)
		y += h + 1*tema.Rem
	}
	if t.Incierto && !t.Ocupado {
		aviso("pos.retryHint", "warning-text")
	}
	excesivo := t.efectivoExcesivo(total)
	if excesivo {
		aviso("pos.cashLimit", "danger-text")
	}
	clave := "pos.charge"
	if t.Incierto {
		clave = "pos.retry"
	}
	b := ui.NuevoBoton(t.t(clave, i18n.Vars{"amount": t.dinero(total)}), t.Cobrar).Con(ui.Primario, ui.TallaLG)
	b.Ancho, b.Cargando = ancho, t.Ocupado
	b.Inerte = t.SoloLectura || len(t.Lineas) == 0 || excesivo
	l.poner(b, 0, y, ancho, CobrarTPV)
	return l.objeto(y + CobrarTPV)
}

// hecha es `.erp-tpv-success`: el tic, «Venta registrada», la referencia, el total, el
// cambio y «Imprimir factura», «Abrir» y «Nueva venta», todo centrado.
func (t *TPV) hecha(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	f := t.Venta.Factura
	l := &lienzo{}
	var y float32 = 2 * tema.Rem
	centro := func(o fyne.CanvasObject, w, h float32) {
		l.poner(o, (ancho-w)/2, y, w, h)
		y += h + 1*tema.Rem
	}
	texto := func(s string, e tema.Escalon, peso ui.Peso, token string) {
		w := ui.Medir(s, e, peso).Width
		l.texto(ui.Texto(s, e, peso, tema.Color(token, v)), max((ancho-w)/2, 0), y)
		y += e.Linea + 1*tema.Rem
	}
	centro(iconos.Trazo(iconos.Tic, 32, "success-text", v).Objeto(), 32, 32)
	texto(t.t("pos.recorded"), escalonHechaTPV, ui.SemiNegrita, "text")
	texto(f.Ref, tema.TextoBase, ui.Normal, "text")
	total, _ := PrecioTPV(Registro{Data: map[string]any{"precio_con_iva": f.Data["total"]}})
	o, w := textoTabular(t.dinero(total), escalonGranTPV, ui.Negrita, tema.Color("text", v))
	centro(o, w, escalonGranTPV.Linea)
	if t.Venta.Cambio > 0 {
		texto(t.t("pos.change")+": "+t.dinero(t.Venta.Cambio), tema.TextoBase, ui.Normal, "text")
	}
	boton := func(s string, variante ui.Variante, al func()) {
		b := ui.NuevoBoton(s, al).Con(variante, ui.TallaMD)
		b.Inerte = t.Ocupado
		centro(b, b.MinSize().Width, TactilTPV)
	}
	if t.AlImprimir != nil {
		boton(t.t("form.print"), ui.Primario, func() { t.AlImprimir(f) })
	}
	if t.AlVerFactura != nil {
		boton(t.t("action.open"), ui.Secundario, func() { t.AlVerFactura(f.ID) })
	}
	boton(t.t("pos.newSale"), ui.Secundario, t.NuevaVenta)
	return l.objeto(y - 1*tema.Rem + 2*tema.Rem)
}

// cajaSuelta es una `ui.Caja` para colocarla en un lienzo con el resto.
func cajaSuelta(c *ui.Caja) fyne.CanvasObject { return container.New(&soloCaja{c}, c.Objetos()...) }

// parrafoNombreTPV es el nombre de un producto: parte palabras largas, como el
// `overflow-wrap: anywhere` de la hoja.
func parrafoNombreTPV(s string) *ui.Parrafo {
	p := ui.NuevoParrafoConTinta(s, escalonNombreTPV, ui.Medio, sinColor)
	p.PartirPalabras = true
	return p
}

// --- La tesela de un producto (`.erp-tpv-product`) ---------------------------------

// TeselaTPV es un producto del catálogo: su código, su nombre y, abajo, el precio y las
// unidades en la cesta (o «+»). Pulsarla añade una unidad. Con unidades se rellena del
// tono sólido de su categoría.
type TeselaTPV struct {
	ui.Pulsable

	Codigo, NombreProd, Precio string
	Cantidad                   int64
	Tono                       ui.TonoTarjeta
	// Movil es el `padding: 0.875rem` y el `min-height: 9.75rem` del teléfono.
	Movil bool
	// Imagen es la del producto, si la tiene (`.erp-tpv-product-with-image`): cubre la
	// tesela y el nombre y el precio van en una etiqueta opaca abajo.
	Imagen image.Image
}

// Medidas de la etiqueta sobre la imagen: el `padding: 0.5rem 0.75rem` y el
// `margin-inline: -0.5rem; margin-bottom: -0.5rem` de `.erp-tpv-product-info`.
const (
	EtiquetaTPVPY     float32 = 0.5 * tema.Rem
	EtiquetaTPVPX     float32 = 0.75 * tema.Rem
	EtiquetaTPVMargen float32 = 0.5 * tema.Rem
)

var _ fyne.Widget = (*TeselaTPV)(nil)

// NuevaTeselaTPV crea la tesela del producto.
func NuevaTeselaTPV(p Registro, cantidad int64, alPulsar func()) *TeselaTPV {
	codigo, _ := p.Data["codigo"].(string)
	if codigo == "" {
		codigo = categoriaTPV(p)
	}
	if codigo == "" {
		codigo = SinValor
	}
	te := &TeselaTPV{Codigo: codigo, NombreProd: NombreDe(p), Cantidad: cantidad}
	te.Nombre, te.AlPulsar = te.NombreProd, alPulsar
	te.ExtendBaseWidget(te)
	return te
}

func (te *TeselaTPV) pad() float32 {
	if te.Movil {
		return 0.875 * tema.Rem
	}
	return TeselaTPVPad
}

func (te *TeselaTPV) minimo() float32 {
	if te.Movil {
		return 9.75 * tema.Rem
	}
	return TeselaTPVAlto
}

// AltoPara es la altura con ese ancho: el nombre se reparte en líneas.
func (te *TeselaTPV) AltoPara(ancho float32) float32 {
	dentro := max(ancho-2*te.pad(), 0)
	par := parrafoNombreTPV(te.NombreProd)
	if te.Imagen != nil {
		// Con imagen el nombre va en la etiqueta, sin su margen de arriba.
		alto := 2*te.pad() + tema.TextoXS.Linea + TeselaTPVNombreMY + te.altoEtiqueta(dentro) - EtiquetaTPVMargen
		return max(alto, te.minimo())
	}
	alto := 2*te.pad() + tema.TextoXS.Linea + 2*TeselaTPVNombreMY + ui.AltoParrafo(par, dentro) + te.altoAbajo(dentro)
	return max(alto, te.minimo())
}

// anchoEtiqueta es el de dentro de la etiqueta sobre la imagen.
func anchoEtiqueta(dentro float32) float32 {
	return max(dentro+2*EtiquetaTPVMargen-2*EtiquetaTPVPX, 0)
}

// altoEtiqueta es el de la etiqueta: el nombre, su `margin-bottom: 0.5rem` y la fila
// del precio, con su relleno.
func (te *TeselaTPV) altoEtiqueta(dentro float32) float32 {
	w := anchoEtiqueta(dentro)
	return 2*EtiquetaTPVPY + ui.AltoParrafo(parrafoNombreTPV(te.NombreProd), w) + 0.5*tema.Rem + te.altoAbajo(w)
}

// altoAbajo es el de la fila del precio y la cuenta: «Configura precio e IVA» se
// reparte en líneas al lado del cuadro, como el `<strong>` de la web.
func (te *TeselaTPV) altoAbajo(dentro float32) float32 {
	return max(LadoCuentaTPV, ui.AltoParrafo(parrafoPrecioTPV(te.Precio), anchoPrecioTPV(dentro)))
}

func anchoPrecioTPV(dentro float32) float32 { return max(dentro-LadoCuentaTPV-HuecoDos, 0) }

func parrafoPrecioTPV(s string) *ui.Parrafo {
	return ui.NuevoParrafoConTinta(s, escalonPrecioTPV, ui.SemiNegrita, sinColor)
}

// MinSize es el mínimo de la hoja; el alto real lo da la fila.
func (te *TeselaTPV) MinSize() fyne.Size {
	te.ExtendBaseWidget(te)
	return fyne.NewSize(TeselaTPVMin, te.minimo())
}

// colores son el fondo, el borde y la tinta del estado de ahora.
func (te *TeselaTPV) colores(v tema.Variante) (fondo, borde, tinta, apagada color.NRGBA) {
	tono := string(te.Tono)
	if tono == "" {
		tono = string(ui.TarjetaAzul)
	}
	fondo, borde = tema.Color("card-"+tono, v), tema.Color("border-subtle", v)
	tinta, apagada = tema.Color("text", v), tema.Color("text-muted", v)
	if te.Cantidad > 0 && te.Imagen == nil {
		fondo = tema.Color("card-solid-"+tono, v)
		tinta = tema.Color("on-card-solid-"+tono, v)
		apagada = tinta
	}
	if te.Encima() && !te.Inerte {
		borde = tema.Color("primary", v)
	}
	if te.Inerte {
		// `opacity: 0.55`.
		fondo, borde, tinta, apagada = ui.Atenuar(fondo, 0.55), ui.Atenuar(borde, 0.55), ui.Atenuar(tinta, 0.55), ui.Atenuar(apagada, 0.55)
	}
	return fondo, borde, tinta, apagada
}

// CreateRenderer construye la tesela.
func (te *TeselaTPV) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &teselaTPVRenderer{
		te:     te,
		aro:    ui.NuevoAro(tema.RadioLG, ui.SinHueco, v),
		caja:   ui.NuevaCaja(sinColor, sinColor, tema.RadioLG),
		codigo: ui.Texto(te.Codigo, tema.TextoXS, ui.Normal, sinColor),
		nombre: parrafoNombreTPV(te.NombreProd),
		precio: parrafoPrecioTPV(te.Precio),
		cuenta: ui.NuevaCaja(tema.Color("primary", v), sinColor, tema.RadioSM),
	}
	cifra := "+"
	if te.Cantidad > 0 {
		cifra = strconv.FormatInt(te.Cantidad, 10)
	}
	r.cifra = ui.Texto(cifra, escalonCuentaTPV, ui.SemiNegrita, tema.Color("on-primary", v))
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	if te.Imagen != nil {
		// La foto cubre la tesela (`object-fit: cover`); el código y la etiqueta van sobre
		// una superficie casi opaca para que se lean sobre cualquier foto.
		r.foto = canvas.NewImageFromImage(te.Imagen)
		r.foto.FillMode, r.foto.CornerRadius = canvas.ImageFillCover, tema.RadioLG
		r.fondoCodigo = canvas.NewRectangle(sinColor)
		r.fondoCodigo.CornerRadius = tema.RadioSM
		r.fondoEtiqueta = canvas.NewRectangle(sinColor)
		r.fondoEtiqueta.CornerRadius = tema.RadioSM
		r.objetos = append(r.objetos, r.foto, r.fondoCodigo, r.fondoEtiqueta)
	}
	r.objetos = append(r.objetos, r.codigo, r.nombre, r.precio)
	r.objetos = append(r.objetos, r.cuenta.Objetos()...)
	r.objetos = append(r.objetos, r.cifra)
	r.Refresh()
	return r
}

type teselaTPVRenderer struct {
	te             *TeselaTPV
	aro            *ui.Aro
	caja, cuenta   *ui.Caja
	codigo, cifra  *canvas.Text
	nombre, precio *ui.Parrafo
	// foto y los dos fondos solo con imagen.
	foto                       *canvas.Image
	fondoCodigo, fondoEtiqueta *canvas.Rectangle
	objetos                    []fyne.CanvasObject
}

func (r *teselaTPVRenderer) MinSize() fyne.Size           { return r.te.MinSize() }
func (r *teselaTPVRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *teselaTPVRenderer) Destroy()                     {}

func (r *teselaTPVRenderer) Layout(t fyne.Size) {
	pad := r.te.pad()
	dentro := max(t.Width-2*pad, 0)
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	if r.foto != nil {
		r.layoutConFoto(t, pad, dentro)
		return
	}
	ui.Truncar(r.codigo, r.te.Codigo, tema.TextoXS, ui.Normal, dentro)
	ui.ColocarTexto(r.codigo, tema.TextoXS, pad, pad)
	y := pad + tema.TextoXS.Linea + TeselaTPVNombreMY
	hn := ui.AltoParrafo(r.nombre, dentro)
	r.nombre.Move(fyne.NewPos(pad, y))
	r.nombre.Resize(fyne.NewSize(dentro, hn))
	// El precio y la cuenta, pegados abajo (`margin-block-start: auto`).
	abajo := t.Height - pad - LadoCuentaTPV
	ui.MoverCaja(r.cuenta, fyne.NewPos(t.Width-pad-LadoCuentaTPV, abajo), fyne.NewSize(LadoCuentaTPV, LadoCuentaTPV))
	wc := ui.Medir(r.cifra.Text, escalonCuentaTPV, ui.SemiNegrita).Width
	ui.ColocarTexto(r.cifra, escalonCuentaTPV, t.Width-pad-LadoCuentaTPV+(LadoCuentaTPV-wc)/2, abajo+(LadoCuentaTPV-escalonCuentaTPV.Linea)/2)
	// `align-items: center` con el cuadro; si el precio parte en líneas, sube.
	wp := anchoPrecioTPV(dentro)
	hp := ui.AltoParrafo(r.precio, wp)
	r.precio.Move(fyne.NewPos(pad, t.Height-pad-max(hp, LadoCuentaTPV)+max(LadoCuentaTPV-hp, 0)/2))
	r.precio.Resize(fyne.NewSize(wp, hp))
}

// layoutConFoto es `.erp-tpv-product-with-image`: la foto de borde a borde, el código
// arriba sobre su fondo y la etiqueta con el nombre, el precio y la cuenta, pegada abajo.
func (r *teselaTPVRenderer) layoutConFoto(t fyne.Size, pad, dentro float32) {
	r.foto.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.foto.Resize(fyne.NewSize(max(t.Width-2*tema.Borde, 0), max(t.Height-2*tema.Borde, 0)))
	ui.Truncar(r.codigo, r.te.Codigo, tema.TextoXS, ui.Normal, dentro)
	wc := min(ui.Medir(r.codigo.Text, tema.TextoXS, ui.Normal).Width, dentro)
	r.fondoCodigo.Move(fyne.NewPos(pad-0.25*tema.Rem, pad-0.125*tema.Rem))
	r.fondoCodigo.Resize(fyne.NewSize(wc+0.5*tema.Rem, tema.TextoXS.Linea+0.25*tema.Rem))
	ui.ColocarTexto(r.codigo, tema.TextoXS, pad, pad)
	// La etiqueta: un poco más ancha que el contenido y bajando medio rem del relleno.
	alto := r.te.altoEtiqueta(dentro)
	x0, y0 := pad-EtiquetaTPVMargen, t.Height-pad+EtiquetaTPVMargen-alto
	r.fondoEtiqueta.Move(fyne.NewPos(x0, y0))
	r.fondoEtiqueta.Resize(fyne.NewSize(dentro+2*EtiquetaTPVMargen, alto))
	w := anchoEtiqueta(dentro)
	x, y := x0+EtiquetaTPVPX, y0+EtiquetaTPVPY
	hn := ui.AltoParrafo(r.nombre, w)
	r.nombre.Move(fyne.NewPos(x, y))
	r.nombre.Resize(fyne.NewSize(w, hn))
	abajo := y0 + alto - EtiquetaTPVPY - LadoCuentaTPV
	ui.MoverCaja(r.cuenta, fyne.NewPos(x+w-LadoCuentaTPV, abajo), fyne.NewSize(LadoCuentaTPV, LadoCuentaTPV))
	wci := ui.Medir(r.cifra.Text, escalonCuentaTPV, ui.SemiNegrita).Width
	ui.ColocarTexto(r.cifra, escalonCuentaTPV, x+w-LadoCuentaTPV+(LadoCuentaTPV-wci)/2, abajo+(LadoCuentaTPV-escalonCuentaTPV.Linea)/2)
	wp := anchoPrecioTPV(w)
	hp := ui.AltoParrafo(r.precio, wp)
	r.precio.Move(fyne.NewPos(x, y0+alto-EtiquetaTPVPY-max(hp, LadoCuentaTPV)+max(LadoCuentaTPV-hp, 0)/2))
	r.precio.Resize(fyne.NewSize(wp, hp))
}

func (r *teselaTPVRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta, apagada := r.te.colores(v)
	if r.foto != nil {
		// Sobre la foto, la tinta de siempre en la superficie elevada al 96 %.
		sobre := tema.Color("surface-raised", v)
		sobre.A = 245
		r.fondoCodigo.FillColor, r.fondoEtiqueta.FillColor = sobre, sobre
		r.fondoCodigo.Refresh()
		r.fondoEtiqueta.Refresh()
		tinta, apagada = tema.Color("text", v), tema.Color("text-muted", v)
		if r.te.Inerte {
			tinta, apagada = ui.Atenuar(tinta, 0.55), ui.Atenuar(apagada, 0.55)
		}
		r.foto.Translucency = 0
		if r.te.Inerte {
			r.foto.Translucency = 0.45
		}
		r.foto.Refresh()
	}
	r.aro.Ver(r.te.Enfocado())
	r.aro.Pintar(v)
	r.caja.Pintar(fondo, borde)
	r.codigo.Color, r.precio.Tinta, r.nombre.Tinta = apagada, tinta, tinta
	r.nombre.Refresh()
	r.codigo.Refresh()
	r.precio.Refresh()
	r.Layout(r.te.Size())
}

// --- Con el servicio --------------------------------------------------------------

// VeTPV es la condición de la web para enseñar el punto de venta: el módulo encendido y
// lectura en él, en las facturas y en los productos.
func (p *Pantalla) VeTPV() bool {
	return slices.Contains(p.Catalogo.Activos, "tpv") && p.PuedeLeer("tpv") &&
		p.PuedeLeer("facturacion") && p.PuedeLeer("productos")
}

// objetoTPV pone lo que depende de los permisos y pinta.
func (p *Pantalla) objetoTPV(v tema.Variante) fyne.CanvasObject {
	t := p.tpv
	t.Ventana = p.Ventana.Width
	// Cobrar crea, asienta y cobra una factura: hacen falta los tres verbos.
	t.SoloLectura = !p.PuedeAccion("tpv", "crear") || slices.ContainsFunc([]string{"crear", "asentar", "pagar"}, func(verbo string) bool {
		return !p.PuedeAccion("facturacion", verbo)
	})
	t.PuedeCrearProducto = p.PuedeCrear("producto") && p.lienzo != nil
	// `altas.puedeCrear('presupuesto')` y `puedeAccion('productos', 'escribir')` de la web;
	// subir imágenes pide además un selector de ficheros.
	t.PuedePresupuesto = p.PuedeCrear(ModeloPresupuesto)
	t.PuedeEditarImagenes = p.PuedeAccion("productos", "escribir") && p.ElegirFicheros != nil
	return t.Objeto(v)
}

// engancharTPV conecta el punto de venta con el servicio.
func (p *Pantalla) engancharTPV() {
	t := p.tpv
	t.AlCambio, t.Buscar = p.Repintar, p.buscarRelacion
	t.AlBuscar = p.buscarTPV
	t.AlMas = p.masTPV
	t.AlAbrirCaja = p.abrirCajaTPV
	t.AlCobrar = p.cobrarTPV
	t.AlVerFactura = func(id string) { p.abrirRegistro(api.ERPFacturaVenta, id) }
	t.AlImprimir = func(f Registro) {
		p.descargar(&Ficha{Modelo: Modelo{Clave: api.ERPFacturaVenta}, Registro: &f}, "imprimir")
	}
	t.AlCatalogo = func() {
		if m, ok := p.modeloPorClave("producto"); ok {
			p.Abrir(m)
		}
	}
	p.engancharTPVExtras()
	t.AlNuevoProducto = func() {
		if m, ok := p.modeloPorClave("producto"); ok {
			p.AltaRapida(m, nil, nil, func(Registro) { p.cargarTPV() })
		}
	}
}

// RetardoBusquedaTPV es el de la web antes de pedir lo tecleado.
const RetardoBusquedaTPV = 180 * time.Millisecond

// buscarTPV pide el catálogo con lo tecleado, cuando se deja de teclear.
func (p *Pantalla) buscarTPV(string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.espera != nil {
		p.espera.Stop()
	}
	p.espera = time.AfterFunc(RetardoBusquedaTPV, func() { enHilo(p.cargarTPV) })
}

// cargarTPV pide la primera página del catálogo (los productos activos) y las cajas
// abiertas. Si la elegida ya no está abierta, se elige la primera.
func (p *Pantalla) cargarTPV() {
	if p.svc == nil {
		return
	}
	t := p.tpv
	q := t.Busqueda
	// El cobro que quedó sin confirmar al cerrar la app vuelve en cuanto se entra.
	t.Pendientes = p.CobrosTPV
	if !t.restaurado {
		t.Restaurar()
	}
	// Como la web, pedir de nuevo se lleva el aviso de antes: un fallo de carga no se
	// queda pegado encima del catálogo cuando la red ya volvió.
	t.Cargando, t.Fallo = true, ""
	p.Repintar()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		prod, err := p.svc.ListarCon(ctx, "producto", api.ERPFiltro{Q: q, Estado: "activa", Limite: PaginaTPV})
		var ses api.ERPPagina
		if err == nil {
			ses, err = p.svc.ListarCon(ctx, api.ERPModeloSesionTPV, api.ERPFiltro{Estado: "abierto", Limite: 100})
		}
		enHilo(func() {
			if t.Busqueda != q {
				return // ya se pidió otra búsqueda
			}
			t.Cargando = false
			if err != nil {
				t.Fallo = MensajeDeError(err, p.tr)
				p.Repintar()
				return
			}
			t.Productos, t.Siguiente, t.Sesiones = prod.Registros, prod.Siguiente, ses.Registros
			if !slices.ContainsFunc(t.Sesiones, func(r Registro) bool { return r.ID == t.Sesion }) && t.pendiente == nil {
				t.Sesion = ""
				if len(t.Sesiones) > 0 {
					t.Sesion = t.Sesiones[0].ID
				}
			}
			p.Repintar()
			p.cargarImagenesTPV()
		})
	})
}

// masTPV añade la página siguiente del catálogo detrás de la que hay.
func (p *Pantalla) masTPV() {
	t := p.tpv
	if p.svc == nil || t.Cargando || t.Siguiente == "" {
		return
	}
	q, cursor := t.Busqueda, t.Siguiente
	t.Cargando = true
	p.Repintar()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		pag, err := p.svc.ListarCon(ctx, "producto", api.ERPFiltro{Q: q, Estado: "activa", Limite: PaginaTPV, Cursor: cursor})
		enHilo(func() {
			t.Cargando = false
			if err != nil {
				t.Fallo = MensajeDeError(err, p.tr)
			} else if t.Busqueda == q {
				t.Productos, t.Siguiente = append(t.Productos, pag.Registros...), pag.Siguiente
				defer p.cargarImagenesTPV()
			}
			p.Repintar()
		})
	})
}

// abrirCajaTPV abre una sesión de caja a nombre de quien vende, y la elige.
func (p *Pantalla) abrirCajaTPV(caja string) {
	t := p.tpv
	if p.svc == nil || t.Ocupado {
		return
	}
	t.Ocupado, t.Fallo = true, ""
	p.Repintar()
	// El `user.name || user.email` de la web: el nombre de la portada, o el `sub`.
	vendedor := p.inicio.Nombre
	if vendedor == "" {
		vendedor = p.yo()
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		s, err := p.svc.Crear(ctx, api.ERPModeloSesionTPV, map[string]any{
			"caja": caja, "vendedor": vendedor, "fecha_inicio": time.Now().UTC().Format(time.RFC3339),
			"efectivo": 0, "tarjeta": 0, "total": 0,
		})
		enHilo(func() {
			t.Ocupado = false
			if err != nil {
				t.Fallo = MensajeDeError(err, p.tr)
			} else {
				t.Sesiones = append(t.Sesiones, s)
				t.Sesion = s.ID
				t.caja.PonerTexto("")
			}
			p.Repintar()
		})
	})
}

// cobrarTPV envía el cobro. Lo que haya pasado lo decide TPV.Cobrado; una venta hecha
// deja viejas las listas de facturas en caché.
func (p *Pantalla) cobrarTPV(c api.ERPCobroTPV) {
	t := p.tpv
	if p.svc == nil {
		t.Cobrado(api.ERPVentaTPV{}, errSinServicio, p.t("error.generic"))
		p.Repintar()
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		v, err := p.svc.CobrarTPV(ctx, c)
		enHilo(func() {
			t.Cobrado(v, err, MensajeDeError(err, p.tr))
			if err == nil {
				p.revision++
				p.olvidar(api.ERPFacturaVenta)
			}
			p.Repintar()
		})
	})
}

// errSinServicio: sin servicio no se envió nada, y lo que no se envió no está incierto.
var errSinServicio = &api.Error{Status: http.StatusUnprocessableEntity}
