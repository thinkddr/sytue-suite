// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package odoo es el driver TabularSource para Odoo (JSON-RPC 2.0): trae modelos
// (res.partner, account.move…) a Sytue Sheets vía `execute_kw`/`search_read`.
// Implementa connector.Driver (Test) y la capacidad opcional connector.TabularSource
// (Query). Todo el egress sale por rt.HTTP (SSRF + rate-limit + auditoría).
//
// Auth: Odoo lleva las credenciales EN EL CUERPO del RPC (db, uid, api_key), no en
// una cabecera, así que —a diferencia de los conectores header-auth— este driver
// SÍ recibe su secreto en Instance.Secrets (la pasarela retira la clave "bearer"
// convencional y entrega el resto; ver el despacho de /data/query). El uid se
// resuelve con `common.authenticate` (o se toma de la config si el admin lo fija).
package odoo

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
const Kind = "odoo"

// Claves de Instance.Config (públicas) y de Instance.Secrets (sensibles).
const (
	ConfigBaseURL  = "base_url" // raíz del Odoo (p. ej. https://acme.odoo.com)
	ConfigDatabase = "database" // nombre de la base de datos Odoo
	ConfigLogin    = "login"    // usuario/login para authenticate
	ConfigUID      = "uid"      // opcional: uid ya conocido (evita authenticate)

	secretAPIKey   = "api_key"  // clave de API (Odoo 14+) o…
	secretPassword = "password" // …contraseña; cualquiera vale como credencial
)

const (
	maxRespBytes   = 16 << 20 // tope de lectura de la respuesta RPC
	defaultLimit   = 10000    // tope de filas si la spec no lo fija
	rpcEndpoint    = "/jsonrpc"
	searchReadMeth = "search_read"
)

// Driver es el conector Odoo. Sin estado: cada operación usa el Runtime inyectado.
type Driver struct{}

// New construye el driver (se cablea en el Registry del gateway).
func New() *Driver { return &Driver{} }

// Kind identifica al driver en el Registry.
func (*Driver) Kind() string { return Kind }

// Test verifica conectividad e identidad del servidor con `common.version` (no
// requiere credenciales), usando EXCLUSIVAMENTE rt.HTTP. La validación de
// credenciales ocurre en Query (authenticate), que sí recibe el secreto.
func (*Driver) Test(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	base, err := baseURL(inst)
	if err != nil {
		return err
	}
	var ver map[string]any
	if err := call(ctx, rt, base, rpcParams("common", "version", nil), &ver); err != nil {
		return fmt.Errorf("odoo: test: %w", err)
	}
	if len(ver) == 0 {
		return fmt.Errorf("odoo: la raíz no respondió como un servidor Odoo")
	}
	return nil
}

// Query ejecuta search_read sobre un modelo y devuelve la Table. spec es odooQuery
// (modelo + fields + domain + limit/offset). Resuelve el uid (config o
// authenticate) y llama execute_kw. Egress por rt.HTTP.
func (*Driver) Query(ctx context.Context, inst connector.Instance, rt connector.Runtime, spec json.RawMessage) (connector.Table, error) {
	base, err := baseURL(inst)
	if err != nil {
		return connector.Table{}, err
	}
	db := strings.TrimSpace(inst.Config[ConfigDatabase])
	if db == "" {
		return connector.Table{}, fmt.Errorf("%w: falta %q en la config", connector.ErrBadQuery, ConfigDatabase)
	}
	secret := credential(inst)
	if secret == "" {
		return connector.Table{}, fmt.Errorf("%w: falta la credencial (%q/%q) de la instancia", connector.ErrBadQuery, secretAPIKey, secretPassword)
	}

	var q odooQuery
	if len(spec) > 0 {
		if err := json.Unmarshal(spec, &q); err != nil {
			return connector.Table{}, fmt.Errorf("%w: la spec no es JSON válido", connector.ErrBadQuery)
		}
	}
	if err := q.validate(); err != nil {
		return connector.Table{}, err
	}

	uid, err := resolveUID(ctx, rt, base, inst, db, secret)
	if err != nil {
		return connector.Table{}, err
	}

	kw := map[string]any{
		"fields": q.fields(),
		"limit":  q.limit(),
		"offset": q.Offset,
	}
	args := []any{db, uid, secret, q.model(), searchReadMeth, []any{q.domainOrEmpty()}, kw}
	var records []map[string]any
	if err := call(ctx, rt, base, rpcParams("object", "execute_kw", args), &records); err != nil {
		return connector.Table{}, fmt.Errorf("odoo: search_read: %w", err)
	}
	return connector.BuildTable(records, q.fields(), q.limit()), nil
}

// resolveUID devuelve el uid: el de la config si está fijado, o el de
// common.authenticate con la credencial.
func resolveUID(ctx context.Context, rt connector.Runtime, base string, inst connector.Instance, db, secret string) (int, error) {
	if raw := strings.TrimSpace(inst.Config[ConfigUID]); raw != "" {
		var uid int
		if _, err := fmt.Sscan(raw, &uid); err == nil && uid > 0 {
			return uid, nil
		}
	}
	login := strings.TrimSpace(inst.Config[ConfigLogin])
	if login == "" {
		return 0, fmt.Errorf("%w: falta %q (o %q) en la config", connector.ErrBadQuery, ConfigLogin, ConfigUID)
	}
	var uid int
	if err := call(ctx, rt, base, rpcParams("common", "authenticate", []any{db, login, secret, map[string]any{}}), &uid); err != nil {
		return 0, fmt.Errorf("odoo: authenticate: %w", err)
	}
	if uid <= 0 {
		return 0, fmt.Errorf("odoo: authenticate rechazó las credenciales")
	}
	return uid, nil
}

