// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package powerbi es el driver TabularSource para Power BI vía executeQueries
// (POST /v1.0/myorg/datasets/{datasetId}/executeQueries): ejecuta una consulta DAX
// contra un dataset y trae el resultado a Sytue Sheets como tabla. Implementa
// connector.Driver (Test) y la capacidad opcional connector.TabularSource (Query).
//
// La respuesta de executeQueries anida results[0].tables[0].rows, donde cada fila
// es un objeto {"Tabla[Columna]": valor}. Este driver APLANA ese shape: las
// columnas son las claves (en el orden de la primera fila, no alfabético, para
// respetar el orden del EVALUATE) y las filas, los valores; la normalización a
// Table (tipos incluidos) la hace connector.BuildTable, ya que DAX devuelve valores
// JSON tipados (número/texto/bool/null).
//
// Este es el plano de DATOS (L3, DAX→tabla), NO el embed de un informe (esa es otra
// ola). Auth: el Bearer de Entra sale del vault y lo inyecta rt.HTTP como cabecera;
// el driver NUNCA lo ve ni construye su propio http.Client. Egress por rt.HTTP
// (SSRF + rate-limit + auditoría).
package powerbi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

// Kind es la clave del Registry con la que se registra este driver.
const Kind = "powerbi"

// Claves de Instance.Config (públicas). El Bearer de Entra va en
// Instance.Secrets["bearer"] y lo inyecta el Runtime como cabecera (no es config).
const (
	ConfigAPIBase   = "api_base"   // opcional: raíz de la API (nubes soberanas); por defecto https://api.powerbi.com
	ConfigDatasetID = "dataset_id" // opcional: dataset por defecto (la spec puede sobrescribirlo)
)

const (
	defaultAPIBase = "https://api.powerbi.com"
	maxRespBytes   = 32 << 20 // tope de lectura de la respuesta (defensa anti-OOM)
	defaultMaxRows = 10000    // tope de filas materializadas si la spec no lo fija
)

// Driver es el conector Power BI (datos DAX). Sin estado: cada operación usa el
// Runtime inyectado. Su valor cero es utilizable; New lo devuelve por simetría.
type Driver struct{}

// New construye el driver (se cablea en el Registry del gateway).
func New() *Driver { return &Driver{} }

// Kind identifica al driver en el Registry.
func (*Driver) Kind() string { return Kind }

// Test verifica auth + conectividad haciendo GET al dataset configurado (o al
// listado de datasets si no hay uno fijado), usando EXCLUSIVAMENTE rt.HTTP.
func (*Driver) Test(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	base, err := apiBase(inst)
	if err != nil {
		return err
	}
	u := base + "/v1.0/myorg/datasets"
	if ds := strings.TrimSpace(inst.Config[ConfigDatasetID]); ds != "" {
		u += "/" + url.PathEscape(ds)
	}
	if err := do(ctx, rt, http.MethodGet, u, nil, nil); err != nil {
		return fmt.Errorf("powerbi: test: %w", err)
	}
	return nil
}

// Query ejecuta una consulta DAX y devuelve la Table. spec es powerbiQuery (query
// DAX + dataset_id). Aplana results[0].tables[0].rows. Egress por rt.HTTP.
func (*Driver) Query(ctx context.Context, inst connector.Instance, rt connector.Runtime, spec json.RawMessage) (connector.Table, error) {
	base, err := apiBase(inst)
	if err != nil {
		return connector.Table{}, err
	}
	var q powerbiQuery
	if len(spec) > 0 {
		if err := json.Unmarshal(spec, &q); err != nil {
			return connector.Table{}, fmt.Errorf("%w: la spec no es JSON válido", connector.ErrBadQuery)
		}
	}
	dataset := firstNonEmpty(q.DatasetID, q.Dataset, inst.Config[ConfigDatasetID])
	if err := q.validate(dataset); err != nil {
		return connector.Table{}, err
	}

	reqBody := map[string]any{
		"queries":            []any{map[string]any{"query": q.dax()}},
		"serializerSettings": map[string]any{"includeNulls": true},
	}
	u := base + "/v1.0/myorg/datasets/" + url.PathEscape(strings.TrimSpace(dataset)) + "/executeQueries"
	var resp executeQueriesResp
	if err := do(ctx, rt, http.MethodPost, u, reqBody, &resp); err != nil {
		return connector.Table{}, fmt.Errorf("powerbi: executeQueries: %w", err)
	}
	if resp.Error != nil {
		return connector.Table{}, fmt.Errorf("powerbi: la API devolvió un error %s: %s", resp.Error.Code, resp.Error.Message)
	}
	rawRows, err := resp.firstTableRows()
	if err != nil {
		return connector.Table{}, err
	}
	records, order := flatten(rawRows)
	return connector.BuildTable(records, order, q.maxRows()), nil
}

