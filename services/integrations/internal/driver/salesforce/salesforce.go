// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package salesforce es el driver TabularSource + SearchCapability de Salesforce:
// trae objetos del CRM (Account, Contact, Opportunity, cualquier SObject) a Sytue
// Sheets como tablas vía SOQL, y alimenta la búsqueda federada de Sytue Hub con SOSL.
//
// Auth: token OAuth2 de la instancia (Authorization: Bearer). NO se mintea aquí: el
// token vive en el vault bajo la clave convencional "bearer" y lo INYECTA el Runtime
// de la pasarela en cada petición (como los conectores header-auth tipo OData), así
// que este driver nunca ve el secreto. Todo el egress sale por rt.HTTP (SSRF +
// rate-limit + auditoría); el driver jamás construye su propio http.Client.
//
// Paginación: la REST API de Salesforce es server-driven —una página trae hasta 2000
// registros y `nextRecordsUrl` apunta a la siguiente—. Se sigue con guardia de MISMO
// HOST (un nextRecordsUrl que cambiara de host filtraría el token inyectado) y con
// topes de filas/páginas: nunca se materializa un dataset entero sin límite.
package salesforce

import (
	"context"
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
const Kind = "salesforce"

// Claves de Instance.Config (públicas; sin secretos: el token lo inyecta el Runtime).
const (
	ConfigInstanceURL = "instance_url" // raíz de la org: https://acme.my.salesforce.com
	ConfigSubdomain   = "subdomain"    // alternativa (campo del alta 1-click): «acme»
	ConfigAPIVersion  = "api_version"  // opcional: v61.0 (o 61.0)
)

const (
	defaultAPIVersion = "v61.0"
	salesforceHost    = ".my.salesforce.com"

	maxRespBytes   = 32 << 20 // tope de lectura por página (defensa anti-OOM)
	maxPages       = 1000     // tope de saltos de nextRecordsUrl (defensa anti-bucle)
	defaultMaxRows = 10000    // tope de filas acumuladas si la spec no lo fija
)

// Driver es el conector Salesforce. Sin estado: cada operación usa el Runtime inyectado.
type Driver struct{}

// New construye el driver (se cablea en el Registry del gateway).
func New() *Driver { return &Driver{} }

// Kind identifica al driver en el Registry.
func (*Driver) Kind() string { return Kind }

// Test verifica que la org es alcanzable y que la credencial vale: GET /services/data
// (el listado de versiones de la REST API). El Bearer del vault lo inyecta el Runtime,
// así que un token inválido se manifiesta como 401 aquí. Un destino privado ⇒
// ssrfhttp.ErrBlocked por rt.HTTP.
func (*Driver) Test(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	base, err := baseURL(inst)
	if err != nil {
		return err
	}
	resp, err := doGet(ctx, rt, base+"/services/data")
	if err != nil {
		return fmt.Errorf("salesforce: test: %w", err)
	}
	if _, err := readOK(resp); err != nil {
		return fmt.Errorf("salesforce: test: %w", err)
	}
	return nil
}

// Query ejecuta una consulta SOQL y devuelve sus registros como Table, siguiendo
// `nextRecordsUrl` hasta agotar filas/páginas. spec es sfQuery.
func (*Driver) Query(ctx context.Context, inst connector.Instance, rt connector.Runtime, spec json.RawMessage) (connector.Table, error) {
	base, err := baseURL(inst)
	if err != nil {
		return connector.Table{}, err
	}
	version, err := apiVersion(inst)
	if err != nil {
		return connector.Table{}, err
	}
	var q sfQuery
	if len(spec) > 0 {
		if err := json.Unmarshal(spec, &q); err != nil {
			return connector.Table{}, fmt.Errorf("%w: la spec no es JSON válido", connector.ErrBadQuery)
		}
	}
	soql, err := q.statement()
	if err != nil {
		return connector.Table{}, err
	}

	first := base + "/services/data/" + version + "/query?q=" + url.QueryEscape(soql)
	records, err := fetchAll(ctx, rt, base, first, q.maxRows())
	if err != nil {
		return connector.Table{}, err
	}
	return connector.BuildTable(records, presentFields(soqlFields(soql), records), q.maxRows()), nil
}

// Search implementa connector.SearchCapability: SOSL (FIND {…}) sobre las entidades de
// CRM de más uso (Account, Contact, Opportunity, Case) y mapeo a hits con deep-link
// Lightning al registro. Una sola página acotada por limit; egress por rt.HTTP.
func (*Driver) Search(ctx context.Context, inst connector.Instance, rt connector.Runtime, query string, limit int) ([]connector.ExternalHit, error) {
	term := escapeSOSL(strings.TrimSpace(query))
	if term == "" {
		return nil, nil
	}
	base, err := baseURL(inst)
	if err != nil {
		return nil, err
	}
	version, err := apiVersion(inst)
	if err != nil {
		return nil, err
	}
	sosl := "FIND {" + term + "} IN ALL FIELDS RETURNING " +
		"Account(Id,Name,Website), Contact(Id,Name,Email), Opportunity(Id,Name,StageName), Case(Id,CaseNumber,Subject) " +
		"LIMIT " + strconv.Itoa(clampLimit(limit))

	resp, err := doGet(ctx, rt, base+"/services/data/"+version+"/search?q="+url.QueryEscape(sosl))
	if err != nil {
		return nil, fmt.Errorf("salesforce: search: %w", err)
	}
	body, err := readOK(resp)
	if err != nil {
		return nil, err
	}
	var sr struct {
		SearchRecords []map[string]any `json:"searchRecords"`
	}
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("salesforce: la respuesta de búsqueda no es JSON: %w", err)
	}
	hits := make([]connector.ExternalHit, 0, len(sr.SearchRecords))
	for _, rec := range sr.SearchRecords {
		if h, ok := hitOf(base, rec); ok {
			hits = append(hits, h)
		}
	}
	return hits, nil
}

