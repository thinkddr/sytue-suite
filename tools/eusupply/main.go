// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command eusupply applies Sytue's strict EU-origin supply-chain policy.
// It inventories distribution hosts from tracked build/deploy sources and
// rejects unknown providers, expired exceptions and new non-EU references.
package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

const (
	approved  = "eu-approved"
	temporary = "temporary-exception"
	blocked   = "blocked"
	// identifier: la cadena PARECE un host pero no lo es. Nadie la resuelve, no
	// se descarga nada de ella y no hay entidad que trate ningún dato nuestro.
	//
	// Es el mismo criterio que ya se aplica al identificador de sistema de un
	// <!DOCTYPE> (ver más abajo), extendido a las otras dos familias que lo
	// cumplen:
	//
	//   · Grupos de API de Kubernetes. `monitoring.coreos.com/v1` es un campo
	//     `apiVersion`, no una URL: jamás sale una petición hacia ahí. Igual
	//     `cert-manager.io` en una anotación.
	//   · Espacios de nombres de módulos Go. `go.uber.org/zap` identifica al
	//     módulo; quien lo SIRVE es `proxy.golang.org`, que sí está inventariado
	//     y es donde la política tiene efecto. Bloquear el espacio de nombres no
	//     quita ni añade una sola descarga.
	//
	// NO es una vía para relajar el gate: un identificador sigue exigiendo su
	// ficha con evidencia, dueño y fecha de revisión, y sigue caducando a los
	// 183 días. Lo único que cambia es que no se cuenta como dependencia, porque
	// no lo es, y un registro de cumplimiento solo vale si dice la verdad.
	identifier = "identifier"
	// pinnedArtifact: ENTRADA de un artefacto público cuya integridad ya está
	// garantizada. No sale nada nuestro y no puede llegar algo distinto de lo que
	// se pidió, que es el único riesgo real de esta dirección.
	//
	// No es una etiqueta de conveniencia: exige que la mitigación EXISTA, y en
	// esta suite existe por dos vías comprobables.
	//
	//   · Runtime: los charts apuntan a `rg.fr-par.scw.cloud`, el registro
	//     privado. `deploy/production/upstream-images.txt` cataloga las veinte
	//     imágenes de terceros con su digest inmutable y las firma, y
	//     `upstream-vigencia-test.sh` rechaza cualquiera que no esté catalogada.
	//     Lo que aparece en los charts es la PROCEDENCIA de lo espejado.
	//   · Build: los treinta `FROM` de los Dockerfile llevan `@sha256:`. Un
	//     digest no se puede suplantar; como mucho, desaparecer.
	//
	// El riesgo que queda es de DISPONIBILIDAD —que retiren el artefacto y no se
	// pueda reconstruir—, no de soberanía. Es la política que ya escribió el
	// README de compliance: «se mitiga con digest fijado, sha256 comprobado y
	// espejo propio, no cortando la conexión».
	//
	// Si algún día una referencia de estas pierde el digest, quien lo detecta es
	// el gate de vigencia, no este: aquí no se comprueba la forma de la etiqueta.
	pinnedArtifact = "pinned-artifact"
)

type provider struct {
	Pattern, Status, Reviewed, ExceptionUntil string
}

type observation struct{ Host, Path string }

