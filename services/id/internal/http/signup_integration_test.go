// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El alta autoservicio de principio a fin contra un PostgreSQL real: se elige un
// @usuario, se confirma el móvil con el código del SMS y con eso nacen la persona,
// su organización personal —con el contrato gratuito y el alta guiada hecha— y su
// buzón usuario@<dominio>, en una transacción; se entra al momento y también por
// @usuario y por móvil; un @usuario ocupado o reservado se rechaza sin mandar
// ningún SMS; y la subida a pro cambia la cuota solo cuando el pago está cobrado.
// Los límites del código y el correo de recuperación: movil_y_recuperacion_integration_test.go.
package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwt"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/commerce"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// formulario envía un POST como lo haría el navegador desde la propia página.
func (l *liveServer) formulario(t *testing.T, path string, campos url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(campos.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Accept-Language", "es")
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	return rr
}

func TestSignup_CreaLaCuentaPersonalConSuUsuario(t *testing.T) {
	l := newLiveServer(t)
	const pw = "una-contraseña-larga-2026"
	// El dominio raíz de este servidor de pruebas: el de su emisor sin «id.».
	email := "ana.perez@" + suiteDomain(itIssuer)

	rr := l.alta(t, url.Values{"username": {"@Ana.Perez"}, "name": {"Ana"}, "password": {pw}, "phone": {"+34 655 000 111"}})
	// Acaba en «Todo listo», con su @usuario y su correo a la vista y el botón a
	// Sytue Hub (sin return_to no hay otra app a la que volver).
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), i18n.T("es", "signup.done_title")) ||
		!strings.Contains(rr.Body.String(), `href="`+l.srv.suiteHomeURL()+`"`) || !strings.Contains(rr.Body.String(), email) {
		t.Fatalf("alta: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if cookieNamed(rr.Result().Cookies(), sessionCookieName) == nil {
		t.Fatal("tras el alta se entra sin volver a teclear la contraseña: falta la cookie de sesión")
	}

	// La persona: @usuario elegido, su dirección en el dominio raíz y verificada.
	u, err := l.st.GetUserByUsername(l.ctx, "ana.perez")
	if err != nil {
		t.Fatalf("la cuenta no se creó: %v", err)
	}
	if u.Email != email || !u.EmailVerified || u.Name != "Ana" || u.PasswordHash == "" || u.Phone != "+34655000111" {
		t.Fatalf("cuenta inesperada: %+v", u)
	}

	// Su organización personal: una, owner, slug derivado, nombre = correo.
	mems, err := l.st.ListOrgMembershipsForUser(l.ctx, u.ID)
	if err != nil || len(mems) != 1 {
		t.Fatalf("membresías: %v (%v)", mems, err)
	}
	m := mems[0]
	if m.Kind != domain.OrgKindPersonal || m.Role != domain.RoleOwner || m.Name != email ||
		m.Slug != domain.PersonalSlug(email) || !strings.HasPrefix(m.Slug, domain.PersonalSlugPrefix) {
		t.Fatalf("organización personal inesperada: %+v", m)
	}

	// Su buzón es EXACTAMENTE la dirección elegida, no el patrón nombre.apellidos.
	buzones, err := l.st.ListOrgMailboxes(l.ctx, m.OrgID, u.ID)
	if err != nil || len(buzones) != 1 || buzones[0].Address != email || !buzones[0].Primary {
		t.Fatalf("buzón: %+v (%v)", buzones, err)
	}

	// Con el contrato gratuito: 5 GiB, sin pagar nada, y con derecho de uso.
	sub, err := l.st.GetOrgSubscription(l.ctx, m.OrgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription: %v", err)
	}
	ahora := time.Now()
	if sub.Plan != commerce.PlanPersonalFree || sub.UnitPriceCents != 0 || !sub.Entitled(ahora, 1, 0) ||
		sub.StorageBytes(ahora) != commerce.PersonalFreeStorageBytes {
		t.Fatalf("suscripción inesperada: %+v", sub)
	}

	// Y el alta guiada, hecha: el portal no le preguntará «¿para quién es Sytue?».
	ob, err := l.st.GetOnboarding(l.ctx, u.ID)
	if err != nil || ob.CompletedAt == nil || ob.Mode != string(domain.OnboardingIndividual) || ob.OrgID != m.OrgID {
		t.Fatalf("alta guiada: %+v (%v)", ob, err)
	}

	// Entra con el @usuario, con o sin arroba, con la dirección completa y con el móvil.
	for _, ident := range []string{"@ana.perez", "Ana.Perez", email, "+34 655 000 111"} {
		rr = l.formulario(t, "/login/", url.Values{"email": {ident}, "password": {pw}})
		if rr.Code != http.StatusFound {
			t.Fatalf("login con %q: status=%d cuerpo=%s", ident, rr.Code, rr.Body)
		}
	}
}

