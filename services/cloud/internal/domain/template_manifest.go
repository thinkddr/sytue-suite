// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Plantillas de Sytue Cloud — catálogo declarativo embebido en el binario
// ---------------------------------------------------------------------------
//
// Una plantilla es un manifiesto JSON declarativo, NO una imagen de disco ni un
// repo que clonemos. Un «1-Click» de DigitalOcean es una imagen de máquina
// virtual; aquí no hay máquinas que arrancar, así que se copia la SENSACIÓN, no
// la implementación: el manifiesto describe con qué ajustes arranca un proyecto
// y el servidor los aplica de una vez.
//
// El catálogo es GLOBAL y de SOLO LECTURA (mismo carácter que los tamaños de
// instancia o el tarifario): vive en ficheros embebidos, no en una tabla, así
// que no hay migración, ni CRUD, ni RLS. Añadir una plantilla es añadir un
// fichero.

// TemplateManifest es una plantilla del catálogo.
//
// INVARIANTE DE SEGURIDAD — el manifiesto NO PUEDE expresar configuración de pod.
// El esquema es CERRADO: ParseTemplateManifest decodifica con
// DisallowUnknownFields, así que una clave que no esté AQUÍ es un error de carga
// que revienta el arranque (y el test del catálogo). Por eso `securityContext`,
// `hostNetwork`, `capabilities`, `volumes`, `nodeSelector` o `command` no se
// filtran con una lista negra —que habría que ir ampliando cada vez que
// Kubernetes añade un campo— sino por no estar en la lista blanca. El PodSpec lo
// sigue escribiendo el controlador: «seguro por defecto» es el suelo ESTRUCTURAL
// y ninguna plantilla puede bajarlo, ni por error ni a propósito.
type TemplateManifest struct {
	// ID es el slug estable de la plantilla (etiqueta DNS, p. ej. "go-api-postgres").
	ID string `json:"id"`
	// Name y Description son lo que se pinta en el catálogo. Description, honesta
	// y de una línea: describe los AJUSTES, no promete desplegar una app ajena.
	Name        string `json:"name"`
	Description string `json:"description"`
	// Version versiona el manifiesto (semver), para poder decir con qué revisión
	// del catálogo se creó un proyecto.
	Version string `json:"version"`
	// License es la licencia del STACK que la plantilla configura —y por tanto de
	// lo que el usuario acaba ejecutando—. Obligatoria y contra una allowlist
	// permisiva: así «nada de AGPL ni source-available» es un gate MECÁNICO del
	// test de carga, no una revisión humana que se olvida.
	License string `json:"license"`
	// Maintainer es quién responde por la plantilla ("Sytue" para las del catálogo).
	Maintainer string `json:"maintainer"`
	// Framework guía la build (ver builder/detect.go).
	Framework Framework `json:"framework"`
	// Build son los comandos y la estrategia de construcción.
	Build TemplateBuild `json:"build"`
	// AppPort es el puerto en el que escucha la app; 0 = el default (8080).
	AppPort int `json:"app_port,omitempty"`
	// HealthPath es la ruta de la sonda de salud; vacío = el default ("/").
	HealthPath string `json:"health_path,omitempty"`
	// Services son los servicios gestionados que la plantilla aprovisiona. Hasta
	// ahora esto era decorativo (se pintaba y no viajaba); aquí SÍ se aprovisionan.
	Services []ServiceKind `json:"services,omitempty"`
	// Env declara cada variable de entorno y, sobre todo, DE DÓNDE sale su valor.
	Env []TemplateEnv `json:"env,omitempty"`
	// InstanceSize es el tamaño de instancia sugerido; vacío = el default.
	InstanceSize string `json:"instance_size,omitempty"`
	// DocsURL apunta a la documentación OFICIAL del stack; nunca a un repo
	// clonable que afirmemos desplegable.
	DocsURL string `json:"docs_url,omitempty"`
	// StarterRepo es el repositorio PLANTILLA de Sytue Git del que se copia el
	// esqueleto cuando el usuario pide «crear un repositorio nuevo», en la forma
	// «owner/repo». Vacío = esta plantilla solo configura ajustes y el usuario
	// tiene que traer su propio repositorio.
	//
	// Es una referencia y no un árbol de ficheros embebido a propósito: un
	// esqueleto de Astro o de Go envejece con su ecosistema, y tenerlo dentro del
	// binario obligaría a una release de la suite para actualizar una dependencia
	// de un fichero de ejemplo. Como repositorio, lo mantiene quien mantiene el
	// stack, se le ven los commits y se puede probar clonándolo.
	//
	// El copiado lo hace la forja (`POST /repos/{owner}/{repo}/generate`), así que
	// aquí no hay ni un byte de contenido ajeno.
	StarterRepo string `json:"starter_repo,omitempty"`
	// Tags son etiquetas de filtrado del catálogo.
	Tags []string `json:"tags,omitempty"`
}

