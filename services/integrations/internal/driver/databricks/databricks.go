// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package databricks es el driver TabularSource para Databricks SQL vía la
// Statement Execution API (/api/2.0/sql/statements): ejecuta una sentencia SQL en
// un SQL warehouse y trae el resultado a Sytue Sheets como tabla. Implementa
// connector.Driver (Test) y la capacidad opcional connector.TabularSource (Query).
//
// Flujo asíncrono de la API: POST envía la sentencia; si no termina dentro del
// wait_timeout, la respuesta llega en estado PENDING/RUNNING y este driver SONDEA
// GET /statements/{id} hasta un estado terminal. En SUCCEEDED el resultado viene
// en CHUNKS (disposition INLINE, format JSON_ARRAY): el chunk 0 va en la respuesta
// y los siguientes se piden por índice a /result/chunks/{n}, ensamblados sin
// cargar todo de golpe (topes de filas/bytes/chunks). El schema del manifiesto da
// los tipos lógicos de las columnas (no se infieren: JSON_ARRAY codifica todo como
// texto).
//
// Auth: el Bearer (PAT u OAuth) sale del vault y lo inyecta rt.HTTP como cabecera;
// el driver NUNCA lo ve ni construye su propio http.Client. Todo el egress
// (submit, sondeo, chunks) sale por rt.HTTP (SSRF + rate-limit + auditoría).
package databricks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

// Kind es la clave del Registry con la que se registra este driver.
const Kind = "databricks"

// Claves de Instance.Config (públicas). El Bearer va en Instance.Secrets["bearer"]
// y lo inyecta el Runtime como cabecera (no es config).
const (
	ConfigWorkspaceURL = "workspace_url" // raíz del workspace (https://dbc-xxxx.cloud.databricks.com)
	ConfigWarehouseID  = "warehouse_id"  // id del SQL warehouse por defecto
)

const (
	maxRespBytes   = 32 << 20 // tope de lectura por respuesta (defensa anti-OOM)
	defaultMaxRows = 10000    // tope de filas acumuladas si la spec no lo fija
	maxChunks      = 10000    // tope de saltos de chunk (defensa anti-bucle)
	maxPolls       = 60       // tope de sondeos de estado antes de rendirse
	pollInitial    = 500 * time.Millisecond
	pollMax        = 3 * time.Second
	statementsPath = "/api/2.0/sql/statements"
)

// Driver es el conector Databricks SQL. Sin estado: cada operación usa el Runtime
// inyectado. Su valor cero es utilizable; New lo devuelve por simetría.
type Driver struct{}

// New construye el driver (se cablea en el Registry del gateway).
func New() *Driver { return &Driver{} }

// Kind identifica al driver en el Registry.
func (*Driver) Kind() string { return Kind }

// Test verifica auth + conectividad + existencia del warehouse haciendo GET al
// endpoint del SQL warehouse configurado, usando EXCLUSIVAMENTE rt.HTTP.
func (*Driver) Test(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	base, err := workspaceURL(inst)
	if err != nil {
		return err
	}
	wh := strings.TrimSpace(inst.Config[ConfigWarehouseID])
	if wh == "" {
		return fmt.Errorf("%w: falta %q en la config de la instancia", connector.ErrBadQuery, ConfigWarehouseID)
	}
	if err := do(ctx, rt, http.MethodGet, base+"/api/2.0/sql/warehouses/"+url.PathEscape(wh), nil, nil); err != nil {
		return fmt.Errorf("databricks: test: %w", err)
	}
	return nil
}

// Query ejecuta una sentencia SQL y devuelve la Table. spec es databricksQuery
// (statement + warehouse_id opcional). Envía la sentencia, sondea hasta un estado
// terminal si sigue en ejecución, y ensambla los chunks del resultado. Egress por
// rt.HTTP.
func (*Driver) Query(ctx context.Context, inst connector.Instance, rt connector.Runtime, spec json.RawMessage) (connector.Table, error) {
	base, err := workspaceURL(inst)
	if err != nil {
		return connector.Table{}, err
	}
	var q databricksQuery
	if len(spec) > 0 {
		if err := json.Unmarshal(spec, &q); err != nil {
			return connector.Table{}, fmt.Errorf("%w: la spec no es JSON válido", connector.ErrBadQuery)
		}
	}
	warehouse := firstNonEmpty(q.WarehouseID, inst.Config[ConfigWarehouseID])
	if err := q.validate(warehouse); err != nil {
		return connector.Table{}, err
	}

	resp, err := submit(ctx, rt, base, warehouse, q.statement(), q.maxRows())
	if err != nil {
		return connector.Table{}, err
	}
	resp, err = awaitTerminal(ctx, rt, base, resp)
	if err != nil {
		return connector.Table{}, err
	}
	rows, err := collectRows(ctx, rt, base, resp, q.maxRows())
	if err != nil {
		return connector.Table{}, err
	}
	return buildTable(resp.manifestColumns(), rows), nil
}

