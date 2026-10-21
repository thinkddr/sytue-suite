// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// # Obligaciones: el calendario fiscal, laboral y mercantil de la organización
//
// Las formas son las de `services/syerp/internal/obligaciones` (Instancia, Sancion,
// Guia, Documento) y la envoltura de `handlers_organizacion.go` (instanciaVista). La web
// las lee en `Obligaciones.tsx`.

// ERPSancion es lo que cuesta llegar tarde, en cifras: la interfaz compone la frase en
// el idioma de quien mira. `Nota` es la letra pequeña, en el idioma del país.
type ERPSancion struct {
	// Tipo: recargo (porcentaje sobre la cuota), fija (importe) o nota (solo texto).
	Tipo                string    `json:"tipo"`
	RecargoBasePct      float64   `json:"recargo_base_pct,omitempty"`
	RecargoPctMes       float64   `json:"recargo_pct_mes,omitempty"`
	RecargoMaxPct       float64   `json:"recargo_max_pct,omitempty"`
	ConRequerimientoPct []float64 `json:"con_requerimiento_pct,omitempty"`
	// FijaCentimos es el importe fijo (tipo fija) o el mínimo (tipo recargo).
	FijaCentimos int64  `json:"fija_centimos,omitempty"`
	Moneda       string `json:"moneda,omitempty"`
	Nota         string `json:"nota,omitempty"`
}

// ERPGuia es qué es una obligación, qué hace falta, cómo se hace y qué pasa si no llega.
type ERPGuia struct {
	Que       string   `json:"que"`
	Necesitas []string `json:"necesitas"`
	Pasos     []string `json:"pasos"`
	SiNo      string   `json:"si_no"`
}

// ERPDocumentoObligacion es un fichero que conviene subir al presentarla, con lo que
// tiene que traer para valer.
type ERPDocumentoObligacion struct {
	Clave      string   `json:"clave"`
	Nombre     string   `json:"nombre"`
	Requisitos []string `json:"requisitos"`
}

// ERPPortal es la sede donde se presenta.
type ERPPortal struct {
	Nombre string `json:"nombre"`
	URL    string `json:"url"`
}

// ERPObligacion es una presentación de un periodo. Las fechas llegan como instantes
// (medianoche UTC) y se leen por su día en UTC, como hace la web.
type ERPObligacion struct {
	// Proceso es el progreso compartido por la organización, persistido por el servicio.
	Proceso        *ERPProcesoObligacion `json:"proceso,omitempty"`
	Clave          string                `json:"clave"`
	Nombre         string                `json:"nombre"`
	Concepto       string                `json:"concepto"`
	ConceptoRotulo string                `json:"concepto_rotulo"`
	Ambito         string                `json:"ambito"`
	AmbitoRotulo   string                `json:"ambito_rotulo"`
	Periodicidad   string                `json:"periodicidad"`
	Periodo        string                `json:"periodo"`
	Limite         time.Time             `json:"limite"`
	// Recomendada es «tu fecha»: días hábiles antes del límite, para que haya margen.
	Recomendada         time.Time   `json:"recomendada"`
	Portal              ERPPortal   `json:"portal"`
	Sancion             *ERPSancion `json:"sancion,omitempty"`
	Calculable          bool        `json:"calculable"`
	LimiteDomiciliacion *time.Time  `json:"limite_domiciliacion,omitempty"`
	Guia                *ERPGuia    `json:"guia,omitempty"`
	// Pendientes es lo que falta en el ERP para presentarla (banco, facturas, nóminas).
	// nil: no se comprueba nada; con ceros: está lista.
	Pendientes            map[string]int           `json:"pendientes,omitempty"`
	CalendarioProvisional bool                     `json:"calendario_provisional,omitempty"`
	Documentos            []ERPDocumentoObligacion `json:"documentos,omitempty"`
}

// ERPTerritorio es una región con hacienda propia para algunos impuestos.
type ERPTerritorio struct {
	Nombre string `json:"nombre"`
	Nota   string `json:"nota"`
}