var (
	// An expected request read from a local mock log is data, not an egress.
	shellExpectedRequest = regexp.MustCompile(`(?m)^[ \t]*\[[ \t]*"\$\(cat "[^"\n]*/peticiones"\)"[ \t]*=[ \t]*'(?:GET|POST|PUT|PATCH|DELETE) https?://[^'\n]+'[ \t]*\]`)
	urlHost              = regexp.MustCompile(`(?i)https?://([a-z0-9][a-z0-9.-]*\.[a-z]{2,})`)
	// El prefijo NO capturante evita el falso positivo de una ruta con punto:
	// en `github.com/thinkddr/sytue-one`, `\b` dejaba casar otra vez a partir
	// de la barra y «daniel.delgado» entraba como si fuera un host más, pidiendo
	// que se inventariara el nombre del propietario del repositorio.
	hostPath       = regexp.MustCompile(`(?i)(?:^|[^a-z0-9./@-])([a-z0-9][a-z0-9.-]*\.[a-z]{2,})/[a-z0-9_./@+%-]+`)
	fromImage      = regexp.MustCompile(`(?im)^\s*FROM(?:\s+--platform=\S+)?\s+(\S+)`)
	yamlImage      = regexp.MustCompile(`(?im)^\s*(?:image|source):\s*["']?([^\s"'#]+)`)
	workflowAction = regexp.MustCompile(`(?im)^\s*(?:-\s+)?uses:\s*([^\s#]+)`)
	// doctype: el identificador de sistema de un <!DOCTYPE> nombra un FORMATO,
	// no un proveedor. La URL de la DTD de un plist de Apple no se descarga
	// nunca —ni el empaquetador ni ningún analizador de XML del build la
	// resuelven—, así que inventariar «Apple Inc.» por ella metería una
	// dependencia inexistente en un registro de cumplimiento cuyo único valor
	// es decir la verdad sobre quién trata qué. Se recorta antes de buscar.
	doctype = regexp.MustCompile(`(?is)<!DOCTYPE[^>]*>`)
	// xmlns: MISMO caso que el doctype, y por el mismo motivo. Un espacio de
	// nombres XML identifica un FORMATO —`http://s3.amazonaws.com/doc/2006-03-01/`
	// es como se llama el esquema de la API S3, y lo escribe cualquier cliente que
	// hable ese protocolo, incluido contra MinIO o Scaleway—. Nadie lo resuelve ni
	// se conecta ahí. Inventariar «Amazon» por él metería en el registro de
	// cumplimiento un proveedor que no trata ni un byte nuestro, que es
	// exactamente lo que ese registro no puede permitirse.
	xmlns = regexp.MustCompile(`(?i)xmlns(?::[a-z0-9_-]+)?="[^"]*"`)
)

func main() {
	inventoryPath := flag.String("inventory", "compliance/supply-chain/providers.tsv", "provider inventory")
	baselinePath := flag.String("baseline", "compliance/supply-chain/blocked-baseline.tsv", "known blocked observations")
	printObserved := flag.Bool("print", false, "print all observations as TSV")
	flag.Parse()

	providers, err := readProviders(*inventoryPath)
	if err != nil {
		fatal(err)
	}
	baseline, err := readBaseline(*baselinePath)
	if err != nil {
		fatal(err)
	}
	observed, err := scanTrackedFiles()
	if err != nil {
		fatal(err)
	}

	now := time.Now().UTC()
	var failures []string
	seenBaseline := make(map[string]bool)
	for _, obs := range observed {
		if *printObserved {
			fmt.Printf("%s\t%s\n", obs.Host, obs.Path)
		}
		p, ok := matchProvider(obs.Host, providers)
		if !ok {
			failures = append(failures, fmt.Sprintf("host sin inventariar: %s (%s)", obs.Host, obs.Path))
			continue
		}
		if err := validateEvidence(p, now); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", p.Pattern, err))
		}
		key := obs.Host + "\t" + obs.Path
		switch p.Status {
		case approved, temporary, identifier, pinnedArtifact:
		case blocked:
			if !baseline[key] {
				failures = append(failures, "nueva dependencia bloqueada: "+key)
			} else {
				seenBaseline[key] = true
			}
		default:
			failures = append(failures, fmt.Sprintf("estado inválido %q para %s", p.Status, p.Pattern))
		}
	}
	for key := range baseline {
		if !seenBaseline[key] {
			failures = append(failures, "baseline obsoleto (elimínalo al cerrar la dependencia): "+key)
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		for _, failure := range failures {
			fmt.Fprintln(os.Stderr, "ERROR:", failure)
		}
		fmt.Fprintf(os.Stderr, "\nGate UE: %d incumplimiento(s).\n", len(failures))
		os.Exit(1)
	}
	fmt.Printf("Gate UE: %d referencias verificadas; ninguna dependencia nueva fuera de la UE.\n", len(observed))
}

func readProviders(path string) ([]provider, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var result []provider
	s := bufio.NewScanner(f)
	for line := 1; s.Scan(); line++ {
		text := strings.TrimSpace(s.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Split(text, "\t")
		if len(fields) != 11 {
			return nil, fmt.Errorf("%s:%d: se esperaban 11 columnas TSV, hay %d", path, line, len(fields))
		}
		if fields[6] == "" || fields[7] == "" || fields[8] == "" || fields[10] == "" {
			return nil, fmt.Errorf("%s:%d: evidencia, owner, fecha y licencia son obligatorios", path, line)
		}
		result = append(result, provider{
			Pattern: fields[0], Status: fields[1], Reviewed: fields[8], ExceptionUntil: fields[9],
		})
	}
	return result, s.Err()
}

func readBaseline(path string) (map[string]bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool)
	for line, raw := range strings.Split(string(b), "\n") {
		text := strings.TrimSpace(raw)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if strings.Count(text, "\t") != 1 {
			return nil, fmt.Errorf("%s:%d: formato esperado host<TAB>ruta", path, line+1)
		}
		result[text] = true
	}
	return result, nil
}

