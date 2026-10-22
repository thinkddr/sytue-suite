// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga configuración tipada desde el entorno del proceso.
//
// Reglas (docs/CONVENTIONS.md §13):
//   - 100 % a partir de variables de entorno (12-factor). Nunca secretos en git.
//   - Todos los servicios Go comparten estas primitivas de carga para mantener
//     un comportamiento idéntico (prefijos, defaults, validación).
//
// No usa librerías de terceros: solo stdlib, para que sea reutilizable por
// cualquier servicio sin acoplar dependencias.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment representa el entorno lógico de ejecución.
type Environment string

const (
	EnvDev     Environment = "dev"
	EnvStaging Environment = "staging"
	EnvProd    Environment = "prod"
)

// Frontera de confianza del navegador, compartida por los API que aceptan
// peticiones con credenciales entre orígenes, por el `form-action`/`img-src` de
// la CSP del proveedor de identidad y por el handshake de los WebSocket.
//
// Cerrada a propósito: bajo la zona sytue.eu también viven *.apps.sytue.eu y
// *.preview.sytue.eu, donde corre código de CLIENTES, y no se confía en un
// origen por colgar del dominio.
//
// Las listas de producción y de desarrollo están SEPARADAS, y no unidas en una
// sola constante como antes: aquello mandaba en producción una allowlist con 17
// orígenes `*.sytue.localhost` dentro —incluido el `form-action` de la página
// que teclea contraseñas—. Se combinan en FirstPartyWebOrigins según el entorno,
// que es el mismo criterio con el que ya se decide el resto de lo que cambia
// entre prod y dev.
//
// `portal.sytue.eu` NO está: el portal se publica en `one.sytue.eu`
// (deploy/helm/sytue-one/values.yaml, `hosts.portal: one`) y ese nombre es
// NXDOMAIN, así que era una entrada colgada de la allowlist.
const firstPartyWebOriginsProdCSV = "https://sytue.eu,https://www.sytue.eu,https://one.sytue.eu,https://admin.sytue.eu,https://id.sytue.eu,https://hub.sytue.eu,https://mail.sytue.eu,https://drive.sytue.eu,https://docus.sytue.eu,https://calcus.sytue.eu,https://diapos.sytue.eu,https://certipdf.sytue.eu,https://calendar.sytue.eu,https://contacts.sytue.eu,https://notes.sytue.eu,https://photos.sytue.eu,https://erp.sytue.eu,https://rooms.sytue.eu,https://cast.sytue.eu,https://pages.sytue.eu,https://web.sytue.eu,https://bases.sytue.eu,https://account.sytue.eu,https://cloud.sytue.eu,https://integrations.sytue.eu"

const firstPartyWebOriginsDevCSV = "https://sytue.localhost,https://www.sytue.localhost,https://one.sytue.localhost,https://admin.sytue.localhost,https://console.sytue.localhost,https://id.sytue.localhost,https://hub.sytue.localhost,https://mail.sytue.localhost,https://drive.sytue.localhost,https://docus.sytue.localhost,https://calcus.sytue.localhost,https://diapos.sytue.localhost,https://certipdf.sytue.localhost,https://calendar.sytue.localhost,https://contacts.sytue.localhost,https://notes.sytue.localhost,https://photos.sytue.localhost,https://erp.sytue.localhost,https://rooms.sytue.localhost,https://cast.sytue.localhost,https://pages.sytue.localhost,https://web.sytue.localhost,https://bases.sytue.localhost,https://account.sytue.localhost,https://cloud.sytue.localhost,https://integrations.sytue.localhost"

// La forja y la consola de plataforma son de Sytue pero quedan FUERA de la
// allowlist de CORS a propósito: no son API que reciban peticiones con
// credenciales entre orígenes, son destinos a los que el IdP manda el navegador
// (la cadena de redirección del login y el barrido de cookies del cierre de
// sesión, logout.go). Solo las necesita la CSP del IdP.
//
// `corporate.sytue.eu` es el HOST DE ORGANIZACIÓN de quien opera la plataforma:
// la suite entera vive ahí bajo el prefijo de cada aplicación
// (`corporate.sytue.eu/mail`, `/one`…) y sus diez clients registran ahí su
// callback. El barrido de cierre pide una imagen a cada uno, así que sin este
// origen en `img-src` el navegador BLOQUEA las diez peticiones y la sesión
// sobrevive al «salir» —en silencio, porque una imagen bloqueada por CSP no
// rompe nada visible—. Va nombrado y no como `https://*.sytue.eu` por lo que
// explica el comentario de abajo: en CSP ese comodín casa cualquier número de
// etiquetas y en `form-action` autorizaría a mandar la contraseña a un host de
// clientes.
//
// Solo en producción: en dev no hay host de organización, y las migraciones no
// registran ningún `corporate.sytue.localhost`.
//
// `gio.sytue.eu` y `sytue.com` estuvieron aquí mientras existió el frente en Gio
// (ADR 0045): el IdP mandaba el navegador allí y sin ellos en `form-action` Chrome
// cortaba el login. Salieron con la baja de su cliente, `sytue-gio` (migración 0096).
const sytueOperatedExtraProdCSV = "https://git.sytue.eu,https://platform.sytue.eu,https://corporate.sytue.eu"