// --- Consulta ----------------------------------------------------------------

// odooQuery es la spec de una consulta Odoo (JSON opaco para el núcleo). Model y
// Dataset son sinónimos; Fields/Select también. Domain es un dominio Odoo crudo
// (lista de tuplas). Limit/Offset paginan; MaxRows es alias de Limit.
type odooQuery struct {
	Model   string   `json:"model,omitempty"`
	Dataset string   `json:"dataset,omitempty"`
	Fields  []string `json:"fields,omitempty"`
	Select  []string `json:"select,omitempty"`
	Domain  []any    `json:"domain,omitempty"`
	Limit   int      `json:"limit,omitempty"`
	Offset  int      `json:"offset,omitempty"`
	MaxRows int      `json:"max_rows,omitempty"`
}

func (q odooQuery) model() string {
	if strings.TrimSpace(q.Model) != "" {
		return strings.TrimSpace(q.Model)
	}
	return strings.TrimSpace(q.Dataset)
}

func (q odooQuery) fields() []string {
	if len(q.Fields) > 0 {
		return q.Fields
	}
	return q.Select
}

func (q odooQuery) limit() int {
	if q.Limit > 0 {
		return q.Limit
	}
	if q.MaxRows > 0 {
		return q.MaxRows
	}
	return defaultLimit
}

func (q odooQuery) domainOrEmpty() []any {
	if q.Domain == nil {
		return []any{}
	}
	return q.Domain
}

func (q odooQuery) validate() error {
	if q.model() == "" {
		return fmt.Errorf("%w: falta 'model' (modelo Odoo, p. ej. res.partner)", connector.ErrBadQuery)
	}
	if q.Offset < 0 || q.Limit < 0 || q.MaxRows < 0 {
		return fmt.Errorf("%w: limit/offset no pueden ser negativos", connector.ErrBadQuery)
	}
	return nil
}

// --- JSON-RPC ----------------------------------------------------------------

// rpcRequest es la envoltura JSON-RPC 2.0 de Odoo.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	ID      int    `json:"id"`
}

// rpcResponse es la respuesta JSON-RPC 2.0: result XOR error.
type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Message string `json:"message"`
	} `json:"data"`
}

// rpcParams construye los params de una llamada a un servicio Odoo (common/object).
func rpcParams(service, method string, args []any) map[string]any {
	if args == nil {
		args = []any{}
	}
	return map[string]any{"service": service, "method": method, "args": args}
}

// call ejecuta una llamada JSON-RPC contra {base}/jsonrpc por rt.HTTP y, si out !=
// nil, decodifica el "result" en out. Un "error" JSON-RPC se mapea a un error
// legible sin filtrar el cuerpo remoto crudo.
func call(ctx context.Context, rt connector.Runtime, base string, params, out any) error {
	payload, err := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: "call", Params: params, ID: 1})
	if err != nil {
		return fmt.Errorf("odoo: serializar petición: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+rpcEndpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Integration-Gateway/1")

	resp, err := rt.HTTP.Do(req)
	if err != nil {
		// Preserva ssrfhttp.ErrBlocked / errores de transporte (errors.Is en la capa HTTP).
		return err
	}
	defer drainClose(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("odoo: el origen respondió HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes+1))
	if err != nil {
		return fmt.Errorf("odoo: leer respuesta: %w", err)
	}
	if int64(len(body)) > maxRespBytes {
		return fmt.Errorf("odoo: respuesta demasiado grande (> %d bytes)", maxRespBytes)
	}
	var rr rpcResponse
	if err := json.Unmarshal(body, &rr); err != nil {
		return fmt.Errorf("odoo: la respuesta no es JSON-RPC: %w", err)
	}
	if rr.Error != nil {
		msg := rr.Error.Data.Message
		if msg == "" {
			msg = rr.Error.Message
		}
		return fmt.Errorf("odoo: error RPC %d: %s", rr.Error.Code, strings.TrimSpace(msg))
	}
	if out != nil && len(rr.Result) > 0 {
		if err := json.Unmarshal(rr.Result, out); err != nil {
			return fmt.Errorf("odoo: decodificar resultado: %w", err)
		}
	}
	return nil
}

// --- helpers -----------------------------------------------------------------

// baseURL valida y normaliza (sin barra final) la raíz del Odoo.
func baseURL(inst connector.Instance) (string, error) {
	raw := strings.TrimSpace(inst.Config[ConfigBaseURL])
	if raw == "" {
		return "", fmt.Errorf("%w: falta %q en la config de la instancia", connector.ErrBadQuery, ConfigBaseURL)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w: %q no es una URL http(s) válida", connector.ErrBadQuery, ConfigBaseURL)
	}
	return strings.TrimRight(raw, "/"), nil
}

// credential extrae la credencial de Instance.Secrets (api_key o password).
func credential(inst connector.Instance) string {
	if v := inst.Secrets[secretAPIKey]; v != "" {
		return v
	}
	return inst.Secrets[secretPassword]
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
