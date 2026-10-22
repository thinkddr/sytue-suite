// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Plano de PRESENTACIÓN (L4) del driver Power BI: mintea un artefacto de embed
// acotado (app-owns-data) para incrustar un INFORME en la suite (Sytue Docs), no
// datos (eso es executeQueries, powerbi.go). Implementa la capacidad opcional
// connector.EmbedBroker.
//
// Flujo app-owns-data, TODO por rt.HTTP (SSRF + rate-limit + auditoría + Bearer del
// service principal inyectado por el Runtime; el driver NUNCA ve el token):
//
//	(1) GET del informe → embedUrl canónica (no se hardcodea el host: las nubes
//	    soberanas difieren) y datasetId.
//	(2) POST GenerateToken del informe → token de embed de vida corta + caducidad.
//
// El token viaja en el CUERPO de la respuesta de GenerateToken; la auditoría de
// egress jamás persiste cuerpos (solo host/método/status/bytes/latencia y el
// Authorization como "REDACTED"), así que el artefacto no aparece en egress_audit
// por construcción. El BFF lo entrega al iframe por canal seguro (postMessage), nunca
// en query string ni en localStorage.
//
// ponytail: el handshake de app-owns-data PLENO (render fiable del token en el iframe)
// lo hace el SDK powerbi-client en el navegador; aquí se mintea el artefacto correcto
// server-side y el front hace el handshake mínimo por postMessage (ver reporte de la
// ola: powerbi-client queda como follow-up marcado, sin añadir la dependencia).
package powerbi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

// embedReq es la petición de embed (JSON opaco para el núcleo): el informe a
// incrustar. GroupID (workspace) es opcional —los informes de «Mi área de trabajo»
// no lo llevan—; AccessLevel por defecto "View" (L4 es presentación de solo lectura).
type embedReq struct {
	ReportID    string `json:"report_id"`
	GroupID     string `json:"group_id,omitempty"`
	AccessLevel string `json:"access_level,omitempty"`
}

func (q embedReq) accessLevel() string {
	if a := strings.TrimSpace(q.AccessLevel); a != "" {
		return a
	}
	return "View"
}

func (q embedReq) validate() error {
	if strings.TrimSpace(q.ReportID) == "" {
		return fmt.Errorf("%w: falta 'report_id' (el informe a incrustar)", connector.ErrBadQuery)
	}
	if strings.ContainsAny(q.ReportID, "/?#") {
		return fmt.Errorf("%w: report_id inválido", connector.ErrBadQuery)
	}
	if q.GroupID != "" && strings.ContainsAny(q.GroupID, "/?#") {
		return fmt.Errorf("%w: group_id inválido", connector.ErrBadQuery)
	}
	switch strings.TrimSpace(q.AccessLevel) {
	case "", "View", "Edit", "Create":
	default:
		return fmt.Errorf("%w: access_level inválido (View|Edit|Create)", connector.ErrBadQuery)
	}
	return nil
}

// reportMeta es el subconjunto del informe que necesitamos: la embedUrl canónica y
// el dataset (para un GenerateToken multi-recurso futuro; hoy basta el informe).
type reportMeta struct {
	EmbedURL  string `json:"embedUrl"`
	DatasetID string `json:"datasetId"`
}

// genTokenResp es la respuesta de GenerateToken: el artefacto de embed acotado.
type genTokenResp struct {
	Token      string    `json:"token"`
	TokenID    string    `json:"tokenId"`
	Expiration time.Time `json:"expiration"` // RFC3339
}

// EmbedToken mintea el artefacto de embed del informe (app-owns-data). req es embedReq.
// Egress por rt.HTTP. El artefacto se devuelve al llamante server-side (nunca al
// navegador directamente).
func (*Driver) EmbedToken(ctx context.Context, inst connector.Instance, rt connector.Runtime, req json.RawMessage) (connector.Embed, error) {
	base, err := apiBase(inst)
	if err != nil {
		return connector.Embed{}, err
	}
	var q embedReq
	if len(req) > 0 {
		if err := json.Unmarshal(req, &q); err != nil {
			return connector.Embed{}, fmt.Errorf("%w: la petición de embed no es JSON válido", connector.ErrBadQuery)
		}
	}
	if err := q.validate(); err != nil {
		return connector.Embed{}, err
	}
	path := reportPath(q.GroupID, q.ReportID)

	// (1) Metadatos del informe: embedUrl canónica + dataset.
	var meta reportMeta
	if err := do(ctx, rt, http.MethodGet, base+path, nil, &meta); err != nil {
		return connector.Embed{}, fmt.Errorf("powerbi: leer informe: %w", err)
	}
	if strings.TrimSpace(meta.EmbedURL) == "" {
		return connector.Embed{}, fmt.Errorf("powerbi: el informe no expone embedUrl")
	}

	// (2) Mint del token de embed acotado. El token va en el cuerpo (nunca auditado).
	var tok genTokenResp
	body := map[string]any{"accessLevel": q.accessLevel()}
	if err := do(ctx, rt, http.MethodPost, base+path+"/GenerateToken", body, &tok); err != nil {
		return connector.Embed{}, fmt.Errorf("powerbi: GenerateToken: %w", err)
	}
	if strings.TrimSpace(tok.Token) == "" {
		return connector.Embed{}, fmt.Errorf("powerbi: GenerateToken no devolvió token")
	}
	return connector.Embed{
		URL:       meta.EmbedURL,
		Token:     tok.Token,
		TokenType: "Embed", // app-owns-data
		ExpiresAt: tok.Expiration,
	}, nil
}

// reportPath construye la ruta del informe (con workspace si se dio group_id, o «Mi
// área de trabajo» si no). Cada segmento va PathEscape (frontera de confianza).
func reportPath(groupID, reportID string) string {
	if strings.TrimSpace(groupID) != "" {
		return "/v1.0/myorg/groups/" + url.PathEscape(strings.TrimSpace(groupID)) +
			"/reports/" + url.PathEscape(strings.TrimSpace(reportID))
	}
	return "/v1.0/myorg/reports/" + url.PathEscape(strings.TrimSpace(reportID))
}

// Compile-time: el driver satisface la capacidad opcional EmbedBroker.
var _ connector.EmbedBroker = (*Driver)(nil)