const sytueOperatedExtraDevCSV = "https://git.sytue.localhost,https://platform.sytue.localhost"

// FirstPartyWebOrigins devuelve los orígenes de primera parte del entorno dado.
// En prod, solo los de producción.
func FirstPartyWebOrigins(env Environment) string {
	if env.IsProd() {
		return firstPartyWebOriginsProdCSV
	}
	return firstPartyWebOriginsProdCSV + "," + firstPartyWebOriginsDevCSV
}

// SytueOperatedWebOrigins añade a los anteriores la forja y la consola de
// plataforma. Existe para que la CSP del IdP pueda NOMBRAR orígenes en vez de
// escribir `https://*.sytue.eu`: la CSP de los navegadores casa ese comodín con
// CUALQUIER número de etiquetas, así que allí también cubriría *.apps.sytue.eu
// y *.preview.sytue.eu. (El comodín de CORS de `httpx` sí está acotado a una
// etiqueta y por eso ahí sí se usa; son dos mecanismos distintos.)
func SytueOperatedWebOrigins(env Environment) string {
	if env.IsProd() {
		return firstPartyWebOriginsProdCSV + "," + sytueOperatedExtraProdCSV
	}
	return FirstPartyWebOrigins(env) + "," + sytueOperatedExtraProdCSV + "," + sytueOperatedExtraDevCSV
}

// FirstPartyCORSOrigins es la allowlist de CORS, y NO la misma que la de CSP.
//
// Añade `https://*.<apex>` a los nombres de siempre para cubrir el dominio de
// cada ORGANIZACIÓN —`<slug>.sytue.eu`, que es donde corre la suite después del
// login—. Sin él, en producción las notificaciones, la presencia y el conmutador
// de aplicaciones fallaban contra el Hub desde el navegador: los tres degradan
// en silencio, así que se veía en la consola de cualquier sesión real y en
// ningún gate.
//
// POR QUÉ NO VALE METERLO EN `firstPartyWebOriginsProdCSV`. Esa constante
// alimenta TAMBIÉN la CSP del IdP, y el comodín de CSP lo casan los navegadores
// con cualquier número de etiquetas: en `form-action` autorizaría a enviar el
// formulario de la contraseña a `algo.apps.sytue.eu`, que es donde corre código
// de clientes. Hay un test que lo prohíbe expresamente
// (services/id/internal/http/csp_origenes_test.go) y fue el que paró este cambio
// cuando se intentó por la vía corta.
//
// El comodín de CORS de `httpx.originAllowed` sí está acotado a UNA etiqueta,
// así que aquí `algo.apps.sytue.eu` queda fuera. Son dos mecanismos con la misma
// sintaxis y distinta semántica, y por eso son dos listas.
func FirstPartyCORSOrigins(env Environment) string {
	if env.IsProd() {
		return "https://*.sytue.eu," + firstPartyWebOriginsProdCSV
	}
	return "https://*.sytue.eu,https://*.sytue.localhost," + FirstPartyWebOrigins(env)
}

// FirstPartyWebSocketHosts es la misma lista en el formato host-only que espera
// OriginPatterns de nhooyr/websocket. Se DERIVA de la anterior en vez de
// repetirla: eran dos literales distintos con los mismos 32 nombres y nada
// impedía que se separaran.
func FirstPartyWebSocketHosts(env Environment) string {
	// Sin comodín de zona: `*` en filepath.Match casa puntos, así que `*.sytue.eu`
	// abriría el socket a `x.apps.sytue.eu` y `pr-42.preview.sytue.eu`, donde corre
	// código de clientes. El host de la organización (`corporate.sytue.eu`) lo
	// cubre httpx.WSOriginPatterns con el Host de la propia petición.
	return strings.ReplaceAll(FirstPartyWebOrigins(env), "https://", "")
}

