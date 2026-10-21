// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"slices"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/config"
)

// oauthClientCacheTTL acota cuánto tarda en surtir efecto una credencial recién
// registrada desde la superficie de plataforma. Corto a propósito: quien acaba de
// pegar un client_id está a punto de probarlo, y esperar un minuto a que «empiece
// a funcionar» se parece demasiado a que no funcione.
const oauthClientCacheTTL = 15 * time.Second

// oauthClientResolver decide con qué credencial se presenta Sytue ante un
// proveedor.
//
// Dos orígenes, y el orden importa:
//
//  1. La tabla `platform_oauth_clients`, que es lo que se edita desde la
//     superficie de plataforma. Gana, porque es lo que alguien acaba de tocar.
//  2. SYTUE_INTEGRATIONS_OAUTH_CLIENTS del entorno, que es como se configuraba
//     antes. Se conserva como respaldo para no romper un despliegue que ya lo
//     tuviera puesto, y porque es el único origen disponible mientras las
//     migraciones aún no han corrido.
//
// Los asistentes normales requieren una credencial de plataforma y solo piden
// consentimiento a cada persona. Las conexiones BYO existentes conservan su
// credencial explícita por compatibilidad; no se copian a cuentas nuevas.
type oauthClientResolver struct {
	store   platformOAuthLoader
	fromEnv map[string]config.OAuthClient

	mu       sync.RWMutex
	cached   map[string]config.OAuthClient
	cachedAt time.Time
}

// platformOAuthLoader es lo único que el resolutor necesita del store. Interfaz y
// no el tipo concreto para poder probarlo sin base de datos: lo que hay que fijar
// aquí es la PRECEDENCIA y el comportamiento ante un fallo de lectura, y ninguna
// de las dos cosas necesita PostgreSQL para comprobarse.
type platformOAuthLoader interface {
	LoadPlatformOAuthSecrets(ctx context.Context) (map[string]struct{ ClientID, ClientSecret string }, error)
}

func newOAuthClientResolver(st platformOAuthLoader, fromEnv map[string]config.OAuthClient) *oauthClientResolver {
	return &oauthClientResolver{store: st, fromEnv: fromEnv}
}

// templateAliases define equivalencias entre plantillas para la resolución de credenciales OAuth de plataforma.
// Una credencial registrada para Microsoft 365 o Azure sirve para OneDrive y Teams;
// una para Google Workspace sirve para Google Drive y Google Chat/Meet.
var templateAliases = map[string][]string{
	"gmail-mail":                {"google-workspace", "google-drive"},
	"microsoft-mail":            {"microsoft365", "azure", "onedrive"},
	"slack-chat":                {"slack"},
	"microsoft-teams-chat":      {"microsoft-teams", "microsoft365", "azure", "onedrive"},
	"google-chat-conversations": {"google-workspace", "google-drive"},
	"whatsapp-business":         {"meta"},
	"slack-rooms":               {"slack"},
	"microsoft-teams-rooms":     {"microsoft-teams", "microsoft365", "azure", "onedrive"},
	"dropbox-storage":           {"dropbox"},
	"google-drive":              {"google-workspace"},
	"google-workspace":          {"google-drive"},
	"google-chat":               {"google-workspace", "google-drive"},
	"google-meet":               {"google-workspace", "google-drive"},
	"onedrive":                  {"microsoft365", "azure"},
	"microsoft-teams":           {"microsoft365", "azure"},
	"microsoft365":              {"azure", "onedrive", "microsoft-teams"},
	"azure":                     {"microsoft365", "onedrive", "microsoft-teams"},
}

// The product selected during platform registration does not create a second
// vendor application. Exact registrations and explicit aliases retain priority.
var sharedOAuthClients = [][]string{
	{"google-workspace", "google-drive", "gmail-mail", "google-chat", "google-chat-conversations", "google-meet"},
	{"microsoft365", "azure", "onedrive", "microsoft-teams", "microsoft-teams-rooms", "microsoft-teams-chat", "microsoft-mail"},
	{"slack", "slack-rooms", "slack-chat"},
}

func lookupOAuthClient(m map[string]config.OAuthClient, templateKey string) (config.OAuthClient, bool) {
	if m == nil {
		return config.OAuthClient{}, false
	}
	if c, ok := m[templateKey]; ok && c.ClientID != "" {
		return c, true
	}
	for _, alias := range templateAliases[templateKey] {
		if c, ok := m[alias]; ok && c.ClientID != "" {
			return c, true
		}
	}
	for _, family := range sharedOAuthClients {
		if !slices.Contains(family, templateKey) {
			continue
		}
		for _, alias := range family {
			if c, ok := m[alias]; ok && c.ClientID != "" {
				return c, true
			}
		}
	}
	return config.OAuthClient{}, false
}

// get devuelve la credencial de un proveedor, o la cero si no hay ninguna.
//
// Un fallo al leer la tabla NO vacía lo que ya se tenía: se sigue sirviendo la
// última copia buena, y si no hay ninguna se cae al entorno. Una caída de la base
// no puede convertirse en «todos los proveedores pasan a BYO» a mitad de un
// consentimiento, que es la forma más ruidosa posible de fallar.
func (r *oauthClientResolver) get(ctx context.Context, templateKey string) config.OAuthClient {
	r.mu.RLock()
	fresco := r.cached != nil && time.Since(r.cachedAt) < oauthClientCacheTTL
	if fresco {
		c, ok := lookupOAuthClient(r.cached, templateKey)
		r.mu.RUnlock()
		if ok {
			return c
		}
		cEnv, _ := lookupOAuthClient(r.fromEnv, templateKey)
		return cEnv
	}
	r.mu.RUnlock()

	cargado, err := r.store.LoadPlatformOAuthSecrets(ctx)
	if err == nil {
		conv := make(map[string]config.OAuthClient, len(cargado))
		for k, v := range cargado {
			conv[k] = config.OAuthClient{ClientID: v.ClientID, ClientSecret: v.ClientSecret}
		}
		r.mu.Lock()
		r.cached, r.cachedAt = conv, time.Now()
		r.mu.Unlock()
		if c, ok := lookupOAuthClient(conv, templateKey); ok {
			return c
		}
		cEnv, _ := lookupOAuthClient(r.fromEnv, templateKey)
		return cEnv
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := lookupOAuthClient(r.cached, templateKey); ok {
		return c
	}
	cEnv, _ := lookupOAuthClient(r.fromEnv, templateKey)
	return cEnv
}

// invalidate fuerza la siguiente lectura contra la base. Lo llama la superficie de
// plataforma tras escribir, para que probar la credencial recién guardada no
// dependa de esperar a que caduque la caché.
//
// Marca la copia como CADUCADA, no la borra. La diferencia importa cuando la
// relectura falla: con la copia borrada, un fallo de la base justo después de
// guardar dejaría todos los proveedores en BYO, que es justo lo que el respaldo
// existe para evitar. Caducada, se vuelve a leer, y si no se puede se sigue
// sirviendo lo último bueno.
func (r *oauthClientResolver) invalidate() {
	r.mu.Lock()
	r.cachedAt = time.Time{}
	r.mu.Unlock()
}
