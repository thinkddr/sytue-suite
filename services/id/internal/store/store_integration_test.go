// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de Sytue ID contra un PostgreSQL real (efímero, vía
// platform/pkg/pgtest sobre testcontainers-go). Tras el build-tag `integration`
// (no corren en el build/CI por defecto). Ejercen los flujos nucleares del IdP
// sobre la BD real: aprovisionamiento SCIM (idempotente por external_id/email),
// alta de organización y membresías con rol, flujo Authorization Code (emisión y
// consumo one-shot anti-replay), rotación de refresh tokens con detección de
// reuso (revocación de familia), bootstrap/persistencia de claves de firma JWKS
// y emisión/validación de access token con audiencia (RFC 9068), y el
// aislamiento por organización de sesiones/grants (multi-tenant).
package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwt"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

const testIssuer = "https://id.sytue.eu"

func newStore(t *testing.T) (*store.Store, context.Context) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	return store.New(h.Pool), context.Background()
}

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func seedUser(t *testing.T, ctx context.Context, st *store.Store, email string) domain.User {
	t.Helper()
	u, err := st.CreateUser(ctx, nil, domain.User{Email: email, EmailVerified: true, Name: "Ada"})
	if err != nil {
		t.Fatalf("CreateUser(%q): %v", email, err)
	}
	return u
}

// TestSCIMProvisioning cubre el aprovisionamiento SCIM idempotente: alta por
// external_id, deduplicación por email (unique index case-insensitive), lookup y
// paginación con filtro userName eq, y desaprovisionamiento (borrado lógico).
func TestSCIMProvisioning(t *testing.T) {
	st, ctx := newStore(t)

	u := domain.User{Email: "Ada@sytue.eu", ExternalID: "scim-ext-1", Name: "Ada Lovelace"}
	created, err := st.CreateUser(ctx, nil, u)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.Email != "ada@sytue.eu" {
		t.Fatalf("email no normalizado a minúsculas: %q", created.Email)
	}

	// Reaprovisionar con el mismo email (distinto casing) debe entrar en conflicto.
	if _, err := st.CreateUser(ctx, nil, domain.User{Email: "ADA@sytue.eu"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("alta duplicada por email = %v, quiero ErrConflict", err)
	}
	// Y por external_id.
	if _, err := st.CreateUser(ctx, nil, domain.User{Email: "otra@sytue.eu", ExternalID: "scim-ext-1"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("alta duplicada por external_id = %v, quiero ErrConflict", err)
	}

	// Lookup por external_id (idempotencia del proveedor SCIM).
	byExt, err := st.GetUserByExternalID(ctx, "scim-ext-1")
	if err != nil || byExt.ID != created.ID {
		t.Fatalf("GetUserByExternalID: %v (%q vs %q)", err, byExt.ID, created.ID)
	}

	// Paginación + filtro userName eq (SCIM ListResponse).
	seedUser(t, ctx, st, "bob@sytue.eu")
	users, total, err := st.ListUsers(ctx, "ada@sytue.eu", 0, 50)
	if err != nil {
		t.Fatalf("ListUsers(filter): %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != created.ID {
		t.Fatalf("filtro userName eq => total=%d len=%d", total, len(users))
	}
	all, totalAll, err := st.ListUsers(ctx, "", 0, 50)
	if err != nil || totalAll != 2 || len(all) != 2 {
		t.Fatalf("ListUsers(all) => total=%d len=%d err=%v", totalAll, len(all), err)
	}

	// Desaprovisionamiento: borrado lógico preservando la fila (auditoría).
	if err := st.DeleteUser(ctx, nil, created.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	got, err := st.GetUserByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetUserByID tras delete: %v", err)
	}
	if got.Status != domain.StatusDeleted {
		t.Fatalf("borrado lógico: status=%q", got.Status)
	}
}

// TestOrgMembershipTenancy cubre alta de organización (tenant) y membresías con
// rol, incluyendo el upsert de rol y el aislamiento de un usuario que no pertenece
// a la organización.
func TestOrgMembershipTenancy(t *testing.T) {
	st, ctx := newStore(t)

	org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	if _, err := st.GetOrgBySlug(ctx, "acme"); err != nil {
		t.Fatalf("GetOrgBySlug: %v", err)
	}
	// Slug único.
	if _, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Otra"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("slug duplicado = %v, quiero ErrConflict", err)
	}

	owner := seedUser(t, ctx, st, "owner@sytue.eu")
	member := seedUser(t, ctx, st, "member@sytue.eu")
	outsider := seedUser(t, ctx, st, "outsider@sytue.eu")

	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: org.ID, UserID: owner.ID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	// Upsert: promover a owner no duplica, actualiza el rol.
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: org.ID, UserID: owner.ID, Role: domain.RoleOwner}); err != nil {
		t.Fatalf("UpsertMembership(promote): %v", err)
	}
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: org.ID, UserID: member.ID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("UpsertMembership(member): %v", err)
	}

	roles, err := st.RolesForUserInOrg(ctx, org.ID, owner.ID)
	if err != nil || len(roles) != 1 || roles[0] != string(domain.RoleOwner) {
		t.Fatalf("RolesForUserInOrg(owner) => %v (%v)", roles, err)
	}

	// Aislamiento: un usuario ajeno no tiene membresía en la organización.
	if _, err := st.GetMembership(ctx, org.ID, outsider.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetMembership(outsider) = %v, quiero ErrNotFound", err)
	}

	mships, err := st.ListMembershipsForUser(ctx, member.ID)
	if err != nil || len(mships) != 1 || mships[0].OrgID != org.ID {
		t.Fatalf("ListMembershipsForUser(member) => %v (%v)", mships, err)
	}
}

