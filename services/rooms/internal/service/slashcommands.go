// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/rooms/internal/compat"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// EJECUCIÓN de un slash command de terceros.
//
// El POST sale con el MISMO cuerpo y la MISMA firma que Slack, así que un comando ya
// escrito para Slack funciona pegando su URL. Va por el cliente SSRF-safe de los
// salientes: la URL la pone un admin, pero un admin también puede equivocarse (o ser
// engañado) y apuntarla a la red interna.
//
// El resultado es SÍNCRONO y acotado a unos segundos. Un comando que tarde más debe
// contestar «trabajando…» y usar `response_url`, igual que en Slack.

// slashTimeout es lo que se espera al comando. Alguien está mirando el compositor: más
// de unos segundos y parece que la interfaz se ha colgado.
const slashTimeout = 3 * time.Second

// SlashResult es lo que se le devuelve a quien ejecutó el comando.
type SlashResult struct {
	// Text es la respuesta ya normalizada a Markdown de Rooms. Vacío = el comando
	// aceptó el trabajo y contestará luego por `response_url`.
	Text string `json:"text"`
	// ResponseType dice si el texto se publicó en el canal o solo lo ve quien ejecutó.
	ResponseType string `json:"response_type"`
	// MessageID es el mensaje publicado, cuando ResponseType es in_channel.
	MessageID uuid.UUID `json:"message_id,omitempty"`
}

// ErrSlashFailed indica que el comando no contestó o contestó mal. Se distingue de «no
// existe» para poder decirle a la persona cuál de las dos cosas pasó.
var ErrSlashFailed = errors.New("service: el comando no respondió")

// CreateSlashCommand registra un comando de terceros (admin del workspace).
//
// Genera los dos secretos: el de verificación, que viaja en cada ejecución, y el de
// firma, que no sale nunca. Son distintos por lo mismo que en los salientes: quien vea
// una ejecución no debe poder fabricar la siguiente.
func (s *Service) CreateSlashCommand(ctx context.Context, name, description, usage, requestURL, actor string) (domain.SlashCommand, error) {
	n := domain.NormalizeSlashName(name)
	if n == "" || !domain.ValidCallbackURL(requestURL) {
		return domain.SlashCommand{}, domain.ErrInvalidInput
	}
	token, _, err := domain.NewOpaqueToken()
	if err != nil {
		return domain.SlashCommand{}, err
	}
	secret, err := domain.NewSigningSecret()
	if err != nil {
		return domain.SlashCommand{}, err
	}
	return s.pstore.CreateSlashCommand(ctx, domain.SlashCommand{
		Name:          n,
		Description:   strings.TrimSpace(description),
		UsageHint:     strings.TrimSpace(usage),
		RequestURL:    strings.TrimSpace(requestURL),
		Token:         token,
		SigningSecret: secret,
	}, actor)
}

// ListSlashCommands devuelve los comandos de la organización.
//
// Los secretos solo viajan si quien pregunta es admin: el autocompletado del compositor
// los pide para TODOS, y mandarle a cada persona el secreto con el que se firman las
// ejecuciones sería repartir la llave por gusto.
func (s *Service) ListSlashCommands(ctx context.Context, actor string) ([]domain.SlashCommand, error) {
	out, err := s.pstore.ListSlashCommands(ctx)
	if err != nil {
		return nil, err
	}
	admin, err := s.pstore.IsWorkspaceAdmin(ctx, actor)
	if err != nil {
		return nil, err
	}
	if admin {
		return out, nil
	}
	for i := range out {
		out[i].Token = ""
		out[i].SigningSecret = ""
	}
	return out, nil
}

// DeleteSlashCommand borra un comando (admin del workspace).
func (s *Service) DeleteSlashCommand(ctx context.Context, id uuid.UUID, actor string) error {
	return s.pstore.DeleteSlashCommand(ctx, id, actor)
}

