// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

var (
	MailProviderNames = map[string]string{"gmail": "Gmail", "microsoft": "Microsoft Outlook", "yahoo": "Yahoo Mail", "imap": "IMAP", "pop3": "POP3"}
	mailConsentHosts  = map[string]string{"gmail": "accounts.google.com", "microsoft": "login.microsoftonline.com", "yahoo": "api.login.yahoo.com"}
)

type ExternalMailServer struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Security string `json:"security"`
}
type ExternalMailConfiguration struct {
	Provider     string             `json:"provider"`
	Name         string             `json:"name"`
	Address      string             `json:"address"`
	Incoming     ExternalMailServer `json:"incoming"`
	Outgoing     ExternalMailServer `json:"outgoing"`
	Password     string             `json:"password"`
	SMTPPassword string             `json:"smtp_password,omitempty"`
	InstanceID   string             `json:"instance_id,omitempty"`
}
type ExternalMailCapabilities struct {
	Read           bool `json:"read"`
	Send           bool `json:"send"`
	Actions        bool `json:"actions"`
	Attachments    bool `json:"attachments"`
	ContinuousSync bool `json:"continuous_sync"`
}
type ExternalMailAccount struct {
	Services      []string `json:"services"`
	Configuration *struct {
		Address  string             `json:"address"`
		Incoming ExternalMailServer `json:"incoming"`
		Outgoing ExternalMailServer `json:"outgoing"`
	} `json:"configuration,omitempty"`
	InstanceID   string                   `json:"instance_id"`
	Provider     string                   `json:"provider"`
	Name         string                   `json:"name"`
	Address      string                   `json:"address"`
	Connected    bool                     `json:"connected"`
	Capabilities ExternalMailCapabilities `json:"capabilities"`
}

// ICloudMail es iCloud Mail, que no ofrece inicio de sesión a otras apps: es IMAP
// y SMTP con una contraseña de app. Sus servidores son fijos, así que Mail pide
// solo la dirección y esa contraseña y guarda una conexión «imap» con estos. Es
// el mismo preajuste que ICLOUD_MAIL en @sytue/api-clients.
var ICloudMail = struct {
	Name               string
	Incoming, Outgoing ExternalMailServer
	// AccountPage es donde se crea la contraseña de app: «Inicio de sesión y
	// seguridad» de la cuenta de Apple.
	AccountPage string
}{
	Name:        "iCloud Mail",
	Incoming:    ExternalMailServer{Host: "imap.mail.me.com", Port: 993, Security: "tls"},
	Outgoing:    ExternalMailServer{Host: "smtp.mail.me.com", Port: 587, Security: "starttls"},
	AccountPage: "https://account.apple.com/",
}

// IsICloud dice si una conexión de la lista es un buzón de iCloud guardado con
// ese preajuste.
func (a ExternalMailAccount) IsICloud() bool {
	return a.Provider == "imap" && a.Configuration != nil && strings.EqualFold(a.Configuration.Incoming.Host, ICloudMail.Incoming.Host)
}

// ProviderName es el nombre del proveedor de una conexión de la lista: «iCloud
// Mail» para el preajuste (mailProviderName en @sytue/api-clients).
func (a ExternalMailAccount) ProviderName() string {
	if a.IsICloud() {
		return ICloudMail.Name
	}
	return MailProviderNames[a.Provider]
}

// ServesMail says whether Mail opens this account: one connected only for
// Calendar, Contacts or Notes is listed too, but has no mailbox. Without
// services (accounts from before linked services) it is a mailbox.
func (a ExternalMailAccount) ServesMail() bool {
	return a.Services == nil || slices.Contains(a.Services, "mail")
}

type ExternalMailProvider struct {
	Services      []LinkedServiceCapability `json:"services"`
	Provider      string                    `json:"provider"`
	Available     bool                      `json:"available"`
	Authorization string                    `json:"authorization"`
}
type ExternalMailAccounts struct {
	Items     []ExternalMailAccount  `json:"items"`
	Providers []ExternalMailProvider `json:"providers"`
}
type ExternalMailFolder struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Total  int64  `json:"total"`
	Unread int64  `json:"unread"`
}

// Estados de una cuenta en el resumen: «reconnect» pide volver a iniciar sesión;
// «unavailable» es del proveedor (no contestó a tiempo o falló).
const (
	ExternalMailOK          = "ok"
	ExternalMailReconnect   = "reconnect"
	ExternalMailUnavailable = "unavailable"
)

// ExternalMailAccountSummary es una cuenta como la da el resumen: con sus
// carpetas, vacías salvo en «ok» (ExternalMailAccountSummary en @sytue/api-clients).
type ExternalMailAccountSummary struct {
	ExternalMailAccount
	State   string               `json:"state"`
	Folders []ExternalMailFolder `json:"folders"`
}

