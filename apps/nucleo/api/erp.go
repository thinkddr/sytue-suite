// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ERP es el cliente de Sytue ERP (services/syerp, `/api/v1`). El servicio es UNA API para
// todos sus modelos —contactos, facturas, asientos…— y se describe a sí mismo: el catálogo
// dice qué modelos hay, con qué campos y de qué tipo, YA TRADUCIDO al idioma que se pida.
type ERP struct {
	c      *Client
	idioma string
}

// NewERP crea el cliente sobre la raíz del servicio (ServiceURL(apex, "erp")). `idioma` va
// en Accept-Language: es el de los rótulos del catálogo.
func NewERP(servicio string, s Session, h *http.Client, idioma string) *ERP {
	return &ERP{c: New(servicio+"/api/v1", s, h), idioma: idioma}
}

// Los tipos de campo que el formateo distingue; el resto se enseña tal cual.
const (
	ERPDinero     = "dinero" // en CÉNTIMOS
	ERPFecha      = "fecha"
	ERPFechaHora  = "fechahora"
	ERPBooleano   = "booleano"
	ERPSeleccion  = "seleccion"
	ERPRelacion   = "relacion"
	ERPEntero     = "entero"
	ERPDecimal    = "decimal" // cadena canónica, no flotante
	ERPTextoLargo = "texto_largo"
)

// ERPOpcion es un valor de una selección, o un estado, con su rótulo traducido.
type ERPOpcion struct {
	Valor    string `json:"valor"`
	Etiqueta string `json:"etiqueta"`
}

// ERPCampo es un campo de un modelo.
type ERPCampo struct {
	Clave     string `json:"clave"`
	Tipo      string `json:"tipo"`
	Etiqueta  string `json:"etiqueta"`
	Requerido bool   `json:"requerido,omitempty"`
	// Modelo es el modelo apuntado, solo en las relaciones.
	Modelo   string      `json:"modelo,omitempty"`
	Opciones []ERPOpcion `json:"opciones,omitempty"`
}

// ERPModelo es un tipo de registro: sus campos, cuáles salen en la lista y sus estados.
type ERPModelo struct {
	Clave    string `json:"clave"`
	Modulo   string `json:"modulo"`
	Etiqueta string `json:"etiqueta"`
	// Numerado: el documento lleva referencia (FV-2026-00042) que pone el servicio.
	Numerado bool `json:"numerado,omitempty"`
	// Contable: al confirmarse genera asiento. Lo que ya está asentado no se retoca
	// a la ligera, y esta vista lo dice antes de guardar.
	Contable bool `json:"contable,omitempty"`
	// Cobro: el documento se EMITE (una factura de cliente). Es lo que decide si se puede
	// sacar como factura electrónica, en Facturae o en UBL.
	Cobro bool `json:"cobro,omitempty"`
	// Recibe: el documento llega de un proveedor y entra desde su factura electrónica.
	Recibe  bool        `json:"recibe,omitempty"`
	Campos  []ERPCampo  `json:"campos"`
	Lista   []string    `json:"lista"`
	Estados []ERPOpcion `json:"estados,omitempty"`
}

// ERPModulo agrupa modelos: es una sección del menú.
type ERPModulo struct {
	Clave    string `json:"clave"`
	Etiqueta string `json:"etiqueta"`
	// Dependencias son los módulos que se encienden con este.
	Dependencias []string    `json:"dependencias,omitempty"`
	Modelos      []ERPModelo `json:"modelos"`
}

// ERPCatalogo es lo que el ERP sabe hacer. Activos son los módulos que la organización
// ha encendido; vacío es que aún no ha elegido ninguno.
type ERPCatalogo struct {
	Modulos []ERPModulo `json:"modulos"`
	Activos []string    `json:"activos"`
}

// ERPRegistro es un registro de cualquier modelo. Data lleva sus campos por clave.
type ERPRegistro struct {
	ID        string         `json:"id"`
	Modelo    string         `json:"modelo"`
	Ref       string         `json:"ref,omitempty"`
	Etiqueta  string         `json:"etiqueta"`
	Estado    string         `json:"estado,omitempty"`
	Data      map[string]any `json:"data"`
	CreadoEn  time.Time      `json:"creado_en"`
	EditadoEn time.Time      `json:"editado_en"`
}

