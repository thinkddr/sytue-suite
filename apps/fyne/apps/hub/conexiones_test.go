// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

type almacenHub struct {
	available    bool
	err          error
	calls        int
	providers    map[string]bool
	accountCalls int
	accountsErr  error
}

func (s *almacenHub) ExternalStorage(context.Context) (api.AlmacenExterno, error) {
	s.calls++
	return api.AlmacenExterno{Available: s.available}, s.err
}

func (s *almacenHub) ExternalStorageAccounts(context.Context) (api.CuentasAlmacenExterno, error) {
	s.accountCalls++
	return api.CuentasAlmacenExterno{Providers: s.providers}, s.accountsErr
}

func TestElAccesoNativoDeHubUsaOAuthSinKEKSinAmpliarPermisos(t *testing.T) {
	admin := auth.Reclamos{Sub: "person", Tenant: "org", Roles: []string{"admin"}}
	for _, caso := range []struct {
		nombre              string
		claims              auth.Reclamos
		providers           map[string]bool
		err, accountsErr    error
		want                bool
		calls, accountCalls int
	}{
		{nombre: "google", claims: admin, providers: map[string]bool{"gdrive": true}, want: true, calls: 1, accountCalls: 1},
		{nombre: "microsoft", claims: admin, providers: map[string]bool{"onedrive": true}, want: true, calls: 1, accountCalls: 1},
		{nombre: "dropbox", claims: admin, providers: map[string]bool{"dropbox": true}, want: true, calls: 1, accountCalls: 1},
		{nombre: "todos apagados", claims: admin, providers: map[string]bool{"gdrive": false}, calls: 1, accountCalls: 1},
		{nombre: "desconocido", claims: admin, providers: map[string]bool{"unsupported": true}, calls: 1, accountCalls: 1},
		{nombre: "miembro", claims: auth.Reclamos{Sub: "person", Tenant: "org", Roles: []string{"member"}}, providers: map[string]bool{"gdrive": true}},
		{nombre: "Drive deniega", claims: admin, providers: map[string]bool{"gdrive": true}, err: errors.New("forbidden"), calls: 1},
		{nombre: "OAuth falla", claims: admin, providers: map[string]bool{"gdrive": true}, accountsErr: errors.New("offline"), calls: 1, accountCalls: 1},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			almacen := &almacenHub{providers: caso.providers, err: caso.err, accountsErr: caso.accountsErr}
			v := NuevaVista(traductor(t), nil, time.Now()).ConAlmacenamiento(almacen)
			v.cargarAccesosConexiones(t.Context(), caso.claims)
			if v.lanzador.PuedeConectarAlmacen != caso.want || almacen.calls != caso.calls || almacen.accountCalls != caso.accountCalls {
				t.Fatalf("shortcut=%v consultas=%d/%d; quiero %v %d/%d", v.lanzador.PuedeConectarAlmacen, almacen.calls, almacen.accountCalls, caso.want, caso.calls, caso.accountCalls)
			}
		})
	}
}

func TestElAccesoNativoDeHubSeAutorizaEnDrive(t *testing.T) {
	for _, caso := range []struct {
		name      string
		claims    auth.Reclamos
		available bool
		err       error
		want      bool
		calls     int
	}{
		{name: "personal free", claims: auth.Reclamos{Sub: "person", Tenant: "p-person", OrgKind: "personal", Plan: "personal_free", Roles: []string{"owner"}}, available: true, want: true, calls: 1},
		{name: "organization owner", claims: auth.Reclamos{Sub: "person", Tenant: "org", Roles: []string{"owner"}}, available: true, want: true, calls: 1},
		{name: "organization admin", claims: auth.Reclamos{Sub: "person", Tenant: "org", Roles: []string{"admin"}}, available: true, want: true, calls: 1},
		{name: "member", claims: auth.Reclamos{Sub: "person", Tenant: "org", Roles: []string{"member"}}, available: true},
		{name: "delegated Admin permission", claims: auth.Reclamos{Sub: "person", Tenant: "org", Perms: []string{"users:read"}}, available: true},
		{name: "Drive unavailable", claims: auth.Reclamos{Sub: "person", Tenant: "org", Roles: []string{"admin"}}, calls: 1},
		{name: "Drive rejects", claims: auth.Reclamos{Sub: "person", Tenant: "org", Roles: []string{"admin"}}, available: true, err: errors.New("forbidden"), calls: 1},
		{name: "no session", available: true},
	} {
		t.Run(caso.name, func(t *testing.T) {
			almacen := &almacenHub{available: caso.available, err: caso.err}
			v := NuevaVista(traductor(t), nil, time.Now()).ConAlmacenamiento(almacen)
			v.cargarAccesosConexiones(context.Background(), caso.claims)
			if v.lanzador.PuedeConectarAlmacen != caso.want || almacen.calls != caso.calls {
				t.Fatalf("shortcut=%v calls=%d", v.lanzador.PuedeConectarAlmacen, almacen.calls)
			}
		})
	}
}

func TestElConsentimientoDeHubNoUsaElRegistroDeOtraCuenta(t *testing.T) {
	c := &conexionApp{proveedor: api.OAuthTemplate{Key: "google-drive"}}
	if c.disponible() {
		t.Fatal("un catálogo antiguo ofrece consentimiento sin registro")
	}
	c.proveedor.Available = true
	if !c.disponible() {
		t.Fatal("el registro de plataforma no se puede usar")
	}
	c.proveedor.Available = false
	c.cuentas = []api.OAuthInstance{{ID: "own", TemplateKey: "google-drive", OAuthAvailable: true}}
	if !c.disponible() {
		t.Fatal("la app registrada de la organización no se puede usar")
	}
	c.instancia = "another"
	if c.disponible() {
		t.Fatal("otra cuenta toma prestado el registro de la seleccionada")
	}
}
