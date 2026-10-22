// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package jira es el driver TabularSource + SearchCapability de Jira Cloud: trae
// incidencias por JQL a Sytue Sheets como tabla (una fila por issue, campos anidados
// APLANADOS: status → status.name, assignee → assignee.displayName…) y alimenta la
// búsqueda federada de Sytue Hub con deep-links /browse/KEY.
//
// Auth: por defecto el token del vault (clave convencional "bearer") que INYECTA el
// Runtime de la pasarela —el driver no lo ve—, que es donde el CONSENTIMIENTO OAuth 2.0
// deja el access token. Como los API tokens de Atlassian se usan con auth BÁSICA (correo
// + token), si la instancia guarda el token bajo el secreto "api_token" y declara el
// correo en la config, el driver compone la cabecera Basic. Todo el egress sale por
// rt.HTTP (SSRF + rate-limit + auditoría).
//
// Consentimiento ⇒ OTRA RAÍZ: Atlassian atiende las peticiones autorizadas por OAuth 2.0
// (3LO) en https://api.atlassian.com/ex/jira/{cloudid} y NO en tu-sitio.atlassian.net.
// Por eso, cuando la pasarela inyecta un Bearer y el sitio es de Atlassian Cloud, el
// driver resuelve antes el cloudid del sitio. Sin esto, declarar `oauth` en la plantilla
// habría prometido un consentimiento que acaba en 401 DESPUÉS de que el admin autorice.
// Los deep-links /browse/KEY siguen apuntando al sitio: los abre una persona.
//
// Paginación: /rest/api/3/search es por offset (startAt/maxResults, sin enlace
// «siguiente» en el cuerpo); se pagina con topes de filas y de páginas, sin cargar el
// backlog entero en memoria.
package jira

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
const Kind = "jira"

// Claves de Instance.Config (públicas) y de Instance.Secrets (auth básica opcional).
const (
	ConfigSite  = "site"  // «acme» o la URL completa https://acme.atlassian.net
	ConfigEmail = "email" // opcional: correo de la cuenta dueña del API token (auth básica)

	secretEmail    = "email"
	secretAPIToken = "api_token" // opcional: API token de Atlassian (auth básica)
)

const (
	atlassianHost = ".atlassian.net"
	// atlassianAPI es la raíz por la que Atlassian atiende las peticiones autorizadas por
	// consentimiento (3LO). No es configurable: es de Atlassian, no del tenant.
	atlassianAPI = "https://api.atlassian.com"

	maxRespBytes    = 32 << 20 // tope de lectura por página (defensa anti-OOM)
	maxPages        = 1000     // tope de páginas (defensa anti-bucle)
	defaultPageSize = 50       // issues por petición si la spec no lo fija
	maxPageSize     = 100      // tope de Jira Cloud para maxResults
	defaultMaxRows  = 10000    // tope de filas acumuladas si la spec no lo fija
)

// defaultFields son los campos que se piden (y el orden de columnas) cuando la spec no
// los fija: lo que una hoja de seguimiento espera ver de un issue.
var defaultFields = []string{"summary", "status", "assignee", "reporter", "priority", "issuetype", "project", "created", "updated", "labels"}

// Driver es el conector Jira Cloud. Sin estado: cada operación usa el Runtime inyectado.
type Driver struct{}

// New construye el driver (se cablea en el Registry del gateway).
func New() *Driver { return &Driver{} }

// Kind identifica al driver en el Registry.
func (*Driver) Kind() string { return Kind }

// Test verifica que el sitio es alcanzable y que la credencial vale:
// GET /rest/api/3/myself. Un destino privado ⇒ ssrfhttp.ErrBlocked por rt.HTTP.
func (*Driver) Test(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	site, err := baseURL(inst)
	if err != nil {
		return err
	}
	base, err := restBase(ctx, rt, inst, site)
	if err != nil {
		return err
	}
	resp, err := doGet(ctx, rt, inst, base+"/rest/api/3/myself")
	if err != nil {
		return fmt.Errorf("jira: test: %w", err)
	}
	if _, err := readOK(resp); err != nil {
		return fmt.Errorf("jira: test: %w", err)
	}
	return nil
}

