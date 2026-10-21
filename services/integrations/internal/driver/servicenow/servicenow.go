// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package servicenow es el driver TabularSource + SearchCapability de ServiceNow:
// trae cualquier tabla de la instancia (incident, sys_user, cmdb_ci…) a Sytue Sheets
// vía la Table API, y alimenta la búsqueda federada de Sytue Hub con incidencias y
// artículos de la base de conocimiento.
//
// Auth: por defecto el token del vault (clave convencional "bearer") que INYECTA el
// Runtime de la pasarela —el driver no lo ve—, que es donde el CONSENTIMIENTO OAuth 2.0
// deja el access token; la Table API lo acepta tal cual como Bearer. Si la instancia
// guarda en su lugar un par usuario/contraseña (secretos "username"/"password", el modo
// clásico de las cuentas de integración de ServiceNow) y NO hay consentimiento, el driver
// compone la cabecera Basic. Todo el egress sale por rt.HTTP (SSRF + rate-limit +
// auditoría).
//
// Normalización: la Table API devuelve los campos como ESCALAR o como objeto
// —{"value","display_value"} con sysparm_display_value=all, {"link","value"} en los
// campos de referencia—. Aquí se aplanan a escalar (display_value si lo hay, si no
// value) para que una hoja de cálculo no reciba objetos JSON en cada celda.
//
// Paginación: la Table API es por offset (sysparm_limit/sysparm_offset), sin enlace
// «siguiente» en el cuerpo; se pagina con topes de filas y de páginas, sin cargar el
// dataset entero en memoria.
package servicenow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

// Kind es la clave del Registry con la que se registra este driver.
const Kind = "servicenow"

// Claves de Instance.Config (públicas) y de Instance.Secrets (auth básica opcional).
const (
	ConfigInstance = "instance" // «acme» o la URL completa https://acme.service-now.com
	ConfigUsername = "username" // opcional: usuario de la cuenta de integración (auth básica)

	secretUsername = "username" // opcional (alternativa a la config)
	secretPassword = "password" // opcional: contraseña de la cuenta de integración
)

const (
	serviceNowHost = ".service-now.com"

	maxRespBytes    = 32 << 20 // tope de lectura por página (defensa anti-OOM)
	maxPages        = 1000     // tope de páginas (defensa anti-bucle)
	defaultPageSize = 1000     // filas por petición si la spec no lo fija
	maxPageSize     = 5000     // tope duro del tamaño de página
	defaultMaxRows  = 10000    // tope de filas acumuladas si la spec no lo fija
)

// Driver es el conector ServiceNow. Sin estado: cada operación usa el Runtime inyectado.
type Driver struct{}

// New construye el driver (se cablea en el Registry del gateway).
func New() *Driver { return &Driver{} }

// Kind identifica al driver en el Registry.
func (*Driver) Kind() string { return Kind }

// Test verifica que la instancia es alcanzable y que la credencial vale: una fila de
// sys_user por la Table API. Un destino privado ⇒ ssrfhttp.ErrBlocked por rt.HTTP.
func (*Driver) Test(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	base, err := baseURL(inst)
	if err != nil {
		return err
	}
	resp, err := doGet(ctx, rt, inst, base+"/api/now/table/sys_user?sysparm_limit=1&sysparm_fields=sys_id")
	if err != nil {
		return fmt.Errorf("servicenow: test: %w", err)
	}
	if _, err := readOK(resp); err != nil {
		return fmt.Errorf("servicenow: test: %w", err)
	}
	return nil
}

