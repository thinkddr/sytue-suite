// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// OAuthField is a configuration value declared by the gateway manifest.
type OAuthField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Help        string `json:"help"`
	Placeholder string `json:"placeholder"`
	Required    bool   `json:"required"`
}

// OAuthTemplate contains public consent metadata, never provider credentials.
type OAuthTemplate struct {
	Key         string
	Name        string
	Description string
	Fields      []OAuthField
	Scopes      []string
	Host        string
	Available   bool
}

// OAuthInstance is the gateway's account inventory, without its configuration/vault.
type OAuthInstance struct {
	ID             string `json:"id"`
	TemplateKey    string `json:"template_key"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	HasSecrets     bool   `json:"has_secrets"`
	OAuthAvailable bool   `json:"oauth_available"`
}

func (i OAuthInstance) Active() bool { return i.Status == "active" && i.HasSecrets }

// PreferredOAuthInstance reuses a matching registered app before an incomplete
// draft. An explicit selection never falls through to a different account.
func PreferredOAuthInstance(accounts []OAuthInstance, template, requested string) (OAuthInstance, bool) {
	var selected OAuthInstance
	best := -1
	for _, account := range accounts {
		if account.TemplateKey != template || requested != "" && account.ID != requested {
			continue
		}
		score := 0
		if account.Active() {
			score++
		}
		if account.OAuthAvailable {
			score += 2
		}
		if score > best {
			selected, best = account, score
		}
	}
	return selected, best >= 0
}

// OAuthAuthorization binds the browser return to the consent started in this app.
type OAuthAuthorization struct {
	InstanceID   string
	AuthorizeURL string
	State        string
	Section      string
}

type OAuthCallback struct {
	State string `json:"state"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

type OAuthResult struct {
	Section  string `json:"section"`
	Instance string `json:"instance"`
	Error    string `json:"error"`
}

var (
	ErrOAuthState       = errors.New("oauth: consent does not match this app")
	ErrOAuthDenied      = errors.New("oauth: access was not granted")
	ErrOAuthUnavailable = errors.New("oauth: provider connection is not configured")
)

var nativeConsentProducts = map[string]string{
	"native-storage": "drive", "native-personal-storage": "drive", "native-rooms": "rooms", "native-mail": "mail", "native-apps": "one",
	"native-calendar": "calendar", "native-contacts": "contacts", "native-notes": "notes",
}

// NativeConsentProduct is the fixed allowlist shared with the gateway callback.
func NativeConsentProduct(section string) string { return nativeConsentProducts[section] }

// NativeConsentProductForScheme uses the same allowlist when dispatching an OS
// callback. Keeping a second list in Suite previously omitted the Mail return.
func NativeConsentProductForScheme(scheme string) string {
	for _, product := range nativeConsentProducts {
		if Esquema(product) == scheme {
			return product
		}
	}
	return ""
}

// Accepts checks the local nonce before any authenticated finish request is sent.
func (a OAuthAuthorization) Accepts(c OAuthCallback) bool {
	return a.State != "" && subtle.ConstantTimeCompare([]byte(a.State), []byte(c.State)) == 1
}

// ParseOAuthCallback accepts only the product's dedicated integration callback.
// The Sytue ID login callback remains owned by auth.OIDC.
func ParseOAuthCallback(product string, u *url.URL) (OAuthCallback, bool) {
	if u == nil || u.Scheme != Esquema(product) || u.Host != "connect" || u.Path != "/callback" ||
		u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return OAuthCallback{}, false
	}
	q := u.Query()
	for _, key := range []string{"state", "code", "error"} {
		if len(q[key]) > 1 || len(q.Get(key)) > 4096 {
			return OAuthCallback{}, false
		}
	}
	if q.Get("state") == "" {
		return OAuthCallback{}, false
	}
	return OAuthCallback{State: q.Get("state"), Code: q.Get("code"), Error: q.Get("error")}, true
}

type oauthTemplatesWire struct {
	Templates []struct {
		Key            string `json:"key"`
		Enabled        bool   `json:"enabled"`
		OAuthAvailable bool   `json:"oauth_available"`
		Manifest       struct {
			Name        string       `json:"name"`
			Description string       `json:"description"`
			Fields      []OAuthField `json:"fields"`
			OAuth       *struct {
				AuthorizeURL string   `json:"authorize_url"`
				Scopes       []string `json:"scopes"`
			} `json:"oauth"`
		} `json:"manifest"`
	} `json:"templates"`
}

// OAuthTemplates reads the same manifests used by the Admin consent wizard.
func (i *Integraciones) OAuthTemplates(ctx context.Context) ([]OAuthTemplate, error) {
	body, err := JSON[oauthTemplatesWire](ctx, i.c, Request{Path: "/templates"})
	if err != nil {
		return nil, err
	}
	var out []OAuthTemplate
	for _, item := range body.Templates {
		m := item.Manifest
		if !item.Enabled || m.OAuth == nil {
			continue
		}
		u, err := url.Parse(m.OAuth.AuthorizeURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			continue
		}
		out = append(out, OAuthTemplate{
			Key: item.Key, Name: m.Name, Description: m.Description,
			Fields: m.Fields, Scopes: m.OAuth.Scopes, Host: u.Hostname(), Available: item.OAuthAvailable,
		})
	}
	return out, nil
}