// TestListOrgUsersPageKeyset cubre el listado admin de usuarios por organización
// con paginación keyset (created_at, id) DESC (índice 0007): orden estable con
// desempate por id en empates de created_at, avance por cursor sin OFFSET,
// more=false al agotar y aislamiento por org (los miembros de otra org no salen).
func TestListOrgUsersPageKeyset(t *testing.T) {
	st, ctx := newStore(t)

	org, _ := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	otra, _ := st.CreateOrg(ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})

	base := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	mk := func(email string, at time.Time, o domain.Organization) domain.User {
		t.Helper()
		u := seedUser(t, ctx, st, email)
		// created_at determinista (con empates provocados) para fijar el orden.
		if _, err := st.DB().Exec(ctx, `UPDATE users SET created_at=$2 WHERE id=$1`, u.ID, at); err != nil {
			t.Fatalf("fijar created_at(%s): %v", email, err)
		}
		u.CreatedAt = at
		if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: o.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
			t.Fatalf("UpsertMembership(%s): %v", email, err)
		}
		return u
	}
	u1 := mk("u1@sytue.eu", base.Add(1*time.Minute), org)
	u2 := mk("u2@sytue.eu", base.Add(2*time.Minute), org)
	u3 := mk("u3@sytue.eu", base.Add(3*time.Minute), org) // empate con u4
	u4 := mk("u4@sytue.eu", base.Add(3*time.Minute), org) // empate con u3
	mk("ajena@sytue.eu", base.Add(9*time.Minute), otra)   // no debe aparecer

	// Con empate de created_at desempata id DESC.
	hi, lo := u3, u4
	if u4.ID > u3.ID {
		hi, lo = u4, u3
	}
	want := []string{hi.ID, lo.ID, u2.ID, u1.ID}

	// Página 1 (limit 3): los tres más recientes y more=true.
	p1, more, err := st.ListOrgUsersPage(ctx, org.ID, "", nil, 3)
	if err != nil || !more || len(p1) != 3 {
		t.Fatalf("página 1 = (%d, more=%v, %v), quiero (3, true, nil)", len(p1), more, err)
	}
	for i, w := range want[:3] {
		if p1[i].ID != w {
			t.Fatalf("página 1 [%d] = %s, quiero %s", i, p1[i].ID, w)
		}
	}
	if p1[0].Role != string(domain.RoleMember) || p1[0].Email == "" {
		t.Fatalf("fila sin rol/email: %+v", p1[0])
	}

	// Página 2 desde el cursor (created_at, id) del último visto: el resto, sin
	// duplicados ni huecos, y more=false al agotar.
	last := p1[len(p1)-1]
	p2, more, err := st.ListOrgUsersPage(ctx, org.ID, "", &store.UserCursor{CreatedAt: last.CreatedAt, ID: last.ID}, 3)
	if err != nil || more || len(p2) != 1 || p2[0].ID != want[3] {
		t.Fatalf("página 2 = (%d, more=%v, %v), quiero solo %s", len(p2), more, err, want[3])
	}

	// La org ajena solo ve a su miembro (aislamiento multi-tenant).
	pa, more, err := st.ListOrgUsersPage(ctx, otra.ID, "", nil, 50)
	if err != nil || more || len(pa) != 1 || pa[0].Email != "ajena@sytue.eu" {
		t.Fatalf("org ajena = (%d, more=%v, %v)", len(pa), more, err)
	}
}

