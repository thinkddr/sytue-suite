// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	iddb "git.sytue.eu/sytue/one/services/id/db"
)

// cspDirectiva extrae una directiva de la cabecera CSP que emite securityHeaders
// EN PRODUCCIÓN, que es la que ve un cliente. El entorno importa: la allowlist de
// orígenes de primera parte es distinta en dev.
func cspDirectiva(t *testing.T, nombre string) []string {
	t.Helper()
	cfg := Config{CookieSecure: true}
	cfg.Env = pcfg.EnvProd
	rr := httptest.NewRecorder()
	securityHeaders(cfg)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	for _, d := range strings.Split(rr.Header().Get("Content-Security-Policy"), ";") {
		campos := strings.Fields(d)
		if len(campos) > 0 && campos[0] == nombre {
			return campos[1:]
		}
	}
	t.Fatalf("la CSP del IdP no lleva la directiva %q", nombre)
	return nil
}

// TestCSPSinComodinDeHost: ni `form-action` ni `img-src` pueden autorizar por
// comodín de host.
//
// El comodín de CSP casa con CUALQUIER número de etiquetas, así que un
// `https://*.sytue.eu` incluye `*.apps.sytue.eu`, `*.preview.sytue.eu` y el
// `<slug>.sytue.eu` que el alta publica para cada organización cliente
// (onboarding.go, ensureTenantDNS): hosts que sirven código de CLIENTES. Con
// ellos en `form-action`, un formulario inyectado en la página de login puede
// postear la contraseña al host de un cliente; con ellos en `img-src`, un `<img>`
// inyectado filtra la visita. La frontera de confianza está declarada al
// contrario y por extenso en platform/pkg/config.
func TestCSPSinComodinDeHost(t *testing.T) {
	for _, dir := range []string{"form-action", "img-src"} {
		for _, origen := range cspDirectiva(t, dir) {
			if strings.Contains(origen, "*") {
				t.Errorf("%s autoriza por comodín (%q): abarca los hosts donde corre código de clientes", dir, origen)
			}
		}
	}
}

// TestCSPProdSinOrigenesDeDesarrollo: la allowlist de orígenes era UNA sola
// constante con producción y desarrollo concatenados, así que la cabecera viva de
// id.sytue.eu llevaba de verdad 17 orígenes `*.sytue.localhost` en `img-src` y en
// `form-action` — la directiva que decide a dónde puede enviarse el formulario
// que teclea la contraseña—. Ahora la lista depende del entorno.
func TestCSPProdSinOrigenesDeDesarrollo(t *testing.T) {
	for _, dir := range []string{"form-action", "img-src"} {
		for _, origen := range cspDirectiva(t, dir) {
			if strings.Contains(origen, ".localhost") {
				t.Errorf("%s autoriza un origen de desarrollo en producción: %q", dir, origen)
			}
			// El portal se publica en one.sytue.eu (hosts.portal: one), así que
			// portal.sytue.eu es NXDOMAIN: una entrada colgada de la allowlist.
			if strings.Contains(origen, "portal.sytue.eu") {
				t.Errorf("%s autoriza %q, que no resuelve", dir, origen)
			}
		}
	}
	// Y en dev la lista tiene que seguir trayéndolos, o no se puede entrar contra
	// sytue.localhost desde ninguna de las nueve superficies.
	if !strings.Contains(pcfg.SytueOperatedWebOrigins(pcfg.EnvDev), "https://one.sytue.localhost") {
		t.Error("en dev la CSP del IdP debe seguir autorizando one.sytue.localhost")
	}
}

// retiradosDelBarrido son hosts que las migraciones SIEMBRAN pero que otra
// migración posterior retira de `redirect_uris`. El test lee el SQL, no la base,
// así que los ve; no deben exigir sitio en la CSP.
var retiradosDelBarrido = map[string]string{
	"https://chat.sytue.eu": "renombrado a rooms (mig 0023) y retirado en 0041",
	"https://meet.sytue.eu": "renombrado a rooms (mig 0023) y retirado en 0041",
	// Y el tercero, que faltaba: la 0041 lo retira igual que a los otros dos, y
	// el 2026-09-11 se retiró también su 301, su certificado y su DNS. Sin esta
	// línea, el test exige sitio en la CSP para un origen que ya no existe.
	"https://console.sytue.eu": "renombrado a admin (mig 0026) y retirado en 0041",
	// El frente en Gio: lo siembra la 0070 y la 0096 borra el client entero.
	"https://gio.sytue.eu": "cliente sytue-gio dado de alta en 0070 y de baja en 0096",
	// La app conjunta de Docs: la 0112 borra el client `sytue-docs` (ADR 0076). Su host
	// sigue siendo el de la API del servicio, pero ya no hay web que barrer.
	"https://docs.sytue.eu": "cliente sytue-docs de la 0002, borrado en 0112",
}

// TestCSPCubreElBarridoDeCierre: el barrido de cierre de sesión carga el
// `/logout` de cada superficie como <img> (logout.go), así que `img-src` tiene
// que autorizar el origen de TODOS los clients de primera parte —y `form-action`
// el final de la cadena de redirects del POST de login, que acaba en el mismo
// sitio—.
//
// La lista de verdad no vive en un fichero de configuración: vive en los
// `redirect_uris` que siembran las migraciones. Por eso el test los lee de ahí, y
// eso es justo lo que le da valor: acotar la CSP a la allowlist de CORS —que es
// lo que parece obvio— deja fuera la forja y la consola de plataforma, y el
// cierre de sesión de las dos deja de ejecutarse sin que nada lo diga. Un host de
// primera parte nuevo falla aquí el día que se siembra.
func TestCSPCubreElBarridoDeCierre(t *testing.T) {
	const suite = "sytue.eu" // producción; los hosts de dev van en la misma lista

	permitidos := make(map[string]bool)
	for _, o := range cspDirectiva(t, "img-src") {
		permitidos[o] = true
	}

	re := regexp.MustCompile(`https://[a-z0-9.-]+\.` + regexp.QuoteMeta(suite) + `/[^'\s,]*`)
	var vistos int
	err := fs.WalkDir(iddb.Migrations, ".", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(ruta, ".up.sql") {
			return err
		}
		sql, err := fs.ReadFile(iddb.Migrations, ruta)
		if err != nil {
			return err
		}
		for _, uri := range re.FindAllString(string(sql), -1) {
			barrido := sweepURI(uri, suite)
			if barrido == "" {
				continue // no es un callback de los que se barren (post-logout, .well-known…)
			}
			u, err := url.Parse(barrido)
			if err != nil {
				t.Fatalf("%s: URI de barrido ilegible %q: %v", ruta, barrido, err)
			}
			origen := u.Scheme + "://" + u.Host
			if _, retirado := retiradosDelBarrido[origen]; retirado {
				continue
			}
			vistos++
			if !permitidos[origen] {
				t.Errorf("img-src no autoriza %s, que el cierre de sesión pide en cada logout (%s)", origen, ruta)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("recorrer las migraciones de Sytue ID: %v", err)
	}
	if vistos == 0 {
		t.Fatal("ninguna URI de barrido en las migraciones: el test no está comprobando nada")
	}
}