// ERPPagina es una página de una lista. Etiquetas resuelve los identificadores de las
// relaciones de ESTA página a su nombre, de una vez: sin ello, cada fila sería otra consulta.
type ERPPagina struct {
	Registros []ERPRegistro     `json:"registros"`
	Etiquetas map[string]string `json:"etiquetas,omitempty"`
	Siguiente string            `json:"siguiente,omitempty"`
}

func (e *ERP) pedir(ruta string, q url.Values) Request {
	return Request{Path: ruta, Query: q, Header: http.Header{"Accept-Language": {e.idioma}}}
}

// Catalogo devuelve los módulos y modelos, con los rótulos en el idioma del cliente.
func (e *ERP) Catalogo(ctx context.Context) (ERPCatalogo, error) {
	return JSON[ERPCatalogo](ctx, e.c, e.pedir("/catalogo", nil))
}

// Listar devuelve una página de registros de un modelo. q filtra por texto; cursor vacío
// es la primera página.
func (e *ERP) Listar(ctx context.Context, modelo, q, cursor string, limite int) (ERPPagina, error) {
	return e.ListarCon(ctx, modelo, ERPFiltro{Q: q, Cursor: cursor, Limite: limite})
}

// ERPFiltro es todo lo que acota una lista, con el sentido que le da el servicio
// (`filtroDe` en services/syerp/internal/http/handlers.go).
type ERPFiltro struct {
	// Q busca por texto; Estado es uno de los que declara el modelo.
	Q, Estado string
	// Orden es la clave de una columna; vacío es el de siempre (lo último, arriba), que
	// es el único que va por índice. Asc solo vale con Orden puesto.
	Orden string
	Asc   bool
	// Papelera lista lo borrado en vez de lo vivo.
	Papelera bool
	// Cursor es el `siguiente` de la página anterior; vacío, la primera.
	Cursor string
	Limite int
}

// ListarCon devuelve una página de registros de un modelo con esos filtros.
func (e *ERP) ListarCon(ctx context.Context, modelo string, f ERPFiltro) (ERPPagina, error) {
	// «limite», en español: es el nombre que lee el servicio. El escritorio de Fyne mandaba
	// `limit` y el servicio lo ignoraba sin decir nada.
	consulta := url.Values{"limite": {strconv.Itoa(f.Limite)}}
	for clave, valor := range map[string]string{"q": f.Q, "estado": f.Estado, "orden": f.Orden, "cursor": f.Cursor} {
		if valor != "" {
			consulta.Set(clave, valor)
		}
	}
	// El servicio solo entiende «asc»: sin él, descendente.
	if f.Orden != "" && f.Asc {
		consulta.Set("dir", "asc")
	}
	if f.Papelera {
		consulta.Set("papelera", "1")
	}
	return JSON[ERPPagina](ctx, e.c, e.pedir("/"+url.PathEscape(modelo)+"/", consulta))
}

// ERPEstadisticas es el recuento de un modelo en el servidor: el total, por estado y, si
// el modelo tiene importe, la suma en CÉNTIMOS. Es lo que lleva la cabecera de la lista y
// los números de las píldoras, y no se puede contar en el cliente: la lista es una página.
type ERPEstadisticas struct {
	Total      int            `json:"total"`
	PorEstado  map[string]int `json:"por_estado"`
	SumaDinero int64          `json:"suma_dinero,omitempty"`
}

// Estadisticas pide el recuento de un modelo (`GET /api/v1/{modelo}/estadisticas`).
func (e *ERP) Estadisticas(ctx context.Context, modelo string) (ERPEstadisticas, error) {
	return JSON[ERPEstadisticas](ctx, e.c, e.pedir("/"+url.PathEscape(modelo)+"/estadisticas", nil))
}

// ERPPermisos es lo que quien pide puede hacer en cada módulo del ERP: uno de los
// cuatro niveles de `services/syerp/internal/permisos` (ninguno, lectura, usuario,
// administrador). Los decide el SERVICIO, y esta vista solo enseña lo que concedan:
// un cliente no reparte permisos ni decide quién aprueba.
type ERPPermisos struct {
	Administrador bool              `json:"administrador"`
	Modulos       map[string]string `json:"modulos"`
	// Verbos son los gestos efectivos resueltos por el servicio, incluida la
	// segregación de funciones que limita también a administradores.
	Verbos map[string][]string `json:"verbos"`
	// Pedidos son los módulos en los que ya ha pedido editar (la franja lo dice).
	Pedidos []string `json:"pedidos,omitempty"`
}

