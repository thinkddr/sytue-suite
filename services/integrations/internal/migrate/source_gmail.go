// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ConfigGmailBase apunta la API de Gmail a otra raíz (nubes soberanas y los
// servidores simulados de los tests).
const ConfigGmailBase = "gmail_base"

const (
	defaultGmailBase = "https://gmail.googleapis.com"
	//nolint:gosec // G101: identificador de scope OAuth público, no una credencial.
	scopeGmailRead = "https://www.googleapis.com/auth/gmail.readonly"
	// gmailPageSize es el tamaño de página del listado (tope de Gmail: 500).
	gmailPageSize = 100
)

// gmailLabelRoles traduce las etiquetas de sistema de Gmail al papel que tiene
// esa carpeta en cualquier buzón. Sin esto, el correo ENVIADO acabaría en la
// bandeja de entrada de destino, que es justo lo que arruina una mudanza.
var gmailLabelRoles = map[string]string{
	"INBOX":     "inbox",
	"SENT":      "sent",
	"DRAFT":     "drafts",
	"SPAM":      "junk",
	"TRASH":     "trash",
	"IMPORTANT": "",
	"STARRED":   "",
	"UNREAD":    "",
	"CHAT":      "",
}

// gmailIgnoredLabels son etiquetas que NO son carpetas: son estados (leído,
// destacado) o categorías de la bandeja que Gmail calcula solo. Crearlas en
// destino llenaría el buzón de carpetas vacías.
var gmailIgnoredLabels = map[string]bool{
	"UNREAD": true, "STARRED": true, "IMPORTANT": true, "CHAT": true,
	"CATEGORY_PERSONAL": true, "CATEGORY_SOCIAL": true, "CATEGORY_PROMOTIONS": true,
	"CATEGORY_UPDATES": true, "CATEGORY_FORUMS": true,
}

// gmail lee el buzón de la cuenta que autorizó la conexión.
//
// Va SIEMPRE sobre `/users/me`: con el consentimiento de una persona ese «me» es
// ella, y con delegación de dominio, la persona impersonada. El token lo inyecta
// la pasarela o lo mintea el driver, igual que en el resto del origen Google.
type gmail struct {
	*google
}