func validateEvidence(p provider, now time.Time) error {
	reviewed, err := time.Parse(time.DateOnly, p.Reviewed)
	if err != nil {
		return fmt.Errorf("reviewed_on inválido: %w", err)
	}
	if now.Sub(reviewed) > 183*24*time.Hour {
		return fmt.Errorf("evidencia caducada: revisada %s (>183 días)", p.Reviewed)
	}
	if p.Status != temporary {
		return nil
	}
	until, err := time.Parse(time.DateOnly, p.ExceptionUntil)
	if err != nil {
		return fmt.Errorf("temporary-exception sin exception_until válido: %w", err)
	}
	if now.After(until.Add(24 * time.Hour)) {
		return fmt.Errorf("excepción caducada el %s", p.ExceptionUntil)
	}
	return nil
}

func matchProvider(host string, providers []provider) (provider, bool) {
	for _, p := range providers {
		if host == p.Pattern || (strings.HasPrefix(p.Pattern, "*.") && strings.HasSuffix(host, p.Pattern[1:])) {
			return p, true
		}
	}
	return provider{}, false
}

func scanTrackedFiles() ([]observation, error) {
	cmd := exec.CommandContext(context.Background(), "git", "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	unique := make(map[string]observation)
	for _, raw := range bytes.Split(out, []byte{0}) {
		path := string(raw)
		if path == "" || !relevant(path) {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		scanFile(path, string(body), unique)
	}
	result := make([]observation, 0, len(unique))
	for _, obs := range unique {
		result = append(result, obs)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Host == result[j].Host {
			return result[i].Path < result[j].Path
		}
		return result[i].Host < result[j].Host
	})
	return result, nil
}

func scanFile(path, text string, unique map[string]observation) {
	base := filepath.Base(path)
	if strings.HasSuffix(base, "-test.sh") {
		text = shellExpectedRequest.ReplaceAllString(text, "")
	}
	if base == "bun.lock" {
		addObservation(unique, "registry.npmjs.org", path)
		addObservation(unique, "bun.sh", path)
	}
	if base == "go.mod" || base == "go.sum" {
		addObservation(unique, "proxy.golang.org", path)
		addObservation(unique, "sum.golang.org", path)
	}
	for _, re := range []*regexp.Regexp{urlHost, hostPath} {
		limpio := xmlns.ReplaceAllString(doctype.ReplaceAllString(text, ""), "")
		for _, match := range re.FindAllStringSubmatch(limpio, -1) {
			if re == hostPath && (strings.Contains(strings.ToLower(match[0]), ".app/contents/") || strings.Contains(strings.ToLower(match[0]), ".app/info.plist") || strings.Contains(strings.ToLower(match[0]), ".app/embedded.mobileprovision") || strings.HasSuffix(strings.ToLower(match[0]), ".app/main")) {
				continue
			}
			addObservation(unique, strings.ToLower(match[1]), path)
		}
	}
	for _, re := range []*regexp.Regexp{fromImage, yamlImage} {
		for _, match := range re.FindAllStringSubmatch(text, -1) {
			addObservation(unique, imageHost(match[1]), path)
			addObservation(unique, componentHost(match[1]), path)
		}
	}
	for _, match := range workflowAction.FindAllStringSubmatch(text, -1) {
		value := strings.Trim(match[1], "\"'")
		if !strings.HasPrefix(value, "./") && !strings.HasPrefix(value, "http") {
			addObservation(unique, "github.com", path)
		}
	}
}

func gifCatalogSource(path, ext string) bool {
	return strings.HasPrefix(path, "services/rooms/internal/gifcatalog/") && ext == ".go" && !strings.HasSuffix(path, "_test.go")
}

func supplyManifest(base string) bool {
	switch base {
	case "go.mod", "go.sum", "bun.lock", "package.json", "Makefile":
		return true
	}
	return strings.HasSuffix(base, "Dockerfile") || strings.HasPrefix(base, "Dockerfile.") || strings.HasSuffix(base, "Containerfile")
}

// runtimeProviderSource enumera las integraciones que conectan con proveedores
// externos en caliente. Sus URL también pasan por el inventario de suministro.
func runtimeProviderSource(path string) bool {
	if strings.HasPrefix(path, "services/integrations/internal/linkedservices/") && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
		return true
	}
	if strings.HasPrefix(path, "services/integrations/internal/http/external_chat_") && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
		return true
	}
	switch path {
	case "services/integrations/internal/mailbox/cloud.go",
		"services/integrations/internal/http/handlers_external_mail.go",
		"services/integrations/db/seeds/templates/gmail-mail.json",
		"services/integrations/db/seeds/templates/microsoft-mail.json",
		"services/integrations/db/seeds/templates/yahoo-mail.json",
		"platform/pkg/objectstore/externo_dropbox.go",
		"services/integrations/internal/http/handlers_internal_rooms.go",
		"services/integrations/internal/http/handlers_whatsapp_signup.go",
		"services/integrations/internal/http/handlers_whatsapp_consent.go",
		"services/integrations/db/seeds/templates/whatsapp-business.json",
		"services/integrations/internal/http/consent/whatsapp.html",
		"services/integrations/db/seeds/templates/dropbox-storage.json",
		"services/integrations/db/seeds/templates/slack-rooms.json",
		"services/integrations/db/seeds/templates/slack-chat.json",
		"services/integrations/db/seeds/templates/microsoft-teams-chat.json",
		"services/integrations/db/seeds/templates/google-chat-conversations.json",
		"services/integrations/db/seeds/templates/microsoft-teams-rooms.json",
		"services/integrations/db/seeds/templates/google-chat.json",
		"services/integrations/db/seeds/templates/google-meet.json":
		return true
	default:
		return false
	}
}

