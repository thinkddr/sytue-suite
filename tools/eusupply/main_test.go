// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestMatchProviderExactAndWildcard(t *testing.T) {
	providers := []provider{{Pattern: "api.example.eu"}, {Pattern: "*.sytue.eu"}}
	for _, host := range []string{"api.example.eu", "git.sytue.eu"} {
		if _, ok := matchProvider(host, providers); !ok {
			t.Fatalf("%s no hizo match", host)
		}
	}
	if _, ok := matchProvider("sytue.eu", providers); ok {
		t.Fatal("el wildcard no debe incluir el apex")
	}
}

func TestElCatalogoGifNoOcultaSuProveedor(t *testing.T) {
	if !relevant("services/rooms/internal/gifcatalog/catalog.go") {
		t.Fatal("el proveedor que Rooms consulta en caliente queda fuera del inventario")
	}
	if relevant("services/rooms/internal/gifcatalog/catalog_test.go") {
		t.Fatal("las URL de los dobles herméticos no son proveedores")
	}
}

func TestImageAndComponentOriginAreIndependent(t *testing.T) {
	if got := componentHost("ghcr.io/element-hq/synapse:v1.163.0"); got != "element.io" {
		t.Fatalf("Matrix image must retain its upstream origin: %q", got)
	}
	if got := imageHost("minio/minio:latest"); got != "docker.io" {
		t.Fatalf("registry = %q", got)
	}
	if got := componentHost("minio/minio:latest"); got != "min.io" {
		t.Fatalf("component = %q", got)
	}
	if got := imageHost("rg.fr-par.scw.cloud/sytue/postgres:18"); got != "rg.fr-par.scw.cloud" {
		t.Fatalf("registry privado = %q", got)
	}
	if got := componentHost("rg.fr-par.scw.cloud/sytue/postgres:18"); got != "postgresql.org" {
		t.Fatalf("un mirror no debe ocultar el origen: %q", got)
	}
}

