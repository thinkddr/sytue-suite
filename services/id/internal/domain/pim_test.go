// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"reflect"
	"sort"
	"testing"
	"time"
)

// EffectivePerms es el núcleo de la semántica PIM y se evalúa contra un «ahora»
// INYECTADO (no el reloj real), de modo determinista. Matriz clave:
//   - elegible-pero-no-activado ⇒ sin activación en la lista ⇒ SIN perms.
//   - activado y vigente ⇒ perms presentes.
//   - activado pero EXPIRADO (now >= expires) ⇒ perms desaparecen.
//   - pending / revoked / denied ⇒ no conceden (flujo de aprobación).
//   - asignación DIRECTA (direct) ⇒ concede siempre, con o sin PIM (retrocompat 0019).
//   - built-in activo ⇒ concede el set delegable (sin roles.manage).
func TestEffectivePerms(t *testing.T) {
	base := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	future := base.Add(30 * time.Minute)
	past := base.Add(-1 * time.Minute)

	// EffectivePerms devuelve el set ORDENADO; el esperado del built-in se ordena igual.
	sortedGrantable := append([]string(nil), grantablePerms()...)
	sort.Strings(sortedGrantable)

	custom := func(status string, exp time.Time, perms ...string) PIMActivation {
		return PIMActivation{Perms: perms, ExpiresAt: exp, Status: status}
	}

	cases := []struct {
		name   string
		direct []string
		acts   []PIMActivation
		want   []string
	}{
		{
			name: "elegible-sin-activar-sin-perms", // no hay activación ⇒ nada
			acts: nil,
			want: nil,
		},
		{
			name: "activado-vigente-concede",
			acts: []PIMActivation{custom(ActivationActive, future, "users.manage")},
			want: []string{"users.manage"},
		},
		{
			name: "activado-expirado-no-concede", // now >= expires
			acts: []PIMActivation{custom(ActivationActive, past, "users.manage")},
			want: nil,
		},
		{
			name: "pending-no-concede", // espera aprobación
			acts: []PIMActivation{custom(ActivationPending, future, "users.manage")},
			want: nil,
		},
		{
			name: "revoked-no-concede",
			acts: []PIMActivation{custom(ActivationRevoked, future, "users.manage")},
			want: nil,
		},
		{
			name: "denied-no-concede",
			acts: []PIMActivation{custom(ActivationDenied, future, "users.manage")},
			want: nil,
		},
		{
			name:   "directo-concede-sin-pim", // retrocompat: asignación permanente
			direct: []string{"groups.manage"},
			acts:   nil,
			want:   []string{"groups.manage"},
		},
		{
			name:   "union-directo-y-activo-dedup-ordenado",
			direct: []string{"users.manage"},
			acts: []PIMActivation{
				custom(ActivationActive, future, "users.manage", "audit.view"),
			},
			want: []string{"audit.view", "users.manage"}, // dedup + orden
		},
		{
			name: "builtin-activo-concede-set-delegable",
			acts: []PIMActivation{{Builtin: true, ExpiresAt: future, Status: ActivationActive}},
			want: sortedGrantable,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EffectivePerms(c.direct, c.acts, base)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("EffectivePerms = %v, quiero %v", got, c.want)
			}
		})
	}

	// El set delegable NUNCA incluye roles.manage (reservado a owner/admin).
	for _, p := range grantablePerms() {
		if p == string(PermRolesManage) {
			t.Fatal("grantablePerms no debe incluir roles.manage (anti auto-escalada)")
		}
	}
}

// La frontera EXACTA de la ventana: en expires_at ya NO concede (now < expires es
// estricto), un instante antes sí. Con un «ahora» inyectado se comprueba sin flakiness.
func TestEffectivePermsWindowBoundary(t *testing.T) {
	exp := time.Date(2026, 7, 23, 12, 30, 0, 0, time.UTC)
	act := []PIMActivation{{Perms: []string{"users.manage"}, ExpiresAt: exp, Status: ActivationActive}}

	if got := EffectivePerms(nil, act, exp.Add(-time.Nanosecond)); len(got) != 1 {
		t.Fatalf("justo antes de expirar debería conceder, got %v", got)
	}
	if got := EffectivePerms(nil, act, exp); got != nil {
		t.Fatalf("en expires_at NO debería conceder, got %v", got)
	}
	if got := EffectivePerms(nil, act, exp.Add(time.Minute)); got != nil {
		t.Fatalf("pasado expires_at NO debería conceder, got %v", got)
	}
}

// ValidActivationDuration: pedir más del máximo se rechaza (no se recorta); 0/negativo
// también. Igual al máximo es válido.
func TestValidActivationDuration(t *testing.T) {
	cases := []struct {
		requested, max int
		ok             bool
	}{
		{30, 60, true},
		{60, 60, true},  // igual al máximo
		{61, 60, false}, // por encima del máximo ⇒ rechazo explícito
		{0, 60, false},  // no positivo
		{-5, 60, false}, // negativo
		{30, 0, false},  // máximo inválido
	}
	for _, c := range cases {
		if got := ValidActivationDuration(c.requested, c.max); got != c.ok {
			t.Fatalf("ValidActivationDuration(%d,%d) = %v, quiero %v", c.requested, c.max, got, c.ok)
		}
	}
}

// ElevatableBuiltin: solo owner/admin son objetivo de elevación built-in.
func TestElevatableBuiltin(t *testing.T) {
	for _, r := range []string{"owner", "admin"} {
		if !ElevatableBuiltin(r) {
			t.Fatalf("%q debería ser elevable", r)
		}
	}
	for _, r := range []string{"member", "billing", "", "root"} {
		if ElevatableBuiltin(r) {
			t.Fatalf("%q NO debería ser elevable", r)
		}
	}
}

// EffectiveStatus espeja EffectivePerms para el reporte: una activación 'active'
// caducada se muestra 'expired'; pending/revoked se muestran tal cual.
func TestEffectiveStatus(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	if s := EffectiveStatus(ActivationActive, future, now); s != ActivationActive {
		t.Fatalf("active vigente = %q, quiero active", s)
	}
	if s := EffectiveStatus(ActivationActive, past, now); s != ActivationExpired {
		t.Fatalf("active caducada = %q, quiero expired", s)
	}
	if s := EffectiveStatus(ActivationPending, time.Time{}, now); s != ActivationPending {
		t.Fatalf("pending = %q, quiero pending", s)
	}
}
