// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestAlta recorre el asistente contra un Sytue ID de mentira: lo que se lee, lo que se
// manda y a qué ruta, tal y como lo hace el proxy de la web.
//
// Visto en ROJO a mano (en el frontal): con `Guardar` mandando `billing` aunque venga
// nil —sin el `omitempty`—, el servicio recibiría `"billing":null` y el test lo dice.
func TestAlta(t *testing.T) {
	cuerpos := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clave := r.Method + " " + r.URL.Path
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			cuerpos[clave] = string(b)
		}
		switch clave {
		case "GET /api/v1/onboarding/":
			_, _ = io.WriteString(w, `{"mode":"organization","step":"personas","completed":false,"suite_domain":"sytue.eu",`+
				`"account":{"email":"ada@acme.eu","name":"Ada"},`+
				`"organization":{"id":"o1","slug":"acme","name":"Acme","subdomain":"acme.sytue.eu","dns":{"host":"acme.sytue.eu","status":"live"}},`+
				`"domain":{"host":"acme.eu","record_name":"_sytue.acme.eu","record_value":"sytue-verify=x","verified":false},`+
				`"billing":{"legal_name":"Acme SL","tax_id":"B1","address":"","billing_email":"f@acme.eu"},`+
				`"invitations":[{"email":"b@acme.eu","role":"member"}],"roles":[{"id":"r1","name":"Finanzas","permissions":["billing:read"]}],`+
				`"recommended_roles":[{"key":"finance","name":"Finanzas","summary":"Ve las facturas","permissions":["billing:read"]}]}`)
		case "PATCH /api/v1/onboarding/":
			_, _ = io.WriteString(w, `{"mode":"organization","step":"facturacion","completed":false}`)
		case "GET /api/v1/onboarding/slug":
			_, _ = io.WriteString(w, `{"slug":"`+r.URL.Query().Get("slug")+`","available":true,"subdomain":"acme.sytue.eu"}`)
		case "POST /api/v1/onboarding/organization":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"o1","slug":"acme","name":"Acme","subdomain":"acme.sytue.eu","dns":{"host":"acme.sytue.eu","status":"unconfigured"}}`)
		case "GET /api/v1/onboarding/subscription":
			_, _ = io.WriteString(w, `{"status":"pending","seats":1,"active_members":2,"unit_price_cents":500,"currency":"EUR","paid_until":null,"entitled":false}`)
		case "POST /api/v1/onboarding/subscription/checkout":
			_, _ = io.WriteString(w, `{"checkout_url":"https://pago.ejemplo/c/1","amount_cents":1500,"seats":3}`)
		case "POST /api/v1/onboarding/domain":
			_, _ = io.WriteString(w, `{"host":"acme.eu","record_name":"_sytue.acme.eu","record_value":"sytue-verify=x","verified":false,"status":"mismatch","found":["otro"]}`)
		case "POST /api/v1/onboarding/domain/cloudflare":
			_, _ = io.WriteString(w, `{"zone":{"id":"z","name":"acme.eu","status":"active"},"records":[{"key":"verify","type":"TXT","name":"_sytue.acme.eu","content":"sytue-verify=x","purpose":"Prueba","state":"conflict","current":"viejo"}],"applied":0,"token_stored":false,"domain":{"host":"acme.eu","verified":false}}`)
		case "GET /api/v1/onboarding/domain-availability":
			_, _ = io.WriteString(w, `{"domain":"acme.eu","status":"available","purchasable_here":false,"price":{"currency":"EUR","registration":"9.10","renewal":"9.10"}}`)
		case "POST /api/v1/onboarding/invitations":
			_, _ = io.WriteString(w, `{"sent":1,"results":[{"email":"b@acme.eu","status":"sent"},{"email":"x","status":"invalid","detail":"No parece una dirección de correo."}]}`)
		case "POST /api/v1/onboarding/roles":
			_, _ = io.WriteString(w, `{"results":[{"key":"finance","status":"created"}]}`)
		default:
			t.Errorf("petición inesperada: %s", clave)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	a, ctx := NewAlta(srv.URL, nil, nil), context.Background()

	e, err := a.Estado(ctx)
	if err != nil || e.Mode != AltaOrganizacion || e.Step != AltaPasoPersonas || e.Organization == nil ||
		e.Organization.DNS.Status != "live" || e.Domain.RecordName != "_sytue.acme.eu" || e.Billing.LegalName != "Acme SL" ||
		len(e.Invitations) != 1 || len(e.Roles) != 1 || e.RecommendedRoles[0].Key != "finance" || e.Account.Email != "ada@acme.eu" {
		t.Fatalf("estado: %+v, %v", e, err)
	}
	if err := a.Guardar(ctx, AltaGuardado{Mode: AltaOrganizacion, Step: AltaPasoFacturacion}); err != nil {
		t.Fatal(err)
	}
	if c := cuerpos["PATCH /api/v1/onboarding/"]; c != `{"mode":"organization","step":"facturacion"}` {
		t.Errorf("guardar manda %s", c)
	}
	if s, err := a.Identificador(ctx, "acme"); err != nil || !s.Available || s.Subdomain != "acme.sytue.eu" {
		t.Errorf("identificador: %+v, %v", s, err)
	}
	if o, err := a.CrearOrganizacion(ctx, "Acme", "acme"); err != nil || o.ID != "o1" || o.DNS.Status != "unconfigured" {
		t.Errorf("crear: %+v, %v", o, err)
	}
	if s, err := a.Suscripcion(ctx); err != nil || s.UnitPriceCents != 500 || s.ActiveMembers != 2 || s.Entitled {
		t.Errorf("suscripción: %+v, %v", s, err)
	}
	if u, err := a.Pagar(ctx, 3); err != nil || u != "https://pago.ejemplo/c/1" || cuerpos["POST /api/v1/onboarding/subscription/checkout"] != `{"seats":3}` {
		t.Errorf("pagar: %q, %v", u, err)
	}
	if d, err := a.Dominio(ctx, "acme.eu"); err != nil || d.Status != "mismatch" || d.Found[0] != "otro" {
		t.Errorf("dominio: %+v, %v", d, err)
	}
	cf, err := a.Cloudflare(ctx, AltaCloudflarePeticion{Domain: "acme.eu", Token: "t"})
	if err != nil || cf.Zone.Name != "acme.eu" || cf.Records[0].State != "conflict" || cf.Records[0].Current != "viejo" {
		t.Errorf("cloudflare: %+v, %v", cf, err)
	}
	// `replace` viaja como lista vacía, no como null: el servicio la recorre.
	var enviado map[string]any
	_ = json.Unmarshal([]byte(cuerpos["POST /api/v1/onboarding/domain/cloudflare"]), &enviado)
	if l, ok := enviado["replace"].([]any); !ok || len(l) != 0 {
		t.Errorf("replace viaja como %v", enviado["replace"])
	}
	if d, err := a.Disponibilidad(ctx, "acme.eu"); err != nil || d.Status != "available" || d.Price.Registration != "9.10" {
		t.Errorf("disponibilidad: %+v, %v", d, err)
	}
	if n, res, err := a.Invitar(ctx, "b@acme.eu, x", "member"); err != nil || n != 1 || len(res) != 2 || res[1].Status != "invalid" {
		t.Errorf("invitar: %d %+v %v", n, res, err)
	}
	if err := a.CrearRoles(ctx, []string{"finance"}); err != nil || cuerpos["POST /api/v1/onboarding/roles"] != `{"keys":["finance"]}` {
		t.Errorf("roles: %v, %s", err, cuerpos["POST /api/v1/onboarding/roles"])
	}
}

// TestElAltaHablaElIdiomaDeSytueID ata el cliente a los fuentes del servicio: las rutas
// montadas y los campos que se leen y se mandan.
func TestElAltaHablaElIdiomaDeSytueID(t *testing.T) {
	for fichero, literales := range map[string][]string{
		"services/id/internal/http/server.go": {
			`r.Route("/onboarding"`, `r.Get("/", s.handleOnboardingGet)`, `r.Patch("/", s.handleOnboardingPatch)`,
			`r.Get("/slug", s.handleOnboardingSlug)`, `r.Post("/organization", s.handleOnboardingCreateOrg)`,
			`r.Get("/subscription", s.handleSubscriptionStatus)`, `r.Post("/subscription/checkout", s.handleSubscriptionCheckout)`,
			`Post("/domain", s.handleOnboardingDomain)`, `Post("/domain/cloudflare", s.handleOnboardingDomainCloudflare)`,
			`Get("/domain-availability", s.handleOnboardingDomainAvailability)`,
			`Post("/invitations", s.handleOnboardingInvite)`, `Post("/roles", s.handleOnboardingRoles)`,
		},
		"services/id/internal/http/onboarding.go": {
			`"mode":              ob.Mode`, `"step":              ob.Step`, `"completed":         completado`,
			`"suite_domain":`, `"recommended_roles":`, `"organization":`, `out["domain"]`, `out["billing"]`,
			`out["invitations"]`, `out["roles"]`, `"account"`,
			"Mode      string `json:\"mode\"`", "Step      string `json:\"step\"`", "Completed bool   `json:\"completed\"`",
			"Billing *billingRequest `json:\"billing\"`", "LegalName    string `json:\"legal_name\"`",
			"TaxID        string `json:\"tax_id\"`", "BillingEmail string `json:\"billing_email\"`",
			`"subdomain":  subdomain`, `"dns":        dnsWire`, `"record_name":`, `"record_value":`, `"verified":`,
			`res["reason"]`, `res["subdomain"]`, `"purchasable_here": false`, `"registration":`, `"renewal":`,
			"Emails string `json:\"emails\"`", "Role   string `json:\"role\"`", `"sent": sent, "results": results`,
			"Keys []string `json:\"keys\"`",
		},
		"services/id/internal/http/onboarding_cloudflare.go": {
			"Domain string `json:\"domain\"`", "Token string `json:\"token\"`", "Mail bool `json:\"mail\"`",
			"Apply bool `json:\"apply\"`", "Replace []string `json:\"replace\"`", "RememberToken bool `json:\"remember_token\"`",
			`"zone":`, `"records": out`, `"applied": applied`, `"token_stored":`,
		},
		"services/id/internal/http/subscriptions.go": {
			"Seats int `json:\"seats\"`", `"checkout_url": checkout.CheckoutURL`, `"seats": sub.Seats, "active_members": members`,
			`"unit_price_cents":`, `"entitled": sub.Entitled(`,
		},
	} {
		fuente := webref.Read(t, fichero)
		for _, literal := range literales {
			if !strings.Contains(fuente, literal) {
				t.Errorf("%s ya no dice %s: revisar alta.go", fichero, literal)
			}
		}
	}
}
