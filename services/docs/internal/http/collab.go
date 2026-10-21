// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/coder/websocket"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/docs/internal/collab"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// wsOriginPatterns es la allowlist de hosts admitidos en el handshake WebSocket
// de colaboración. Lista cerrada de primera parte (platform/pkg/config), nunca
// un comodín de zona: bajo sytue.eu también viven los dominios donde corren las
// publicaciones de clientes de Cloud. Depende del entorno: en prod no entran los
// hosts `.localhost` de desarrollo.
func wsOriginPatterns(env pcfg.Environment) []string {
	return strings.Split(pcfg.FirstPartyWebSocketHosts(env), ",")
}

// getState devuelve el estado CRDT consolidado (carga inicial del editor).
func (h *Handler) getState(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	state, seq, err := h.svc.LoadFull(r.Context(), subj, docID(r), h.hydrateFromS3)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"seq":   seq,
		"state": state, // estado CRDT serializable (elementos + registros)
	})
}

// postOpsReq es un lote de operaciones a aplicar vía REST (alternativa a WS).
type postOpsReq struct {
	Ops []domain.Op `json:"ops"`
	// BaseSeq hace el guardado condicional (ver domain.Service.ApplyOps); ausente,
	// LWW incondicional. Lo manda la web para hojas y presentaciones.
	BaseSeq *int64 `json:"base_seq,omitempty"`
	// Proteccion: contraseñas de hoja/libro, fuera de las operaciones para que no
	// entren jamás en el registro (domain.Proteccion).
	Proteccion *domain.Proteccion `json:"proteccion,omitempty"`
}

// postOps secuencia y persiste operaciones por REST, difundiéndolas a la sala WS.
func (h *Handler) postOps(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req postOpsReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(req.Ops) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("There are no operations to apply."))
		return
	}
	seqd, newSeq, err := h.svc.ApplyOps(domain.ConProteccion(r.Context(), req.Proteccion), subj, docID(r), req.Ops, req.BaseSeq)
	if err != nil {
		fail(w, r, err)
		return
	}
	// Difundir a los colaboradores conectados por WebSocket.
	h.hub.PublishOps(r.Context(), docID(r), seqd, nil)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"seq": newSeq, "ops": seqd})
}

// getOps devuelve operaciones posteriores a ?since (replay para reconexión).
func (h *Handler) getOps(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer); err != nil {
		fail(w, r, err)
		return
	}
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ops, err := h.store.OpsSince(r.Context(), docID(r), since, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ops": domain.RedactarOps(ops)})
}

// maxConnLen acota el identificador de conexión del cliente (UUID = 36 chars).
const maxConnLen = 64

// presenceReq es el heartbeat de presencia del cliente.
type presenceReq struct {
	// Conn es el actor CRDT de la pestaña; identifica la conexión (dos pestañas
	// del mismo usuario = dos conexiones).
	Conn string `json:"conn"`
}

// putPresence registra/renueva la presencia del solicitante en el documento y
// devuelve los miembros presentes, INCLUIDO el lector pasivo (abrir el
// documento = heartbeat = presente). Es el canal de presencia utilizable desde
// el navegador; el WS no lo es (401 sin la cabecera Accept que la API WS no
// puede fijar). Requiere al menos rol viewer.
func (h *Handler) putPresence(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer); err != nil {
		fail(w, r, err)
		return
	}
	var req presenceReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Conn == "" || len(req.Conn) > maxConnLen {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid connection id."))
		return
	}
	members := h.presence.Touch(docID(r), req.Conn, subj.Sub)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"members": members})
}

// collabWS actualiza la conexión a WebSocket y entra en el bucle de colaboración.
// El cliente debe autenticarse (RequireAuth) y tener al menos rol viewer; el rol
// efectivo determina si puede emitir operaciones.
func (h *Handler) collabWS(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	doc, role, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer)
	if err != nil {
		fail(w, r, err)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Allowlist CERRADA de orígenes de primera parte (la misma que usa
		// Rooms). Un comodín sobre la zona casaría también *.apps.sytue.eu y
		// *.preview.sytue.eu, donde se ejecutan publicaciones de clientes de
		// Cloud: código de terceros que no debe poder abrir este WS.
		// Más el host de la propia petición —el de la organización—: ver httpx.WSOriginPatterns.
		OriginPatterns: httpx.WSOriginPatterns(r, wsOriginPatterns(h.env)),
	})
	if err != nil {
		log.From(r.Context()).Debug("collab: no se pudo aceptar WS", "error", err)
		return
	}

	// Contexto desligado del request HTTP para que la conexión sobreviva… pero no a
	// su token: al caducar se cierra, y al reconectar el token nuevo dice lo que vale
	// AHORA —quien salió de la organización ya no trae su `tenant`— (ADR 0052).
	ctx := context.WithoutCancel(r.Context())
	if c, ok := authnClaims(r); ok && !c.Expira.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, c.Expira)
		defer cancel()
	}
	client := collab.NewClient(h.hub, conn, doc.ID, subj, role, subjName(r))
	client.Serve(ctx, h, doc.Seq)
}

// subjName devuelve el nombre legible del usuario (para presencia).
func subjName(r *http.Request) string {
	if claims, ok := authnClaims(r); ok {
		if claims.Name != "" {
			return claims.Name
		}
		return claims.Email
	}
	return ""
}

// --- collab.Handler implementado por *Handler (desacople de transporte) ---

// ApplyOps satisface collab.Handler (autoriza + secuencia + persiste).
func (h *Handler) ApplyOps(ctx context.Context, subj domain.Subject, docID string, ops []domain.Op) ([]domain.SequencedOp, int64, error) {
	return h.svc.ApplyOps(ctx, subj, docID, ops, nil)
}

// OpsSince satisface collab.Handler (replay).
func (h *Handler) OpsSince(ctx context.Context, docID string, since int64, limit int) ([]domain.SequencedOp, error) {
	ops, err := h.store.OpsSince(ctx, docID, since, limit)
	return domain.RedactarOps(ops), err // el historial heredado puede llevar contraseñas en claro
}

// Rol satisface collab.Handler: el rol vigente, para la reautorización periódica.
func (h *Handler) Rol(ctx context.Context, subj domain.Subject, docID string) (domain.Role, error) {
	_, rol, err := h.svc.Authorize(ctx, docID, subj, domain.RoleViewer)
	return rol, err
}

// compile-time: *Handler implementa collab.Handler.
var _ collab.Handler = (*Handler)(nil)
