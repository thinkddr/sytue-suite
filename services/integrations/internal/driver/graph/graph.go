// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package graph es el driver TabularSource para Microsoft Graph (Microsoft 365):
// trae recursos de Graph —users, groups, eventos de calendario, items de drive/
// SharePoint— a Sytue Sheets/Hub como tablas. Implementa connector.Driver (Test) y
// la capacidad opcional connector.TabularSource (Query).
//
// Graph ES OData v4, así que la paginación server-driven se sigue por
// `@odata.nextLink` (misma lógica same-host que el driver odata: un nextLink que
// cambie de host filtraría el token, se bloquea). Soporta $select/$top/$filter.
//
// Auth OAuth2 client-credentials contra Entra: el driver PIDE un token de app al
// token endpoint ({authority}/{tenant}/oauth2/v2.0/token) con client_id +
// client_secret, y lo pone como `Authorization: Bearer` en las llamadas a Graph. A
// diferencia de los conectores header-auth (OData), aquí NO hay un "bearer" estático
// en el vault: el driver recibe el client_secret en Instance.Secrets (como Odoo) y
// mintea un token de vida corta por consulta. Todo el egress —token y datos— sale
// por rt.HTTP (SSRF + rate-limit + auditoría); el driver nunca construye su cliente.
package graph

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
	sytueoauth2 "git.sytue.eu/sytue/one/services/integrations/internal/oauth2"
)

// Kind es la clave del Registry con la que se registra este driver.
const Kind = "graph"

// Claves de Instance.Config (públicas) y de Instance.Secrets (sensible).
const (
	ConfigTenant        = "tenant"         // id de inquilino Entra (GUID o dominio .onmicrosoft.com)
	ConfigClientID      = "client_id"      // id de aplicación (cliente) del registro en Entra
	ConfigGraphBase     = "graph_base"     // opcional: raíz de Graph (nubes soberanas/tests/Sytue)
	ConfigAuthorityBase = "authority_base" // opcional: raíz del token endpoint (soberanas/tests)

	secretClientSecret = "client_secret" // secreto de cliente del registro de app (write-only)
)

const (
	defaultGraphBase     = "https://graph.microsoft.com"
	defaultAuthorityBase = "https://login.microsoftonline.com"
	graphScope           = "https://graph.microsoft.com/.default"
	apiVersion           = "v1.0"

	maxRespBytes   = 32 << 20 // tope de lectura por página (defensa anti-OOM)
	maxPages       = 1000     // tope de saltos de nextLink (defensa anti-bucle)
	defaultMaxRows = 10000    // tope de filas acumuladas si la spec no lo fija
)

// Driver es el conector Microsoft Graph. Sin estado: cada operación usa el Runtime
// inyectado. Su valor cero es utilizable; New lo devuelve por simetría.
type Driver struct {
	// tokens es la caché de access token compartida por toda la pasarela. Sin ella,
	// cada consulta minteaba un token nuevo contra Entra.
	tokens *sytueoauth2.Cache
}

// New construye el driver (se cablea en el Registry del gateway). La caché de tokens la
// comparten todos los drivers: el Runtime se fabrica por petición, así que una caché
// dentro de él no cachearía nada entre peticiones.
func New(tokens *sytueoauth2.Cache) *Driver { return &Driver{tokens: tokens} }

// Kind identifica al driver en el Registry.
func (*Driver) Kind() string { return Kind }

// Test verifica que la raíz de Graph es alcanzable (y no un host privado) con un GET
// anónimo a $metadata (el CSDL de Graph es público). NO valida credenciales: el
// client_secret vive en el vault y solo se entrega a Query (Test recibe la instancia
// sin secretos, por construcción), así que la auth se valida en la primera consulta.
// Un destino privado ⇒ ssrfhttp.ErrBlocked por rt.HTTP.
//
// Contra la propia suite se sondea /me/drive: /compat/graph emula el subconjunto de
// Graph que usan las integraciones, no su $metadata, y pedirlo devolvería un 501 que
// haría fallar la prueba de algo que sí está disponible. Sin token responde 401
// (alcanzable), igual que Graph.
func (*Driver) Test(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	base, err := graphBase(inst)
	if err != nil {
		return err
	}
	probe := base + "/" + apiVersion + "/$metadata"
	if isSytue(inst) {
		probe = base + "/" + apiVersion + "/me/drive"
	}
	resp, err := doGet(ctx, rt, probe, "")
	if err != nil {
		return fmt.Errorf("graph: test: %w", err)
	}
	defer drainClose(resp)
	if resp.StatusCode >= 500 {
		return fmt.Errorf("graph: la raíz de Graph respondió HTTP %d", resp.StatusCode)
	}
	return nil
}

