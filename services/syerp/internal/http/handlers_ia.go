// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/config"
	"git.sytue.eu/sytue/one/services/syerp/internal/ia"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// Sytue AI en el ERP (ver el paquete ia):
//
//   - POST /ia/informe: una frase → un informe (tablero o Excel), todavía sin datos.
//   - POST /informes/datos y /informes/excel: un informe → sus tablas o su Excel.
//   - GET, POST /tableros y DELETE /tableros/{id}: los tableros guardados.
//   - POST /{modelo}/ia/rellenar: un texto pegado → los campos de una ficha.
//
// Proponer y ejecutar van en DOS peticiones a propósito: el modelo de calidad
// tarda del orden de diez segundos y cada petición tiene treinta, y así además
// la persona ve qué va a salir antes de descargarlo.

var (
	errIANoDisponible = httpx.NewError(http.StatusServiceUnavailable, "ia_no_disponible",
		"Sytue AI is not available right now. Try again in a moment.")
	errIALimite = httpx.NewError(http.StatusTooManyRequests, "ia_limite",
		"You have reached your Sytue AI limit for now.")
	errIASinInforme = httpx.NewError(http.StatusUnprocessableEntity, "ia_sin_informe",
		"Sytue AI could not turn that into a report. Try saying it another way.")
	errInformeInvalido = httpx.NewError(http.StatusUnprocessableEntity, "informe_invalido",
		"That report is not valid.")
)

// Topes de lo que se lee para un informe.
const (
	// filasPorParte son las filas de una tabla en pantalla; el Excel llega a EXPORT_MAX_FILAS.
	filasPorParte = 1000
	// filasDeOtraApp acota lo que se lee de cada listado de otra app.
	filasDeOtraApp = 10_000
	// filasPorInforme acota las filas de TODAS las fuentes de un informe juntas.
	//
	// ponytail: las filas viven en memoria mientras se calcula el informe y el pod
	// tiene 192 MiB; esto deja el peor caso en unos 60 MB. Si una organización
	// necesita más, llevar el filtrado y el agrupado a SQL.
	filasPorInforme = 60_000
	maxPeticion     = 2000
	maxTextoFicha   = 20_000
	maxPDFFicha     = 10 << 20
)

// maxPaginaOtraApp acota cada página que llega de otra app. Un JSON de 64 MiB (lo
// que había) son varias veces eso ya decodificado: más que el pod entero (NEG-15).
const maxPaginaOtraApp = 8 << 20

var errPDFSinTexto = httpx.NewError(http.StatusUnprocessableEntity, "pdf_sin_texto",
	"This PDF could not be read. If it is a photo or a scan, paste its text instead.")

// clienteApps habla con las otras apps de la suite, con el token de quien pide.
var clienteApps = &http.Client{Timeout: 10 * time.Second}

