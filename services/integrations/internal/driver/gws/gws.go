// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package gws es el driver TabularSource para Google Workspace: trae el Directory
// (users, groups) a Sytue Sheets/Hub como tablas. Implementa connector.Driver (Test)
// y la capacidad opcional connector.TabularSource (Query).
//
// Auth: cuenta de servicio con delegación a nivel de dominio (el modo server-to-server
// del Directory API). El driver firma un JWT (RS256) con la clave privada de la cuenta
// de servicio —iss=client_email, sub=admin a impersonar, scope de solo lectura del
// Directory— y lo canjea por un access_token en el token endpoint de Google
// (grant jwt-bearer). Como Odoo, el driver recibe su secreto (el JSON de la cuenta de
// servicio) en Instance.Secrets: no hay un "bearer" estático en el vault; se mintea un
// token de vida corta por consulta. La paginación server-driven se sigue por
// `nextPageToken`. Todo el egress —token y datos— sale por rt.HTTP (SSRF + rate-limit
// + auditoría); el driver nunca construye su propio http.Client.
package gws

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	sytueoauth2 "git.sytue.eu/sytue/one/services/integrations/internal/oauth2"
)

// Kind es la clave del Registry con la que se registra este driver.
const Kind = "gws"

// Claves de Instance.Config (públicas) y de Instance.Secrets (sensible).
const (
	ConfigAdminEmail    = "admin_email"    // admin a impersonar (delegación a nivel de dominio)
	ConfigDomain        = "domain"         // opcional: filtra por dominio del Workspace
	ConfigCustomer      = "customer"       // opcional: id de cliente del Directory (def. my_customer)
	ConfigDirectoryBase = "directory_base" // opcional: raíz del Directory API (tests/soberanas)

	secretServiceKey = "service_account_key" // JSON de la cuenta de servicio (write-only)
)

const (
	defaultDirectoryBase = "https://admin.googleapis.com"
	//nolint:gosec // G101: endpoint público del token de Google, no una credencial.
	defaultTokenURL = "https://oauth2.googleapis.com/token"
	defaultCustomer = "my_customer"
	// scope de solo LECTURA del Directory (este es el plano de datos L3; el
	// aprovisionamiento saliente es otra ola).
	//nolint:gosec // G101: es un identificador de scope OAuth público, no una credencial.
	directoryScope = "https://www.googleapis.com/auth/admin.directory.user.readonly https://www.googleapis.com/auth/admin.directory.group.readonly"
	//nolint:gosec // G101: URN público del grant JWT-bearer, no una credencial.
	jwtBearerGrant = "urn:ietf:params:oauth:grant-type:jwt-bearer"

	maxRespBytes   = 32 << 20 // tope de lectura por página (defensa anti-OOM)
	maxPages       = 1000     // tope de saltos de nextPageToken (defensa anti-bucle)
	defaultMaxRows = 10000    // tope de filas acumuladas si la spec no lo fija
	pageSize       = 200      // tamaño de página del Directory (server-driven paging)
)

// allowedResources son los recursos del Directory que este driver expone; cada uno
// anida su array bajo una clave homónima en la respuesta (users→"users", …).
var allowedResources = map[string]struct{}{"users": {}, "groups": {}}

// Driver es el conector Google Workspace. Sin estado: cada operación usa el Runtime
// inyectado. Su valor cero es utilizable; New lo devuelve por simetría.
type Driver struct {
	// tokens es la caché de access token compartida por la pasarela: sin ella cada
	// consulta minteaba un JWT bearer nuevo contra Google.
	tokens *sytueoauth2.Cache
}

// New construye el driver (se cablea en el Registry del gateway).
func New(tokens *sytueoauth2.Cache) *Driver { return &Driver{tokens: tokens} }

// Kind identifica al driver en el Registry.
func (*Driver) Kind() string { return Kind }