// hitOf mapea un registro SOSL a ExternalHit: título por Name/Subject/CaseNumber, tipo
// desde attributes.type y deep-link a la vista Lightning del registro.
func hitOf(base string, rec map[string]any) (connector.ExternalHit, bool) {
	id := asString(rec, "Id")
	if id == "" {
		return connector.ExternalHit{}, false
	}
	sobject := "Record"
	if attrs, ok := rec["attributes"].(map[string]any); ok {
		if t := asString(attrs, "type"); t != "" {
			sobject = t
		}
	}
	title := firstNonEmpty(asString(rec, "Name"), asString(rec, "Subject"), asString(rec, "CaseNumber"))
	if title == "" {
		title = sobject + " " + id
	}
	return connector.ExternalHit{
		Title:   title,
		URL:     base + "/lightning/r/" + url.PathEscape(sobject) + "/" + url.PathEscape(id) + "/view",
		Snippet: firstNonEmpty(asString(rec, "Email"), asString(rec, "StageName"), asString(rec, "Website"), sobject),
		Kind:    strings.ToLower(sobject),
		Source:  "Salesforce",
	}, true
}

// --- Consulta ----------------------------------------------------------------

// sfQuery es la spec de una consulta (JSON opaco para el núcleo): SOQL cruda o un
// SELECT compuesto a partir de object/fields/where/limit. MaxRows acota el total
// acumulado en cliente (independiente de la paginación server-driven).
type sfQuery struct {
	SOQL    string   `json:"soql,omitempty"`
	Query   string   `json:"query,omitempty"` // sinónimo de soql
	Object  string   `json:"object,omitempty"`
	Fields  []string `json:"fields,omitempty"`
	Where   string   `json:"where,omitempty"`
	Limit   int      `json:"limit,omitempty"`
	MaxRows int      `json:"max_rows,omitempty"`
}

func (q sfQuery) maxRows() int {
	if q.MaxRows > 0 {
		return q.MaxRows
	}
	return defaultMaxRows
}

// statement devuelve la SOQL a ejecutar: la cruda si viene, o la compuesta desde
// object/fields. Solo se admiten SELECT (el endpoint /query es de lectura).
func (q sfQuery) statement() (string, error) {
	if q.MaxRows < 0 || q.Limit < 0 {
		return "", fmt.Errorf("%w: limit/max_rows no pueden ser negativos", connector.ErrBadQuery)
	}
	raw := strings.TrimSpace(firstNonEmpty(q.SOQL, q.Query))
	if raw != "" {
		if !strings.HasPrefix(strings.ToUpper(raw), "SELECT") {
			return "", fmt.Errorf("%w: solo se admiten consultas SELECT (SOQL)", connector.ErrBadQuery)
		}
		return raw, nil
	}
	obj := strings.TrimSpace(q.Object)
	if obj == "" {
		return "", fmt.Errorf("%w: falta 'soql' (o 'object' + 'fields')", connector.ErrBadQuery)
	}
	if !isIdentifier(obj) {
		return "", fmt.Errorf("%w: nombre de objeto inválido", connector.ErrBadQuery)
	}
	fields := make([]string, 0, len(q.Fields))
	for _, f := range q.Fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !isIdentifier(f) {
			return "", fmt.Errorf("%w: nombre de campo inválido %q", connector.ErrBadQuery, f)
		}
		fields = append(fields, f)
	}
	if len(fields) == 0 {
		fields = []string{"Id", "Name"}
	}
	stmt := "SELECT " + strings.Join(fields, ", ") + " FROM " + obj
	if w := strings.TrimSpace(q.Where); w != "" {
		stmt += " WHERE " + w
	}
	if q.Limit > 0 {
		stmt += " LIMIT " + strconv.Itoa(q.Limit)
	}
	return stmt, nil
}