// --- Consulta ----------------------------------------------------------------

// databricksQuery es la spec de una consulta Databricks (JSON opaco para el
// núcleo). Statement/Query/SQL son sinónimos de la sentencia; WarehouseID
// sobrescribe el de la config; MaxRows acota las filas materializadas.
type databricksQuery struct {
	Statement   string `json:"statement,omitempty"`
	Query       string `json:"query,omitempty"`
	SQL         string `json:"sql,omitempty"`
	WarehouseID string `json:"warehouse_id,omitempty"`
	MaxRows     int    `json:"max_rows,omitempty"`
}

func (q databricksQuery) statement() string {
	return strings.TrimSpace(firstNonEmpty(q.Statement, q.Query, q.SQL))
}

func (q databricksQuery) maxRows() int {
	if q.MaxRows > 0 {
		return q.MaxRows
	}
	return defaultMaxRows
}

func (q databricksQuery) validate(warehouse string) error {
	if q.statement() == "" {
		return fmt.Errorf("%w: falta 'statement' (la sentencia SQL)", connector.ErrBadQuery)
	}
	if strings.TrimSpace(warehouse) == "" {
		return fmt.Errorf("%w: falta 'warehouse_id' (en la spec o la config)", connector.ErrBadQuery)
	}
	if q.MaxRows < 0 {
		return fmt.Errorf("%w: max_rows no puede ser negativo", connector.ErrBadQuery)
	}
	return nil
}

// --- Modelo de la Statement Execution API ------------------------------------

// dbxResp es la respuesta de POST /statements y GET /statements/{id}: estado +
// (en un estado terminal exitoso) manifiesto y el primer chunk de resultado.
type dbxResp struct {
	StatementID string         `json:"statement_id"`
	Status      dbxStatus      `json:"status"`
	Manifest    *dbxManifest   `json:"manifest"`
	Result      *dbxResultData `json:"result"`
}

type dbxStatus struct {
	State string `json:"state"`
	Error *struct {
		ErrorCode string `json:"error_code"`
		Message   string `json:"message"`
	} `json:"error"`
}

type dbxManifest struct {
	Schema struct {
		Columns []dbxColumn `json:"columns"`
	} `json:"schema"`
	TotalChunkCount int `json:"total_chunk_count"`
}

type dbxColumn struct {
	Name     string `json:"name"`
	TypeName string `json:"type_name"`
	Position int    `json:"position"`
}

// dbxResultData es un chunk de resultado: las filas como arrays de valores (todos
// texto o null en JSON_ARRAY) y el índice del chunk siguiente si lo hay.
type dbxResultData struct {
	ChunkIndex     int     `json:"chunk_index"`
	RowCount       int     `json:"row_count"`
	DataArray      [][]any `json:"data_array"`
	NextChunkIndex *int    `json:"next_chunk_index"`
}

func (r *dbxResp) manifestColumns() []dbxColumn {
	if r.Manifest == nil {
		return nil
	}
	return r.Manifest.Schema.Columns
}

// submit envía la sentencia. Pide disposition INLINE + format JSON_ARRAY (chunks
// embebidos, sin enlaces presignados cross-host) y empuja el tope de filas al
// servidor con row_limit. wait_timeout deja que la API espere hasta 10s antes de
// devolver PENDING/RUNNING (con CONTINUE no cancela la sentencia).
//
// ponytail: solo INLINE; disposition EXTERNAL_LINKS (URLs presignadas a cloud
// storage, cross-host) queda para cuando haga falta streaming de resultados enormes.
func submit(ctx context.Context, rt connector.Runtime, base, warehouse, statement string, maxRows int) (*dbxResp, error) {
	body := map[string]any{
		"warehouse_id":    warehouse,
		"statement":       statement,
		"wait_timeout":    "10s",
		"on_wait_timeout": "CONTINUE",
		"disposition":     "INLINE",
		"format":          "JSON_ARRAY",
	}
	if maxRows > 0 {
		body["row_limit"] = maxRows
	}
	var resp dbxResp
	if err := do(ctx, rt, http.MethodPost, base+statementsPath, body, &resp); err != nil {
		return nil, fmt.Errorf("databricks: enviar sentencia: %w", err)
	}
	return &resp, nil
}

