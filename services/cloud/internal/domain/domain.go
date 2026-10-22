// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define los tipos y reglas de negocio de Sytue Cloud (el PaaS:
// Vercel × Supabase × Google Cloud), independientes del transporte (HTTP) y del
// almacenamiento (PostgreSQL/S3).
//
// Modelo: una organización (tenant) agrupa proyectos; cada proyecto conecta un
// repositorio git, produce builds, despliega deployments (prod/preview) con
// dominios y variables/secretos, y consume servicios gestionados (Sytue
// Postgres, Storage, Functions, Cron, Auth) cuyas credenciales se inyectan en
// los deployments.
//
// Clean-room: control plane original sobre estándares abiertos (OCI, Kubernetes,
// OIDC, S3, PostgREST) y librerías permisivas; sin código de terceros AGPL.
package domain

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// Errores de dominio. La capa HTTP los traduce a *httpx.Error.
var (
	ErrNotFound            = errors.New("cloud: recurso no encontrado")
	ErrConflict            = errors.New("cloud: conflicto de nombres")
	ErrInvalidInput        = errors.New("cloud: entrada inválida")
	ErrInvalidState        = errors.New("cloud: estado inválido para la operación")
	ErrIdentityUnavailable = errors.New("cloud: servicio de identidad no disponible")
	ErrQuotaExceeded       = errors.New("cloud: cuota de la organización superada")
	ErrQueueFull           = errors.New("cloud: demasiado trabajo en cola")
	// ErrSuiteRequired: el plan exige una suscripción activa a la suite Sytue
	// (Mail/Drive/Docs/Rooms). Se mapea a 402 como el resto de bloqueos de pago.
	ErrSuiteRequired    = errors.New("cloud: requiere una suscripción a la suite Sytue")
	ErrPermission       = errors.New("cloud: permiso insuficiente")
	ErrDomainTaken      = errors.New("cloud: el dominio ya está en uso")
	ErrSignatureInvalid = errors.New("cloud: firma de webhook inválida")
)

// slugRe valida slugs DNS-compatibles (usados en subdominios de apps/previews).
var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

// ValidSlug indica si un slug de proyecto/app es DNS-compatible.
func ValidSlug(s string) bool { return slugRe.MatchString(s) }

// ---------------------------------------------------------------------------
// Organización (tenant) — facturación y cuotas a nivel de organización
// ---------------------------------------------------------------------------

// Plan es el plan de facturación de la organización.
type Plan string

const (
	PlanFree Plan = "free"
	PlanPro  Plan = "pro"
	PlanTeam Plan = "team"
)

// ValidPlan valida un plan.
func ValidPlan(p Plan) bool { return p == PlanFree || p == PlanPro || p == PlanTeam }

// Organization es la organización (tenant) propietaria de proyectos y recursos.
type Organization struct {
	ID     uuid.UUID `json:"id"`
	Slug   string    `json:"slug"`
	Name   string    `json:"name"`
	Plan   Plan      `json:"plan"`
	Region string    `json:"region"` // siempre UE (fr-par por defecto)
	// NotifyWebhookURL recibe el aviso POST de deploy (ready/error) por defecto
	// de toda la organización; un proyecto puede sobreescribirlo. Anti-SSRF.
	NotifyWebhookURL string    `json:"notify_webhook_url,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Proyectos
// ---------------------------------------------------------------------------

// Framework es el framework detectado/declarado del proyecto (guía la build).
type Framework string

const (
	FrameworkAstro  Framework = "astro"
	FrameworkStatic Framework = "static"
	FrameworkNode   Framework = "node"
	FrameworkGo     Framework = "go"
	FrameworkDocker Framework = "docker"
	FrameworkDetect Framework = "auto"
)

// ValidFramework valida un framework.
func ValidFramework(f Framework) bool {
	switch f {
	case FrameworkAstro, FrameworkStatic, FrameworkNode, FrameworkGo, FrameworkDocker, FrameworkDetect:
		return true
	default:
		return false
	}
}

// BuildStrategy selecciona CÓMO se construye la imagen OCI de un proyecto:
// con el Dockerfile (propio o generado) vía Kaniko, o con Cloud Native
// Buildpacks (el lifecycle de CNB auto-detecta el lenguaje: deploy sin
// Dockerfile, paridad DigitalOcean App Platform / Heroku / Coolify Nixpacks).
type BuildStrategy string

const (
	// BuildStrategyAuto (valor por defecto, "") deja que el builder elija: solo
	// 'auto'/detect (framework sin resolver) => buildpacks; los gestionados
	// (docker + astro/static/node/go) conservan su Dockerfile => dockerfile. Ver
	// builder.ResolveBuildStrategy.
	BuildStrategyAuto BuildStrategy = ""
	// BuildStrategyDockerfile fuerza Kaniko + Dockerfile (propio o generado).
	BuildStrategyDockerfile BuildStrategy = "dockerfile"
	// BuildStrategyBuildpacks fuerza el lifecycle de CNB (Paketo builder).
	BuildStrategyBuildpacks BuildStrategy = "buildpacks"
)

// ValidBuildStrategy valida una estrategia de build (vacío = auto).
func ValidBuildStrategy(s BuildStrategy) bool {
	switch s {
	case BuildStrategyAuto, BuildStrategyDockerfile, BuildStrategyBuildpacks:
		return true
	default:
		return false
	}
}

// GitProvider es el proveedor del repositorio conectado.
type GitProvider string

const (
	GitGeneric GitProvider = "git"    // repo git genérico (HTTPS/SSH)
	GitHub     GitProvider = "github" // webhooks estilo GitHub
	GitLab     GitProvider = "gitlab" // webhooks estilo GitLab
)

// commitSHARe valida un SHA de commit git (hex de 7 a 64 caracteres, cubre SHA-1
// y SHA-256). GitRefRe valida un nombre de rama/ref sin metacaracteres peligrosos.
var (
	commitSHARe = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)
	gitRefRe    = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._/-]{0,254}$`)
)

// ValidCommitSHA indica si s es un SHA de commit git plausible.
func ValidCommitSHA(s string) bool { return commitSHARe.MatchString(s) }

