// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// estadoDe saca el status HTTP del error de validación (0 si no hay error).
func estadoDe(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var he *httpx.Error
	if !errors.As(err, &he) {
		t.Fatalf("error sin status HTTP: %v", err)
	}
	return he.Status
}

func direcciones(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("persona%d@acme.test", i)
	}
	return out
}

// El tope es de direcciones VÁLIDAS y distintas: 25 pasan, 26 se rechazan
// enteras (no se invita a las primeras y se calla el resto), y repetir una
// dirección no cuenta dos veces.
func TestValidarLote_Tope(t *testing.T) {
	emails, _, _, err := validarLoteInvitaciones(inviteUserRequest{Emails: direcciones(maxBatchInvites)})
	if err != nil || len(emails) != maxBatchInvites {
		t.Fatalf("25 direcciones: emails=%d err=%v", len(emails), err)
	}
	_, _, _, err = validarLoteInvitaciones(inviteUserRequest{Emails: direcciones(maxBatchInvites + 1)})
	if estadoDe(t, err) != http.StatusBadRequest {
		t.Fatalf("26 direcciones: err=%v, quiero 400", err)
	}
	if !strings.Contains(err.Error(), "25") {
		t.Fatalf("el rechazo debe decir el tope: %v", err)
	}
	dup := append(direcciones(maxBatchInvites), "PERSONA0@acme.test")
	if _, _, _, err := validarLoteInvitaciones(inviteUserRequest{Emails: dup}); err != nil {
		t.Fatalf("un duplicado no debe contar contra el tope: %v", err)
	}
}

// Lo que no es un correo no tumba la tanda: vuelve aparte para decírselo por
// dirección, y un elemento puede traer varias pegadas (lo que llega de un CSV).
func TestValidarLote_Invalidos(t *testing.T) {
	emails, bad, role, err := validarLoteInvitaciones(inviteUserRequest{
		Emails: []string{"Ana@Acme.test, luis@acme.test", "no-arroba", "ana@acme.test"},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.Join(emails, ",") != "ana@acme.test,luis@acme.test" {
		t.Fatalf("emails=%v", emails)
	}
	if len(bad) != 1 || bad[0] != "no-arroba" {
		t.Fatalf("bad=%v", bad)
	}
	if role != domain.RoleMember {
		t.Fatalf("sin rol, la tanda invita como member; ha salido %q", role)
	}
	// Solo inválidas sigue siendo una tanda (se contesta por dirección); vacía no.
	if _, bad, _, err := validarLoteInvitaciones(inviteUserRequest{Emails: []string{"x"}}); err != nil || len(bad) != 1 {
		t.Fatalf("solo inválidas: bad=%v err=%v", bad, err)
	}
	if _, _, _, err := validarLoteInvitaciones(inviteUserRequest{Emails: []string{" ", ""}}); estadoDe(t, err) != http.StatusBadRequest {
		t.Fatalf("tanda vacía: err=%v, quiero 400", err)
	}
}

// Owner no se reparte en lote, ni siquiera si lo pide un owner; los demás roles
// sí, y uno inventado se rechaza.
func TestValidarLote_RolOwnerNoSeReparte(t *testing.T) {
	for _, r := range []string{"owner", " owner ", "superadmin"} {
		_, _, _, err := validarLoteInvitaciones(inviteUserRequest{Emails: []string{"a@acme.test"}, Role: r})
		if estadoDe(t, err) != http.StatusBadRequest {
			t.Fatalf("rol %q: err=%v, quiero 400", r, err)
		}
	}
	for _, r := range []domain.Role{domain.RoleAdmin, domain.RoleMember, domain.RoleBilling} {
		_, _, got, err := validarLoteInvitaciones(inviteUserRequest{Emails: []string{"a@acme.test"}, Role: string(r)})
		if err != nil || got != r {
			t.Fatalf("rol %q: got=%q err=%v", r, got, err)
		}
	}
}

// Lo que es de UNA persona no viaja en una tanda: ni `email` a la vez, ni buzón
// elegido, ni contraseña inicial (sería la misma credencial para todos).
func TestValidarLote_SoloCamposDeTanda(t *testing.T) {
	casos := map[string]inviteUserRequest{
		"email y emails": {Email: "a@acme.test", Emails: []string{"b@acme.test"}},
		"buzón":          {Emails: []string{"b@acme.test"}, MailboxLocal: "soporte"},
		"contraseña":     {Emails: []string{"b@acme.test"}, Password: "una-clave-larga-123"},
	}
	for nombre, req := range casos {
		if _, _, _, err := validarLoteInvitaciones(req); estadoDe(t, err) != http.StatusBadRequest {
			t.Fatalf("%s: err=%v, quiero 400", nombre, err)
		}
	}
}