// firstPartyAudiences son los client_id de la suite, que es lo que Sytue ID pone
// en el `aud` de los tokens. Sale de `clientId:` en
// deploy/helm/charts/sytue-frontend/values.yaml más las audiencias de servicio.
//
// ESTABA COPIADA EN CADA SERVICIO y por eso divergió: Sytue Fotos se añadió
// después y su `sytue-syphotos` no entró en ninguna lista, así que TODOS los
// servicios contestaban 401 a la app de Fotos. El síntoma no se parece a una
// audiencia mal listada: la galería sale vacía, la shell no trae notificaciones
// ni presencia y la consola del navegador solo enseña «401». Medido en
// producción el 2026-09-10.
var firstPartyAudiences = []string{
	"sytue-one", "sytue-mail", "sytue-drive", "sytue-docs", "sytue-calendar",
	"sytue-contacts", "sytue-notes", "sytue-syphotos", "sytue-syerp", "sytue-rooms", "sytue-chat",
	// Sytue Cast: vídeo y directo. Entra aquí ANTES de que exista su cliente en el
	// chart, para que el día que se publique no repita el 401 de Fotos.
	"sytue-cast",
	// Sytue Web: el constructor de webs, con la misma previsión. Su origen es
	// `web.<dominio>` (la app; `pages.<dominio>` fue su host hasta el 2026-10-04 y
	// sigue en la lista mientras sirva o redirija); las webs que publica (`*.sites.<dominio>` y los
	// dominios de los clientes) son contenido de terceros y no entran en ninguna lista.
	"sytue-pages",
	// La ofimática con una app por producto (2026-10-12): cada BFF tiene su client y
	// llama al servicio de documentos, al Hub y a Drive con un token de esa audiencia.
	"sytue-docus", "sytue-calcus", "sytue-diapos", "sytue-certipdf",
	// Sytue Bases: tablas relacionales (ADR 0068). Su BFF habla con el Hub (campana,
	// presencia, buscador) con un token cuya audiencia es la suya.
	"sytue-bases",
	"sytue-meet", "sytue-cloud", "sytue-console", "sytue-platform", "sytue-hub",
	// «Mi cuenta» (id.<dominio>/account). Faltaba y su campana, su presencia y su
	// buscador recibían 401 del Hub: el mismo síntoma de arriba, medido el 2026-09-25.
	"sytue-account",
	// El shell de escritorio nativo (Go/Fyne): sus access tokens llevan
	// aud=sytue-desktop y el Hub —entre otros— los acepta por esta lista.
	"sytue-desktop",
	// Las apps móviles (migración 0071): un solo client público para todos los
	// binarios, cada uno con su esquema de vuelta.
	"sytue-mobile",
}

// FirstPartyAudiences devuelve las audiencias aceptadas por un servicio de la
// suite: las de primera parte más las suyas propias, si tiene.
//
// Al añadir una app a la suite, su client_id se pone AQUÍ y en ningún otro
// sitio. Es el valor por defecto de OIDC_ACCEPTED_AUDIENCES; la variable de
// entorno sigue mandando sobre él.
func FirstPartyAudiences(extra ...string) string {
	out := make([]string, 0, len(firstPartyAudiences)+len(extra))
	out = append(out, firstPartyAudiences...)
	out = append(out, extra...)
	return strings.Join(out, ",")
}

// IsProd indica si el entorno es producción.
func (e Environment) IsProd() bool { return e == EnvProd }

// Valid valida que el entorno sea uno de los permitidos.
func (e Environment) Valid() bool {
	switch e {
	case EnvDev, EnvStaging, EnvProd:
		return true
	default:
		return false
	}
}

// Base contiene la configuración común a TODOS los servicios Sytue One.
// Cada servicio embebe Base y añade sus campos específicos.
type Base struct {
	// ServiceName es el nombre lógico del servicio (p. ej. "sytue-id").
	ServiceName string
	// Env es el entorno lógico (dev|staging|prod).
	Env Environment
	// HTTPAddr es la dirección de escucha del servidor HTTP (":8081").
	HTTPAddr string
	// LogLevel: debug|info|warn|error.
	LogLevel string
	// LogFormat: json|text.
	LogFormat string
	// ShutdownTimeout es el plazo máximo para apagado graceful.
	ShutdownTimeout time.Duration
	// Version es la versión/commit del binario (inyectada en build).
	Version string
}