// TestListOrgUsersPageBusqueda cubre el filtro `q` (mig 0044). Lo que se prueba
// no es que encuentre —eso se ve enseguida—, sino las tres formas de que el
// buscador mienta sin que nada falle: que se le escape alguien de otra página,
// que un '%' en el término devuelva a todo el mundo, y que la búsqueda cruce la
// frontera de la organización.
func TestListOrgUsersPageBusqueda(t *testing.T) {
	st, ctx := newStore(t)

	org, _ := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	otra, _ := st.CreateOrg(ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	base := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	mk := func(email, nombre string, at time.Time, o domain.Organization) domain.User {
		t.Helper()
		u := seedUser(t, ctx, st, email)
		if _, err := st.DB().Exec(ctx,
			`UPDATE users SET created_at=$2, name=$3 WHERE id=$1`, u.ID, at, nombre); err != nil {
			t.Fatalf("fijar fila(%s): %v", email, err)
		}
		if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: o.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
			t.Fatalf("UpsertMembership(%s): %v", email, err)
		}
		return u
	}
	// Rellena 30 filas MÁS RECIENTES que la buscada: sin filtro en el servidor,
	// «marisa» no aparece hasta la tercera página y el buscador de la consola
	// —que solo mira lo cargado— responde que no existe.
	buscada := mk("marisa.perez@acme.eu", "Marisa Pérez", base, org)
	for i := range 30 {
		mk(fmt.Sprintf("relleno%02d@acme.eu", i), "Relleno", base.Add(time.Duration(i+1)*time.Minute), org)
	}
	mk("marisa.ajena@otra.eu", "Marisa Ajena", base.Add(time.Hour), otra)

	// Por apellido y por trozo de correo, en la PRIMERA página pese a ser la más
	// antigua de las treinta y una.
	for _, term := range []string{"pérez", "MARISA.PEREZ@ACME", "isa.per"} {
		got, more, err := st.ListOrgUsersPage(ctx, org.ID, term, nil, 10)
		if err != nil {
			t.Fatalf("q=%q: %v", term, err)
		}
		if len(got) != 1 || got[0].ID != buscada.ID {
			t.Errorf("q=%q devolvió %d filas (more=%v); quiero solo a la buscada", term, len(got), more)
		}
	}

	// Un '%' es texto, no comodín: si se colara sin escapar, esto devolvería las
	// treinta y una filas de la organización.
	got, _, err := st.ListOrgUsersPage(ctx, org.ID, "%", nil, 50)
	if err != nil {
		t.Fatalf("q=%%: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("q=%%%% devolvió %d filas: el término no se está escapando", len(got))
	}

	// Y la búsqueda no salta a otra organización aunque el nombre coincida.
	got, _, err = st.ListOrgUsersPage(ctx, org.ID, "marisa", nil, 50)
	if err != nil {
		t.Fatalf("q=marisa: %v", err)
	}
	if len(got) != 1 || got[0].ID != buscada.ID {
		t.Errorf("q=marisa devolvió %d filas: la búsqueda cruza la frontera de la organización", len(got))
	}
}

// TestAuthCodeOneShot cubre el flujo Authorization Code: emisión del código
// (persistido solo como hash) y su consumo atómico one-shot, que rechaza un
// segundo intento (anti-replay) y los códigos expirados.
func TestAuthCodeOneShot(t *testing.T) {
	st, ctx := newStore(t)
	org, _ := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	user := seedUser(t, ctx, st, "ada@sytue.eu")
	now := time.Now().UTC()

	code := domain.AuthCode{
		CodeHash:            hashOf("secret-code"),
		ClientID:            "sytue-one",
		UserID:              user.ID,
		OrgID:               org.ID,
		RedirectURI:         "https://one.sytue.eu/cb",
		Scope:               "openid profile",
		CodeChallenge:       "chal",
		CodeChallengeMethod: "S256",
		AuthTime:            now,
		ExpiresAt:           now.Add(2 * time.Minute),
	}
	if err := st.InsertAuthCode(ctx, nil, code); err != nil {
		t.Fatalf("InsertAuthCode: %v", err)
	}

	// Consumo dentro de transacción: primera vez OK.
	var consumed domain.AuthCode
	err := st.InTx(ctx, func(q pg.Querier) error {
		var e error
		consumed, e = st.ConsumeAuthCode(ctx, q, code.CodeHash, now.Add(time.Second))
		return e
	})
	if err != nil {
		t.Fatalf("ConsumeAuthCode(1): %v", err)
	}
	if consumed.UserID != user.ID || consumed.OrgID != org.ID {
		t.Fatalf("código consumido con datos incorrectos: %+v", consumed)
	}

	// Segundo consumo: rechazado (ya consumido) => ErrNotFound.
	err = st.InTx(ctx, func(q pg.Querier) error {
		_, e := st.ConsumeAuthCode(ctx, q, code.CodeHash, now.Add(2*time.Second))
		return e
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("segundo consumo = %v, quiero ErrNotFound (anti-replay)", err)
	}

	// Código expirado: rechazado.
	expired := code
	expired.CodeHash = hashOf("expired-code")
	expired.ExpiresAt = now.Add(-time.Minute)
	if err := st.InsertAuthCode(ctx, nil, expired); err != nil {
		t.Fatalf("InsertAuthCode(expired): %v", err)
	}
	err = st.InTx(ctx, func(q pg.Querier) error {
		_, e := st.ConsumeAuthCode(ctx, q, expired.CodeHash, now)
		return e
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("consumo de código expirado = %v, quiero ErrNotFound", err)
	}
}

// TestRefreshTokenRotationAndReuse cubre la rotación de refresh tokens y la
// detección de reuso: al reutilizar un token ya rotado se revoca toda la familia
// (chain) para cortar el replay (RFC 6819 / OAuth security BCP).
func TestRefreshTokenRotationAndReuse(t *testing.T) {
	st, ctx := newStore(t)
	user := seedUser(t, ctx, st, "ada@sytue.eu")
	now := time.Now().UTC()

	rt1 := domain.RefreshToken{
		TokenHash: hashOf("rt-1"),
		ClientID:  "sytue-one",
		UserID:    user.ID,
		Scope:     "openid offline_access",
		AuthTime:  now,
		ExpiresAt: now.Add(24 * time.Hour),
	}
	id1, err := st.InsertRefreshToken(ctx, nil, rt1)
	if err != nil {
		t.Fatalf("InsertRefreshToken(1): %v", err)
	}

	// Rotación: se emite un sucesor y se enlaza el antiguo (revocado + rotated_to).
	rt2 := rt1
	rt2.TokenHash = hashOf("rt-2")
	var id2 string
	err = st.InTx(ctx, func(q pg.Querier) error {
		var e error
		id2, e = st.InsertRefreshToken(ctx, q, rt2)
		if e != nil {
			return e
		}
		return st.RotateRefreshToken(ctx, q, id1, id2)
	})
	if err != nil {
		t.Fatalf("rotación: %v", err)
	}

	// El token viejo ya está revocado.
	old, err := st.GetRefreshToken(ctx, nil, rt1.TokenHash)
	if err != nil {
		t.Fatalf("GetRefreshToken(old): %v", err)
	}
	if old.RevokedAt == nil {
		t.Fatal("el refresh token rotado debería estar revocado")
	}
	if old.RotatedTo != id2 {
		t.Fatalf("rotated_to = %q, quiero %q", old.RotatedTo, id2)
	}

	// Reuso detectado: revocar toda la familia desde la raíz.
	if err := st.RevokeRefreshChain(ctx, nil, id1); err != nil {
		t.Fatalf("RevokeRefreshChain: %v", err)
	}
	succ, err := st.GetRefreshToken(ctx, nil, rt2.TokenHash)
	if err != nil {
		t.Fatalf("GetRefreshToken(succ): %v", err)
	}
	if succ.RevokedAt == nil {
		t.Fatal("el sucesor debería quedar revocado tras detectar reuso de la familia")
	}
}

// TestSigningKeysAndTokenAudience cubre el ciclo completo de claves de firma y de
// tokens: bootstrap persiste una clave RSA activa, el Issuer emite un access token
// con la audiencia del resource server, y Verify lo acepta con la audiencia correcta
// y lo rechaza con otra (RFC 9068 §4). También valida la persistencia del JWKS.
func TestSigningKeysAndTokenAudience(t *testing.T) {
	st, ctx := newStore(t)

	km := oidc.NewKeyManager()
	if err := km.Bootstrap(ctx, st); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	// La clave se persistió y hay exactamente una activa.
	if n, err := st.CountSigningKeys(ctx); err != nil || n != 1 {
		t.Fatalf("CountSigningKeys => %d (%v)", n, err)
	}

	set, err := km.PublicJWKS()
	if err != nil {
		t.Fatalf("PublicJWKS: %v", err)
	}
	if set.Len() != 1 {
		t.Fatalf("JWKS público tiene %d claves, quiero 1", set.Len())
	}

	const resourceAud = "https://id.sytue.eu/api"
	iss := oidc.NewIssuer(km, testIssuer, resourceAud, 15*time.Minute, time.Hour)
	now := time.Now().UTC()
	raw, err := iss.IssueAccessToken(oidc.TokenClaims{
		Subject:  "user-123",
		Audience: "sytue-one",
		Scope:    "openid profile",
		Tenant:   "acme",
		Roles:    []string{"owner"},
	}, now)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	// Validación con la audiencia del resource server: aceptado.
	tok, err := oidc.Verify(raw, testIssuer, resourceAud, set)
	if err != nil {
		t.Fatalf("Verify(aud correcta): %v", err)
	}
	if tok.Subject() != "user-123" {
		t.Fatalf("subject = %q", tok.Subject())
	}
	if tenant, ok := tok.Get("tenant"); !ok || tenant != "acme" {
		t.Fatalf("claim tenant = %v (ok=%v)", tenant, ok)
	}

	// Validación con audiencia ajena: rechazado.
	if _, err := oidc.Verify(raw, testIssuer, "https://otro.sytue.eu/api", set); !errors.Is(err, oidc.ErrInvalidToken) {
		t.Fatalf("Verify(aud ajena) = %v, quiero ErrInvalidToken", err)
	}
	// Issuer distinto: rechazado.
	if _, err := oidc.Verify(raw, "https://malicioso.eu", resourceAud, set); !errors.Is(err, oidc.ErrInvalidToken) {
		t.Fatalf("Verify(issuer ajeno) = %v, quiero ErrInvalidToken", err)
	}

	// Tras rotación, el JWKS publica dos claves (activa + next) y un token nuevo
	// sigue validando contra el conjunto recargado.
	if err := km.Rotate(ctx, st); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	set2, _ := km.PublicJWKS()
	if set2.Len() != 2 {
		t.Fatalf("tras rotación JWKS tiene %d claves, quiero 2", set2.Len())
	}
	raw2, err := iss.IssueAccessToken(oidc.TokenClaims{Subject: "user-123", Audience: "sytue-one"}, time.Now().UTC())
	if err != nil {
		t.Fatalf("IssueAccessToken(post-rot): %v", err)
	}
	if _, err := oidc.Verify(raw2, testIssuer, resourceAud, set2); err != nil {
		t.Fatalf("Verify(post-rotación): %v", err)
	}
	// El token emitido antes de la rotación sigue validando (clave 'next' en JWKS).
	if _, err := oidc.Verify(raw, testIssuer, resourceAud, set2); err != nil {
		t.Fatalf("Verify(token pre-rotación tras rotar): %v", err)
	}
	_ = jwt.Token(tok)
}

// TestSessionAndAuditPersistence cubre la persistencia de sesión SSO (por hash),
// su revocación (logout) y el registro de auditoría append-only asociado a la org.
func TestSessionAndAuditPersistence(t *testing.T) {
	st, ctx := newStore(t)
	org, _ := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	user := seedUser(t, ctx, st, "ada@sytue.eu")
	now := time.Now().UTC()

	sid, err := st.InsertSession(ctx, nil, domain.Session{
		TokenHash: hashOf("sess-1"),
		UserID:    user.ID,
		OrgID:     org.ID,
		AuthTime:  now,
		AMR:       []string{"pwd", "mfa"},
		ExpiresAt: now.Add(8 * time.Hour),
	})
	if err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	if sid == "" {
		t.Fatal("InsertSession no devolvió id")
	}

	ss, err := st.GetSessionByToken(ctx, hashOf("sess-1"))
	if err != nil {
		t.Fatalf("GetSessionByToken: %v", err)
	}
	if ss.UserID != user.ID || ss.OrgID != org.ID || len(ss.AMR) != 2 {
		t.Fatalf("sesión recuperada incorrecta: %+v", ss)
	}

	if err := st.RevokeSession(ctx, nil, hashOf("sess-1")); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	revoked, _ := st.GetSessionByToken(ctx, hashOf("sess-1"))
	if revoked.RevokedAt == nil {
		t.Fatal("la sesión debería quedar revocada tras logout")
	}

	// Auditoría append-only ligada a la organización.
	if err := st.Audit(ctx, nil, store.AuditEntry{
		ActorID: user.ID,
		OrgID:   org.ID,
		Action:  "session.login",
		Target:  sid,
		Detail:  map[string]any{"amr": "pwd+mfa"},
	}); err != nil {
		t.Fatalf("Audit: %v", err)
	}
}

// TestSessionDevicesListAndRevoke cubre el self-service de dispositivos de
// /account: listado de sesiones vigentes con marca de sesión actual y orden por
// auth_time desc, revocación por id con guard de usuario (anti-IDOR) y cierre
// de "las demás" — ambas con cascada sobre los refresh tokens de cada sesión
// (encadenan RevokeRefreshTokensForSession).
func TestSessionDevicesListAndRevoke(t *testing.T) {
	st, ctx := newStore(t)
	ada := seedUser(t, ctx, st, "ada@sytue.eu")
	eve := seedUser(t, ctx, st, "eve@sytue.eu")
	now := time.Now().UTC()

	mkSession := func(tok, userID string, at time.Time) string {
		t.Helper()
		id, err := st.InsertSession(ctx, nil, domain.Session{
			TokenHash: hashOf(tok), UserID: userID, AuthTime: at,
			AMR: []string{"pwd"}, UserAgent: "e2e/" + tok, IP: "10.0.0.9",
			ExpiresAt: now.Add(8 * time.Hour),
		})
		if err != nil {
			t.Fatalf("InsertSession(%s): %v", tok, err)
		}
		return id
	}
	mkRefresh := func(tok, userID, sessionID string) string {
		t.Helper()
		// "sytue-one" existe por la migración 0002 (seed de clients).
		id, err := st.InsertRefreshToken(ctx, nil, domain.RefreshToken{
			TokenHash: hashOf(tok), ClientID: "sytue-one", UserID: userID,
			SessionID: sessionID, AuthTime: now, ExpiresAt: now.Add(24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("InsertRefreshToken(%s): %v", tok, err)
		}
		return id
	}

	actual := mkSession("ada-actual", ada.ID, now)
	movil := mkSession("ada-movil", ada.ID, now.Add(-time.Hour))
	tablet := mkSession("ada-tablet", ada.ID, now.Add(-2*time.Hour))
	mkSession("eve-1", eve.ID, now)
	rtMovil := mkRefresh("rt-movil", ada.ID, movil)
	rtTablet := mkRefresh("rt-tablet", ada.ID, tablet)

	// Listado: solo las de ada, más reciente primero, la actual marcada por
	// token_hash y sin exponer el hash.
	list, err := st.ListSessionsForUser(ctx, ada.ID, hashOf("ada-actual"))
	if err != nil {
		t.Fatalf("ListSessionsForUser: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(list) = %d, quiero 3 (sin sesiones de otros usuarios)", len(list))
	}
	if list[0].ID != actual || !list[0].Current {
		t.Fatalf("la primera debería ser la actual marcada: %+v", list[0])
	}
	if list[1].ID != movil || list[1].Current || list[2].ID != tablet {
		t.Fatalf("orden/marca incorrectos: %+v %+v", list[1], list[2])
	}
	// La IP NO se persiste (mig 0039): en la base no queda nada legible —ni la
	// dirección, ni el lugar, ni el token—, y lo que se guarda es la ubicación
	// aproximada cifrada con la clave derivada del token de ESA sesión. Así que
	// aquí `IP` tiene que venir VACÍA: si algún día vuelve con valor, es que la
	// dirección ha vuelto a la base de datos y este test es el que debe avisar.
	if list[0].TokenHash != "" || list[0].IP != "" || list[0].UserAgent == "" {
		t.Fatalf("campos expuestos incorrectos: %+v", list[0])
	}

	// Guard anti-IDOR: eve no puede revocar la sesión de ada (no-op sin error) y
	// el refresh de esa sesión sigue vivo.
	if ok, err := st.RevokeSessionByID(ctx, nil, eve.ID, movil); err != nil || ok {
		t.Fatalf("RevokeSessionByID(ajena) = (%v, %v), quiero (false, nil)", ok, err)
	}
	if ss, _ := st.GetSessionByToken(ctx, hashOf("ada-movil")); ss.RevokedAt != nil {
		t.Fatal("la sesión ajena no debe quedar revocada")
	}

	// Revocación propia por id: sesión revocada + refresh en cascada.
	err = st.InTx(ctx, func(q pg.Querier) error {
		ok, e := st.RevokeSessionByID(ctx, q, ada.ID, movil)
		if e == nil && !ok {
			t.Fatal("RevokeSessionByID(propia) no revocó")
		}
		return e
	})
	if err != nil {
		t.Fatalf("RevokeSessionByID: %v", err)
	}
	if ss, _ := st.GetSessionByToken(ctx, hashOf("ada-movil")); ss.RevokedAt == nil {
		t.Fatal("la sesión propia debería quedar revocada")
	}
	if rt, _ := st.GetRefreshToken(ctx, nil, hashOf("rt-movil")); rt.RevokedAt == nil {
		t.Fatalf("el refresh %s debería caer en cascada con su sesión", rtMovil)
	}
	// Repetir sobre una sesión ya revocada: no-op.
	if ok, err := st.RevokeSessionByID(ctx, nil, ada.ID, movil); err != nil || ok {
		t.Fatalf("RevokeSessionByID(repetida) = (%v, %v), quiero (false, nil)", ok, err)
	}

	// Cerrar las demás: cae tablet (y su refresh), sobrevive la actual y la de eve.
	var n int
	err = st.InTx(ctx, func(q pg.Querier) error {
		var e error
		n, e = st.RevokeOtherSessions(ctx, q, ada.ID, actual)
		return e
	})
	if err != nil || n != 1 {
		t.Fatalf("RevokeOtherSessions = (%d, %v), quiero (1, nil)", n, err)
	}
	if ss, _ := st.GetSessionByToken(ctx, hashOf("ada-actual")); ss.RevokedAt != nil {
		t.Fatal("la sesión actual debe sobrevivir a 'cerrar las demás'")
	}
	if ss, _ := st.GetSessionByToken(ctx, hashOf("ada-tablet")); ss.RevokedAt == nil {
		t.Fatal("la tablet debería quedar revocada")
	}
	if rt, _ := st.GetRefreshToken(ctx, nil, hashOf("rt-tablet")); rt.RevokedAt == nil {
		t.Fatalf("el refresh %s debería caer en cascada", rtTablet)
	}
	if ss, _ := st.GetSessionByToken(ctx, hashOf("eve-1")); ss.RevokedAt != nil {
		t.Fatal("las sesiones de otros usuarios no deben tocarse")
	}
	// El listado ya solo devuelve la actual.
	list, err = st.ListSessionsForUser(ctx, ada.ID, hashOf("ada-actual"))
	if err != nil || len(list) != 1 || !list[0].Current {
		t.Fatalf("listado tras revocar = (%d, %v)", len(list), err)
	}
}