func TestSignup_UsuarioOcupadoOReservado(t *testing.T) {
	l := newLiveServer(t)
	const pw = "otra-contraseña-larga-2026"

	if rr := l.alta(t, url.Values{"username": {"bea"}, "password": {pw}}); rr.Code != http.StatusOK {
		t.Fatalf("primera alta: %d %s", rr.Code, rr.Body)
	}
	// Ocupado: se dice ANTES de mandar el SMS, porque un @usuario es público, y no
	// se crea nada más.
	sms := l.smsDePrueba()
	rr := l.formulario(t, "/signup", url.Values{"username": {"BEA"}, "password": {"distinta-y-larga-2026"}, "phone": {"+34 655 999 888"}})
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), i18n.T("es", "signup.username_taken")) {
		t.Fatalf("@usuario ocupado: status=%d", rr.Code)
	}
	if _, envio := sms.texto["+34655999888"]; envio {
		t.Fatal("con el @usuario ocupado no se gasta un SMS")
	}
	u, _ := l.st.GetUserByUsername(l.ctx, "bea")
	if mems, _ := l.st.ListOrgMembershipsForUser(l.ctx, u.ID); len(mems) != 1 {
		t.Fatalf("un alta repetida no debe crear otra organización: %v", mems)
	}

	// Una dirección del dominio raíz que ya es de OTRA persona tampoco se puede
	// elegir: @usuario y dirección son el mismo nombre.
	otra := l.mkUser(t, "carlos@acme.com", "")
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	if err := l.st.ReserveMailbox(l.ctx, nil, org.ID, otra.ID, "carlos@"+suiteDomain(itIssuer)); err != nil {
		t.Fatalf("ReserveMailbox: %v", err)
	}
	rr = l.formulario(t, "/signup", url.Values{"username": {"carlos"}, "password": {pw}, "phone": {"+34 655 999 777"}})
	if rr.Code != http.StatusConflict {
		t.Fatalf("dirección ajena del dominio raíz: status=%d", rr.Code)
	}
	if _, err := l.st.GetUserByUsername(l.ctx, "carlos"); err == nil {
		t.Fatal("con la dirección cogida no puede quedar una cuenta a medias")
	}

	// Reservado (suplantaría a la plataforma) e inválido.
	if rr := l.formulario(t, "/signup", url.Values{"username": {"soporte"}, "password": {pw}, "phone": {"+34 655 999 666"}}); rr.Code != http.StatusConflict {
		t.Fatalf("@soporte: status=%d", rr.Code)
	}
	if rr := l.formulario(t, "/signup", url.Values{"username": {"ana perez"}, "password": {pw}, "phone": {"+34 655 999 555"}}); rr.Code != http.StatusBadRequest {
		t.Fatalf("@usuario con espacio: status=%d", rr.Code)
	}
	// Una contraseña corta se rechaza ANTES de tocar la base.
	rr = l.formulario(t, "/signup", url.Values{"username": {"corta"}, "password": {"corta"}, "phone": {"+34 655 999 444"}})
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), i18n.T("es", "signup.weak_password")) {
		t.Fatalf("contraseña corta: status=%d", rr.Code)
	}
	if _, err := l.st.GetUserByUsername(l.ctx, "corta"); err == nil {
		t.Fatal("con la contraseña rechazada no debe existir la cuenta")
	}
}

// Una organización de verdad no puede tomar un slug del prefijo personal: es lo
// que evita que alguien se registre como «p-…» y choque —o se haga pasar— por
// la cuenta personal de otra persona.
func TestSignup_ElPrefijoPersonalQuedaReservado(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "dueno@acme.com", "una-contraseña-larga-2026")
	tok := l.token(t, u.ID, "", nil)
	rr, out := l.do(t, http.MethodGet, "/api/v1/onboarding/slug?slug="+domain.PersonalSlug("x@y.z"), tok, nil)
	if rr.Code != http.StatusOK || out["available"] != false {
		t.Fatalf("un slug personal no puede estar disponible para una organización: %d %v", rr.Code, out)
	}
}