// ERPObligaciones es el calendario de un año.
type ERPObligaciones struct {
	Anio         int             `json:"anio"`
	Pais         string          `json:"pais"`
	Verificado   bool            `json:"verificado"`
	Moneda       string          `json:"moneda"`
	Region       string          `json:"region,omitempty"`
	Territorio   *ERPTerritorio  `json:"territorio,omitempty"`
	Obligaciones []ERPObligacion `json:"obligaciones"`
}

// Obligaciones pide el calendario del año (`GET /api/v1/obligaciones?anio=`).
func (e *ERP) Obligaciones(ctx context.Context, anio int) (ERPObligaciones, error) {
	return JSON[ERPObligaciones](ctx, e.c, e.pedir("/obligaciones", url.Values{"anio": {strconv.Itoa(anio)}}))
}

// ERPCalculoObligacion es la cifra que sale del libro para un periodo.
type ERPCalculoObligacion struct {
	Clave             string  `json:"clave"`
	Periodo           string  `json:"periodo"`
	Desde             string  `json:"desde"`
	Hasta             string  `json:"hasta"`
	Tipo              string  `json:"tipo"`
	Porcentaje        float64 `json:"porcentaje"`
	HaberCentimos     int64   `json:"haber_centimos"`
	DebeCentimos      int64   `json:"debe_centimos"`
	ResultadoCentimos int64   `json:"resultado_centimos"`
	// Asentado: hay asientos en el periodo. Sin ellos la cifra no dice nada.
	Asentado bool `json:"asentado"`
}

// CalculoObligacion es «Preparar cifras» (`GET /api/v1/obligaciones/calculo`).
func (e *ERP) CalculoObligacion(ctx context.Context, clave, periodo string) (ERPCalculoObligacion, error) {
	return JSON[ERPCalculoObligacion](ctx, e.c, e.pedir("/obligaciones/calculo", url.Values{"clave": {clave}, "periodo": {periodo}}))
}

// ObligacionesICS baja el calendario como iCalendar, para IMPORTARLO —no suscribirse—
// con sus alarmas, y el nombre con que lo nombra el servicio. Quien llama cierra el lector.
func (e *ERP) ObligacionesICS(ctx context.Context, anio int) (io.ReadCloser, string, error) {
	resp, err := e.c.Do(ctx, e.pedir("/obligaciones/ics", url.Values{"anio": {strconv.Itoa(anio)}}))
	if err != nil {
		return nil, "", err
	}
	return resp.Body, nombreAdjunto(resp.Header.Get("Content-Disposition"), "obligaciones.ics"), nil
}

// --- El asistente de arranque (`Onboarding.tsx`) ------------------------------

// ERPFormaJuridica es una forma jurídica de un país; `Tipo` es persona_fisica o
// sociedad.
type ERPFormaJuridica struct {
	Clave  string `json:"clave"`
	Nombre string `json:"nombre"`
	Tipo   string `json:"tipo"`
}

// ERPPais es un país del asistente: si su calendario está revisado, su moneda y sus
// formas jurídicas.
type ERPPais struct {
	Pais         string             `json:"pais"`
	Verificado   bool               `json:"verificado"`
	Moneda       string             `json:"moneda"`
	Formas       []ERPFormaJuridica `json:"formas"`
	Obligaciones int                `json:"obligaciones"`
}

// Paises pide los países que el ERP conoce (`GET /api/v1/organizacion/paises`).
func (e *ERP) Paises(ctx context.Context) ([]ERPPais, error) {
	r, err := JSON[struct {
		Paises []ERPPais `json:"paises"`
	}](ctx, e.c, e.pedir("/organizacion/paises", nil))
	return r.Paises, err
}

// GuardarOrganizacion crea o sustituye el perfil (`PUT /api/v1/organizacion`, solo
// administración) y devuelve el que quedó.
func (e *ERP) GuardarOrganizacion(ctx context.Context, o ERPOrganizacion) (ERPOrganizacion, error) {
	datos, err := json.Marshal(o)
	if err != nil {
		return ERPOrganizacion{}, err
	}
	r := e.pedir("/organizacion", nil)
	r.Method, r.Body = http.MethodPut, datos
	r.Header.Set("Content-Type", "application/json")
	return JSON[ERPOrganizacion](ctx, e.c, r)
}