// queryPage es la envoltura de una página de /query: registros + enlace a la siguiente.
type queryPage struct {
	Records        []map[string]any `json:"records"`
	Done           bool             `json:"done"`
	NextRecordsURL string           `json:"nextRecordsUrl"`
}

// fetchAll recorre las páginas siguiendo nextRecordsUrl hasta agotar filas o páginas.
// Acota en cliente por maxRows (no carga el dataset entero) y re-valida cada salto por
// SSRF en rt.HTTP, exigiendo además que quede en el MISMO host que la org.
func fetchAll(ctx context.Context, rt connector.Runtime, base, first string, maxRows int) ([]map[string]any, error) {
	baseHost := hostOf(base)
	out := make([]map[string]any, 0, 128)
	next := first
	for page := 0; next != "" && page < maxPages; page++ {
		resp, err := doGet(ctx, rt, next)
		if err != nil {
			// Preserva ssrfhttp.ErrBlocked / errores de transporte (errors.Is aguas arriba).
			return nil, fmt.Errorf("salesforce: %w", err)
		}
		body, err := readOK(resp)
		if err != nil {
			return nil, err
		}
		var pg queryPage
		if err := json.Unmarshal(body, &pg); err != nil {
			return nil, fmt.Errorf("salesforce: la respuesta no es un resultado de consulta JSON: %w", err)
		}
		for _, rec := range pg.Records {
			delete(rec, "attributes") // metadatos de control (type/url): no son columnas.
			out = append(out, rec)
			if len(out) >= maxRows {
				return out[:maxRows], nil
			}
		}
		nl := strings.TrimSpace(pg.NextRecordsURL)
		if pg.Done || nl == "" {
			break
		}
		if next, err = resolveNext(next, baseHost, nl); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// resolveNext resuelve un nextRecordsUrl (normalmente una ruta relativa) y exige que
// apunte al MISMO host que la org: un salto cross-host filtraría el token inyectado.
func resolveNext(current, baseHost, nl string) (string, error) {
	nu, err := url.Parse(nl)
	if err != nil {
		return "", fmt.Errorf("salesforce: nextRecordsUrl inválido: %w", err)
	}
	if !nu.IsAbs() {
		b, err := url.Parse(current)
		if err != nil {
			return "", fmt.Errorf("salesforce: URL de página inválida: %w", err)
		}
		nu = b.ResolveReference(nu)
	}
	if nu.Scheme != "http" && nu.Scheme != "https" {
		return "", fmt.Errorf("salesforce: nextRecordsUrl con esquema no permitido")
	}
	if !strings.EqualFold(nu.Hostname(), baseHost) {
		return "", fmt.Errorf("salesforce: nextRecordsUrl cambia de host (%s → %s): bloqueado", baseHost, nu.Hostname())
	}
	return nu.String(), nil
}

// --- helpers -----------------------------------------------------------------

// baseURL normaliza la raíz de la org: instance_url si viene, o el subdominio del alta
// 1-click (https://SUB.my.salesforce.com). Se conserva SOLO esquema+host (una ruta en
// la config no debe colarse en las URLs de API).
func baseURL(inst connector.Instance) (string, error) {
	raw := strings.TrimSpace(inst.Config[ConfigInstanceURL])
	if raw == "" {
		sub := strings.TrimSpace(inst.Config[ConfigSubdomain])
		if sub == "" {
			return "", fmt.Errorf("%w: falta %q (o %q) en la config", connector.ErrBadQuery, ConfigInstanceURL, ConfigSubdomain)
		}
		if strings.ContainsAny(sub, "/:?#@") {
			return "", fmt.Errorf("%w: %q inválido", connector.ErrBadQuery, ConfigSubdomain)
		}
		return "https://" + sub + salesforceHost, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w: %q no es una URL http(s) válida", connector.ErrBadQuery, ConfigInstanceURL)
	}
	return u.Scheme + "://" + u.Host, nil
}

// apiVersion normaliza la versión de la REST API (v61.0). Acepta «61.0» y «v61.0».
func apiVersion(inst connector.Instance) (string, error) {
	v := strings.TrimSpace(inst.Config[ConfigAPIVersion])
	if v == "" {
		return defaultAPIVersion, nil
	}
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	for _, r := range v {
		if (r < '0' || r > '9') && r != '.' {
			return "", fmt.Errorf("%w: %q no es una versión de API válida (p. ej. v61.0)", connector.ErrBadQuery, ConfigAPIVersion)
		}
	}
	if v == "" {
		return "", fmt.Errorf("%w: %q no es una versión de API válida (p. ej. v61.0)", connector.ErrBadQuery, ConfigAPIVersion)
	}
	return "v" + v, nil
}

// soqlFields extrae la lista de campos del SELECT para ordenar las columnas como la
// consulta. Devuelve nil ante agregados/subconsultas/alias (⇒ orden alfabético).
func soqlFields(soql string) []string {
	s := strings.TrimSpace(soql)
	if len(s) < 6 || !strings.EqualFold(s[:6], "SELECT") {
		return nil
	}
	rest := s[6:]
	i := strings.Index(strings.ToUpper(rest), " FROM ")
	if i < 0 {
		return nil
	}
	list := rest[:i]
	if strings.ContainsAny(list, "()") {
		return nil
	}
	parts := strings.Split(list, ",")
	out := make([]string, 0, len(parts))
	for _, f := range parts {
		f = strings.TrimSpace(f)
		if !isIdentifier(f) {
			return nil
		}
		out = append(out, f)
	}
	return out
}

// presentFields filtra el orden propuesto a las claves REALMENTE presentes en los
// registros: Salesforce devuelve las claves con la capitalización del esquema y las
// relaciones anidadas como objeto, así que un nombre que no case no debe crear una
// columna vacía.
func presentFields(order []string, records []map[string]any) []string {
	if len(order) == 0 || len(records) == 0 {
		return nil
	}
	out := make([]string, 0, len(order))
	for _, f := range order {
		for _, r := range records {
			if _, ok := r[f]; ok {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// isIdentifier acepta nombres de campo/objeto SOQL (incluye la notación de relación
// Account.Name); rechaza espacios, comillas y cualquier metacarácter.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

// escapeSOSL escapa los metacaracteres reservados de SOSL en el término de búsqueda
// (evita romper —o inyectar en— la cláusula FIND {…}).
func escapeSOSL(s string) string {
	const reserved = `\?&|!{}[]()^~*:"'+-`
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		if r < 0x20 {
			continue
		}
		if strings.ContainsRune(reserved, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// doGet emite un GET por rt.HTTP con Accept JSON. La auth (Bearer del vault) la inyecta
// el Runtime: el driver no fija Authorization.
func doGet(ctx context.Context, rt connector.Runtime, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Integration-Gateway/1")
	return rt.HTTP.Do(req)
}

// readOK exige 2xx y devuelve el cuerpo acotado a maxRespBytes.
func readOK(resp *http.Response) ([]byte, error) {
	defer drainClose(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes+1))
	if err != nil {
		return nil, fmt.Errorf("salesforce: leer respuesta: %w", err)
	}
	if int64(len(body)) > maxRespBytes {
		return nil, fmt.Errorf("salesforce: respuesta demasiado grande (> %d bytes)", maxRespBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("salesforce: el origen respondió HTTP %d: %s", resp.StatusCode, errorMessage(body))
	}
	return body, nil
}

// errorMessage extrae el mensaje de un error de la REST API de Salesforce, que llega
// como array [{errorCode, message}].
func errorMessage(data []byte) string {
	var arr []struct {
		ErrorCode string `json:"errorCode"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(data, &arr); err == nil && len(arr) > 0 {
		msg := strings.TrimSpace(arr[0].Message)
		if msg == "" {
			msg = arr[0].ErrorCode
		}
		if msg != "" {
			return truncate(msg, 300)
		}
	}
	return "(sin detalle)"
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

// clampLimit acota el LIMIT de una búsqueda a un rango razonable (1..50).
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