func relevant(path string) bool {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	if gifCatalogSource(path, ext) || runtimeProviderSource(path) {
		return true
	}
	// Lo que NO es nuestro no se escanea, y esto no es laxitud: es la diferencia
	// entre un gate que se lee y uno que se ignora.
	//
	//   · node_modules —que no debería estar versionado, y estuvo: 468 ficheros
	//     entraron en el commit de migración— ya está cubierto por `bun.lock`,
	//     que este mismo escáner inventaría como registry.npmjs.org. Mirar
	//     dentro añade el `package.json` de cada dependencia y con él sus URL de
	//     documentación, que no son proveedores de nada.
	//   · Los CRD de un chart vendorizado son ESQUEMAS de terceros. Sus URL son
	//     citas —semver.org, tools.ietf.org, controller-gen.kubebuilder.io— y
	//     sumaban 66 de los 95 incumplimientos, tapando los que sí importan. El
	//     resto del chart (Chart.yaml, values.yaml) SÍ se sigue mirando: ahí es
	//     donde están las imágenes que de verdad se despliegan.
	if strings.HasPrefix(path, "node_modules/") || strings.Contains(path, "/node_modules/") {
		return false
	}
	if strings.Contains(path, "/.vendor/") && strings.Contains(path, "/crds/") {
		return false
	}
	if supplyManifest(base) {
		return true
	}
	// `codemagic.yaml` va por NOMBRE y no por prefijo de directorio: Codemagic lo
	// exige en la RAÍZ del repositorio, y la raíz no está en la lista de abajo.
	// Sin esta línea el gate salía en verde sobre un fichero de CI entero —con su
	// proveedor, sus descargas y sus credenciales— que no miraba nadie, que es
	// exactamente el punto ciego que este gate existe para no tener.
	if base == "codemagic.yaml" {
		return true
	}
	// Si esta lista se desalinea del directorio real de workflows, el gate deja
	// de mirarlos y sigue saliendo verde: no falla, simplemente no revisa nada.
	// Ya pasó estando aquí `.github/workflows/` mientras los que corrían eran
	// otros, y por ahí se colaron un `curl` sin tope y una descarga fuera del
	// lockfile. Lo fija TestRevisaElDirectorioDeWorkflowsReal.
	//
	// `.forgejo/workflows/` sigue en la lista mientras exista el puente de
	// secretos que corre en git.gay (`migrate-github-secrets.yml`): es un
	// fichero de CI con su proveedor y su credencial, y un directorio de
	// workflows que el gate no mira es el punto ciego del párrafo de arriba.
	// Los componentes exportables también llevan workflows bajo su propio
	// directorio (tools/astro-fyne). Se reconoce el segmento completo de CI,
	// sin ampliar el escaneo a todos los YAML de tools/ ni a rutas parecidas.
	// `apps/fyne/xcode/ci_scripts/` es lo que ejecuta Xcode Cloud, que también
	// clona el repositorio entero: por lo mismo que `codemagic.yaml`.
	for _, prefix := range []string{".github/workflows/", ".forgejo/workflows/", "deploy/", "scripts/", "apps/fyne/scripts/", "apps/fyne/xcode/ci_scripts/"} {
		workflow := prefix == ".github/workflows/" || prefix == ".forgejo/workflows/"
		if strings.HasPrefix(path, prefix) || (workflow && strings.Contains(path, "/"+prefix)) {
			switch ext {
			case ".sh", ".py", ".tf", ".yaml", ".yml", ".txt", ".json", ".toml":
				return true
			}
		}
	}
	return false
}

