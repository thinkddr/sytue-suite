// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/totp"
)

// TestSeedAllowed comprueba el guardarraíl: el seed SOLO se permite en dev.
func TestSeedAllowed(t *testing.T) {
	cases := []struct {
		env  config.Environment
		want bool
	}{
		{config.EnvDev, true},
		{config.EnvStaging, false},
		{config.EnvProd, false},
		{config.Environment("otro"), false},
	}
	for _, c := range cases {
		if got := seedAllowed(c.env); got != c.want {
			t.Errorf("seedAllowed(%q) = %v, quiero %v", c.env, got, c.want)
		}
	}
}

// TestResolveSeedPassword comprueba que se usa la del entorno o el default.
func TestResolveSeedPassword(t *testing.T) {
	if got := resolveSeedPassword(""); got != seedDefaultPass {
		t.Errorf("resolveSeedPassword(\"\") = %q, quiero el default %q", got, seedDefaultPass)
	}
	if got := resolveSeedPassword("mi-clave"); got != "mi-clave" {
		t.Errorf("resolveSeedPassword(\"mi-clave\") = %q, quiero \"mi-clave\"", got)
	}
}

// TestSeedPasswordHashesWithRealMechanism verifica que la contraseña efectiva
// del seed se hashea con el Argon2id REAL del servicio y luego valida.
func TestSeedPasswordHashesWithRealMechanism(t *testing.T) {
	pass := resolveSeedPassword("")
	hash, err := domain.HashPassword(pass)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := domain.VerifyPassword(pass, hash)
	if err != nil || !ok {
		t.Fatalf("VerifyPassword(default) = %v, %v; quiero true, nil", ok, err)
	}
	if bad, _ := domain.VerifyPassword("otra", hash); bad {
		t.Fatal("VerifyPassword aceptó una contraseña incorrecta")
	}
}

// TestResolveSeedClientSecret: se usa el del entorno o el default conocido.
func TestResolveSeedClientSecret(t *testing.T) {
	if got := resolveSeedClientSecret(""); got != seedDefaultClientSecret {
		t.Errorf("resolveSeedClientSecret(\"\") = %q, quiero el default %q", got, seedDefaultClientSecret)
	}
	if got := resolveSeedClientSecret("otro"); got != "otro" {
		t.Errorf("resolveSeedClientSecret(\"otro\") = %q, quiero \"otro\"", got)
	}
}

// TestSeedClientSecretVerifies es el camino crítico: el secreto en claro que el
// BFF del frontend envía al token endpoint debe validar contra el hash sembrado.
func TestSeedClientSecretVerifies(t *testing.T) {
	secret := resolveSeedClientSecret("")
	hash, err := domain.HashPassword(secret)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := domain.VerifyPassword(secret, hash)
	if err != nil || !ok {
		t.Fatalf("VerifyPassword(secreto seed) = %v, %v; quiero true, nil", ok, err)
	}
	if bad, _ := domain.VerifyPassword("secreto-erroneo", hash); bad {
		t.Fatal("VerifyPassword aceptó un client secret incorrecto")
	}
}

// TestSeedUserRoleAdminDesbloqueaConsole: el rol sembrado es admin (exige step-up
// MFA, Role.RequiresMFA) para poder acceder a Sytue Admin en dev.
func TestSeedUserRoleAdminDesbloqueaConsole(t *testing.T) {
	if seedUserRole != domain.RoleAdmin {
		t.Fatalf("seedUserRole=%q; se espera admin para desbloquear Console", seedUserRole)
	}
	if !seedUserRole.RequiresMFA() {
		t.Fatalf("seedUserRole=%q debería exigir MFA (owner/admin)", seedUserRole)
	}
}

// TestSeedMemberRoleSinConsole: el segundo usuario de dev es member (SIN rol de
// consola) y NO exige MFA, para ejercer el contrato de denegación (member → 403).
func TestSeedMemberRoleSinConsole(t *testing.T) {
	if seedMemberRole != domain.RoleMember {
		t.Fatalf("seedMemberRole=%q; se espera member (sin rol de consola)", seedMemberRole)
	}
	if seedMemberRole.RequiresMFA() {
		t.Fatalf("seedMemberRole=%q no debería exigir MFA (member/billing)", seedMemberRole)
	}
	if seedMemberEmail == seedUserEmail {
		t.Fatalf("el member (%q) debe ser distinto del admin (%q)", seedMemberEmail, seedUserEmail)
	}
}

// TestSeedTOTPSecretCumpleElSegundoFactor es el camino crítico del step-up: el
// secreto TOTP sembrado debe decodificar y producir un código que valide con el
// MISMO mecanismo (totp) que usa handleMFAVerify, para que el admin de dev pueda
// cumplir el segundo factor que su rol exige.
func TestSeedTOTPSecretCumpleElSegundoFactor(t *testing.T) {
	secret, err := totp.DecodeSecret(seedTOTPSecretB32)
	if err != nil {
		t.Fatalf("DecodeSecret(%q): %v", seedTOTPSecretB32, err)
	}
	if len(secret) == 0 {
		t.Fatal("el secreto TOTP sembrado está vacío")
	}
	now := time.Now()
	code := totp.Code(secret, now)
	if !totp.Validate(secret, code, now) {
		t.Fatalf("el código %q no valida contra el secreto sembrado", code)
	}
}