// Query trae filas de una tabla (Table API) como Table, paginando por offset hasta
// agotar filas/páginas. spec es snQuery.
func (*Driver) Query(ctx context.Context, inst connector.Instance, rt connector.Runtime, spec json.RawMessage) (connector.Table, error) {
	base, err := baseURL(inst)
	if err != nil {
		return connector.Table{}, err
	}
	var q snQuery
	if len(spec) > 0 {
		if err := json.Unmarshal(spec, &q); err != nil {
			return connector.Table{}, fmt.Errorf("%w: la spec no es JSON válido", connector.ErrBadQuery)
		}
	}
	if err := q.validate(); err != nil {
		return connector.Table{}, err
	}

	maxRows, pageSize := q.maxRows(), q.pageSize()
	out := make([]map[string]any, 0, 128)
	offset := q.Offset
	for page := 0; page < maxPages; page++ {
		recs, err := fetchPage(ctx, rt, inst, base+"/api/now/table/"+url.PathEscape(q.table())+"?"+q.params(pageSize, offset).Encode())
		if err != nil {
			return connector.Table{}, err
		}
		for _, rec := range recs {
			out = append(out, flatten(rec))
			if len(out) >= maxRows {
				return connector.BuildTable(out[:maxRows], q.Fields, maxRows), nil
			}
		}
		if len(recs) < pageSize {
			break // última página (la Table API no lleva enlace «siguiente»)
		}
		offset += len(recs)
	}
	return connector.BuildTable(out, q.Fields, maxRows), nil
}

// Search implementa connector.SearchCapability: busca incidencias y artículos de la
// base de conocimiento por texto y los mapea a hits con deep-link a la instancia. Si
// una de las dos tablas falla (ACL/plugin ausente), se degrada con la otra; solo si
// fallan TODAS se propaga el error. Egress por rt.HTTP.
func (*Driver) Search(ctx context.Context, inst connector.Instance, rt connector.Runtime, query string, limit int) ([]connector.ExternalHit, error) {
	term := sanitizeTerm(query)
	if term == "" {
		return nil, nil
	}
	base, err := baseURL(inst)
	if err != nil {
		return nil, err
	}
	n := clampLimit(limit)
	sources := []struct {
		table, fields, query, kind string
	}{
		{"incident", "sys_id,number,short_description,state", "short_descriptionLIKE" + term + "^ORnumberSTARTSWITH" + term, "incident"},
		{"kb_knowledge", "sys_id,number,short_description", "short_descriptionLIKE" + term + "^ORnumberSTARTSWITH" + term, "article"},
	}

	hits := make([]connector.ExternalHit, 0, n)
	var firstErr error
	ok := 0
	for _, s := range sources {
		ps := url.Values{
			"sysparm_query":         {s.query},
			"sysparm_fields":        {s.fields},
			"sysparm_limit":         {strconv.Itoa(n)},
			"sysparm_display_value": {"true"},
		}
		recs, err := fetchPage(ctx, rt, inst, base+"/api/now/table/"+s.table+"?"+ps.Encode())
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ok++
		for _, rec := range recs {
			r := flatten(rec)
			title := strings.TrimSpace(asString(r, "short_description"))
			number := strings.TrimSpace(asString(r, "sys_id"))
			if num := strings.TrimSpace(asString(r, "number")); num != "" {
				number = num
				if title == "" {
					title = num
				} else {
					title = num + " · " + title
				}
			}
			if title == "" {
				continue
			}
			hits = append(hits, connector.ExternalHit{
				Title:   title,
				URL:     recordURL(base, s.table, asString(r, "sys_id")),
				Snippet: firstNonEmpty(asString(r, "state"), number),
				Kind:    s.kind,
				Source:  "ServiceNow",
			})
			if len(hits) >= n {
				return hits, nil
			}
		}
	}
	if ok == 0 && firstErr != nil {
		return nil, firstErr
	}
	return hits, nil
}

// recordURL compone el deep-link al registro en la UI de la instancia.
func recordURL(base, table, sysID string) string {
	if sysID == "" {
		return base
	}
	return base + "/" + url.PathEscape(table) + ".do?sys_id=" + url.QueryEscape(sysID)
}

// --- Consulta ----------------------------------------------------------------