// Peticiones es la ruta de «pedir acceso de edición» de un módulo.
func (e *ERP) Peticiones(modulo string) Peticiones {
	return NuevasPeticiones(e.c, "/modulos/"+url.PathEscape(modulo)+"/edit-requests")
}

// Los cuatro niveles, como los nombra el servicio.
const (
	ERPNinguno       = "ninguno"
	ERPLectura       = "lectura"
	ERPUsuario       = "usuario"
	ERPAdministrador = "administrador"
)

// Alcanza dice si el nivel en `modulo` llega a `nivel`. Un módulo sin nivel puesto se
// trata como cerrado: lo que el servicio no ha concedido, no se ofrece.
func (p ERPPermisos) Alcanza(modulo, nivel string) bool {
	orden := func(n string) int {
		return slices.Index([]string{ERPNinguno, ERPLectura, ERPUsuario, ERPAdministrador}, n)
	}
	return orden(p.Modulos[modulo]) >= orden(nivel) && orden(nivel) >= 0
}

// PuedeAccion exige tanto escritura del módulo como el verbo concedido. No
// infiere gestos del nivel: una lista vacía o ausente se trata como cerrada.
func (p ERPPermisos) PuedeAccion(modulo, verbo string) bool {
	return p.Alcanza(modulo, ERPUsuario) && slices.Contains(p.Verbos[modulo], verbo)
}

// MisPermisos pregunta qué puede hacer quien ha entrado, módulo a módulo.
func (e *ERP) MisPermisos(ctx context.Context) (ERPPermisos, error) {
	return JSON[ERPPermisos](ctx, e.c, e.pedir("/mis-permisos", nil))
}

// Obtener devuelve UN registro entero. La lista trae los campos, pero una ficha que se
// va a editar se relee: entre que se listó y se abre pudo cambiar.
func (e *ERP) Obtener(ctx context.Context, modelo, id string) (ERPRegistro, error) {
	return JSON[ERPRegistro](ctx, e.c, e.pedir("/"+url.PathEscape(modelo)+"/"+url.PathEscape(id), nil))
}

// cuerpoDatos es lo que esperan Crear y Actualizar: `{"data": {...}}`.
type cuerpoDatos struct {
	Data map[string]any `json:"data"`
}

// Crear inserta un registro. `data` va con los valores YA en la forma que el servicio
// valida: el dinero en céntimos enteros, los decimales como cadena canónica, las fechas
// en AAAA-MM-DD y las fechas con hora en RFC3339 (services/syerp/internal/domain).
func (e *ERP) Crear(ctx context.Context, modelo string, data map[string]any) (ERPRegistro, error) {
	return e.escribir(ctx, http.MethodPost, "/"+url.PathEscape(modelo)+"/", cuerpoDatos{Data: data})
}

// Actualizar REEMPLAZA los datos del registro: el servicio valida el cuerpo entero
// contra el modelo, así que `data` lleva todos los campos, no solo los que cambian.
func (e *ERP) Actualizar(ctx context.Context, modelo, id string, data map[string]any) (ERPRegistro, error) {
	return e.escribir(ctx, http.MethodPut, "/"+url.PathEscape(modelo)+"/"+url.PathEscape(id), cuerpoDatos{Data: data})
}

// CambiarEstado mueve el documento dentro del flujo que declara su modelo. El servicio
// rechaza un estado que no esté declarado: el cliente propone, no decide.
func (e *ERP) CambiarEstado(ctx context.Context, modelo, id, estado string) (ERPRegistro, error) {
	return e.escribir(ctx, http.MethodPost, "/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/estado",
		struct {
			Estado string `json:"estado"`
		}{estado})
}

// Borrar manda el registro a la PAPELERA (`DELETE`): no lo destruye, y Restaurar lo
// devuelve. Aun así, desde fuera de la papelera parece definitivo, así que quien llame
// tiene que haber preguntado antes.
func (e *ERP) Borrar(ctx context.Context, modelo, id string) error {
	return e.sinCuerpo(ctx, http.MethodDelete, "/"+url.PathEscape(modelo)+"/"+url.PathEscape(id), nil)
}