// MaxCommitMessage acota el asunto de commit que se guarda y se muestra. El
// CHECK de la tabla `deployments` repite el mismo tope a propósito: bajarlo
// obliga a tocar las dos mitades.
const MaxCommitMessage = 200

// SanitizeCommitMessage normaliza el mensaje de un commit para guardarlo y
// mostrarlo. El texto lo escribe QUIEN HACE EL PUSH —no la plataforma—, así que
// entra como dato hostil: se queda solo la PRIMERA línea (el asunto, que es lo
// que la UI enseña en una fila truncada), sin caracteres de control que rompan
// el renderizado ni los logs, y con un tope duro de MaxCommitMessage runas.
//
// No escapa HTML: eso es trabajo de quien pinta (Preact escapa por defecto), y
// hacerlo aquí guardaría `&amp;` en la base de datos.
func SanitizeCommitMessage(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > MaxCommitMessage {
		// -1 para que el resultado quepa EXACTAMENTE en el tope contando la elipsis.
		s = strings.TrimSpace(string(r[:MaxCommitMessage-1])) + "…"
	}
	return s
}

// ValidGitRef indica si s es un nombre de rama/ref git seguro (sin espacios,
// sin '..', sin caracteres de control ni metacaracteres). Se usa para el contexto
// git de Kaniko (`git://host#ref`) y evita refs manipulados.
func ValidGitRef(s string) bool {
	if !gitRefRe.MatchString(s) || strings.Contains(s, "..") {
		return false
	}
	return true
}

// ValidateRepoURL valida la URL de un repositorio conectado (anti-SSRF): solo
// admite https:// y SSH estilo git@host:path. Rechaza esquemas peligrosos
// (file/gopher/http en claro) y destinos internos (loopback, IP privadas,
// link-local/metadata cloud). El fetch/clone se realiza server-side por el Job de
// Kaniko, así que un destino arbitrario permitiría alcanzar servicios internos.
func ValidateRepoURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%w: repo_url vacío", ErrInvalidInput)
	}
	// Forma SSH scp-like: git@host:owner/repo(.git)
	if strings.HasPrefix(raw, "git@") {
		rest := raw[len("git@"):]
		host, _, ok := strings.Cut(rest, ":")
		// La única host key SSH inventariada hoy es la de GitHub (excepción
		// temporal aprobada). HTTPS genérico sigue disponible; una nueva forja SSH
		// exige añadir y revisar su clave antes de aceptar deploy keys.
		if !ok || !strings.EqualFold(host, "github.com") || isBlockedHost(host) {
			return fmt.Errorf("%w: host SSH de repo_url no permitido", ErrInvalidInput)
		}
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: repo_url mal formada", ErrInvalidInput)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: repo_url debe usar https o SSH (git@host)", ErrInvalidInput)
	}
	host := u.Hostname()
	if host == "" || isBlockedHost(host) {
		return fmt.Errorf("%w: host de repo_url no permitido", ErrInvalidInput)
	}
	return nil
}

// isBlockedHost indica si un host es un destino interno/no enrutable que no debe
// alcanzarse desde una build (SSRF): loopback, privadas RFC1918, link-local
// (incluida la metadata 169.254.169.254), ULA IPv6 y nombres locales del clúster.
func isBlockedHost(host string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".cluster.local") ||
		!strings.Contains(host, ".") { // nombre "desnudo" (p. ej. servicio de k8s)
		if ip := net.ParseIP(host); ip == nil {
			return true
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		return IsInternalIP(ip)
	}
	return false
}

// IsInternalIP indica si ip es un destino interno/no enrutable al que un fetch
// server-side (clone de repo, webhook saliente) NO debe conectar (SSRF).
// Delega en ssrfhttp.IsBlockedIP, el denylist ÚNICO de la suite: loopback,
// privadas RFC1918/ULA, link-local (incluida la metadata 169.254.169.254),
// multicast, unspecified y CGNAT 100.64.0.0/10 —que es la CIDR de pods del
// clúster, así que omitirla dejaba alcanzables los servicios internos—. Una
// sola lista para que no vuelva a divergir. Lo comparten isBlockedHost
// (chequeo por host, pre-conexión) y el dialer del emisor de webhooks
// (post-resolución DNS, defensa contra rebinding a una IP interna).
func IsInternalIP(ip net.IP) bool { return ssrfhttp.IsBlockedIP(ip) }

// ValidateWebhookURL valida la URL destino de un webhook saliente de
// notificación de deploy (anti-SSRF), con el mismo criterio que ValidateRepoURL:
// admite http/https y rechaza esquemas peligrosos y destinos internos por HOST
// (loopback, RFC1918, link-local/metadata, nombres locales del clúster). Es la
// validación PRE-persistencia; el emisor añade un guard en el dial (IsInternalIP
// tras resolver DNS) contra rebinding de un hostname público a una IP interna.
func ValidateWebhookURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%w: URL de webhook vacía", ErrInvalidInput)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: URL de webhook mal formada", ErrInvalidInput)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("%w: la URL de webhook debe usar http o https", ErrInvalidInput)
	}
	host := u.Hostname()
	if host == "" || isBlockedHost(host) {
		return fmt.Errorf("%w: host de webhook no permitido", ErrInvalidInput)
	}
	return nil
}

// mailAddrRe es una dirección de correo suficiente para un destinatario de
// alertas: hay un arroba, algo a cada lado y un punto en el dominio. No se
// pretende implementar el RFC 5322 —que acepta cosas que ningún servidor real
// entrega— sino descartar lo que es un error de tecleo.
var mailAddrRe = regexp.MustCompile(`^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$`)

// ValidateAlertChannel valida el destino de una alerta: un webhook http(s) con
// las mismas garantías anti-SSRF de siempre, o un `mailto:` con una dirección.
//
// El correo entra por aquí y no por un campo nuevo porque el canal YA es una URL:
// `mailto:` es un esquema legítimo y evita una columna, una migración y un
// segundo camino en el consumidor de alertas.
func ValidateAlertChannel(raw string) error {
	raw = strings.TrimSpace(raw)
	if addr, ok := MailChannelAddress(raw); ok {
		if !mailAddrRe.MatchString(addr) {
			return fmt.Errorf("%w: dirección de correo inválida en el canal de alerta", ErrInvalidInput)
		}
		return nil
	}
	return ValidateWebhookURL(raw)
}

