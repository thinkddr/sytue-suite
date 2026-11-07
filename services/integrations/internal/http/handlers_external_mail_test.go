// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/mailbox"
)

func cuentaDeCorreo(id, template, address string, services []string, created time.Time) domain.AppInstance {
	encoded, _ := json.Marshal(mailConfiguration{Address: address})
	cfg := map[string]string{"_mail_config": string(encoded)}
	if services != nil {
		raw, _ := json.Marshal(services)
		cfg[mailServicesKey] = string(raw)
	}
	raw, _ := json.Marshal(cfg)
	return domain.AppInstance{ID: id, TemplateKey: template, Status: domain.StatusActive, HasSecrets: true, Config: raw, CreatedAt: created}
}

func borradorDeCorreo(id, template string, created time.Time) domain.AppInstance {
	return domain.AppInstance{ID: id, TemplateKey: template, Status: domain.StatusDraft, Config: json.RawMessage(`{}`), CreatedAt: created}
}

// B2: «Añadir» cancelado dejaba una fila «Gmail» sin dirección por intento.
func TestExternalMailHidesDraftsAndListsOldestFirst(t *testing.T) {
	now := time.Now()
	instances := []domain.AppInstance{ // como los devuelve el store: más nuevo primero
		borradorDeCorreo("borrador", "gmail-mail", now),
		cuentaDeCorreo("segunda", "microsoft-mail", "b@example.eu", nil, now.Add(-time.Hour)),
		{ID: "chat", TemplateKey: tmplSlackChat, Status: domain.StatusActive, HasSecrets: true, CreatedAt: now.Add(-90 * time.Minute)},
		cuentaDeCorreo("primera", "gmail-mail", "a@example.eu", nil, now.Add(-2*time.Hour)),
	}
	items := externalMailItems(instances)
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item["instance_id"].(string))
	}
	if !reflect.DeepEqual(got, []string{"primera", "segunda"}) {
		t.Fatalf("cuentas = %v; quiero las conectadas, de la más antigua a la más nueva", got)
	}
	if instances[0].ID != "borrador" {
		t.Fatal("ordenar la respuesta reordenó la lista del llamador")
	}
}

func TestExternalMailReusesTheNewestDraftAndPurgesAbandonedOnes(t *testing.T) {
	now := time.Now()
	instances := []domain.AppInstance{
		borradorDeCorreo("gmail-reciente", "gmail-mail", now.Add(-time.Minute)),
		borradorDeCorreo("gmail-viejo", "gmail-mail", now.Add(-48*time.Hour)),
		borradorDeCorreo("outlook-de-hoy", "microsoft-mail", now.Add(-time.Hour)),
		borradorDeCorreo("outlook-viejo", "microsoft-mail", now.Add(-25*time.Hour)),
		{ID: "chat-viejo", TemplateKey: tmplSlackChat, Status: domain.StatusDraft, CreatedAt: now.Add(-72 * time.Hour)},
		cuentaDeCorreo("conectada", "gmail-mail", "a@example.eu", nil, now.Add(-72*time.Hour)),
	}
	reuse, stale := mailDrafts(instances, "gmail-mail", now)
	if reuse != "gmail-reciente" {
		t.Fatalf("reutiliza %q; quiero el borrador más reciente de Gmail", reuse)
	}
	slices.Sort(stale)
	if !reflect.DeepEqual(stale, []string{"gmail-viejo", "outlook-viejo"}) {
		t.Fatalf("borra %v; quiero solo borradores de correo de más de un día", stale)
	}
	if reuse, _ := mailDrafts(instances[5:], "gmail-mail", now); reuse != "" {
		t.Fatalf("reutilizó una cuenta conectada: %q", reuse)
	}
}

// B3: la misma dirección dos veces pintaba el buzón dos veces.
func TestExternalMailDuplicateNeedsSameProviderAddressAndService(t *testing.T) {
	now := time.Now()
	instances := []domain.AppInstance{
		cuentaDeCorreo("solo-calendario", "gmail-mail", "ana@example.eu", []string{"calendar"}, now),
		cuentaDeCorreo("correo", "gmail-mail", "Ana@Example.eu", []string{"mail", "calendar"}, now),
		cuentaDeCorreo("imap", "mail-imap", "ana@example.eu", nil, now),
		borradorDeCorreo("borrador", "gmail-mail", now),
	}
	for _, tc := range []struct {
		name, template, address, except, service, want string
	}{
		{"misma cuenta de correo", "gmail-mail", "ana@example.eu", "borrador", "mail", "correo"},
		{"la propia cuenta al reconectar", "gmail-mail", "ana@example.eu", "correo", "mail", ""},
		{"otro proveedor", "microsoft-mail", "ana@example.eu", "", "mail", ""},
		{"otra dirección", "gmail-mail", "bea@example.eu", "", "mail", ""},
		{"sin dirección", "gmail-mail", "", "", "mail", ""},
		{"calendario ya conectado", "gmail-mail", "ana@example.eu", "", "calendar", "solo-calendario"},
		{"notas aún no", "gmail-mail", "ana@example.eu", "", "notes", ""},
		{"IMAP repetido", "mail-imap", "ANA@example.eu", "", "mail", "imap"},
		{"otra dirección en la propia cuenta", "gmail-mail", "ana@example.eu", "solo-calendario", "mail", "correo"},
	} {
		if got := duplicateMailAccount(instances, tc.template, tc.address, tc.except, tc.service); got != tc.want {
			t.Errorf("%s: %q, quiero %q", tc.name, got, tc.want)
		}
	}
}