// La subida a pro: registrar el cobro fija el contrato pero NO la cuota, que
// solo cambia cuando el pago está cobrado. Y el token lo publica todo.
func TestSignup_PasarAEnterpriseCambiaLaCuotaSoloAlCobrar(t *testing.T) {
	l := newLiveServer(t)
	rr := l.alta(t, url.Values{"username": {"carla"}, "password": {"una-contraseña-larga-2026"}})
	if rr.Code != http.StatusOK {
		t.Fatalf("alta: %d", rr.Code)
	}
	u, _ := l.st.GetUserByUsername(l.ctx, "carla")
	mems, _ := l.st.ListOrgMembershipsForUser(l.ctx, u.ID)
	orgID := mems[0].OrgID
	ahora := time.Now()

	// Por decisión del 2026-09-13 el paso a pro está cerrado: el estado lo dice
	// y el checkout lo rechaza, aunque la cuenta sea personal y no esté pagada.
	// Si commerce.PersonalUpgradeAvailable pasa a true, este bloque se retira.
	// Pedido por un client de la suite: /onboarding no se abre a una app de terceros.
	api, err := l.iss.IssueAccessToken(oidc.TokenClaims{
		Subject: u.ID, Audience: "sytue-console", Scope: "openid", Tenant: mems[0].Slug,
	}, ahora.UTC())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if !commerce.PersonalUpgradeAvailable {
		rr, estado := l.doCuerpo(t, http.MethodGet, "/api/v1/onboarding/subscription", api, "", "")
		if rr.Code != http.StatusOK || estado["upgrade_available"] != false {
			t.Fatalf("el estado debe decir que el pro no se puede contratar: %d %v", rr.Code, estado)
		}
		rr, _ = l.doCuerpo(t, http.MethodPost, "/api/v1/onboarding/subscription/checkout", api, "application/json", `{"seats":1}`)
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("checkout con el pro cerrado: %d %s", rr.Code, rr.Body.String())
		}
	}

	if err := l.st.RegisterSubscriptionCheckout(l.ctx, orgID, "cst_1", "tr_1", 1, commerce.EnterpriseSeatPriceCents, commerce.PlanEnterprise); err != nil {
		t.Fatalf("RegisterSubscriptionCheckout: %v", err)
	}
	sub, _ := l.st.GetOrgSubscription(l.ctx, orgID)
	if sub.Plan != commerce.PlanEnterprise || sub.UnitPriceCents != commerce.EnterpriseSeatPriceCents || sub.Paid(ahora) {
		t.Fatalf("tras el checkout el contrato es pro pero sin cobrar: %+v", sub)
	}
	if sub.StorageBytes(ahora) != commerce.PersonalFreeStorageBytes || !sub.Entitled(ahora, 1, 0) {
		t.Fatalf("sin cobrar, la cuota sigue siendo la gratuita y se sigue entrando: %+v", sub)
	}

	if err := l.st.RecordVerifiedPayment(l.ctx, "tr_1", orgID, "paid", commerce.EnterpriseSeatPriceCents, "sub_1", ahora); err != nil {
		t.Fatalf("RecordVerifiedPayment: %v", err)
	}
	sub, _ = l.st.GetOrgSubscription(l.ctx, orgID)
	if !sub.Paid(ahora) || sub.StorageBytes(ahora) != commerce.EnterpriseStorageBytes ||
		sub.MailboxBytes(ahora) != commerce.EnterpriseMailboxBytes {
		t.Fatalf("cobrado: %+v cuota=%d", sub, sub.StorageBytes(ahora))
	}

	// El access token lleva la clase, el plan y las cuotas.
	raw, err := l.iss.IssueAccessToken(oidc.TokenClaims{
		Subject: u.ID, Audience: "sytue-drive", Scope: "openid", Tenant: mems[0].Slug,
		OrgKind: string(domain.OrgKindPersonal), Plan: string(sub.EffectivePlan(ahora)),
		StorageBytes: sub.StorageBytes(ahora), MailboxBytes: sub.MailboxBytes(ahora),
	}, ahora.UTC())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	tok, err := jwt.ParseInsecure([]byte(raw))
	if err != nil {
		t.Fatalf("ParseInsecure: %v", err)
	}
	kind, _ := tok.Get("org_kind")
	plan, _ := tok.Get("plan")
	bytes, _ := tok.Get("storage_bytes")
	if kind != "personal" || plan != string(commerce.PlanEnterprise) || bytes != float64(commerce.EnterpriseStorageBytes) {
		t.Fatalf("claims del token: kind=%v plan=%v storage=%v", kind, plan, bytes)
	}
}

