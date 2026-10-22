// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Endpoints del conector de datos externos (Power Query para Sytue Sheets): CRUD
// de conexiones y consultas guardadas + execute/refresh. Todas las rutas son
// tenant-scoped (RLS en el store). Las credenciales se cifran al escribir y
// NUNCA se devuelven ni se registran; el endpoint execute las descifra solo en
// memoria y llama al ejecutor (SSRF en REST, guard SELECT + solo-lectura en PG).
//
// Contrato de execute (para el carril editor):
//
//	POST /api/v1/connectors/execute
//	  body (inline):     {"connection_id":"<id>","query":{QuerySpec}}
//	  body (guardada):   {"query_id":"<id>"}
//	  200 -> {"columns":[{"name":..,"type":..}], "rows":[[..],..]}
//	  400 config/consulta inválida o destino SSRF bloqueado; 404 conexión/consulta
//	  inexistente; 429 rate-limit; 502 fallo del origen externo; 504 timeout.
//
// Ámbito: las conexiones son recursos COMPARTIDOS del tenant: cualquier miembro
// autenticado las ve, les guarda consultas y las ejecuta (el aislamiento es por tenant
// vía RLS). CAMBIARLAS O BORRARLAS es de quien las creó o de quien administra la
// organización, y cambiar a dónde viaja su credencial sin volver a escribirla la
// invalida (connectors.Destino): con la edición abierta a todos, bastaba apuntar la
// conexión de otra persona a un servidor propio y ejecutarla para recibir su secreto.
// ponytail: sin ACL por-usuario para USARLAS; se añade si un caso lo pide.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/connectors"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// connectorStore es el contrato de persistencia del conector (lo satisface
// *store.Store). Se declara aquí para no acoplarlo a domain.Store (estas tablas
// no son del dominio documental) y permitir fakes en tests.
type connectorStore interface {
	CreateConnection(ctx context.Context, name string, kind connectors.Kind, config json.RawMessage, sealed []byte, createdBy string) (*connectors.DataConnection, error)
	ListConnections(ctx context.Context) ([]connectors.DataConnection, error)
	GetConnection(ctx context.Context, id string) (*connectors.DataConnection, error)
	UpdateConnection(ctx context.Context, id, name string, config json.RawMessage, sealed []byte, replaceCreds bool) error
	DeleteConnection(ctx context.Context, id string) error
	LoadConnectionSecret(ctx context.Context, id string) (connectors.Kind, json.RawMessage, []byte, error)

	CreateQuery(ctx context.Context, connID, name string, spec json.RawMessage, createdBy string) (*connectors.DataQuery, error)
	ListQueries(ctx context.Context, connID string) ([]connectors.DataQuery, error)
	UpdateQuery(ctx context.Context, id, name string, spec json.RawMessage) error
	DeleteQuery(ctx context.Context, id string) error
	LoadQueryForExecute(ctx context.Context, queryID string) (connectors.Kind, json.RawMessage, []byte, json.RawMessage, error)
}

// failConnector mapea los errores del conector a los HTTP canónicos.
func failConnector(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.Fail(w, r, httpx.ErrNotFound)
	case errors.Is(err, connectors.ErrInvalid):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(userMessageConnector(err)))
	case errors.Is(err, ssrfhttp.ErrBlocked), errors.Is(err, ssrfhttp.ErrInvalidURL):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Destination not allowed (blocked by network security)."))
	case errors.Is(err, connectors.ErrNoCipher):
		httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "encryption_unavailable",
			"El cifrado de credenciales no está configurado en el servidor."))
	case errors.Is(err, connectors.ErrCiphertext):
		httpx.Fail(w, r, httpx.ErrInternal) // credencial corrupta: no es culpa del cliente
	case errors.Is(err, connectors.ErrNotAllowed):
		// El código es el de la pasarela: la interfaz pone el aviso en el idioma de
		// quien lo lee (extdata.msg.notAllowed).
		httpx.Fail(w, r, httpx.NewError(http.StatusForbidden, "connection_not_allowed",
			"No estás en la lista de quién puede consultar esta conexión."))
	case errors.Is(err, connectors.ErrUpstream):
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "upstream_error",
			"El origen de datos externo devolvió un error."))
	case errors.Is(err, context.DeadlineExceeded):
		httpx.Fail(w, r, httpx.NewError(http.StatusGatewayTimeout, "timeout",
			"El origen de datos externo tardó demasiado."))
	case errors.Is(err, tenancy.ErrNoTenant):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The token has no tenant."))
	default:
		httpx.Fail(w, r, err)
	}
}

// userMessageConnector expone el mensaje (seguro, en español) del error de
// validación: connectors.ErrInvalid tiene la forma "…entrada inválida: <msg>".
func userMessageConnector(err error) string {
	const marker = "entrada inválida: "
	msg := err.Error()
	if i := strings.Index(msg, marker); i >= 0 {
		return msg[i+len(marker):]
	}
	return "Invalid request."
}

// --- Conexiones ---

