// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests del camino «mi dominio está en Cloudflare, configúralo por mí», SIN red y
// SIN base de datos: un httptest mantiene una zona DNS de mentira pero CON ESTADO
// (los registros que se crean quedan y se releen), que es lo único que permite
// comprobar de verdad lo que importa:
//
//   - la previsualización distingue «ya está bien», «falta» y «hay otra cosa ahí»;
//   - no se pisa nada sin permiso explícito, y con permiso sí;
//   - crear es idempotente: la segunda pasada no duplica ni vuelve a escribir;
//   - un fallo parcial se cuenta en SU registro y no da la operación por buena;
//   - el token del cliente no sale ni en la respuesta ni en los logs.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/id/internal/dns"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// tokenDelCliente es el secreto de tercero: ninguna respuesta ni ningún log puede
// contenerlo. Se busca literalmente en los tests.
const tokenDelCliente = "cf-token-DEL-CLIENTE-no-debe-filtrarse"

// --- Una zona de Cloudflare de mentira, pero con estado ---------------------

type fakeZone struct {
	t    *testing.T
	recs []dns.Record
	// writes cuenta cuántas veces se ha TOCADO la zona (POST + PUT). Es lo que
	// demuestra la idempotencia: la segunda pasada no debe subirlo.
	writes int
	next   int
	// rejects hace fallar la escritura de un tipo de registro concreto, para el
	// caso de fallo parcial.
	rejects string
}

