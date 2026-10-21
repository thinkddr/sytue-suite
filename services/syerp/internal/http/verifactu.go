// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"bufio"
	"context"
	"errors"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
	"git.sytue.eu/sytue/one/services/syerp/internal/verifactu"
)

// VERI*FACTU por HTTP: el estado y la activación, la declaración responsable, los
// registros de cada factura con su QR, la corrección de lo que rechaza la AEAT y
// la exportación de todos los registros.

var (
	errVerifactuNoDisponible = httpx.NewError(http.StatusConflict, "verifactu_no_disponible",
		"VERI*FACTU cannot be enabled for this organisation.")
	errFacturaEmitida = httpx.NewError(http.StatusConflict, "factura_emitida",
		"This invoice has already been sent to the Spanish Tax Agency: correct it with a corrective invoice.")
	errVerifactuPendiente = httpx.NewError(http.StatusConflict, "verifactu_pendiente",
		"The Spanish Tax Agency has not answered for this invoice yet.")
	errNadaQueSubsanar = httpx.NewError(http.StatusConflict, "nada_que_subsanar",
		"There is nothing to correct for this invoice at the Spanish Tax Agency.")
	errNIFConVerifactu = httpx.NewError(http.StatusConflict, "nif_con_verifactu",
		"The organisation's tax ID cannot change while VERI*FACTU is enabled.")
)

// Motivos por los que una organización no puede activar VERI*FACTU.
const (
	motivoSinPerfil      = "sin_perfil"      // no ha hecho el asistente
	motivoPais           = "pais"            // no es española
	motivoForal          = "foral"           // País Vasco y Navarra: TicketBAI y su sistema propio
	motivoIGIC           = "igic_ipsi"       // Canarias, Ceuta y Melilla: sin IVA
	motivoNIF            = "nif"             // NIF que no puede ser
	motivoSinCertificado = "sin_certificado" // el servicio no tiene con qué presentar
	motivoDeclaracion    = "declaracion"     // producción sin la declaración responsable suscrita
)

// estadoVerifactu es lo que ve la interfaz.
type estadoVerifactu struct {
	store.Verifactu
	// Motivo es por qué no se puede activar; vacío si se puede (o ya está).
	Motivo  string `json:"motivo,omitempty"`
	Entorno string `json:"entorno"`
}

// motivoVerifactu dice por qué la organización no puede usar VERI*FACTU, o "".
func (h *Handlers) motivoVerifactu(ctx context.Context) (string, error) {
	org, err := h.store.Organizacion(ctx)
	switch {
	case errors.Is(err, store.ErrNoEncontrado):
		return motivoSinPerfil, nil
	case err != nil:
		return "", err
	case org.Pais != "ES":
		return motivoPais, nil
	}
	switch cp := strings.TrimSpace(org.CP); {
	case len(cp) != 5:
		return motivoSinPerfil, nil
	case strings.HasPrefix(cp, "01"), strings.HasPrefix(cp, "20"), strings.HasPrefix(cp, "48"), strings.HasPrefix(cp, "31"):
		return motivoForal, nil
	case strings.HasPrefix(cp, "35"), strings.HasPrefix(cp, "38"), strings.HasPrefix(cp, "51"), strings.HasPrefix(cp, "52"):
		return motivoIGIC, nil
	}
	if v := domain.VariantesNIF(org.NIF); len(v) == 0 || len(v[len(v)-1]) != 9 {
		return motivoNIF, nil
	}
	if h.verifactu == nil {
		return motivoSinCertificado, nil
	}
	if h.cfg.VerifactuProduccion && verifactu.DeclaracionFecha == "" {
		return motivoDeclaracion, nil
	}
	return "", nil
}

func (h *Handlers) entornoVerifactu() string {
	if h.cfg.VerifactuProduccion {
		return "produccion"
	}
	return "pruebas"
}