// awaitTerminal sondea GET /statements/{id} mientras el estado sea PENDING/RUNNING,
// con backoff acotado y respetando la cancelación del contexto, hasta SUCCEEDED (o
// un estado terminal de fallo). Acota el número de sondeos (defensa anti-bucle).
func awaitTerminal(ctx context.Context, rt connector.Runtime, base string, resp *dbxResp) (*dbxResp, error) {
	interval := pollInitial
	for polls := 0; polls < maxPolls; polls++ {
		switch strings.ToUpper(resp.Status.State) {
		case "SUCCEEDED":
			return resp, nil
		case "PENDING", "RUNNING":
			// sigue en ejecución: sondear tras el backoff.
		case "FAILED", "CANCELED", "CLOSED":
			return nil, fmt.Errorf("databricks: la sentencia terminó en estado %s: %s", resp.Status.State, statusMessage(resp))
		default:
			return nil, fmt.Errorf("databricks: estado de sentencia desconocido %q", resp.Status.State)
		}
		if resp.StatementID == "" {
			return nil, fmt.Errorf("databricks: respuesta sin statement_id para sondear")
		}
		if err := sleep(ctx, interval); err != nil {
			return nil, err
		}
		interval = nextBackoff(interval)
		next, err := getStatus(ctx, rt, base, resp.StatementID)
		if err != nil {
			return nil, err
		}
		resp = next
	}
	return nil, fmt.Errorf("databricks: la sentencia sigue en ejecución tras %d sondeos", maxPolls)
}

// collectRows ensambla las filas del resultado: el chunk 0 (embebido en resp) más
// los siguientes chunks pedidos por índice, hasta agotar chunks o alcanzar maxRows.
func collectRows(ctx context.Context, rt connector.Runtime, base string, resp *dbxResp, maxRows int) ([][]any, error) {
	if resp.Result == nil {
		return nil, nil
	}
	out := make([][]any, 0, len(resp.Result.DataArray))
	out = append(out, resp.Result.DataArray...)
	if len(out) >= maxRows {
		return out[:maxRows], nil
	}
	next := resp.Result.NextChunkIndex
	for guard := 0; next != nil && guard < maxChunks; guard++ {
		chunk, err := getChunk(ctx, rt, base, resp.StatementID, *next)
		if err != nil {
			return nil, err
		}
		out = append(out, chunk.DataArray...)
		if len(out) >= maxRows {
			return out[:maxRows], nil
		}
		next = chunk.NextChunkIndex
	}
	return out, nil
}

func getStatus(ctx context.Context, rt connector.Runtime, base, id string) (*dbxResp, error) {
	var resp dbxResp
	if err := do(ctx, rt, http.MethodGet, base+statementsPath+"/"+url.PathEscape(id), nil, &resp); err != nil {
		return nil, fmt.Errorf("databricks: sondear estado: %w", err)
	}
	return &resp, nil
}

func getChunk(ctx context.Context, rt connector.Runtime, base, id string, index int) (*dbxResultData, error) {
	u := base + statementsPath + "/" + url.PathEscape(id) + "/result/chunks/" + strconv.Itoa(index)
	var chunk dbxResultData
	if err := do(ctx, rt, http.MethodGet, u, nil, &chunk); err != nil {
		return nil, fmt.Errorf("databricks: leer chunk %d: %w", index, err)
	}
	return &chunk, nil
}

// --- Materialización ---------------------------------------------------------