// Query ejecuta una JQL y devuelve los issues aplanados como Table, paginando por
// startAt/maxResults hasta agotar filas/páginas. spec es jiraQuery.
func (*Driver) Query(ctx context.Context, inst connector.Instance, rt connector.Runtime, spec json.RawMessage) (connector.Table, error) {
	site, err := baseURL(inst)
	if err != nil {
		return connector.Table{}, err
	}
	base, err := restBase(ctx, rt, inst, site)
	if err != nil {
		return connector.Table{}, err
	}
	var q jiraQuery
	if len(spec) > 0 {
		if err := json.Unmarshal(spec, &q); err != nil {
			return connector.Table{}, fmt.Errorf("%w: la spec no es JSON válido", connector.ErrBadQuery)
		}
	}
	if err := q.validate(); err != nil {
		return connector.Table{}, err
	}

	fields := q.fields()
	maxRows, pageSize := q.maxRows(), q.pageSize()
	out := make([]map[string]any, 0, 128)
	startAt := q.StartAt
	for page := 0; page < maxPages; page++ {
		ps := url.Values{
			"jql":        {q.jql()},
			"startAt":    {strconv.Itoa(startAt)},
			"maxResults": {strconv.Itoa(pageSize)},
			"fields":     {strings.Join(fields, ",")},
		}
		pg, err := fetchPage(ctx, rt, inst, base+"/rest/api/3/search?"+ps.Encode())
		if err != nil {
			return connector.Table{}, err
		}
		for _, iss := range pg.Issues {
			out = append(out, flattenIssue(iss))
			if len(out) >= maxRows {
				return connector.BuildTable(out[:maxRows], columnOrder(fields), maxRows), nil
			}
		}
		startAt += len(pg.Issues)
		if len(pg.Issues) == 0 || len(pg.Issues) < pageSize || (pg.Total > 0 && startAt >= pg.Total) {
			break
		}
	}
	return connector.BuildTable(out, columnOrder(fields), maxRows), nil
}

// Search implementa connector.SearchCapability: issues que casan con el texto
// (JQL `text ~ "…"`), ordenados por actualización, con deep-link /browse/KEY. Una sola
// página acotada por limit; egress por rt.HTTP.
func (*Driver) Search(ctx context.Context, inst connector.Instance, rt connector.Runtime, query string, limit int) ([]connector.ExternalHit, error) {
	term := escapeJQL(strings.TrimSpace(query))
	if term == "" {
		return nil, nil
	}
	site, err := baseURL(inst)
	if err != nil {
		return nil, err
	}
	base, err := restBase(ctx, rt, inst, site)
	if err != nil {
		return nil, err
	}
	ps := url.Values{
		"jql":        {`text ~ "` + term + `" ORDER BY updated DESC`},
		"maxResults": {strconv.Itoa(clampLimit(limit))},
		"fields":     {"summary,status,issuetype"},
	}
	pg, err := fetchPage(ctx, rt, inst, base+"/rest/api/3/search?"+ps.Encode())
	if err != nil {
		return nil, err
	}
	hits := make([]connector.ExternalHit, 0, len(pg.Issues))
	for _, iss := range pg.Issues {
		rec := flattenIssue(iss)
		key := asString(rec, "key")
		if key == "" {
			continue
		}
		title := key
		if s := strings.TrimSpace(asString(rec, "summary")); s != "" {
			title = key + " · " + s
		}
		hits = append(hits, connector.ExternalHit{
			Title: title,
			// El deep-link es para una PERSONA: siempre al sitio, nunca a la raíz de API.
			URL:     site + "/browse/" + url.PathEscape(key),
			Snippet: firstNonEmpty(asString(rec, "status"), asString(rec, "issuetype")),
			Kind:    "issue",
			Source:  "Jira",
		})
	}
	return hits, nil
}

