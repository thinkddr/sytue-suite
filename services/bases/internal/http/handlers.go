// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

type nombreDescripcion struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handlers) listarBases(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	bs, err := h.Store.ListarBases(r.Context(), a)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, bs)
}

// buscar responde a la búsqueda federada del Hub: {"items":[…]}, solo bases a las que
// quien busca tiene acceso.
func (h *Handlers) buscar(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limite, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	bs, err := h.Store.BuscarBases(r.Context(), a, r.URL.Query().Get("q"), limite)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": bs})
}

func (h *Handlers) crearBase(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in nombreDescripcion
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	b, err := h.Store.CrearBase(r.Context(), a, in.Name, in.Description)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, b)
}

func (h *Handlers) obtenerBase(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	b, err := h.Store.ObtenerBase(r.Context(), a, bid)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, b)
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func (h *Handlers) actualizarBase(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in nombreDescripcion
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.ActualizarBase(r.Context(), a, bid, in.Name, in.Description))
}

func (h *Handlers) borrarBase(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.BorrarBase(r.Context(), a, bid))
}

func (h *Handlers) auditoria(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	antes, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limite, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	es, err := h.Store.ListarAuditoria(r.Context(), a, bid, max(antes, 0), limite)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, es)
}

// --- permisos ---

func (h *Handlers) listarPermisos(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ps, err := h.Store.ListarPermisos(r.Context(), a, bid)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ps)
}

func (h *Handlers) revocarPermiso(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	pid, err3 := id(r, "pid")
	if err = firstErr(err, err2, err3); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.RevocarPermiso(r.Context(), a, bid, pid))
}

// --- tablas y campos ---

func (h *Handlers) listarTablas(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ts, err := h.Store.ListarTablas(r.Context(), a, bid)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ts)
}

func (h *Handlers) crearTabla(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in nombreDescripcion
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := h.Store.CrearTabla(r.Context(), a, bid, in.Name)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, t)
}

func (h *Handlers) importar(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := domain.LeerCSV(http.MaxBytesReader(w, r.Body, maxCuerpoImport))
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	tabla, campos, err := h.Store.ImportarTabla(r.Context(), a, bid, r.URL.Query().Get("name"), t)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, store.TablaConCampos{Tabla: tabla, Fields: campos})
}

func (h *Handlers) renombrarTabla(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	tid, err2 := id(r, "tid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in nombreDescripcion
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.RenombrarTabla(r.Context(), a, tid, in.Name))
}