// TemplateBuild son los ajustes de construcción de la plantilla. Se corresponden
// uno a uno con los campos homónimos del proyecto.
type TemplateBuild struct {
	// Strategy fuerza dockerfile (Kaniko) o buildpacks (CNB); vacío = auto.
	Strategy BuildStrategy `json:"strategy,omitempty"`
	// Install y Build son los comandos de instalación de dependencias y de build.
	Install string `json:"install,omitempty"`
	Build   string `json:"build,omitempty"`
	// OutputDir es el directorio de artefactos (sitios estáticos).
	OutputDir string `json:"output_dir,omitempty"`
}

// TemplateEnvOrigin dice de dónde sale el valor de una variable de entorno. Es
// el corazón de la promesa del producto: EL USUARIO NUNCA INVENTA UNA CONTRASEÑA.
type TemplateEnvOrigin string

const (
	// EnvOriginValue: literal escrito en el manifiesto. Como el manifiesto vive en
	// el repositorio, un literal NUNCA puede ser secreto (ver validateEnv).
	EnvOriginValue TemplateEnvOrigin = "value"
	// EnvOriginAsk: lo pide el asistente al crear el proyecto. Idealmente cero o
	// uno por plantilla: cada pregunta es un paso más entre el usuario y su app.
	EnvOriginAsk TemplateEnvOrigin = "ask"
	// EnvOriginGenerate: secreto aleatorio de 256 bits que genera LA PLATAFORMA y
	// se guarda cifrado. Es a la vez lo más fácil (no hay nada que teclear) y lo
	// más seguro (no hay contraseña humana que adivinar ni reutilizar).
	EnvOriginGenerate TemplateEnvOrigin = "generate"
	// EnvOriginFromService: la inyecta el controlador desde las credenciales del
	// servicio gestionado (DATABASE_URL, S3_*). No se persiste como variable del
	// proyecto: solo se DECLARA, para que el catálogo pueda decir qué recibirá la app.
	EnvOriginFromService TemplateEnvOrigin = "from_service"
)

// TemplateEnv es una variable de entorno declarada por la plantilla.
type TemplateEnv struct {
	// Key es la clave POSIX (MAYUS_CON_GUION_BAJO).
	Key string `json:"key"`
	// Origin es de dónde sale el valor.
	Origin TemplateEnvOrigin `json:"origin"`
	// Value es el literal, solo con origin=value.
	Value string `json:"value,omitempty"`
	// Prompt es la etiqueta que ve el usuario, solo con origin=ask.
	Prompt string `json:"prompt,omitempty"`
	// Secret marca que el valor se cifra en reposo y no se devuelve por la API.
	// Con origin=generate es SIEMPRE true (lo fuerza la validación); con
	// origin=value es SIEMPRE false (un literal del repositorio no es un secreto).
	Secret bool `json:"secret,omitempty"`
	// Service es el servicio gestionado que la aporta, solo con origin=from_service.
	Service ServiceKind `json:"service,omitempty"`
}

// templateLicenses es la allowlist PERMISIVA de licencias. Ampliar esta lista es
// una decisión deliberada: es el único sitio donde se decide qué código puede
// acabar corriendo un cliente por recomendación nuestra.
var templateLicenses = []string{
	"MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC", "MPL-2.0",
}

// templateServiceKinds son los servicios que una plantilla puede declarar. Se
// quedan fuera cron/job/functions porque los tres exigen una imagen OCI del
// propio proyecto (ProvisionService la valida contra el registro), y esa imagen
// no existe hasta que termina la PRIMERA build — es decir, después del alta.
var templateServiceKinds = []ServiceKind{ServicePostgres, ServiceStorage, ServiceAuth}

//go:embed templates/*.json
var templateFiles embed.FS

// StarterTemplates es el catálogo, cargado del embed al arrancar y ordenado por
// id. Un manifiesto inválido NO degrada en silencio: revienta el proceso al
// iniciar, que es cuando se nota y es barato arreglarlo.
var StarterTemplates = mustLoadTemplates()