// --- Consulta ----------------------------------------------------------------

// jiraQuery es la spec de una consulta (JSON opaco para el núcleo): JQL cruda (o
// project/status como atajo), campos a traer, offset inicial y topes.
type jiraQuery struct {
	JQL        string   `json:"jql,omitempty"`
	Project    string   `json:"project,omitempty"` // atajo: project = "X"
	Status     string   `json:"status,omitempty"`  // atajo: status = "Y"
	Fields     []string `json:"fields,omitempty"`
	StartAt    int      `json:"start_at,omitempty"`
	MaxResults int      `json:"max_results,omitempty"` // tamaño de página
	MaxRows    int      `json:"max_rows,omitempty"`
}

func (q jiraQuery) maxRows() int {
	if q.MaxRows > 0 {
		return q.MaxRows
	}
	return defaultMaxRows
}

func (q jiraQuery) pageSize() int {
	n := q.MaxResults
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

func (q jiraQuery) fields() []string {
	out := make([]string, 0, len(q.Fields))
	for _, f := range q.Fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return defaultFields
	}
	return out
}

// jql compone la consulta: la JQL cruda si viene; si no, los atajos project/status; si
// tampoco, todo el sitio ordenado por actualización.
func (q jiraQuery) jql() string {
	if s := strings.TrimSpace(q.JQL); s != "" {
		return s
	}
	var clauses []string
	if p := strings.TrimSpace(q.Project); p != "" {
		clauses = append(clauses, `project = "`+escapeJQL(p)+`"`)
	}
	if s := strings.TrimSpace(q.Status); s != "" {
		clauses = append(clauses, `status = "`+escapeJQL(s)+`"`)
	}
	if len(clauses) == 0 {
		return "ORDER BY updated DESC"
	}
	return strings.Join(clauses, " AND ") + " ORDER BY updated DESC"
}

func (q jiraQuery) validate() error {
	if q.StartAt < 0 || q.MaxResults < 0 || q.MaxRows < 0 {
		return fmt.Errorf("%w: start_at/max_results/max_rows no pueden ser negativos", connector.ErrBadQuery)
	}
	for _, f := range q.fields() {
		for _, r := range f {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			default:
				return fmt.Errorf("%w: nombre de campo inválido %q", connector.ErrBadQuery, f)
			}
		}
	}
	return nil
}

// issue es un issue tal como lo devuelve la API (key + campos sin aplanar).
type issue struct {
	ID     string         `json:"id"`
	Key    string         `json:"key"`
	Fields map[string]any `json:"fields"`
}

// searchPage es la respuesta de /rest/api/3/search.
type searchPage struct {
	StartAt int     `json:"startAt"`
	Total   int     `json:"total"`
	Issues  []issue `json:"issues"`
}

// fetchPage pide una página de issues.
func fetchPage(ctx context.Context, rt connector.Runtime, inst connector.Instance, rawURL string) (searchPage, error) {
	resp, err := doGet(ctx, rt, inst, rawURL)
	if err != nil {
		// Preserva ssrfhttp.ErrBlocked / errores de transporte (errors.Is aguas arriba).
		return searchPage{}, fmt.Errorf("jira: %w", err)
	}
	body, err := readOK(resp)
	if err != nil {
		return searchPage{}, err
	}
	var pg searchPage
	if err := json.Unmarshal(body, &pg); err != nil {
		return searchPage{}, fmt.Errorf("jira: la respuesta no es un resultado de búsqueda JSON: %w", err)
	}
	return pg, nil
}