func (z *fakeZone) envelope(w http.ResponseWriter, result string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true,"errors":[],"messages":[],"result":` + result + `}`))
}

func (z *fakeZone) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if got := r.Header.Get("Authorization"); got != "Bearer "+tokenDelCliente {
		z.t.Errorf("Cloudflare debe recibir el token del cliente, no otro: %q", got)
	}
	switch {
	case r.URL.Path == "/zones":
		if r.URL.Query().Get("name") != "acme.com" {
			z.envelope(w, `[]`)
			return
		}
		z.envelope(w, `[{"id":"zone-cliente","name":"acme.com","status":"active"}]`)

	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/dns_records"):
		name, typ := r.URL.Query().Get("name.exact"), r.URL.Query().Get("type")
		out := make([]dns.Record, 0, len(z.recs))
		for _, rec := range z.recs {
			if rec.Name == name && (typ == "" || rec.Type == typ) {
				out = append(out, rec)
			}
		}
		body, _ := json.Marshal(out)
		z.envelope(w, string(body))

	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/dns_records"):
		var rec dns.Record
		_ = json.NewDecoder(r.Body).Decode(&rec)
		if z.rejects != "" && rec.Type == z.rejects {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"success":false,"result":null,"errors":[{"code":9207,
				"message":"content is invalid"}]}`))
			return
		}
		z.writes++
		z.next++
		rec.ID = "rec-" + string(rune('a'+z.next))
		z.recs = append(z.recs, rec)
		body, _ := json.Marshal(rec)
		z.envelope(w, string(body))

	case r.Method == http.MethodPut:
		id := r.URL.Path[strings.LastIndexByte(r.URL.Path, '/')+1:]
		var rec dns.Record
		_ = json.NewDecoder(r.Body).Decode(&rec)
		z.writes++
		rec.ID = id
		for i := range z.recs {
			if z.recs[i].ID == id {
				z.recs[i] = rec
			}
		}
		body, _ := json.Marshal(rec)
		z.envelope(w, string(body))

	default:
		z.t.Errorf("llamada inesperada a Cloudflare: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

// cfServer monta el Server (sin store: estas funciones no lo tocan) y el cliente
// apuntado a la zona de mentira. Devuelve también el log capturado.
func cfServer(t *testing.T, z *fakeZone) (*Server, *dns.Client, *bytes.Buffer) {
	t.Helper()
	z.t = t
	srv := httptest.NewServer(z)
	t.Cleanup(srv.Close)
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
	var logged bytes.Buffer
	s := &Server{
		cfg: Config{Issuer: "https://id.sytue.test", MailMXHost: "mx.sytue.test"},
		log: slog.New(slog.NewTextHandler(&logged, nil)),
		cf:  dns.New("token-DE-PLATAFORMA", "acct", "zone-sytue", srv.URL),
	}
	return s, s.cf.ForCustomer(tokenDelCliente, "zone-cliente"), &logged
}

// perfil es el dominio ya reclamado por la organización (lo que devolvería el
// store): el mismo token de verificación del camino manual.
func perfil() store.OrgProfile {
	return store.OrgProfile{OrgID: "org-1", Domain: "acme.com", DomainToken: "1234abcd"}
}

// byKey indexa el resultado para poder afirmar sobre un registro concreto.
func byKey(out []recordWire) map[string]recordWire {
	m := make(map[string]recordWire, len(out))
	for _, w := range out {
		m[w.Key] = w
	}
	return m
}

// --- El plan: solo lo que Sytue puede poner de verdad ------------------------

// Sin correo, un solo registro: el de verificación. Nada más se toca del dominio
// de nadie sin motivo.
func TestCloudflarePlan_SinCorreoSoloVerificacion(t *testing.T) {
	s, _, _ := cfServer(t, &fakeZone{})
	plan := s.cloudflarePlan(perfil(), false)
	if len(plan) != 1 {
		t.Fatalf("plan = %+v; sin correo solo va la verificación", plan)
	}
	if plan[0].Type != "TXT" || plan[0].Name != "_sytue.acme.com" {
		t.Fatalf("el registro de verificación no es el del camino manual: %+v", plan[0])
	}
	if plan[0].Content != "sytue-verification=1234abcd" {
		t.Fatalf("valor = %q; debe salir de domain.RecordValue", plan[0].Content)
	}
}

// Con correo: MX + SPF reales, y DKIM/DMARC como PENDIENTE (no se inventa la
// clave pública, que hoy no sale de services/mail).
func TestCloudflarePlan_ConCorreoNoInventaDKIM(t *testing.T) {
	s, _, _ := cfServer(t, &fakeZone{})
	plan := s.cloudflarePlan(perfil(), true)
	if len(plan) != 4 {
		t.Fatalf("plan = %d registros, quiero verificación + MX + SPF + DKIM pendiente", len(plan))
	}
	mx := plan[1]
	if mx.Type != "MX" || mx.Name != "acme.com" || mx.Content != "mx.sytue.test" || mx.Priority != 10 {
		t.Fatalf("MX inesperado: %+v", mx)
	}
	spf := plan[2]
	if spf.Type != "TXT" || spf.Match != "v=spf1" || !strings.HasPrefix(spf.Content, "v=spf1 ") {
		t.Fatalf("SPF inesperado: %+v", spf)
	}
	dkim := plan[3]
	if !dkim.Pending {
		t.Fatal("el DKIM debe ir marcado como pendiente")
	}
	if dkim.Content != "" {
		t.Fatalf("un pendiente NO puede llevar valor: %q", dkim.Content)
	}
	if dkim.Detail == "" {
		t.Fatal("un pendiente tiene que explicar por qué")
	}
}

// Sin MX configurado en el despliegue, el correo no se ofrece: no se pinta lo que
// no se puede hacer.
func TestCloudflarePlan_SinMXConfiguradoNoOfreceCorreo(t *testing.T) {
	s, _, _ := cfServer(t, &fakeZone{})
	s.cfg.MailMXHost = ""
	if plan := s.cloudflarePlan(perfil(), true); len(plan) != 1 {
		t.Fatalf("plan = %+v; sin MX configurado no hay registros de correo", plan)
	}
}

// --- Previsualizar: leer, nunca escribir ------------------------------------

// Los tres estados a la vez: el TXT ya puesto y bueno, el MX que falta, y un SPF
// del cliente que choca con el nuestro. Y un TXT ajeno del vértice que NO es un
// conflicto (no compite por el mismo hueco).
func TestCloudflarePreview_DistingueLosTresEstados(t *testing.T) {
	z := &fakeZone{recs: []dns.Record{
		{ID: "r1", Type: "TXT", Name: "_sytue.acme.com", Content: `"sytue-verification=1234abcd"`},
		{ID: "r2", Type: "TXT", Name: "acme.com", Content: `"v=spf1 include:_spf.google.com ~all"`},
		{ID: "r3", Type: "TXT", Name: "acme.com", Content: `"google-site-verification=abc"`},
	}}
	s, cf, _ := cfServer(t, z)
	plan := s.cloudflarePlan(perfil(), true)

	out, applied, err := s.applyCloudflarePlan(context.Background(), cf, plan,
		cloudflareRequest{Apply: false}, "acme.com")
	if err != nil {
		t.Fatalf("previsualizar: %v", err)
	}
	if applied != 0 || z.writes != 0 {
		t.Fatalf("previsualizar NO puede escribir en la zona (applied=%d writes=%d)", applied, z.writes)
	}
	got := byKey(out)
	if st := got["txt:_sytue.acme.com"].State; st != stateOK {
		t.Fatalf("el TXT ya está y es el nuestro: state = %q", st)
	}
	if st := got["mx:acme.com"].State; st != stateMissing {
		t.Fatalf("el MX no está: state = %q", st)
	}
	spf := got["txt:acme.com"]
	if spf.State != stateConflict {
		t.Fatalf("un SPF ajeno es un conflicto, no un hueco libre: %+v", spf)
	}
	// Sin el valor actual a la vista, el cliente no puede decidir.
	if spf.Current != "v=spf1 include:_spf.google.com ~all" {
		t.Fatalf("current = %q; hay que enseñar lo que hay AHORA", spf.Current)
	}
	// El google-site-verification convive: no puede aparecer como conflicto.
	if strings.Contains(spf.Current, "google-site-verification") {
		t.Fatalf("un TXT que no compite por el hueco no es un conflicto: %q", spf.Current)
	}
}

// --- Aplicar: real e idempotente --------------------------------------------

func TestCloudflareApply_CreaDeVerdadYNoDuplica(t *testing.T) {
	z := &fakeZone{}
	s, cf, _ := cfServer(t, z)
	plan := s.cloudflarePlan(perfil(), true)
	req := cloudflareRequest{Apply: true}

	out, applied, err := s.applyCloudflarePlan(context.Background(), cf, plan, req, "acme.com")
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	// Verificación + MX + SPF. El DKIM pendiente NO se escribe.
	if applied != 3 || z.writes != 3 {
		t.Fatalf("applied=%d writes=%d; deben crearse 3 registros (el pendiente no)", applied, z.writes)
	}
	got := byKey(out)
	for _, k := range []string{"txt:_sytue.acme.com", "mx:acme.com", "txt:acme.com"} {
		if got[k].State != stateCreated {
			t.Fatalf("%s: state = %q, quiero created (%+v)", k, got[k].State, got[k])
		}
	}
	if got["txt:sytue._domainkey.acme.com"].State != statePending {
		t.Fatalf("el DKIM debe seguir pendiente: %+v", got["txt:sytue._domainkey.acme.com"])
	}
	// El registro creado en la zona es el correcto, con su prioridad.
	var mx *dns.Record
	for i := range z.recs {
		if z.recs[i].Type == "MX" {
			mx = &z.recs[i]
		}
	}
	if mx == nil || mx.Content != "mx.sytue.test" || mx.Priority != 10 {
		t.Fatalf("el MX de la zona no es el esperado: %+v", mx)
	}
	// El TXT de verificación está: el dominio queda verificado en el mismo paso.
	if !verifiedByZone(out) {
		t.Fatal("con el TXT creado y releído, el dominio debe quedar verificado")
	}

	// Segunda pasada: nada que hacer. Ni una escritura más, ni un duplicado.
	out2, applied2, err := s.applyCloudflarePlan(context.Background(), cf, plan, req, "acme.com")
	if err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if applied2 != 0 || z.writes != 3 {
		t.Fatalf("la segunda pasada no debe escribir (applied=%d writes=%d)", applied2, z.writes)
	}
	if len(z.recs) != 3 {
		t.Fatalf("la zona tiene %d registros: se han duplicado", len(z.recs))
	}
	for k, w := range byKey(out2) {
		if w.State != stateOK && w.State != statePending {
			t.Fatalf("%s: state = %q; en la segunda pasada ya está todo puesto", k, w.State)
		}
	}
	if !verifiedByZone(out2) {
		t.Fatal("el dominio sigue verificado en la segunda pasada")
	}
}

// --- Conflictos: nada se pisa sin permiso explícito -------------------------

func TestCloudflareApply_NoPisaSinPermiso(t *testing.T) {
	z := &fakeZone{recs: []dns.Record{
		{ID: "r2", Type: "TXT", Name: "acme.com", Content: `"v=spf1 include:_spf.google.com -all"`},
	}}
	s, cf, _ := cfServer(t, z)
	plan := s.cloudflarePlan(perfil(), true)

	out, _, err := s.applyCloudflarePlan(context.Background(), cf, plan,
		cloudflareRequest{Apply: true}, "acme.com")
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	spf := byKey(out)["txt:acme.com"]
	if spf.State != stateConflict {
		t.Fatalf("sin permiso, el conflicto se queda como está: %+v", spf)
	}
	if z.recs[0].Content != `"v=spf1 include:_spf.google.com -all"` {
		t.Fatalf("¡se ha pisado el SPF del cliente sin permiso!: %+v", z.recs[0])
	}
	// El resto sí se crea: un conflicto no bloquea lo que no choca.
	if byKey(out)["txt:_sytue.acme.com"].State != stateCreated {
		t.Fatal("un conflicto en un registro no puede impedir crear los demás")
	}
}

func TestCloudflareApply_SustituyeConPermiso(t *testing.T) {
	z := &fakeZone{recs: []dns.Record{
		{ID: "r2", Type: "TXT", Name: "acme.com", Content: `"v=spf1 include:_spf.google.com -all"`},
	}}
	s, cf, _ := cfServer(t, z)
	plan := s.cloudflarePlan(perfil(), true)

	out, applied, err := s.applyCloudflarePlan(context.Background(), cf, plan,
		cloudflareRequest{Apply: true, Replace: []string{"TXT:acme.com"}}, "acme.com")
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	spf := byKey(out)["txt:acme.com"]
	if spf.State != stateReplaced {
		t.Fatalf("con permiso explícito sí se sustituye: %+v", spf)
	}
	if spf.Current != "" {
		t.Fatalf("ya no hay nada estorbando: current = %q", spf.Current)
	}
	if z.recs[0].Content != "v=spf1 mx ~all" {
		t.Fatalf("el SPF de la zona no se ha sustituido: %+v", z.recs[0])
	}
	// Verificación + MX creados, SPF sustituido.
	if applied != 3 {
		t.Fatalf("applied = %d, quiero 3", applied)
	}
}

// --- Fallo parcial: se cuenta, no se disimula -------------------------------

// Cloudflare rechaza UN registro: ese sale como fallido con su motivo, los demás
// se crean, y la operación NO se da por buena en bloque.
func TestCloudflareApply_FalloParcialSeReporta(t *testing.T) {
	z := &fakeZone{rejects: "MX"}
	s, cf, logged := cfServer(t, z)
	plan := s.cloudflarePlan(perfil(), true)

	out, applied, err := s.applyCloudflarePlan(context.Background(), cf, plan,
		cloudflareRequest{Apply: true}, "acme.com")
	if err != nil {
		t.Fatalf("un fallo de UN registro no puede tumbar la operación entera: %v", err)
	}
	got := byKey(out)
	mx := got["mx:acme.com"]
	if mx.State != stateFailed {
		t.Fatalf("el MX rechazado debe salir como failed: %+v", mx)
	}
	if !strings.Contains(mx.Detail, "content is invalid") {
		t.Fatalf("hay que decir POR QUÉ falló, con el motivo real: %q", mx.Detail)
	}
	if got["txt:_sytue.acme.com"].State != stateCreated || got["txt:acme.com"].State != stateCreated {
		t.Fatalf("los demás registros sí se crean: %+v", got)
	}
	if applied != 2 {
		t.Fatalf("applied = %d; solo cuentan los que se aplicaron de verdad", applied)
	}
	// Nada en el log delata el token del cliente (y sí se ha registrado el fallo).
	if !strings.Contains(logged.String(), "escribir en la zona del cliente falló") {
		t.Fatalf("el fallo debe quedar registrado para el operador: %s", logged.String())
	}
	if strings.Contains(logged.String(), tokenDelCliente) {
		t.Fatal("¡el token del cliente ha acabado en el log!")
	}
}

// --- El token es un secreto de tercero --------------------------------------

// Ni la previsualización ni el resultado pueden devolver el token: el navegador
// lo envió, pero no vuelve nunca (ni entero ni troceado).
func TestCloudflare_TokenNoVuelveEnLaRespuesta(t *testing.T) {
	z := &fakeZone{}
	s, cf, logged := cfServer(t, z)
	plan := s.cloudflarePlan(perfil(), true)

	out, _, err := s.applyCloudflarePlan(context.Background(), cf, plan,
		cloudflareRequest{Apply: true, Token: tokenDelCliente}, "acme.com")
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	body, _ := json.Marshal(out)
	if strings.Contains(string(body), tokenDelCliente) {
		t.Fatalf("¡el token del cliente sale en la respuesta!: %s", body)
	}
	if strings.Contains(logged.String(), tokenDelCliente) {
		t.Fatalf("¡el token del cliente sale en el log!: %s", logged.String())
	}
}

// --- Comparación de valores --------------------------------------------------

// Cloudflare devuelve los TXT entrecomillados y los destinos con punto final: si
// no se normaliza, un registro correcto pasaría por «falta» y se duplicaría.
func TestCloudflareApply_ToleraComillasYPuntoFinal(t *testing.T) {
	z := &fakeZone{recs: []dns.Record{
		{ID: "r1", Type: "TXT", Name: "_sytue.acme.com", Content: `"sytue-verification=1234abcd"`},
		{ID: "r2", Type: "MX", Name: "acme.com", Content: "mx.sytue.test.", Priority: 10},
	}}
	s, cf, _ := cfServer(t, z)
	plan := s.cloudflarePlan(perfil(), true)

	out, applied, err := s.applyCloudflarePlan(context.Background(), cf, plan,
		cloudflareRequest{Apply: true}, "acme.com")
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	got := byKey(out)
	if got["txt:_sytue.acme.com"].State != stateOK || got["mx:acme.com"].State != stateOK {
		t.Fatalf("un registro ya correcto no se vuelve a crear: %+v", got)
	}
	if applied != 1 || z.writes != 1 { // solo el SPF, que sí faltaba
		t.Fatalf("applied=%d writes=%d; solo faltaba el SPF", applied, z.writes)
	}
}

// Un MX con la misma máquina pero OTRA prioridad no es el nuestro: es un conflicto
// (la prioridad decide por dónde entra el correo).
func TestCloudflareApply_PrioridadDistintaEsConflicto(t *testing.T) {
	z := &fakeZone{recs: []dns.Record{
		{ID: "r2", Type: "MX", Name: "acme.com", Content: "mx.sytue.test", Priority: 50},
	}}
	s, cf, _ := cfServer(t, z)
	plan := s.cloudflarePlan(perfil(), true)

	out, _, err := s.applyCloudflarePlan(context.Background(), cf, plan,
		cloudflareRequest{Apply: true}, "acme.com")
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	if st := byKey(out)["mx:acme.com"].State; st != stateConflict {
		t.Fatalf("otra prioridad = conflicto, no «ya está»: %q", st)
	}
}

// --- Verificación: solo la decide el TXT ------------------------------------

// Si el TXT de verificación falla, el dominio NO queda verificado por mucho que
// el resto haya salido bien.
func TestVerifiedByZone_SoloCuentaElTXTDeVerificacion(t *testing.T) {
	if verifiedByZone([]recordWire{
		{Type: "TXT", Name: "_sytue.acme.com", State: stateFailed},
		{Type: "MX", Name: "acme.com", State: stateCreated},
	}) {
		t.Fatal("sin el TXT no hay verificación")
	}
	if verifiedByZone([]recordWire{{Type: "MX", Name: "acme.com", State: stateCreated}}) {
		t.Fatal("sin registro de verificación en la lista, no se verifica nada")
	}
	if !verifiedByZone([]recordWire{{Type: "TXT", Name: "_sytue.acme.com", State: stateOK}}) {
		t.Fatal("el TXT ya puesto y correcto verifica el dominio")
	}
}