// --- Consulta ----------------------------------------------------------------

// powerbiQuery es la spec de una consulta Power BI (JSON opaco para el núcleo).
// Query/DAX son sinónimos de la consulta DAX; DatasetID/Dataset del dataset;
// MaxRows acota las filas materializadas.
type powerbiQuery struct {
	Query     string `json:"query,omitempty"`
	DAX       string `json:"dax,omitempty"`
	DatasetID string `json:"dataset_id,omitempty"`
	Dataset   string `json:"dataset,omitempty"`
	MaxRows   int    `json:"max_rows,omitempty"`
}

func (q powerbiQuery) dax() string {
	return strings.TrimSpace(firstNonEmpty(q.Query, q.DAX))
}

func (q powerbiQuery) maxRows() int {
	if q.MaxRows > 0 {
		return q.MaxRows
	}
	return defaultMaxRows
}

func (q powerbiQuery) validate(dataset string) error {
	if q.dax() == "" {
		return fmt.Errorf("%w: falta 'query' (la consulta DAX)", connector.ErrBadQuery)
	}
	if strings.TrimSpace(dataset) == "" {
		return fmt.Errorf("%w: falta 'dataset_id' (en la spec o la config)", connector.ErrBadQuery)
	}
	if strings.ContainsAny(dataset, "/?#") {
		return fmt.Errorf("%w: dataset_id inválido", connector.ErrBadQuery)
	}
	if q.MaxRows < 0 {
		return fmt.Errorf("%w: max_rows no puede ser negativo", connector.ErrBadQuery)
	}
	return nil
}

// --- Modelo de executeQueries ------------------------------------------------

// executeQueriesResp es la respuesta de executeQueries: los result sets (o un
// error de la API/DAX). Solo se usa el primer result set / primera tabla.
type executeQueriesResp struct {
	Results []struct {
		Tables []struct {
			Rows  []json.RawMessage `json:"rows"`
			Error *apiError         `json:"error"`
		} `json:"tables"`
		Error *apiError `json:"error"`
	} `json:"results"`
	Error *apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// firstTableRows devuelve las filas crudas del primer result set / primera tabla,
// propagando un error DAX embebido si lo hay.
func (r executeQueriesResp) firstTableRows() ([]json.RawMessage, error) {
	if len(r.Results) == 0 {
		return nil, nil
	}
	res := r.Results[0]
	if res.Error != nil {
		return nil, fmt.Errorf("powerbi: error en el result set: %s", res.Error.Message)
	}
	if len(res.Tables) == 0 {
		return nil, nil
	}
	tbl := res.Tables[0]
	if tbl.Error != nil {
		return nil, fmt.Errorf("powerbi: error en la consulta DAX: %s", tbl.Error.Message)
	}
	return tbl.Rows, nil
}

// flatten decodifica las filas crudas a registros para connector.BuildTable y
// deduce el orden de columnas de las claves de la PRIMERA fila (preservando el
// orden del EVALUATE en vez del alfabético). Las filas mal formadas se omiten.
//
// ponytail: el orden sale de la primera fila; DAX garantiza filas homogéneas, así
// que basta. Si alguna fila trajera claves extra, BuildTable las añade al final.
func flatten(rawRows []json.RawMessage) ([]map[string]any, []string) {
	records := make([]map[string]any, 0, len(rawRows))
	var order []string
	for i, raw := range rawRows {
		var rec map[string]any
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue
		}
		if i == 0 || order == nil {
			order = orderedKeys(raw)
		}
		records = append(records, rec)
	}
	return records, order
}