// flattenIssue aplana un issue a un registro plano: key/id + cada campo reducido a un
// escalar legible (status → name, assignee → displayName, labels → lista separada por
// comas). Lo que no se pueda reducir queda tal cual y BuildTable lo serializa a JSON.
func flattenIssue(iss issue) map[string]any {
	out := make(map[string]any, len(iss.Fields)+2)
	out["key"] = iss.Key
	out["id"] = iss.ID
	for k, v := range iss.Fields {
		out[k] = flattenValue(v)
	}
	return out
}

// flattenValue reduce un valor de campo de Jira a un escalar cuando puede.
func flattenValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range []string{"displayName", "name", "value", "key"} {
			if s, ok := t[k].(string); ok && s != "" {
				return s
			}
		}
		return v
	case []any:
		parts := make([]string, 0, len(t))
		for _, el := range t {
			s, ok := flattenValue(el).(string)
			if !ok {
				return v // lista no reducible (p. ej. adjuntos): se serializa a JSON.
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, ", ")
	default:
		return v
	}
}

// columnOrder antepone la clave del issue al orden de campos pedido.
func columnOrder(fields []string) []string {
	return append([]string{"key"}, fields...)
}

// --- helpers -----------------------------------------------------------------

// baseURL normaliza la raíz del sitio: el nombre («acme» ⇒ https://acme.atlassian.net)
// o una URL http(s) completa (Data Center/tests). Se conserva SOLO esquema+host.
func baseURL(inst connector.Instance) (string, error) {
	raw := strings.TrimSpace(inst.Config[ConfigSite])
	if raw == "" {
		return "", fmt.Errorf("%w: falta %q en la config", connector.ErrBadQuery, ConfigSite)
	}
	if !strings.Contains(raw, "://") {
		if strings.ContainsAny(raw, "/:?#@") {
			return "", fmt.Errorf("%w: %q inválido", connector.ErrBadQuery, ConfigSite)
		}
		if !strings.Contains(raw, ".") {
			raw += atlassianHost
		}
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w: %q no es un sitio ni una URL http(s) válida", connector.ErrBadQuery, ConfigSite)
	}
	return u.Scheme + "://" + u.Host, nil
}

// restBase decide contra qué raíz salen las llamadas REST.
//
// Un token de CONSENTIMIENTO (3LO) no vale contra el sitio: Atlassian atiende esas
// peticiones en api.atlassian.com/ex/jira/{cloudid} y en tu-sitio.atlassian.net responde
// 401. Así que con Bearer inyectado y sitio de Atlassian Cloud hay que resolver el
// cloudid y hablar por ahí. En cualquier otro caso —auth básica correo+token, o un Jira
// Data Center con su PAT en su propio host— se conserva la raíz del sitio, que es como
// funcionan las conexiones YA creadas: convertir la plantilla no rompe ninguna.
func restBase(ctx context.Context, rt connector.Runtime, inst connector.Instance, site string) (string, error) {
	if !inst.HasBearer || !isCloudSite(site) {
		return site, nil
	}
	id, err := cloudID(ctx, rt, inst, site)
	if err != nil {
		return "", err
	}
	return atlassianAPI + "/ex/jira/" + id, nil
}

// isCloudSite dice si la raíz es un sitio de Atlassian Cloud (el único que exige la raíz
// de API aparte; un Data Center vive en el dominio del cliente).
func isCloudSite(base string) bool {
	u, err := url.Parse(base)
	return err == nil && strings.HasSuffix(strings.ToLower(u.Hostname()), atlassianHost)
}

