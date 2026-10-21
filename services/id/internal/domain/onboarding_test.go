// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests OFFLINE de las reglas del alta inicial: recorridos, identificador,
// dominio de la organización y catálogo de roles recomendados. Todo lo que el
// usuario ve como «disponible» o «este rol podrá hacer X» se decide aquí; la
// verificación del TXT se prueba en platform/pkg/dominio, donde vive.
package domain

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestOnboardingStepsIndividualSaltaPersonasYRoles(t *testing.T) {
	ind := OnboardingSteps(OnboardingIndividual)
	for _, s := range ind {
		if s == StepPeople || s == StepRoles {
			t.Fatalf("el recorrido individual no debe incluir %q: %v", s, ind)
		}
	}
	org := OnboardingSteps(OnboardingOrganization)
	var people, roles bool
	for _, s := range org {
		people = people || s == StepPeople
		roles = roles || s == StepRoles
	}
	if !people || !roles {
		t.Fatalf("el recorrido de organización debe incluir personas y roles: %v", org)
	}
	if org[0] != StepMode || org[len(org)-1] != StepDone {
		t.Fatalf("orden inesperado: %v", org)
	}
}

func TestValidOnboardingStep(t *testing.T) {
	if ValidOnboardingStep(OnboardingIndividual, StepRoles) {
		t.Fatal("«roles» no pertenece al recorrido individual")
	}
	if !ValidOnboardingStep(OnboardingIndividual, StepBilling) {
		t.Fatal("«facturacion» sí pertenece al recorrido individual")
	}
	if ValidOnboardingStep(OnboardingOrganization, "paso-inventado") {
		t.Fatal("un paso inventado por el navegador no puede guardarse")
	}
}

func TestSlugReservado(t *testing.T) {
	for _, s := range []string{"id", "mail", "admin", "WWW", " drive "} {
		if !SlugReserved(s) {
			t.Fatalf("%q debe estar reservado (colisiona con un host de la suite)", s)
		}
	}
	if SlugReserved("acme") {
		t.Fatal("acme no está reservado")
	}
	// Hosts de Cloud bajo el apex (apps ya estaba): preview/ingress/registry.
	for _, s := range []string{"preview", "ingress", "registry", "apps"} {
		if !SlugReserved(s) || SlugAllowedForOrg(s) {
			t.Fatalf("%q (host de Cloud) debe estar reservado para una organización", s)
		}
	}
	// Prefijo nas-: el túnel de Drive de `foo` vive en `nas-foo.<dominio>`, así que
	// una organización `nas-foo` lo pisaría. Reservado como prefijo, igual que `p-`.
	for _, s := range []string{"nas-foo", "NAS-acme", "nas-"} {
		if !SlugReserved(s) || SlugAllowedForOrg(s) {
			t.Fatalf("%q empieza por el prefijo reservado nas- y no debe poder ser una organización", s)
		}
	}
	// Pero un nombre que solo CONTIENE «nas» sin el guion sí se permite.
	if SlugReserved("nasa") || !SlugAllowedForOrg("nasa") {
		t.Fatal("«nasa» no empieza por nas- y no debe estar reservado")
	}
}

// TestSinOrgDeLaWebEstaReservada es el guardián de los nombres de página de
// la lista de reservados (platform/pkg/tenancy/reservados.go). La fuente de
// verdad son los `sinOrg` de las apps web
// (apps/frontend/apps/*/src/middleware.ts): ahí decide cada app qué primer
// segmento del path es una página y cuál es una organización (ADR 0056), y una
// org que tomara uno de esos nombres haría que el middleware tratara SUS
// páginas como si fueran rutas dentro de ella. Go no puede importar
// TypeScript, así que el test lee esos ficheros y compara: si una app estrena
// una página y aquí no se añade al literal, esto se pone en rojo — la lista no
// se queda vieja sola.
func TestSinOrgDeLaWebEstaReservada(t *testing.T) {
	raiz := filepath.Join("..", "..", "..", "..")
	ficheros, err := filepath.Glob(filepath.Join(raiz, "apps", "frontend", "apps", "*", "src", "middleware.ts"))
	if err != nil || len(ficheros) == 0 {
		t.Fatalf("no se encuentra apps/frontend/apps/*/src/middleware.ts (%v): ¿se movió de sitio? el guardián habría quedado desactivado", err)
	}
	// Y el de la ofimática común, que las apps de Docus, Calcus, Diapos y CertiPDF
	// reexportan en vez de escribir el suyo: sus páginas son las de ese paquete.
	ficheros = append(ficheros, filepath.Join(raiz, "apps", "frontend", "packages", "office", "src", "middleware.ts"))
	// El array `sinOrg` de cada middleware, en una línea o repartido en varias:
	// `sinOrg: ['events'],` y `sinOrg: [\n 'access',\n …\n]`. Mismo estilo con el
	// que abrir_test.go lee los conjuntos de la web.
	re := regexp.MustCompile(`(?s)sinOrg:\s*\[([^\]]*)\]`)
	for _, f := range ficheros {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("leer %s: %v", f, err)
		}
		app := filepath.Base(filepath.Dir(filepath.Dir(f)))
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			for _, nombre := range entradasSinOrg(t, filepath.Dir(f), app, m[1]) {
				// Solo protege lo que una org puede tomar: si el nombre no es un
				// slug válido ('.well-known' de Rooms, por ejemplo), ninguna org
				// puede colarse por ahí y no tiene que estar reservado.
				if !tenancy.ValidSlug(nombre) {
					continue
				}
				if !SlugReserved(nombre) {
					t.Errorf("%s declara la página %q y no está reservada: una organización podría llamarse así y su middleware la trataría como página (ADR 0056). Añádela a platform/pkg/tenancy/reservados.go", app, nombre)
				}
			}
		}
	}
}

