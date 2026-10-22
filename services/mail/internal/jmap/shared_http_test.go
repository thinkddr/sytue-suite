// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de los handlers de buzones compartidos + delegación (fake store, sin
// PostgreSQL). Cubren: listar accesibles, comprobación de ACL deny-by-default en
// lectura, gestión del ACL gated por `manage`, delegación del propio buzón, y la
// verificación server-side del envío-como (buzón compartido) / envío-en-nombre-de
// (delegación, con Sender sellado).
package jmap

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// --- listar buzones accesibles -------------------------------------------

func TestListSharedMailboxesOK(t *testing.T) {
	st := &fakeStore{accessible: []domain.SharedMailbox{
		{AccountID: uuid.New(), Address: "soporte@sytue.eu", Kind: domain.KindShared, Permission: domain.PermSend},
	}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listSharedMailboxes(rec, req(http.MethodGet, "/api/v1/shared-mailboxes", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	m := decodeBody(t, rec)
	items, ok := m["shared_mailboxes"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("shared_mailboxes = %+v", m["shared_mailboxes"])
	}
}

func TestListSharedMailboxesNoAuth(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.listSharedMailboxes(rec, noAuthReq(http.MethodGet, "/api/v1/shared-mailboxes", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth => %d, quiero 401", rec.Code)
	}
}

// --- lectura gated por ACL (deny-by-default) -----------------------------

func TestListSharedFoldersDenyWithoutGrant(t *testing.T) {
	// Sin fila de ACL (GranteePermission => ErrNotFound) NO hay acceso: 403.
	st := &fakeStore{granteeErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listSharedFolders(rec, req(http.MethodGet, "/x", "", true, map[string]string{"accountID": uuid.NewString()}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sin ACL => %d, quiero 403", rec.Code)
	}
}

func TestListSharedFoldersOK(t *testing.T) {
	st := &fakeStore{granteePerm: domain.PermRead, mailboxes: []domain.Mailbox{{ID: uuid.New(), Name: "Inbox"}}}
	a := newAPI(st, nil, nil)
	acct := uuid.New()
	rec := httptest.NewRecorder()
	a.listSharedFolders(rec, req(http.MethodGet, "/x", "", true, map[string]string{"accountID": acct.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if st.lastGranteeAcct != acct {
		t.Fatalf("ACL comprobada sobre %v, quiero %v", st.lastGranteeAcct, acct)
	}
}

func TestListSharedMessagesRejectsForeignMailbox(t *testing.T) {
	// Grantee con read sobre la cuenta, pero el mailbox NO pertenece a ella:
	// no puede leer una carpeta de otra cuenta pasando su id. 404.
	st := &fakeStore{granteePerm: domain.PermRead, mbInAccount: false}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listSharedMessages(rec, req(http.MethodGet, "/x", "", true, map[string]string{
		"accountID": uuid.NewString(), "mailboxID": uuid.NewString(),
	}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("mailbox ajeno => %d, quiero 404", rec.Code)
	}
}

func TestListSharedMessagesOK(t *testing.T) {
	st := &fakeStore{
		granteePerm: domain.PermRead, mbInAccount: true,
		messages: []domain.Message{{ID: uuid.New(), Subject: "ticket"}},
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listSharedMessages(rec, req(http.MethodGet, "/x?limit=10", "", true, map[string]string{
		"accountID": uuid.NewString(), "mailboxID": uuid.NewString(),
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
}

// --- gestión del ACL: solo `manage` ---------------------------------------

func TestListGrantsRequiresManage(t *testing.T) {
	st := &fakeStore{granteePerm: domain.PermSend} // send < manage
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listGrants(rec, req(http.MethodGet, "/x", "", true, map[string]string{"accountID": uuid.NewString()}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("send no gestiona ACL => %d, quiero 403", rec.Code)
	}
}

func TestPostGrantOK(t *testing.T) {
	st := &fakeStore{granteePerm: domain.PermManage}
	a := newAPI(st, nil, nil)
	acct := uuid.New()
	rec := httptest.NewRecorder()
	a.postGrant(rec, req(http.MethodPost, "/x", `{"grantee":"ben@sytue.eu","permission":"send"}`, true,
		map[string]string{"accountID": acct.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastGrantAcct != acct || st.lastGrantee != "ben@sytue.eu" || st.lastGrantPerm != domain.PermSend {
		t.Fatalf("grant no propagado: %v %q %q", st.lastGrantAcct, st.lastGrantee, st.lastGrantPerm)
	}
}

func TestPostGrantInvalidPermission(t *testing.T) {
	st := &fakeStore{granteePerm: domain.PermManage}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.postGrant(rec, req(http.MethodPost, "/x", `{"grantee":"ben@sytue.eu","permission":"root"}`, true,
		map[string]string{"accountID": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("permiso inválido => %d, quiero 400", rec.Code)
	}
}

func TestPostGrantRevoke(t *testing.T) {
	st := &fakeStore{granteePerm: domain.PermManage}
	a := newAPI(st, nil, nil)
	acct := uuid.New()
	rec := httptest.NewRecorder()
	a.postGrant(rec, req(http.MethodPost, "/x", `{"grantee":"ben@sytue.eu","delete":true}`, true,
		map[string]string{"accountID": acct.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
	if st.lastRevokeAcct != acct || st.lastRevokee != "ben@sytue.eu" {
		t.Fatalf("revoke no propagado: %v %q", st.lastRevokeAcct, st.lastRevokee)
	}
}

func TestPostGrantForbiddenForReader(t *testing.T) {
	st := &fakeStore{granteePerm: domain.PermRead}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.postGrant(rec, req(http.MethodPost, "/x", `{"grantee":"ben@sytue.eu","permission":"read"}`, true,
		map[string]string{"accountID": uuid.NewString()}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read no gestiona ACL => %d, quiero 403", rec.Code)
	}
	if st.lastGrantee != "" {
		t.Fatalf("no debe llegar al store: %q", st.lastGrantee)
	}
}

// --- delegación del propio buzón (dueño ⇒ manage implícito) ---------------

func TestPostDelegationOwnAccount(t *testing.T) {
	own := domain.Account{ID: uuid.New(), Address: "dir@sytue.eu", Subject: "dir-sub"}
	st := &fakeStore{acc: own}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.postDelegation(rec, req(http.MethodPost, "/api/v1/delegation", `{"grantee":"asst@sytue.eu","permission":"send"}`, true, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204 (%s)", rec.Code, rec.Body.String())
	}
	// El grant se aplica sobre MI cuenta, sin comprobación de ACL (soy el dueño).
	if st.lastGrantAcct != own.ID || st.lastGrantee != "asst@sytue.eu" {
		t.Fatalf("delegación no sobre la propia cuenta: %v %q", st.lastGrantAcct, st.lastGrantee)
	}
}

// --- crear buzón compartido ----------------------------------------------

// adminDeAcme son las claims de quien administra la organización: lo único que crea
// buzones compartidos.
var adminDeAcme = authn.Claims{Subject: "user-1", Email: "ada@acme.sytue.eu", Name: "Ada", Tenant: "acme", Roles: []string{"admin"}}

func crearCompartido(a *API, c authn.Claims, address string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	a.createSharedMailbox(rec, conClaims(req(http.MethodPost, "/api/v1/shared-mailboxes",
		`{"address":"`+address+`","display_name":"Soporte"}`, true, nil), c))
	return rec
}

func TestCreateSharedMailboxOK(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@acme.sytue.eu", Subject: "ada-sub", Tenant: "acme"}}
	a := newAPI(st, nil, nil)
	a.cfg.PrimaryDomain = "sytue.eu"
	rec := crearCompartido(a, adminDeAcme, "soporte@acme.sytue.eu")
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastSharedAddr != "soporte@acme.sytue.eu" || st.lastOwnerSubj != "ada-sub" {
		t.Fatalf("creación no propagada: %q owner=%q", st.lastSharedAddr, st.lastOwnerSubj)
	}
}

func TestCreateSharedMailboxForeignDomain(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@acme.sytue.eu", Tenant: "acme"}}
	a := newAPI(st, nil, nil)
	a.cfg.PrimaryDomain = "sytue.eu"
	rec := crearCompartido(a, adminDeAcme, "soporte@gmail.com")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("dominio ajeno => %d, quiero 400", rec.Code)
	}
	if st.lastSharedAddr != "" {
		t.Fatalf("no debe llegar al store: %q", st.lastSharedAddr)
	}
}

// COM-03. Crear un buzón compartido no pedía rol, no miraba las direcciones
// reservadas y no tenía tope: cualquier cuenta, también una personal gratuita, se
// quedaba con `seguridad@<dominio raíz>` y enviaba y recibía en nombre de la plataforma.
func TestCreateSharedMailboxPideRolReservadosYTope(t *testing.T) {
	nueva := func(direccion, tenant string, yaHay int) (*fakeStore, *API) {
		st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: direccion, Tenant: tenant, Subject: "sub"}, compartidos: yaHay}
		a := newAPI(st, nil, nil)
		a.cfg.PrimaryDomain = "sytue.eu"
		return st, a
	}
	// Una cuenta personal vive en el dominio raíz y es `owner` de su organización de una.
	personal := authn.Claims{Subject: "u", Tenant: "p-3f9a", OrgKind: "personal", Roles: []string{"owner"}}

	t.Run("un miembro sin rol no crea buzones compartidos", func(t *testing.T) {
		for _, c := range []authn.Claims{
			{Subject: "u", Tenant: "acme"},
			{Subject: "u", Tenant: "acme", Roles: []string{"member"}},
		} {
			st, a := nueva("ana@acme.sytue.eu", "acme", 0)
			rec := crearCompartido(a, c, "nominas@acme.sytue.eu")
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "shared_mailbox_role") {
				t.Fatalf("roles %v => %d %s, quiero 403 shared_mailbox_role", c.Roles, rec.Code, rec.Body.String())
			}
			if st.lastSharedAddr != "" {
				t.Fatalf("ha llegado al store: %q", st.lastSharedAddr)
			}
		}
	})

	t.Run("nadie se queda con una dirección reservada del dominio raíz", func(t *testing.T) {
		for _, addr := range []string{"seguridad@sytue.eu", "soporte@sytue.eu", "no-reply@sytue.eu", "sytue-id@sytue.eu", "facturacion@sytue.eu"} {
			st, a := nueva("ana@sytue.eu", "p-3f9a", 0)
			rec := crearCompartido(a, personal, addr)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "alias_reserved") {
				t.Fatalf("%s => %d %s, quiero 400 alias_reserved", addr, rec.Code, rec.Body.String())
			}
			if st.lastSharedAddr != "" {
				t.Fatalf("%s ha llegado al store", addr)
			}
		}
		// La misma palabra en el subdominio de una organización es asunto suyo.
		st, a := nueva("ada@acme.sytue.eu", "acme", 0)
		if rec := crearCompartido(a, adminDeAcme, "soporte@acme.sytue.eu"); rec.Code != http.StatusCreated || st.lastSharedAddr == "" {
			t.Fatalf("soporte@acme.sytue.eu => %d (%s), quiero 201", rec.Code, rec.Body.String())
		}
	})

	t.Run("hay tope", func(t *testing.T) {
		for _, c := range []struct {
			nombre string
			claims authn.Claims
			cuenta string
			yaHay  int
			quiere int
		}{
			{"organización por debajo", adminDeAcme, "ada@acme.sytue.eu", maxBuzonesCompartidos - 1, http.StatusCreated},
			{"organización en el tope", adminDeAcme, "ada@acme.sytue.eu", maxBuzonesCompartidos, http.StatusConflict},
			{"cuenta personal, el primero", personal, "ana@sytue.eu", 0, http.StatusCreated},
			{"cuenta personal, el segundo", personal, "ana@sytue.eu", maxBuzonesCompartidosPersonal, http.StatusConflict},
		} {
			st, a := nueva(c.cuenta, c.claims.Tenant, c.yaHay)
			dom := c.cuenta[strings.IndexByte(c.cuenta, '@')+1:]
			rec := crearCompartido(a, c.claims, "equipo.norte@"+dom)
			if rec.Code != c.quiere {
				t.Fatalf("%s => %d (%s), quiero %d", c.nombre, rec.Code, rec.Body.String(), c.quiere)
			}
			if c.quiere == http.StatusConflict && (!strings.Contains(rec.Body.String(), "shared_mailbox_limit") || st.lastSharedAddr != "") {
				t.Fatalf("%s: falta shared_mailbox_limit o ha llegado al store: %s", c.nombre, rec.Body.String())
			}
		}
	})
}

// --- envío-como (buzón compartido) / en-nombre-de (delegación) ------------

// Enviar-COMO un buzón compartido con permiso send: From = la compartida, SIN
// cabecera Sender (la compartida ES la identidad).
func TestSendAsSharedMailbox(t *testing.T) {
	st := &fakeStore{
		acc:          domain.Account{ID: uuid.New(), Address: "ada@sytue.eu", Subject: "ada-sub"},
		aliasOwned:   false,
		grantedIdent: domain.GrantedIdentity{Address: "soporte@sytue.eu", Kind: domain.KindShared, Permission: domain.PermSend},
	}
	out := &fakeOutbound{}
	a := newAPI(st, nil, out)
	rec := httptest.NewRecorder()
	body := `{"from":"soporte@sytue.eu","recipients":["cli@x.com"],"raw":"From: soporte@sytue.eu\r\n\r\nhola"}`
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", body, true, nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("código = %d, quiero 202 (%s)", rec.Code, rec.Body.String())
	}
	if out.lastFrom != "soporte@sytue.eu" {
		t.Fatalf("from = %q, quiero la compartida", out.lastFrom)
	}
	if strings.Contains(string(out.lastRaw), "Sender:") {
		t.Fatalf("enviar-como NO debe llevar Sender:\n%s", out.lastRaw)
	}
}

// Enviar-EN-NOMBRE-DE un buzón delegado (cuenta personal de otro) con permiso send:
// From = el directivo, y el servidor SELLA Sender = el remitente real (el asistente).
func TestSendOnBehalfOfDelegated(t *testing.T) {
	st := &fakeStore{
		acc:          domain.Account{ID: uuid.New(), Address: "asst@sytue.eu", Subject: "asst-sub"},
		aliasOwned:   false,
		grantedIdent: domain.GrantedIdentity{Address: "dir@sytue.eu", Kind: domain.KindUser, Permission: domain.PermSend},
	}
	out := &fakeOutbound{}
	a := newAPI(st, nil, out)
	rec := httptest.NewRecorder()
	body := `{"from":"dir@sytue.eu","recipients":["cli@x.com"],"raw":"From: dir@sytue.eu\r\n\r\nhola"}`
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", body, true, nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("código = %d, quiero 202 (%s)", rec.Code, rec.Body.String())
	}
	if out.lastFrom != "dir@sytue.eu" {
		t.Fatalf("from = %q, quiero el directivo", out.lastFrom)
	}
	if !strings.Contains(string(out.lastRaw), "Sender: asst@sytue.eu") {
		t.Fatalf("on-behalf debe sellar Sender = remitente real:\n%s", out.lastRaw)
	}
}

// Con ACL pero permiso insuficiente (solo read) NO se puede enviar: 403.
func TestSendAsInsufficientPermission(t *testing.T) {
	st := &fakeStore{
		acc:          domain.Account{ID: uuid.New(), Address: "ada@sytue.eu", Subject: "ada-sub"},
		grantedIdent: domain.GrantedIdentity{Address: "soporte@sytue.eu", Kind: domain.KindShared, Permission: domain.PermRead},
	}
	out := &fakeOutbound{}
	a := newAPI(st, nil, out)
	rec := httptest.NewRecorder()
	body := `{"from":"soporte@sytue.eu","recipients":["cli@x.com"],"raw":"x"}`
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", body, true, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("solo-read => %d, quiero 403", rec.Code)
	}
	if out.lastFrom != "" {
		t.Fatalf("no debe encolar sin permiso: %q", out.lastFrom)
	}
}

// Sin ninguna ACL sobre la dirección From ajena: 403 (no es alias ni compartida/delegada).
func TestSendAsNoGrant(t *testing.T) {
	st := &fakeStore{
		acc:        domain.Account{ID: uuid.New(), Address: "ada@sytue.eu", Subject: "ada-sub"},
		grantedErr: domain.ErrNotFound,
	}
	out := &fakeOutbound{}
	a := newAPI(st, nil, out)
	rec := httptest.NewRecorder()
	body := `{"from":"ceo@sytue.eu","recipients":["cli@x.com"],"raw":"x"}`
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", body, true, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sin ACL => %d, quiero 403", rec.Code)
	}
}
