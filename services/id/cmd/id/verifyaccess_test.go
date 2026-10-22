// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/totp"
)

// Las cuentas efímeras solo son seguras si se pueden BORRAR: el barrido las
// busca por prefijo, así que un email o un slug fuera del prefijo dejaría
// credenciales conocidas vivas en producción sin que nadie lo notara.
func TestNuevaIdentidadEsEfimeraYUsable(t *testing.T) {
	id, err := nuevaIdentidad("tenant", "ab12cd34", "sytue.eu", verifyPrefix+"ab12cd34")
	if err != nil {
		t.Fatalf("nuevaIdentidad: %v", err)
	}
	if !strings.HasPrefix(id.Email, verifyPrefix) {
		t.Fatalf("el email %q no lo alcanza el barrido por prefijo %q", id.Email, verifyPrefix)
	}
	if !strings.HasSuffix(id.Email, "@sytue.eu") {
		t.Fatalf("email fuera del dominio verificado: %q", id.Email)
	}
	// El slug ES el tenant y la dirección <slug>.sytue.eu: uno inválido rompería
	// el alta de la organización cliente solo en producción.
	if !tenancy.ValidSlug(id.OrgSlug) {
		t.Fatalf("slug de organización efímera inválido: %q", id.OrgSlug)
	}
	if !strings.HasPrefix(id.OrgSlug, verifyPrefix) {
		t.Fatalf("el slug %q no lo alcanza el barrido por prefijo", id.OrgSlug)
	}
	if err := domain.ValidatePassword(domain.DefaultPasswordPolicy, id.Password); err != nil {
		t.Fatalf("la contraseña generada no cumple la política del propio alta: %v", err)
	}
	// Sin una semilla válida el segundo factor nunca se completaría y la
	// verificación fallaría culpando al despliegue.
	ahora := time.Unix(1_700_000_000, 0)
	if code := totp.Code(id.Secret, ahora); len(code) != 6 {
		t.Fatalf("código TOTP inesperado: %q", code)
	}
	if !totp.Validate(id.Secret, totp.Code(id.Secret, ahora), ahora) {
		t.Fatal("el código generado no lo acepta el propio validador del IdP")
	}
}

// Dos ejecuciones no pueden colisionar: el sufijo aleatorio es lo que permite
// que un barrido incompleto no bloquee el despliegue siguiente.
func TestNuevaIdentidadNoColisionaEntreRoles(t *testing.T) {
	admin, err := nuevaIdentidad("platform", "ab12cd34", "sytue.eu", "")
	if err != nil {
		t.Fatalf("nuevaIdentidad: %v", err)
	}
	inquilino, err := nuevaIdentidad("tenant", "ab12cd34", "sytue.eu", verifyPrefix+"ab12cd34")
	if err != nil {
		t.Fatalf("nuevaIdentidad: %v", err)
	}
	if admin.Email == inquilino.Email {
		t.Fatal("las dos cuentas comparten email: la prueba de denegación no probaría nada")
	}
	if admin.Password == inquilino.Password {
		t.Fatal("las dos cuentas comparten contraseña")
	}
}

// consolaFalsa levanta una consola global que se comporta como la de verdad:
// niega con el código que corresponda mientras no haya elevación viva y la
// concede por `/api/platform/activate`. Devuelve el cliente ya apuntado a ella y
// el contador de elevaciones pedidas.
func consolaFalsa(t *testing.T, motivoInicial string, elevable bool) (*http.Client, *int) {
	t.Helper()
	elevado := false
	elevaciones := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/platform/activate":
			elevaciones++
			if !elevable {
				http.Error(w, `{"error":"upstream","status":403,"code":"forbidden"}`, http.StatusForbidden)
				return
			}
			// La API exige motivo y una ventana de 5 a 480 minutos: un cuerpo que
			// no los cumpla haría fallar la elevación SOLO en producción.
			var req struct {
				Justification   string `json:"justification"`
				DurationMinutes int    `json:"duration_minutes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("cuerpo de elevación ilegible: %v", err)
			}
			if strings.TrimSpace(req.Justification) == "" {
				t.Error("la elevación va sin motivo: la API la rechazaría")
			}
			if req.DurationMinutes < 5 || req.DurationMinutes > 480 {
				t.Errorf("ventana fuera de rango: %d", req.DurationMinutes)
			}
			elevado = true
			_, _ = w.Write([]byte(`{"activation":{"status":"active"}}`))
		case "/api/platform/usage":
			if elevado {
				_, _ = w.Write([]byte(`{"orgs":1}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"upstream","status":403,"code":"` + motivoInicial + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	destino := srv.Listener.Addr().String()
	cl := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // certificado efímero del propio test
		DialContext: func(ctx context.Context, red, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, red, destino)
		},
	}}
	return cl, &elevaciones
}

// El recorrido completo: sin elevar niega con `activation_required`, la
// elevación se pide, y solo DESPUÉS la consola responde. Es el contrato que el
// despliegue daba por roto («la consola global es inalcanzable») cuando lo único
// que pasaba es que nadie se elevaba.
func TestAlcanzarConsolaGlobalSeElevaAntesDeEntrar(t *testing.T) {
	cl, elevaciones := consolaFalsa(t, "activation_required", true)
	if err := alcanzarConsolaGlobal(t.Context(), cl, "platform.sytue.eu"); err != nil {
		t.Fatalf("la consola global debería ser alcanzable elevándose: %v", err)
	}
	if *elevaciones != 1 {
		t.Fatalf("elevaciones pedidas = %d, se esperaba exactamente 1", *elevaciones)
	}
}

// Y el fallo que NO se puede tapar elevándose: quien no está en la allowlist
// recibe el 403 parco. Si el verificador intentara elevarse igualmente, un admin
// global que se quedó fuera de `platform_admins` pasaría por «solo le faltaba
// elevarse» y el despliegue lo daría por bueno.
func TestAlcanzarConsolaGlobalNoTapaLaAllowlist(t *testing.T) {
	cl, elevaciones := consolaFalsa(t, "forbidden", true)
	err := alcanzarConsolaGlobal(t.Context(), cl, "platform.sytue.eu")
	if err == nil {
		t.Fatal("un admin fuera de platform_admins no puede dar la consola por alcanzable")
	}
	if !strings.Contains(err.Error(), "platform_admins") {
		t.Fatalf("el error no señala la allowlist: %v", err)
	}
	if *elevaciones != 0 {
		t.Fatalf("se pidió elevación (%d) ante un 403 que no era de elevación", *elevaciones)
	}
}

// Un `amr` sin segundo factor se arregla en el step-up de /authorize, no en la
// allowlist ni elevándose: decir cuál de los tres candados cerró es la mitad del
// valor de esta comprobación.
func TestAlcanzarConsolaGlobalDistingueElSegundoFactor(t *testing.T) {
	cl, elevaciones := consolaFalsa(t, "mfa_required", true)
	err := alcanzarConsolaGlobal(t.Context(), cl, "platform.sytue.eu")
	if err == nil || !strings.Contains(err.Error(), "amr") {
		t.Fatalf("el error no señala el segundo factor: %v", err)
	}
	if *elevaciones != 0 {
		t.Fatalf("se pidió elevación (%d) sin segundo factor", *elevaciones)
	}
}
