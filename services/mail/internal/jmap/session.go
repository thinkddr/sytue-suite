// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// Capabilities JMAP soportadas (URNs RFC 8620/8621).
const (
	capCore = "urn:ietf:params:jmap:core"
	capMail = "urn:ietf:params:jmap:mail"
)

// sessionResource es el recurso de sesión JMAP (RFC 8620 §2).
type sessionResource struct {
	Capabilities    map[string]any         `json:"capabilities"`
	Accounts        map[string]accountInfo `json:"accounts"`
	PrimaryAccounts map[string]string      `json:"primaryAccounts"`
	Username        string                 `json:"username"`
	APIURL          string                 `json:"apiUrl"`
	DownloadURL     string                 `json:"downloadUrl"`
	UploadURL       string                 `json:"uploadUrl"`
	State           string                 `json:"state"`
}

type accountInfo struct {
	Name                string         `json:"name"`
	IsPersonal          bool           `json:"isPersonal"`
	IsReadOnly          bool           `json:"isReadOnly"`
	AccountCapabilities map[string]any `json:"accountCapabilities"`
}

// handleSession devuelve el recurso de sesión para el usuario autenticado.
func (a *API) handleSession(w http.ResponseWriter, r *http.Request) {
	// Asegurar/cargar la cuenta del usuario (provisiona buzones de sistema). Por
	// `account` y no por `EnsureAccount` a secas: es el mismo camino que recorren
	// los demás handlers, y con él la dirección principal sigue a la que Sytue ID
	// asigna en vez de congelarse en la del alta. Esta ruta es además la que abre
	// el reconciliador de buzones, así que es por donde entra una mudanza de
	// dominio sin que la persona tenga que asomarse al webmail.
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	accID := acc.ID.String()
	res := sessionResource{
		Capabilities: map[string]any{
			capCore: map[string]any{
				"maxSizeUpload":       a.cfg.MaxMessageBytes,
				"maxConcurrentUpload": 4,
				"maxSizeRequest":      10 * 1024 * 1024,
				"maxObjectsInGet":     500,
				"maxObjectsInSet":     500,
				"collationAlgorithms": []string{"i;ascii-casemap"},
			},
			capMail: map[string]any{
				"maxMailboxesPerEmail":       nil,
				"maxSizeAttachmentsPerEmail": a.cfg.MaxMessageBytes,
				"emailQuerySortOptions":      []string{"receivedAt", "subject", "from"},
			},
		},
		Accounts: map[string]accountInfo{
			accID: {
				Name:       acc.Address,
				IsPersonal: true,
				IsReadOnly: false,
				AccountCapabilities: map[string]any{
					capCore: map[string]any{},
					capMail: map[string]any{},
				},
			},
		},
		PrimaryAccounts: map[string]string{capCore: accID, capMail: accID},
		Username:        acc.Address,
		APIURL:          a.cfg.PublicURL + "/jmap",
		DownloadURL:     a.cfg.PublicURL + "/jmap/download/{accountId}/{blobId}/{name}",
		UploadURL:       a.cfg.PublicURL + "/jmap/upload/{accountId}",
		State:           "0",
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

// jmapRequest es el sobre de petición JMAP (RFC 8620 §3.3).
type jmapRequest struct {
	Using       []string          `json:"using"`
	MethodCalls [][3]any          `json:"methodCalls"`
	CreatedIDs  map[string]string `json:"createdIds,omitempty"`
}

// jmapResponse es el sobre de respuesta JMAP.
type jmapResponse struct {
	MethodResponses [][3]any          `json:"methodResponses"`
	SessionState    string            `json:"sessionState"`
	CreatedIDs      map[string]string `json:"createdIds,omitempty"`
}

// handleJMAP procesa el sobre por lotes. Esta implementación enruta los métodos
// soportados (Mailbox/get, Email/query, Email/get, Email/set, Email/send) a la
// capa REST/store subyacente; métodos no soportados devuelven un error JMAP.
func (a *API) handleJMAP(w http.ResponseWriter, r *http.Request) {
	var req jmapRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp := jmapResponse{SessionState: "0", MethodResponses: make([][3]any, 0, len(req.MethodCalls))}
	for _, call := range req.MethodCalls {
		name, _ := call[0].(string)
		callID := call[2]
		result, err := a.dispatch(r, name, call[1])
		if err != nil {
			resp.MethodResponses = append(resp.MethodResponses, [3]any{
				"error", map[string]any{"type": "serverFail", "description": err.Error()}, callID,
			})
			continue
		}
		resp.MethodResponses = append(resp.MethodResponses, [3]any{name, result, callID})
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// dispatch enruta un método JMAP individual a su implementación. Cubre los
// métodos que usa el cliente web de Sytue Mail —Mailbox/get, Mailbox/set,
// Email/get, Email/query y Email/set— más Email/import, que es por donde entra
// un buzón que se muda desde otro proveedor.
func (a *API) dispatch(r *http.Request, method string, args any) (any, error) {
	claims, _ := authn.FromContext(r.Context())
	acc, err := a.store.AccountBySubject(r.Context(), claims.Subject)
	if err != nil {
		return nil, err
	}
	raw := toRawArgs(args)
	switch method {
	case "Mailbox/get":
		return a.mailboxGet(r, acc)
	case "Mailbox/set":
		return a.mailboxSet(r, acc, raw)
	case "Email/get":
		return a.emailGet(r, acc, raw)
	case "Email/query":
		return a.emailQuery(r, acc, raw)
	case "Email/set":
		return a.emailSet(r, acc, raw)
	case "Email/import":
		return a.emailImport(r, acc, raw)
	default:
		return nil, errUnsupportedMethod(method)
	}
}