// Test verifica que el Directory API es alcanzable (y no un host privado) con un GET a
// la colección de users. NO valida credenciales: la clave de la cuenta de servicio vive
// en el vault y solo se entrega a Query (Test recibe la instancia sin secretos, por
// construcción), así que sin token Google responde 401 —reachable— y la auth se valida
// en la primera consulta. Un destino privado ⇒ ssrfhttp.ErrBlocked por rt.HTTP.
func (*Driver) Test(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	base, err := directoryBase(inst)
	if err != nil {
		return err
	}
	u := base + "/admin/directory/v1/users?customer=" + url.QueryEscape(customer(inst)) + "&maxResults=1"
	resp, err := doGet(ctx, rt, u, "")
	if err != nil {
		return fmt.Errorf("gws: test: %w", err)
	}
	defer drainClose(resp)
	if resp.StatusCode >= 500 {
		return fmt.Errorf("gws: el Directory API respondió HTTP %d", resp.StatusCode)
	}
	return nil
}

// Query trae un recurso del Directory (users|groups) como Table. spec es gwsQuery
// (resource + query/fields). Mintea un token (cuenta de servicio, jwt-bearer) y sigue
// la paginación `nextPageToken` hasta agotar filas/páginas. Egress por rt.HTTP.
func (d *Driver) Query(ctx context.Context, inst connector.Instance, rt connector.Runtime, spec json.RawMessage) (connector.Table, error) {
	base, err := directoryBase(inst)
	if err != nil {
		return connector.Table{}, err
	}
	subject := strings.TrimSpace(inst.Config[ConfigAdminEmail])
	if subject == "" {
		return connector.Table{}, fmt.Errorf("%w: falta %q en la config (admin a impersonar)", connector.ErrBadQuery, ConfigAdminEmail)
	}
	sa, err := parseServiceAccount(inst.Secrets[secretServiceKey])
	if err != nil {
		return connector.Table{}, err
	}

	var q gwsQuery
	if len(spec) > 0 {
		if err := json.Unmarshal(spec, &q); err != nil {
			return connector.Table{}, fmt.Errorf("%w: la spec no es JSON válido", connector.ErrBadQuery)
		}
	}
	if err := q.validate(); err != nil {
		return connector.Table{}, err
	}

	token, err := cachedToken(ctx, d.tokens, rt, inst, sa, subject, directoryScope)
	if err != nil {
		return connector.Table{}, err
	}
	records, err := fetchAll(ctx, rt, q.buildURL(base, inst), q.resource(), token, q.maxRows())
	if err != nil {
		return connector.Table{}, err
	}
	return connector.BuildTable(records, nil, q.maxRows()), nil
}

// Search implementa connector.SearchCapability: busca PERSONAS en el Directory
// (users con el parámetro query) y las devuelve como hits externos para la búsqueda
// unificada de la suite. Mintea un token (cuenta de servicio, jwt-bearer, como
// Query) y consulta una sola página acotada por limit. Egress por rt.HTTP; un
// destino privado ⇒ ssrfhttp.ErrBlocked.
func (d *Driver) Search(ctx context.Context, inst connector.Instance, rt connector.Runtime, query string, limit int) ([]connector.ExternalHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	base, err := directoryBase(inst)
	if err != nil {
		return nil, err
	}
	subject := strings.TrimSpace(inst.Config[ConfigAdminEmail])
	if subject == "" {
		return nil, fmt.Errorf("%w: falta %q en la config (admin a impersonar)", connector.ErrBadQuery, ConfigAdminEmail)
	}
	sa, err := parseServiceAccount(inst.Secrets[secretServiceKey])
	if err != nil {
		return nil, err
	}
	token, err := cachedToken(ctx, d.tokens, rt, inst, sa, subject, directoryScope)
	if err != nil {
		return nil, err
	}

	v := url.Values{}
	v.Set("customer", customer(inst))
	if d := strings.TrimSpace(inst.Config[ConfigDomain]); d != "" {
		v.Set("domain", d)
	}
	// ponytail: prefijo de nombre; el Directory no permite OR entre campos en un solo
	// 'query'. Ampliar a email/otros campos si la cobertura de búsqueda hace falta.
	v.Set("query", "name:"+query)
	v.Set("fields", "users(primaryEmail,name/fullName)")
	v.Set("maxResults", strconv.Itoa(clampSearchLimit(limit)))
	u := base + "/admin/directory/v1/users?" + v.Encode()

	resp, err := doGet(ctx, rt, u, token)
	if err != nil {
		return nil, fmt.Errorf("gws: search: %w", err)
	}
	body, err := readOK(resp)
	if err != nil {
		return nil, err
	}
	recs, _, err := extractRecords(body, "users")
	if err != nil {
		return nil, err
	}
	hits := make([]connector.ExternalHit, 0, len(recs))
	for _, rec := range recs {
		email := asString(rec, "primaryEmail")
		title := ""
		if n, ok := rec["name"].(map[string]any); ok {
			title = asString(n, "fullName")
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
			Source:  "Google Workspace",
		})
	}
	return hits, nil
}

