// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package intents resuelve los intents cross-app de Sytue Hub: acciones que
// conectan dos productos de la suite y devuelven deep-links de cara al usuario
// (el shell los abre). Hub NO ejecuta la acción en el servicio destino; compone
// el enlace profundo a su compositor con los parámetros prerellenados, de modo
// que la autorización y la ejecución la sigue haciendo cada servicio.
//
// Intents soportados (CONVENTIONS §11):
//   - attach-drive-to-mail   : abrir borrador de Mail con ficheros de Drive.
//   - create-event-from-mail : abrir compositor de Calendar desde un correo.
//   - share-to-chat          : abrir Chat para compartir un recurso en un canal.
//   - call-channel           : abrir Sytue Rooms e iniciar la videollamada de un canal.
package intents

import (
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// Targets son las URLs públicas de las superficies destino de los deep-links.
type Targets struct {
	Drive    string
	Mail     string
	Calendar string
	Contacts string
	Chat     string
}

// Resolver compone deep-links para los intents a partir de las URLs públicas.
type Resolver struct {
	t Targets
}

// NewResolver construye el resolver normalizando las bases (sin barra final).
func NewResolver(t Targets) *Resolver {
	t.Drive = strings.TrimRight(t.Drive, "/")
	t.Mail = strings.TrimRight(t.Mail, "/")
	t.Calendar = strings.TrimRight(t.Calendar, "/")
	t.Contacts = strings.TrimRight(t.Contacts, "/")
	t.Chat = strings.TrimRight(t.Chat, "/")
	return &Resolver{t: t}
}

// Resolve despacha la petición al resolvedor del tipo de intent.
func (r *Resolver) Resolve(req domain.IntentRequest) (domain.IntentResult, error) {
	if !domain.ValidIntent(req.Type) {
		return domain.IntentResult{}, domain.ErrUnknownIntent
	}
	switch req.Type {
	case domain.IntentAttachDriveToMail:
		return r.attachDriveToMail(req)
	case domain.IntentCreateEventFromMail:
		return r.createEventFromMail(req)
	case domain.IntentShareToChat:
		return r.shareToChat(req)
	case domain.IntentCallChannel:
		return r.callChannel(req)
	default:
		return domain.IntentResult{}, domain.ErrUnknownIntent
	}
}

// attachDriveToMail compone un deep-link a un borrador de Mail con los ficheros
// de Drive adjuntos por referencia (ids), más un destinatario opcional.
func (r *Resolver) attachDriveToMail(req domain.IntentRequest) (domain.IntentResult, error) {
	fileIDs := nonEmpty(req.IDs)
	if len(fileIDs) == 0 {
		return domain.IntentResult{}, domain.ErrInvalidInput
	}
	q := url.Values{}
	q.Set("action", "compose")
	for _, id := range fileIDs {
		q.Add("attach_drive", id)
	}
	if to := req.Params["to"]; to != "" {
		q.Set("to", to)
	}
	if subj := req.Params["subject"]; subj != "" {
		q.Set("subject", subj)
	}

	// A la RAÍZ de Mail, que es la página que monta la bandeja y abre el redactor al ver
	// `action=compose`. Iba a `/compose`, que Mail no tiene: en producción era un 404
	// (medido el 2026-09-23) y «Adjuntar a un correo» de Drive no llegaba a ningún sitio.
	res := domain.IntentResult{
		Type:   domain.IntentAttachDriveToMail,
		Target: domain.SourceMail,
		URL:    r.t.Mail + "/?" + q.Encode(),
		Label:  "Adjuntar en un correo",
	}
	for _, id := range fileIDs {
		res.Links = append(res.Links, domain.DeepLink{
			Rel: "source", Source: domain.SourceDrive,
			URL: r.t.Drive + "/files/" + url.PathEscape(id),
		})
	}
	return res, nil
}

// createEventFromMail compone un deep-link al compositor de Calendar prerellenado
// desde un correo (asunto -> título; cuerpo -> descripción; enlace de vuelta).
func (r *Resolver) createEventFromMail(req domain.IntentRequest) (domain.IntentResult, error) {
	messageID := firstNonEmpty(req.Params["message_id"], first(req.IDs))
	if messageID == "" {
		return domain.IntentResult{}, domain.ErrInvalidInput
	}
	q := url.Values{}
	q.Set("action", "create")
	q.Set("from_mail", messageID)
	if title := req.Params["subject"]; title != "" {
		q.Set("title", title)
	}
	if desc := req.Params["body"]; desc != "" {
		q.Set("description", truncate(desc, 2000))
	}
	if when := req.Params["start"]; when != "" {
		q.Set("start", when)
	}
	// Invitados ← participantes del correo (emails separados por coma). Se pasan
	// tal cual al compositor de Calendar, que los sanea (valida cada email) antes
	// de prerellenar; aquí solo se acota su longitud para no desbordar la URL.
	if att := req.Params["attendees"]; att != "" {
		q.Set("attendees", truncate(att, 2000))
	}

	res := domain.IntentResult{
		Type:   domain.IntentCreateEventFromMail,
		Target: domain.SourceCalendar,
		URL:    r.t.Calendar + "/events/new?" + q.Encode(),
		Label:  "Crear evento",
		Links: []domain.DeepLink{{
			Rel: "source", Source: domain.SourceMail,
			URL: r.t.Mail + "/messages/" + url.PathEscape(messageID),
		}},
	}
	return res, nil
}

// shareToChat compone un deep-link a Chat con un recurso preparado para compartir.
// El recurso se identifica por su origen (source) e id; opcionalmente un canal.
func (r *Resolver) shareToChat(req domain.IntentRequest) (domain.IntentResult, error) {
	src := domain.Source(req.Params["source"])
	resourceID := firstNonEmpty(req.Params["id"], first(req.IDs))
	if resourceID == "" || !domain.ValidSource(src) || src == domain.SourceChat {
		return domain.IntentResult{}, domain.ErrInvalidInput
	}
	q := url.Values{}
	q.Set("action", "share")
	q.Set("share_source", string(src))
	q.Set("share_id", resourceID)
	if ch := req.Params["channel_id"]; ch != "" {
		q.Set("channel", ch)
	}
	if text := req.Params["text"]; text != "" {
		q.Set("text", truncate(text, 1000))
	}

	res := domain.IntentResult{
		Type:   domain.IntentShareToChat,
		Target: domain.SourceChat,
		URL:    r.t.Chat + "/share?" + q.Encode(),
		Label:  "Compartir en Chat",
	}
	if link := r.sourceLink(src, resourceID); link != "" {
		res.Links = append(res.Links, domain.DeepLink{Rel: "source", Source: src, URL: link})
	}
	return res, nil
}

// callChannel compone un deep-link a Sytue Rooms (servido en el host público de
// Chat) que abre la videollamada del canal indicado. Rooms lee el parámetro `call`
// al montar y arranca (o reutiliza) la sala del canal vía su BFF.
func (r *Resolver) callChannel(req domain.IntentRequest) (domain.IntentResult, error) {
	channelID := firstNonEmpty(req.Params["channel_id"], first(req.IDs))
	if channelID == "" {
		return domain.IntentResult{}, domain.ErrInvalidInput
	}
	q := url.Values{}
	q.Set("call", channelID)
	return domain.IntentResult{
		Type:   domain.IntentCallChannel,
		Target: domain.SourceChat,
		URL:    r.t.Chat + "/?" + q.Encode(),
		Label:  "Iniciar videollamada",
	}, nil
}

// sourceLink compone el deep-link al recurso en su servicio de origen.
func (r *Resolver) sourceLink(src domain.Source, id string) string {
	switch src {
	case domain.SourceDrive:
		return r.t.Drive + "/files/" + url.PathEscape(id)
	case domain.SourceMail:
		return r.t.Mail + "/messages/" + url.PathEscape(id)
	case domain.SourceCalendar:
		return r.t.Calendar + "/events/" + url.PathEscape(id)
	case domain.SourceContacts:
		return r.t.Contacts + "/contacts/" + url.PathEscape(id)
	default:
		return ""
	}
}

// --- helpers ---

func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func first(in []string) string {
	if len(in) > 0 {
		return strings.TrimSpace(in[0])
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