// Query trae una colección de un recurso Graph como Table. spec es graphQuery
// (resource + $select/$filter/$top). Mintea un token de app (client-credentials) y
// sigue la paginación `@odata.nextLink` hasta agotar filas/páginas. Egress por rt.HTTP.
func (d *Driver) Query(ctx context.Context, inst connector.Instance, rt connector.Runtime, spec json.RawMessage) (connector.Table, error) {
	base, err := graphBase(inst)
	if err != nil {
		return connector.Table{}, err
	}
	var q graphQuery
	if len(spec) > 0 {
		if err := json.Unmarshal(spec, &q); err != nil {
			return connector.Table{}, fmt.Errorf("%w: la spec no es JSON válido", connector.ErrBadQuery)
		}
	}
	if err := q.validate(); err != nil {
		return connector.Table{}, err
	}

	token, err := instanceToken(ctx, d.tokens, rt, inst)
	if err != nil {
		return connector.Table{}, err
	}
	records, err := fetchAll(ctx, rt, base, q.buildURL(base), token, q.maxRows())
	if err != nil {
		return connector.Table{}, err
	}
	return connector.BuildTable(records, q.Select, q.maxRows()), nil
}

// Search implementa connector.SearchCapability: busca PERSONAS en el directorio de
// Entra (Graph /users con la consulta avanzada $search) y las devuelve como hits
// externos para la búsqueda unificada de la suite. Mintea un token de app
// (client-credentials, como Query) y consulta una sola página acotada por limit.
// Egress por rt.HTTP; un destino privado ⇒ ssrfhttp.ErrBlocked.
func (d *Driver) Search(ctx context.Context, inst connector.Instance, rt connector.Runtime, query string, limit int) ([]connector.ExternalHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if isSytue(inst) {
		// La búsqueda de este driver es el directorio de Entra (/users con $search), que
		// la capa /compat de Sytue no emula: las personas de la suite ya las busca el Hub
		// nativamente. No participar es la respuesta correcta; fingir un fallo por
		// conector en cada búsqueda federada, no.
		return nil, nil
	}
	base, err := graphBase(inst)
	if err != nil {
		return nil, err
	}
	token, err := instanceToken(ctx, d.tokens, rt, inst)
	if err != nil {
		return nil, err
	}

	// $search de Graph exige la cabecera ConsistencyLevel: eventual y $count=true.
	u := base + "/" + apiVersion + "/users?$search=" + url.QueryEscape(`"displayName:`+query+`"`) +
		"&$select=id,displayName,userPrincipalName,mail&$count=true&$top=" + strconv.Itoa(clampSearchLimit(limit))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Integration-Gateway/1")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("ConsistencyLevel", "eventual")

	resp, err := rt.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graph: search: %w", err)
	}
	body, err := readOK(resp)
	if err != nil {
		return nil, err
	}
	var pg graphPage
	if err := json.Unmarshal(body, &pg); err != nil {
		return nil, fmt.Errorf("graph: la respuesta de búsqueda no es una colección JSON: %w", err)
	}
	hits := make([]connector.ExternalHit, 0, len(pg.Value))
	for _, rec := range pg.Value {
		title := asString(rec, "displayName")
		email := asString(rec, "userPrincipalName")
		if email == "" {
			email = asString(rec, "mail")
		}
		if title == "" {
			title = email
		}
		if title == "" {
			continue
		}
		hits = append(hits, connector.ExternalHit{
			Title:   title,
			URL:     mailtoOr(email), // ponytail: un usuario de directorio no tiene URL http canónica; mailto es el deep-link accionable
			Snippet: email,
			Kind:    "person",
			Source:  "Microsoft 365",
		})
	}
	return hits, nil
}

// --- Reutilización por otras capacidades del gateway --------------------------

// APIBase devuelve la raíz de Graph configurada para la instancia (por defecto
// https://graph.microsoft.com). La expone para que la migración de contenido
// (internal/migrate) apunte a la misma nube que el resto del conector.
func APIBase(inst connector.Instance) (string, error) { return graphBase(inst) }