// --- Consulta ----------------------------------------------------------------

// gwsQuery es la spec de una consulta al Directory (JSON opaco para el núcleo).
// Resource y Dataset son sinónimos (users|groups); Query es el filtro 'query' del
// Directory API (p. ej. "orgUnitPath=/Ventas"); Fields es la proyección parcial
// (partial response); MaxRows acota el total acumulado en cliente.
type gwsQuery struct {
	Resource string `json:"resource,omitempty"`
	Dataset  string `json:"dataset,omitempty"`
	Query    string `json:"query,omitempty"`
	Fields   string `json:"fields,omitempty"`
	MaxRows  int    `json:"max_rows,omitempty"`
}

func (q gwsQuery) resource() string {
	if strings.TrimSpace(q.Resource) != "" {
		return strings.TrimSpace(q.Resource)
	}
	return strings.TrimSpace(q.Dataset)
}

func (q gwsQuery) maxRows() int {
	if q.MaxRows > 0 {
		return q.MaxRows
	}
	return defaultMaxRows
}

func (q gwsQuery) validate() error {
	if _, ok := allowedResources[q.resource()]; !ok {
		return fmt.Errorf("%w: 'resource' debe ser 'users' o 'groups'", connector.ErrBadQuery)
	}
	if q.MaxRows < 0 {
		return fmt.Errorf("%w: max_rows no puede ser negativo", connector.ErrBadQuery)
	}
	return nil
}

// buildURL compone la URL de la PRIMERA página del recurso con customer/domain/query/
// fields y el tamaño de página. Los saltos siguientes añaden pageToken (ver fetchAll).
func (q gwsQuery) buildURL(base string, inst connector.Instance) string {
	v := url.Values{}
	v.Set("customer", customer(inst))
	if d := strings.TrimSpace(inst.Config[ConfigDomain]); d != "" {
		v.Set("domain", d)
	}
	if s := strings.TrimSpace(q.Query); s != "" {
		v.Set("query", s)
	}
	if f := strings.TrimSpace(q.Fields); f != "" {
		v.Set("fields", f)
	}
	v.Set("maxResults", strconv.Itoa(pageSize))
	return base + "/admin/directory/v1/" + q.resource() + "?" + v.Encode()
}

// fetchAll recorre las páginas del Directory siguiendo nextPageToken hasta agotar filas
// o páginas, acotando en cliente por maxRows (no carga todas las páginas en memoria). El
// pageToken se añade a la MISMA URL (mismo host por construcción); cada GET se re-valida
// por SSRF en rt.HTTP.
func fetchAll(ctx context.Context, rt connector.Runtime, first, arrayKey, token string, maxRows int) ([]map[string]any, error) {
	out := make([]map[string]any, 0, 128)
	pageToken := ""
	for page := 0; page < maxPages; page++ {
		u := first
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		resp, err := doGet(ctx, rt, u, token)
		if err != nil {
			// Preserva ssrfhttp.ErrBlocked / errores de transporte (errors.Is en la capa HTTP).
			return nil, fmt.Errorf("gws: %w", err)
		}
		body, err := readOK(resp)
		if err != nil {
			return nil, err
		}
		recs, next, err := extractRecords(body, arrayKey)
		if err != nil {
			return nil, err
		}
		for _, rec := range recs {
			out = append(out, rec)
			if len(out) >= maxRows {
				return out[:maxRows], nil
			}
		}
		if next == "" {
			break
		}
		pageToken = next
	}
	return out, nil
}