// MailChannelAddress extrae la dirección de un canal `mailto:`. ok=false si el
// canal no es de correo.
//
// Se descarta todo lo que venga después de «?» (asunto, cuerpo, cc): un mailto
// puede llevarlo y aquí no significa nada, salvo la posibilidad de que alguien
// cuele cabeceras en el correo que enviamos nosotros.
func MailChannelAddress(raw string) (string, bool) {
	const prefijo = "mailto:"
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(trimmed), prefijo) {
		return "", false
	}
	addr := trimmed[len(prefijo):]
	if i := strings.IndexByte(addr, '?'); i >= 0 {
		addr = addr[:i]
	}
	return strings.TrimSpace(addr), true
}

// Project es una unidad desplegable conectada a un repositorio git.
type Project struct {
	ID            uuid.UUID   `json:"id"`
	Tenant        string      `json:"-"`
	Slug          string      `json:"slug"`
	Name          string      `json:"name"`
	RepoURL       string      `json:"repo_url"`
	GitProvider   GitProvider `json:"git_provider"`
	ProductionRef string      `json:"production_ref"` // rama de producción (main)
	Framework     Framework   `json:"framework"`
	// BuildStrategy selecciona dockerfile (Kaniko) vs buildpacks (CNB); vacío =
	// auto (el builder elige según el framework). Ver builder.ResolveBuildStrategy.
	BuildStrategy BuildStrategy `json:"build_strategy,omitempty"`
	RootDir       string        `json:"root_dir,omitempty"` // subdirectorio del monorepo
	BuildCommand  string        `json:"build_command,omitempty"`
	OutputDir     string        `json:"output_dir,omitempty"`
	InstallCmd    string        `json:"install_command,omitempty"`
	// AppPort es el puerto en el que ESCUCHA la app (el que sondean las probes y
	// al que enruta el Service). 0 = usar DefaultAppPort. Antes era una constante
	// del controlador, lo que dejaba colgada en `deploying` a cualquier imagen que
	// no escuchara en 8080 (Next 3000, Django 8000, Flask 5000…).
	AppPort int `json:"app_port,omitempty"`
	// HealthPath es la ruta HTTP de las sondas de readiness/liveness. Vacío = "/".
	HealthPath string `json:"health_path,omitempty"`
	// InstanceSize es el tamaño del catálogo (domain.InstanceSizes); vacío = basic.
	InstanceSize string `json:"instance_size,omitempty"`
	// InstanceCount son las instancias deseadas (mínimo del autoescalado); 0 = 1.
	InstanceCount int `json:"instance_count,omitempty"`
	// AutoscaleMaxCount es el TECHO del autoescalado; 0 = automático (la regla
	// histórica: 5× las instancias, mínimo 3). Sin él, la única forma de aguantar
	// un pico era subir el mínimo, que se paga las 24 horas haya tráfico o no.
	AutoscaleMaxCount int `json:"autoscale_max_count,omitempty"`
	// AutoscaleCPUTarget es el % de CPU al que el HPA empieza a escalar;
	// 0 = DefaultCPUTarget.
	AutoscaleCPUTarget int `json:"autoscale_cpu_target,omitempty"`
	// ReleaseCommand se ejecuta ANTES de que arranque la versión nueva, con su
	// misma imagen y su mismo entorno: el sitio donde va una migración de base de
	// datos. Vacío = no hay nada que ejecutar, que es el caso por defecto.
	//
	// Vive en el proyecto y no en el deployment porque es configuración, como la
	// escala: así un rollback no revive el comando de release de la versión vieja.
	ReleaseCommand string `json:"release_command,omitempty"`
	// PreviewAuthHash es la línea htpasswd que cierra las previews del proyecto.
	// NUNCA viaja al cliente (`json:"-"`): lo que la interfaz necesita saber es SI
	// están protegidas, y para eso está PreviewProtected.
	PreviewAuthHash string `json:"-"`
	// PreviewProtected es el hash de arriba reducido a lo que sí puede salir: un
	// sí/no. Lo deriva el store al leer la fila, para que ninguna capa de arriba
	// tenga que acordarse de calcularlo (ni de no publicar el hash).
	PreviewProtected bool `json:"preview_protected,omitempty"`
	// Paused apaga el proyecto sin borrarlo: cero réplicas y cero facturación de
	// cómputo. Dominio, TLS, variables, base de datos e historial se quedan.
	Paused bool `json:"paused,omitempty"`
	// HTTPRules son las redirecciones y cabeceras que Traefik aplica ANTES de que
	// la petición llegue a la app: lo que en Vercel se escribe en `vercel.json`.
	HTTPRules HTTPRules `json:"http_rules"`
	OwnerSub  string    `json:"owner_sub,omitempty"`
	// NotifyWebhookURL recibe el aviso POST de deploy (ready/error) de ESTE
	// proyecto; si está vacío, cae al de la organización. Validada anti-SSRF.
	NotifyWebhookURL string    `json:"notify_webhook_url,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// DefaultAppPort es el puerto de las plantillas de Dockerfile que genera el
// builder; se usa cuando el proyecto no fija uno propio.
const DefaultAppPort = 8080

// DefaultHealthPath es la ruta de las sondas cuando el proyecto no fija otra.
const DefaultHealthPath = "/"

// EffectiveAppPort devuelve el puerto de escucha efectivo del proyecto.
func (p Project) EffectiveAppPort() int {
	if p.AppPort <= 0 || p.AppPort > 65535 {
		return DefaultAppPort
	}
	return p.AppPort
}

// EffectiveHealthPath devuelve la ruta de sondeo efectiva del proyecto.
func (p Project) EffectiveHealthPath() string {
	if p.HealthPath == "" || !strings.HasPrefix(p.HealthPath, "/") {
		return DefaultHealthPath
	}
	return p.HealthPath
}

// PreviewAuthUser es el usuario fijo de las previews protegidas.
//
// Fijo y no configurable a propósito: un campo más que rellenar en un formulario
// no aporta nada —lo que protege es la contraseña— y sí añade una forma de
// equivocarse al teclearlo en el navegador.
const PreviewAuthUser = "preview"

// MaxReleaseCommandLen es el tope del comando de release. Coincide con el CHECK
// de la migración 0038: el valor acaba en el `args` de un contenedor, y pasarse
// aquí produce un rechazo del API server de Kubernetes cuando ya es tarde.
const MaxReleaseCommandLen = 2000

// ValidateReleaseCommand acepta un comando vacío (no hay release) o uno acotado.
//
// NO se valida el CONTENIDO: es una orden de shell del cliente que corre en su
// propio contenedor, con su imagen y sus credenciales. Filtrarla por palabras
// sería teatro —quien pueda desplegar ya ejecuta lo que quiera dentro de su
// imagen— y rompería comandos legítimos.
func ValidateReleaseCommand(cmd string) error {
	if len(cmd) > MaxReleaseCommandLen {
		return fmt.Errorf("%w: el comando de release no puede pasar de %d caracteres",
			ErrInvalidInput, MaxReleaseCommandLen)
	}
	return nil
}

// EffectiveInstanceSize devuelve el tamaño de instancia efectivo del proyecto.
func (p Project) EffectiveInstanceSize() InstanceSize {
	return InstanceSizeOrDefault(p.InstanceSize)
}

// EffectiveInstanceCount devuelve las instancias efectivas (siempre >= 1).
func (p Project) EffectiveInstanceCount() int {
	if p.InstanceCount < 1 {
		return 1
	}
	if p.InstanceCount > MaxInstanceCount {
		return MaxInstanceCount
	}
	return p.InstanceCount
}

// EffectiveAutoscaleMax devuelve el techo efectivo del autoescalado, siempre >=
// el mínimo (EffectiveInstanceCount).
//
// Ese suelo no es cosmético: un HPA con maxReplicas < minReplicas lo RECHAZA el
// API server, y como el techo y las instancias se cambian por separado, subir
// las instancias por encima de un techo fijado antes dejaría la app sin poder
// aplicar NINGÚN objeto —no solo sin escalar—. Se resuelve aquí, en el único
// sitio por el que pasan todos los llamantes, en vez de validar la combinación
// en cada superficie.
func (p Project) EffectiveAutoscaleMax() int {
	suelo := p.EffectiveInstanceCount()
	if p.AutoscaleMaxCount <= 0 {
		// Automático: la regla que el controlador aplicaba antes de que el techo
		// fuese configurable. El suelo de 3 que llevaba escrito era inalcanzable
		// —el mínimo nunca baja de 1, así que 5× nunca baja de 5— y no se replica.
		// Tampoco se acota a MaxInstanceCount: hacerlo BAJARÍA el techo de las apps
		// con más de 10 instancias, que hoy ya lo tienen mayor.
		return suelo * autoscaleAutoFactor
	}
	if p.AutoscaleMaxCount < suelo {
		return suelo
	}
	return p.AutoscaleMaxCount
}

// EffectiveAutoscaleCPUTarget devuelve el objetivo de CPU efectivo del HPA. Un
// valor fuera de rango persistido (migración a medias, escritura futura que se
// olvide de validar) degrada al defecto en vez de llegar a Kubernetes.
func (p Project) EffectiveAutoscaleCPUTarget() int {
	if p.AutoscaleCPUTarget < MinCPUTarget || p.AutoscaleCPUTarget > MaxCPUTarget {
		return DefaultCPUTarget
	}
	return p.AutoscaleCPUTarget
}

// ValidateAppPort acepta el rango de puertos TCP; 0 significa «usa el default».
func ValidateAppPort(port int) error {
	if port == 0 || (port >= 1 && port <= 65535) {
		return nil
	}
	return fmt.Errorf("%w: el puerto debe estar entre 1 y 65535", ErrInvalidInput)
}

// HostnameDeApp es el nombre público de una app del catálogo con web.
//
// Antes era `<app>-<slug>.<dominio de apps>`, que es la misma forma que la URL de
// producción de un proyecto: otra organización creaba el proyecto `gitea-foo` y
// su Ingress competía con el Gitea del proyecto `foo` por el mismo host
// (CLOUD-03). Ahora cuelga de su propia etiqueta `app.` —un slug no lleva
// puntos, así que ningún proyecto la produce— y lleva delante el UUID del
// proyecto, como las funciones (controller/hostname_funcion.go).
func HostnameDeApp(proyecto uuid.UUID, appID, dominioApps string) string {
	return proyecto.String() + "-" + appID + ".app." + dominioApps
}

// ValidateProjectDir valida root_dir y output_dir: una ruta RELATIVA dentro del
// repositorio, sin `..`, sin caracteres de control y acotada. root_dir acaba en
// `APP=/workspace/<root_dir>` del contenedor que lleva la credencial de push y
// en `--local=context=` de BuildKit: con `../` salía de /workspace (CLOUD-15).
// Vacío = la raíz del repositorio.
func ValidateProjectDir(dir string) error {
	if dir == "" {
		return nil
	}
	if len(dir) > 256 || strings.HasPrefix(dir, "/") || strings.Contains(dir, "\\") {
		return fmt.Errorf("%w: directorio inválido: ruta relativa dentro del repositorio", ErrInvalidInput)
	}
	for _, r := range dir {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%w: directorio inválido: caracteres de control", ErrInvalidInput)
		}
	}
	for _, seg := range strings.Split(dir, "/") {
		if seg == ".." {
			return fmt.Errorf("%w: directorio inválido: no puede salir del repositorio", ErrInvalidInput)
		}
	}
	return nil
}

// ValidateHealthPath exige una ruta ABSOLUTA y acotada; vacío = «usa el default».
// Se rechaza aquí y no en el Deployment porque allí el fallo sería opaco.
func ValidateHealthPath(path string) error {
	if path == "" {
		return nil
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("%w: la ruta de health check debe empezar por /", ErrInvalidInput)
	}
	if len(path) > 512 {
		return fmt.Errorf("%w: la ruta de health check es demasiado larga", ErrInvalidInput)
	}
	if strings.ContainsAny(path, " \t\n\r") {
		return fmt.Errorf("%w: la ruta de health check no puede llevar espacios", ErrInvalidInput)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Builds
// ---------------------------------------------------------------------------

// BuildStatus es el estado del ciclo de vida de una build (Kaniko).
type BuildStatus string

const (
	BuildQueued    BuildStatus = "queued"
	BuildRunning   BuildStatus = "running"
	BuildSucceeded BuildStatus = "succeeded"
	BuildFailed    BuildStatus = "failed"
	BuildCanceled  BuildStatus = "canceled"
)

// IsTerminal indica si el estado de build es final.
func (s BuildStatus) IsTerminal() bool {
	return s == BuildSucceeded || s == BuildFailed || s == BuildCanceled
}

// Build es una construcción de imagen OCI de un commit (Kaniko rootless).
type Build struct {
	ID            uuid.UUID   `json:"id"`
	Tenant        string      `json:"-"`
	ProjectID     uuid.UUID   `json:"project_id"`
	CommitSHA     string      `json:"commit_sha"`
	GitRef        string      `json:"git_ref"`
	Status        BuildStatus `json:"status"`
	Framework     Framework   `json:"framework,omitempty"` // detectado durante la build
	ImageRef      string      `json:"image_ref,omitempty"` // registry/imagen@sha256:...
	LogsKey       string      `json:"-"`                   // clave S3 del log de build
	KanikoJobName string      `json:"-"`
	TriggeredBy   string      `json:"triggered_by,omitempty"`
	StartedAt     *time.Time  `json:"started_at,omitempty"`
	FinishedAt    *time.Time  `json:"finished_at,omitempty"`
	ErrorMsg      string      `json:"error,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Deployments
// ---------------------------------------------------------------------------

// DeployTarget distingue producción de previews por PR.
type DeployTarget string

const (
	TargetProduction DeployTarget = "production"
	TargetPreview    DeployTarget = "preview"
)

// ValidTarget valida el destino de despliegue.
func ValidTarget(t DeployTarget) bool {
	return t == TargetProduction || t == TargetPreview
}

// PreviewWithheldNotice redacta el aviso de una vista previa a la que le falta
// algo por no heredar lo de producción. Vacío si no falta nada.
//
// Nombra las claves y los tipos de servicio y dice qué hacer: es lo único que
// distingue «mi app no arranca» de «mi app no arranca PORQUE le falta STRIPE_KEY
// y eso se arregla dándole uno de pruebas».
func PreviewWithheldNotice(secretKeys, serviceKinds []string) string {
	var partes []string
	if len(secretKeys) > 0 {
		partes = append(partes, "secretos sin valor para vistas previas: "+strings.Join(secretKeys, ", ")+
			" (créalos con ámbito «preview»)")
	}
	if len(serviceKinds) > 0 {
		partes = append(partes, "servicios sin equivalente de vista previa: "+strings.Join(serviceKinds, ", ")+
			" (crea uno con destino «preview»)")
	}
	if len(partes) == 0 {
		return ""
	}
	return "Las vistas previas no reciben los secretos ni los servicios de producción. Faltan: " +
		strings.Join(partes, "; ") + "."
}

// DeployStatus es el estado del ciclo de vida de un deployment.
type DeployStatus string

const (
	DeployQueued    DeployStatus = "queued"
	DeployBuilding  DeployStatus = "building"
	DeployDeploying DeployStatus = "deploying"
	DeployReady     DeployStatus = "ready"
	DeployError     DeployStatus = "error"
	DeployCanceled  DeployStatus = "canceled"
	DeployRetired   DeployStatus = "retired" // reemplazado por una promoción posterior
)

// IsActive indica si el deployment sirve tráfico.
func (s DeployStatus) IsActive() bool { return s == DeployReady }

// Deployment es una instancia desplegada de una build (prod o preview).
type Deployment struct {
	ID        uuid.UUID    `json:"id"`
	Tenant    string       `json:"-"`
	ProjectID uuid.UUID    `json:"project_id"`
	BuildID   *uuid.UUID   `json:"build_id,omitempty"`
	ImageRef  string       `json:"image_ref,omitempty"` // imagen BYO (sin build)
	Target    DeployTarget `json:"target"`
	Status    DeployStatus `json:"status"`
	CommitSHA string       `json:"commit_sha"`
	// CommitMessage es el asunto del commit tal y como llegó del webhook, ya
	// saneado (ver SanitizeCommitMessage). Vacío en los despliegues que no nacen
	// de un push: redeploy manual, rollback, plantilla e imagen BYO.
	CommitMessage string     `json:"commit_message,omitempty"`
	GitRef        string     `json:"git_ref"`
	PRNumber      *int       `json:"pr_number,omitempty"`
	URL           string     `json:"url"` // https://<app>.apps.sytue.eu o preview
	Replicas      int        `json:"replicas"`
	CreatedBy     string     `json:"created_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ReadyAt       *time.Time `json:"ready_at,omitempty"`
	ErrorMsg      string     `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// Dominios + TLS (cert-manager)
// ---------------------------------------------------------------------------

// DomainStatus es el estado de verificación/aprovisionamiento TLS de un dominio.
//
//nolint:revive // el "stutter" es un cruce de sentidos: paquete domain (DDD) y entidad "dominio personalizado" del PaaS.
type DomainStatus string

const (
	DomainPending  DomainStatus = "pending"  // esperando verificación DNS
	DomainVerified DomainStatus = "verified" // DNS verificado, emitiendo TLS
	DomainActive   DomainStatus = "active"   // TLS emitido y sirviendo
	DomainFailed   DomainStatus = "failed"
)

// Domain es un dominio personalizado asociado a un proyecto (CNAME → ingress).
type Domain struct {
	ID          uuid.UUID    `json:"id"`
	Tenant      string       `json:"-"`
	ProjectID   uuid.UUID    `json:"project_id"`
	Hostname    string       `json:"hostname"`
	Status      DomainStatus `json:"status"`
	VerifyToken string       `json:"verify_token,omitempty"` // TXT para verificación
	IsPrimary   bool         `json:"is_primary"`
	CreatedAt   time.Time    `json:"created_at"`
	VerifiedAt  *time.Time   `json:"verified_at,omitempty"`
}

// hostnameRe valida un FQDN básico.
var hostnameRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)

// ValidHostname valida un nombre de host (FQDN).
func ValidHostname(h string) bool {
	h = strings.ToLower(strings.TrimSpace(h))
	return len(h) <= 253 && hostnameRe.MatchString(h)
}

// ---------------------------------------------------------------------------
// Variables de entorno / secretos
// ---------------------------------------------------------------------------

// EnvScope distingue dónde aplica una variable (todos, prod, o previews).
type EnvScope string

const (
	ScopeAll        EnvScope = "all"
	ScopeProduction EnvScope = "production"
	ScopePreview    EnvScope = "preview"
)

// ValidScope valida un EnvScope.
func ValidScope(s EnvScope) bool {
	return s == ScopeAll || s == ScopeProduction || s == ScopePreview
}

// EnvVar es una variable de entorno del proyecto. Si IsSecret, el valor se
// cifra en reposo y nunca se devuelve en claro por la API.
type EnvVar struct {
	ID        uuid.UUID `json:"id"`
	Tenant    string    `json:"-"`
	ProjectID uuid.UUID `json:"project_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value,omitempty"` // vacío en respuestas si IsSecret
	IsSecret  bool      `json:"is_secret"`
	Scope     EnvScope  `json:"scope"`
	UpdatedAt time.Time `json:"updated_at"`
}

// envKeyRe valida una clave de variable de entorno (estilo POSIX).
var envKeyRe = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// ValidEnvKey valida una clave de variable de entorno.
func ValidEnvKey(k string) bool { return envKeyRe.MatchString(k) }

// ---------------------------------------------------------------------------
// Servicios gestionados (Sytue Postgres, Storage, Functions, Cron, Auth)
// ---------------------------------------------------------------------------

// ServiceKind es el tipo de servicio gestionado aprovisionado para un proyecto.
type ServiceKind string

const (
	// ServicePostgres aprovisiona una base Sytue Postgres + PostgREST (API REST
	// automática), con pooling, backups y branching.
	ServicePostgres ServiceKind = "postgres"
	// ServiceStorage aprovisiona un bucket S3 dedicado (Object Storage).
	ServiceStorage ServiceKind = "storage"
	// ServiceFunctions aprovisiona el runtime serverless de funciones.
	ServiceFunctions ServiceKind = "functions"
	// ServiceCron aprovisiona un job programado (batchv1.CronJob por servicio).
	ServiceCron ServiceKind = "cron"
	// ServiceJob aprovisiona un job puntual run-to-completion (batchv1.Job).
	ServiceJob ServiceKind = "job"
	// ServiceWorker aprovisiona un proceso PERMANENTE sin puerto: el consumidor
	// de una cola, el que procesa vídeo, el que manda correos en segundo plano.
	// Es el «worker component» de DigitalOcean App Platform.
	//
	// No es una función (no sirve HTTP ni tiene dirección), ni un cron (no
	// termina), ni un job (no corre una vez): sin este tipo, la única forma de
	// tener un consumidor era disfrazarlo de función y dejarle un puerto abierto
	// que nadie usa.
	ServiceWorker ServiceKind = "worker"
	// ServiceAuth aprovisiona auth gestionada (realm de Sytue ID por proyecto).
	ServiceAuth ServiceKind = "auth"
	// ServiceApp levanta una app open-source del catálogo (ADR 0029): caché,
	// buscador, cola… La imagen NUNCA viene del cliente, viene del catálogo.
	ServiceApp ServiceKind = "app"
)

// ServiceRunsCommand indica si el servicio ejecuta un comando del cliente con
// las credenciales del proyecto (functions, cron, job, worker). Crearlo en
// producción es desplegar (CLOUD-04).
func ServiceRunsCommand(k ServiceKind) bool {
	switch k {
	case ServiceFunctions, ServiceCron, ServiceJob, ServiceWorker:
		return true
	default:
		return false
	}
}

// ValidServiceKind valida un tipo de servicio gestionado.
func ValidServiceKind(k ServiceKind) bool {
	switch k {
	case ServicePostgres, ServiceStorage, ServiceFunctions, ServiceCron, ServiceJob,
		ServiceWorker, ServiceAuth, ServiceApp:
		return true
	default:
		return false
	}
}

// DefaultBackupSchedule es cuándo se hace la copia diaria de Sytue Postgres si el
// cliente no elige otra cosa: las 3 de la mañana, que es cuando menos molesta a
// una base de datos con tráfico de oficina.
const DefaultBackupSchedule = "0 3 * * *"

// ValidCronSchedule hace una validación superficial de una expresión cron: 5
// campos separados por espacios, o una macro (@daily, @hourly, @every 1h…). No
// es un parser completo —el API server de Kubernetes valida el schedule al
// admitir el CronJob—; solo rechaza entradas obviamente malformadas antes de
// persistir el servicio.
func ValidCronSchedule(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "@") {
		switch s {
		case "@hourly", "@daily", "@midnight", "@weekly", "@monthly", "@yearly", "@annually":
			return true
		}
		return strings.HasPrefix(s, "@every ") && strings.TrimSpace(s[len("@every "):]) != ""
	}
	return len(strings.Fields(s)) == 5
}

