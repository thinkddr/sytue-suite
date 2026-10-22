// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// ErrUpstream indica un fallo del origen externo (conexión, HTTP no-2xx,
// respuesta ilegible/no-JSON, demasiado grande). La capa HTTP lo mapea a 502: no
// es culpa del cliente sino del destino remoto.
var ErrUpstream = errors.New("connectors: fallo del origen externo")

// executeREST hace la petición HTTP a la API externa con la barrera SSRF, inyecta
// la autenticación, acota el cuerpo y aplana el JSON a filas/columnas. El cliente
// (client) ya lleva el dialer SSRF-safe y la re-validación de redirects. oauthToken
// es el access token ya resuelto para auth OAuth2 (vacío para el resto de auth).
func executeREST(ctx context.Context, client *http.Client, cfg RestConfig, creds Credentials, q RestQuery, lim Limits, oauthToken string) (*Result, error) {
	finalURL, err := buildRestURL(cfg, creds, q)
	if err != nil {
		return nil, err
	}
	// Barrera 1 SSRF: valida esquema y que el host resuelva a IP pública.
	if err := ssrfhttp.ValidateURL(ctx, finalURL); err != nil {
		return nil, err
	}

	var body io.Reader = http.NoBody
	if q.Method == http.MethodPost && len(q.Body) > 0 {
		body = bytes.NewReader(q.Body)
	}
	req, err := http.NewRequestWithContext(ctx, q.Method, finalURL, body)
	if err != nil {
		return nil, invalid("no se pudo construir la petición")
	}
	req.Header.Set("Accept", "application/json, text/csv;q=0.9")
	req.Header.Set("User-Agent", "Sytue-Docs-Connector/1")
	if q.Method == http.MethodPost && len(q.Body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	// Cabeceras estáticas de la conexión, luego las de la consulta (no secretas).
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	for k, v := range q.Headers {
		req.Header.Set(k, v)
	}
	applyRestAuth(req, cfg.Auth, creds, oauthToken) // la api_key-en-query ya la puso buildRestURL

	resp, err := client.Do(req)
	if err != nil {
		// No envolver err crudo hacia el cliente (puede llevar el host/URL): la capa
		// HTTP no lo expone, pero mantenemos el detalle solo para logs de servidor.
		return nil, fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		// 401: con OAuth2 dispara UN refresh+reintento (ver executeRESTAuthed). Sigue
		// envuelto en ErrUpstream, así que si el 401 persiste mapea a 502 como antes.
		return nil, fmt.Errorf("%w: HTTP 401: %w", ErrUpstream, errUnauthorized)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUpstream, resp.StatusCode)
	}
	// Lee hasta MaxBytes+1: si excede, la respuesta es demasiado grande (no OOM).
	raw, err := io.ReadAll(io.LimitReader(resp.Body, lim.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: leer respuesta: %w", ErrUpstream, err)
	}
	if int64(len(raw)) > lim.MaxBytes {
		return nil, fmt.Errorf("%w: respuesta demasiado grande (> %d bytes)", ErrUpstream, lim.MaxBytes)
	}
	return bodyToResult(raw, resp.Header.Get("Content-Type"), q.RootPath, lim.MaxRows)
}

// bodyToResult lee la respuesta como JSON o, si el origen dice que es CSV —o es texto
// que no es JSON, como el de un fichero publicado en la web—, como CSV.
func bodyToResult(raw []byte, contentType, rootPath string, maxRows int) (*Result, error) {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "csv") || (strings.HasPrefix(ct, "text/") && !json.Valid(raw)) {
		return csvToResult(raw, maxRows)
	}
	return jsonToResult(raw, rootPath, maxRows)
}