// BuildVersion es la versión con la que se construyó la imagen que corre.
//
// Sale de `SYTUE_BUILD_VERSION`, sin prefijo de servicio a propósito: es un dato
// del build, idéntico para los once binarios de una misma release, y darle un
// nombre por servicio obligaría a acordarse de once sitios cada vez. El
// Dockerfile de cada uno la hornea como ENV desde su `--build-arg VERSION`.
//
// No va en ldflags porque cambiaría el binario en cada build y rompería su
// reproducibilidad byte a byte, que es la razón por la que ya se inyectaba por
// entorno. Sigue mandando `<PREFIJO>_VERSION` si alguien la pone: el orden es
// configuración explícita, luego lo horneado, luego "dev".
//
// "dev" y no vacío: una imagen construida a mano tiene que poder decirlo.
func BuildVersion() string {
	if v := os.Getenv("SYTUE_BUILD_VERSION"); v != "" {
		return v
	}
	return "dev"
}

// LoadBase carga la configuración común desde el entorno usando el prefijo
// dado (p. ej. "SYTUE_ID"). El nombre del servicio es obligatorio.
func LoadBase(prefix, serviceName string) (Base, error) {
	l := NewLoader(prefix)
	b := Base{
		ServiceName:     serviceName,
		Env:             Environment(l.GetDefault("ENV", string(EnvDev))),
		HTTPAddr:        l.GetDefault("HTTP_ADDR", ":8080"),
		LogLevel:        l.GetDefault("LOG_LEVEL", "info"),
		LogFormat:       l.GetDefault("LOG_FORMAT", "json"),
		ShutdownTimeout: l.GetDurationDefault("SHUTDOWN_TIMEOUT", 15*time.Second),
		Version:         l.GetDefault("VERSION", BuildVersion()),
	}
	if !b.Env.Valid() {
		l.errs = append(l.errs, fmt.Errorf("ENV inválido: %q (dev|staging|prod)", b.Env))
	}
	return b, l.Err()
}

// Loader acumula errores de lectura para reportarlos todos juntos. Lee
// variables con un prefijo común: Get("FOO") -> "<PREFIX>_FOO".
type Loader struct {
	prefix string
	errs   []error
}

// NewLoader crea un Loader con el prefijo indicado (sin guion bajo final).
func NewLoader(prefix string) *Loader {
	return &Loader{prefix: strings.TrimSuffix(prefix, "_")}
}

func (l *Loader) key(name string) string {
	if l.prefix == "" {
		return name
	}
	return l.prefix + "_" + name
}

// Lookup devuelve el valor crudo y si la variable estaba definida.
func (l *Loader) Lookup(name string) (string, bool) {
	return os.LookupEnv(l.key(name))
}

// Get devuelve el valor o cadena vacía si no está definido.
func (l *Loader) Get(name string) string {
	v, _ := l.Lookup(name)
	return v
}

// GetDefault devuelve el valor o el default si no está definido/está vacío.
func (l *Loader) GetDefault(name, def string) string {
	if v, ok := l.Lookup(name); ok && v != "" {
		return v
	}
	return def
}

// Require devuelve el valor y registra un error si falta o está vacío.
func (l *Loader) Require(name string) string {
	v, ok := l.Lookup(name)
	if !ok || v == "" {
		l.errs = append(l.errs, fmt.Errorf("falta variable de entorno obligatoria %s", l.key(name)))
		return ""
	}
	return v
}

// DevHubInternalSecret es el secreto de la puerta interna del Hub en desarrollo. Está
// escrito aquí, así que es público: fuera de dev no vale.
const DevHubInternalSecret = "dev-hub-internal-secret" //nolint:gosec // G101: es el de desarrollo, público a propósito; fuera de dev se rechaza

// HubInternalSecret es el secreto compartido de `/internal/notifications` del Hub
// (HUB_INTERNAL_SECRET), el mismo en el Hub y en cada servicio que avisa. En dev cae
// al de desarrollo; fuera, falta o es el de desarrollo = el servicio no arranca.
//
// Hasta el 2026-09-25 cada servicio caía en silencio a DevHubInternalSecret, ningún
// chart lo daba, y la puerta estaba publicada en hub.sytue.eu: cualquiera podía dejar
// un aviso con su enlace en la campana de cualquiera. Lo da el Secret
// `sytue-hub-internal` que genera el chart.
func (l *Loader) HubInternalSecret(env Environment) string {
	if env == EnvDev {
		return l.GetDefault("HUB_INTERNAL_SECRET", DevHubInternalSecret)
	}
	v := l.Require("HUB_INTERNAL_SECRET")
	if v == DevHubInternalSecret {
		l.errs = append(l.errs, fmt.Errorf("%s es el secreto de desarrollo, que es público", l.key("HUB_INTERNAL_SECRET")))
	}
	return v
}