// buildTable normaliza el schema del manifiesto + las filas (data_array) al Table
// del contrato. No usa connector.BuildTable a propósito: JSON_ARRAY entrega TODOS
// los valores como texto, así que los tipos vienen del schema (fiables), no de
// inferir sobre strings. Coacciona los escalares numéricos/booleanos a su tipo JSON
// para que Sheets reciba números como números (preservando la precisión con
// json.Number, no float64).
func buildTable(cols []dbxColumn, rows [][]any) connector.Table {
	tc := make([]connector.Column, len(cols))
	logical := make([]string, len(cols))
	for i, c := range cols {
		lt := logicalType(c.TypeName)
		logical[i] = lt
		tc[i] = connector.Column{Name: c.Name, Type: lt}
	}
	out := make([][]any, 0, len(rows))
	for _, r := range rows {
		row := make([]any, len(cols))
		for i := range cols {
			if i < len(r) {
				row[i] = coerce(logical[i], r[i])
			}
		}
		out = append(out, row)
	}
	return connector.Table{Columns: tc, Rows: out}
}

// logicalType mapea un type_name de Spark SQL al tipo lógico de la suite,
// coherente con la inferencia de connector.Table.
func logicalType(typeName string) string {
	switch strings.ToUpper(strings.TrimSpace(typeName)) {
	case "BYTE", "SHORT", "INT", "INTEGER", "LONG", "BIGINT", "FLOAT", "DOUBLE", "DECIMAL":
		return "number"
	case "BOOLEAN":
		return "boolean"
	case "DATE", "TIMESTAMP", "TIMESTAMP_NTZ":
		return "datetime"
	case "STRING", "CHAR", "VARCHAR", "BINARY":
		return "text"
	case "ARRAY", "MAP", "STRUCT":
		return "json"
	default:
		return "text"
	}
}

// coerce convierte el valor textual de JSON_ARRAY al tipo JSON que corresponde a la
// columna. null se preserva (nil); number → json.Number (precisión intacta);
// boolean → bool; el resto se deja como texto crudo.
func coerce(logical string, v any) any {
	s, ok := v.(string)
	if !ok {
		return v // null (nil) u otro valor ya tipado
	}
	switch logical {
	case "number":
		if isJSONNumber(s) {
			return json.Number(s)
		}
		return s
	case "boolean":
		switch s {
		case "true":
			return true
		case "false":
			return false
		}
		return s
	default:
		return s
	}
}

// isJSONNumber comprueba que s es un número finito (sintaxis válida de número JSON),
// para poder emitirlo como json.Number sin corromper la salida (rechaza Inf/NaN/"").
func isJSONNumber(s string) bool {
	if s == "" {
		return false
	}
	f, err := strconv.ParseFloat(s, 64)
	return err == nil && !math.IsInf(f, 0) && !math.IsNaN(f)
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
			return fmt.Errorf("databricks: serializar petición: %w", err)
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
		return fmt.Errorf("databricks: leer respuesta: %w", err)
	}
	if int64(len(data)) > maxRespBytes {
		return fmt.Errorf("databricks: respuesta demasiado grande (> %d bytes)", maxRespBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("databricks: el origen respondió HTTP %d: %s", resp.StatusCode, apiErrorMessage(data))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("databricks: la respuesta no es JSON válido: %w", err)
		}
	}
	return nil
}

// --- helpers -----------------------------------------------------------------

// workspaceURL valida y normaliza (sin barra final) la raíz del workspace.
func workspaceURL(inst connector.Instance) (string, error) {
	raw := strings.TrimSpace(inst.Config[ConfigWorkspaceURL])
	if raw == "" {
		return "", fmt.Errorf("%w: falta %q en la config de la instancia", connector.ErrBadQuery, ConfigWorkspaceURL)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w: %q no es una URL http(s) válida", connector.ErrBadQuery, ConfigWorkspaceURL)
	}
	return strings.TrimRight(raw, "/"), nil
}

// statusMessage extrae el mensaje de error del estado (para un estado terminal de
// fallo), sin filtrar más que lo necesario.
func statusMessage(resp *dbxResp) string {
	if resp.Status.Error != nil && strings.TrimSpace(resp.Status.Error.Message) != "" {
		return strings.TrimSpace(resp.Status.Error.Message)
	}
	return "(sin detalle)"
}

// apiErrorMessage extrae el "message" de un cuerpo de error de la API, acotado.
func apiErrorMessage(data []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &e); err == nil && strings.TrimSpace(e.Message) != "" {
		return truncate(strings.TrimSpace(e.Message), 300)
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

// nextBackoff aumenta el intervalo de sondeo un 50%, acotado a pollMax.
func nextBackoff(d time.Duration) time.Duration {
	d += d / 2
	if d > pollMax {
		return pollMax
	}
	return d
}

// sleep espera d respetando la cancelación del contexto.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
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