// Las copias repetidas de antes de B3: reconectar la caducada la repara, no
// apunta a la otra. Antes daba siempre «ya conectada» y solo se podía desconectar.
func TestReconnectingAnExistingDuplicateRepairsIt(t *testing.T) {
	now := time.Now()
	instances := []domain.AppInstance{
		cuentaDeCorreo("copia-a", "gmail-mail", "ana@example.eu", []string{"mail"}, now.Add(-2*time.Hour)),
		cuentaDeCorreo("copia-b", "gmail-mail", "ana@example.eu", []string{"mail"}, now.Add(-time.Hour)),
	}
	for _, except := range []string{"copia-a", "copia-b"} {
		if got := duplicateMailAccount(instances, "gmail-mail", "ANA@example.eu", except, "mail"); got != "" {
			t.Errorf("reconectar %s apunta a %q; quiero repararla", except, got)
		}
	}
	if got := duplicateMailAccount(instances, "gmail-mail", "ana@example.eu", "", "mail"); got != "copia-a" {
		t.Errorf("un alta nueva apunta a %q; quiero la primera", got)
	}
}

// Desconectar en Mail la cuenta que también está conectada para Calendario
// revocaba en Google el permiso de las dos: el Calendario dejaba de funcionar.
func TestDisconnectKeepsTheProviderGrantWhileAnotherConnectionSharesIt(t *testing.T) {
	now := time.Now()
	correo := cuentaDeCorreo("correo", "gmail-mail", "ana@example.eu", []string{"mail"}, now)
	calendario := cuentaDeCorreo("calendario", "gmail-mail", "Ana@Example.eu", []string{"calendar"}, now)
	otraDireccion := cuentaDeCorreo("bea", "gmail-mail", "bea@example.eu", []string{"calendar"}, now)
	outlook := cuentaDeCorreo("outlook", "microsoft-mail", "ana@example.eu", []string{"calendar"}, now)
	borrador := borradorDeCorreo("borrador", "gmail-mail", now)
	if !sharedMailGrant([]domain.AppInstance{correo, calendario}, correo) {
		t.Fatal("revoca el permiso que usa la conexión de Calendario")
	}
	for name, others := range map[string][]domain.AppInstance{
		"sola":           {correo},
		"otra dirección": {correo, otraDireccion},
		"otro proveedor": {correo, outlook},
		"un borrador":    {correo, borrador},
	} {
		if sharedMailGrant(others, correo) {
			t.Errorf("%s: no revoca un permiso que nadie más usa", name)
		}
	}
}

// B7: quitar el permiso en la pantalla granular de Google llegaba como «exchange».
func TestOAuthFinishTellsMissingScopeAndExistingAccountApart(t *testing.T) {
	err := applyMailServicesGrant("gmail-mail", map[string]string{}, json.RawMessage(`"openid email"`), []string{"https://mail.google.com/"}, nil)
	if !errors.Is(err, errMailScope) || !errors.Is(err, mailbox.ErrAuth) {
		t.Fatalf("sin permiso de correo = %v; quiero errMailScope, que sigue siendo ErrAuth", err)
	}
	for _, tc := range []struct {
		err            error
		code, existing string
	}{
		{err, "scope", ""},
		{fmt.Errorf("canje: %w", &errMailAccountExists{instance: "la-de-antes"}), "exists", "la-de-antes"},
		{mailbox.ErrAuth, "exchange", ""},
		{errors.New("el proveedor rechazó el canje con 400"), "exchange", ""},
	} {
		if code, existing := finishError(tc.err); code != tc.code || existing != tc.existing {
			t.Errorf("%v → (%q, %q); quiero (%q, %q)", tc.err, code, existing, tc.code, tc.existing)
		}
	}
}
