// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la FUSIÓN DE IDENTIDAD contra PostgreSQL real (pgtest).
//
// Lo que se fija aquí es que absorber una organización no deja a nadie apuntando a
// una organización de la que acaba de dejar de ser miembro. La fusión mueve las
// pertenencias y borra las del origen; cualquier fila que siga nombrando al origen
// y que se use para AUTORIZAR queda, a partir de ese momento, denegando.
//
// Pasó en producción con `corporate`: `onboarding_progress` se quedaba en la
// organización absorbida, el asistente de alta re-verifica ser OWNER de esa org en
// cada paso, y respondía 403 «No eres propietario de esta organización». Como el
// portal manda a /bienvenida a quien no tenga el alta completada, el usuario
// aterrizaba justo en la pantalla que ya no podía cargar.
package store_test

import (
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestMergeIdentityCierraElAltaDelOrigen: tras fusionar, el alta guiada de quien
// venía del origen apunta al destino y queda completada, que es lo que saca al
// usuario del asistente en vez de dejarlo en una pantalla que le deniega.
func TestMergeIdentityCierraElAltaDelOrigen(t *testing.T) {
	st, ctx := newStore(t)

	origen, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "huerfana", Name: "Huérfana"})
	if err != nil {
		t.Fatalf("CreateOrg(origen): %v", err)
	}
	destino, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "corporate", Name: "Corporate"})
	if err != nil {
		t.Fatalf("CreateOrg(destino): %v", err)
	}

	ada := seedUser(t, ctx, st, "ada@huerfana.example")
	if err := st.UpsertMembership(ctx, nil, domain.Membership{
		OrgID: origen.ID, UserID: ada.ID, Role: domain.RoleOwner,
	}); err != nil {
		t.Fatalf("UpsertMembership(origen): %v", err)
	}
	// Alta a medias: creó su organización y no llegó al final. Es el estado con el
	// que el portal redirige a /bienvenida.
	if _, err := st.UpsertOnboarding(ctx, nil, ada.ID, "organization", "personas", origen.ID, false); err != nil {
		t.Fatalf("UpsertOnboarding: %v", err)
	}

	if _, err := st.MergeIdentity(ctx, origen.ID, destino.ID); err != nil {
		t.Fatalf("MergeIdentity: %v", err)
	}

	ob, err := st.GetOnboarding(ctx, ada.ID)
	if err != nil {
		t.Fatalf("GetOnboarding: %v", err)
	}
	if ob.OrgID != destino.ID {
		t.Errorf("el alta sigue apuntando a %q; debería seguir a la organización destino %q",
			ob.OrgID, destino.ID)
	}
	if ob.CompletedAt == nil {
		t.Error("el alta quedó abierta: el portal seguiría mandando a /bienvenida, " +
			"y allí el asistente exige ser OWNER, que la fusión acaba de quitar")
	}

	// La razón de fondo, comprobada en vez de razonada: en el origen ya no queda
	// rol ninguno. Cualquier autorización que se apoye en esa organización deniega.
	// Sin pertenencia, la consulta devuelve ErrNotFound en vez de lista vacía; las
	// dos formas dicen lo mismo aquí y el asistente trata igual a ambas.
	roles, err := st.RolesForUserInOrg(ctx, origen.ID, ada.ID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
	case err != nil:
		t.Fatalf("RolesForUserInOrg(origen): %v", err)
	case len(roles) != 0:
		t.Errorf("el origen conserva roles %v tras la fusión", roles)
	}
}

// TestMergeIdentityNoReescribeAltasYaCompletadas: la fusión mueve la referencia,
// pero no toca cuándo se dio de alta alguien que ya había terminado.
func TestMergeIdentityNoReescribeAltasYaCompletadas(t *testing.T) {
	st, ctx := newStore(t)

	origen, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "origen", Name: "Origen"})
	if err != nil {
		t.Fatalf("CreateOrg(origen): %v", err)
	}
	destino, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "destino", Name: "Destino"})
	if err != nil {
		t.Fatalf("CreateOrg(destino): %v", err)
	}
	grace := seedUser(t, ctx, st, "grace@origen.example")
	if err := st.UpsertMembership(ctx, nil, domain.Membership{
		OrgID: origen.ID, UserID: grace.ID, Role: domain.RoleOwner,
	}); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	if _, err := st.UpsertOnboarding(ctx, nil, grace.ID, "organization", "listo", origen.ID, true); err != nil {
		t.Fatalf("UpsertOnboarding: %v", err)
	}
	antes, err := st.GetOnboarding(ctx, grace.ID)
	if err != nil {
		t.Fatalf("GetOnboarding(antes): %v", err)
	}
	if antes.CompletedAt == nil {
		t.Fatal("el alta debería nacer completada en este caso")
	}

	if _, err := st.MergeIdentity(ctx, origen.ID, destino.ID); err != nil {
		t.Fatalf("MergeIdentity: %v", err)
	}

	despues, err := st.GetOnboarding(ctx, grace.ID)
	if err != nil {
		t.Fatalf("GetOnboarding(después): %v", err)
	}
	if despues.OrgID != destino.ID {
		t.Errorf("el alta no siguió al destino: %q", despues.OrgID)
	}
	if !despues.CompletedAt.Equal(*antes.CompletedAt) {
		t.Errorf("la fusión reescribió la fecha de alta: %v → %v", *antes.CompletedAt, *despues.CompletedAt)
	}
}

