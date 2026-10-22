// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// # Lo que cuelga de un registro y lo que no es un modelo
//
// Tareas, documentos, historial, libro diario, balance, resumen de la portada y el perfil
// de la organización. Las formas son las de `services/syerp/internal/store` y
// `.../obligaciones`, y las ata `TestERPSeguimientoHablaElIdiomaDelServicio` leyendo el
// código del servicio: un nombre de campo cambiado allí se ve aquí en rojo.

// ERPActividad es una tarea con fecha sobre un registro (las «actividades» de Odoo).
type ERPActividad struct {
	ID               string `json:"id"`
	Modelo           string `json:"modelo"`
	Registro         string `json:"registro"`
	RegistroEtiqueta string `json:"registro_etiqueta,omitempty"`
	// Tipo es uno de ERPTiposActividad.
	Tipo              string `json:"tipo"`
	Resumen           string `json:"resumen"`
	Notas             string `json:"notas,omitempty"`
	Vence             string `json:"vence"` // AAAA-MM-DD
	ResponsableNombre string `json:"responsable_nombre,omitempty"`
	Estado            string `json:"estado"`
}

// ERPTiposActividad son los seis que admite el servicio, en el orden del selector de la
// web (`TIPOS` de Seguimiento.tsx).
var ERPTiposActividad = []string{"subir_documento", "pedir_documento", "llamada", "correo", "reunion", "tarea"}

// ERPFiltroActividades acota la lista de tareas.
type ERPFiltroActividades struct {
	// Estado es «pendiente», «hecha» o «cancelada»; vacío, todas.
	Estado string
	// Mias deja solo las de quien pide.
	Mias bool
	// Hasta deja las que vencen ese día o antes (AAAA-MM-DD).
	Hasta string
	// Modelo y Registro acotan a las de un registro.
	Modelo, Registro string
}

// Actividades lista las tareas (`GET /api/v1/actividades`).
func (e *ERP) Actividades(ctx context.Context, f ERPFiltroActividades) ([]ERPActividad, error) {
	q := url.Values{}
	for clave, valor := range map[string]string{"estado": f.Estado, "hasta": f.Hasta, "modelo": f.Modelo, "registro": f.Registro} {
		if valor != "" {
			q.Set(clave, valor)
		}
	}
	if f.Mias {
		q.Set("mias", "1")
	}
	r, err := JSON[struct {
		Actividades []ERPActividad `json:"actividades"`
	}](ctx, e.c, e.pedir("/actividades", q))
	return r.Actividades, err
}

// ERPPersona es alguien de la organización que puede llevar una tarea (`GET /equipo`).
type ERPPersona struct {
	Sub    string `json:"sub"`
	Nombre string `json:"nombre"`
	Correo string `json:"correo"`
	// Yo es quien pregunta (el `yo` de la respuesta): el selector lo pone el primero
	// como «Yo (su nombre)» y no lo repite.
	Yo bool `json:"-"`
}

// Equipo es la gente de la organización, para el selector de «Responsable».
func (e *ERP) Equipo(ctx context.Context) ([]ERPPersona, error) {
	r, err := JSON[struct {
		Personas []ERPPersona `json:"personas"`
		Yo       string       `json:"yo"`
	}](ctx, e.c, e.pedir("/equipo", nil))
	for i := range r.Personas {
		r.Personas[i].Yo = r.Yo != "" && r.Personas[i].Sub == r.Yo
	}
	return r.Personas, err
}

// CrearActividad planifica una tarea sobre un registro. Con `para`, la lleva esa
// persona, y le llega el aviso; sin él, quien la crea.
func (e *ERP) CrearActividad(ctx context.Context, modelo, registro, tipo, resumen, vence string, para ...ERPPersona) error {
	cuerpo := map[string]string{
		"modelo": modelo, "registro": registro, "tipo": tipo, "resumen": resumen, "vence": vence,
	}
	if len(para) > 0 && para[0].Sub != "" {
		cuerpo["responsable_sub"] = para[0].Sub
		cuerpo["responsable_nombre"] = cmp.Or(para[0].Nombre, para[0].Correo)
	}
	return e.enviar(ctx, "/actividades", cuerpo)
}