type ExternalMailAddress struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}
type ExternalMailAttachment struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Size          int64  `json:"size,omitempty"`
	ContentBase64 string `json:"content_base64,omitempty"`
}
type ExternalMailMessage struct {
	ID          string                   `json:"id"`
	FolderID    string                   `json:"folder_id"`
	Subject     string                   `json:"subject"`
	From        []ExternalMailAddress    `json:"from"`
	To          []ExternalMailAddress    `json:"to"`
	Cc          []ExternalMailAddress    `json:"cc,omitempty"`
	Date        string                   `json:"date"`
	Snippet     string                   `json:"snippet"`
	Text        string                   `json:"text"`
	HTML        string                   `json:"html"`
	Seen        bool                     `json:"seen"`
	Flagged     bool                     `json:"flagged"`
	Attachments []ExternalMailAttachment `json:"attachments"`
}
type ExternalMailPage struct {
	Items      []ExternalMailMessage `json:"items"`
	NextCursor string                `json:"next_cursor"`
}
type ExternalMailOutgoing struct {
	ExpectedAddress string                   `json:"expected_address"`
	To              []ExternalMailAddress    `json:"to"`
	Cc              []ExternalMailAddress    `json:"cc,omitempty"`
	Bcc             []ExternalMailAddress    `json:"bcc,omitempty"`
	Subject         string                   `json:"subject"`
	Text            string                   `json:"text"`
	HTML            string                   `json:"html,omitempty"`
	Attachments     []ExternalMailAttachment `json:"attachments,omitempty"`
	InReplyTo       string                   `json:"in_reply_to,omitempty"`
}
type ExternalMailSent struct {
	ID            string `json:"id,omitempty"`
	SentCopySaved *bool  `json:"sent_copy_saved,omitempty"`
}

func (i *Integraciones) MailAccounts(ctx context.Context) (ExternalMailAccounts, error) {
	return JSON[ExternalMailAccounts](ctx, i.c, Request{Path: "/external-mail/accounts"})
}

// MailSummary son los buzones que pinta el lateral de Mail, cada uno con sus
// carpetas y no leídos, en una petición: solo las cuentas con buzón, y una que
// falla no tumba las demás.
func (i *Integraciones) MailSummary(ctx context.Context) ([]ExternalMailAccountSummary, error) {
	result, err := JSON[struct {
		Items []ExternalMailAccountSummary `json:"items"`
	}](ctx, i.c, Request{Path: "/external-mail/summary"})
	return result.Items, err
}

func (i *Integraciones) MailConnect(ctx context.Context, provider, instance string, services ...string) (OAuthAuthorization, error) {
	return i.MailConnectDesde(ctx, provider, instance, "native-mail", services...)
}

func (i *Integraciones) MailConnectDesde(ctx context.Context, provider, instance, section string, services ...string) (OAuthAuthorization, error) {
	switch NativeConsentProduct(section) {
	case "mail", "calendar", "contacts", "notes":
	default:
		return OAuthAuthorization{}, ErrOAuthState
	}
	host := mailConsentHosts[provider]
	if host == "" {
		return OAuthAuthorization{}, ErrOAuthState
	}
	result, err := PostJSON[struct {
		InstanceID   string `json:"instance_id"`
		AuthorizeURL string `json:"authorize_url"`
	}](ctx, i.c, "/external-mail/connect", map[string]any{"provider": provider, "instance_id": instance, "return_to": section, "services": services})
	if err != nil {
		return OAuthAuthorization{}, err
	}
	return autorizacionPersonal(result.AuthorizeURL, result.InstanceID, host, section)
}

func (i *Integraciones) MailConfigure(ctx context.Context, configuration ExternalMailConfiguration) (string, error) {
	result, err := PostJSON[struct {
		InstanceID string `json:"instance_id"`
	}](ctx, i.c, "/external-mail/accounts", configuration)
	return result.InstanceID, err
}

func (i *Integraciones) MailFolders(ctx context.Context, instance, expectedAddress string) ([]ExternalMailFolder, error) {
	result, err := JSON[struct {
		Items []ExternalMailFolder `json:"items"`
	}](ctx, i.c, Request{Path: ruta("/external-mail/", instance, "/folders"), Query: url.Values{"expected_address": {expectedAddress}}})
	return result.Items, err
}

func (i *Integraciones) MailMessages(ctx context.Context, instance, folder, cursor, expectedAddress string) (ExternalMailPage, error) {
	return JSON[ExternalMailPage](ctx, i.c, Request{Path: ruta("/external-mail/", instance, "/messages"), Query: url.Values{"folder": {folder}, "cursor": {cursor}, "expected_address": {expectedAddress}}})
}

func (i *Integraciones) MailMessage(ctx context.Context, instance, folder, id, expectedAddress string) (ExternalMailMessage, error) {
	return JSON[ExternalMailMessage](ctx, i.c, Request{Path: ruta("/external-mail/", instance, "/message"), Query: url.Values{"folder": {folder}, "id": {id}, "expected_address": {expectedAddress}}})
}

func (i *Integraciones) MailSend(ctx context.Context, instance string, outgoing ExternalMailOutgoing) (ExternalMailSent, error) {
	return PostJSON[ExternalMailSent](ctx, i.c, ruta("/external-mail/", instance, "/send"), outgoing)
}

func (i *Integraciones) MailAction(ctx context.Context, instance, folder, id, action, destination, expectedAddress string) error {
	_, err := PostJSON[struct{}](ctx, i.c, ruta("/external-mail/", instance, "/actions"), map[string]string{"expected_address": expectedAddress, "folder_id": folder, "message_id": id, "action": action, "destination": destination})
	return err
}

func (i *Integraciones) MailDisconnect(ctx context.Context, instance string) error {
	response, err := i.c.Do(ctx, Request{Method: http.MethodDelete, Path: ruta("/external-mail/", instance, "")})
	if err != nil {
		return err
	}
	return response.Body.Close()
}
