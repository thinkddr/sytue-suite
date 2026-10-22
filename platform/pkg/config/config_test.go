// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLoaderGetDefault(t *testing.T) {
	t.Setenv("SYTUE_ID_LOG_LEVEL", "debug")
	l := NewLoader("SYTUE_ID")
	if got := l.GetDefault("LOG_LEVEL", "info"); got != "debug" {
		t.Fatalf("GetDefault = %q, want debug", got)
	}
	if got := l.GetDefault("MISSING", "fallback"); got != "fallback" {
		t.Fatalf("GetDefault missing = %q, want fallback", got)
	}
}

func TestLoaderRequire(t *testing.T) {
	l := NewLoader("SVC")
	_ = l.Require("DATABASE_URL")
	if l.Err() == nil {
		t.Fatal("esperaba error por variable obligatoria ausente")
	}
}

func TestLoaderTypes(t *testing.T) {
	t.Setenv("X_N", "42")
	t.Setenv("X_B", "true")
	t.Setenv("X_D", "5s")
	t.Setenv("X_LIST", "a, b ,c")
	l := NewLoader("X")
	if n := l.GetIntDefault("N", 0); n != 42 {
		t.Fatalf("int = %d", n)
	}
	if !l.GetBoolDefault("B", false) {
		t.Fatal("bool != true")
	}
	if d := l.GetDurationDefault("D", time.Second); d != 5*time.Second {
		t.Fatalf("dur = %v", d)
	}
	if list := l.GetCSV("LIST"); len(list) != 3 || list[1] != "b" {
		t.Fatalf("csv = %v", list)
	}
	if l.Err() != nil {
		t.Fatalf("err inesperado: %v", l.Err())
	}
}

func TestLoadBaseInvalidEnv(t *testing.T) {
	t.Setenv("APP_ENV", "qa")
	if _, err := LoadBase("APP", "svc"); err == nil {
		t.Fatal("esperaba error por ENV inválido")
	}
}

func TestLoadBaseDefaults(t *testing.T) {
	b, err := LoadBase("EMPTYPFX", "sytue-id")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if b.Env != EnvDev || b.LogFormat != "json" || b.ServiceName != "sytue-id" {
		t.Fatalf("defaults inesperados: %+v", b)
	}
	if b.Env.IsProd() {
		t.Fatal("dev no debe ser prod")
	}
}

// La versión que se enseña en la consola de plataforma sale de aquí. Las tres
// procedencias importan y tienen un orden: lo que configure el despliegue manda
// sobre lo horneado en la imagen, y "dev" solo aparece cuando no hay ninguna de
// las dos — que es como se distingue una imagen de release de una hecha a mano.
func TestVersionDeLaImagen(t *testing.T) {
	// Sin nada: "dev". Si esto devolviera cadena vacía, la pantalla enseñaría un
	// hueco que parece un fallo de carga.
	if got := BuildVersion(); got != "dev" {
		t.Fatalf("sin entorno = %q, quiero dev", got)
	}

	// Horneada en la imagen (SYTUE_BUILD_VERSION), sin prefijo de servicio: es el
	// mismo dato para los once binarios de una release.
	t.Setenv("SYTUE_BUILD_VERSION", "v1.2.3")
	if got := BuildVersion(); got != "v1.2.3" {
		t.Fatalf("horneada = %q, quiero v1.2.3", got)
	}
	base, err := LoadBase("SYTUE_ID", "sytue-id")
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if base.Version != "v1.2.3" {
		t.Errorf("Version = %q: el servicio no ve la versión de su propia imagen", base.Version)
	}

	// Y el despliegue puede sobreescribirla por servicio.
	t.Setenv("SYTUE_ID_VERSION", "v9.9.9")
	base, err = LoadBase("SYTUE_ID", "sytue-id")
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if base.Version != "v9.9.9" {
		t.Errorf("Version = %q: la configuración explícita ya no manda sobre lo horneado", base.Version)
	}
}

// El Origin de un navegador que abre Rooms desde `corporate.sytue.eu/rooms` es
// el host de la ORGANIZACIÓN, no el del producto; sin el comodín, el handshake
// del WebSocket daba 403 en producción y Rooms se quedaba sin tiempo real. Se
// comprueba con la misma función de patrones que usa nhooyr/websocket
// (filepath.Match), no con un `strings.Contains` que aprobaría un comodín mal
// escrito.