// Verifactu — GET /api/v1/verifactu.
func (h *Handlers) Verifactu(w http.ResponseWriter, r *http.Request) {
	v, err := h.store.EstadoVerifactu(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := estadoVerifactu{Verifactu: v, Entorno: h.entornoVerifactu()}
	if !v.Activo {
		if out.Motivo, err = h.motivoVerifactu(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// ActivarVerifactu — POST /api/v1/verifactu/activar. Owner y admin.
func (h *Handlers) ActivarVerifactu(w http.ResponseWriter, r *http.Request) {
	motivo, err := h.motivoVerifactu(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if motivo != "" {
		httpx.Fail(w, r, errVerifactuNoDisponible.WithDetails(map[string]string{"motivo": motivo}))
		return
	}
	v, err := h.store.ActivarVerifactu(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, estadoVerifactu{Verifactu: v, Entorno: h.entornoVerifactu()})
}

// registrosDeFactura devuelve los registros de una factura de venta y la URL de su
// QR, que existe desde que se genera el alta (aunque la AEAT aún no haya
// contestado) y deja de enseñarse si la factura se anula.
func (h *Handlers) registrosDeFactura(ctx context.Context, reg store.Registro) ([]store.RegistroVerifactu, string, error) {
	if reg.Modelo != "factura_venta" {
		return []store.RegistroVerifactu{}, "", nil
	}
	registros, err := h.store.RegistrosVerifactu(ctx, reg.ID)
	if err != nil || len(registros) == 0 || registros[len(registros)-1].Tipo != "alta" {
		return registros, "", err
	}
	alta := registros[len(registros)-1]
	base, _ := domain.Centimos(reg.Data, "base")
	iva, _ := domain.Centimos(reg.Data, "iva")
	return registros, verifactu.URLQR(h.cfg.VerifactuProduccion, alta.NIF, alta.NumSerie, alta.Fecha, base+iva), nil
}

// RegistrosFacturaVerifactu — GET /api/v1/{modelo}/{id}/verifactu.
func (h *Handlers) RegistrosFacturaVerifactu(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	reg, err := h.store.Obtener(r.Context(), m.Clave, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	registros, qr, err := h.registrosDeFactura(r.Context(), reg)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"registros": registros, "qr": qr})
}

// SubsanarVerifactu — POST /api/v1/{modelo}/{id}/verifactu/subsanar: vuelve a
// enviar la factura con sus datos de ahora tras un rechazo o una aceptación con
// errores.
func (h *Handlers) SubsanarVerifactu(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if m.Clave != "factura_venta" {
		httpx.Fail(w, r, errNadaQueSubsanar)
		return
	}
	if err := h.store.SubsanarVerifactu(r.Context(), chi.URLParam(r, "id"), sub(r)); err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	h.RegistrosFacturaVerifactu(w, r)
}

// ExportarVerifactu — GET /api/v1/verifactu/exportar: todos los registros de la
// organización en un XML, en el orden de la cadena.
func (h *Handlers) ExportarVerifactu(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="verifactu-registros.xml"`)
	w.Header().Set("Cache-Control", "no-store")
	b := bufio.NewWriter(w)
	_, _ = b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<RegistrosFacturacion>` + "\n")
	err := h.store.ExportarVerifactu(r.Context(), func(x string) error {
		_, err := b.WriteString(x + "\n")
		return err
	})
	if err != nil {
		// Las cabeceras ya salieron: se corta el documento, que queda mal formado
		// a propósito para que no pase por completo.
		_ = b.Flush()
		return
	}
	_, _ = b.WriteString(`</RegistrosFacturacion>` + "\n")
	_ = b.Flush()
}

// DeclaracionVerifactu — GET /api/v1/verifactu/declaracion: la declaración
// responsable del productor (RD 1007/2023, Orden HAC/1177/2024), consultable desde
// el propio programa.
func (h *Handlers) DeclaracionVerifactu(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = plantillaDeclaracion.Execute(w, map[string]string{
		"Nombre": verifactu.NombreSistema, "ID": verifactu.IDSistema, "Version": h.cfg.Version,
		"Productor": verifactu.ProductorNombre, "NIF": verifactu.ProductorNIF, "Direccion": verifactu.ProductorDireccion,
		"Fecha": verifactu.DeclaracionFecha, "Lugar": verifactu.DeclaracionLugar,
		"Web": verifactu.ProductorWeb, "Correo": verifactu.ProductorCorreo,
	})
}

var plantillaDeclaracion = template.Must(template.New("declaracion").Parse(`<!doctype html>
<html lang="es"><head><meta charset="utf-8"><title>Declaración responsable · {{.Nombre}}</title>
<style>
body{font:15px/1.55 system-ui,sans-serif;max-width:46rem;margin:2rem auto;padding:0 1rem;color:#111}
h1{font-size:1.35rem}h2{font-size:1rem;margin:1.4rem 0 .3rem}p{margin:.2rem 0}
.pendiente{border:1px solid #b45309;background:#fffbeb;padding:.6rem .8rem;border-radius:6px}
@media print{body{margin:0}}
</style></head><body>
<h1>Declaración responsable del sistema informático de facturación</h1>
{{if not .Fecha}}<p class="pendiente">Pendiente de suscribir por la entidad productora: hasta entonces este sistema solo se puede usar con el entorno de pruebas de la Agencia Tributaria.</p>{{end}}
<h2>1.a) Nombre del sistema informático</h2><p>{{.Nombre}}</p>
<h2>1.b) Código identificador del sistema informático</h2><p>{{.ID}}</p>
<h2>1.c) Identificador completo de la versión</h2><p>{{.Version}}</p>
<h2>1.d) Componentes y funcionalidades</h2>
<p>Servicio en la nube (software como servicio) que se usa desde el navegador y desde las aplicaciones de Sytue. Lo forman el servicio del ERP, que expide las facturas y genera, encadena, conserva y remite los registros de facturación, y su base de datos PostgreSQL. Permite registrar clientes, expedir facturas y facturas rectificativas con su código QR tributario, anularlas, consultar la facturación y su estado en la Agencia Tributaria, exportar los registros de facturación y llevar la contabilidad.</p>
<p>Gestiona de forma independiente la facturación de cada organización cliente, cumpliendo separadamente con la normativa del apartado 1.k) para cada una de ellas, como si se tratara de sistemas informáticos de facturación distintos, cada uno con su número de instalación.</p>
<h2>1.e) ¿Solo puede funcionar como «VERI*FACTU»?</h2><p>S - Sí</p>
<h2>1.f) ¿Permite ser usado por varios obligados tributarios?</h2><p>S - Sí</p>
<h2>1.g) Tipos de firma</h2>
<p>Al funcionar exclusivamente como «VERI*FACTU» no firma electrónicamente los registros de facturación ni genera registros de evento: quedan firmados al remitirse a la Agencia Tributaria con la autenticación de un certificado electrónico cualificado.</p>
<h2>1.h) Razón social de la entidad productora</h2><p>{{.Productor}}</p>
<h2>1.i) NIF de la entidad productora</h2><p>{{.NIF}}</p>
<h2>1.j) Dirección postal de contacto</h2><p>{{.Direccion}}</p>
<h2>1.k) Cumplimiento</h2>
<p>La entidad productora hace constar que este sistema informático, en la versión indicada, cumple con lo dispuesto en el artículo 29.2.j) de la Ley 58/2003, de 17 de diciembre, General Tributaria, en el Reglamento que establece los requisitos que deben adoptar los sistemas y programas informáticos o electrónicos que soporten los procesos de facturación de empresarios y profesionales, y la estandarización de formatos de los registros de facturación, aprobado por el Real Decreto 1007/2023, de 5 de diciembre, en la Orden HAC/1177/2024, de 17 de octubre, y en la sede electrónica de la Agencia Estatal de Administración Tributaria para todo aquello que complete las especificaciones de dicha orden.</p>
<h2>1.l) Fecha y lugar de suscripción</h2><p>{{if .Fecha}}{{.Fecha}}, {{.Lugar}}{{else}}Pendiente{{end}}</p>
<h2>2.a) Otras formas de contacto</h2><p>Correo electrónico: {{.Correo}}</p>
<h2>2.b) Direcciones de internet</h2><p>{{.Web}}</p>
<h2>2.c) Forma de cumplir las especificaciones</h2>
<p>La expedición de la factura y la generación de su registro de facturación se consolidan en una sola transacción de la base de datos. La base de datos impide modificar o borrar los registros generados, bifurcar su encadenamiento y alterar una factura expedida; las correcciones se hacen con registros de subsanación, de anulación o con facturas rectificativas. Los registros se remiten automáticamente a la Agencia Tributaria respetando el tiempo de espera entre envíos que esta indica, y el resultado de cada uno queda anotado junto al registro.</p>
</body></html>`))
