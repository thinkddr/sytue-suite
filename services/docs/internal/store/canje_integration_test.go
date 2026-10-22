// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Canjear contra Postgres de verdad (ADR 0052): lo canjeado desde otra organización
// se escribe en la anfitriona; `docs_anfitrion` solo la dice a quien es externo; y
// revocar el enlace quita lo canjeado sin tocar un permiso directo.
func TestCanjearEnlaceEntreOrganizaciones(t *testing.T) {
	st := newStore(t)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	doc, _, err := st.CreateDocument(acme, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "T", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	link, err := st.CreateShareLink(acme, doc.ID, domain.HashToken("tk"), "", "owner", domain.RoleEditor, enUnMes())
	if err != nil {
		t.Fatal(err)
	}
	e, err := st.EnlaceParaCanjear(globex, domain.HashToken("tk"))
	if err != nil || e.LinkID != link.ID || e.Tenant != "acme" || e.Role != domain.RoleEditor {
		t.Fatalf("resolver para canjear: %+v %v", e, err)
	}
	if escrito, err := st.Canjear(acme, e, "bea", true); err != nil || !escrito {
		t.Fatalf("canjear bea: %v %v", escrito, err)
	}
	// Un permiso directo de otra persona, que el enlace no debe tocar.
	if err := st.UpsertPermission(acme, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: "ana", Role: domain.RoleViewer, GrantedBy: "owner"}); err != nil {
		t.Fatal(err)
	}
	if escrito, err := st.Canjear(acme, e, "ana", false); err != nil || escrito {
		t.Fatalf("el canje pisó un permiso directo: %v %v", escrito, err)
	}

	if a, _ := st.Anfitrion(globex, doc.ID, "bea"); a != "acme" {
		t.Errorf("anfitriona de bea = %q", a)
	}
	if a, _ := st.Anfitrion(globex, doc.ID, "ana"); a != "" {
		t.Errorf("ana no es externa y se le dice la anfitriona: %q", a)
	}
	if a, _ := st.Anfitrion(globex, doc.ID, "carla"); a != "" {
		t.Errorf("a quien no tiene nada se le dice la anfitriona: %q", a)
	}
	// Desde su organización no ve el documento: la petición tiene que pasar a la anfitriona.
	if _, err := st.GetDocument(globex, doc.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("globex ve el documento de acme: %v", err)
	}

	rol := func(sub string) domain.Role {
		perms, _ := st.ListPermissions(acme, doc.ID)
		for _, p := range perms {
			if p.PrincipalID == sub {
				return p.Role
			}
		}
		return ""
	}
	if rol("bea") != domain.RoleEditor || rol("ana") != domain.RoleViewer {
		t.Fatalf("antes de revocar: bea=%q ana=%q", rol("bea"), rol("ana"))
	}
	if err := st.RevokeShareLink(acme, doc.ID, link.ID); err != nil {
		t.Fatal(err)
	}
	if rol("bea") != "" || rol("ana") != domain.RoleViewer {
		t.Errorf("tras revocar: bea=%q (debe desaparecer) ana=%q (directo, se queda)", rol("bea"), rol("ana"))
	}
	if a, _ := st.Anfitrion(globex, doc.ID, "bea"); a != "" {
		t.Errorf("con el enlace revocado se sigue diciendo la anfitriona: %q", a)
	}
	// El dueño se lo vuelve a dar a mano: deja de depender del enlace muerto.
	if err := st.UpsertPermission(acme, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: "bea", Role: domain.RoleCommenter, GrantedBy: "owner"}); err != nil {
		t.Fatal(err)
	}
	if rol("bea") != domain.RoleCommenter {
		t.Errorf("el permiso dado a mano sigue escondido tras el enlace revocado: %q", rol("bea"))
	}
	if a, _ := st.Anfitrion(globex, doc.ID, "bea"); a != "acme" {
		t.Errorf("tras dárselo a mano, la anfitriona de bea = %q", a)
	}

	// Quitar a alguien que entró por el enlace lo BLOQUEA: no vuelve a canjear ESE
	// enlace, y uno nuevo del dueño sí.
	otro, err := st.CreateShareLink(acme, doc.ID, domain.HashToken("tk2"), "", "owner", domain.RoleEditor, enUnMes())
	if err != nil {
		t.Fatal(err)
	}
	e2, _ := st.EnlaceParaCanjear(globex, domain.HashToken("tk2"))
	// Otro enlace vivo del mismo documento, creado ANTES de que el dueño la quite.
	if _, err := st.CreateShareLink(acme, doc.ID, domain.HashToken("tk4"), "", "owner", domain.RoleEditor, enUnMes()); err != nil {
		t.Fatal(err)
	}
	ePrevio, _ := st.EnlaceParaCanjear(globex, domain.HashToken("tk4"))
	if escrito, err := st.Canjear(acme, e2, "carla", true); err != nil || !escrito {
		t.Fatalf("canjear carla: %v %v", escrito, err)
	}
	if fuera, _ := st.DeFuera(globex, "carla", domain.ListFilter{}); len(fuera) != 1 || fuera[0].ID != doc.ID {
		t.Errorf("en la lista de carla no está el documento canjeado: %+v", fuera)
	}
	if fuera, _ := st.DeFuera(globex, "nadie", domain.ListFilter{}); len(fuera) != 0 {
		t.Errorf("a quien no canjeó se le listan documentos de fuera: %+v", fuera)
	}
	if err := st.RevokePermission(acme, doc.ID, domain.PrincipalUser, "carla"); err != nil {
		t.Fatal(err)
	}
	if rol("carla") != "" {
		t.Errorf("quitada, carla sigue con %q", rol("carla"))
	}
	if escrito, _ := st.Canjear(acme, e2, "carla", true); escrito {
		t.Error("quitada, carla vuelve a canjear el mismo enlace")
	}
	if escrito, _ := st.Canjear(acme, ePrevio, "carla", true); escrito {
		t.Error("quitada, carla vuelve a entrar por OTRO enlace que ya existía")
	}
	if a, _ := st.Anfitrion(globex, doc.ID, "carla"); a != "" {
		t.Errorf("quitada, se le sigue diciendo la anfitriona: %q", a)
	}
	if fuera, _ := st.DeFuera(globex, "carla", domain.ListFilter{}); len(fuera) != 0 {
		t.Errorf("quitada, el documento sigue en su lista: %+v", fuera)
	}
	if subs, _ := st.CanjeadosDe(acme, doc.ID, otro.ID); len(subs) != 1 || subs[0] != "carla" {
		t.Errorf("canjeados del enlace: %v", subs)
	}
	tercero, _ := st.CreateShareLink(acme, doc.ID, domain.HashToken("tk3"), "", "owner", domain.RoleCommenter, enUnMes())
	e3, _ := st.EnlaceParaCanjear(globex, domain.HashToken("tk3"))
	if escrito, _ := st.Canjear(acme, e3, "carla", true); !escrito || rol("carla") != domain.RoleCommenter {
		t.Errorf("con un enlace NUEVO, carla no entra: rol=%q (enlace %s)", rol("carla"), tercero.ID)
	}
}