type connectionReq struct {
	Name        string                  `json:"name"`
	Kind        connectors.Kind         `json:"kind"`
	Config      json.RawMessage         `json:"config"`
	Credentials *connectors.Credentials `json:"credentials,omitempty"`
}

func (h *Handler) listConnections(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	conns, err := h.connStore.ListConnections(r.Context())
	if err != nil {
		failConnector(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"connections": conns})
}

func (h *Handler) createConnection(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req connectionReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, config, err := validateConnectionInput(req)
	if err != nil {
		failConnector(w, r, err)
		return
	}
	sealed, err := h.sealCredentials(req.Credentials)
	if err != nil {
		failConnector(w, r, err)
		return
	}
	conn, err := h.connStore.CreateConnection(r.Context(), name, req.Kind, config, sealed, subj.Sub)
	if err != nil {
		failConnector(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"connection": conn})
}

func (h *Handler) getConnection(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	conn, err := h.connStore.GetConnection(r.Context(), chi.URLParam(r, "connId"))
	if err != nil {
		failConnector(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"connection": conn})
}

// errSoloQuienLaCreo: cambiar o borrar una conexión no es de cualquier miembro.
var errSoloQuienLaCreo = httpx.ErrForbidden.WithCode("connection_owner_only").
	WithMessage("Only whoever created this connection or an organisation administrator can change or delete it.")

// conexionEditable carga la conexión de la ruta si quien llama puede cambiarla o
// borrarla: quien la creó o quien administra la organización. Si no, ya ha contestado.
func (h *Handler) conexionEditable(w http.ResponseWriter, r *http.Request) (*connectors.DataConnection, bool) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return nil, false
	}
	existing, err := h.connStore.GetConnection(r.Context(), chi.URLParam(r, "connId"))
	if err != nil {
		failConnector(w, r, err)
		return nil, false
	}
	if (subj.Sub == "" || existing.CreatedBy != subj.Sub) && !esAdminOrg(r) {
		httpx.Fail(w, r, errSoloQuienLaCreo)
		return nil, false
	}
	return existing, true
}

func (h *Handler) updateConnection(w http.ResponseWriter, r *http.Request) {
	existing, ok := h.conexionEditable(w, r)
	if !ok {
		return
	}
	var req connectionReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// El kind no se puede cambiar tras crear: se toma el de la conexión existente.
	req.Kind = existing.Kind
	name, config, err := validateConnectionInput(req)
	if err != nil {
		failConnector(w, r, err)
		return
	}
	// credentials ausente => no se toca; presente => se reemplaza (secret vacío
	// elimina la credencial).
	var sealed []byte
	replaceCreds := req.Credentials != nil
	if replaceCreds && req.Credentials.Secret != "" {
		sealed, err = h.sealCredentials(req.Credentials)
		if err != nil {
			failConnector(w, r, err)
			return
		}
	}
	// …salvo que cambie a dónde viaja: entonces la guardada deja de valer y hay que
	// volver a escribirla (la respuesta sale con `has_credentials: false`).
	if !replaceCreds && connectors.Destino(existing.Kind, existing.Config) != connectors.Destino(existing.Kind, config) {
		replaceCreds = true
	}
	if err := h.connStore.UpdateConnection(r.Context(), existing.ID, name, config, sealed, replaceCreds); err != nil {
		failConnector(w, r, err)
		return
	}
	conn, err := h.connStore.GetConnection(r.Context(), existing.ID)
	if err != nil {
		failConnector(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"connection": conn})
}