// TestCatalogoDeProductosEstaReservado es el guardián de la parte de hosts y
// productos de la lista de reservados. La fuente de verdad es el catálogo de la web
// (apps/frontend/packages/config/src/tokens.ts): cada clave de producto del JSON
// que importa `productColor`, cada subdominio (`at('…')` de appDomains) y cada prefijo de
// app (`appPaths`). Cast, Notepads y SyPhotos estrenaron host y prefijo sin
// entrar aquí, y una organización llamada `notes` o `cast` podía darse de alta.
func TestCatalogoDeProductosEstaReservado(t *testing.T) {
	ruta := filepath.Join("..", "..", "..", "..", "apps", "frontend", "packages", "config", "src", "tokens.ts")
	b, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("leer %s: %v (¿se movió el catálogo? el guardián habría quedado desactivado)", ruta, err)
	}
	src := string(b)
	bloque := func(nombre string) string {
		m := regexp.MustCompile(`(?s)\b` + nombre + `[^=]*=\s*\{(.*?)\n\}`).FindStringSubmatch(src)
		if m == nil {
			t.Fatalf("tokens.ts: no encuentro `%s = {…}`", nombre)
		}
		return m[1]
	}
	vistos := map[string]bool{}
	if !regexp.MustCompile(`(?m)^\s*export\s+const\s+productColor\s*=\s*values\(\s*colors\.product\s*\)\s*;?\s*$`).MatchString(src) {
		t.Fatal("tokens.ts: productColor ya no deriva de colors.product; adapta el guardián a la nueva fuente")
	}
	importacion := regexp.MustCompile(`(?m)^\s*import\s+colors\s+from\s+['"]([^'"]+)['"]\s*;?\s*$`).FindStringSubmatch(src)
	if importacion == nil {
		t.Fatal("tokens.ts: no encuentro el JSON importado como colors")
	}
	paleta, err := os.ReadFile(filepath.Join(filepath.Dir(ruta), importacion[1]))
	if err != nil {
		t.Fatalf("leer el catálogo de colores: %v", err)
	}
	var colores struct {
		Product map[string]json.RawMessage `json:"product"`
	}
	if err := json.Unmarshal(paleta, &colores); err != nil || len(colores.Product) == 0 {
		t.Fatalf("el JSON de colores no contiene el catálogo de productos: %v", err)
	}
	for nombre := range colores.Product {
		if !strings.HasPrefix(nombre, "$") {
			vistos[nombre] = true
		}
	}
	if len(vistos) == 0 {
		t.Fatal("el catálogo de colores no contiene productos, solo notas")
	}
	for _, m := range regexp.MustCompile(`at\('([a-z0-9-]+)'\)`).FindAllStringSubmatch(bloque("appDomains"), -1) {
		vistos[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`'/([a-z0-9-]+)'`).FindAllStringSubmatch(bloque("appPaths"), -1) {
		vistos[m[1]] = true
	}
	for _, imprescindible := range []string{"drive", "admin", "cast"} {
		if !vistos[imprescindible] {
			t.Fatalf("tokens.ts: no he leído %q; el guardián no está mirando donde cree", imprescindible)
		}
	}
	for nombre := range vistos {
		if !SlugReserved(nombre) {
			t.Errorf("el catálogo de productos usa %q (producto, host o prefijo) y no está reservado: una organización podría tomarlo. Añádelo a platform/pkg/tenancy/reservados.go", nombre)
		}
	}
}

// entradasSinOrg lee los nombres de un array `sinOrg`. Además de literales
// admite `...CONSTANTE`, que busca como `CONSTANTE = [ ... ]` en los .ts de la
// app (Cast la comparte con su enrutado). Cualquier otra cosa es un error y no
// un salto: un spread que el test no seguía dejó las nueve páginas de Cast sin
// reservar, con el guardián en verde.
func entradasSinOrg(t *testing.T, src, app, cuerpo string) []string {
	t.Helper()
	var out []string
	for _, crudo := range strings.Split(cuerpo, ",") {
		e := strings.TrimSpace(crudo)
		switch {
		case e == "":
		case strings.HasPrefix(e, "'") || strings.HasPrefix(e, "\""):
			out = append(out, strings.Trim(e, "'\""))
		case strings.HasPrefix(e, "..."):
			out = append(out, constanteDeLaApp(t, src, app, strings.TrimPrefix(e, "..."))...)
		default:
			t.Errorf("%s: no sé leer la entrada %q de sinOrg; usa literales o `...CONSTANTE` con un array literal", app, e)
		}
	}
	return out
}

