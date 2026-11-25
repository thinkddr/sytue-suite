// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// ChatClient resuelve la MEMBRESÍA de un canal de Sytue Rooms contra services/rooms
// (GET /internal/channel-access), que es la única fuente de verdad del roster. Se
// consulta en CADA petición de un adjunto de canal: así salir del canal revoca el
// acceso al instante, sin ACL replicada en Drive que pueda quedarse obsoleta.
//
// Autenticación service-to-service: el MISMO secreto compartido que usa meet→chat
// (CHAT_INTERNAL_SECRET), en Authorization: Bearer. No viaja el JWT del usuario: el
// endpoint interno de Chat no lo pide y así Drive no depende de la audiencia del
// token. El tenant y el sujeto van en la query (Chat fija su RLS con el tenant).
//
// Clean-room: cliente HTTP propio con la stdlib, sin dependencias nuevas (mismo
// patrón que el DriveClient de services/rooms, en sentido inverso).
type ChatClient struct {
	baseURL string
	secret  string
	http    *http.Client
}

// NewChatClient construye el cliente. Devuelve nil si falta la URL o el secreto: sin
// verja configurada, la superficie de adjuntos de canal queda CERRADA (fail-closed),
// nunca abierta.
func NewChatClient(baseURL, secret string) *ChatClient {
	if strings.TrimSpace(baseURL) == "" || secret == "" {
		return nil
	}
	return &ChatClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		secret:  secret,
		// Camino caliente (cada descarga/miniatura): timeout corto y fail-closed.
		// ponytail: sin caché de resultados; añadir un TTL de pocos segundos por
		// (canal, sujeto) si el probe aparece en los perfiles de latencia.
		http: &http.Client{Timeout: 3 * time.Second},
	}
}

// ChannelTenant devuelve la organización donde vive el canal si el sujeto puede
// LEERLO: la del contexto, o la ANFITRIONA cuando está en él como persona de otra
// organización (ADR 0040), que es donde viven sus mensajes y, por tanto, sus
// adjuntos. Rooms contesta 204 en el primer caso y 200 con {"tenant": …} en el
// segundo. domain.ErrNotFound si no puede (o el canal no existe: no se distingue,
// para no filtrar existencia). Cualquier otro fallo (red, 5xx, secreto inválido,
// una respuesta que no se entiende) es un error: el llamador NO sirve el adjunto.
func (c *ChatClient) ChannelTenant(ctx context.Context, channelID uuid.UUID, subject string) (tenancy.Tenant, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return tenancy.Tenant{}, err
	}
	q := url.Values{
		"tenant":  {t.Slug},
		"channel": {channelID.String()},
		"subject": {subject},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/internal/channel-access?"+q.Encode(), http.NoBody)
	if err != nil {
		return tenancy.Tenant{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return tenancy.Tenant{}, fmt.Errorf("chat: verja de canal: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return t, nil
	case http.StatusOK:
		var cuerpo struct {
			Tenant string `json:"tenant"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&cuerpo); err != nil {
			return tenancy.Tenant{}, fmt.Errorf("chat: verja de canal: respuesta ilegible: %w", err)
		}
		anfitriona, err := tenancy.New(cuerpo.Tenant)
		if err != nil {
			return tenancy.Tenant{}, fmt.Errorf("chat: verja de canal: %w", err)
		}
		return anfitriona, nil
	case http.StatusNotFound, http.StatusForbidden:
		return tenancy.Tenant{}, domain.ErrNotFound
	default:
		return tenancy.Tenant{}, fmt.Errorf("chat: verja de canal: respuesta %d", resp.StatusCode)
	}
}

// Verifica en compilación que el cliente satisface la verja del servicio.
var _ ChannelAuthorizer = (*ChatClient)(nil)
