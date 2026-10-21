// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package authn

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lestrrat-go/jwx/v2/jwt"
)

// Los claims del plan son lo que Drive y Mail usan para fijar la cuota de una
// cuenta personal, y lo que cierra SyERP y Sytue Admin. Si se leen mal, una
// cuenta de 5 GB podría tener el defecto del servicio (10 GB) o entrar donde no
// debe, sin que nada avise.
func TestClaimsDelPlanSeLeenDelToken(t *testing.T) {
	tok := jwt.New()
	_ = tok.Set(jwt.SubjectKey, "u1")
	_ = tok.Set("tenant", "p-0123456789abcdef")
	_ = tok.Set("org_kind", "personal")
	_ = tok.Set("plan", "personal_free")
	// jwx entrega los números como float64 al deserializar JSON: es el caso real.
	_ = tok.Set("storage_bytes", float64(5<<30))
	_ = tok.Set("mailbox_bytes", int64(1<<30))

	c := claimsFromToken(tok)
	if !c.Personal() || c.OrgKind != "personal" || c.Plan != "personal_free" {
		t.Fatalf("clase/plan: %+v", c)
	}
	if c.StorageBytes != 5<<30 || c.MailboxBytes != 1<<30 {
		t.Fatalf("cuotas: storage=%d mailbox=%d", c.StorageBytes, c.MailboxBytes)
	}

	// Un token anterior a la distinción no es personal, y no trae cuota.
	viejo := jwt.New()
	_ = viejo.Set(jwt.SubjectKey, "u2")
	_ = viejo.Set("tenant", "acme")
	if c := claimsFromToken(viejo); c.Personal() || c.StorageBytes != 0 {
		t.Fatalf("un token sin claims de plan no puede ser personal ni traer cuota: %+v", c)
	}
}

// Los grupos del token son los que resuelven una ACL por grupo (Sytue Bases): si se
// leyeran mal, un permiso de grupo no concedería nada o, peor, concedería de más.
func TestClaimGroupsSeLeeDelToken(t *testing.T) {
	tok := jwt.New()
	_ = tok.Set(jwt.SubjectKey, "u1")
	_ = tok.Set("groups", []any{"ventas", "", "dirección"}) // así llega tras deserializar JSON
	if c := claimsFromToken(tok); len(c.Groups) != 2 || c.Groups[0] != "ventas" || c.Groups[1] != "dirección" {
		t.Fatalf("groups: %#v", c.Groups)
	}
	sin := jwt.New()
	_ = sin.Set(jwt.SubjectKey, "u2")
	_ = sin.Set("roles", []string{"admin"})
	if c := claimsFromToken(sin); len(c.Groups) != 0 {
		t.Fatalf("sin claim no hay grupos, y los roles no lo son: %#v", c.Groups)
	}
}

func TestRequireOrgKind(t *testing.T) {
	v := &Verifier{}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	soloOrgs := v.RequireOrgKind("org")(ok)

	sirve := func(c Claims) int {
		r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		r = r.WithContext(Into(r.Context(), c))
		rr := httptest.NewRecorder()
		soloOrgs.ServeHTTP(rr, r)
		return rr.Code
	}
	if got := sirve(Claims{Subject: "u", OrgKind: "personal", Roles: []string{"owner"}}); got != http.StatusForbidden {
		// Es owner de su propia organización personal: por rol entraría. Por eso
		// existe esta verja.
		t.Fatalf("una cuenta personal debe recibir 403 aunque sea owner: %d", got)
	}
	if got := sirve(Claims{Subject: "u", OrgKind: "org"}); got != http.StatusNoContent {
		t.Fatalf("una organización entra: %d", got)
	}
	// Sin el claim = anterior a la distinción = organización.
	if got := sirve(Claims{Subject: "u"}); got != http.StatusNoContent {
		t.Fatalf("un token viejo no debe quedarse fuera: %d", got)
	}
	// Sin claims en el contexto: no autenticado.
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()
	soloOrgs.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("sin claims: %d", rr.Code)
	}
}