// snQuery es la spec de una consulta (JSON opaco para el núcleo). Table/Dataset son
// sinónimos; Query es un sysparm_query crudo (sintaxis de ServiceNow); Limit es el
// tamaño de página y MaxRows el total acumulado en cliente.
type snQuery struct {
	Table        string   `json:"table,omitempty"`
	Dataset      string   `json:"dataset,omitempty"`
	Query        string   `json:"query,omitempty"`
	Fields       []string `json:"fields,omitempty"`
	Limit        int      `json:"limit,omitempty"`
	Offset       int      `json:"offset,omitempty"`
	MaxRows      int      `json:"max_rows,omitempty"`
	DisplayValue string   `json:"display_value,omitempty"` // true | false | all
}

func (q snQuery) table() string {
	if t := strings.TrimSpace(q.Table); t != "" {
		return t
	}
	return strings.TrimSpace(q.Dataset)
}

func (q snQuery) maxRows() int {
	if q.MaxRows > 0 {
		return q.MaxRows
	}
	return defaultMaxRows
}

func (q snQuery) pageSize() int {
	n := q.Limit
	if n <= 0 {
		n = defaultPageSize
	}
	if n > maxPageSize {
		n = maxPageSize
	}
	if m := q.maxRows(); n > m {
		n = m
	}
	return n
}

func (q snQuery) validate() error {
	t := q.table()
	if t == "" {
		return fmt.Errorf("%w: falta 'table' (tabla de ServiceNow, p. ej. incident)", connector.ErrBadQuery)
	}
	for _, r := range t {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return fmt.Errorf("%w: nombre de tabla inválido", connector.ErrBadQuery)
		}
	}
	if q.Limit < 0 || q.Offset < 0 || q.MaxRows < 0 {
		return fmt.Errorf("%w: limit/offset/max_rows no pueden ser negativos", connector.ErrBadQuery)
	}
	switch strings.TrimSpace(q.DisplayValue) {
	case "", "true", "false", "all":
	default:
		return fmt.Errorf("%w: display_value debe ser 'true', 'false' o 'all'", connector.ErrBadQuery)
	}
	return nil
}

// params compone los sysparm_* de una página.
func (q snQuery) params(pageSize, offset int) url.Values {
	ps := url.Values{
		"sysparm_limit":  {strconv.Itoa(pageSize)},
		"sysparm_offset": {strconv.Itoa(offset)},
	}
	if s := strings.TrimSpace(q.Query); s != "" {
		ps.Set("sysparm_query", s)
	}
	if len(q.Fields) > 0 {
		ps.Set("sysparm_fields", strings.Join(q.Fields, ","))
	}
	if dv := strings.TrimSpace(q.DisplayValue); dv != "" {
		ps.Set("sysparm_display_value", dv)
	}
	return ps
}

// fetchPage pide una página de la Table API y devuelve sus registros crudos.
func fetchPage(ctx context.Context, rt connector.Runtime, inst connector.Instance, rawURL string) ([]map[string]any, error) {
	resp, err := doGet(ctx, rt, inst, rawURL)
	if err != nil {
		// Preserva ssrfhttp.ErrBlocked / errores de transporte (errors.Is aguas arriba).
		return nil, fmt.Errorf("servicenow: %w", err)
	}
	body, err := readOK(resp)
	if err != nil {
		return nil, err
	}
	var pg struct {
		Result []map[string]any `json:"result"`
	}
	if err := json.Unmarshal(body, &pg); err != nil {
		return nil, fmt.Errorf("servicenow: la respuesta no es un resultado JSON de la Table API: %w", err)
	}
	return pg.Result, nil
}

// flatten normaliza los campos de un registro a escalar: ServiceNow devuelve objetos
// {"value","display_value"} (sysparm_display_value=all) y {"link","value"} en las
// referencias. Se prefiere display_value; si falta o está vacío, value.
func flatten(rec map[string]any) map[string]any {
	out := make(map[string]any, len(rec))
	for k, v := range rec {
		out[k] = scalar(v)
	}
	return out
}

func scalar(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if dv, ok := m["display_value"]; ok && !emptyValue(dv) {
		return dv
	}
	if val, ok := m["value"]; ok {
		return val
	}
	return v
}

func emptyValue(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && s == ""
}