func TestSignup_ConCaptchaActivo(t *testing.T) {
	l := newLiveServer(t)
	l.srv.cfg.SignupCaptchaEnabled = true

	const username, pw = "david", "una-contraseña-larga-2026"

	// 1. Sin captcha -> 400 Bad Request con mensaje de captcha fallido
	rr := l.formulario(t, "/signup", url.Values{
		"username": {username},
		"password": {pw},
	})
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), i18n.T("es", "signup.captcha_failed")) {
		t.Fatalf("alta sin captcha: code=%d, cuerpo=%s", rr.Code, rr.Body)
	}

	// 2. Con captcha inválido -> 400 Bad Request
	rr = l.formulario(t, "/signup", url.Values{
		"username": {username},
		"password": {pw},
		"altcha":   {"payload-invalido-base64"},
	})
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), i18n.T("es", "signup.captcha_failed")) {
		t.Fatalf("alta con captcha inválido: code=%d, cuerpo=%s", rr.Code, rr.Body)
	}

	// 3. Obtener reto desde /signup/captcha
	r := httptest.NewRequest(http.MethodGet, "/signup/captcha", http.NoBody)
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("obtener reto: status=%d", rec.Code)
	}
	var ch altcha.Challenge
	if err := json.NewDecoder(rec.Body).Decode(&ch); err != nil {
		t.Fatalf("decode reto: %v", err)
	}

	// 4. Resolver reto y enviar
	sol, err := altcha.SolveChallenge(ch)
	if err != nil {
		t.Fatalf("resolver reto: %v", err)
	}
	rr = l.alta(t, url.Values{
		"username": {username},
		"name":     {"David"},
		"password": {pw},
		"altcha":   {sol},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("alta con captcha resuelto: code=%d, loc=%q", rr.Code, rr.Header().Get("Location"))
	}

	// Comprobar que la cuenta se creó
	u, err := l.st.GetUserByUsername(l.ctx, username)
	if err != nil || u.Name != "David" {
		t.Fatalf("cuenta no creada correctamente: %v, user: %+v", err, u)
	}

	// 5. Replay attack: intentar reutilizar el mismo payload resuelto debe fallar
	rr2 := l.formulario(t, "/signup", url.Values{
		"username": {"otro"},
		"password": {pw},
		"phone":    {"+34 655 999 333"},
		"altcha":   {sol},
	})
	if rr2.Code != http.StatusBadRequest || !strings.Contains(rr2.Body.String(), i18n.T("es", "signup.captcha_failed")) {
		t.Fatalf("reuso de captcha resuelto debe fallar: code=%d, cuerpo=%s", rr2.Code, rr2.Body)
	}
}

// LA VERJA DE requireOrgKind NO TENÍA TEST, y es la que impide que el alta
// gratuita abra la consola de administración a cualquiera: la persona es `owner`
// de su organización personal, así que por rol entraría en TODO. Se comprueba
// con la cuenta tal como la deja el alta y con un token de owner de su propia
// organización, que es exactamente lo que acuña el login.
func TestSignup_LaCuentaPersonalNoEntraEnAdmin(t *testing.T) {
	l := newLiveServer(t)
	rr := l.alta(t, url.Values{
		"username": {"sola"}, "name": {"Sola"}, "password": {"Contrasena-larga-12"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("alta: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	u, err := l.st.GetUserByUsername(l.ctx, "sola")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	tok := l.token(t, u.ID, domain.PersonalSlug(u.Email), []string{"owner"})

	for _, ruta := range []string{"/api/v1/admin/users", "/api/v1/admin/roles", "/api/v1/admin/invitations"} {
		rr, cuerpo := l.do(t, http.MethodGet, ruta, tok, nil)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s: una cuenta personal debe recibir 403 y recibió %d: %v", ruta, rr.Code, cuerpo)
		}
		if !strings.Contains(rr.Body.String(), "personal accounts") {
			t.Fatalf("%s: el 403 no dice que es por ser cuenta personal: %s", ruta, rr.Body)
		}
	}
}
