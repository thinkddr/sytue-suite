// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"maps"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// De una dirección a la persona de UNA organización, para reconstruir las menciones de
// Docs: la de entrar o un buzón vivo, solo de gente activa de esa organización. Nadie
// de otra organización, ni suspendido, ni un buzón que se está retirando.
func TestResolverDireccionesDeOrg(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	acme := orgConBuzones(t, ctx, st, "acme")
	globex := orgConBuzones(t, ctx, st, "globex")
	miembro := func(org domain.Organization, email, nombre string) domain.User {
		u := usuarioConNombre(t, ctx, st, email, nombre, "Prueba")
		if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
			t.Fatal(err)
		}
		return u
	}
	ana := miembro(acme, "ana@personal.example", "Ana")
	buzon, err := st.EnsureOrgMailbox(ctx, nil, acme.ID, ana.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	zoe := miembro(globex, "zoe@personal.example", "Zoe")
	buzonDeGlobex, err := st.EnsureOrgMailbox(ctx, nil, globex.ID, zoe.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	suspendida := miembro(acme, "sus@personal.example", "Sus")
	if _, err := st.SetUserStatus(ctx, nil, suspendida.ID, domain.StatusSuspended); err != nil {
		t.Fatal(err)
	}
	// Un buzón que se está retirando ya no es de nadie.
	bea := miembro(acme, "bea@personal.example", "Bea")
	if _, err := st.DB().Exec(ctx, `INSERT INTO org_mailboxes (org_id, user_id, address, primary_address, removed_at)
		VALUES ($1, $2, 'bea.vieja@acme.sytue.eu', false, now())`, acme.ID, bea.ID); err != nil {
		t.Fatal(err)
	}
	// Y una dirección que es el correo de entrar de una y el buzón de otra: de nadie.
	miembro(acme, "comun@acme.example", "Carla")
	if _, err := st.DB().Exec(ctx, `INSERT INTO org_mailboxes (org_id, user_id, address, primary_address)
		VALUES ($1, $2, 'comun@acme.example', false)`, acme.ID, bea.ID); err != nil {
		t.Fatal(err)
	}

	// Un miembro que se da de alta con el buzón de Ana como correo de entrar, sin
	// verificarlo, no la deja sin avisos.
	intruso := miembro(acme, "intruso@personal.example", "Intruso")
	if _, err := st.DB().Exec(ctx, `UPDATE users SET email = $1, email_verified = false WHERE id = $2`, buzon, intruso.ID); err != nil {
		t.Fatal(err)
	}

	// Y a quien dio de alta el IdP de acme por SCIM: su correo nace sin verificar, pero
	// lo afirma la organización.
	scim := miembro(acme, "scim@acme.example", "Scim")
	if _, err := st.DB().Exec(ctx, `UPDATE users SET email_verified = false, external_id = 'idp-1' WHERE id = $1`, scim.ID); err != nil {
		t.Fatal(err)
	}

	got, err := st.ResolverDireccionesDeOrg(ctx, acme.ID, []string{
		"ana@personal.example", buzon, "zoe@personal.example", buzonDeGlobex, "sus@personal.example", "nadie@x.es",
		"bea.vieja@acme.sytue.eu", "comun@acme.example", "scim@acme.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	quiero := map[string]string{"ana@personal.example": ana.ID, buzon: ana.ID, "scim@acme.example": scim.ID}
	if !maps.Equal(got, quiero) {
		t.Fatalf("resueltas %v, quiero %v", got, quiero)
	}
}

// El directorio GLOBAL (quien colabora desde otra organización): una dirección cuenta
// si es el correo de entrar VERIFICADO de una cuenta activa, o el buzón vivo de alguien
// que sigue siendo miembro de una organización activa; y es de nadie si es de dos a la
// vez (aunque una de las dos formas no cuente) o si el buzón se está retirando.
func TestDuenosGlobales(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	acme := orgConBuzones(t, ctx, st, "acme")
	globex := orgConBuzones(t, ctx, st, "globex")
	initech := orgConBuzones(t, ctx, st, "initech")
	miembro := func(org domain.Organization, email, nombre string) domain.User {
		u := usuarioConNombre(t, ctx, st, email, nombre, "Global")
		if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
			t.Fatal(err)
		}
		return u
	}
	zoe := miembro(globex, "zoe@personal.example", "Zoe")
	buzonZoe, err := st.EnsureOrgMailbox(ctx, nil, globex.ID, zoe.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	ana := miembro(acme, "ana@personal.example", "Ana")
	sus := miembro(globex, "sus@personal.example", "Sus")
	if _, err := st.SetUserStatus(ctx, nil, sus.ID, domain.StatusSuspended); err != nil {
		t.Fatal(err)
	}
	bea := miembro(globex, "bea@personal.example", "Bea")
	if _, err := st.DB().Exec(ctx, `INSERT INTO org_mailboxes (org_id, user_id, address, primary_address, removed_at)
		VALUES ($1, $2, 'bea.vieja@globex.sytue.eu', false, now())`, globex.ID, bea.ID); err != nil {
		t.Fatal(err)
	}
	miembro(acme, "comun@x.example", "Carla")
	if _, err := st.DB().Exec(ctx, `INSERT INTO org_mailboxes (org_id, user_id, address, primary_address)
		VALUES ($1, $2, 'comun@x.example', false)`, globex.ID, bea.ID); err != nil {
		t.Fatal(err)
	}
	// Eli se fue de globex y conserva su buzón; Iñigo es de initech, que se suspendió.
	eli := miembro(globex, "eli@personal.example", "Eli")
	buzonEli, err := st.EnsureOrgMailbox(ctx, nil, globex.ID, eli.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveMembership(ctx, nil, globex.ID, eli.ID); err != nil {
		t.Fatal(err)
	}
	ini := miembro(initech, "ini@personal.example", "Ini")
	buzonIni, err := st.EnsureOrgMailbox(ctx, nil, initech.ID, ini.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().Exec(ctx, `UPDATE organizations SET status = 'suspended' WHERE id = $1`, initech.ID); err != nil {
		t.Fatal(err)
	}
	// Y Ugo, que se dio de alta con una dirección que no ha verificado.
	ugo := miembro(acme, "ugo@personal.example", "Ugo")
	if _, err := st.DB().Exec(ctx, `UPDATE users SET email_verified = false WHERE id = $1`, ugo.ID); err != nil {
		t.Fatal(err)
	}

	// Alguien que se da de alta con el buzón de Zoe como correo de entrar, sin
	// verificarlo, no la deja sin avisos (no vuelve ambigua su dirección).
	intruso := usuarioConNombre(t, ctx, st, "intruso@personal.example", "Intruso", "Global")
	if _, err := st.DB().Exec(ctx, `UPDATE users SET email = $1, email_verified = false WHERE id = $2`, buzonZoe, intruso.ID); err != nil {
		t.Fatal(err)
	}
	// Una cuenta que el IdP de OTRA organización (acme) dio de alta por SCIM con el buzón
	// de Yol no la deja sin avisos: en el directorio global, SCIM no afirma nada.
	yol := miembro(globex, "yol@personal.example", "Yol")
	buzonYol, err := st.EnsureOrgMailbox(ctx, nil, globex.ID, yol.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	scim := miembro(acme, "scim@acme.example", "Scim")
	if _, err := st.DB().Exec(ctx, `UPDATE users SET email = $1, email_verified = false, external_id = 'idp-x' WHERE id = $2`, buzonYol, scim.ID); err != nil {
		t.Fatal(err)
	}
	pedidas := []string{
		buzonYol,
		"zoe@personal.example", buzonZoe, "ana@personal.example", "sus@personal.example",
		"bea.vieja@globex.sytue.eu", "comun@x.example", "nadie@x.es", buzonEli, buzonIni, "ugo@personal.example",
	}
	duenos, err := st.DuenosGlobales(ctx, pedidas)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range pedidas {
		if sub := store.Unico(duenos[e]); sub != "" {
			got[e] = sub
		}
	}
	quiero := map[string]string{"zoe@personal.example": zoe.ID, buzonZoe: zoe.ID, "ana@personal.example": ana.ID, buzonYol: yol.ID}
	if !maps.Equal(got, quiero) {
		t.Fatalf("resueltas %v, quiero %v", got, quiero)
	}

	activos, err := st.MiembrosActivos(ctx, globex.ID, []string{zoe.ID, eli.ID, sus.ID, ana.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(activos, []string{zoe.ID}) {
		t.Fatalf("miembros activos de globex %v, quiero solo a Zoe", activos)
	}
}
