// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del ALTA INICIAL guiada (mig 0027) contra un PostgreSQL
// real (pgtest, build-tag `integration`). Comprueban que cada paso hace algo de
// verdad, no que la pantalla avance:
//
//   - recorrido individual y recorrido organización de principio a fin;
//   - REANUDACIÓN: lo guardado en cada paso se relee tal cual (simula cerrar el
//     navegador y volver);
//   - disponibilidad del identificador contra la BD (libre / ocupado / reservado);
//   - verificación del dominio propio con resolver INYECTADO (verificado / aún
//     sin propagar / registro equivocado) y su sello en BD;
//   - roles recomendados creados con sus permisos REALES, visibles en /admin/roles;
//   - omitir facturación no bloquea el alta;
//   - el resumen final sale de la BD (invitaciones y roles reales).
//
// El resolver DNS se sustituye por uno de prueba: ni un test sale a la red.
package http

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	dnsclient "git.sytue.eu/sytue/one/services/id/internal/dns"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// stubResolver es el resolver DNS inyectado en el Server durante el test.
type stubResolver struct {
	txts []string
	err  error
}

func (s *stubResolver) LookupTXT(_ context.Context, _ string) ([]string, error) {
	return s.txts, s.err
}

// newcomer crea un usuario SIN organización ni roles (el caso real del alta: aún
// no es owner de nada) y devuelve su token de acceso.
func (l *liveServer) newcomer(t *testing.T, email string) (domain.User, string) {
	t.Helper()
	u := l.mkUser(t, email, "alta-inicial-123")
	return u, l.token(t, u.ID, "", nil)
}