func (h *Handlers) borrarTabla(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	tid, err2 := id(r, "tid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.BorrarTabla(r.Context(), a, tid))
}

func (h *Handlers) crearCampo(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	tid, err2 := id(r, "tid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in domain.EntradaCampo
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.Store.CrearCampo(r.Context(), a, tid, in)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, c)
}

func (h *Handlers) renombrarCampo(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	fid, err2 := id(r, "fid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// {"name": …} renombra; {"formula": …} cambia la expresión de un campo de fórmula.
	var in struct {
		Name    string  `json:"name"`
		Formula *string `json:"formula"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Formula != nil {
		if err := h.Store.CambiarFormula(r.Context(), a, fid, *in.Formula); err != nil || in.Name == "" {
			h.hecho(w, r, err)
			return
		}
	}
	h.hecho(w, r, h.Store.RenombrarCampo(r.Context(), a, fid, in.Name))
}

func (h *Handlers) borrarCampo(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	fid, err2 := id(r, "fid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.BorrarCampo(r.Context(), a, fid))
}

// --- registros ---

type entradaRegistro struct {
	Rev   int64                      `json:"rev"`
	Data  map[string]json.RawMessage `json:"data"`
	Links map[string][]string        `json:"links"`
}

func (h *Handlers) listarRegistros(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	tid, err2 := id(r, "tid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	vista := ""
	if v := r.URL.Query().Get("view"); v != "" {
		if vista, err = idDe(v); err != nil {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
	}
	limite, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	p, err := h.Store.ListarRegistros(r.Context(), a, tid, vista, limite, r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

func (h *Handlers) crearRegistro(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	tid, err2 := id(r, "tid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in entradaRegistro
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	reg, err := h.Store.CrearRegistro(r.Context(), a, tid, in.Data, in.Links)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, reg)
}

func (h *Handlers) actualizarRegistro(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	rid, err2 := id(r, "rid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in entradaRegistro
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	reg, err := h.Store.ActualizarRegistro(r.Context(), a, rid, in.Rev, in.Data, in.Links)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, reg)
}

func (h *Handlers) borrarRegistro(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	rid, err2 := id(r, "rid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.BorrarRegistro(r.Context(), a, rid, r.URL.Query().Get("force") == "true"))
}

// --- vistas ---

type entradaVista struct {
	Name   string             `json:"name"`
	Kind   domain.TipoVista   `json:"kind"`
	Config domain.ConfigVista `json:"config"`
}

func (h *Handlers) listarVistas(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	tid, err2 := id(r, "tid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	vs, err := h.Store.ListarVistas(r.Context(), a, tid)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, vs)
}

func (h *Handlers) crearVista(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	tid, err2 := id(r, "tid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in entradaVista
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := h.Store.CrearVista(r.Context(), a, tid, in.Name, in.Kind, in.Config)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, v)
}

func (h *Handlers) borrarVista(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	vid, err2 := id(r, "vid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.BorrarVista(r.Context(), a, vid))
}

// --- exportación ---

var noNombreFichero = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func (h *Handlers) exportar(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	tid, err2 := id(r, "tid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	formato := r.URL.Query().Get("format")
	if formato != "csv" && formato != "json" {
		httpx.Fail(w, r, errorAPI(domain.ErrInvalido))
		return
	}
	// Por tramos: la tabla no cabe en memoria (ya no tiene el techo de 50.000). La cabecera
	// sale con el primero, así que un 403 o un 404 siguen siendo un estado y no un fichero.
	// Una tabla grande no sale en los 30 s de escritura del servidor.
	conPlazoDeTransferencia(w)
	ex := exportador{w: w, formato: formato}
	if err := h.Store.ExportarTabla(r.Context(), a, tid, ex.tramo); err != nil {
		if ex.campos == nil {
			httpx.Fail(w, r, errorAPI(err))
			return
		}
		// Ya hay medio fichero enviado: se corta la conexión para que la descarga FALLE,
		// en vez de cerrar bien un fichero al que le faltan registros.
		panic(http.ErrAbortHandler)
	}
	if ex.csv == nil {
		_, _ = w.Write([]byte("]}\n"))
	}
}

// exportador escribe una exportación tramo a tramo, en CSV o en JSON.
type exportador struct {
	w       http.ResponseWriter
	formato string
	campos  []domain.Campo // los del primer tramo: la cabecera ya está escrita con ellos
	csv     *csv.Writer
	n       int
}

func (e *exportador) cabecera(ex store.Exportacion) {
	if e.campos = ex.Fields; e.campos == nil {
		e.campos = []domain.Campo{}
	}
	nombre := strings.Trim(noNombreFichero.ReplaceAllString(ex.Tabla, "_"), "_")
	if nombre == "" {
		nombre = "tabla"
	}
	e.w.Header().Set("Content-Disposition", `attachment; filename="`+nombre+`.`+e.formato+`"`)
	e.w.Header().Set("X-Content-Type-Options", "nosniff")
	if e.formato == "json" {
		e.w.Header().Set("Content-Type", "application/json; charset=utf-8")
		cab, _ := json.Marshal(map[string]any{"table": ex.Tabla, "fields": e.campos})
		// {"fields":…,"table":…,"records":[ … ]}
		_, _ = e.w.Write(append(cab[:len(cab)-1], []byte(`,"records":[`)...))
		return
	}
	e.w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	_, _ = e.w.Write([]byte("\xef\xbb\xbf"))
	e.csv = csv.NewWriter(e.w)
	_ = e.csv.Write(cabeceraExportada(e.campos))
}

func (e *exportador) tramo(ex store.Exportacion) error {
	if e.campos == nil {
		e.cabecera(ex)
	}
	if e.csv != nil {
		for _, rg := range ex.Records {
			if err := e.csv.Write(filaExportada(e.campos, rg, ex.Labels)); err != nil {
				return err
			}
		}
		e.csv.Flush()
		return e.csv.Error()
	}
	for _, rg := range ex.Records {
		fila, _ := json.Marshal(registroExportado{rg, valoresDeEnlaces(e.campos, rg, ex.Labels)})
		if e.n > 0 {
			fila = append([]byte{','}, fila...)
		}
		if _, err := e.w.Write(fila); err != nil {
			return err
		}
		e.n++
	}
	return nil
}

// registroExportado es un registro en la exportación JSON: además de los ids de sus
// enlaces (`links`), el texto de cada registro enlazado (`link_values`, en el mismo orden).
type registroExportado struct {
	domain.Registro
	LinkValues map[string][]string `json:"link_values"`
}

// valoresDeEnlaces da, por campo de enlace, el texto de los registros enlazados.
func valoresDeEnlaces(campos []domain.Campo, rg domain.Registro, etiquetas map[string]string) map[string][]string {
	out := map[string][]string{}
	for _, f := range campos {
		if f.Kind == domain.TipoRelacion && len(rg.Links[f.ID]) > 0 {
			out[f.ID] = textosDe(rg.Links[f.ID], etiquetas)
		}
	}
	return out
}

func textosDe(ids []string, etiquetas map[string]string) []string {
	vs := make([]string, len(ids))
	for i, id := range ids {
		vs[i] = etiquetas[id]
	}
	return vs
}

// cabeceraExportada y filaExportada son la fila de títulos y la de un registro del CSV.
// Un campo de enlace ocupa DOS columnas: la suya, con el texto de los registros enlazados
// (lo que una persona quiere leer), y «<nombre> (id)», con sus ids (lo que una máquina
// necesita para volver a casarlos).
func cabeceraExportada(campos []domain.Campo) []string {
	cab := make([]string, 0, len(campos))
	for _, f := range campos {
		cab = append(cab, domain.CeldaCSV(f.Name))
		if f.Kind == domain.TipoRelacion {
			cab = append(cab, domain.CeldaCSV(f.Name+" (id)"))
		}
	}
	return cab
}

func filaExportada(campos []domain.Campo, rg domain.Registro, etiquetas map[string]string) []string {
	fila := make([]string, 0, len(campos))
	for _, f := range campos {
		if f.Kind != domain.TipoRelacion {
			fila = append(fila, celdaExportada(f, rg))
			continue
		}
		// El texto de un registro enlazado lo escribió alguien: pasa por CeldaCSV como
		// cualquier otro texto. Los ids son UUID y no pueden ser una fórmula.
		fila = append(fila,
			domain.CeldaCSV(strings.Join(textosDe(rg.Links[f.ID], etiquetas), "; ")),
			strings.Join(rg.Links[f.ID], ";"))
	}
	return fila
}

// celdaExportada pasa el valor de un registro a texto de CSV. Solo lo que es texto
// del usuario pasa por domain.CeldaCSV: un número negativo no es una fórmula.
func celdaExportada(f domain.Campo, rg domain.Registro) string {
	if f.Kind == domain.TipoAdjunto {
		// El nombre de un fichero lo puso quien lo subió: se neutraliza como cualquier texto.
		return domain.CeldaCSV(nombresDeAdjuntos(rg.Attachments[f.ID]))
	}
	raw := rg.Data[f.ID]
	if len(raw) == 0 {
		return ""
	}
	switch f.Efectivo().Kind {
	case domain.TipoNumero:
		var n float64
		if json.Unmarshal(raw, &n) == nil {
			return strconv.FormatFloat(n, 'f', -1, 64)
		}
	case domain.TipoCasilla:
		var b bool
		if json.Unmarshal(raw, &b) == nil {
			return strconv.FormatBool(b)
		}
	case domain.TipoMultiple:
		var l []string
		if json.Unmarshal(raw, &l) == nil {
			return domain.CeldaCSV(strings.Join(l, "; "))
		}
	default:
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return domain.CeldaCSV(s)
		}
	}
	return ""
}
