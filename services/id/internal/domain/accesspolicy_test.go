// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
	"time"
)

// mon10 es un lunes a las 10:00 UTC (para las franjas horarias). 2024-01-01 fue lunes.
var mon10 = time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

func ctx(userID, ip, clientID string, groups ...string) AccessContext {
	return AccessContext{UserID: userID, GroupIDs: groups, IP: ip, ClientID: clientID, Now: mon10}
}

// allPolicy construye una política habilitada que aplica a todos, con la acción dada.
func allPolicy(id string, action AccessAction) AccessPolicy {
	return AccessPolicy{
		ID: id, Name: id, Enabled: true, Priority: 100, Action: action,
		StartMinute: -1, EndMinute: -1,
	}
}

func TestEvaluate_NoPolicies_Allows(t *testing.T) {
	if d := EvaluateAccess(nil, ctx("u1", "1.2.3.4", "")); d.Action != AccessAllow {
		t.Fatalf("sin políticas debe permitir, got %q", d.Action)
	}
}

func TestEvaluate_DisabledPolicy_Ignored(t *testing.T) {
	p := allPolicy("p", AccessBlock)
	p.Enabled = false
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u1", "1.2.3.4", "")); d.Action != AccessAllow {
		t.Fatalf("una política deshabilitada no debe aplicar, got %q", d.Action)
	}
}

func TestEvaluate_BlockAndRequireMFA_ActionsFire(t *testing.T) {
	if d := EvaluateAccess([]AccessPolicy{allPolicy("b", AccessBlock)}, ctx("u1", "1.2.3.4", "")); d.Action != AccessBlock || d.PolicyID != "b" {
		t.Fatalf("block debe disparar con su policy id, got %+v", d)
	}
	if d := EvaluateAccess([]AccessPolicy{allPolicy("m", AccessRequireMFA)}, ctx("u1", "1.2.3.4", "")); d.Action != AccessRequireMFA {
		t.Fatalf("require_mfa debe disparar, got %q", d.Action)
	}
}

func TestEvaluate_MostRestrictiveWins(t *testing.T) {
	mfa := allPolicy("mfa", AccessRequireMFA)
	mfa.Priority = 10
	block := allPolicy("block", AccessBlock)
	block.Priority = 50
	// Aunque require_mfa tiene menor prioridad (se evalúa antes), gana el block.
	d := EvaluateAccess([]AccessPolicy{mfa, block}, ctx("u1", "1.2.3.4", ""))
	if d.Action != AccessBlock || d.PolicyID != "block" {
		t.Fatalf("debe ganar la más restrictiva (block), got %+v", d)
	}
}

func TestEvaluate_TieBrokenByPriority(t *testing.T) {
	b1 := allPolicy("first", AccessBlock)
	b1.Priority = 5
	b2 := allPolicy("second", AccessBlock)
	b2.Priority = 9
	// Dos block: decide el de menor prioridad (evaluado primero), para un audit estable.
	if d := EvaluateAccess([]AccessPolicy{b2, b1}, ctx("u1", "1.2.3.4", "")); d.PolicyID != "first" {
		t.Fatalf("con empate gana la menor prioridad, got %q", d.PolicyID)
	}
}

func TestEvaluate_BreakGlassExclusion(t *testing.T) {
	p := allPolicy("block", AccessBlock)
	p.ExcludeUsers = []string{"admin"}
	// El admin excluido NO sufre el block (sale allow); otro usuario sí.
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("admin", "1.2.3.4", "")); d.Action != AccessAllow {
		t.Fatalf("usuario excluido (break-glass) debe pasar, got %q", d.Action)
	}
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("bob", "1.2.3.4", "")); d.Action != AccessBlock {
		t.Fatalf("usuario NO excluido debe bloquearse, got %q", d.Action)
	}
}

func TestEvaluate_ExcludeByGroup(t *testing.T) {
	p := allPolicy("block", AccessBlock)
	p.ExcludeGroups = []string{"g-breakglass"}
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u1", "1.2.3.4", "", "g-breakglass")); d.Action != AccessAllow {
		t.Fatalf("miembro de grupo excluido debe pasar, got %q", d.Action)
	}
}