// accessibleResource es un sitio al que da acceso el token del consentimiento.
type accessibleResource struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// cloudID resuelve el identificador que Atlassian da al sitio PARA ESTE TOKEN: se pide el
// listado de recursos accesibles del propio token (con el Bearer que inyecta el Runtime,
// por eso no se fija cabecera aquí).
//
// ponytail: una llamada extra por operación —el driver es sin estado a propósito—; si
// pesa, cachear por instancia, que el cloudid de un sitio no cambia.
func cloudID(ctx context.Context, rt connector.Runtime, inst connector.Instance, site string) (string, error) {
	resp, err := doGet(ctx, rt, inst, atlassianAPI+"/oauth/token/accessible-resources")
	if err != nil {
		return "", fmt.Errorf("jira: recursos accesibles: %w", err)
	}
	body, err := readOK(resp)
	if err != nil {
		return "", err
	}
	var res []accessibleResource
	if err := json.Unmarshal(body, &res); err != nil {
		return "", fmt.Errorf("jira: la lista de recursos accesibles no es JSON: %w", err)
	}
	return pickCloudID(res, site)
}

// pickCloudID elige el recurso que ES el sitio configurado. Quedarse con el primero sería
// más corto y estaría MAL: un consentimiento puede abarcar varios sitios de Atlassian y
// se acabarían leyendo incidencias de otra organización sin que nadie lo notara.
func pickCloudID(res []accessibleResource, site string) (string, error) {
	want := strings.ToLower(strings.TrimRight(site, "/"))
	for _, r := range res {
		if r.ID != "" && strings.ToLower(strings.TrimRight(r.URL, "/")) == want {
			return r.ID, nil
		}
	}
	return "", fmt.Errorf("%w: el consentimiento no autoriza el sitio %s; vuelve a conectar eligiendo ese sitio", connector.ErrBadQuery, site)
}

// doGet emite un GET por rt.HTTP con Accept JSON. Si la instancia usa el API token de
// Atlassian (auth básica correo+token), añade la cabecera; si no, la inyecta el Runtime.
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

// basicAuth compone la cabecera Basic (correo:API token) si la instancia los trae; ""
// si la auth va por el Bearer que inyecta el Runtime.
//
// El Bearer GANA cuando existe: ahí deja el canje del consentimiento el access token, y
// seguir mandando la credencial vieja haría que autorizar no sirviera de nada —la
// conexión seguiría colgando de un token pegado a mano que el admin cree haber
// sustituido—. Sin Bearer, la auth básica sigue funcionando igual que siempre.
func basicAuth(inst connector.Instance) string {
	if inst.HasBearer {
		return ""
	}
	email := strings.TrimSpace(firstNonEmpty(inst.Config[ConfigEmail], inst.Secrets[secretEmail]))
	token := inst.Secrets[secretAPIToken]
	if email == "" || token == "" {
		return ""
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token))
}

// readOK exige 2xx y devuelve el cuerpo acotado a maxRespBytes.
func readOK(resp *http.Response) ([]byte, error) {
	defer drainClose(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes+1))
	if err != nil {
		return nil, fmt.Errorf("jira: leer respuesta: %w", err)
	}
	if int64(len(body)) > maxRespBytes {
		return nil, fmt.Errorf("jira: respuesta demasiado grande (> %d bytes)", maxRespBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jira: el origen respondió HTTP %d: %s", resp.StatusCode, errorMessage(body))
	}
	return body, nil
}

// errorMessage extrae los errorMessages/errors del cuerpo de error de la API de Jira.
func errorMessage(data []byte) string {
	var e struct {
		ErrorMessages []string          `json:"errorMessages"`
		Errors        map[string]string `json:"errors"`
	}
	if err := json.Unmarshal(data, &e); err == nil {
		if len(e.ErrorMessages) > 0 && strings.TrimSpace(e.ErrorMessages[0]) != "" {
			return truncate(strings.TrimSpace(e.ErrorMessages[0]), 300)
		}
		for _, v := range e.Errors {
			if strings.TrimSpace(v) != "" {
				return truncate(strings.TrimSpace(v), 300)
			}
		}
	}
	return "(sin detalle)"
}

// escapeJQL escapa comillas y barras en un literal JQL (y descarta caracteres de
// control): el término no puede cerrar la cadena ni alterar la consulta.
func escapeJQL(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch {
		case r < 0x20:
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func asString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
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