// extractRecords decodifica una página del Directory: el array del recurso (bajo la
// clave arrayKey: "users"/"groups") y el nextPageToken. Decodifica solo lo necesario
// (no re-serializa la página entera).
func extractRecords(body []byte, arrayKey string) ([]map[string]any, string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, "", fmt.Errorf("gws: la respuesta no es un objeto JSON: %w", err)
	}
	var next string
	if t, ok := raw["nextPageToken"]; ok {
		_ = json.Unmarshal(t, &next)
	}
	var recs []map[string]any
	if arr, ok := raw[arrayKey]; ok {
		if err := json.Unmarshal(arr, &recs); err != nil {
			return nil, "", fmt.Errorf("gws: %q no es una lista de objetos: %w", arrayKey, err)
		}
	}
	return recs, next, nil
}

// --- Cuenta de servicio + OAuth2 (jwt-bearer) --------------------------------

// serviceAccount es lo justo del JSON de una cuenta de servicio de Google para firmar y
// canjear el assertion: el email (iss), la clave privada RSA PEM y el token endpoint.
type serviceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
	// ClientID es el identificador NUMÉRICO de la cuenta de servicio. No hace falta
	// para firmar: es lo que pide la pantalla de delegación a nivel de dominio de la
	// consola de administración, y tenerlo aquí permite llevar al administrador a esa
	// pantalla con el cliente y los permisos ya rellenos en vez de dictárselos.
	ClientID string `json:"client_id"`
}

// ServiceAccountClientID devuelve el client_id de la cuenta de servicio de la
// instancia, o "" si no hay cuenta de servicio o su JSON no lo trae. No es un
// secreto (identifica al cliente, no lo autentica): la pantalla de delegación de
// Google lo pide escrito a mano.
func ServiceAccountClientID(inst connector.Instance) string {
	sa, err := parseServiceAccount(inst.Secrets[secretServiceKey])
	if err != nil {
		return ""
	}
	return sa.ClientID
}

// parseServiceAccount decodifica y valida el JSON de la cuenta de servicio del vault.
func parseServiceAccount(raw string) (serviceAccount, error) {
	if strings.TrimSpace(raw) == "" {
		return serviceAccount{}, fmt.Errorf("%w: falta el secreto %q de la instancia", connector.ErrBadQuery, secretServiceKey)
	}
	var sa serviceAccount
	if err := json.Unmarshal([]byte(raw), &sa); err != nil {
		return serviceAccount{}, fmt.Errorf("%w: el secreto no es un JSON de cuenta de servicio válido", connector.ErrBadQuery)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return serviceAccount{}, fmt.Errorf("%w: la cuenta de servicio necesita client_email y private_key", connector.ErrBadQuery)
	}
	if sa.TokenURI == "" {
		sa.TokenURI = defaultTokenURL
	}
	return sa, nil
}

// UserToken mintea un access_token impersonando a subject (delegación a nivel de
// dominio) con el scope pedido, usando la cuenta de servicio de la instancia. Lo
// expone para la migración de contenido (internal/migrate), que necesita scopes
// distintos a los del Directory (Drive/People de solo lectura) pero exactamente el
// MISMO mecanismo de auth: el minteo del token vive en UN solo sitio. subject
// vacío ⇒ el admin configurado en la instancia. Egress por rt.HTTP.
func UserToken(ctx context.Context, rt connector.Runtime, inst connector.Instance, subject, scope string) (string, error) {
	sa, err := parseServiceAccount(inst.Secrets[secretServiceKey])
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(subject) == "" {
		subject = strings.TrimSpace(inst.Config[ConfigAdminEmail])
	}
	if subject == "" {
		return "", fmt.Errorf("%w: falta el usuario a impersonar (%q o el usuario de origen)", connector.ErrBadQuery, ConfigAdminEmail)
	}
	return fetchToken(ctx, rt, sa, subject, scope)
}

// fetchToken firma un JWT con la cuenta de servicio (impersonando a subject, con el
// scope pedido) y lo canjea por un access_token en el token endpoint por rt.HTTP. El
// assertion va en el CUERPO (form-urlencoded), así que este es el único punto donde
// el driver usa el secreto.
//
// ponytail: sin caché de token —una consulta mintea uno y lo usa para sus páginas—;
// añadir caché por org:instance (TTL de expires_in) si el volumen de consultas aprieta.
func fetchToken(ctx context.Context, rt connector.Runtime, sa serviceAccount, subject, scope string) (string, error) {
	assertion, err := signJWT(sa, subject, scope)
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type": {jwtBearerGrant},
		"assertion":  {assertion},
	}
	return sytueoauth2.Fetch(ctx, rt, sytueoauth2.Request{
		TokenURL: sa.TokenURI,
		Form:     form,
		Provider: "gws",
	})
}