// DevDocsDriveInternalSecret es el secreto de desarrollo de las dos puertas internas
// entre Sytue Docs y Sytue Drive (la papelera compartida). Público: fuera de dev no vale.
const DevDocsDriveInternalSecret = "dev-docs-drive-internal-secret" //nolint:gosec // G101: es el de desarrollo, público a propósito; fuera de dev se rechaza

// SecretoOpcional es el régimen de un secreto OPCIONAL compartido con otro servicio
// de la suite: en dev, vacío cae al de desarrollo para que los dos extremos casen sin
// configurar nada; fuera de dev, vacío se queda vacío —la puerta no abre y el resto
// del servicio no se entera— y el de desarrollo se RECHAZA: está en el código, así
// que es público, y un secreto público en producción es una puerta abierta que
// parece cerrada. Es el de HubInternalSecret sin la obligación.
func (l *Loader) SecretoOpcional(env Environment, name, dev string) string {
	v := l.Get(name)
	switch {
	case env == EnvDev && v == "":
		return dev
	case env != EnvDev && v == dev:
		l.errs = append(l.errs, fmt.Errorf("%s es el secreto de desarrollo, que es público", l.key(name)))
		return ""
	}
	return v
}

// GetBoolDefault parsea un booleano (1,t,true,yes,on) con default.
func (l *Loader) GetBoolDefault(name string, def bool) bool {
	v, ok := l.Lookup(name)
	if !ok || v == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "t", "true", "yes", "y", "on":
		return true
	case "0", "f", "false", "no", "n", "off":
		return false
	default:
		l.errs = append(l.errs, fmt.Errorf("%s: booleano inválido %q", l.key(name), v))
		return def
	}
}

// GetIntDefault parsea un entero con default.
func (l *Loader) GetIntDefault(name string, def int) int {
	v, ok := l.Lookup(name)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: entero inválido %q", l.key(name), v))
		return def
	}
	return n
}

// GetFloatDefault parsea un decimal con default. Existe porque hay valores que
// NO son enteros y redondearlos cambia lo que hacen: un presupuesto de 1,50 €
// leído como entero es 1 € —un tercio menos— sin que nada avise.
func (l *Loader) GetFloatDefault(name string, def float64) float64 {
	v, ok := l.Lookup(name)
	if !ok || v == "" {
		return def
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: decimal inválido %q", l.key(name), v))
		return def
	}
	return f
}

// GetDurationDefault parsea una duración (formato time.ParseDuration) con default.
func (l *Loader) GetDurationDefault(name string, def time.Duration) time.Duration {
	v, ok := l.Lookup(name)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: duración inválida %q", l.key(name), v))
		return def
	}
	return d
}

// GetCSV devuelve una lista a partir de un valor separado por comas.
func (l *Loader) GetCSV(name string) []string {
	v, ok := l.Lookup(name)
	if !ok || strings.TrimSpace(v) == "" {
		return nil
	}
	return splitCSV(v)
}

// GetCSVDefault es GetCSV con un CSV por defecto cuando la variable no está o está vacía.
func (l *Loader) GetCSVDefault(name, def string) []string {
	if v := l.GetCSV(name); len(v) > 0 {
		return v
	}
	return splitCSV(def)
}

// splitCSV parte un CSV, recorta espacios y descarta entradas vacías.
func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// Err devuelve el conjunto de errores acumulados (nil si no hubo).
func (l *Loader) Err() error {
	return errors.Join(l.errs...)
}

// NormalizeS3Endpoint extrae host:port de un endpoint S3 que puede venir como
// URL («http://seaweedfs:8333») y deduce el flag de TLS de su esquema, porque el
// cliente de MinIO espera el host pelado y el TLS como booleano aparte.
//
// El esquema MANDA sobre el useSSL recibido: si el endpoint dice http://, se
// habla en claro aunque S3_USE_SSL venga a true. Así un useSSL mal heredado del
// chart no pide TLS a un servidor que habla en claro («server gave HTTP response
// to HTTPS client», piloto, 2026-09-09). Sin esquema, useSSL decide.
func NormalizeS3Endpoint(endpoint string, useSSL bool) (string, bool) {
	switch {
	case strings.HasPrefix(endpoint, "https://"):
		return strings.TrimPrefix(endpoint, "https://"), true
	case strings.HasPrefix(endpoint, "http://"):
		return strings.TrimPrefix(endpoint, "http://"), false
	default:
		return endpoint, useSSL
	}
}
