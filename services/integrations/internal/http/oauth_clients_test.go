// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/config"
)

type loaderFalso struct {
	datos  map[string]struct{ ClientID, ClientSecret string }
	err    error
	visita int
}

func (l *loaderFalso) LoadPlatformOAuthSecrets(context.Context) (map[string]struct{ ClientID, ClientSecret string }, error) {
	l.visita++
	if l.err != nil {
		return nil, l.err
	}
	return l.datos, nil
}

func entorno(id string) map[string]config.OAuthClient {
	return map[string]config.OAuthClient{"google-workspace": {ClientID: id, ClientSecret: "s-env"}}
}

// La tabla gana al entorno: es lo que alguien acaba de editar en la interfaz, y
// si no ganara, cambiar una credencial desde la superficie de plataforma no haría
// nada visible mientras el chart siguiera trayendo la vieja.
func TestLaTablaTienePrecedenciaSobreElEntorno(t *testing.T) {
	l := &loaderFalso{datos: map[string]struct{ ClientID, ClientSecret string }{
		"google-workspace": {ClientID: "id-tabla", ClientSecret: "s-tabla"},
	}}
	r := newOAuthClientResolver(l, entorno("id-entorno"))
	got := r.get(context.Background(), "google-workspace")
	if got.ClientID != "id-tabla" || got.ClientSecret != "s-tabla" {
		t.Fatalf("ganó el entorno: %+v", got)
	}
}

// Un proveedor que solo está en el entorno sigue funcionando: es como se
// configuraba antes y hay despliegues que lo tienen puesto.
func TestElEntornoSigueValiendoDeRespaldo(t *testing.T) {
	l := &loaderFalso{datos: map[string]struct{ ClientID, ClientSecret string }{}}
	r := newOAuthClientResolver(l, entorno("id-entorno"))
	if got := r.get(context.Background(), "google-workspace"); got.ClientID != "id-entorno" {
		t.Fatalf("no se cayó al entorno: %+v", got)
	}
}

// Un proveedor sin credencial en ningún sitio devuelve la cero, que es lo que
// hace que el flujo diga «registra tu propia app» en vez de intentar un
// consentimiento con client_id vacío.
func TestSinCredencialDevuelveLaCero(t *testing.T) {
	l := &loaderFalso{datos: map[string]struct{ ClientID, ClientSecret string }{}}
	r := newOAuthClientResolver(l, nil)
	if got := r.get(context.Background(), "notion"); got.ClientID != "" || got.ClientSecret != "" {
		t.Fatalf("se inventó una credencial: %+v", got)
	}
}

// Un fallo de la base NO vacía lo ya conocido. Convertir una caída de PostgreSQL
// en «todos los proveedores pasan a BYO» a mitad de un consentimiento es la forma
// más ruidosa posible de fallar, y encima transitoria: se arregla sola y nadie
// entiende qué pasó.
func TestUnFalloDeLaBaseNoBorraLoQueYaSeSabia(t *testing.T) {
	l := &loaderFalso{datos: map[string]struct{ ClientID, ClientSecret string }{
		"google-workspace": {ClientID: "id-tabla", ClientSecret: "s-tabla"},
	}}
	r := newOAuthClientResolver(l, entorno("id-entorno"))
	if got := r.get(context.Background(), "google-workspace"); got.ClientID != "id-tabla" {
		t.Fatalf("primera lectura mal: %+v", got)
	}
	l.err = errors.New("la base se cayó")
	r.invalidate() // fuerza a releer, y la relectura falla
	if got := r.get(context.Background(), "google-workspace"); got.ClientID != "id-tabla" {
		t.Fatalf("un fallo de lectura borró la credencial: %+v", got)
	}
}