// ListFolders convierte las etiquetas de Gmail en carpetas.
func (g *gmail) ListFolders(ctx context.Context) ([]MailFolder, error) {
	base, err := g.gmailBase()
	if err != nil {
		return nil, err
	}
	//nolint:tagliatelle // los tags son el CONTRATO de la API de Gmail (camelCase).
	var resp struct {
		Labels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"labels"`
	}
	if err := getJSON(ctx, g.rt, base+"/gmail/v1/users/me/labels", g.token, &resp); err != nil {
		return nil, err
	}

	out := make([]MailFolder, 0, len(resp.Labels))
	for _, l := range resp.Labels {
		if gmailIgnoredLabels[l.ID] {
			continue // es un estado o una categoría calculada, no una carpeta.
		}
		f := MailFolder{ID: l.ID, Name: gmailFolderName(l.Name), Role: gmailLabelRoles[l.ID]}
		out = append(out, f)
	}
	return out, nil
}

// ListMessages devuelve una página de mensajes con sus metadatos (sin cuerpo).
func (g *gmail) ListMessages(ctx context.Context, cursor string) ([]MailMessage, string, error) {
	base, err := g.gmailBase()
	if err != nil {
		return nil, "", err
	}
	q := url.Values{}
	q.Set("maxResults", strconv.Itoa(gmailPageSize))
	// includeSpamTrash: el spam y la papelera del origen viajan a SUS carpetas.
	// Dejarlos fuera parece limpio hasta que alguien echa en falta un correo que
	// Gmail clasificó mal.
	q.Set("includeSpamTrash", "true")
	if cursor != "" {
		q.Set("pageToken", cursor)
	}
	//nolint:tagliatelle // contrato de la API de Gmail.
	var list struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := getJSON(ctx, g.rt, base+"/gmail/v1/users/me/messages?"+q.Encode(), g.token, &list); err != nil {
		return nil, "", err
	}

	out := make([]MailMessage, 0, len(list.Messages))
	for _, ref := range list.Messages {
		m, err := g.messageMeta(ctx, base, ref.ID)
		if err != nil {
			// Un mensaje que no se puede leer no tumba la página: se registra al
			// copiarlo, con su motivo.
			out = append(out, MailMessage{ID: ref.ID, Skip: "no se pudieron leer sus datos en el origen: " + err.Error()})
			continue
		}
		out = append(out, m)
	}
	return out, list.NextPageToken, nil
}

// messageMeta trae los metadatos de un mensaje: dónde estaba, cuándo llegó y en
// qué estado. El cuerpo se pide aparte, solo cuando toca copiarlo.
func (g *gmail) messageMeta(ctx context.Context, base, id string) (MailMessage, error) {
	q := url.Values{}
	q.Set("format", "metadata")
	q.Set("metadataHeaders", "Subject")
	//nolint:tagliatelle // contrato de la API de Gmail.
	var msg struct {
		ID           string   `json:"id"`
		LabelIDs     []string `json:"labelIds"`
		SizeEstimate int64    `json:"sizeEstimate"`
		InternalDate string   `json:"internalDate"`
		Payload      struct {
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
		} `json:"payload"`
	}
	u := base + "/gmail/v1/users/me/messages/" + url.PathEscape(id) + "?" + q.Encode()
	if err := getJSON(ctx, g.rt, u, g.token, &msg); err != nil {
		return MailMessage{}, err
	}

	m := MailMessage{ID: msg.ID, SizeBytes: msg.SizeEstimate}
	for _, h := range msg.Payload.Headers {
		if strings.EqualFold(h.Name, "Subject") {
			m.Subject = h.Value
			break
		}
	}
	// internalDate son milisegundos desde epoch: es la fecha REAL de recepción en
	// el buzón, que es la que ordena el correo migrado.
	if ms, err := strconv.ParseInt(msg.InternalDate, 10, 64); err == nil && ms > 0 {
		m.ReceivedAt = time.UnixMilli(ms).UTC()
	}
	m.FolderID = gmailPrimaryLabel(msg.LabelIDs)
	for _, l := range msg.LabelIDs {
		switch l {
		case "UNREAD":
			m.Seen = false
		case "STARRED":
			m.Flagged = true
		case "DRAFT":
			m.Draft = true
		}
	}
	// En Gmail, «leído» es la AUSENCIA de UNREAD.
	m.Seen = !containsLabel(msg.LabelIDs, "UNREAD")
	return m, nil
}

// OpenMessage devuelve el mensaje RFC 5322 tal cual, sin reescribir cabeceras:
// lo que se importa es el original, con su firma y sus trazas intactas.
func (g *gmail) OpenMessage(ctx context.Context, id string) ([]byte, error) {
	base, err := g.gmailBase()
	if err != nil {
		return nil, err
	}
	//nolint:tagliatelle // contrato de la API de Gmail.
	var msg struct {
		Raw string `json:"raw"`
	}
	u := base + "/gmail/v1/users/me/messages/" + url.PathEscape(id) + "?format=raw"
	if err := getJSON(ctx, g.rt, u, g.token, &msg); err != nil {
		return nil, err
	}
	if msg.Raw == "" {
		return nil, fmt.Errorf("%w: el origen no devolvió el contenido del mensaje", ErrUnimportable)
	}
	// Gmail codifica el mensaje en base64url (RFC 4648 §5).
	raw, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(strings.TrimRight(msg.Raw, "="))
	if err != nil {
		return nil, fmt.Errorf("%w: el mensaje de origen no se pudo decodificar: %s",
			ErrUnimportable, err.Error())
	}
	return raw, nil
}

// gmailBase resuelve la raíz de la API (configurable para tests y soberanas).
func (g *gmail) gmailBase() (string, error) {
	return baseURLOr(g.gmailAPIBase, defaultGmailBase)
}

// gmailPrimaryLabel elige la carpeta de un mensaje cuando el origen usa
// ETIQUETAS y el destino, carpetas. El orden no es arbitrario: un correo en la
// papelera está en la papelera aunque también esté etiquetado «Proyectos».
func gmailPrimaryLabel(labels []string) string {
	for _, priority := range []string{"TRASH", "SPAM", "DRAFT", "SENT"} {
		if containsLabel(labels, priority) {
			return priority
		}
	}
	// Una etiqueta propia describe mejor el correo que «INBOX».
	for _, l := range labels {
		if !gmailIgnoredLabels[l] && gmailLabelRoles[l] == "" && !strings.HasPrefix(l, "CATEGORY_") {
			return l
		}
	}
	if containsLabel(labels, "INBOX") {
		return "INBOX"
	}
	// Archivado: ni bandeja, ni papelera, ni etiqueta propia.
	return "ARCHIVE"
}

// gmailFolderName limpia el nombre de una etiqueta anidada («Trabajo/2024»).
func gmailFolderName(name string) string {
	switch name {
	case "INBOX":
		return "Inbox"
	case "SENT":
		return "Enviados"
	case "DRAFT":
		return "Borradores"
	case "SPAM":
		return "Spam"
	case "TRASH":
		return "Papelera"
	}
	if i := strings.LastIndex(name, "/"); i >= 0 && i+1 < len(name) {
		return name[i+1:]
	}
	return name
}

func containsLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

var _ MailSource = (*gmail)(nil)