// --- helpers -----------------------------------------------------------------

// baseURL normaliza la raíz de la instancia: el nombre («acme» ⇒
// https://acme.service-now.com) o una URL http(s) completa (nubes dedicadas/tests).
// Se conserva SOLO esquema+host.
func baseURL(inst connector.Instance) (string, error) {
	raw := strings.TrimSpace(inst.Config[ConfigInstance])
	if raw == "" {
		return "", fmt.Errorf("%w: falta %q en la config", connector.ErrBadQuery, ConfigInstance)
	}
	if !strings.Contains(raw, "://") {
		if strings.ContainsAny(raw, "/:?#@") {
			return "", fmt.Errorf("%w: %q inválido", connector.ErrBadQuery, ConfigInstance)
		}
		if !strings.Contains(raw, ".") {
			raw += serviceNowHost
		}
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w: %q no es una instancia ni una URL http(s) válida", connector.ErrBadQuery, ConfigInstance)
	}
	return u.Scheme + "://" + u.Host, nil
}

// doGet emite un GET por rt.HTTP con Accept JSON. Si la instancia usa auth básica
// (usuario/contraseña), añade la cabecera; si no, la auth la inyecta el Runtime.
func doGet(ctx context.Context, rt connector.Runtime, inst connector.Instance, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Integration-Gateway/1")
	if auth := basicAuth(inst); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	return rt.HTTP.Do(req)
}

// basicAuth compone la cabecera Basic si la instancia trae usuario+contraseña; "" si
// la auth va por el Bearer que inyecta el Runtime.
//
// El Bearer GANA cuando existe: ahí deja el canje del consentimiento el access token, y
// seguir mandando la credencial vieja haría que autorizar no sirviera de nada —el
// asistente diría «conectado» y la conexión seguiría colgando de una contraseña de
// servicio—. Sin Bearer, la auth básica sigue funcionando igual que siempre.
func basicAuth(inst connector.Instance) string {
	if inst.HasBearer {
		return ""
	}
	user := strings.TrimSpace(firstNonEmpty(inst.Secrets[secretUsername], inst.Config[ConfigUsername]))
	pass := inst.Secrets[secretPassword]
	if user == "" || pass == "" {
		return ""
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// readOK exige 2xx y devuelve el cuerpo acotado a maxRespBytes.
func readOK(resp *http.Response) ([]byte, error) {
	defer drainClose(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes+1))
	if err != nil {
		return nil, fmt.Errorf("servicenow: leer respuesta: %w", err)
	}
	if int64(len(body)) > maxRespBytes {
		return nil, fmt.Errorf("servicenow: respuesta demasiado grande (> %d bytes)", maxRespBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("servicenow: el origen respondió HTTP %d: %s", resp.StatusCode, errorMessage(body))
	}
	return body, nil
}

// errorMessage extrae el "error.message"/"error.detail" del cuerpo de error de la
// REST API de ServiceNow.
func errorMessage(data []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
			Detail  string `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &e); err == nil {
		if msg := strings.TrimSpace(firstNonEmpty(e.Error.Message, e.Error.Detail)); msg != "" {
			return truncate(msg, 300)
		}
	}
	return "(sin detalle)"
}

// sanitizeTerm limpia el término de búsqueda de los delimitadores de sysparm_query
// (^ y ,) y de caracteres de control: no debe poder alterar la consulta compuesta.
func sanitizeTerm(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == '^' || r == ',' {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func asString(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// clampLimit acota los resultados de una búsqueda a un rango razonable (1..50).
func clampLimit(n int) int {
	if n <= 0 {
		return 10
	}
	if n > 50 {
		return 50
	}
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// drainClose consume y cierra el cuerpo para reutilizar la conexión.
func drainClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}

// Compile-time: satisface Driver y las capacidades opcionales TabularSource y
// SearchCapability.
var (
	_ connector.Driver           = (*Driver)(nil)
	_ connector.TabularSource    = (*Driver)(nil)
	_ connector.SearchCapability = (*Driver)(nil)
)