func (h *Handler) deleteConnection(w http.ResponseWriter, r *http.Request) {
	existing, ok := h.conexionEditable(w, r)
	if !ok {
		return
	}
	if err := h.connStore.DeleteConnection(r.Context(), existing.ID); err != nil {
		failConnector(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// validateConnectionInput valida nombre + kind + config y devuelve la config
// saneada (canónica). No toca credenciales.
func validateConnectionInput(req connectionReq) (name string, config json.RawMessage, err error) {
	name, err = connectors.ValidateName(req.Name)
	if err != nil {
		return "", nil, err
	}
	if !req.Kind.Valid() {
		return "", nil, connectors.ErrInvalid
	}
	config, err = connectors.SanitizeConfig(req.Kind, req.Config)
	if err != nil {
		return "", nil, err
	}
	return name, config, nil
}

// sealCredentials cifra las credenciales (o nil si no hay secreto). Sin Sealer
// configurado y con secreto presente => ErrNoCipher (no se guarda en claro).
func (h *Handler) sealCredentials(c *connectors.Credentials) ([]byte, error) {
	if c == nil || c.Secret == "" {
		return nil, nil
	}
	//nolint:gosec // G117: se serializa precisamente PARA cifrarlo con el Sealer; nunca se persiste ni se responde en claro.
	plain, _ := json.Marshal(c)
	return h.sealer.Seal(plain)
}

// --- Consultas guardadas ---

type queryReq struct {
	Name string          `json:"name"`
	Spec json.RawMessage `json:"spec"`
}

func (h *Handler) listQueries(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	qs, err := h.connStore.ListQueries(r.Context(), chi.URLParam(r, "connId"))
	if err != nil {
		failConnector(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"queries": qs})
}

func (h *Handler) createQuery(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	connID := chi.URLParam(r, "connId")
	var req queryReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, spec, err := h.validateQuerySpec(r.Context(), connID, req)
	if err != nil {
		failConnector(w, r, err)
		return
	}
	q, err := h.connStore.CreateQuery(r.Context(), connID, name, spec, subj.Sub)
	if err != nil {
		failConnector(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"query": q})
}

func (h *Handler) updateQuery(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	connID := chi.URLParam(r, "connId")
	var req queryReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, spec, err := h.validateQuerySpec(r.Context(), connID, req)
	if err != nil {
		failConnector(w, r, err)
		return
	}
	if err := h.connStore.UpdateQuery(r.Context(), chi.URLParam(r, "queryId"), name, spec); err != nil {
		failConnector(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) deleteQuery(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if err := h.connStore.DeleteQuery(r.Context(), chi.URLParam(r, "queryId")); err != nil {
		failConnector(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// validateQuerySpec valida {name, spec} contra el kind de la conexión y devuelve
// nombre + spec saneada (canónica).
func (h *Handler) validateQuerySpec(ctx context.Context, connID string, req queryReq) (name string, spec json.RawMessage, err error) {
	name, err = connectors.ValidateName(req.Name)
	if err != nil {
		return "", nil, err
	}
	conn, err := h.connStore.GetConnection(ctx, connID)
	if err != nil {
		return "", nil, err
	}
	spec, err = connectors.SanitizeSpec(conn.Kind, req.Spec)
	if err != nil {
		return "", nil, err
	}
	return name, spec, nil
}

// --- Ejecutar / refrescar ---

type executeReq struct {
	// Modo 1: consulta guardada.
	QueryID string `json:"query_id,omitempty"`
	// Modo 2: inline (conexión + consulta).
	ConnectionID string          `json:"connection_id,omitempty"`
	Query        json.RawMessage `json:"query,omitempty"`
}

func (h *Handler) executeConnectorQuery(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	// Rate-limit por tenant: el execute abre red saliente (coste/abuso).
	if h.execLimiter != nil {
		if res, err := h.execLimiter.Allow(r.Context(), "connectors:"+subj.Tenant); err == nil && !res.Allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(res.RetryAfter.Seconds())+1))
			httpx.Fail(w, r, httpx.ErrTooManyRequests)
			return
		}
	}

	var req executeReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	kind, config, sealed, spec, err := h.resolveExecuteTarget(r, req)
	if err != nil {
		failConnector(w, r, err)
		return
	}

	creds, err := h.openCredentials(sealed)
	if err != nil {
		failConnector(w, r, err)
		return
	}

	// Passthrough del JWT del usuario para el origen "gateway" (Sytue Integration
	// Gateway): el token efímero viaja por el contexto, no como credencial. Cubre
	// tanto el execute inline como el refresh de consulta guardada (ambos aquí).
	ctx := connectors.WithUserToken(r.Context(), bearerFrom(r))

	res, err := h.executor.ExecuteRaw(ctx, kind, config, creds, spec)
	if err != nil {
		failConnector(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

// bearerFrom extrae el token Bearer crudo de la cabecera Authorization entrante
// (la ruta ya pasó RequireAuth, así que el formato es "Bearer <jwt>"). Vacío si no
// hay cabecera Bearer. authn.bearerToken es interno de la plataforma, de ahí este
// extractor mínimo local.
func bearerFrom(r *http.Request) string {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}

// resolveExecuteTarget obtiene kind/config/credencial-cifrada/spec según el modo:
// consulta guardada (query_id) o inline (connection_id + query).
func (h *Handler) resolveExecuteTarget(r *http.Request, req executeReq) (connectors.Kind, json.RawMessage, []byte, json.RawMessage, error) {
	if req.QueryID != "" {
		return h.connStore.LoadQueryForExecute(r.Context(), req.QueryID)
	}
	if req.ConnectionID == "" || len(req.Query) == 0 {
		return "", nil, nil, nil, connectors.ErrInvalid
	}
	kind, config, sealed, err := h.connStore.LoadConnectionSecret(r.Context(), req.ConnectionID)
	if err != nil {
		return "", nil, nil, nil, err
	}
	return kind, config, sealed, req.Query, nil
}

// openCredentials descifra la credencial (o Credentials vacío si no hay).
func (h *Handler) openCredentials(sealed []byte) (connectors.Credentials, error) {
	if len(sealed) == 0 {
		return connectors.Credentials{}, nil
	}
	plain, err := h.sealer.Open(sealed)
	if err != nil {
		return connectors.Credentials{}, err
	}
	var c connectors.Credentials
	if err := json.Unmarshal(plain, &c); err != nil {
		return connectors.Credentials{}, connectors.ErrCiphertext
	}
	return c, nil
}
