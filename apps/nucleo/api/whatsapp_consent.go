// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"net/url"
)

type WhatsAppSignupMetadata struct {
	Available    bool   `json:"available"`
	AppID        string `json:"app_id"`
	ConfigID     string `json:"config_id"`
	GraphVersion string `json:"graph_version"`
}

type WhatsAppSignup struct {
	InstanceID   string `json:"instance_id"`
	State        string `json:"state"`
	AppID        string `json:"app_id"`
	ConfigID     string `json:"config_id"`
	GraphVersion string `json:"graph_version"`
}

type WhatsAppNumber struct {
	ID                   string `json:"id"`
	DisplayPhone         string `json:"display_phone"`
	VerifiedName         string `json:"verified_name"`
	RegistrationRequired bool   `json:"registration_required"`
}

func (i *Integraciones) WhatsAppSignupMetadata(ctx context.Context) (WhatsAppSignupMetadata, error) {
	return JSON[WhatsAppSignupMetadata](ctx, i.c, Request{Path: "/whatsapp/signup"})
}

func (i *Integraciones) BeginWhatsAppSignup(ctx context.Context, instance string) (WhatsAppSignup, error) {
	return PostJSON[WhatsAppSignup](ctx, i.c, "/whatsapp/signup/start", struct {
		InstanceID string `json:"instance_id,omitempty"`
	}{instance})
}

func (i *Integraciones) WhatsAppNumbers(ctx context.Context, instance string) ([]WhatsAppNumber, error) {
	out, err := JSON[struct {
		Numbers []WhatsAppNumber `json:"numbers"`
	}](ctx, i.c, Request{Path: "/whatsapp/numbers", Query: url.Values{"instance": {instance}}})
	return out.Numbers, err
}

// LeerRetornoWhatsApp carries no authorization or success flag. The wizard must
// re-read its own instance using the native session before showing any number.
func LeerRetornoWhatsApp(u *url.URL) (string, bool) {
	if u == nil || u.Scheme != Esquema("rooms") || u.Host != "connect" || u.Path != "/whatsapp" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return "", false
	}
	q := u.Query()
	instance := q.Get("instance")
	if len(q) != 1 || len(q["instance"]) != 1 || instance == "" || len(instance) > 128 {
		return "", false
	}
	return instance, true
}
