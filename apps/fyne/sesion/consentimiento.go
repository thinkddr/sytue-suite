// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"net/url"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type autorizacionApp struct {
	start   api.OAuthAuthorization
	llegada chan api.OAuthCallback
}

// AutorizarApp opens the system browser and completes the gateway consent with
// the native user's bearer token. Call from a background goroutine, with a context
// canceled when its wizard closes. Provider tokens remain in the gateway vault.
func (s *Suite) AutorizarApp(ctx context.Context, gateway *api.Integraciones, template, instanceID, section string) (string, error) {
	return s.autorizarCuenta(ctx, gateway, section, func(ctx context.Context) (api.OAuthAuthorization, error) {
		return gateway.OAuthStart(ctx, template, instanceID, section)
	})
}

// AutorizarAppNueva conserva las cuentas previas y vincula una identidad aparte.
// Con error puede devolver el ID del borrador para reintentar; sólo nil confirma.
func (s *Suite) AutorizarAppNueva(ctx context.Context, gateway *api.Integraciones, template, section string) (string, error) {
	return s.autorizarCuenta(ctx, gateway, section, func(ctx context.Context) (api.OAuthAuthorization, error) {
		return gateway.OAuthStartNew(ctx, template, section)
	})
}

// AutorizarConversaciones connects a personal external account through the same
// system-browser callback owner, without requiring organisation administration.
func (s *Suite) AutorizarConversaciones(ctx context.Context, gateway *api.Integraciones, provider, instance string) (string, error) {
	return s.autorizarCuenta(ctx, gateway, "native-rooms", func(ctx context.Context) (api.OAuthAuthorization, error) {
		return gateway.ExternalChatStart(ctx, provider, instance)
	})
}

func (s *Suite) AutorizarReunion(ctx context.Context, gateway *api.Integraciones, provider, instance string) (string, error) {
	return s.autorizarCuenta(ctx, gateway, "native-rooms", func(ctx context.Context) (api.OAuthAuthorization, error) {
		return gateway.ExternalMeetingStart(ctx, provider, instance)
	})
}

// AutorizarNube conecta una nube personal (Google Drive, OneDrive o Dropbox) de la
// persona, con vuelta a Drive y sin pasar por la administración de la organización.
func (s *Suite) AutorizarNube(ctx context.Context, gateway *api.Integraciones, provider string) (string, error) {
	return s.autorizarCuenta(ctx, gateway, "native-personal-storage", func(ctx context.Context) (api.OAuthAuthorization, error) {
		return gateway.NubeConectar(ctx, provider, "")
	})
}

// AutorizarCorreo uses the shared native consent owner, including its nonce,
// system-browser return and authenticated exchange. Provider tokens stay server-side.
func (s *Suite) AutorizarCorreo(ctx context.Context, gateway *api.Integraciones, provider, instance string, services ...string) (string, error) {
	return s.AutorizarCorreoDesde(ctx, gateway, provider, instance, "native-mail", services...)
}

func (s *Suite) AutorizarCorreoDesde(ctx context.Context, gateway *api.Integraciones, provider, instance, section string, services ...string) (string, error) {
	draftID := ""
	id, err := s.autorizarCuenta(ctx, gateway, section, func(ctx context.Context) (api.OAuthAuthorization, error) {
		start, err := gateway.MailConnectDesde(ctx, provider, instance, section, services...)
		draftID = start.InstanceID
		return start, err
	})
	if err != nil && id == "" {
		// Retain only the validated draft identity so a denied/canceled consent
		// can be retried without creating another account. This is not a grant.
		return draftID, err
	}
	return id, err
}

func (s *Suite) autorizarCuenta(ctx context.Context, gateway *api.Integraciones, section string, begin func(context.Context) (api.OAuthAuthorization, error)) (string, error) {
	if gateway == nil || s.app == nil || api.NativeConsentProduct(section) == "" ||
		s.app.Metadata().Custom["esquema"] != api.Esquema(api.NativeConsentProduct(section)) {
		return "", api.ErrOAuthState
	}
	flow := &autorizacionApp{llegada: make(chan api.OAuthCallback, 1)}
	s.mu.Lock()
	if s.consentimiento != nil {
		s.mu.Unlock()
		return "", errors.New("oauth: another account connection is open")
	}
	s.consentimiento = flow
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.consentimiento == flow {
			s.consentimiento = nil
		}
		s.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	start, err := begin(ctx)
	if err != nil {
		return start.InstanceID, err
	}
	s.mu.Lock()
	flow.start = start
	s.mu.Unlock()
	opened := make(chan error, 1)
	fyne.Do(func() { opened <- api.AbrirFuera(s.app, start.AuthorizeURL) })
	select {
	case err := <-opened:
		if err != nil {
			return start.InstanceID, err
		}
	case <-ctx.Done():
		return start.InstanceID, ctx.Err()
	}
	select {
	case callback := <-flow.llegada:
		result, err := gateway.OAuthFinish(ctx, callback)
		if err != nil {
			return start.InstanceID, err
		}
		if err := result.ConsentError(start); err != nil {
			return start.InstanceID, err
		}
		return result.Instance, nil
	case <-ctx.Done():
		return start.InstanceID, ctx.Err()
	}
}

// recibirConsentimiento reserves a separate path from Sytue ID's auth callback.
// An unsolicited or stale return never triggers an authenticated token exchange.
func (s *Suite) recibirConsentimiento(u *url.URL) bool {
	if s.app == nil || u == nil || u.Scheme != s.app.Metadata().Custom["esquema"] {
		return false
	}
	product := api.NativeConsentProductForScheme(u.Scheme)
	if product == "" {
		return false
	}
	callback, ok := api.ParseOAuthCallback(product, u)
	if !ok {
		return false
	}
	s.mu.Lock()
	flow := s.consentimiento
	if flow != nil && flow.start.Accepts(callback) {
		select {
		case flow.llegada <- callback:
		default:
		}
	}
	s.mu.Unlock()
	return true
}