// Restaurar saca el registro de la papelera.
func (e *ERP) Restaurar(ctx context.Context, modelo, id string) error {
	return e.sinCuerpo(ctx, http.MethodPost, "/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/restaurar", []byte("{}"))
}

// Duplicar clona el registro como BORRADOR con referencia nueva, en el servidor: la
// factura de este mes es la del anterior con otra fecha. Devuelve la copia.
func (e *ERP) Duplicar(ctx context.Context, modelo, id string) (ERPRegistro, error) {
	return e.escribir(ctx, http.MethodPost, "/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/duplicar", struct{}{})
}

func (e *ERP) sinCuerpo(ctx context.Context, metodo, ruta string, cuerpo []byte) error {
	r := e.pedir(ruta, nil)
	r.Method, r.Body = metodo, cuerpo
	if cuerpo != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.c.Do(ctx, r)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Asentar lleva un documento contable al libro diario y devuelve la referencia del
// asiento. Es de IDA SOLA: el asiento es inmutable y solo se deshace con otro que lo
// anule. La web no pregunta antes (el botón dice «Llevar al libro»), y aquí tampoco.
func (e *ERP) Asentar(ctx context.Context, modelo, id string) (string, error) {
	r := e.pedir("/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/asentar", nil)
	r.Method, r.Body = http.MethodPost, []byte("{}")
	r.Header.Set("Content-Type", "application/json")
	a, err := JSON[struct {
		Ref string `json:"ref"`
	}](ctx, e.c, r)
	return a.Ref, err
}

// Exportar baja la lista en CSV con esos filtros (`GET /api/v1/{modelo}/export`), o solo
// las filas de `ids` si los hay, y el nombre con que la nombra el servicio.
//
// Quien llama cierra el lector.
func (e *ERP) Exportar(ctx context.Context, modelo, q, estado string, ids []string) (io.ReadCloser, string, error) {
	consulta := url.Values{}
	if len(ids) > 0 {
		consulta.Set("ids", strings.Join(ids, ","))
	} else {
		for clave, valor := range map[string]string{"q": q, "estado": estado} {
			if valor != "" {
				consulta.Set(clave, valor)
			}
		}
	}
	resp, err := e.c.Do(ctx, e.pedir("/"+url.PathEscape(modelo)+"/export", consulta))
	if err != nil {
		return nil, "", err
	}
	return resp.Body, nombreAdjunto(resp.Header.Get("Content-Disposition"), modelo+".csv"), nil
}

func (e *ERP) escribir(ctx context.Context, metodo, ruta string, cuerpo any) (ERPRegistro, error) {
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return ERPRegistro{}, err
	}
	r := e.pedir(ruta, nil)
	r.Method, r.Body = metodo, datos
	r.Header.Set("Content-Type", "application/json")
	return JSON[ERPRegistro](ctx, e.c, r)
}

// --- La factura de cara afuera: lo que se imprime, lo que se manda y lo que exige la ley ---

// ERPFacturaVenta es la clave del modelo de las facturas emitidas. El servicio la mira
// por su nombre para imprimir y para VERI*FACTU (`services/syerp/internal/http/
// imprimir.go` y `verifactu.go`): solo una factura de cliente se imprime y solo ella se
// registra en la Agencia Tributaria.
const ERPFacturaVenta = "factura_venta"

// Imprimir devuelve la factura para el cliente: una página lista para imprimir, con su
// QR tributario y la leyenda de su régimen de IVA
// (`GET /api/v1/{modelo}/{id}/imprimir`). El servicio la sirve como HTML —no como PDF—,
// así que se guarda con extensión .html y se imprime desde el navegador.
//
// Quien llama cierra el lector.
func (e *ERP) Imprimir(ctx context.Context, modelo, id string) (io.ReadCloser, error) {
	return e.documento(ctx, modelo, id, "/imprimir")
}

// Facturae devuelve la factura emitida como Facturae 3.2.2 SIN FIRMAR
// (`GET /api/v1/{modelo}/{id}/facturae`), y el nombre con que la nombra el servicio.
func (e *ERP) Facturae(ctx context.Context, modelo, id string) (io.ReadCloser, string, error) {
	return e.documentoConNombre(ctx, modelo, id, "/facturae", "facturae.xml")
}