func TestOnboarding_RecorridoOrganizacionCompleto(t *testing.T) {
	l := newLiveServer(t)
	u, tok := l.newcomer(t, "founder@id.test")

	// Paso 1 — modo. Se guarda al instante (es lo que permite retomar).
	rr, out := l.do(t, http.MethodPatch, "/api/v1/onboarding", tok,
		map[string]any{"mode": "organization", "step": domain.StepIdentity})
	if rr.Code != http.StatusOK || out["mode"] != "organization" {
		t.Fatalf("guardar modo: status=%d cuerpo=%s", rr.Code, rr.Body)
	}

	// Paso 2 — identidad: disponibilidad REAL del identificador y creación.
	rr, out = l.do(t, http.MethodGet, "/api/v1/onboarding/slug?slug=acme", tok, nil)
	if rr.Code != http.StatusOK || out["available"] != true {
		t.Fatalf("acme debía estar libre: %v", out)
	}
	if out["subdomain"] != "acme."+suiteDomain(itIssuer) {
		t.Fatalf("subdominio propuesto inesperado: %v", out)
	}
	rr, out = l.do(t, http.MethodPost, "/api/v1/onboarding/organization", tok,
		map[string]string{"name": "ACME Europa", "slug": "acme"})
	if rr.Code != http.StatusCreated || out["slug"] != "acme" {
		t.Fatalf("crear org: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	org, err := l.st.GetOrgBySlug(l.ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	// El que da de alta queda como OWNER de verdad.
	if mem, err := l.st.GetMembership(l.ctx, org.ID, u.ID); err != nil || mem.Role != domain.RoleOwner {
		t.Fatalf("el fundador debe ser owner: %+v (%v)", mem, err)
	}
	// Ya no está libre: la disponibilidad se consulta contra la BD.
	_, out = l.do(t, http.MethodGet, "/api/v1/onboarding/slug?slug=acme", tok, nil)
	if out["available"] != false || out["reason"] == "" {
		t.Fatalf("acme debía estar ocupado con motivo: %v", out)
	}

	// Paso 3 — personas: invitaciones REALES (fila en BD + correo por el mismo
	// camino que las de Sytue Admin).
	rr, out = l.do(t, http.MethodPost, "/api/v1/onboarding/invitations", tok,
		map[string]string{"emails": "ana@acme.com, bruno@acme.com, no-es-correo", "role": "member"})
	if rr.Code != http.StatusOK {
		t.Fatalf("invitar: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if n, _ := out["sent"].(float64); n != 2 {
		t.Fatalf("enviadas=%v, quiero 2 (la tercera no es un correo): %v", out["sent"], out)
	}
	invs, err := l.st.ListPendingInvitations(l.ctx, org.ID)
	if err != nil || len(invs) != 2 {
		t.Fatalf("invitaciones en BD=%d (%v)", len(invs), err)
	}

	// Paso 5 — roles recomendados con permisos REALES.
	rr, out = l.do(t, http.MethodPost, "/api/v1/onboarding/roles", tok,
		map[string]any{"keys": []string{"soporte", "facturacion"}})
	if rr.Code != http.StatusOK {
		t.Fatalf("crear roles: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	roles, err := l.st.ListCustomRoles(l.ctx, org.ID)
	if err != nil || len(roles) != 2 {
		t.Fatalf("roles en BD=%d (%v)", len(roles), err)
	}
	byName := map[string][]string{}
	for _, r := range roles {
		byName[r.Name] = r.Permissions
	}
	sop, ok := byName["Soporte"]
	if !ok {
		t.Fatalf("falta el rol Soporte: %v", byName)
	}
	if !domain.HasPermission(nil, sop, domain.PermUsersManage) ||
		domain.HasPermission(nil, sop, domain.PermBillingManage) {
		// La explicación que lee el usuario dice justo esto: ayuda con cuentas,
		// nada de facturación.
		t.Fatalf("permisos de Soporte incorrectos: %v", sop)
	}
	if fac := byName["Facturación"]; !domain.HasPermission(nil, fac, domain.PermBillingManage) ||
		domain.HasPermission(nil, fac, domain.PermUsersManage) {
		t.Fatalf("permisos de Facturación incorrectos: %v", byName["Facturación"])
	}
	// Y son roles normales: la pantalla de Roles de Sytue Admin los ve.
	adminTok := l.token(t, u.ID, org.Slug, []string{"owner"})
	rr, out = l.do(t, http.MethodGet, "/api/v1/admin/roles", adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("listar roles: status=%d", rr.Code)
	}
	if items, _ := out["roles"].([]any); len(items) != 2 {
		t.Fatalf("la pantalla de Roles debe verlos: %v", out)
	}

	// Paso 6 — resumen: sale de la BD, no del formulario.
	rr, out = l.do(t, http.MethodPatch, "/api/v1/onboarding", tok,
		map[string]any{"step": domain.StepDone, "completed": true})
	if rr.Code != http.StatusOK || out["completed"] != true {
		t.Fatalf("completar: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	rr, out = l.do(t, http.MethodGet, "/api/v1/onboarding", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("resumen: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	orgOut, _ := out["organization"].(map[string]any)
	if orgOut == nil || orgOut["slug"] != "acme" || orgOut["subdomain"] != "acme."+suiteDomain(itIssuer) {
		t.Fatalf("organización del resumen: %v", out["organization"])
	}
	if items, _ := out["invitations"].([]any); len(items) != 2 {
		t.Fatalf("invitaciones del resumen=%v (deben salir de la BD)", out["invitations"])
	}
	if items, _ := out["roles"].([]any); len(items) != 2 {
		t.Fatalf("roles del resumen=%v (deben salir de la BD)", out["roles"])
	}
	if out["completed"] != true {
		t.Fatalf("el alta debe constar completada: %v", out)
	}
}

func TestOnboarding_RecorridoIndividualYFacturacionOpcional(t *testing.T) {
	l := newLiveServer(t)
	_, tok := l.newcomer(t, "solo@id.test")

	// El recorrido individual no tiene pasos de personas ni de roles.
	rr, out := l.do(t, http.MethodPatch, "/api/v1/onboarding", tok,
		map[string]any{"mode": "individual", "step": domain.StepIdentity})
	if rr.Code != http.StatusOK {
		t.Fatalf("modo individual: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	steps, _ := out["steps"].([]any)
	for _, s := range steps {
		if s == domain.StepPeople || s == domain.StepRoles {
			t.Fatalf("el recorrido individual no debe ofrecer %v: %v", s, steps)
		}
	}
	// Guardar el paso de roles en modo individual es un paso que no existe → 400.
	rr, _ = l.do(t, http.MethodPatch, "/api/v1/onboarding", tok,
		map[string]any{"step": domain.StepRoles})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("paso ajeno al recorrido: status=%d, quiero 400", rr.Code)
	}

	// Por debajo se crea igualmente una organización (modelo de datos único).
	rr, _ = l.do(t, http.MethodPost, "/api/v1/onboarding/organization", tok,
		map[string]string{"name": "Marta Ruiz", "slug": "marta"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("crear cuenta: status=%d", rr.Code)
	}

	// Omitir la facturación NO bloquea: se cierra el alta sin enviarla.
	rr, out = l.do(t, http.MethodPatch, "/api/v1/onboarding", tok,
		map[string]any{"step": domain.StepDone, "completed": true})
	if rr.Code != http.StatusOK || out["completed"] != true {
		t.Fatalf("completar sin facturación: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	_, out = l.do(t, http.MethodGet, "/api/v1/onboarding", tok, nil)
	billing, _ := out["billing"].(map[string]any)
	if billing == nil || billing["legal_name"] != "" {
		t.Fatalf("facturación vacía esperada: %v", out["billing"])
	}

	// Y si luego la rellena, se persiste y se relee tal cual.
	rr, _ = l.do(t, http.MethodPatch, "/api/v1/onboarding", tok, map[string]any{
		"billing": map[string]string{
			"legal_name": "Marta Ruiz", "tax_id": "12345678Z",
			"address": "Calle Mayor 1, Madrid", "billing_email": "Facturas@Marta.es",
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("guardar facturación: status=%d", rr.Code)
	}
	_, out = l.do(t, http.MethodGet, "/api/v1/onboarding", tok, nil)
	billing, _ = out["billing"].(map[string]any)
	if billing["tax_id"] != "12345678Z" || billing["billing_email"] != "facturas@marta.es" {
		t.Fatalf("facturación releída: %v", billing)
	}
}

func TestOnboarding_ReanudacionTrasCerrarElNavegador(t *testing.T) {
	l := newLiveServer(t)
	u, tok := l.newcomer(t, "vuelve@id.test")

	l.do(t, http.MethodPatch, "/api/v1/onboarding", tok,
		map[string]any{"mode": "organization", "step": domain.StepIdentity})
	l.do(t, http.MethodPost, "/api/v1/onboarding/organization", tok,
		map[string]string{"name": "Globex", "slug": "globex"})
	l.do(t, http.MethodPatch, "/api/v1/onboarding", tok, map[string]any{
		"step":    domain.StepBilling,
		"billing": map[string]string{"legal_name": "Globex SL", "tax_id": "B12345678"},
	})

	// Cierra el navegador: token NUEVO, sesión nueva, ni un dato en el cliente.
	fresh := l.token(t, u.ID, "", nil)
	rr, out := l.do(t, http.MethodGet, "/api/v1/onboarding", fresh, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("retomar: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["mode"] != "organization" || out["step"] != domain.StepBilling {
		t.Fatalf("debe retomar donde lo dejó: mode=%v step=%v", out["mode"], out["step"])
	}
	orgOut, _ := out["organization"].(map[string]any)
	if orgOut == nil || orgOut["name"] != "Globex" {
		t.Fatalf("la org creada debe seguir ahí: %v", out["organization"])
	}
	billing, _ := out["billing"].(map[string]any)
	if billing["legal_name"] != "Globex SL" || billing["tax_id"] != "B12345678" {
		t.Fatalf("los datos ya introducidos deben volver: %v", billing)
	}
	// No se crea una segunda organización al volver: se renombra la suya.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/onboarding/organization", fresh,
		map[string]string{"name": "Globex Europa", "slug": "otro-slug"})
	if rr.Code != http.StatusOK {
		t.Fatalf("renombrar: status=%d", rr.Code)
	}
	if _, err := l.st.GetOrgBySlug(l.ctx, "otro-slug"); err == nil {
		t.Fatal("no debe crearse una segunda organización")
	}
	org, err := l.st.GetOrgBySlug(l.ctx, "globex")
	if err != nil || org.Name != "Globex Europa" {
		t.Fatalf("la org debe renombrarse: %+v (%v)", org, err)
	}
}

func TestOnboarding_SlugReservadoYFormato(t *testing.T) {
	l := newLiveServer(t)
	_, tok := l.newcomer(t, "slugs@id.test")

	for _, c := range []struct{ slug, why string }{
		{"mail", "reservado por la suite"},
		{"AC ME", "formato inválido"},
		{"ab", "demasiado corto"},
	} {
		_, out := l.do(t, http.MethodGet,
			"/api/v1/onboarding/slug?slug="+url.QueryEscape(c.slug), tok, nil)
		if out["available"] != false || out["reason"] == "" {
			t.Fatalf("%s (%s): %v", c.slug, c.why, out)
		}
	}
	// Y el reservado tampoco cuela por POST directo (la UI no es la defensa).
	l.do(t, http.MethodPatch, "/api/v1/onboarding", tok,
		map[string]any{"mode": "organization", "step": domain.StepIdentity})
	rr, _ := l.do(t, http.MethodPost, "/api/v1/onboarding/organization", tok,
		map[string]string{"name": "X", "slug": "mail"})
	if rr.Code != http.StatusConflict {
		t.Fatalf("slug reservado por API: status=%d, quiero 409", rr.Code)
	}
}

func TestOnboarding_VerificacionDeDominioReal(t *testing.T) {
	l := newLiveServer(t)
	_, tok := l.newcomer(t, "dominio@id.test")
	l.do(t, http.MethodPatch, "/api/v1/onboarding", tok,
		map[string]any{"mode": "organization", "step": domain.StepIdentity})
	l.do(t, http.MethodPost, "/api/v1/onboarding/organization", tok,
		map[string]string{"name": "ACME", "slug": "acme"})

	// 1) Aún sin propagar: pulsar el botón NO verifica nada.
	l.srv.resolver = &stubResolver{err: &net.DNSError{Err: "no such host", IsNotFound: true}}
	rr, out := l.do(t, http.MethodPost, "/api/v1/onboarding/domain", tok,
		map[string]string{"domain": "https://www.ACME.com/"})
	if rr.Code != http.StatusOK {
		t.Fatalf("dominio: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["host"] != "acme.com" || out["record_name"] != "_sytue.acme.com" {
		t.Fatalf("registro a publicar: %v", out)
	}
	if out["status"] != string(domain.DomainPending) || out["verified"] != false {
		t.Fatalf("sin registro publicado no puede darse por verificado: %v", out)
	}
	record, _ := out["record_value"].(string)
	if record == "" {
		t.Fatalf("falta el valor del registro: %v", out)
	}

	// 2) Registro equivocado: se dice qué se encontró de verdad.
	l.srv.resolver = &stubResolver{txts: []string{"sytue-verification=de-otro-sitio"}}
	_, out = l.do(t, http.MethodPost, "/api/v1/onboarding/domain", tok,
		map[string]string{"domain": "acme.com"})
	if out["status"] != string(domain.DomainMismatch) || out["verified"] != false {
		t.Fatalf("registro equivocado: %v", out)
	}
	if found, _ := out["found"].([]any); len(found) != 1 {
		t.Fatalf("debe devolver lo encontrado: %v", out["found"])
	}

	// 3) Registro correcto: verificado y SELLADO en BD.
	l.srv.resolver = &stubResolver{txts: []string{record}}
	_, out = l.do(t, http.MethodPost, "/api/v1/onboarding/domain", tok,
		map[string]string{"domain": "acme.com"})
	if out["status"] != string(domain.DomainVerified) || out["verified"] != true {
		t.Fatalf("registro correcto: %v", out)
	}
	org, err := l.st.GetOrgBySlug(l.ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	prof, err := l.st.GetOrgProfile(l.ctx, org.ID)
	if err != nil || prof.DomainVerifiedAt == nil || prof.Domain != "acme.com" {
		t.Fatalf("la verificación debe quedar sellada en BD: %+v (%v)", prof, err)
	}
	// El resumen la refleja al retomar.
	_, out = l.do(t, http.MethodGet, "/api/v1/onboarding", tok, nil)
	dom, _ := out["domain"].(map[string]any)
	if dom == nil || dom["verified"] != true || dom["host"] != "acme.com" {
		t.Fatalf("dominio en el resumen: %v", out["domain"])
	}

	// 4) Cambiar de dominio anula la verificación anterior (no se hereda el sello).
	l.srv.resolver = &stubResolver{err: &net.DNSError{Err: "no such host", IsNotFound: true}}
	_, out = l.do(t, http.MethodPost, "/api/v1/onboarding/domain", tok,
		map[string]string{"domain": "otra-empresa.com"})
	if out["verified"] != false {
		t.Fatalf("un dominio nuevo empieza sin verificar: %v", out)
	}
}

func TestOnboarding_AislamientoEntreUsuarios(t *testing.T) {
	l := newLiveServer(t)
	_, tokA := l.newcomer(t, "a@id.test")
	_, tokB := l.newcomer(t, "b@id.test")

	l.do(t, http.MethodPatch, "/api/v1/onboarding", tokA,
		map[string]any{"mode": "organization", "step": domain.StepIdentity})
	l.do(t, http.MethodPost, "/api/v1/onboarding/organization", tokA,
		map[string]string{"name": "ACME", "slug": "acme"})

	// B no ve nada de A: su progreso está vacío y su org es nil.
	rr, out := l.do(t, http.MethodGet, "/api/v1/onboarding", tokB, nil)
	if rr.Code != http.StatusOK || out["organization"] != nil {
		t.Fatalf("B no debe ver la organización de A: %v", out)
	}
	// Y sin org propia no puede invitar ni crear roles «de nadie».
	rr, _ = l.do(t, http.MethodPost, "/api/v1/onboarding/invitations", tokB,
		map[string]string{"emails": "x@acme.com", "role": "member"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invitar sin org: status=%d, quiero 400", rr.Code)
	}
	rr, _ = l.do(t, http.MethodPost, "/api/v1/onboarding/roles", tokB,
		map[string]any{"keys": []string{"soporte"}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("roles sin org: status=%d, quiero 400", rr.Code)
	}
}

// Crear una organización TENIENDO YA OTRA no es «alta terminada».
//
// `completed` se resolvía como «¿tiene alguna membresía?». Para el invitado es lo
// correcto —su organización existe y alguien la administra, no hay nada que
// preguntarle—, pero atrapaba también a quien acababa de crear una organización
// nueva: su alta se declaraba terminada en el acto y el asistente dejaba de
// guiarle, con lo que esa organización se quedaba sin dominio, sin suscripción y
// sin ninguna puerta para configurarlos (conectar un dominio propio SOLO se podía
// hacer desde aquí; ver la cabecera de admin_domain.go).
//
// Medido contra producción el 2026-08-13 con una organización recién creada:
// `completed: true` conviviendo con `step: "identidad"` —el primer paso—, sin
// dominio, sin suscripción, sin invitaciones y sin roles.
func TestOnboarding_CrearOrgTeniendoOtraNoEsAltaTerminada(t *testing.T) {
	l := newLiveServer(t)
	usuariaA, tokA := l.newcomer(t, "duena@id.test")
	_, tokB := l.newcomer(t, "invitada@id.test")

	// A hace su alta entera y la termina.
	l.do(t, http.MethodPatch, "/api/v1/onboarding", tokA,
		map[string]any{"mode": "organization", "step": domain.StepIdentity})
	l.do(t, http.MethodPost, "/api/v1/onboarding/organization", tokA,
		map[string]string{"name": "Primera", "slug": "primera"})
	l.do(t, http.MethodPatch, "/api/v1/onboarding", tokA,
		map[string]any{"step": domain.StepDone, "completed": true})

	// Para administrar su organización, A necesita un token que la nombre. El del
	// alta se acuñó ANTES de que la organización existiera, así que no lleva ni
	// tenant ni rol, y `requireRole` lee el claim y no la base de datos: con él,
	// añadir a alguien responde 403. En la vida real la credencial se renueva al
	// terminar el alta; aquí se acuña la equivalente.
	adminA := l.token(t, usuariaA.ID, "primera", []string{"owner"})

	// B entra INVITADA: nunca pasa por el asistente, así que no tiene alta propia.
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/orgs/primera/members", adminA,
		map[string]string{"email": "invitada@id.test", "role": "member"}); rr.Code >= 300 {
		t.Fatalf("añadir a B como miembro: status=%d", rr.Code)
	}
	// Para ella el alta SÍ está terminada: es el atajo que hay que conservar.
	if rr, out := l.do(t, http.MethodGet, "/api/v1/onboarding", tokB, nil); rr.Code != http.StatusOK ||
		out["completed"] != true {
		t.Fatalf("invitada: completed=%v, quiero true (%v)", out["completed"], out)
	}

	// Y ahora B crea SU organización. Tiene membresía previa, pero su alta acaba
	// de empezar: declararla terminada la dejaría sin forma de configurarla.
	l.do(t, http.MethodPatch, "/api/v1/onboarding", tokB,
		map[string]any{"mode": "organization", "step": domain.StepIdentity})
	l.do(t, http.MethodPost, "/api/v1/onboarding/organization", tokB,
		map[string]string{"name": "Segunda", "slug": "segunda"})

	rr, out := l.do(t, http.MethodGet, "/api/v1/onboarding", tokB, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("estado del alta de B: status=%d", rr.Code)
	}
	if out["completed"] != false {
		t.Fatalf("crear org teniendo otra: completed=%v, quiero false (%v)", out["completed"], out)
	}
	// Y la organización que devuelve es la SUYA, no la de A.
	org, _ := out["organization"].(map[string]any)
	if org == nil || org["slug"] != "segunda" {
		t.Fatalf("el asistente debe hablar de la org recién creada: %v", out["organization"])
	}
}

// --- Dirección en Sytue: publicación REAL del subdominio --------------------

// El asistente no dice «tu dirección funciona» porque el identificador esté
// guardado: crea el registro CNAME en NUESTRA zona de Cloudflare y refleja lo que
// pasó de verdad. Aquí el Cloudflare es un httptest que imita la API v4, y se
// comprueba que la respuesta del alta lleva el estado real y que la publicación
// queda AUDITADA (es una acción con efecto fuera de Sytue).
func TestOnboarding_PublicaSubdominioEnCloudflare(t *testing.T) {
	l := newLiveServer(t)
	u, tok := l.newcomer(t, "dns@id.test")

	var creado map[string]any
	cf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet { // aún no está en la zona
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&creado)
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"rec-1"}}`))
	}))
	defer cf.Close()
	defer ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false })()
	l.srv.cfg.TenantDNSTarget = "apps." + suiteDomain(itIssuer)
	l.srv.cfg.TenantDNSProxied = true
	// A los DOS campos, como hace el constructor real (`s.zoneDNS = s.cf`):
	// `ensureTenantDNS` publica por `zoneDNS`, la interfaz que abstrae Cloudflare
	// de Scaleway, y dejarla a nil hacía que este test saliera por la guarda de
	// «no configurado» sin llegar a la publicación que dice comprobar.
	cl := dnsclient.New("token-de-prueba", "acct-1", "zone-1", cf.URL)
	l.srv.cf, l.srv.zoneDNS = cl, cl

	rr, out := l.do(t, http.MethodPost, "/api/v1/onboarding/organization", tok,
		map[string]string{"name": "ACME Europa", "slug": "acme"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("crear org: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	dnsOut, _ := out["dns"].(map[string]any)
	if dnsOut["status"] != "live" {
		t.Fatalf("la dirección debía quedar publicada: %v", out["dns"])
	}
	want := "acme." + suiteDomain(itIssuer)
	if dnsOut["host"] != want {
		t.Fatalf("host publicado = %v, quiero %q", dnsOut["host"], want)
	}
	if creado["type"] != "CNAME" || creado["name"] != want {
		t.Fatalf("el registro creado en Cloudflare no es el esperado: %v", creado)
	}

	// La publicación se audita con el nombre exacto que se creó.
	org, err := l.st.GetOrgBySlug(l.ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	rows, _, aerr := l.st.ListAuditPage(l.ctx, org.ID,
		store.AuditFilter{Action: "org.subdomain.published"}, nil, 10)
	if aerr != nil {
		t.Fatal(aerr)
	}
	if len(rows) != 1 || rows[0].Target != want || rows[0].ActorID != u.ID {
		t.Fatalf("auditoría de la publicación: %+v", rows)
	}

	// Al retomar el alta el estado se relee de Cloudflare: ya está, no se re-crea.
	creado = nil
	_, out = l.do(t, http.MethodGet, "/api/v1/onboarding", tok, nil)
	orgOut, _ := out["organization"].(map[string]any)
	dnsOut, _ = orgOut["dns"].(map[string]any)
	if dnsOut["status"] != "live" {
		t.Fatalf("al retomar, la dirección sigue publicada: %v", orgOut["dns"])
	}
}

// Sin token de Cloudflare el alta NO se rompe y NO se finge: la dirección queda
// reservada y el asistente lo dice («unconfigured»), sin afirmar que resuelve.
func TestOnboarding_SinCloudflareElAltaSigue(t *testing.T) {
	l := newLiveServer(t)
	_, tok := l.newcomer(t, "sin-cf@id.test")
	if l.srv.cf != nil {
		t.Fatal("el harness no configura Cloudflare: cf debe ser nil")
	}
	rr, out := l.do(t, http.MethodPost, "/api/v1/onboarding/organization", tok,
		map[string]string{"name": "ACME", "slug": "acme"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("el alta no puede depender de Cloudflare: status=%d %s", rr.Code, rr.Body)
	}
	dnsOut, _ := out["dns"].(map[string]any)
	if dnsOut["status"] != "unconfigured" {
		t.Fatalf("sin Cloudflare no se afirma nada: %v", out["dns"])
	}
	// Y la consulta de precios lo dice igual de claro, sin inventar disponibilidad.
	_, out = l.do(t, http.MethodGet, "/api/v1/onboarding/domain-availability?domain=acme.com", tok, nil)
	if out["status"] != "unconfigured" || out["purchasable_here"] != false {
		t.Fatalf("disponibilidad sin configurar: %v", out)
	}
}
