// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"fmt"
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

// CanAccessChannel devuelve nil si el sujeto puede LEER el canal; domain.ErrNotFound
// si no puede (o el canal no existe: no se distingue, para no filtrar existencia).
// Cualquier otro fallo (red, 5xx, secreto inválido) es un error: el llamador NO
// sirve el adjunto.
func (c *ChatClient) CanAccessChannel(ctx context.Context, channelID uuid.UUID, subject string) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	q := url.Values{
		"tenant":  {t.Slug},
		"channel": {channelID.String()},
		"subject": {subject},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/internal/channel-access?"+q.Encode(), http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("chat: verja de canal: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusNotFound, http.StatusForbidden:
		return domain.ErrNotFound
	default:
		return fmt.Errorf("chat: verja de canal: respuesta %d", resp.StatusCode)
	}
}

// Verifica en compilación que el cliente satisface la verja del servicio.
var _ ChannelAuthorizer = (*ChatClient)(nil)