// ValidateImageRef valida una referencia OCI a desplegar sin build: la imagen
// BYO de un deployment (ya construida en el registry) y la imagen de un
// cron/job gestionado. Exige tag o digest y que el REPOSITORIO (host+path sin
// tag/digest) sea EXACTAMENTE <registry>/<slug> del proyecto que despliega —el
// mismo repo donde el builder publica (builder.imageDestination)—. No basta con
// anclar el host: todos los tenants comparten el namespace <registry> y el pull
// no está gateado por-namespace, así que permitir cualquier repo bajo el host
// dejaría a un tenant desplegar/ejecutar la imagen de OTRO. Trust boundary de
// aislamiento + residencia UE (el clúster solo tiene pull creds de este registry).
// ponytail: exige el slug exacto del proyecto; si algún día los tenants pushean
// repos arbitrarios, pasar a un prefijo por-tenant en vez del slug.
func ValidateImageRef(ref, registry, slug string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("%w: image_ref vacío", ErrInvalidInput)
	}
	// Tag (última componente con ':') o digest ('@sha256:…').
	if !strings.Contains(ref, "@") && !strings.Contains(lastPathSegment(ref), ":") {
		return fmt.Errorf("%w: image_ref requiere tag o digest", ErrInvalidInput)
	}
	if strings.TrimSpace(registry) == "" || slug == "" {
		return fmt.Errorf("%w: registry de plataforma no configurado", ErrInvalidInput)
	}
	want := strings.TrimRight(registry, "/") + "/" + slug
	if imageRepo(ref) != want {
		return fmt.Errorf("%w: image_ref debe residir en el repositorio del proyecto (%s)", ErrInvalidInput, want)
	}
	return nil
}