func TestEvaluate_IncludeScoping(t *testing.T) {
	byUser := allPolicy("block", AccessBlock)
	byUser.IncludeUsers = []string{"target"}
	if d := EvaluateAccess([]AccessPolicy{byUser}, ctx("target", "1.2.3.4", "")); d.Action != AccessBlock {
		t.Fatalf("usuario incluido debe verse afectado, got %q", d.Action)
	}
	if d := EvaluateAccess([]AccessPolicy{byUser}, ctx("other", "1.2.3.4", "")); d.Action != AccessAllow {
		t.Fatalf("usuario NO incluido no debe verse afectado, got %q", d.Action)
	}
	byGroup := allPolicy("mfa", AccessRequireMFA)
	byGroup.IncludeGroups = []string{"eng"}
	if d := EvaluateAccess([]AccessPolicy{byGroup}, ctx("u", "1.2.3.4", "", "eng")); d.Action != AccessRequireMFA {
		t.Fatalf("miembro del grupo incluido debe verse afectado, got %q", d.Action)
	}
	if d := EvaluateAccess([]AccessPolicy{byGroup}, ctx("u", "1.2.3.4", "", "sales")); d.Action != AccessAllow {
		t.Fatalf("no-miembro del grupo incluido no debe verse afectado, got %q", d.Action)
	}
}

func TestEvaluate_IPInsideOutsideCIDR(t *testing.T) {
	p := allPolicy("block", AccessBlock)
	p.IPRanges = []string{"10.0.0.0/8", "192.168.1.0/24"}
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u", "10.5.5.5", "")); d.Action != AccessBlock {
		t.Fatalf("IP dentro del CIDR debe aplicar, got %q", d.Action)
	}
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u", "8.8.8.8", "")); d.Action != AccessAllow {
		t.Fatalf("IP fuera del CIDR no debe aplicar, got %q", d.Action)
	}
}

func TestEvaluate_IPNegate_OutsideNetwork(t *testing.T) {
	// "Exigir MFA fuera de la red de oficina": negate ⇒ aplica cuando la IP NO está.
	p := allPolicy("mfa", AccessRequireMFA)
	p.IPRanges = []string{"203.0.113.0/24"}
	p.IPNegate = true
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u", "8.8.8.8", "")); d.Action != AccessRequireMFA {
		t.Fatalf("fuera de la red (negate) debe aplicar, got %q", d.Action)
	}
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u", "203.0.113.7", "")); d.Action != AccessAllow {
		t.Fatalf("dentro de la red (negate) no debe aplicar, got %q", d.Action)
	}
}

func TestEvaluate_ClientCondition(t *testing.T) {
	p := allPolicy("block", AccessBlock)
	p.ClientIDs = []string{"sytue-mail"}
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u", "1.2.3.4", "sytue-mail")); d.Action != AccessBlock {
		t.Fatalf("app en la lista debe aplicar, got %q", d.Action)
	}
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u", "1.2.3.4", "sytue-drive")); d.Action != AccessAllow {
		t.Fatalf("otra app no debe aplicar, got %q", d.Action)
	}
	// Sin app conocida (createSession/flujos no interactivos): la condición por-app no casa.
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u", "1.2.3.4", "")); d.Action != AccessAllow {
		t.Fatalf("sin client_id la política por-app no debe aplicar, got %q", d.Action)
	}
}

func TestEvaluate_RequireTrustedDevice(t *testing.T) {
	// Política "exigir dispositivo de confianza": bloquea salvo desde un dispositivo trusted.
	p := allPolicy("block", AccessBlock)
	p.RequireTrustedDevice = true

	// Dispositivo de confianza ⇒ NO aplica (pasa).
	trusted := AccessContext{UserID: "u", IP: "1.2.3.4", TrustedDevice: true, Now: mon10}
	if d := EvaluateAccess([]AccessPolicy{p}, trusted); d.Action != AccessAllow {
		t.Fatalf("un dispositivo de confianza debe pasar, got %q", d.Action)
	}
	// Dispositivo NO de confianza (o desconocido) ⇒ aplica la acción de la política.
	untrusted := AccessContext{UserID: "u", IP: "1.2.3.4", TrustedDevice: false, Now: mon10}
	if d := EvaluateAccess([]AccessPolicy{p}, untrusted); d.Action != AccessBlock {
		t.Fatalf("un dispositivo no de confianza debe sufrir la acción, got %q", d.Action)
	}
	// require_mfa como acción: mismo comportamiento (fuerza el segundo factor).
	m := allPolicy("mfa", AccessRequireMFA)
	m.RequireTrustedDevice = true
	if d := EvaluateAccess([]AccessPolicy{m}, untrusted); d.Action != AccessRequireMFA {
		t.Fatalf("require_mfa sobre dispositivo no de confianza debe exigir MFA, got %q", d.Action)
	}
}