// AppToken mintea un token de APLICACIÓN (client-credentials) de la instancia. Lo
// usa la migración de contenido, que recorre /users/{upn}/drive y /users/{upn}/
// contacts con la misma credencial de Entra que el resto del conector; así el
// minteo del token vive en UN solo sitio. Egress por rt.HTTP.
func AppToken(ctx context.Context, rt connector.Runtime, inst connector.Instance) (string, error) {
	tenant := strings.TrimSpace(inst.Config[ConfigTenant])
	clientID := strings.TrimSpace(inst.Config[ConfigClientID])
	secret := inst.Secrets[secretClientSecret]
	if tenant == "" || clientID == "" || secret == "" {
		return "", fmt.Errorf("%w: faltan credenciales de la instancia", connector.ErrBadQuery)
	}
	authority, err := authorityBase(inst)
	if err != nil {
		return "", err
	}
	// Sin caché a propósito: la migración pide el token al arrancar el job y no vuelve.
	return sytueoauth2.Fetch(ctx, rt, appTokenRequest(authority, tenant, clientID, secret))
}

// appTokenRequest arma el canje client-credentials contra Entra. Lo comparten el camino
// cacheado del driver y el de un solo disparo de la migración, para que el grant y el
// scope se escriban UNA vez.
func appTokenRequest(authority, tenant, clientID, secret string) sytueoauth2.Request {
	return sytueoauth2.Request{
		TokenURL: authority + "/" + url.PathEscape(tenant) + "/oauth2/v2.0/token",
		Form: url.Values{
			"grant_type":    {"client_credentials"},
			"client_id":     {clientID},
			"client_secret": {secret},
			"scope":         {graphScope},
		},
		Provider: "graph",
	}
}

// instanceToken mintea el token con el que se llama a la API, SEGÚN LA INSTANCIA: el de
// aplicación de Entra contra Microsoft 365, o el de Sytue ID (acotado a una persona)
// cuando la instancia apunta a los endpoints /compat de la propia suite. Ver sytue.go.
func instanceToken(ctx context.Context, tokens *sytueoauth2.Cache, rt connector.Runtime, inst connector.Instance) (string, error) {
	if isSytue(inst) {
		return sytueToken(ctx, rt, inst)
	}
	// Camino CACHEADO: es el de las consultas del usuario, que antes minteaba un token
	// por cada una. AppToken (sin caché) queda para el disparo único de la migración.
	tenant := strings.TrimSpace(inst.Config[ConfigTenant])
	clientID := strings.TrimSpace(inst.Config[ConfigClientID])
	secret := inst.Secrets[secretClientSecret]
	if tenant == "" || clientID == "" || secret == "" {
		return "", fmt.Errorf("%w: faltan credenciales de la instancia", connector.ErrBadQuery)
	}
	return fetchToken(ctx, tokens, rt, inst, tenant, clientID, secret)
}

// --- Consulta ----------------------------------------------------------------

// graphQuery es la spec de una consulta Graph (JSON opaco para el núcleo). Resource
// y Dataset son sinónimos (p. ej. "users", "groups", "me/events",
// "sites/root/lists"); Filter es un $filter OData crudo; Top acota la página;
// MaxRows el total acumulado en cliente (independiente del server-driven paging).
type graphQuery struct {
	Resource string   `json:"resource,omitempty"`
	Dataset  string   `json:"dataset,omitempty"`
	Select   []string `json:"select,omitempty"`
	Filter   string   `json:"filter,omitempty"`
	Top      int      `json:"top,omitempty"`
	MaxRows  int      `json:"max_rows,omitempty"`
}

func (q graphQuery) resource() string {
	if strings.TrimSpace(q.Resource) != "" {
		return strings.TrimSpace(q.Resource)
	}
	return strings.TrimSpace(q.Dataset)
}

func (q graphQuery) maxRows() int {
	if q.MaxRows > 0 {
		return q.MaxRows
	}
	return defaultMaxRows
}

func (q graphQuery) validate() error {
	r := q.resource()
	if r == "" {
		return fmt.Errorf("%w: falta 'resource' (recurso Graph, p. ej. users)", connector.ErrBadQuery)
	}
	// El host lo fija graphBase; validar el path evita URLs malformadas e inyección de
	// query/fragmento (un '/' anidado SÍ es válido: users/{id}/messages).
	if strings.HasPrefix(r, "/") || strings.Contains(r, "..") || strings.ContainsAny(r, "?#") || strings.IndexFunc(r, isSpace) >= 0 {
		return fmt.Errorf("%w: nombre de recurso inválido", connector.ErrBadQuery)
	}
	if q.Top < 0 {
		return fmt.Errorf("%w: top no puede ser negativo", connector.ErrBadQuery)
	}
	if q.MaxRows < 0 {
		return fmt.Errorf("%w: max_rows no puede ser negativo", connector.ErrBadQuery)
	}
	return nil
}