// orderedKeys lee las claves de un objeto JSON EN ORDEN de aparición (map de Go no
// conserva el orden). Recorre el stream de tokens saltando los valores.
func orderedKeys(raw json.RawMessage) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	var keys []string
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return keys
		}
		key, _ := kt.(string)
		keys = append(keys, key)
		if err := skipValue(dec); err != nil {
			return keys
		}
	}
	return keys
}

// skipValue consume el siguiente valor del decoder (escalar o compuesto anidado),
// contando la profundidad de llaves/corchetes.
func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := tok.(json.Delim)
	if !ok || (d != '{' && d != '[') {
		return nil // escalar: ya consumido
	}
	depth := 1
	for depth > 0 {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		if dd, ok := t.(json.Delim); ok {
			if dd == '{' || dd == '[' {
				depth++
			} else {
				depth--
			}
		}
	}
	return nil
}

// --- red ---------------------------------------------------------------------

// do emite una petición por rt.HTTP con el cuerpo JSON dado (si body != nil) y, si
// out != nil, decodifica la respuesta 2xx en out. Un no-2xx se mapea a un error
// legible con el "message" de la API (sin filtrar el cuerpo crudo entero).
func do(ctx context.Context, rt connector.Runtime, method, rawURL string, body, out any) error {
	var rdr io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("powerbi: serializar petición: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Integration-Gateway/1")

	resp, err := rt.HTTP.Do(req)
	if err != nil {
		// Preserva ssrfhttp.ErrBlocked / errores de transporte (errors.Is en la capa HTTP).
		return err
	}
	defer drainClose(resp)
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes+1))
	if err != nil {
		return fmt.Errorf("powerbi: leer respuesta: %w", err)
	}
	if int64(len(data)) > maxRespBytes {
		return fmt.Errorf("powerbi: respuesta demasiado grande (> %d bytes)", maxRespBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("powerbi: el origen respondió HTTP %d: %s", resp.StatusCode, apiErrorMessage(data))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("powerbi: la respuesta no es JSON válido: %w", err)
		}
	}
	return nil
}

// --- helpers -----------------------------------------------------------------

// apiBase valida y normaliza (sin barra final) la raíz de la API; por defecto
// https://api.powerbi.com (config api_base la sobrescribe para nubes soberanas).
func apiBase(inst connector.Instance) (string, error) {
	raw := strings.TrimSpace(inst.Config[ConfigAPIBase])
	if raw == "" {
		return defaultAPIBase, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w: %q no es una URL http(s) válida", connector.ErrBadQuery, ConfigAPIBase)
	}
	return strings.TrimRight(raw, "/"), nil
}

// apiErrorMessage extrae el "error.message" de un cuerpo de error de la API,
// acotado. Power BI anida el error bajo la clave "error".
func apiErrorMessage(data []byte) string {
	var e struct {
		Error apiError `json:"error"`
	}
	if err := json.Unmarshal(data, &e); err == nil && strings.TrimSpace(e.Error.Message) != "" {
		return truncate(strings.TrimSpace(e.Error.Message), 300)
	}
	return "(sin detalle)"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// drainClose consume y cierra el cuerpo para reutilizar la conexión.
func drainClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}

// Compile-time: satisface Driver y la capacidad opcional TabularSource.
var (
	_ connector.Driver        = (*Driver)(nil)
	_ connector.TabularSource = (*Driver)(nil)
)