// CerrarActividad da una tarea por hecha o la cancela (`como` es «hecha» o «cancelar»).
func (e *ERP) CerrarActividad(ctx context.Context, id, como string) error {
	return e.enviar(ctx, "/actividades/"+url.PathEscape(id)+"/"+como, struct{}{})
}

// ERPAdjunto es un documento subido a un registro.
type ERPAdjunto struct {
	ID       string    `json:"id"`
	Modelo   string    `json:"modelo"`
	Registro string    `json:"registro"`
	Nombre   string    `json:"nombre"`
	Tipo     string    `json:"tipo"`
	Tamano   int64     `json:"tamano"`
	CreadoEn time.Time `json:"creado_en"`
}

// Adjuntos lista los documentos de un registro.
func (e *ERP) Adjuntos(ctx context.Context, modelo, registro string) ([]ERPAdjunto, error) {
	r, err := JSON[struct {
		Adjuntos []ERPAdjunto `json:"adjuntos"`
	}](ctx, e.c, e.pedir("/adjuntos", url.Values{"modelo": {modelo}, "registro": {registro}}))
	return r.Adjuntos, err
}

// SubirAdjunto sube un documento a un registro y devuelve cuántas tareas cerró (subir la
// factura de un gasto da por hecha la tarea «sube la factura»). El servicio decide el
// tipo por el CONTENIDO, no por `tipo`, que solo va de cabecera.
func (e *ERP) SubirAdjunto(ctx context.Context, modelo, registro, nombre, tipo string, datos []byte) (int, error) {
	r := e.pedir("/adjuntos", url.Values{"modelo": {modelo}, "registro": {registro}, "nombre": {nombre}})
	r.Method, r.Body = http.MethodPost, datos
	if tipo == "" {
		tipo = "application/octet-stream"
	}
	r.Header.Set("Content-Type", tipo)
	res, err := JSON[struct {
		TareasHechas int `json:"tareas_hechas"`
	}](ctx, e.c, r)
	return res.TareasHechas, err
}