func TestEvaluate_RequireTrustedDevice_ExcludedAndNoPolicy(t *testing.T) {
	// Sin política de dispositivo ⇒ el estado del dispositivo es irrelevante (allow).
	if d := EvaluateAccess([]AccessPolicy{allPolicy("p", AccessAllow)},
		AccessContext{UserID: "u", TrustedDevice: false, Now: mon10}); d.Action != AccessAllow {
		t.Fatalf("sin condición de dispositivo debe permitir, got %q", d.Action)
	}
	// Usuario EXCLUIDO (break-glass) desde un dispositivo no de confianza ⇒ pasa igual.
	p := allPolicy("block", AccessBlock)
	p.RequireTrustedDevice = true
	p.ExcludeUsers = []string{"admin"}
	if d := EvaluateAccess([]AccessPolicy{p},
		AccessContext{UserID: "admin", TrustedDevice: false, Now: mon10}); d.Action != AccessAllow {
		t.Fatalf("un usuario excluido debe pasar aunque el dispositivo no sea de confianza, got %q", d.Action)
	}
	// PoliciesNeedDevice: detecta la exigencia solo en políticas habilitadas.
	if !PoliciesNeedDevice([]AccessPolicy{p}) {
		t.Fatal("PoliciesNeedDevice debe ser true con una política habilitada que la exige")
	}
	off := p
	off.Enabled = false
	if PoliciesNeedDevice([]AccessPolicy{off}) {
		t.Fatal("PoliciesNeedDevice debe ignorar políticas deshabilitadas")
	}
}

func TestEvaluate_TimeWindow(t *testing.T) {
	// Franja 09:00–17:00 (540..1020). mon10 (10:00) cae dentro.
	p := allPolicy("block", AccessBlock)
	p.StartMinute, p.EndMinute = 540, 1020
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u", "1.2.3.4", "")); d.Action != AccessBlock {
		t.Fatalf("dentro de la franja debe aplicar, got %q", d.Action)
	}
	early := AccessContext{UserID: "u", Now: time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC)}
	if d := EvaluateAccess([]AccessPolicy{p}, early); d.Action != AccessAllow {
		t.Fatalf("fuera de la franja no debe aplicar, got %q", d.Action)
	}
}

func TestEvaluate_TimeWindow_WrapsMidnight(t *testing.T) {
	// Franja 22:00–06:00 (1320..360): cruza medianoche.
	p := allPolicy("mfa", AccessRequireMFA)
	p.StartMinute, p.EndMinute = 1320, 360
	night := AccessContext{UserID: "u", Now: time.Date(2024, 1, 1, 23, 30, 0, 0, time.UTC)}
	if d := EvaluateAccess([]AccessPolicy{p}, night); d.Action != AccessRequireMFA {
		t.Fatalf("23:30 debe caer en la franja nocturna, got %q", d.Action)
	}
	noon := AccessContext{UserID: "u", Now: mon10}
	if d := EvaluateAccess([]AccessPolicy{p}, noon); d.Action != AccessAllow {
		t.Fatalf("10:00 no debe caer en la franja nocturna, got %q", d.Action)
	}
}

func TestEvaluate_DaysMask(t *testing.T) {
	// Solo entre semana (Lun..Vie): bits 1..5.
	p := allPolicy("mfa", AccessRequireMFA)
	p.DaysMask = (1 << time.Monday) | (1 << time.Tuesday) | (1 << time.Wednesday) |
		(1 << time.Thursday) | (1 << time.Friday)
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u", "1.2.3.4", "")); d.Action != AccessRequireMFA {
		t.Fatalf("lunes debe aplicar, got %q", d.Action)
	}
	sun := AccessContext{UserID: "u", Now: time.Date(2024, 1, 7, 10, 0, 0, 0, time.UTC)} // domingo
	if d := EvaluateAccess([]AccessPolicy{p}, sun); d.Action != AccessAllow {
		t.Fatalf("domingo no debe aplicar, got %q", d.Action)
	}
}

func TestEvaluate_AllConditionsAreAND(t *testing.T) {
	// Bloquear solo a un grupo, desde fuera de la red, para una app concreta.
	p := allPolicy("block", AccessBlock)
	p.IncludeGroups = []string{"contractors"}
	p.IPRanges = []string{"10.0.0.0/8"}
	p.IPNegate = true
	p.ClientIDs = []string{"sytue-drive"}
	full := ctx("u", "8.8.8.8", "sytue-drive", "contractors")
	if d := EvaluateAccess([]AccessPolicy{p}, full); d.Action != AccessBlock {
		t.Fatalf("con TODAS las condiciones debe aplicar, got %q", d.Action)
	}
	// Falla una sola condición (app distinta) ⇒ no aplica.
	if d := EvaluateAccess([]AccessPolicy{p}, ctx("u", "8.8.8.8", "sytue-mail", "contractors")); d.Action != AccessAllow {
		t.Fatalf("si una condición no casa, no debe aplicar, got %q", d.Action)
	}
}
