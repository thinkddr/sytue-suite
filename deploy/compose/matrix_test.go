// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compose

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Guardián del complemento Matrix de Compose (matrix.yml + matrix/configurar.py):
// es el mismo Synapse que producción (deploy/helm/charts/matrix) y no debe
// abrirse más que allí. Un cambio que active el registro, la federación o el
// panel de administración en la instalación local se publicaría tal cual en la
// suite, que lo usa en el servidor único con un dominio en Internet.

// configurarMatrix ejecuta matrix/configurar.py con secretos de prueba y devuelve
// el homeserver.yaml (JSON) ya decodificado.
func configurarMatrix(t *testing.T, entorno map[string]string) (map[string]any, string, error) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("sin python3: no se puede generar la configuración de Matrix")
	}
	cmd := exec.Command(python, "-I", filepath.Join("matrix", "configurar.py"),
		filepath.Join(raiz, "apps", "nucleo", "storage", "products.txt"))
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	for k, v := range entorno {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var salida, errores bytes.Buffer
	cmd.Stdout, cmd.Stderr = &salida, &errores
	if err := cmd.Run(); err != nil {
		return nil, errores.String(), err
	}
	var cfg map[string]any
	if err := json.Unmarshal(salida.Bytes(), &cfg); err != nil {
		t.Fatalf("configurar.py no escribe JSON válido: %v", err)
	}
	return cfg, errores.String(), nil
}

func secretosDePrueba() map[string]string {
	return map[string]string{
		"DOMINIO":                    "ejemplo.test",
		"MATRIX_DB_PASSWORD":         "prueba-bd",
		"MATRIX_REGISTRATION_SECRET": "prueba-registro",
		"MATRIX_MACAROON_SECRET":     "prueba-macaroon",
		"MATRIX_FORM_SECRET":         "prueba-formulario",
		"OIDC_MATRIX":                "prueba-oidc",
	}
}

func TestConfigurarMatrixEsLaDeProduccion(t *testing.T) {
	cfg, errores, err := configurarMatrix(t, secretosDePrueba())
	if err != nil {
		t.Fatalf("configurar.py falla: %v\n%s", err, errores)
	}

	// Las opciones cerradas de producción, con el valor que tienen en el chart.
	// Cada una se comprueba también en la plantilla del chart: si producción
	// cambia, este test obliga a decidir si la instalación de Compose la sigue.
	plantilla, err := os.ReadFile(filepath.Join(raiz, "deploy", "helm", "charts", "matrix", "templates", "runtime.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cerradas := map[string]any{
		"enable_registration":                         false,
		"allow_guest_access":                          false,
		"enable_metrics":                              false,
		"enable_search":                               false,
		"enable_room_list_search":                     false,
		"allow_public_rooms_without_auth":             false,
		"allow_public_rooms_over_federation":          false,
		"report_stats":                                false,
		"url_preview_enabled":                         false,
		"suppress_key_server_warning":                 true,
		"encryption_enabled_by_default_for_room_type": "all",
	}
	for clave, quiero := range cerradas {
		if got := cfg[clave]; got != quiero {
			t.Errorf("%s = %v; producción tiene %v", clave, got, quiero)
		}
		linea := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(clave) + `: ` + regexp.QuoteMeta(yamlEscalar(quiero)) + `\s*$`)
		if !linea.Match(plantilla) {
			t.Errorf("el chart de Matrix ya no tiene «%s: %v»: revisa si Compose debe seguirlo", clave, quiero)
		}
	}
	for _, lista := range []string{"federation_domain_whitelist", "trusted_key_servers"} {
		if v, ok := cfg[lista].([]any); !ok || len(v) != 0 {
			t.Errorf("%s = %v; debe ser una lista vacía (sin federación)", lista, cfg[lista])
		}
	}
	for _, sub := range []string{"password_config", "presence", "user_directory"} {
		m, _ := cfg[sub].(map[string]any)
		if m["enabled"] != false {
			t.Errorf("%s.enabled = %v; debe ser false", sub, m["enabled"])
		}
	}
	if m, _ := cfg["password_config"].(map[string]any); m["localdb_enabled"] != false {
		t.Errorf("password_config.localdb_enabled debe ser false: solo se entra por Sytue ID")
	}

	// Solo la API de cliente: ni federation ni keys ni media de otro servidor.
	oyentes, _ := cfg["listeners"].([]any)
	if len(oyentes) != 1 {
		t.Fatalf("listeners = %v; debe haber uno", cfg["listeners"])
	}
	oyente, _ := oyentes[0].(map[string]any)
	recursos, _ := oyente["resources"].([]any)
	if len(recursos) != 1 {
		t.Fatalf("resources = %v; debe haber uno", oyente["resources"])
	}
	nombres, _ := recursos[0].(map[string]any)["names"].([]any)
	if len(nombres) != 1 || nombres[0] != "client" {
		t.Errorf("el oyente publica %v; solo debe publicar client", nombres)
	}

	// La vuelta del SSO solo a Rooms (web y nativa) y al bucle local de la app nativa.
	sso, _ := cfg["sso"].(map[string]any)
	quiero := []any{
		"https://rooms.ejemplo.test/rooms/matrix/callback",
		"https://rooms.ejemplo.test/rooms/matrix/native/callback",
		"http://127.0.0.1:",
	}
	if got, _ := json.Marshal(sso["client_whitelist"]); string(got) != mustJSON(t, quiero) {
		t.Errorf("sso.client_whitelist = %s; quiero %s", got, mustJSON(t, quiero))
	}
	if cfg["server_name"] != "matrix.ejemplo.test" || cfg["public_baseurl"] != "https://matrix.ejemplo.test" {
		t.Errorf("server_name/public_baseurl no siguen al dominio: %v %v", cfg["server_name"], cfg["public_baseurl"])
	}

	// Sytue ID es el único proveedor: el general y uno por producto con
	// almacenamiento privado, con PKCE y sin enlazar cuentas existentes.
	productos := leerProductos(t)
	proveedores, _ := cfg["oidc_providers"].([]any)
	if len(proveedores) != 1+len(productos) {
		t.Fatalf("oidc_providers tiene %d; quiero 1 + %d productos", len(proveedores), len(productos))
	}
	vistos := map[string]bool{}
	for _, p := range proveedores {
		prov, _ := p.(map[string]any)
		id, _ := prov["idp_id"].(string)
		vistos[id] = true
		if prov["client_id"] != "sytue-matrix" || prov["issuer"] != "https://id.ejemplo.test" {
			t.Errorf("%s: client_id/issuer inesperados: %v %v", id, prov["client_id"], prov["issuer"])
		}
		if prov["pkce_method"] != "always" || prov["allow_existing_users"] != false || prov["enable_registration"] != false {
			t.Errorf("%s: debe exigir PKCE y no enlazar ni registrar usuarios", id)
		}
		if a, _ := prov["authorization_endpoint"].(string); !strings.HasPrefix(a, "https://id.ejemplo.test/") {
			t.Errorf("%s: el navegador debe ir a la autorización pública, no a %q", id, a)
		}
	}
	if !vistos["sytue"] {
		t.Error("falta el proveedor general «sytue»")
	}
	for _, p := range productos {
		if !vistos["sytue-storage-"+p] {
			t.Errorf("falta el proveedor de almacenamiento de %q", p)
		}
	}
}