// Lo que se prueba aquí es que el gate NO exija inventariar cosas que no son
// proveedores. Cada falso positivo empuja a inventarse una ficha —una empresa,
// una sede, una licencia— para algo que nadie descarga, y un inventario con
// entradas ficticias ya no responde la única pregunta que importa.
func TestNoSeInventariaLoQueNoEsProveedor(t *testing.T) {
	casos := []struct{ nombre, ruta, texto string }{
		{
			// TLD reservado por la IANA (RFC 2606): no lo puede registrar nadie.
			"host inalcanzable a proposito en un test",
			"deploy/production/tests/x.sh",
			"SCW_API=http://scaleway.invalid sh \"$GUION\"",
		},
		{
			// Identificador de formato, no una descarga.
			"DTD de un plist de Apple",
			"deploy/x.sh",
			`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			unique := map[string]observation{}
			scanFile(c.ruta, c.texto, unique)
			for _, obs := range unique {
				t.Fatalf("no debía observarse nada y salió %q", obs.Host)
			}
		})
	}

	// La contraprueba: un host de verdad en el mismo tipo de fichero SÍ se
	// observa. Sin esto, un recorte demasiado goloso dejaría el gate ciego y
	// el test seguiría en verde.
	unique := map[string]observation{}
	scanFile("deploy/x.sh", "curl https://registry.npmjs.org/nanoid", unique)
	if _, ok := unique["registry.npmjs.org\tdeploy/x.sh"]; !ok {
		t.Fatalf("un host real debe seguir observándose, salió %v", unique)
	}
}

func TestValidateEvidence(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	valid := provider{Status: temporary, Reviewed: "2026-08-02", ExceptionUntil: "2026-10-01"}
	if err := validateEvidence(valid, now); err != nil {
		t.Fatalf("evidencia válida: %v", err)
	}

	expired := valid
	expired.ExceptionUntil = "2026-08-01"
	if err := validateEvidence(expired, now); err == nil || !strings.Contains(err.Error(), "caducada") {
		t.Fatalf("se esperaba caducidad, got %v", err)
	}

	stale := provider{Status: approved, Reviewed: "2025-01-01"}
	if err := validateEvidence(stale, now); err == nil || !strings.Contains(err.Error(), "evidencia caducada") {
		t.Fatalf("se esperaba evidencia caducada, got %v", err)
	}
}

// TestRevisaElDirectorioDeWorkflowsReal comprueba que el gate mira los
// workflows que de verdad se ejecutan.
//
// Es la comprobación más barata contra el fallo más caro de este programa: si
// el prefijo de relevant() apunta a un directorio que ya no existe, el gate no
// falla —sale verde—, simplemente deja de revisar los workflows. Estando ahí
// `.github/workflows/` mientras corrían los otros, pasaron un `curl` sin tope y
// una descarga fuera del lockfile.
//
// Por eso no se compara contra una constante sino contra el disco: se listan
// los ficheros de workflow reales del repositorio y se exige que relevant()
// diga que sí a todos.
func TestRevisaElDirectorioDeWorkflowsReal(t *testing.T) {
	// SOBRE LOS FICHEROS VERSIONADOS, no sobre lo que haya en disco.
	//
	// `relevant()` se aplica en producción a la salida de `git ls-files`
	// (ver scanTrackedFiles), así que probarlo contra un recorrido del
	// filesystem comprueba algo que no ocurre. Y falla por ruido: un worktree
	// bajo `.claude/worktrees/agent-*/` trae su propia copia de
	// `.github/workflows/`, cuyas rutas no empiezan por ese prefijo y hacen
	// caer el test sin que nada del repositorio esté mal. También lo harían un
	// `vendor/`, una copia de seguridad o un clon anidado.
	//
	// Añadir `.claude` a las exclusiones habría tapado este caso y dejado el
	// siguiente. Preguntarle a git es lo que de verdad iguala test y uso.
	salida, err := exec.Command("git", "-C", "../..", "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var encontrados int
	for _, crudo := range bytes.Split(salida, []byte{0}) {
		ruta := string(crudo)
		if ruta == "" || !strings.Contains(ruta, "workflows") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(ruta))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		encontrados++
		if !relevant(ruta) {
			t.Errorf("relevant(%q) = false: el gate NO revisa este workflow", ruta)
		}
	}
	if encontrados == 0 {
		t.Fatal("no se encontró ningún workflow: el test no está comprobando nada")
	}
}

func TestWorkflowsAnidadosMantienenElPerimetro(t *testing.T) {
	t.Parallel()
	casos := []struct {
		ruta string
		want bool
	}{
		{".github/workflows/ci.yml", true},
		{".forgejo/workflows/ci.yaml", true},
		{"tools/astro-fyne/.github/workflows/ci.yml", true},
		{"tools/exportable/nativo/.forgejo/workflows/ci.yaml", true},
		{"apps/fyne/xcode/ci_scripts/ci_post_clone.sh", true},
		{"apps/fyne/xcode/generar.py", false},
		{"tools/astro-fyne/.github/workflows-extra/ci.yml", false},
		{"tools/astro-fyne/not.github/workflows/ci.yml", false},
		{"tools/astro-fyne/workflows/ejemplo.yml", false},
		{"tools/astro-fyne/.github/workflows/README.md", false},
		{"node_modules/dependencia/.github/workflows/ci.yml", false},
		{"tools/node_modules/dependencia/.forgejo/workflows/ci.yml", false},
		{"deploy/helm/.vendor/chart/crds/.github/workflows/ci.yml", false},
	}
	for _, caso := range casos {
		if got := relevant(caso.ruta); got != caso.want {
			t.Errorf("relevant(%q) = %t, want %t", caso.ruta, got, caso.want)
		}
	}
}

func TestAccionesDeWorkflowConGuionSeInventarian(t *testing.T) {
	t.Parallel()
	const ruta = "tools/astro-fyne/.github/workflows/ci.yml"
	casos := []struct {
		texto string
		want  bool
	}{
		{"uses: actions/checkout@sha", true},
		{"  - uses: actions/checkout@sha", true},
		{"    uses: oven-sh/setup-bun@sha # versión fijada", true},
		{"  - uses: './.github/actions/toolchain'", false},
		{"  # - uses: actions/checkout@sha", false},
		{"  - reuses: actions/checkout@sha", false},
		{"  -uses: actions/checkout@sha", false},
		{"  run: echo uses: actions/checkout@sha", false},
	}
	for _, caso := range casos {
		unique := map[string]observation{}
		scanFile(ruta, caso.texto, unique)
		_, got := unique["github.com\t"+ruta]
		if got != caso.want {
			t.Errorf("proveedor de %q observado = %t, want %t", caso.texto, got, caso.want)
		}
	}
}

// El inventario REAL no puede tener como `blocked` lo que no es un canal.
//
// Es la comprobación que faltaba el 2026-08-11, cuando el registro declaraba 49
// dependencias bloqueadas y 34 de ellas no eran dependencias de nada: grupos de
// API de Kubernetes (`monitoring.coreos.com` es un campo `apiVersion`), prefijos
// de etiqueta (`app.kubernetes.io/name`) y espacios de nombres de módulos Go, que
// sirve `proxy.golang.org` y no ellos. Había 232 líneas de baseline existiendo
// solo para silenciarlos, y un registro de cumplimiento que cuenta mal no protege:
// entierra los quince canales que sí importan bajo el ruido.
func TestElInventarioNoBloqueaIdentificadores(t *testing.T) {
	ps, err := readProviders("../../compliance/supply-chain/providers.tsv")
	if err != nil {
		t.Fatalf("inventario: %v", err)
	}
	// Muestra de lo que NO puede volver a aparecer como dependencia bloqueada.
	noSonCanales := []string{"monitoring.coreos.com", "*.kubernetes.io", "go.uber.org", "gopkg.in"}
	for _, host := range noSonCanales {
		p, ok := matchProvider(strings.TrimPrefix(host, "*."), ps)
		if !ok {
			t.Errorf("%s salió del inventario: sigue haciendo falta su ficha", host)
			continue
		}
		if p.Status == blocked {
			t.Errorf("%s está como blocked, y no es un canal del que se descargue nada", host)
		}
	}

	// LA CONTRAPRUEBA, que es la mitad que importa: los canales de verdad siguen
	// bloqueados. Sin esto, marcarlo todo como identificador dejaría el gate
	// mudo y este test en verde.
	for _, host := range []string{"registry.npmjs.org", "proxy.golang.org", "sum.golang.org", "go.dev"} {
		p, ok := matchProvider(host, ps)
		if !ok {
			t.Errorf("%s no está inventariado", host)
			continue
		}
		if p.Status != blocked {
			t.Errorf("%s pasó a %q: es un canal de descarga y debe seguir vigilado", host, p.Status)
		}
	}
}

// Los registros de imagen son `pinned-artifact`, y eso EXIGE que la mitigación
// exista: sin digest, la etiqueta sería una excusa.
//
// Por eso el test no se conforma con leer el estado del inventario: comprueba
// que los `FROM` de los Dockerfile de servicio siguen llevando `@sha256:`. El día
// que alguien escriba `FROM golang:1.27` a secas, esa imagen deja de estar
// fijada y la clasificación deja de ser cierta —da igual lo que diga el TSV—.
func TestLosRegistrosFijadosLoEstanDeVerdad(t *testing.T) {
	ps, err := readProviders("../../compliance/supply-chain/providers.tsv")
	if err != nil {
		t.Fatalf("inventario: %v", err)
	}
	for _, host := range []string{"docker.io", "gcr.io", "ghcr.io", "quay.io"} {
		p, ok := matchProvider(host, ps)
		if !ok || p.Status != pinnedArtifact {
			t.Errorf("%s debería estar como pinned-artifact, está como %q", host, p.Status)
		}
	}

	rutas, err := filepath.Glob("../../services/*/Dockerfile*")
	if err != nil || len(rutas) == 0 {
		t.Fatalf("no se encontraron Dockerfile de servicio: %v", err)
	}
	base := regexp.MustCompile(`(?im)^FROM(?:\s+--platform=\S+)?\s+(\S+)`)
	sinDigest := 0
	for _, ruta := range rutas {
		b, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatalf("%s: %v", ruta, err)
		}
		for _, m := range base.FindAllStringSubmatch(string(b), -1) {
			img := m[1]
			if img == "scratch" || strings.HasPrefix(img, "$") {
				continue
			}
			if !strings.Contains(img, "@sha256:") {
				t.Errorf("%s: %q sin digest fijado, así que pinned-artifact deja de ser cierto", ruta, img)
				sinDigest++
			}
		}
	}
	if sinDigest == 0 && len(rutas) < 5 {
		t.Errorf("solo se revisaron %d Dockerfile: el glob no está mirando donde debe", len(rutas))
	}
}

// Lo que NO es una máquina no entra en el inventario.
//
// El regex de host/ruta casa también con un `owner/repo` escrito a pelo —el
// valor por defecto de SYTUE_GIT_REPO es `thinkddr/sytue-suite`— y pedía
// inventariar «thinkddr» como proveedor. Se distingue por el TLD, y la
// trampa está en preguntarlo bien: `githubusercontent.com` figura en la lista de
// sufijos públicos como sufijo PRIVADO, así que preguntar por el host completo
// descartaría `raw.githubusercontent.com`, que es una dependencia de verdad y
// está en la baseline.
func TestSoloSeInventarianNombresDeMaquina(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"git.gay", "raw.githubusercontent.com", "api.cloudflare.com", "helm.sh", "json-schema.org"} {
		if !esNombreDeMaquina(host) {
			t.Errorf("%s es un host real y se ha descartado", host)
		}
	}
	for _, noEsHost := range []string{"thinkddr", "sytue-one", "algo.invalido"} {
		if esNombreDeMaquina(noEsHost) {
			t.Errorf("%s no es un host y se ha colado", noEsHost)
		}
	}
}

// Lo vendorizado y lo instalado no se escanea, pero SOLO en la parte que es
// documentación de terceros: los valores y el Chart de un chart vendorizado
// siguen mirándose, que es donde están las imágenes que de verdad se despliegan.
func TestNoSeEscaneaLoAjeno(t *testing.T) {
	t.Parallel()
	fuera := []string{
		"test/e2e/node_modules/typescript/package.json",
		"node_modules/undici-types/package.json",
		"tools/astro-fyne/node_modules/example/.github/workflows/ci.yml",
		"deploy/helm/.vendor/traefik-41.1.0/crds/hub.traefik.io_apiversions.yaml",
	}
	for _, path := range fuera {
		if relevant(path) {
			t.Errorf("%s no debería escanearse", path)
		}
	}
	dentro := []string{
		"deploy/helm/.vendor/traefik-41.1.0/values.yaml",
		"deploy/helm/.vendor/traefik-41.1.0/Chart.yaml",
		"deploy/helm/bin/deploy.sh",
		"tools/astro-fyne/.github/workflows/ci.yml",
		"tools/compiler/.forgejo/workflows/ci.yaml",
	}
	for _, path := range dentro {
		if !relevant(path) {
			t.Errorf("%s SÍ debería escanearse", path)
		}
	}
}

func TestDropboxExternoNoOcultaSuProveedor(t *testing.T) {
	for _, path := range []string{
		"platform/pkg/objectstore/externo_dropbox.go",
		"services/integrations/internal/http/handlers_internal_rooms.go",
		"services/integrations/internal/http/handlers_whatsapp_signup.go",
		"services/integrations/internal/http/handlers_whatsapp_consent.go",
		"services/integrations/db/seeds/templates/whatsapp-business.json",
		"services/integrations/internal/http/consent/whatsapp.html",
		"services/integrations/db/seeds/templates/dropbox-storage.json",
		"services/integrations/db/seeds/templates/slack-rooms.json",
		"services/integrations/db/seeds/templates/microsoft-teams-rooms.json",
		"services/integrations/db/seeds/templates/google-chat.json",
		"services/integrations/db/seeds/templates/google-meet.json",
	} {
		if !relevant(path) {
			t.Fatalf("el proveedor elegido para almacenamiento debe estar inventariado: %s", path)
		}
	}
	if relevant("platform/pkg/objectstore/externo_dropbox_test.go") {
		t.Fatal("las respuestas herméticas del test no son proveedores")
	}
}

func TestExternalConversationProviderSourcesCannotEscapeInventory(t *testing.T) {
	for _, path := range []string{"services/integrations/internal/http/external_chat_grant.go", "services/integrations/internal/http/external_chat_slack.go", "services/integrations/db/seeds/templates/slack-chat.json", "services/integrations/db/seeds/templates/microsoft-teams-chat.json", "services/integrations/db/seeds/templates/google-chat-conversations.json"} {
		if !relevant(path) {
			t.Errorf("provider omitted: %s", path)
		}
	}
	if relevant("services/integrations/internal/http/external_chat_test.go") {
		t.Fatal("test-only hosts included as runtime providers")
	}
}

func TestAppleBundlePathIsNotAnExternalHost(t *testing.T) {
	observed := map[string]observation{}
	scanFile("apps/fyne/scripts/empaquetar-test.sh", `Rooms.app/Contents/MacOS/Rooms`, observed)
	if len(observed) != 0 {
		t.Fatalf("bundle mistaken for network: %#v", observed)
	}
	scanFile("fixture.sh", `https://rooms.app/Contents/file`, observed)
	if len(observed) != 1 {
		t.Fatal("actual HTTPS host must still be inventoried")
	}
}

func TestMailRuntimeProvidersAreInventoried(t *testing.T) {
	for _, path := range []string{"services/integrations/internal/linkedservices/client.go", "services/integrations/internal/linkedservices/dav.go"} {
		if !runtimeProviderSource(path) {
			t.Fatalf("linked provider not scanned: %s", path)
		}
	}
	if runtimeProviderSource("services/integrations/internal/linkedservices/client_test.go") {
		t.Fatal("synthetic provider fixtures are not runtime hosts")
	}
	for _, path := range []string{"services/integrations/internal/mailbox/cloud.go", "services/integrations/internal/http/handlers_external_mail.go", "services/integrations/db/seeds/templates/gmail-mail.json", "services/integrations/db/seeds/templates/microsoft-mail.json", "services/integrations/db/seeds/templates/yahoo-mail.json"} {
		if !runtimeProviderSource(path) {
			t.Fatalf("mail provider not scanned: %s", path)
		}
	}
}

func TestAppleBundlePathsAndExpectedMockRequestsAreNotProviders(t *testing.T) {
	path := "apps/fyne/scripts/empaquetar-test.sh"
	unique := map[string]observation{}
	scanFile(path, `cmp "$ios_fixture/Sytue Rooms.app/main" "$ios_fixture/main.antes"
 assert 'Payload/Sytue Rooms.app/embedded.mobileprovision' in nombres
 [ "$(cat "$falso/peticiones")" = 'GET https://androidpublisher.googleapis.com/androidpublisher/v3/applications/eu.sytue.mail/generatedApks/3013004' ] || exit 1`, unique)
	if len(unique) != 0 {
		t.Fatalf("local paths and expected mock data became providers: %v", unique)
	}
	// A real URL in the same test script must remain visible, including the same
	// .app/main or .app/embedded.mobileprovision suffix. The URL matcher cannot be suppressed with the path matcher.
	scanFile(path, `curl https://rooms.app/embedded.mobileprovision
 curl https://androidpublisher.googleapis.com/androidpublisher/v3/applications`, unique)
	for _, host := range []string{"rooms.app", "androidpublisher.googleapis.com"} {
		if _, ok := unique[host+"\t"+path]; !ok {
			t.Errorf("real request was hidden: %s", host)
		}
	}
	// The exact comparison in a production script is not a fixture exemption.
	production := "apps/fyne/scripts/play-publicar.sh"
	scanFile(production, `[ "$(cat "$falso/peticiones")" = 'GET https://androidpublisher.googleapis.com/api' ]`, unique)
	if _, ok := unique["androidpublisher.googleapis.com\t"+production]; !ok {
		t.Fatal("production endpoint was hidden")
	}
}