// TestCleanupExpiredEjecutaTodasSusConsultas: la purga periódica corre entera.
//
// No comprueba cuánto borra —eso depende de los márgenes de retención, que son
// política y cambian—, sino que NINGUNA de sus consultas revienta. La de sesiones
// lo hacía: `$1 - interval '7 days'` deja el parámetro sin tipo, PostgreSQL lo
// resuelve como interval y la comparación acaba siendo `timestamptz < interval`.
// Al ir las consultas en secuencia, ese error abortaba también la purga de
// refresh tokens y las siguientes. En producción eran diez minutos de WARN y las
// dos tablas que más crecen sin limpiar nunca, sin nada más visible.
func TestCleanupExpiredEjecutaTodasSusConsultas(t *testing.T) {
	st, ctx := newStore(t)

	ada := seedUser(t, ctx, st, "ada@cleanup.example")
	// Una sesión caducada hace mucho más que el margen de retención: si la purga
	// llega hasta ella, desaparece; si la consulta revienta, ni se intenta.
	vieja := time.Now().UTC().Add(-90 * 24 * time.Hour)
	if _, err := st.InsertSession(ctx, nil, domain.Session{
		TokenHash: hashOf("sess-caducada"), UserID: ada.ID, AuthTime: vieja,
		AMR: []string{"pwd"}, ExpiresAt: vieja.Add(12 * time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	if err := st.CleanupExpired(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}

	if _, err := st.GetSessionByToken(ctx, hashOf("sess-caducada")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("la sesión caducada sigue ahí tras la purga (err=%v)", err)
	}
}

// TestAltaDeOrganizacionesFusionadasQuedaReparada: la migración 0042 arregla lo
// que las fusiones anteriores al arreglo dejaron roto.
//
// pgtest aplica TODAS las migraciones al arrancar, así que aquí se reproduce el
// estado defectuoso a mano —una fila de alta apuntando a una organización ya
// marcada como fusionada— y se comprueba que la misma sentencia lo repara. Sin
// esto, el arreglo del código convive con cuentas que siguen viendo el 403 y
// nada lo dice.
func TestAltaDeOrganizacionesFusionadasQuedaReparada(t *testing.T) {
	st, ctx := newStore(t)

	origen, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "absorbida", Name: "Absorbida"})
	if err != nil {
		t.Fatalf("CreateOrg(origen): %v", err)
	}
	destino, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "receptora", Name: "Receptora"})
	if err != nil {
		t.Fatalf("CreateOrg(destino): %v", err)
	}
	hedy := seedUser(t, ctx, st, "hedy@absorbida.example")
	if _, err := st.UpsertOnboarding(ctx, nil, hedy.ID, "organization", "personas", origen.ID, false); err != nil {
		t.Fatalf("UpsertOnboarding: %v", err)
	}
	// El estado que dejaba la fusión antigua: origen marcado, alta sin mover.
	if _, err := st.DB().Exec(ctx,
		`UPDATE organizations SET status = 'merged', merged_into_org_id = $2, merged_at = now()
		 WHERE id = $1`, origen.ID, destino.ID); err != nil {
		t.Fatalf("marcar el origen como fusionado: %v", err)
	}

	// La misma sentencia de 0042_alta_de_organizaciones_fusionadas.up.sql.
	if _, err := st.DB().Exec(ctx, `
		UPDATE onboarding_progress op
		SET org_id       = o.merged_into_org_id,
		    completed_at = COALESCE(op.completed_at, now()),
		    updated_at   = now()
		FROM organizations o
		WHERE op.org_id = o.id
		  AND o.status = 'merged'
		  AND o.merged_into_org_id IS NOT NULL`); err != nil {
		t.Fatalf("reparación: %v", err)
	}

	ob, err := st.GetOnboarding(ctx, hedy.ID)
	if err != nil {
		t.Fatalf("GetOnboarding: %v", err)
	}
	if ob.OrgID != destino.ID {
		t.Errorf("el alta sigue en la organización absorbida (%q)", ob.OrgID)
	}
	if ob.CompletedAt == nil {
		t.Error("el alta sigue abierta: el portal continuaría mandando a /bienvenida")
	}
}