// SetStarterTemplatesForTest sustituye el catálogo y devuelve cómo restaurarlo.
//
// El catálogo es un embed global, así que sin esta costura una función que lo
// consulte solo se puede probar con las plantillas que haya escritas ese día: el
// test del alta con repositorio nuevo se saltaba entero por no haber todavía
// ninguna con `starter_repo`, y un test que siempre se salta no comprueba nada.
func SetStarterTemplatesForTest(tpls []TemplateManifest) (restore func()) {
	previo := StarterTemplates
	StarterTemplates = tpls
	return func() { StarterTemplates = previo }
}

// StarterTemplateByID devuelve la plantilla con el id dado, o (zero, false).
func StarterTemplateByID(id string) (TemplateManifest, bool) {
	for _, t := range StarterTemplates {
		if t.ID == id {
			return t, true
		}
	}
	return TemplateManifest{}, false
}

// LoadTemplateManifests lee, valida y devuelve TODOS los manifiestos embebidos,
// ordenados por id. Cualquier fichero que no cumpla el esquema es un error.
func LoadTemplateManifests() ([]TemplateManifest, error) {
	entries, err := fs.ReadDir(templateFiles, "templates")
	if err != nil {
		return nil, fmt.Errorf("plantillas: leer directorio: %w", err)
	}
	out := make([]TemplateManifest, 0, len(entries))
	seen := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := templateFiles.ReadFile("templates/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("plantillas: leer %s: %w", e.Name(), err)
		}
		m, err := ParseTemplateManifest(raw)
		if err != nil {
			return nil, fmt.Errorf("plantillas: %s: %w", e.Name(), err)
		}
		if prev, dup := seen[m.ID]; dup {
			return nil, fmt.Errorf("plantillas: %s: id %q duplicado (ya lo usa %s)", e.Name(), m.ID, prev)
		}
		seen[m.ID] = e.Name()
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("plantillas: el catálogo embebido está vacío")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func mustLoadTemplates() []TemplateManifest {
	list, err := LoadTemplateManifests()
	if err != nil {
		panic(err)
	}
	return list
}

// ParseTemplateManifest decodifica y valida un manifiesto. La decodificación es
// ESTRICTA (DisallowUnknownFields): ahí es donde muere cualquier intento de
// colar configuración de pod.
func ParseTemplateManifest(raw []byte) (TemplateManifest, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m TemplateManifest
	if err := dec.Decode(&m); err != nil {
		return TemplateManifest{}, fmt.Errorf("manifiesto inválido: %w", err)
	}
	if dec.More() {
		return TemplateManifest{}, fmt.Errorf("manifiesto inválido: contenido extra tras el objeto JSON")
	}
	if err := m.validate(); err != nil {
		return TemplateManifest{}, err
	}
	// generate es secreto por definición: no es una opción del manifiesto, así que
	// se fija aquí y no se confía en que el fichero lo declare.
	for i := range m.Env {
		if m.Env[i].Origin == EnvOriginGenerate {
			m.Env[i].Secret = true
		}
	}
	return m, nil
}

// validate comprueba la identidad, la licencia y los ajustes de ejecución.
func (m TemplateManifest) validate() error {
	switch {
	case !ValidSlug(m.ID):
		return fmt.Errorf("id inválido %q: debe ser una etiqueta DNS (minúsculas [a-z0-9-], 1-40)", m.ID)
	case strings.TrimSpace(m.Name) == "":
		return fmt.Errorf("%s: falta name", m.ID)
	case strings.TrimSpace(m.Description) == "":
		return fmt.Errorf("%s: falta description", m.ID)
	case strings.TrimSpace(m.Version) == "":
		return fmt.Errorf("%s: falta version", m.ID)
	case strings.TrimSpace(m.Maintainer) == "":
		return fmt.Errorf("%s: falta maintainer", m.ID)
	case !slices.Contains(templateLicenses, m.License):
		return fmt.Errorf("%s: licencia %q fuera de la allowlist permisiva %v", m.ID, m.License, templateLicenses)
	case !ValidFramework(m.Framework):
		return fmt.Errorf("%s: framework inválido %q", m.ID, m.Framework)
	case !ValidBuildStrategy(m.Build.Strategy):
		return fmt.Errorf("%s: build.strategy inválida %q", m.ID, m.Build.Strategy)
	case m.DocsURL != "" && !strings.HasPrefix(m.DocsURL, "https://"):
		return fmt.Errorf("%s: docs_url debe ser https, es %q", m.ID, m.DocsURL)
	case m.StarterRepo != "" && !validStarterRepo(m.StarterRepo):
		return fmt.Errorf(
			"%s: starter_repo tiene que ser «owner/repo» de Sytue Git y llegó %q. No es una URL: el "+
				"host lo pone el despliegue, igual que el registro en el catálogo de apps",
			m.ID, m.StarterRepo,
		)
	}
	if err := ValidateAppPort(m.AppPort); err != nil {
		return fmt.Errorf("%s: %w", m.ID, err)
	}
	if err := ValidateHealthPath(m.HealthPath); err != nil {
		return fmt.Errorf("%s: %w", m.ID, err)
	}
	if err := ValidateInstanceSize(m.InstanceSize); err != nil {
		return fmt.Errorf("%s: %w", m.ID, err)
	}
	for _, k := range m.Services {
		if !slices.Contains(templateServiceKinds, k) {
			return fmt.Errorf("%s: una plantilla no puede aprovisionar %q (solo %v)", m.ID, k, templateServiceKinds)
		}
	}
	if len(m.Services) != len(uniqueKinds(m.Services)) {
		return fmt.Errorf("%s: servicio declarado dos veces", m.ID)
	}
	return m.validateEnv()
}

// validateEnv comprueba que cada variable declara un origen coherente. Las dos
// reglas que importan: un literal del manifiesto no puede ser secreto (vive en
// el repositorio, en claro, para siempre) y una variable from_service tiene que
// venir de un servicio que la plantilla aprovisione de verdad.
func (m TemplateManifest) validateEnv() error {
	seen := make(map[string]bool, len(m.Env))
	for _, e := range m.Env {
		switch {
		case !ValidEnvKey(e.Key):
			return fmt.Errorf("%s: clave de variable inválida %q (use MAYUS_CON_GUION_BAJO)", m.ID, e.Key)
		case seen[e.Key]:
			return fmt.Errorf("%s: variable %q declarada dos veces", m.ID, e.Key)
		}
		seen[e.Key] = true
		if err := m.validateEnvOrigin(e); err != nil {
			return err
		}
	}
	return nil
}

func (m TemplateManifest) validateEnvOrigin(e TemplateEnv) error {
	switch e.Origin {
	case EnvOriginValue:
		if e.Value == "" {
			return fmt.Errorf("%s: %s con origin=value necesita value", m.ID, e.Key)
		}
		if e.Secret {
			return fmt.Errorf("%s: %s no puede ser secret con origin=value: un literal del "+
				"manifiesto está en el repositorio en claro; usa origin=generate", m.ID, e.Key)
		}
	case EnvOriginAsk:
		if strings.TrimSpace(e.Prompt) == "" {
			return fmt.Errorf("%s: %s con origin=ask necesita prompt (lo lee el usuario)", m.ID, e.Key)
		}
		if e.Value != "" {
			return fmt.Errorf("%s: %s con origin=ask no lleva value", m.ID, e.Key)
		}
	case EnvOriginGenerate:
		if e.Value != "" || e.Prompt != "" {
			return fmt.Errorf("%s: %s con origin=generate no lleva value ni prompt: "+
				"el valor lo genera la plataforma", m.ID, e.Key)
		}
	case EnvOriginFromService:
		if !slices.Contains(m.Services, e.Service) {
			return fmt.Errorf("%s: %s viene de un servicio %q que la plantilla no aprovisiona",
				m.ID, e.Key, e.Service)
		}
		if e.Value != "" || e.Prompt != "" || e.Secret {
			return fmt.Errorf("%s: %s con origin=from_service solo lleva service: "+
				"el valor lo inyecta el controlador", m.ID, e.Key)
		}
	default:
		return fmt.Errorf("%s: %s tiene origin desconocido %q (value|ask|generate|from_service)",
			m.ID, e.Key, e.Origin)
	}
	return nil
}

// starterRepoRe acota `owner/repo` a lo que Gitea acepta como nombre. Es
// deliberadamente estricta: este valor se interpola en la ruta de una llamada a la
// API de la forja, así que un `..` o una barra de más ahí es un cambio de destino.
var starterRepoRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}/[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

func validStarterRepo(s string) bool {
	return starterRepoRe.MatchString(s) && !strings.Contains(s, "..")
}

func uniqueKinds(ks []ServiceKind) []ServiceKind {
	seen := make(map[ServiceKind]bool, len(ks))
	out := make([]ServiceKind, 0, len(ks))
	for _, k := range ks {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}