// tokenDe es el token de la sesión: con él se habla con Sytue AI y con las otras
// apps, que así aplican sus propios permisos a quien pide el informe.
func tokenDe(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

func errorDeIA(err error) error {
	var estado asistente.ErrEstado
	if errors.As(err, &estado) && estado == http.StatusTooManyRequests {
		return errIALimite
	}
	return errIANoDisponible
}

type cuerpoPeticionIA struct {
	Peticion string `json:"peticion"`
	// Para es "tablero" o "excel".
	Para string `json:"para"`
	// Modelo es la fuente que la persona tiene abierta, si tiene alguna.
	Modelo string `json:"modelo"`
}

// ProponerInforme — POST /api/v1/ia/informe.
func (h *Handlers) ProponerInforme(w http.ResponseWriter, r *http.Request) {
	// El WriteTimeout del servidor son 30 s: esta respuesta tiene su propio plazo.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(plazoPropuesta))
	var c cuerpoPeticionIA
	if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c.Peticion = strings.TrimSpace(c.Peticion)
	if c.Peticion == "" || len([]rune(c.Peticion)) > maxPeticion {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Say what you need in up to 2000 characters."))
		return
	}
	fuentes, indice, err := h.fuentes(r.Context(), httpx.LanguageFromRequest(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sobre := ""
	if f, ok := indice[c.Modelo]; ok {
		sobre = f.Clave + " «" + f.Etiqueta + "»"
	}
	para := ia.ParaTablero
	if c.Para == ia.ParaExcel {
		para = ia.ParaExcel
	}
	texto, err := h.ia.Pedir(r.Context(), tokenDe(r), asistente.Peticion{
		Action: "informe", Kind: "erp", Modo: "calidad", Esfuerzo: "high",
		Instruction: ia.InstruccionInforme(para, c.Peticion, sobre, time.Now().UTC()),
		Context:     ia.ContextoFuentes(fuentes),
	})
	if err != nil {
		log.From(r.Context()).Warn("sytue ai: informe sin respuesta", "error", err)
		httpx.Fail(w, r, errorDeIA(err))
		return
	}
	inf, err := ia.InformeDe(texto)
	if err != nil {
		httpx.Fail(w, r, errIASinInforme)
		return
	}
	// Lo que no vale se DESCARTA en vez de rechazar el informe entero: de doce
	// partes, una con un campo inventado no puede tirar las otras once.
	errs := inf.Comprobar(indice)
	validas := make([]ia.Parte, 0, len(inf.Partes))
	for i, p := range inf.Partes {
		if errs[i] == nil && len(validas) < ia.MaxPartes {
			validas = append(validas, p)
		}
	}
	if len(validas) < len(inf.Partes) {
		log.From(r.Context()).Info("sytue ai: partes descartadas", "propuestas", len(inf.Partes), "validas", len(validas))
	}
	if len(validas) == 0 {
		httpx.Fail(w, r, errIASinInforme)
		return
	}
	inf.Partes = validas
	httpx.JSON(w, r, http.StatusOK, map[string]any{"informe": inf})
}

// DatosInforme — POST /api/v1/informes/datos: las tablas de cada parte.
func (h *Handlers) DatosInforme(w http.ResponseWriter, r *http.Request) {
	inf, errs, indice, ok := h.informeDelCuerpo(w, r)
	if !ok {
		return
	}
	liberar, ok := tomarTurnoInforme(r.Context())
	if !ok {
		httpx.Fail(w, r, errInformeOcupado)
		return
	}
	defer liberar()
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"datos": h.tablas(r, inf, errs, indice, filasPorParte),
	})
}

// ExcelInforme — POST /api/v1/informes/excel: el informe como libro de Excel, una
// hoja por parte.
func (h *Handlers) ExcelInforme(w http.ResponseWriter, r *http.Request) {
	inf, errs, indice, ok := h.informeDelCuerpo(w, r)
	if !ok {
		return
	}
	liberar, ok := tomarTurnoInforme(r.Context())
	if !ok {
		httpx.Fail(w, r, errInformeOcupado)
		return
	}
	defer liberar()
	idioma := httpx.LanguageFromRequest(r)
	tope := h.cfg.ExportMaxFilas
	if tope <= 0 {
		tope = config.ExportMaxFilasDefecto
	}
	tablas := h.tablas(r, inf, errs, indice, tope)
	hojas := make([]ia.Hoja, len(inf.Partes))
	for i, p := range inf.Partes {
		t := tablas[i]
		switch {
		case t.Error == ia.ErrorSinPermiso:
			t = aviso(h.cat.Rotulo("aviso.sin_permiso", idioma))
		case t.Error != "":
			t = aviso(h.cat.Rotulo("aviso.no_disponible", idioma))
		case t.Recortada:
			// Igual que en el CSV: quien abre el fichero tiene que ver ahí mismo
			// que le faltan filas.
			t.Filas = append(t.Filas, []any{h.cat.Rotulo("aviso.truncado", idioma) + " " + strconv.Itoa(tope)})
		}
		hojas[i] = ia.Hoja{Nombre: cmp.Or(p.Titulo, inf.Titulo), Tabla: t}
	}
	var buf bytes.Buffer
	if err := ia.EscribirXLSX(&buf, hojas); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="informe-`+time.Now().UTC().Format(time.DateOnly)+`.xlsx"`)
	_, _ = w.Write(buf.Bytes())
}

// turnoInforme deja calcular UN informe a la vez en cada réplica. `filasPorInforme`
// acota uno solo en unos 60 MB; sin turno, tres a la vez (o el mismo botón pulsado
// tres veces) pasaban de los 192 MiB del pod y lo mataba el OOM, con todo lo que
// estuviera atendiendo (NEG-15). El segundo espera a que acabe el primero, dentro
// del plazo de su petición.
var turnoInforme = make(chan struct{}, 1)

var errInformeOcupado = httpx.NewError(http.StatusServiceUnavailable, "informe_ocupado",
	"Another report is being prepared. Try again in a moment.")

func tomarTurnoInforme(ctx context.Context) (func(), bool) {
	select {
	case turnoInforme <- struct{}{}:
		return func() { <-turnoInforme }, true
	case <-ctx.Done():
		return nil, false
	}
}

func aviso(texto string) ia.Tabla {
	return ia.Tabla{Columnas: []ia.Campo{{Clave: "aviso", Tipo: ia.TipoTexto, Etiqueta: texto}}}
}

// informeDelCuerpo lee {informe} y lo comprueba contra las fuentes de HOY: un
// tablero guardado cuyo módulo se apagó sigue abriendo, con esa parte avisada.
func (h *Handlers) informeDelCuerpo(w http.ResponseWriter, r *http.Request) (ia.Informe, []error, map[string]ia.Fuente, bool) {
	var c struct {
		Informe ia.Informe `json:"informe"`
	}
	if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Fail(w, r, err)
		return ia.Informe{}, nil, nil, false
	}
	if n := len(c.Informe.Partes); n == 0 || n > ia.MaxPartes {
		httpx.Fail(w, r, errInformeInvalido)
		return ia.Informe{}, nil, nil, false
	}
	_, indice, err := h.fuentes(r.Context(), httpx.LanguageFromRequest(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return ia.Informe{}, nil, nil, false
	}
	return c.Informe, c.Informe.Comprobar(indice), indice, true
}

// tablas ejecuta cada parte. Cada fuente se lee UNA vez aunque la usen varias.
func (h *Handlers) tablas(r *http.Request, inf ia.Informe, errs []error, indice map[string]ia.Fuente, tope int) []ia.Tabla {
	ctx, idioma := r.Context(), httpx.LanguageFromRequest(r)
	out := make([]ia.Tabla, len(inf.Partes))
	type carga struct {
		filas     []ia.Fila
		recortada bool
		error     string
		// procedencia: solo la trae la fuente de Sytue Bases.
		procedencia map[string]any
	}
	leidas := map[string]carga{}
	restantes := filasPorInforme
	opciones := ia.Opciones{Ahora: time.Now().UTC(), Tope: tope, RotuloCuenta: h.cat.Rotulo("medida.cuenta", idioma)}
	for i, p := range inf.Partes {
		fuente, ok := indice[p.Consulta.Fuente]
		if errs[i] != nil || !ok {
			out[i] = ia.Tabla{Error: ia.ErrorInvalida}
			continue
		}
		c, hay := leidas[fuente.Clave]
		if !hay {
			if fuente.Clave == fuenteBases {
				c.filas, c.recortada, c.error, c.procedencia = h.filasDeBases(ctx, fuente, restantes)
			} else {
				c.filas, c.recortada, c.error = h.leer(ctx, tokenDe(r), fuente, restantes)
			}
			restantes = max(restantes-len(c.filas), 0)
			leidas[fuente.Clave] = c
		}
		if c.error != "" {
			out[i] = ia.Tabla{Error: c.error}
			continue
		}
		t := p.Consulta.Ejecutar(fuente, c.filas, opciones)
		// Calculado sobre una fuente cortada también es «recortado»: una suma de
		// las primeras cincuenta mil facturas no es la facturación.
		t.Recortada = t.Recortada || c.recortada
		t.Procedencia = c.procedencia
		out[i] = t
	}
	return out
}

// --- Fuentes ------------------------------------------------------------------

// fuentes son los modelos de los módulos ENCENDIDOS, el libro diario y los
// listados de las otras apps que este despliegue tiene configuradas.
func (h *Handlers) fuentes(ctx context.Context, idioma string) ([]ia.Fuente, map[string]ia.Fuente, error) {
	activos, err := h.store.ModulosActivos(ctx)
	if err != nil {
		return nil, nil, err
	}
	out, indice := h.fuentesDe(activos, idioma)
	// Lo que la persona no puede leer no es fuente: ni la IA lo ve ni un informe
	// guardado por otra persona lo enseña.
	visibles := out[:0]
	for _, f := range out {
		modulo := h.moduloDe(f.Clave)
		if f.Clave == fuenteLibro {
			modulo = "contabilidad"
		}
		if f.App != "" || (modulo != "" && h.puede(ctx, modulo, permisos.Lectura)) {
			visibles = append(visibles, f)
		} else {
			delete(indice, f.Clave)
		}
	}
	if f, ok := h.fuenteDeBases(ctx); ok {
		visibles = append(visibles, f)
		indice[f.Clave] = f
	}
	return visibles, indice, nil
}

// fuentesDe arma las fuentes para unos módulos encendidos, sin tocar la base.
func (h *Handlers) fuentesDe(activos []string, idioma string) ([]ia.Fuente, map[string]ia.Fuente) {
	var out []ia.Fuente
	indice := map[string]ia.Fuente{}
	anadir := func(f ia.Fuente) {
		if _, repetida := indice[f.Clave]; !repetida {
			indice[f.Clave] = f
			out = append(out, f)
		}
	}
	for _, mod := range h.cat.Modulos() {
		if !slices.Contains(activos, mod.Clave) {
			continue
		}
		for _, m := range mod.Modelos {
			anadir(h.fuenteDeModelo(m, idioma))
		}
	}
	anadir(ia.Fuente{Clave: fuenteLibro, Etiqueta: h.cat.Rotulo("fuente.apuntes", idioma), Campos: []ia.Campo{
		{Clave: "fecha", Tipo: ia.TipoFecha, Etiqueta: h.cat.Rotulo("campo.fecha", idioma)},
		{Clave: "diario", Tipo: ia.TipoTexto, Etiqueta: h.cat.Rotulo("campo.diario", idioma)},
		{Clave: "ref", Tipo: ia.TipoTexto, Etiqueta: h.cat.Rotulo("campo.ref", idioma)},
		{Clave: "concepto", Tipo: ia.TipoTexto, Etiqueta: h.cat.Rotulo("campo.descripcion", idioma)},
		{Clave: "cuenta", Tipo: ia.TipoTexto, Etiqueta: h.cat.Rotulo("campo.cuenta", idioma)},
		{Clave: "debe", Tipo: ia.TipoDinero, Etiqueta: h.cat.Rotulo("campo.debe", idioma)},
		{Clave: "haber", Tipo: ia.TipoDinero, Etiqueta: h.cat.Rotulo("campo.haber", idioma)},
	}})
	for _, e := range otrasApps {
		if e.base(h.cfg) == "" {
			continue
		}
		f := ia.Fuente{Clave: e.clave, Etiqueta: h.cat.Rotulo(e.rotulo, idioma), App: e.app}
		for _, c := range e.campos {
			f.Campos = append(f.Campos, ia.Campo{Clave: c.clave, Tipo: c.tipo, Etiqueta: h.cat.Rotulo(c.rotulo, idioma)})
		}
		anadir(f)
	}
	return out, indice
}

const fuenteLibro = "apuntes"

// fuenteDeModelo describe un modelo del catálogo como fuente de informes.
func (h *Handlers) fuenteDeModelo(m *catalogo.Modelo, idioma string) ia.Fuente {
	v := h.cat.VistaModelo(m, idioma)
	f := ia.Fuente{Clave: m.Clave, Etiqueta: v.Etiqueta}
	if v.Numerado {
		f.Campos = append(f.Campos, ia.Campo{Clave: "ref", Tipo: ia.TipoTexto, Etiqueta: h.cat.Rotulo("campo.ref", idioma)})
	}
	for _, c := range v.Campos {
		campo := ia.Campo{Clave: c.Clave, Tipo: tipoDeInforme(c.Tipo), Etiqueta: c.Etiqueta}
		for _, o := range c.Opciones {
			if campo.Valores == nil {
				campo.Valores = map[string]string{}
			}
			campo.Valores[o.Valor] = o.Etiqueta
		}
		f.Campos = append(f.Campos, campo)
	}
	if len(v.Estados) > 0 {
		estado := ia.Campo{Clave: "estado", Tipo: ia.TipoTexto, Etiqueta: h.cat.Rotulo("campo.estado", idioma), Valores: map[string]string{}}
		for _, e := range v.Estados {
			estado.Valores[e.Valor] = e.Etiqueta
		}
		f.Campos = append(f.Campos, estado)
	}
	return f
}

func tipoDeInforme(tipo string) string {
	switch tipo {
	case catalogo.TipoEntero, catalogo.TipoDecimal:
		return ia.TipoNumero
	case catalogo.TipoDinero:
		return ia.TipoDinero
	case catalogo.TipoFecha, catalogo.TipoFechaHora:
		return ia.TipoFecha
	case catalogo.TipoBooleano:
		return ia.TipoBooleano
	}
	return ia.TipoTexto
}

// leer trae hasta `restantes` filas de una fuente: del store si es del ERP, de su
// app si no. Devuelve el código de error de la tabla si no se pudo.
func (h *Handlers) leer(ctx context.Context, token string, f ia.Fuente, restantes int) ([]ia.Fila, bool, string) {
	if restantes <= 0 {
		return nil, true, ""
	}
	tope := h.cfg.ExportMaxFilas
	if tope <= 0 {
		tope = config.ExportMaxFilasDefecto
	}
	tope = min(tope, restantes)
	if f.App != "" {
		for _, e := range otrasApps {
			if e.clave == f.Clave {
				return e.leer(ctx, h.cfg, token, min(filasDeOtraApp, restantes))
			}
		}
	}
	var filas []ia.Fila
	var err error
	recortada := false
	if f.Clave == fuenteLibro {
		filas, recortada, err = h.filasDelLibro(ctx, tope)
	} else if m, ok := h.cat.Modelo(f.Clave); ok {
		filas, recortada, err = h.filasDeModelo(ctx, m, tope)
	}
	if err != nil {
		log.From(ctx).Warn("sytue ai: no se pudo leer la fuente", "fuente", f.Clave, "error", err)
		return nil, false, ia.ErrorNoDisponible
	}
	return filas, recortada, ""
}

// filasDeModelo recorre el modelo por tramos, como la exportación CSV, con cada
// valor en su tipo: el dinero en céntimos, la relación por su nombre.
func (h *Handlers) filasDeModelo(ctx context.Context, m *catalogo.Modelo, tope int) ([]ia.Fila, bool, error) {
	f := store.Filtro{Modelo: m.Clave, Limite: filasPorTramo}
	var out []ia.Fila
	for {
		pag, err := h.store.Listar(ctx, f)
		if err != nil {
			return nil, false, err
		}
		etiquetas, err := h.store.ResolverEtiquetas(ctx, idsReferenciados(m, pag.Registros))
		if err != nil {
			return nil, false, err
		}
		for _, reg := range pag.Registros {
			if len(out) == tope {
				return out, true, nil
			}
			out = append(out, filaDe(m, reg, etiquetas))
		}
		if pag.Siguiente == "" {
			return out, false, nil
		}
		f.Cursor = pag.Siguiente
	}
}

func filaDe(m *catalogo.Modelo, reg store.Registro, etiquetas map[string]string) ia.Fila {
	fila := ia.Fila{"ref": reg.Ref, "estado": reg.Estado}
	for _, c := range m.Campos {
		v := reg.Data[c.Clave]
		switch c.Tipo {
		case catalogo.TipoDinero:
			if cents, ok := centimos(v); ok {
				fila[c.Clave] = cents
			}
		case catalogo.TipoEntero, catalogo.TipoDecimal:
			if x, err := strconv.ParseFloat(texto(v), 64); err == nil {
				fila[c.Clave] = x
			}
		case catalogo.TipoBooleano:
			if b, ok := v.(bool); ok {
				fila[c.Clave] = b
			}
		case catalogo.TipoRelacion:
			id, _ := v.(string)
			fila[c.Clave] = legible(etiquetas, id)
		default:
			if v != nil {
				fila[c.Clave] = texto(v)
			}
		}
	}
	return fila
}

func (h *Handlers) filasDelLibro(ctx context.Context, tope int) ([]ia.Fila, bool, error) {
	lineas, err := h.store.LineasDeLibro(ctx, tope+1)
	if err != nil {
		return nil, false, err
	}
	recortada := len(lineas) > tope
	out := make([]ia.Fila, 0, min(len(lineas), tope))
	for _, l := range lineas[:min(len(lineas), tope)] {
		out = append(out, ia.Fila{
			"fecha": l.Fecha.Format(time.DateOnly), "diario": l.Diario, "ref": l.Ref,
			"concepto": l.Concepto, "cuenta": l.Cuenta, "debe": l.Debe, "haber": l.Haber,
		})
	}
	return out, recortada, nil
}

// --- Otras apps -----------------------------------------------------------------

// otraApp es un listado de otra app de la suite que un informe puede leer. Se lee
// con el token de quien pide, así que cada app decide qué le enseña: el
// directorio de Sytue ID, por ejemplo, solo a quien puede administrar personas.
type otraApp struct {
	clave, app, rotulo string
	base               func(config.Config) string
	// ruta es el listado; lista, la clave del array en la respuesta; siguiente y
	// param, de dónde sale y cómo se pide la página siguiente (vacíos: una sola).
	ruta, lista, siguiente, param string
	campos                        []campoDeApp
}

type campoDeApp struct {
	clave, tipo, rotulo string
	// ruta es dónde está el valor en cada objeto ("emails.0.email").
	ruta string
}

var otrasApps = []otraApp{
	{
		clave: "personas", app: "Sytue ID", rotulo: "fuente.personas",
		base: func(c config.Config) string { return c.IDBaseURL },
		ruta: "/api/v1/admin/users?limit=200", lista: "items", siguiente: "nextCursor", param: "cursor",
		campos: []campoDeApp{
			{"nombre", ia.TipoTexto, "campo.nombre", "name"},
			{"correo", ia.TipoTexto, "campo.correo", "email"},
			{"estado", ia.TipoTexto, "campo.estado", "status"},
			{"departamento", ia.TipoTexto, "campo.departamento", "department"},
			{"puesto", ia.TipoTexto, "campo.puesto", "title"},
			{"ubicacion", ia.TipoTexto, "campo.ubicacion", "location"},
			{"alta", ia.TipoFecha, "campo.fecha", "created_at"},
		},
	},
	{
		clave: "grupos", app: "Sytue ID", rotulo: "fuente.grupos",
		base: func(c config.Config) string { return c.IDBaseURL },
		ruta: "/api/v1/admin/groups", lista: "groups",
		campos: []campoDeApp{
			{"nombre", ia.TipoTexto, "campo.nombre", "name"},
			{"descripcion", ia.TipoTexto, "campo.descripcion", "description"},
			{"miembros", ia.TipoNumero, "campo.miembros", "member_count"},
		},
	},
	{
		clave: "canales", app: "Sytue Rooms", rotulo: "fuente.canales",
		base: func(c config.Config) string { return c.RoomsBaseURL },
		ruta: "/api/v1/admin/channels", lista: "items",
		campos: []campoDeApp{
			{"nombre", ia.TipoTexto, "campo.nombre", "name"},
			{"equipo", ia.TipoTexto, "campo.equipo", "team_name"},
			{"tipo", ia.TipoTexto, "campo.tipo", "kind"},
			{"miembros", ia.TipoNumero, "campo.miembros", "members"},
			{"alta", ia.TipoFecha, "campo.fecha", "created_at"},
		},
	},
	{
		clave: "recursos", app: "Sytue Calendar", rotulo: "fuente.recursos",
		base: func(c config.Config) string { return c.CalendarBaseURL },
		ruta: "/api/v1/resources", lista: "resources",
		campos: []campoDeApp{
			{"nombre", ia.TipoTexto, "campo.nombre", "name"},
			{"tipo", ia.TipoTexto, "campo.tipo", "kind"},
			{"capacidad", ia.TipoNumero, "campo.capacidad", "capacity"},
			{"ubicacion", ia.TipoTexto, "campo.ubicacion", "location"},
			{"correo", ia.TipoTexto, "campo.correo", "email"},
		},
	},
	{
		clave: "agenda", app: "Sytue Contacts", rotulo: "fuente.agenda",
		base: func(c config.Config) string { return c.ContactsBaseURL },
		ruta: "/api/v1/contacts/all?limit=500", lista: "contacts", siguiente: "next_offset", param: "offset",
		campos: []campoDeApp{
			{"nombre", ia.TipoTexto, "campo.nombre", "formatted_name"},
			{"correo", ia.TipoTexto, "campo.correo", "emails.0.email"},
			{"telefono", ia.TipoTexto, "campo.telefono", "phones.0.phone"},
			{"empresa", ia.TipoTexto, "campo.empresa", "organization"},
			{"puesto", ia.TipoTexto, "campo.puesto", "title"},
		},
	},
}

// leer recorre el listado página a página hasta `tope` filas.
func (e otraApp) leer(ctx context.Context, cfg config.Config, token string, tope int) ([]ia.Fila, bool, string) {
	var filas []ia.Fila
	siguiente := ""
	for {
		u := strings.TrimRight(e.base(cfg), "/") + e.ruta
		if siguiente != "" {
			sep := "?"
			if strings.Contains(u, "?") {
				sep = "&"
			}
			u += sep + e.param + "=" + url.QueryEscape(siguiente)
		}
		cuerpo, codigo := pedirListado(ctx, u, token)
		if codigo != "" {
			return nil, false, codigo
		}
		lista, _ := cuerpo[e.lista].([]any)
		for _, x := range lista {
			if len(filas) == tope {
				return filas, true, ""
			}
			obj, _ := x.(map[string]any)
			fila := ia.Fila{}
			for _, c := range e.campos {
				if v := valorDeApp(c.tipo, enRuta(obj, c.ruta)); v != nil {
					fila[c.clave] = v
				}
			}
			filas = append(filas, fila)
		}
		siguiente = textoDeJSON(cuerpo[e.siguiente])
		if e.siguiente == "" || siguiente == "" || len(lista) == 0 {
			return filas, false, ""
		}
	}
}

func pedirListado(ctx context.Context, u, token string) (map[string]any, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, ia.ErrorNoDisponible
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := clienteApps.Do(req)
	if err != nil {
		log.From(ctx).Warn("sytue ai: otra app no responde", "url", u, "error", err)
		return nil, ia.ErrorNoDisponible
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, ia.ErrorSinPermiso
	case resp.StatusCode != http.StatusOK:
		return nil, ia.ErrorNoDisponible
	}
	var cuerpo map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPaginaOtraApp)).Decode(&cuerpo); err != nil {
		return nil, ia.ErrorNoDisponible
	}
	return cuerpo, ""
}

// enRuta saca un valor anidado: "emails.0.email" es el correo del primer email.
func enRuta(v any, ruta string) any {
	for _, paso := range strings.Split(ruta, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[paso]
		case []any:
			i, err := strconv.Atoi(paso)
			if err != nil || i < 0 || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	return v
}

func valorDeApp(tipo string, v any) any {
	switch tipo {
	case ia.TipoNumero:
		if x, ok := v.(float64); ok {
			return x
		}
		return nil
	case ia.TipoFecha:
		if s, ok := v.(string); ok && len(s) >= 10 {
			return s[:10]
		}
		return nil
	}
	if v == nil {
		return nil
	}
	return textoDeJSON(v)
}

func textoDeJSON(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return texto(v)
}

// --- Tableros guardados -----------------------------------------------------------

// Tableros — GET /api/v1/tableros.
func (h *Handlers) Tableros(w http.ResponseWriter, r *http.Request) {
	ts, err := h.store.Tableros(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"tableros": ts})
}

// GuardarTablero — POST /api/v1/tableros {informe}.
func (h *Handlers) GuardarTablero(w http.ResponseWriter, r *http.Request) {
	inf, errs, _, ok := h.informeDelCuerpo(w, r)
	if !ok {
		return
	}
	if slices.ContainsFunc(errs, func(e error) bool { return e != nil }) {
		httpx.Fail(w, r, errInformeInvalido)
		return
	}
	crudo, err := json.Marshal(inf)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := h.store.GuardarTablero(r.Context(), cmp.Or(inf.Titulo, "Sytue AI"), crudo, sub(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"tablero": t})
}

// BorrarTablero — DELETE /api/v1/tableros/{id}. Quien lo creó, o quien administra.
func (h *Handlers) BorrarTablero(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	c, _ := authn.FromContext(r.Context())
	err := h.store.BorrarTablero(r.Context(), id, sub(r), c.HasAnyRole("owner", "admin"))
	if errors.Is(err, store.ErrNoEncontrado) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Fichas -------------------------------------------------------------------------

// RellenarFicha — POST /api/v1/{modelo}/ia/rellenar {texto}: los campos de una
// ficha nueva a partir de un texto pegado. No guarda nada: devuelve `data` para
// el formulario, que la persona revisa antes de guardar.
func (h *Handlers) RellenarFicha(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	var c struct {
		Texto string `json:"texto"`
	}
	// El PDF de la factura, tal cual: se lee con Sytue PDF y sigue como un texto pegado.
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/pdf") {
		pdf, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPDFFicha))
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Send a PDF of up to 10 MB."))
			return
		}
		if c.Texto, err = textoDePDF(r.Context(), h.cfg.DocsBaseURL, tokenDe(r), pdf); err != nil {
			log.From(r.Context()).Warn("sytue ai: PDF sin leer", "modelo", m.Clave, "error", err)
			httpx.Fail(w, r, errPDFSinTexto)
			return
		}
		c.Texto = validation.TruncateRunes(c.Texto, maxTextoFicha)
	} else if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c.Texto = strings.TrimSpace(c.Texto)
	if c.Texto == "" || len(c.Texto) > maxTextoFicha {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Paste the text to read, up to 20000 characters."))
		return
	}
	vista := h.cat.VistaModelo(m, httpx.LanguageFromRequest(r))
	texto, err := h.ia.Pedir(r.Context(), tokenDe(r), asistente.Peticion{
		Action: "rellenar", Kind: "erp", Modo: "interactivo",
		Instruction: ia.InstruccionFicha(vista, time.Now().UTC()),
		Context:     c.Texto,
	})
	if err != nil {
		log.From(r.Context()).Warn("sytue ai: ficha sin respuesta", "modelo", m.Clave, "error", err)
		httpx.Fail(w, r, errorDeIA(err))
		return
	}
	data, relaciones := ia.FichaDe(m, vista, texto)
	etiquetas := map[string]string{}
	for clave, nombre := range relaciones {
		campo, _ := campoDe(m, clave)
		pag, err := h.store.Listar(r.Context(), store.Filtro{Modelo: campo.Modelo, Texto: nombre, Limite: 1})
		if err != nil || len(pag.Registros) == 0 {
			continue
		}
		reg := pag.Registros[0]
		data[clave], etiquetas[reg.ID] = reg.ID, cmp.Or(reg.Etiqueta, reg.Ref)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"data": data, "etiquetas": etiquetas})
}

// textoDePDF lee el texto de un PDF con Sytue PDF (el servicio de Docs), con el
// token de quien pide: el ERP no lleva un lector de PDF propio. Sin texto (un
// escaneo) es un error, porque no hay nada que leer.
func textoDePDF(ctx context.Context, base, token string, pdf []byte) (string, error) {
	var cuerpo bytes.Buffer
	formulario := multipart.NewWriter(&cuerpo)
	parte, err := formulario.CreateFormFile("file", "documento.pdf")
	if err != nil {
		return "", err
	}
	if _, err := parte.Write(pdf); err != nil {
		return "", err
	}
	if err := formulario.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/pdf/text", &cuerpo)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", formulario.FormDataContentType())
	resp, err := clienteApps.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sytue pdf: HTTP %d", resp.StatusCode)
	}
	var leido struct {
		Paginas [][]fragmentoPDF `json:"paginas"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&leido); err != nil {
		return "", err
	}
	texto := lineasDePDF(leido.Paginas)
	if texto == "" {
		return "", errors.New("sytue pdf: el documento no tiene texto")
	}
	return texto, nil
}