// imageRepo devuelve el repositorio (host+path sin tag ni digest) de una
// referencia OCI: quita el digest ('@…') y el tag (':' tras el último '/', para
// no confundir un puerto en el host con un tag).
func imageRepo(ref string) string {
	if i := strings.IndexByte(ref, '@'); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndexByte(ref, ':'); i > strings.LastIndexByte(ref, '/') {
		ref = ref[:i]
	}
	return ref
}

func lastPathSegment(s string) string {
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// ServiceStatus es el estado de aprovisionamiento del servicio gestionado.
type ServiceStatus string

const (
	ServiceProvisioning ServiceStatus = "provisioning"
	ServiceReady        ServiceStatus = "ready"
	ServiceFailed       ServiceStatus = "failed"
	ServiceDeleting     ServiceStatus = "deleting"
)

// ManagedService es una instancia de servicio gestionado de un proyecto. La
// configuración pública vive en Config; las credenciales sensibles (DSN,
// claves S3, API keys) se guardan cifradas y se inyectan en los deployments,
// nunca se exponen en claro por la API.
type ManagedService struct {
	ID        uuid.UUID         `json:"id"`
	Tenant    string            `json:"-"`
	ProjectID uuid.UUID         `json:"project_id"`
	Kind      ServiceKind       `json:"kind"`
	Name      string            `json:"name"`
	Status    ServiceStatus     `json:"status"`
	Config    map[string]string `json:"config,omitempty"` // no sensible (host, db, bucket, rest_url)
	// Target dice a qué despliegues entrega sus credenciales: 'production' (lo
	// normal) o 'preview'. Una vista previa solo recibe las de los servicios
	// creados para vistas previas; nunca las de producción.
	Target    DeployTarget `json:"target"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Uso / facturación
// ---------------------------------------------------------------------------

// UsageRecord es un dato de uso agregado de una organización en un periodo
// (para facturación: minutos de build, GB-hora de cómputo, GB de egress, etc.).
type UsageRecord struct {
	ID         uuid.UUID  `json:"id"`
	Tenant     string     `json:"-"`
	ProjectID  *uuid.UUID `json:"project_id,omitempty"`
	Metric     string     `json:"metric"` // build_minutes|compute_gb_hours|egress_gb|storage_gb|requests
	Quantity   float64    `json:"quantity"`
	Unit       string     `json:"unit"`
	PeriodFrom time.Time  `json:"period_from"`
	PeriodTo   time.Time  `json:"period_to"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Webhooks git
// ---------------------------------------------------------------------------

// GitEvent es un evento normalizado de un webhook git (push / PR).
type GitEvent struct {
	Provider  GitProvider
	RepoURL   string
	Ref       string // refs/heads/main o nombre de rama
	CommitSHA string
	// CommitMessage es el asunto del commit (o el título del PR/MR, que es lo que
	// describe una preview). Sin sanear todavía: lo hace Deploy antes de guardarlo.
	CommitMessage string
	// PRNumber > 0 indica un evento de pull request (crea/actualiza preview).
	PRNumber int
	// Action describe la acción del PR (opened|synchronize|closed) si aplica.
	Action string
	// FromFork: el PR viene de otro repositorio (un fork). Lo fija el parser con
	// lo que firma la forja; un PR así no genera vista previa (ver
	// service.HandleGitEvent).
	FromFork bool
	Pusher   string
	Deleted  bool // push de borrado de rama (retira previews)
	// ChangedPaths son las rutas tocadas por el push, tal y como las manda el
	// proveedor. VACÍA no significa «no cambió nada»: significa que no lo sabemos
	// (un proveedor que no las manda, un push de más commits de los que caben en
	// el payload). Ver TouchesDir.
	ChangedPaths []string
}

// TouchesDir indica si el push afecta al subdirectorio `dir` del repositorio.
//
// Es lo que evita que, en un monorepo, tocar la documentación recompile los seis
// proyectos que viven en él: minutos de compilación cobrados para producir la
// misma imagen. Vercel lo llama «Ignored Build Step».
//
// **Fail-open a propósito**: sin `dir` configurado (el proyecto es el repo
// entero) o sin lista de rutas, devuelve true y se despliega. Equivocarse hacia
// «compila de más» cuesta un build; equivocarse hacia «no compiles» significa que
// un cambio real no llega a producción y nadie se entera, que es mucho peor.
func (e GitEvent) TouchesDir(dir string) bool {
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	if dir == "" || len(e.ChangedPaths) == 0 {
		return true
	}
	prefijo := dir + "/"
	for _, p := range e.ChangedPaths {
		p = strings.TrimPrefix(strings.TrimSpace(p), "/")
		// El propio directorio también cuenta: un fichero llamado igual que la
		// carpeta es raro, pero `dir` exacto aparece si el proveedor lista rutas
		// de directorio.
		if p == dir || strings.HasPrefix(p, prefijo) {
			return true
		}
	}
	return false
}

// IsPullRequest indica si el evento es de un PR (preview por PR).
func (e GitEvent) IsPullRequest() bool { return e.PRNumber > 0 }

// ---------------------------------------------------------------------------
// Monitorización de uptime + alertas (Wave 2)
// ---------------------------------------------------------------------------

// Límites del intervalo de sondeo (segundos). Cota inferior para no martillear
// el objetivo ni el prober; superior = 1 día. Coinciden con el CHECK de la
// migración 0010.
const (
	UptimeMinInterval = 30
	UptimeMaxInterval = 86400
)

// Estados del último sondeo de un uptime check. "" = aún sin sondear (baseline:
// no genera transición). Coinciden con el CHECK de la migración 0010.
const (
	UptimeStatusUnknown = ""
	UptimeStatusUp      = "up"
	UptimeStatusDown    = "down"
)

// UptimeCheck es una sonda HTTP periódica de disponibilidad de una URL pública
// (paridad DigitalOcean/Coolify). El prober guarda solo el último estado.
type UptimeCheck struct {
	ID              uuid.UUID  `json:"id"`
	Tenant          string     `json:"-"`
	ProjectID       uuid.UUID  `json:"project_id"`
	Name            string     `json:"name"`
	TargetURL       string     `json:"target_url"`
	IntervalSeconds int        `json:"interval_seconds"`
	Enabled         bool       `json:"enabled"`
	NextRunAt       time.Time  `json:"next_run_at"`
	LastStatus      string     `json:"last_status"`
	LastLatencyMS   int        `json:"last_latency_ms"`
	LastCheckedAt   *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// ValidateUptimeTarget valida la URL objetivo de un uptime check (anti-SSRF,
// CRÍTICO): SOLO https públicas. Rechaza esquemas peligrosos y todo destino
// interno por HOST (loopback, RFC1918, link-local/metadata, nombres del
// clúster) reusando isBlockedHost — un tenant NO debe sondear recursos internos
// vía el prober. El prober añade además el guard en el dial (post-resolución
// DNS) contra rebinding. Más estricta que ValidateWebhookURL (que admite http).
func ValidateUptimeTarget(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%w: target_url vacío", ErrInvalidInput)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: target_url mal formada", ErrInvalidInput)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: target_url debe usar https", ErrInvalidInput)
	}
	host := u.Hostname()
	if host == "" || isBlockedHost(host) {
		return fmt.Errorf("%w: host de target_url no permitido (interno/no público)", ErrInvalidInput)
	}
	return nil
}

// NormalizeInterval acota el intervalo de sondeo al rango permitido (default 60).
func NormalizeInterval(sec int) int {
	switch {
	case sec <= 0:
		return 60
	case sec < UptimeMinInterval:
		return UptimeMinInterval
	case sec > UptimeMaxInterval:
		return UptimeMaxInterval
	default:
		return sec
	}
}

// AlertPolicy enruta un evento del control plane a un canal (webhook saliente).
// ProjectID nil = alcance organización (casa cualquier proyecto). El consumer
// (patrón DeployNotifier) resuelve las policies que casan y envía por webhook.
type AlertPolicy struct {
	ID         uuid.UUID  `json:"id"`
	Tenant     string     `json:"-"`
	ProjectID  *uuid.UUID `json:"project_id,omitempty"`
	Event      string     `json:"event"`
	ChannelURL string     `json:"channel_url"`
	Enabled    bool       `json:"enabled"`
	Confirmed  bool       `json:"confirmed"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Eventos válidos de una alert_policy (forma corta; el consumer mapea el tipo de
// evento del bus a estas). Coinciden con el CHECK de la migración 0010.
//
// Las alertas por MÉTRICA estuvieron deferidas «hasta que exista el stack de
// métricas», y para cuando alguien volvió a mirar ya existía: resource_samples
// (0026) guarda una muestra cada 15 minutos y el motor de rightsizing calcula
// sobre ella el p95 de memoria y de CPU. La señal estaba medida y nadie la
// escuchaba. Queda pendiente `error_rate.high`, que sí necesita algo que no hay:
// contar respuestas por código en el borde.
const (
	AlertEventDeployFailed = "deploy.failed"
	AlertEventDeployReady  = "deploy.ready"
	AlertEventUptimeDown   = "uptime.down"
	AlertEventUptimeUp     = "uptime.up"
	// AlertEventSpendThreshold avisa al cruzar un umbral del límite de gasto
	// (ver domain/spendalert.go). Es el aviso PREVIO al tope duro: cuando se
	// alcanza el límite, Deploy ya rechaza con ErrSpendLimitExceeded.
	AlertEventSpendThreshold = "billing.spend_threshold"
	// AlertEventSizingAdvice avisa de que el tamaño del proyecto dejó de ser el
	// correcto: sobra (y se puede ahorrar) o se queda corto (y va a caerse). Es lo
	// que hace PROACTIVA la recomendación de domain/rightsizing.go, que hasta
	// entonces solo se veía entrando al panel del proyecto.
	AlertEventSizingAdvice = "sizing.advice"
	// AlertEventCPUHigh / AlertEventMemoryHigh avisan de que el consumo lleva rato
	// pegado al techo del tamaño contratado. Es la alerta que llega ANTES del
	// problema: el aviso de tamaño (arriba) propone cambiar de plan y puede
	// esperar; esto dice «esto se cae hoy».
	AlertEventCPUHigh    = "metric.cpu_high"
	AlertEventMemoryHigh = "metric.memory_high"
)

// MetricAlertPercent es el porcentaje del límite a partir del cual se avisa.
//
// ponytail: umbral FIJO y no configurable por cliente. Un campo por proyecto
// exigiría columna, validación, formulario y una decisión («¿por recurso o
// común?») para un valor que casi nadie toca; el 90 % sostenido es donde empiezan
// los reinicios por memoria y el estrangulamiento de CPU. Si alguien pide
// ajustarlo: columna en projects con este valor por defecto y leerlo aquí.
const MetricAlertPercent = 90

// ValidAlertEvent indica si e es un evento de alerta soportado.
func ValidAlertEvent(e string) bool {
	switch e {
	case AlertEventDeployFailed, AlertEventDeployReady, AlertEventUptimeDown, AlertEventUptimeUp,
		AlertEventSpendThreshold, AlertEventSizingAdvice, AlertEventCPUHigh, AlertEventMemoryHigh:
		return true
	default:
		return false
	}
}

// Topes de cola por organización y por proyecto (CLOUD-06).
const (
	// MaxActiveBuildsPerOrg: builds en cola o en curso de una organización.
	MaxActiveBuildsPerOrg = 5
	// MaxLivePreviewsPerProject: pull requests con preview viva en un proyecto.
	MaxLivePreviewsPerProject = 10
)