// UBL devuelve la factura emitida como XML UBL 2.1 —lo que entiende cualquier otro ERP—
// (`GET /api/v1/{modelo}/{id}/ubl`), y el nombre con que la nombra el servicio.
func (e *ERP) UBL(ctx context.Context, modelo, id string) (io.ReadCloser, string, error) {
	return e.documentoConNombre(ctx, modelo, id, "/ubl", "factura.xml")
}

func (e *ERP) documento(ctx context.Context, modelo, id, cola string) (io.ReadCloser, error) {
	resp, err := e.c.Do(ctx, e.pedir("/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+cola, nil))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (e *ERP) documentoConNombre(ctx context.Context, modelo, id, cola, respaldo string) (io.ReadCloser, string, error) {
	resp, err := e.c.Do(ctx, e.pedir("/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+cola, nil))
	if err != nil {
		return nil, "", err
	}
	return resp.Body, nombreAdjunto(resp.Header.Get("Content-Disposition"), respaldo), nil
}

// Los estados de un registro de facturación, tal y como los nombra el servicio
// (`services/syerp/internal/store/verifactu.go`).
const (
	ERPVFPendiente  = "pendiente"
	ERPVFCorrecto   = "correcto"
	ERPVFConErrores = "aceptado_con_errores"
	ERPVFIncorrecto = "incorrecto"
)

// ERPRegistroVerifactu es un registro de facturación de VERI*FACTU: lo que se mandó a la
// Agencia Tributaria por una factura y lo que ella contestó.
type ERPRegistroVerifactu struct {
	ID       string    `json:"id"`
	Tipo     string    `json:"tipo"` // alta o anulacion
	NIF      string    `json:"nif"`
	NumSerie string    `json:"num_serie"`
	Fecha    time.Time `json:"fecha_expedicion"`
	Huella   string    `json:"huella"`
	Estado   string    `json:"estado"`
	// CSV es el código seguro de verificación que devuelve la Agencia al aceptarlo.
	CSV         string     `json:"csv,omitempty"`
	Codigo      *int       `json:"codigo_error,omitempty"`
	Descripcion string     `json:"descripcion_error,omitempty"`
	GeneradoEn  time.Time  `json:"generado_en"`
	EnviadoEn   *time.Time `json:"enviado_en,omitempty"`
}

// Corregible: la Agencia la rechazó o la aceptó con errores, así que se puede volver a
// mandar con los datos de ahora. Lo demás (pendiente, correcto) no se subsana.
func (r ERPRegistroVerifactu) Corregible() bool {
	return r.Estado == ERPVFIncorrecto || r.Estado == ERPVFConErrores
}

// ERPFacturaVerifactu son los registros de UNA factura, en orden, y la URL del QR con el
// que cualquiera comprueba la factura en la sede de la Agencia. El QR existe desde que se
// genera el alta, aunque la Agencia todavía no haya contestado.
type ERPFacturaVerifactu struct {
	Registros []ERPRegistroVerifactu `json:"registros"`
	QR        string                 `json:"qr"`
}

// Ultimo es el registro que manda: el estado de la factura es el del último.
func (f ERPFacturaVerifactu) Ultimo() (ERPRegistroVerifactu, bool) {
	if len(f.Registros) == 0 {
		return ERPRegistroVerifactu{}, false
	}
	return f.Registros[len(f.Registros)-1], true
}

// RegistrosVerifactu devuelve lo que la Agencia Tributaria sabe de esta factura
// (`GET /api/v1/{modelo}/{id}/verifactu`).
func (e *ERP) RegistrosVerifactu(ctx context.Context, modelo, id string) (ERPFacturaVerifactu, error) {
	return JSON[ERPFacturaVerifactu](ctx, e.c, e.pedir("/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/verifactu", nil))
}

// SubsanarVerifactu vuelve a mandar la factura con sus datos de AHORA, tras un rechazo o
// una aceptación con errores (`POST /api/v1/{modelo}/{id}/verifactu/subsanar`). Devuelve
// los registros ya con el nuevo, como hace el servicio.
func (e *ERP) SubsanarVerifactu(ctx context.Context, modelo, id string) (ERPFacturaVerifactu, error) {
	r := e.pedir("/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/verifactu/subsanar", nil)
	r.Method = http.MethodPost
	return JSON[ERPFacturaVerifactu](ctx, e.c, r)
}