// Adjunto baja un documento. Quien llama cierra el lector.
func (e *ERP) Adjunto(ctx context.Context, id string) (io.ReadCloser, error) {
	resp, err := e.c.Do(ctx, e.pedir("/adjuntos/"+url.PathEscape(id), nil))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// BorrarAdjunto quita un documento de su registro.
func (e *ERP) BorrarAdjunto(ctx context.Context, id string) error {
	return e.sinCuerpo(ctx, http.MethodDelete, "/adjuntos/"+url.PathEscape(id), nil)
}

// ERPCambio es un campo con su valor de antes y el de después.
type ERPCambio struct {
	Antes   any `json:"antes"`
	Despues any `json:"despues"`
}

// ERPEntradaHistorial es una línea del historial de un registro: se creó, cambió algo,
// se borró, se restauró o alguien dejó una nota.
type ERPEntradaHistorial struct {
	ID       string               `json:"id"`
	Tipo     string               `json:"tipo"`
	Sub      string               `json:"sub,omitempty"`
	Nombre   string               `json:"nombre,omitempty"`
	Cambios  map[string]ERPCambio `json:"cambios,omitempty"`
	Texto    string               `json:"texto,omitempty"`
	CreadoEn time.Time            `json:"creado_en"`
}

// Historial devuelve el historial de un registro, lo más reciente primero.
func (e *ERP) Historial(ctx context.Context, modelo, id string) ([]ERPEntradaHistorial, error) {
	r, err := JSON[struct {
		Historial []ERPEntradaHistorial `json:"historial"`
	}](ctx, e.c, e.pedir("/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/historial", nil))
	return r.Historial, err
}

// Anotar deja una nota en el historial de un registro.
func (e *ERP) Anotar(ctx context.Context, modelo, id, texto string) error {
	return e.enviar(ctx, "/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/historial",
		map[string]string{"texto": texto})
}

// ERPApunte es una línea de un asiento. Importes en CÉNTIMOS.
type ERPApunte struct {
	Cuenta   string `json:"cuenta"`
	Concepto string `json:"concepto,omitempty"`
	Debe     int64  `json:"debe"`
	Haber    int64  `json:"haber"`
}

// ERPAsiento es un asiento por partida doble.
type ERPAsiento struct {
	ID       string `json:"id"`
	Diario   string `json:"diario"`
	Ref      string `json:"ref"`
	Fecha    string `json:"fecha"`
	Concepto string `json:"concepto"`
	Estado   string `json:"estado"`
	// Apuntes son sus líneas; el libro las trae siempre.
	Apuntes []ERPApunte `json:"apuntes,omitempty"`
}

// AsientosDe son los asientos que generó un documento contable.
func (e *ERP) AsientosDe(ctx context.Context, modelo, id string) ([]ERPAsiento, error) {
	r, err := JSON[struct {
		Asientos []ERPAsiento `json:"asientos"`
	}](ctx, e.c, e.pedir("/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/asientos", nil))
	return r.Asientos, err
}

// Libro devuelve los últimos `limite` asientos del libro diario.
func (e *ERP) Libro(ctx context.Context, limite int) ([]ERPAsiento, error) {
	r, err := JSON[struct {
		Asientos []ERPAsiento `json:"asientos"`
	}](ctx, e.c, e.pedir("/libro", url.Values{"limite": {strconv.Itoa(limite)}}))
	return r.Asientos, err
}

// AnularAsiento anula un asiento con su motivo. No borra nada: escribe el contrario, y
// solo quien administra la organización puede hacerlo (403 si no).
func (e *ERP) AnularAsiento(ctx context.Context, id, motivo string) error {
	return e.enviar(ctx, "/libro/"+url.PathEscape(id)+"/anular", map[string]string{"motivo": motivo})
}

// ERPSaldoCuenta es una fila del sumas y saldos.
type ERPSaldoCuenta struct {
	Cuenta string `json:"cuenta"`
	Debe   int64  `json:"debe"`
	Haber  int64  `json:"haber"`
	Saldo  int64  `json:"saldo"`
}

// ERPBalance es el sumas y saldos con sus dos totales.
type ERPBalance struct {
	Cuentas    []ERPSaldoCuenta `json:"cuentas"`
	TotalDebe  int64            `json:"total_debe"`
	TotalHaber int64            `json:"total_haber"`
}

// Balance devuelve el sumas y saldos.
func (e *ERP) Balance(ctx context.Context) (ERPBalance, error) {
	return JSON[ERPBalance](ctx, e.c, e.pedir("/balance", nil))
}

// ERPResumen son los indicadores de la portada.
type ERPResumen struct {
	Facturacion struct {
		TotalCentimos          int64  `json:"total_centimos"`
		TotalRegistros         int    `json:"total_registros"`
		PendienteCobroCentimos int64  `json:"pendiente_cobro_centimos"`
		FacturasPendientes     int    `json:"facturas_pendientes"`
		ProximoVencimiento     string `json:"proximo_vencimiento,omitempty"`
	} `json:"facturacion"`
	Contabilidad struct {
		TotalAsientos int64 `json:"total_asientos"`
		TotalDebe     int64 `json:"total_debe"`
		Cuadra        bool  `json:"cuadra"`
	} `json:"contabilidad"`
}

// Resumen pide los indicadores de la portada.
func (e *ERP) Resumen(ctx context.Context) (ERPResumen, error) {
	return JSON[ERPResumen](ctx, e.c, e.pedir("/resumen", nil))
}

// ERPOrganizacion es el perfil de la organización. Lo que decide la interfaz es el PAÍS:
// VERI*FACTU y Facturae solo existen para las españolas.
type ERPOrganizacion struct {
	Nombre    string `json:"nombre"`
	NIF       string `json:"nif"`
	Direccion string `json:"direccion"`
	CP        string `json:"cp"`
	Ciudad    string `json:"ciudad"`
	Pais      string `json:"pais"`
	Forma     string `json:"forma"`
	// Condiciones son los interruptores del perfil (iva, empleados…).
	Condiciones map[string]bool `json:"condiciones"`
}

// Organizacion devuelve el perfil. Un 404 es que todavía no se ha hecho el asistente:
// `EsNoEncontrado` lo distingue de un corte, que no tiene que abrir el asistente.
func (e *ERP) Organizacion(ctx context.Context) (ERPOrganizacion, error) {
	return JSON[ERPOrganizacion](ctx, e.c, e.pedir("/organizacion", nil))
}

// ERPRequisitosFactura es lo que tiene que llevar una factura para valer en el país.
type ERPRequisitosFactura struct {
	Fuente       string   `json:"fuente"`
	Completa     []string `json:"completa"`
	Simplificada []string `json:"simplificada"`
	Extranjera   []string `json:"extranjera"`
}

// RequisitosFactura los pide al servicio.
func (e *ERP) RequisitosFactura(ctx context.Context) (ERPRequisitosFactura, error) {
	return JSON[ERPRequisitosFactura](ctx, e.c, e.pedir("/organizacion/requisitos", nil))
}

// ERPEstadoVerifactu es VERI*FACTU en la organización y cómo van sus envíos.
type ERPEstadoVerifactu struct {
	Activo           bool       `json:"activo"`
	Desde            *time.Time `json:"desde,omitempty"`
	TieneCertificado bool       `json:"tiene_certificado,omitempty"`
	CertificadoNIF   string     `json:"certificado_nif,omitempty"`
	Pendientes       int        `json:"pendientes"`
	ConErrores       int        `json:"con_errores"`
	Rechazadas       int        `json:"rechazadas"`
	ErrorEnvio       string     `json:"error_envio,omitempty"`
	// Motivo es por qué no se puede activar; vacío si se puede o ya está.
	Motivo  string `json:"motivo,omitempty"`
	Entorno string `json:"entorno"`
}

// EstadoVerifactu lo pide al servicio.
func (e *ERP) EstadoVerifactu(ctx context.Context) (ERPEstadoVerifactu, error) {
	return JSON[ERPEstadoVerifactu](ctx, e.c, e.pedir("/verifactu", nil))
}

// ActivarVerifactu lo activa para la organización. Solo quien la administra.
func (e *ERP) ActivarVerifactu(ctx context.Context) (ERPEstadoVerifactu, error) {
	r := e.pedir("/verifactu/activar", nil)
	r.Method = http.MethodPost
	return JSON[ERPEstadoVerifactu](ctx, e.c, r)
}

// GuardarCertificadoVerifactu guarda el certificado cualificado propio de la
// organización (`PUT /api/v1/verifactu/certificado`). Solo quien la administra.
func (e *ERP) GuardarCertificadoVerifactu(ctx context.Context, certPEM, keyPEM string, sello bool) (ERPEstadoVerifactu, error) {
	datos, err := json.Marshal(struct {
		CertPEM string `json:"cert_pem"`
		KeyPEM  string `json:"key_pem"`
		Sello   bool   `json:"sello"`
	}{certPEM, keyPEM, sello})
	if err != nil {
		return ERPEstadoVerifactu{}, err
	}
	r := e.pedir("/verifactu/certificado", nil)
	r.Method, r.Body = http.MethodPut, datos
	r.Header.Set("Content-Type", "application/json")
	return JSON[ERPEstadoVerifactu](ctx, e.c, r)
}

// DeclaracionVerifactu es la declaración responsable del sistema (`GET
// /api/v1/verifactu/declaracion`), una página HTML. Quien llama cierra el lector.
func (e *ERP) DeclaracionVerifactu(ctx context.Context) (io.ReadCloser, string, error) {
	return e.descarga(ctx, "/verifactu/declaracion", "declaracion-responsable.html")
}

// ExportarVerifactu baja todos los registros de facturación (`GET
// /api/v1/verifactu/exportar`), en XML. Quien llama cierra el lector.
func (e *ERP) ExportarVerifactu(ctx context.Context) (io.ReadCloser, string, error) {
	return e.descarga(ctx, "/verifactu/exportar", "verifactu-registros.xml")
}

// descarga pide un fichero y lo nombra como el servicio, o con `respaldo`.
func (e *ERP) descarga(ctx context.Context, ruta, respaldo string) (io.ReadCloser, string, error) {
	resp, err := e.c.Do(ctx, e.pedir(ruta, nil))
	if err != nil {
		return nil, "", err
	}
	return resp.Body, nombreAdjunto(resp.Header.Get("Content-Disposition"), respaldo), nil
}

// ERPModulosActivos es lo que contesta encender o apagar un módulo: los encendidos
// ahora y los que se encendieron o apagaron por dependencia.
type ERPModulosActivos struct {
	Activos      []string `json:"activos"`
	Activados    []string `json:"activados,omitempty"`
	Desactivados []string `json:"desactivados,omitempty"`
}

// CambiarModulo enciende o apaga un área. Apagar no borra nada.
func (e *ERP) CambiarModulo(ctx context.Context, modulo string, activo bool) (ERPModulosActivos, error) {
	datos, err := json.Marshal(map[string]bool{"activo": activo})
	if err != nil {
		return ERPModulosActivos{}, err
	}
	r := e.pedir("/modulos/"+url.PathEscape(modulo), nil)
	r.Method, r.Body = http.MethodPost, datos
	r.Header.Set("Content-Type", "application/json")
	return JSON[ERPModulosActivos](ctx, e.c, r)
}

// Lote aplica una acción a varios registros de un modelo: «estado» (con `estado`),
// «borrar» o «restaurar». Devuelve cuántos tocó.
func (e *ERP) Lote(ctx context.Context, modelo, accion string, ids []string, estado string) (int, error) {
	datos, err := json.Marshal(struct {
		Accion string   `json:"accion"`
		IDs    []string `json:"ids"`
		Estado string   `json:"estado,omitempty"`
	}{accion, ids, estado})
	if err != nil {
		return 0, err
	}
	r := e.pedir("/"+url.PathEscape(modelo)+"/lote", nil)
	r.Method, r.Body = http.MethodPost, datos
	r.Header.Set("Content-Type", "application/json")
	res, err := JSON[struct {
		Afectados int `json:"afectados"`
	}](ctx, e.c, r)
	return res.Afectados, err
}

// EsNoEncontrado dice si un error es un 404 del servicio.
func EsNoEncontrado(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// enviar hace un POST con un cuerpo JSON y descarta la respuesta.
func (e *ERP) enviar(ctx context.Context, ruta string, cuerpo any) error {
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return err
	}
	return e.sinCuerpo(ctx, http.MethodPost, ruta, datos)
}

// ERPResultadoImportacion es lo que pasó con un fichero importado: cuántas filas
// entraron nuevas o actualizadas, qué columnas no se reconocieron y qué filas fallaron.
type ERPResultadoImportacion struct {
	Filas        int      `json:"filas"`
	Creados      int      `json:"creados"`
	Actualizados int      `json:"actualizados"`
	Ignoradas    []string `json:"ignoradas"`
	Fallos       []struct {
		Fila  int    `json:"fila"`
		Error string `json:"error"`
	} `json:"fallos"`
}

// Importar sube un CSV —o, en los modelos que reciben facturas, una factura
// electrónica en XML— a `POST /api/v1/{modelo}/importar`.
func (e *ERP) Importar(ctx context.Context, modelo string, datos []byte, xml bool) (ERPResultadoImportacion, error) {
	r := e.pedir("/"+url.PathEscape(modelo)+"/importar", nil)
	r.Method, r.Body = http.MethodPost, datos
	tipo := "text/csv"
	if xml {
		tipo = "application/xml"
	}
	r.Header.Set("Content-Type", tipo)
	return JSON[ERPResultadoImportacion](ctx, e.c, r)
}