// csvToResult aplana un CSV: la primera fila son los nombres de las columnas y el
// separador es el que más sale en ella de «,», «;» y el tabulador (Excel en español
// exporta con «;»). Los valores van como texto; el paso «Cambiar tipo» los convierte.
func csvToResult(raw []byte, maxRows int) (*Result, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")) // la marca BOM de Excel
	primera, _, _ := bytes.Cut(raw, []byte("\n"))
	sep := ','
	for _, s := range []rune{';', '\t'} {
		if bytes.Count(primera, []byte(string(s))) > bytes.Count(primera, []byte(string(sep))) {
			sep = s
		}
	}
	r := csv.NewReader(bytes.NewReader(raw))
	r.Comma, r.FieldsPerRecord, r.LazyQuotes = sep, -1, true
	cab, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: la respuesta no es JSON ni CSV", ErrUpstream)
	}
	cab = cab[:min(len(cab), maxColumns)]
	cols := make([]Column, len(cab))
	for i, n := range cab {
		if n = strings.TrimSpace(n); n == "" {
			n = "column" + strconv.Itoa(i+1)
		}
		cols[i] = Column{Name: n, Type: "text"}
	}
	rows := [][]any{}
	for len(rows) < maxRows {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: CSV mal formado", ErrUpstream)
		}
		fila := make([]any, len(cols))
		for i := range fila {
			fila[i] = ""
			if i < len(rec) {
				fila[i] = rec[i]
			}
		}
		rows = append(rows, fila)
	}
	return &Result{Columns: cols, Rows: rows}, nil
}

// buildRestURL resuelve la URL final (BaseURL + Path relativo + query) e inyecta
// la api_key si va en query. La ruta ya se validó como relativa; aquí se re-checa
// (defensa) para que jamás pueda cambiar de host.
func buildRestURL(cfg RestConfig, creds Credentials, q RestQuery) (string, error) {
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return "", invalid("URL base inválida")
	}
	ref := &url.URL{}
	if q.Path != "" {
		ref, err = url.Parse(q.Path)
		if err != nil {
			return "", invalid("ruta inválida")
		}
		if ref.IsAbs() || ref.Host != "" {
			return "", invalid("la ruta debe ser relativa a la URL base")
		}
	}
	final := base.ResolveReference(ref)
	if final.Host != base.Host || (final.Scheme != "http" && final.Scheme != "https") {
		return "", invalid("la ruta no puede cambiar el destino")
	}
	vals := final.Query()
	for k, v := range q.Query {
		vals.Set(k, v)
	}
	if cfg.Auth.Type == RestAuthAPIKey && cfg.Auth.In == "query" {
		vals.Set(cfg.Auth.Name, creds.Secret)
	}
	final.RawQuery = vals.Encode()
	return final.String(), nil
}

// applyRestAuth inyecta la autenticación en cabecera (api_key-en-header, Bearer,
// Basic, OAuth2). La api_key-en-query la gestiona buildRestURL. oauthToken es el
// access token ya resuelto (solo se usa en el caso OAuth2).
func applyRestAuth(req *http.Request, auth RestAuth, creds Credentials, oauthToken string) {
	switch auth.Type {
	case RestAuthAPIKey:
		if auth.In == "header" {
			req.Header.Set(auth.Name, creds.Secret)
		}
	case RestAuthBearer:
		req.Header.Set("Authorization", "Bearer "+creds.Secret)
	case RestAuthBasic:
		req.SetBasicAuth(auth.Username, creds.Secret)
	case RestAuthOAuth2ClientCreds:
		// El access token ya se obtuvo (cacheado) en executeRESTAuthed; aquí solo se
		// adjunta como Bearer. creds.Secret es el client_secret y NUNCA se manda a la API.
		req.Header.Set("Authorization", "Bearer "+oauthToken)
	case RestAuthNone:
		// sin auth
	}
}

// -----------------------------------------------------------------------------
// JSON -> tabla.
// -----------------------------------------------------------------------------