// buildURL compone la URL de la PRIMERA página: raíz + /v1.0/ + recurso + parámetros
// OData ($select/$filter/$top). Los saltos siguientes se toman verbatim del nextLink.
func (q graphQuery) buildURL(base string) string {
	u := base + "/" + apiVersion + "/" + q.resource()
	var ps []string
	if len(q.Select) > 0 {
		ps = append(ps, "$select="+url.QueryEscape(strings.Join(q.Select, ",")))
	}
	if strings.TrimSpace(q.Filter) != "" {
		ps = append(ps, "$filter="+url.QueryEscape(q.Filter))
	}
	if q.Top > 0 {
		ps = append(ps, "$top="+strconv.Itoa(q.Top))
	}
	if len(ps) > 0 {
		u += "?" + strings.Join(ps, "&")
	}
	return u
}

// graphPage es la envoltura de una colección Graph (OData v4): los objetos en "value"
// y el enlace a la página siguiente (server-driven paging).
type graphPage struct {
	Value    []map[string]any `json:"value"`
	NextLink string           `json:"@odata.nextLink"`
}

// fetchAll recorre las páginas siguiendo nextLink hasta agotar filas o páginas.
// Acumula los registros (sin las anotaciones @odata.* de control), acotando en
// cliente por maxRows: no carga todas las páginas en memoria de golpe. Cada salto se
// re-valida por SSRF en rt.HTTP y se exige que quede en el MISMO host que la raíz.
func fetchAll(ctx context.Context, rt connector.Runtime, base, first, token string, maxRows int) ([]map[string]any, error) {
	baseHost := hostOf(base)
	out := make([]map[string]any, 0, 128)
	next := first
	for page := 0; next != "" && page < maxPages; page++ {
		resp, err := doGet(ctx, rt, next, token)
		if err != nil {
			// Preserva ssrfhttp.ErrBlocked / errores de transporte (errors.Is en la capa HTTP).
			return nil, fmt.Errorf("graph: %w", err)
		}
		body, err := readOK(resp)
		if err != nil {
			return nil, err
		}
		var pg graphPage
		if err := json.Unmarshal(body, &pg); err != nil {
			return nil, fmt.Errorf("graph: la respuesta no es una colección JSON: %w", err)
		}
		for _, rec := range pg.Value {
			out = append(out, stripControl(rec))
			if len(out) >= maxRows {
				return out[:maxRows], nil
			}
		}
		nl := strings.TrimSpace(pg.NextLink)
		if nl == "" {
			break
		}
		if next, err = resolveNext(next, baseHost, nl); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// resolveNext resuelve un @odata.nextLink (absoluto o relativo a la página actual) y
// exige que apunte al MISMO host que la raíz de Graph. Un nextLink que cambie de host
// filtraría el token a otro destino: se bloquea.
func resolveNext(current, baseHost, nl string) (string, error) {
	nu, err := url.Parse(nl)
	if err != nil {
		return "", fmt.Errorf("graph: nextLink inválido: %w", err)
	}
	if !nu.IsAbs() {
		b, err := url.Parse(current)
		if err != nil {
			return "", fmt.Errorf("graph: URL de página inválida: %w", err)
		}
		nu = b.ResolveReference(nu)
	}
	if nu.Scheme != "http" && nu.Scheme != "https" {
		return "", fmt.Errorf("graph: nextLink con esquema no permitido")
	}
	if !strings.EqualFold(nu.Hostname(), baseHost) {
		return "", fmt.Errorf("graph: nextLink cambia de host (%s → %s): bloqueado", baseHost, nu.Hostname())
	}
	return nu.String(), nil
}

// --- OAuth2 client-credentials (Entra) ---------------------------------------

// tokenResp es la respuesta del token endpoint de Entra (OAuth2): access_token o el
// par error/error_description.
type tokenResp struct {
	AccessToken      string `json:"access_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// fetchToken obtiene un token de app (client-credentials) del token endpoint de Entra
// por rt.HTTP. El client_secret va en el CUERPO (form-urlencoded), no en cabecera, así
// que este es el ÚNICO punto donde el driver usa el secreto de la instancia.
//
// ponytail: sin caché de token —una consulta mintea uno y lo usa para sus páginas—;
// añadir caché por org:instance (TTL de expires_in) si el volumen de consultas aprieta.
func fetchToken(ctx context.Context, tokens *sytueoauth2.Cache, rt connector.Runtime, inst connector.Instance, tenant, clientID, secret string) (string, error) {
	authority, err := authorityBase(inst)
	if err != nil {
		return "", err
	}
	req := appTokenRequest(authority, tenant, clientID, secret)
	// La clave incluye tenant y client: dos instancias de la misma organización contra
	// tenants distintos no pueden compartir token.
	req.Key = "graph|" + inst.OrgID + "|" + inst.ID + "|" + tenant + "|" + clientID
	return tokens.Token(ctx, rt, req)
}

// --- helpers de red ----------------------------------------------------------

// graphBase valida y normaliza (sin barra final) la raíz de Graph; por defecto
// https://graph.microsoft.com (config graph_base la sobrescribe para soberanas/tests).
func graphBase(inst connector.Instance) (string, error) {
	return baseURLOr(inst.Config[ConfigGraphBase], defaultGraphBase, ConfigGraphBase)
}

// authorityBase valida y normaliza (sin barra final) la raíz del token endpoint; por
// defecto https://login.microsoftonline.com.
func authorityBase(inst connector.Instance) (string, error) {
	return baseURLOr(inst.Config[ConfigAuthorityBase], defaultAuthorityBase, ConfigAuthorityBase)
}

func baseURLOr(raw, def, key string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w: %q no es una URL http(s) válida", connector.ErrBadQuery, key)
	}
	return strings.TrimRight(raw, "/"), nil
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// doGet emite un GET por rt.HTTP con Accept JSON y, si token != "", el Bearer minteado
// por el driver (rt.HTTP no inyecta ninguno: estos conectores no tienen bearer estático
// en el vault). No valida el status (lo hacen los llamantes).
func doGet(ctx context.Context, rt connector.Runtime, rawURL, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Integration-Gateway/1")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return rt.HTTP.Do(req)
}

// readOK exige 2xx y devuelve el cuerpo acotado a maxRespBytes.
func readOK(resp *http.Response) ([]byte, error) {
	defer drainClose(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes+1))
	if err != nil {
		return nil, fmt.Errorf("graph: leer respuesta: %w", err)
	}
	if int64(len(body)) > maxRespBytes {
		return nil, fmt.Errorf("graph: respuesta demasiado grande (> %d bytes)", maxRespBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("graph: el origen respondió HTTP %d: %s", resp.StatusCode, graphErrorMessage(body))
	}
	return body, nil
}

// graphErrorMessage extrae el motivo de un cuerpo de error, acotado: el "error.message"
// de Graph o, si el cuerpo es un error OAuth2 (el token endpoint de Entra o de Sytue ID,
// donde "error" es una CADENA), su error_description. Sin este segundo caso, un canje
// rechazado se reportaba como «(sin detalle)» y no había forma de saber por qué.
func graphErrorMessage(data []byte) string {
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &e); err == nil && strings.TrimSpace(e.Error.Message) != "" {
		return truncate(strings.TrimSpace(e.Error.Message), 300)
	}
	var oe struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(data, &oe); err == nil {
		if d := strings.TrimSpace(oe.Description); d != "" {
			return truncate(d, 300)
		}
		if c := strings.TrimSpace(oe.Error); c != "" {
			return truncate(c, 300)
		}
	}
	return "(sin detalle)"
}

// stripControl elimina las anotaciones de control OData (@odata.etag, @odata.id…) de un
// registro para no contaminar las columnas.
func stripControl(rec map[string]any) map[string]any {
	for k := range rec {
		if strings.HasPrefix(k, "@") {
			delete(rec, k)
		}
	}
	return rec
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }

// asString devuelve el valor string de una clave de un registro decodificado, o "".
func asString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// mailtoOr devuelve un deep-link mailto: para un email, o "" si no hay email.
func mailtoOr(email string) string {
	if email == "" {
		return ""
	}
	return "mailto:" + email
}

// clampSearchLimit acota el $top de una búsqueda a un rango razonable (1..50).
func clampSearchLimit(n int) int {
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