func TestFirstPartyAudiences_llevaLasAppsDeLaSuite(t *testing.T) {
	got := FirstPartyAudiences()
	// El client_id de cada app con login. Si se añade una app y su audiencia no
	// entra aquí, TODOS los servicios le contestan 401 y el fallo se ve como
	// pantallas vacías, no como un problema de audiencias.
	for _, aud := range []string{
		"sytue-one", "sytue-mail", "sytue-drive", "sytue-docs", "sytue-calendar",
		"sytue-contacts", "sytue-notes", "sytue-syphotos", "sytue-rooms",
		"sytue-chat", "sytue-meet", "sytue-cloud", "sytue-console",
		"sytue-platform", "sytue-hub", "sytue-desktop", "sytue-mobile", "sytue-account",
		"sytue-cast", "sytue-pages", "sytue-bases",
		"sytue-docus", "sytue-calcus", "sytue-diapos", "sytue-certipdf",
	} {
		if !slices.Contains(strings.Split(got, ","), aud) {
			t.Errorf("falta la audiencia %q en %q", aud, got)
		}
	}
}

// TestFirstPartyAudiences_cubreLosClientesDelChart lee los `clientId:` del chart de la
// web, que es de donde sale el `aud` de cada app: una lista escrita a mano se quedó
// sin `sytue-syphotos` y luego sin `sytue-account`, y las dos veces el Hub contestó
// 401 a esa app sin que nada se pusiera en rojo.
func TestFirstPartyAudiences_cubreLosClientesDelChart(t *testing.T) {
	yaml, err := os.ReadFile("../../../deploy/helm/charts/sytue-frontend/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(FirstPartyAudiences(), ",")
	ids := regexp.MustCompile(`(?m)^\s*clientId:\s*(sytue-[a-z0-9-]+)`).FindAllStringSubmatch(string(yaml), -1)
	if len(ids) == 0 {
		t.Fatal("no hay ningún clientId en el chart: la expresión ya no lo lee")
	}
	for _, m := range ids {
		if !slices.Contains(got, m[1]) {
			t.Errorf("el chart da el cliente %q y los servicios no aceptan su audiencia", m[1])
		}
	}
}

func TestFirstPartyAudiences_anadeLasPropiasAlFinal(t *testing.T) {
	got := strings.Split(FirstPartyAudiences("sytue-ai"), ",")
	if got[len(got)-1] != "sytue-ai" {
		t.Fatalf("la audiencia propia debía ir al final: %v", got)
	}
	if len(got) != len(firstPartyAudiences)+1 {
		t.Fatalf("FirstPartyAudiences duplicó o perdió entradas: %v", got)
	}
}

// TestNormalizeS3Endpoint fija la regla que evitó el incidente del piloto: el
// esquema del endpoint manda sobre el useSSL heredado del chart. Reúne los casos
// que antes vivían sueltos en mail, cloud, drive y syphotos.
func TestNormalizeS3Endpoint(t *testing.T) {
	casos := []struct {
		in       string
		inSSL    bool
		wantHost string
		wantSSL  bool
	}{
		// El esquema explícito gana, contradiga o no al useSSL recibido.
		{"https://h:443", false, "h:443", true},
		{"http://h:80", true, "h:80", false},
		{"http://seaweedfs:8333", true, "seaweedfs:8333", false},
		{"https://s3.fr-par.scw.cloud", false, "s3.fr-par.scw.cloud", true},
		// Sin esquema, el endpoint pasa intacto y decide useSSL.
		{"minio:9000", true, "minio:9000", true},
		{"minio:9000", false, "minio:9000", false},
		{"seaweedfs:8333", false, "seaweedfs:8333", false},
		{"", true, "", true},
	}
	for _, c := range casos {
		host, ssl := NormalizeS3Endpoint(c.in, c.inSSL)
		if host != c.wantHost || ssl != c.wantSSL {
			t.Errorf("NormalizeS3Endpoint(%q,%v) = (%q,%v); se esperaba (%q,%v)",
				c.in, c.inSSL, host, ssl, c.wantHost, c.wantSSL)
		}
	}
}