// jsonToResult aplana un cuerpo JSON a filas/columnas. Con RootPath desciende por
// un json-path simple hasta el array/objeto objetivo. Un array de objetos ⇒
// columnas = unión de claves (orden alfabético estable). Un array de escalares ⇒
// una columna "value". Un objeto único ⇒ una fila.
func jsonToResult(body []byte, rootPath string, maxRows int) (*Result, error) {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%w: la respuesta no es JSON", ErrUpstream)
	}
	target := doc
	if rootPath != "" {
		t, err := walkPath(doc, rootPath)
		if err != nil {
			return nil, err
		}
		target = t
	}

	var records []map[string]any
	switch tv := target.(type) {
	case []any:
		if len(tv) == 0 {
			return &Result{Columns: []Column{}, Rows: [][]any{}}, nil
		}
		if _, ok := tv[0].(map[string]any); !ok {
			return scalarArrayResult(tv, maxRows), nil
		}
		records = make([]map[string]any, 0, len(tv))
		for _, el := range tv {
			if m, ok := el.(map[string]any); ok {
				records = append(records, m)
			}
		}
	case map[string]any:
		records = []map[string]any{tv}
	default:
		return nil, invalid("la respuesta no es una tabla (se esperaba un array de objetos)")
	}

	// Columnas = unión de claves, orden alfabético (determinista: encoding/json no
	// preserva el orden de las claves de un objeto).
	// ponytail: orden alfabético; un orden "según la API" exigiría un parser JSON
	// ordenado — se añade si un caso real lo pide.
	colSet := map[string]struct{}{}
	for _, r := range records {
		for k := range r {
			colSet[k] = struct{}{}
		}
	}
	names := make([]string, 0, len(colSet))
	for k := range colSet {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) > maxColumns {
		names = names[:maxColumns]
	}
	cols := make([]Column, len(names))
	for i, n := range names {
		cols[i] = Column{Name: n, Type: inferColumnType(records, n)}
	}

	rowN := len(records)
	if rowN > maxRows {
		rowN = maxRows
	}
	rows := make([][]any, 0, rowN)
	for _, r := range records[:rowN] {
		row := make([]any, len(names))
		for i, n := range names {
			row[i] = normalizeJSON(r[n])
		}
		rows = append(rows, row)
	}
	return &Result{Columns: cols, Rows: rows}, nil
}

// walkPath desciende por un json-path simple: claves separadas por '.' e índices
// de array como [n] o como segmento numérico (a.b, a.0.b, a[0].b).
func walkPath(v any, path string) (any, error) {
	norm := strings.NewReplacer("[", ".", "]", "").Replace(path)
	for _, tok := range strings.Split(norm, ".") {
		if tok == "" {
			continue
		}
		switch cur := v.(type) {
		case map[string]any:
			nv, ok := cur[tok]
			if !ok {
				return nil, invalid("root_path: clave %q no encontrada", tok)
			}
			v = nv
		case []any:
			idx, err := strconv.Atoi(tok)
			if err != nil || idx < 0 || idx >= len(cur) {
				return nil, invalid("root_path: índice %q inválido", tok)
			}
			v = cur[idx]
		default:
			return nil, invalid("root_path: no se puede descender por %q", tok)
		}
	}
	return v, nil
}

// scalarArrayResult materializa un array de escalares como una única columna.
func scalarArrayResult(arr []any, maxRows int) *Result {
	n := len(arr)
	if n > maxRows {
		n = maxRows
	}
	rows := make([][]any, 0, n)
	for _, el := range arr[:n] {
		rows = append(rows, []any{normalizeJSON(el)})
	}
	typ := "text"
	if len(arr) > 0 {
		typ = jsonType(arr[0])
	}
	return &Result{Columns: []Column{{Name: "value", Type: typ}}, Rows: rows}
}

// inferColumnType infiere el tipo de una columna a partir del primer valor no
// nulo encontrado en las filas.
func inferColumnType(records []map[string]any, name string) string {
	for _, r := range records {
		if v, ok := r[name]; ok && v != nil {
			return jsonType(v)
		}
	}
	return "text"
}

// jsonType clasifica un valor JSON decodificado (encoding/json).
func jsonType(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "text"
	case []any, map[string]any:
		return "json"
	default:
		return "text"
	}
}

// normalizeJSON deja los escalares tal cual y serializa compuestos a JSON crudo.
func normalizeJSON(v any) any {
	switch v.(type) {
	case nil, bool, float64, string:
		return v
	default: // []any, map[string]any
		if b, err := json.Marshal(v); err == nil {
			return json.RawMessage(b)
		}
		return nil
	}
}