// OAuthInstances is shared by Drive, Rooms and the Hub account wizard.
func (i *Integraciones) OAuthInstances(ctx context.Context) ([]OAuthInstance, error) {
	body, err := JSON[struct {
		Instances []OAuthInstance `json:"instances"`
	}](ctx, i.c, Request{Path: "/instances"})
	return body.Instances, err
}

func (i *Integraciones) OAuthCreateInstance(ctx context.Context, template, name string, config map[string]string) (string, error) {
	if config == nil {
		config = map[string]string{}
	}
	body, err := PostJSON[struct {
		ID string `json:"id"`
	}](ctx, i.c, "/instances", map[string]any{
		"template_key": template, "name": name, "config": config,
	})
	return body.ID, err
}

// OAuthStart reuses the matching account and asks the gateway to own PKCE and tokens.
func (i *Integraciones) OAuthStart(ctx context.Context, template, instanceID, section string) (OAuthAuthorization, error) {
	return i.oauthStart(ctx, template, instanceID, section, false)
}

// OAuthStartNew starts a separate account. Existing mounts and conversations keep
// their own provider credentials; choosing another identity cannot overwrite them.
func (i *Integraciones) OAuthStartNew(ctx context.Context, template, section string) (OAuthAuthorization, error) {
	return i.oauthStart(ctx, template, "", section, true)
}

func (i *Integraciones) oauthStart(ctx context.Context, template, instanceID, section string, newAccount bool) (OAuthAuthorization, error) {
	if NativeConsentProduct(section) == "" {
		return OAuthAuthorization{}, ErrOAuthState
	}
	provider, err := i.oauthProvider(ctx, template)
	if err != nil {
		return OAuthAuthorization{}, err
	}
	var account OAuthInstance
	var found bool
	if !newAccount {
		accounts, err := i.OAuthInstances(ctx)
		if err != nil {
			return OAuthAuthorization{}, err
		}
		account, found = PreferredOAuthInstance(accounts, template, instanceID)
		if instanceID != "" && !found {
			return OAuthAuthorization{}, ErrOAuthState
		}
	}
	if !provider.Available && !account.OAuthAvailable {
		return OAuthAuthorization{}, ErrOAuthUnavailable
	}
	if found {
		instanceID = account.ID
	}
	if !found {
		instanceID, err = i.OAuthCreateInstance(ctx, template, provider.Name, nil)
		if err != nil {
			return OAuthAuthorization{}, err
		}
	}
	if instanceID == "" {
		return OAuthAuthorization{}, ErrOAuthState
	}
	authorization, err := i.oauthRequestAuthorization(ctx, instanceID, section, provider)
	if err != nil && newAccount {
		// El borrador pertenece a esta tentativa aunque el proveedor rechace el
		// comienzo. Permite reintentar ese ID sin crear otra cuenta ni pisar una vieja.
		authorization = OAuthAuthorization{InstanceID: instanceID, Section: section}
	}
	return authorization, err
}

func (i *Integraciones) oauthRequestAuthorization(ctx context.Context, instanceID, section string, provider OAuthTemplate) (OAuthAuthorization, error) {
	body, err := PostJSON[struct {
		URL string `json:"authorize_url"`
	}](ctx, i.c,
		"/instances/"+url.PathEscape(instanceID)+"/oauth/start", map[string]string{"return_to": section})
	if err != nil {
		return OAuthAuthorization{}, err
	}
	u, err := url.Parse(body.URL)
	if err != nil || u.Scheme != "https" || u.Hostname() != provider.Host || u.User != nil || u.Fragment != "" {
		return OAuthAuthorization{}, errors.New("oauth: invalid authorization URL")
	}
	state := u.Query().Get("state")
	if state == "" || len(state) > 4096 {
		return OAuthAuthorization{}, ErrOAuthState
	}
	return OAuthAuthorization{InstanceID: instanceID, AuthorizeURL: u.String(), State: state, Section: section}, nil
}

func (i *Integraciones) OAuthFinish(ctx context.Context, callback OAuthCallback) (OAuthResult, error) {
	return PostJSON[OAuthResult](ctx, i.c, "/oauth/finish", callback)
}

// OAuthDisconnect revokes the chosen account through the gateway, never locally.
func (i *Integraciones) OAuthDisconnect(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrOAuthState
	}
	_, err := PostJSON[map[string]any](ctx, i.c, "/instances/"+url.PathEscape(instanceID)+"/deactivate", map[string]any{})
	return err
}

func (r OAuthResult) ConsentError(expected OAuthAuthorization) error {
	if r.Section != expected.Section || r.Instance != expected.InstanceID {
		return ErrOAuthState
	}
	if r.Error == "denied" {
		return ErrOAuthDenied
	}
	if r.Error != "" {
		return fmt.Errorf("oauth: %s", r.Error)
	}
	return nil
}

func (i *Integraciones) oauthProvider(ctx context.Context, template string) (OAuthTemplate, error) {
	templates, err := i.OAuthTemplates(ctx)
	if err != nil {
		return OAuthTemplate{}, err
	}
	for _, provider := range templates {
		if provider.Key == template {
			return provider, nil
		}
	}
	return OAuthTemplate{}, errors.New("oauth: provider unavailable")
}