// La caché evita una consulta por cada paso del consentimiento, que son varios y
// seguidos.
func TestLaCacheEvitaReleerEnCadaLlamada(t *testing.T) {
	l := &loaderFalso{datos: map[string]struct{ ClientID, ClientSecret string }{
		"google-workspace": {ClientID: "id-tabla"},
	}}
	r := newOAuthClientResolver(l, nil)
	for range 5 {
		r.get(context.Background(), "google-workspace")
	}
	if l.visita != 1 {
		t.Fatalf("se leyó la base %d veces, se esperaba 1", l.visita)
	}
}

// Guardar una credencial la deja utilizable EN EL ACTO. Sin esto, quien acaba de
// pegar un client_id lo prueba, no funciona, y concluye que está mal.
func TestInvalidarObligaAReleer(t *testing.T) {
	l := &loaderFalso{datos: map[string]struct{ ClientID, ClientSecret string }{
		"google-workspace": {ClientID: "viejo"},
	}}
	r := newOAuthClientResolver(l, nil)
	r.get(context.Background(), "google-workspace")
	l.datos["google-workspace"] = struct{ ClientID, ClientSecret string }{ClientID: "nuevo"}
	r.invalidate()
	if got := r.get(context.Background(), "google-workspace"); got.ClientID != "nuevo" {
		t.Fatalf("siguió sirviendo la vieja: %+v", got)
	}
}

// Credenciales registradas para una suite (Google Workspace o Microsoft 365)
// deben resolver automáticamente para sus servicios satélite (Google Drive,
// Google Chat/Meet, OneDrive, Teams).
func TestResolucionPorAlias(t *testing.T) {
	l := &loaderFalso{datos: map[string]struct{ ClientID, ClientSecret string }{
		"google-workspace": {ClientID: "g-id", ClientSecret: "g-sec"},
		"microsoft365":     {ClientID: "ms-id", ClientSecret: "ms-sec"},
	}}
	r := newOAuthClientResolver(l, nil)
	ctx := context.Background()

	// Google Drive y Google Chat deben resolver las credenciales de Google Workspace
	if got := r.get(ctx, "google-drive"); got.ClientID != "g-id" || got.ClientSecret != "g-sec" {
		t.Fatalf("google-drive no resolvió alias de google-workspace: %+v", got)
	}
	if got := r.get(ctx, "google-chat"); got.ClientID != "g-id" || got.ClientSecret != "g-sec" {
		t.Fatalf("google-chat no resolvió alias de google-workspace: %+v", got)
	}

	// OneDrive y Teams deben resolver las credenciales de Microsoft 365
	if got := r.get(ctx, "onedrive"); got.ClientID != "ms-id" || got.ClientSecret != "ms-sec" {
		t.Fatalf("onedrive no resolvió alias de microsoft365: %+v", got)
	}
	if got := r.get(ctx, "microsoft-teams"); got.ClientID != "ms-id" || got.ClientSecret != "ms-sec" {
		t.Fatalf("microsoft-teams no resolvió alias de microsoft365: %+v", got)
	}
}

// Operations registers each vendor once. All user-facing products must find the
// same complete credential pair, including a registry keyed by the storage app.
func TestOnePlatformRegistrationServesEveryManagedProduct(t *testing.T) {
	for _, products := range [][]string{
		{"google-drive", "google-workspace", "gmail-mail", "google-meet", "google-chat", "google-chat-conversations"},
		{"onedrive", "microsoft365", "azure", "microsoft-mail", "microsoft-teams", "microsoft-teams-chat", "microsoft-teams-rooms"},
		{"slack", "slack-chat", "slack-rooms"},
	} {
		for _, registered := range products {
			loader := &loaderFalso{datos: map[string]struct{ ClientID, ClientSecret string }{registered: {ClientID: "platform-fixture", ClientSecret: "platform-fixture-secret"}}}
			resolver := newOAuthClientResolver(loader, nil)
			for _, product := range products {
				got := resolver.get(t.Context(), product)
				if got.ClientID != "platform-fixture" || got.ClientSecret != "platform-fixture-secret" {
					t.Fatalf("registration %s unavailable to %s", registered, product)
				}
			}
		}
	}
}