func TestConfigurarMatrixExigeLosSecretos(t *testing.T) {
	for clave := range secretosDePrueba() {
		entorno := secretosDePrueba()
		delete(entorno, clave)
		if _, errores, err := configurarMatrix(t, entorno); err == nil {
			t.Errorf("sin %s configurar.py no debe generar nada", clave)
		} else if !strings.Contains(errores, clave) {
			t.Errorf("sin %s, el error no lo nombra: %q", clave, errores)
		}
	}
}

// El overlay solo publica por Traefik la API de cliente, la de medios y el SSO:
// nada de /_synapse/admin ni de federación (/_matrix/federation, /_matrix/key).
func TestMatrixYmlNoPublicaAdminNiFederacion(t *testing.T) {
	contenido, err := os.ReadFile("matrix.yml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			Image   string   `yaml:"image"`
			Labels  []string `yaml:"labels"`
			Volumes []string `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(contenido, &doc); err != nil {
		t.Fatal(err)
	}
	m, ok := doc.Services["matrix"]
	if !ok {
		t.Fatal("matrix.yml no define el servicio matrix")
	}
	if !regexp.MustCompile(`@sha256:[0-9a-f]{64}$`).MatchString(m.Image) {
		t.Errorf("la imagen de Synapse debe ir fijada por digest: %q", m.Image)
	}
	var regla string
	for _, l := range m.Labels {
		if strings.HasPrefix(l, "traefik.http.routers.matrix.rule=") {
			regla = l
		}
	}
	if regla == "" {
		t.Fatal("falta la regla de Traefik del servicio matrix")
	}
	prefijos := regexp.MustCompile("(?:PathPrefix|Path)\\(`([^`]+)`\\)").FindAllStringSubmatch(regla, -1)
	permitidos := map[string]bool{
		"/_matrix/client": true, "/_matrix/media": true, "/_synapse/client": true,
		"/.well-known/matrix/client": true,
	}
	if len(prefijos) == 0 {
		t.Fatalf("la regla no acota rutas: %q", regla)
	}
	for _, p := range prefijos {
		if !permitidos[p[1]] {
			t.Errorf("Traefik publica %q; solo cliente, medios y SSO", p[1])
		}
	}
	for _, v := range m.Volumes {
		if strings.Contains(v, "homeserver.yaml") && !strings.HasSuffix(v, ":ro") {
			t.Errorf("homeserver.yaml lleva los secretos: debe montarse de solo lectura (%q)", v)
		}
	}
}

// Toda orden de Compose del Makefile sobre la suite local lleva $(MATRIX_F): sin
// él, `make dev-down` o `dev-clean` dejarían Synapse huérfano y su volumen vivo.
func TestElMakefileIncluyeMatrixEnCadaOrdenDeCompose(t *testing.T) {
	f, err := os.Open(filepath.Join(raiz, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	n, vistas := 0, 0
	for sc.Scan() {
		n++
		linea := sc.Text()
		if !strings.HasPrefix(linea, "\t") || !strings.Contains(linea, "$(COMPOSE) -f $(COMPOSE_DIR)/docker-compose.yml") {
			continue
		}
		vistas++
		if !strings.Contains(linea, "$(MATRIX_F)") {
			t.Errorf("Makefile:%d no incluye $(MATRIX_F): %s", n, strings.TrimSpace(linea))
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if vistas == 0 {
		t.Fatal("no se encontró ninguna orden de Compose en el Makefile")
	}
}

func leerProductos(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(raiz, "apps", "nucleo", "storage", "products.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(b))
}

func yamlEscalar(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "true"
		}
		return "false"
	case string:
		return x
	}
	return ""
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