func constanteDeLaApp(t *testing.T, src, app, nombre string) []string {
	t.Helper()
	re := regexp.MustCompile(`(?s)\b` + regexp.QuoteMeta(nombre) + `\s*=\s*\[([^\]]*)\]`)
	var out []string
	encontrada := false
	_ = filepath.WalkDir(src, func(ruta string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(ruta, ".ts") {
			return err
		}
		b, err := os.ReadFile(ruta)
		if err != nil {
			return err
		}
		if m := re.FindStringSubmatch(string(b)); m != nil && !encontrada {
			encontrada = true
			out = entradasSinOrg(t, src, app, m[1])
		}
		return nil
	})
	if !encontrada {
		t.Errorf("%s: sinOrg usa ...%s y no encuentro `%s = [...]` en %s", app, nombre, nombre, src)
	}
	return out
}

func TestNormalizeDomain(t *testing.T) {
	ok := map[string]string{
		"ACME.com":                "acme.com",
		"https://www.acme.com/":   "acme.com",
		"  acme.com.  ":           "acme.com",
		"ana@acme.co.uk":          "acme.co.uk",
		"https://acme.com:443/ab": "acme.com",
	}
	for in, want := range ok {
		got, err := NormalizeDomain(in)
		if err != nil || got != want {
			t.Fatalf("NormalizeDomain(%q) = %q, %v; esperado %q", in, got, err, want)
		}
	}
	// «www.com» sin el «www.» se queda en una extensión suelta: no es un dominio.
	for _, bad := range []string{"", "acme", "-acme.com", "acme .com", "acme..com", "acmé.com", "www.com"} {
		got, err := NormalizeDomain(bad)
		if err == nil {
			t.Fatalf("NormalizeDomain(%q) debía fallar; devolvió %q", bad, got)
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NormalizeDomain(%q): %v; la capa http espera ErrInvalidInput", bad, err)
		}
	}
}

func TestRolesRecomendadosTienenPermisosReales(t *testing.T) {
	want := map[string][]Permission{
		"soporte":      {PermUsersRead, PermUsersManage},
		"facturacion":  {PermBillingManage},
		"seguridad":    {PermAccessPoliciesManage, PermSSOManage, PermAuditView},
		"aplicaciones": {PermAppsManage},
	}
	if len(RecommendedRoles) != len(want) {
		t.Fatalf("catálogo de %d roles; el test cubre %d", len(RecommendedRoles), len(want))
	}
	for _, rec := range RecommendedRoles {
		exp, ok := want[rec.Key]
		if !ok {
			t.Fatalf("rol recomendado sin cobertura: %q", rec.Key)
		}
		if len(rec.Permissions) != len(exp) {
			t.Fatalf("%s: permisos=%v, esperado %v", rec.Key, rec.Permissions, exp)
		}
		for _, p := range exp {
			if !HasPermission(nil, rec.PermissionStrings(), p) {
				t.Fatalf("%s: falta el permiso %q", rec.Key, p)
			}
		}
		// Se crean con la MISMA validación que un rol hecho a mano: ningún permiso
		// fuera del catálogo y ninguno no delegable (roles.manage).
		norm, valid := NormalizePermissions(rec.PermissionStrings())
		if !valid {
			t.Fatalf("%s: permisos no concedibles a un rol personalizado", rec.Key)
		}
		if len(norm) != len(exp) {
			t.Fatalf("%s: normalizados=%v", rec.Key, norm)
		}
		if rec.Name == "" || rec.Summary == "" {
			t.Fatalf("%s: un rol sin nombre o sin explicación llana no se ofrece", rec.Key)
		}
		if HasPermission(nil, rec.PermissionStrings(), PermRolesManage) {
			t.Fatalf("%s: ningún rol recomendado puede conceder roles.manage", rec.Key)
		}
	}
	if _, ok := RecommendedRoleByKey("inventado"); ok {
		t.Fatal("RecommendedRoleByKey no debe inventar roles")
	}
}

func TestParseInviteEmails(t *testing.T) {
	ok, bad := ParseInviteEmails("Ana@acme.com, bruno@acme.com\nana@acme.com; esto-no-es-un-correo")
	if len(ok) != 2 || ok[0] != "ana@acme.com" || ok[1] != "bruno@acme.com" {
		t.Fatalf("ok=%v (normaliza a minúsculas y deduplica)", ok)
	}
	if len(bad) != 1 || bad[0] != "esto-no-es-un-correo" {
		t.Fatalf("bad=%v (lo inválido se reporta, no rompe la tanda)", bad)
	}
}
