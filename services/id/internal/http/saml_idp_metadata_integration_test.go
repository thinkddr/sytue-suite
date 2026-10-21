// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// ID-02: la metadata pública del IdP SAML no acuña la clave de firma de una
// organización por una petición anónima cualquiera. Antes, quien supiera (o
// probara) un slug de organización forzaba la generación de una clave RSA y una
// fila en saml_idp_keys con una simple GET sin sesión. Ahora la clave solo se
// genera para una organización que YA registró algún SP (acto autenticado de su
// administrador), que es además el orden natural: registrar el SP y después
// descargar la metadata.
package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func clavesIdP(t *testing.T, l *liveServer, orgID string) int {
	t.Helper()
	var n int
	if err := l.st.DB().QueryRow(context.Background(),
		`SELECT count(*) FROM saml_idp_keys WHERE org_id = $1`, orgID).Scan(&n); err != nil {
		t.Fatalf("contar saml_idp_keys: %v", err)
	}
	return n
}

func (l *liveServer) metadataIdP(t *testing.T, slug string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/saml/idp/metadata?org="+slug, http.NoBody))
	return rec.Code
}

func TestMetadataIdPNoAcuñaClaveAnonima(t *testing.T) {
	l := newLiveServer(t)

	// Una organización que NO usa SAML saliente (ningún SP registrado).
	sinSAML, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	// GET anónimo ⇒ 404 y, sobre todo, NO se crea ninguna clave (ID-02).
	if code := l.metadataIdP(t, "acme"); code != http.StatusNotFound {
		t.Fatalf("metadata de una org sin SP: código=%d, quiero 404", code)
	}
	if n := clavesIdP(t, l, sinSAML.ID); n != 0 {
		t.Fatalf("ID-02: una GET anónima acuñó una clave de IdP para una org sin SAML (filas=%d)", n)
	}

	// Una organización cuyo administrador SÍ registró un SP: su metadata se sirve
	// (y la clave nace ahí, en el orden natural «registrar el SP → descargar la
	// metadata»), sin regresión para quien ya usa SAML.
	conSAML, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	if _, err := l.st.CreateSAMLServiceProvider(l.ctx, nil, domain.SAMLServiceProvider{
		OrgID: conSAML.ID, Name: "App", EntityID: "https://app.example/saml/metadata",
		ACSURL:       "https://app.example/saml/acs",
		NameIDFormat: "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress", Enabled: true,
	}); err != nil {
		t.Fatalf("CreateSAMLServiceProvider: %v", err)
	}
	if code := l.metadataIdP(t, "beta"); code != http.StatusOK {
		t.Fatalf("metadata de una org con SP registrado: código=%d, quiero 200", code)
	}
	if n := clavesIdP(t, l, conSAML.ID); n != 1 {
		t.Fatalf("la org con SP debería tener su clave de IdP (filas=%d)", n)
	}
	// Y la org sin SAML sigue sin clave: servir la de otra no le crea nada.
	if n := clavesIdP(t, l, sinSAML.ID); n != 0 {
		t.Fatalf("la org sin SAML no debe tener clave (filas=%d)", n)
	}
}