// cachedToken es el camino normal del driver: el mismo canje, pero cacheado. La clave
// lleva el sujeto suplantado y el scope porque el token de Google es POR usuario
// delegado: dos sujetos distintos no pueden compartirlo.
func cachedToken(ctx context.Context, tokens *sytueoauth2.Cache, rt connector.Runtime, inst connector.Instance, sa serviceAccount, subject, scope string) (string, error) {
	assertion, err := signJWT(sa, subject, scope)
	if err != nil {
		return "", err
	}
	return tokens.Token(ctx, rt, sytueoauth2.Request{
		Key:      "gws|" + inst.OrgID + "|" + inst.ID + "|" + subject + "|" + scope,
		TokenURL: sa.TokenURI,
		Form: url.Values{
			"grant_type": {jwtBearerGrant},
			"assertion":  {assertion},
		},
		Provider: "gws",
	})
}

// signJWT construye y firma (RS256) el JWT assertion de la cuenta de servicio: iss=
// client_email, sub=subject (delegación a nivel de dominio), el scope pedido (siempre
// de solo lectura), aud=token endpoint, con vigencia de 1h.
func signJWT(sa serviceAccount, subject, scope string) (string, error) {
	key, err := parseRSAKey(sa.PrivateKey)
	if err != nil {
		return "", err
	}
	now := time.Now()
	header := b64url([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iss":   sa.ClientEmail,
		"sub":   subject,
		"scope": scope,
		"aud":   sa.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("gws: serializar claims: %w", err)
	}
	signingInput := header + "." + b64url(claims)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("gws: firmar JWT: %w", err)
	}
	return signingInput + "." + b64url(sig), nil
}

// parseRSAKey decodifica la clave privada PEM de la cuenta de servicio. Google emite
// PKCS#8 ("BEGIN PRIVATE KEY"); se acepta también PKCS#1 por robustez.
func parseRSAKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("%w: la clave privada de la cuenta de servicio no es PEM", connector.ErrBadQuery)
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%w: la clave de la cuenta de servicio no es RSA", connector.ErrBadQuery)
		}
		return rk, nil
	}
	rk, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: clave privada de cuenta de servicio ilegible", connector.ErrBadQuery)
	}
	return rk, nil
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

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

// clampSearchLimit acota el maxResults de una búsqueda a un rango razonable (1..50).
func clampSearchLimit(n int) int {
	if n <= 0 {
		return 10
	}
	if n > 50 {
		return 50
	}
	return n
}

// --- helpers de red ----------------------------------------------------------

// directoryBase valida y normaliza (sin barra final) la raíz del Directory API; por
// defecto https://admin.googleapis.com (config directory_base la sobrescribe).
func directoryBase(inst connector.Instance) (string, error) {
	raw := strings.TrimSpace(inst.Config[ConfigDirectoryBase])
	if raw == "" {
		return defaultDirectoryBase, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w: %q no es una URL http(s) válida", connector.ErrBadQuery, ConfigDirectoryBase)
	}
	return strings.TrimRight(raw, "/"), nil
}

// customer devuelve el id de cliente del Directory (config o el alias my_customer).
func customer(inst connector.Instance) string {
	if c := strings.TrimSpace(inst.Config[ConfigCustomer]); c != "" {
		return c
	}
	return defaultCustomer
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
		return nil, fmt.Errorf("gws: leer respuesta: %w", err)
	}
	if int64(len(body)) > maxRespBytes {
		return nil, fmt.Errorf("gws: respuesta demasiado grande (> %d bytes)", maxRespBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gws: el origen respondió HTTP %d: %s", resp.StatusCode, googleErrorMessage(body))
	}
	return body, nil
}

// googleErrorMessage extrae el "error.message" (o "error_description") de un cuerpo de
// error de Google, acotado.
func googleErrorMessage(data []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(data, &e); err == nil {
		if m := strings.TrimSpace(e.Error.Message); m != "" {
			return truncate(m, 300)
		}
		if m := strings.TrimSpace(e.ErrorDescription); m != "" {
			return truncate(m, 300)
		}
	}
	return "(sin detalle)"
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