// ExecuteSlashCommand resuelve el comando, lo invoca y publica o devuelve su respuesta.
func (s *Service) ExecuteSlashCommand(ctx context.Context, channelID uuid.UUID, actor, actorName, input string) (SlashResult, error) {
	if s.cfg.MatrixRequired {
		return SlashResult{}, domain.ErrEncryptedRequired
	}
	name, args := domain.SplitSlashInput(input)
	if name == "" {
		return SlashResult{}, domain.ErrInvalidInput
	}
	cmd, err := s.pstore.SlashCommandByName(ctx, name)
	if err != nil {
		return SlashResult{}, err // ErrNotFound ⇒ el handler dice «no existe ese comando»
	}
	// Que quien ejecuta pertenezca al canal: si no, un comando podría usarse para
	// publicar en canales donde no se está.
	if _, err := s.pstore.GetChannel(ctx, channelID, actor); err != nil {
		return SlashResult{}, err
	}

	texto, tipo, err := s.postSlash(ctx, cmd, channelID, actor, actorName, args)
	if err != nil {
		return SlashResult{}, err
	}
	texto = strings.TrimSpace(texto)
	if texto == "" {
		// El comando aceptó y contestará por su cuenta. Devolver vacío es honesto:
		// inventar un «hecho» diría que terminó algo que quizá acaba de empezar.
		return SlashResult{ResponseType: tipo}, nil
	}
	if tipo != domain.SlashResponseInChannel {
		return SlashResult{Text: texto, ResponseType: tipo}, nil
	}

	body, err := domain.NormalizeBody(texto, s.cfg.MaxMessageBytes)
	if err != nil {
		return SlashResult{}, err
	}
	// Se publica como el comando, no como quien lo ejecutó: la respuesta la escribe el
	// bot, y firmarla con el nombre de la persona sería atribuirle algo que no dijo.
	// Reusa el autor sintético de los webhooks para que lleve el badge BOT sin añadir
	// un tercer prefijo que habría que enseñar a reconocer en cada sitio.
	m, err := s.pstore.CreateHookMessage(ctx, channelID, domain.HookAuthorSub(cmd.ID), "/"+cmd.Name, body)
	if err != nil {
		return SlashResult{}, err
	}
	s.publish(ctx, EventMessageCreated, m.ChannelID, m)
	return SlashResult{Text: texto, ResponseType: tipo, MessageID: m.ID}, nil
}

// postSlash valida el destino (barrera SSRF 1) y hace la llamada.
//
// Separado de doSlash igual que postOutgoing lo está de doOutgoing: la validación
// resuelve DNS y rechaza loopback, así que aislarla es lo que permite probar el resto
// contra un servidor de pruebas local sin bajar la barrera en producción.
func (s *Service) postSlash(ctx context.Context, cmd domain.SlashCommand, channelID uuid.UUID, actor, actorName, args string) (string, string, error) {
	if err := validateOutgoingURL(ctx, cmd.RequestURL); err != nil {
		return "", "", err
	}
	return s.doSlash(ctx, cmd, channelID, actor, actorName, args)
}

// doSlash compone el cuerpo, firma y lee la respuesta.
func (s *Service) doSlash(ctx context.Context, cmd domain.SlashCommand, channelID uuid.UUID, actor, actorName, args string) (string, string, error) {
	// Cuerpo EXACTO de un slash command de Slack: es lo que ya sabe leer un comando
	// escrito para él.
	form := url.Values{}
	form.Set("token", cmd.Token)
	form.Set("team_id", cmd.Tenant)
	form.Set("team_domain", cmd.Tenant)
	form.Set("channel_id", channelID.String())
	form.Set("channel_name", channelID.String())
	form.Set("user_id", actor)
	form.Set("user_name", actorName)
	form.Set("command", "/"+cmd.Name)
	form.Set("text", args)
	payload := []byte(form.Encode())

	ctx, cancel := context.WithTimeout(ctx, slashTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cmd.RequestURL, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Rooms-Command/1")
	if cmd.SigningSecret != "" {
		ts := time.Now().Unix()
		req.Header.Set(compat.SlackTimestampHeader, strconv.FormatInt(ts, 10))
		req.Header.Set(compat.SlackSignatureHeader, compat.SignSlack(cmd.SigningSecret, ts, payload))
	}

	client := s.outgoing
	if client == nil {
		client = ssrfSafeClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", errors.Join(ErrSlashFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, outgoingMaxRespBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", ErrSlashFailed
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		// 200 sin cuerpo: aceptado, contestará por su cuenta. Es válido en Slack.
		return "", domain.SlashResponseEphemeral, nil
	}
	// Misma respuesta que un webhook de Slack: {text, response_type, blocks…}.
	return compat.ParseSlackReply(raw), domain.NormalizeSlashResponseType(compat.SlackResponseType(raw)), nil
}