type fragmentoPDF struct {
	Texto string  `json:"texto"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

// lineasDePDF pone los fragmentos de cada página en orden de lectura: de arriba
// abajo (en PDF la y crece hacia arriba) y, en la misma línea, de izquierda a
// derecha. Un fragmento a menos de 3 puntos de altura del primero de la línea va
// en esa línea.
func lineasDePDF(paginas [][]fragmentoPDF) string {
	var b strings.Builder
	for _, fragmentos := range paginas {
		slices.SortStableFunc(fragmentos, func(a, c fragmentoPDF) int { return cmp.Compare(c.Y, a.Y) })
		var lineas [][]fragmentoPDF
		for _, f := range fragmentos {
			if n := len(lineas); n > 0 && math.Abs(lineas[n-1][0].Y-f.Y) <= 3 {
				lineas[n-1] = append(lineas[n-1], f)
			} else {
				lineas = append(lineas, []fragmentoPDF{f})
			}
		}
		for i, linea := range lineas {
			slices.SortStableFunc(linea, func(a, c fragmentoPDF) int { return cmp.Compare(a.X, c.X) })
			if i > 0 {
				b.WriteByte('\n')
			}
			for j, f := range linea {
				if j > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(strings.TrimSpace(f.Texto))
			}
		}
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}