func imageHost(image string) string {
	image = strings.TrimSpace(strings.Trim(image, "\"'"))
	if image == "" || strings.Contains(image, "${") || strings.HasPrefix(image, "sha256:") {
		return ""
	}
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		return "docker.io"
	}
	first := parts[0]
	if !strings.Contains(first, ".") && !strings.Contains(first, ":") && first != "localhost" {
		return "docker.io"
	}
	if strings.Contains(first, ".") {
		return strings.ToLower(first)
	}
	return ""
}

// componentHost conserva la procedencia/gobernanza del software aunque la
// imagen se promueva a un registry europeo. Un mirror cambia la distribución,
// no la entidad matriz ni el origen del componente.
func componentHost(image string) string {
	image = strings.ToLower(strings.TrimSpace(strings.Trim(image, "\"'")))
	components := []struct{ needle, host string }{
		{"postgrest/", "postgrest.org"},
		{"pgbouncer", "pgbouncer.org"},
		{"paketobuildpacks/", "paketo.io"},
		{"dragonfly", "dragonflydb.io"},
		{"livekit", "livekit.io"},
		{"element-hq/synapse", "element.io"},
		{"minio/", "min.io"},
		{"oven/bun", "bun.sh"},
		{"cert-manager", "cert-manager.io"},
		{"traefik", "traefik.io"},
		{"prometheus", "prometheus.io"},
		{"postgres", "postgresql.org"},
		{"debian", "debian.org"},
		{"alpine", "alpinelinux.org"},
		{"golang", "go.dev"},
	}
	for _, component := range components {
		if strings.Contains(image, component.needle) {
			return component.host
		}
	}
	return ""
}

// reservados son sufijos que NUNCA pueden ser un proveedor real: la IANA los
// tiene apartados justo para que nadie los registre (RFC 2606 y RFC 6761). Un
// guion de pruebas que apunta a un host inalcanzable a propósito no tiene
// detrás una empresa con sede, licencia ni evidencia que inventariar, así que
// exigirle una ficha en `providers.tsv` obligaría a inventarse un proveedor
// falso — y un inventario de cumplimiento con entradas ficticias deja de servir
// para lo único que sirve, que es responder quién trata el dato.
var reservados = []string{".localhost", ".test", ".internal", ".invalid", ".example"}

// ¿Es «host» un nombre de máquina de verdad?
//
// El regex de host/ruta también casa con un `owner/repo` escrito a pelo —el
// valor por defecto de SYTUE_GIT_REPO es `daniel.delgado/sytue-one`—, y de ahí
// salía «daniel.delgado» pidiendo sitio en el inventario de proveedores. Se
// pregunta a la lista de sufijos públicos en vez de mantener una lista de TLD a
// mano: `gay` es un TLD real y `delgado` no, y eso no es algo que debamos
// decidir nosotros cada vez que alguien tenga un apellido con punto delante.
func esNombreDeMaquina(host string) bool {
	i := strings.LastIndex(host, ".")
	if i < 0 {
		return false
	}
	// Se pregunta por el TLD SOLO, no por el host entero, y esa diferencia es
	// justo lo que hay que cuidar: `githubusercontent.com` está en la lista como
	// sufijo PRIVADO (icann=false), así que preguntar por el host completo
	// descartaba `raw.githubusercontent.com` —una dependencia real de GitHub—
	// mientras seguía colando cualquier `apellido/repo`. Por el TLD, `com` y
	// `gay` son ICANN y `delgado` no.
	_, icann := publicsuffix.PublicSuffix("x." + host[i+1:])
	return icann
}

func addObservation(dst map[string]observation, host, path string) {
	if host == "" || host == "example.com" || host == "go.mod" || host == "metrics.cockpit" {
		return
	}
	if !esNombreDeMaquina(host) {
		return
	}
	for _, sufijo := range reservados {
		if strings.HasSuffix(host, sufijo) {
			return
		}
	}
	key := host + "\t" + path
	dst[key] = observation{Host: host, Path: path}
}

func fatal(err error) {
	if !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
	}
	os.Exit(2) //nolint:revive // helper de CLI; todos los fallos son terminales.
}
