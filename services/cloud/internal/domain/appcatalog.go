// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Apps open-source como servicio: el catálogo, y por qué es un catálogo
// ---------------------------------------------------------------------------
//
// La promesa es levantar una caché, un buscador o una cola junto a tu app y que
// se conecten solos. Lo que NO puede ser es «dame el nombre de una imagen y la
// arranco»: eso convierte el control-plane en un ejecutor de código arbitrario
// de terceros dentro del clúster, y ninguna política de red lo arregla después.
//
// Por eso la imagen NUNCA viene del cliente. Viene de este catálogo, que es
// código: revisado, con licencia comprobada y FIJADO POR DIGEST. El cliente
// elige una entrada por su id; el resto lo pone la plataforma.
//
// Cuatro invariantes, y los cuatro se comprueban aquí en vez de confiar en que
// el catálogo esté bien escrito:
//
//  1. **Digest, no etiqueta.** `valkey:8` apunta hoy a un contenido y mañana a
//     otro. Un `@sha256:…` es el ÚNICO identificador que no puede cambiar bajo
//     nuestros pies, y es lo que permite firmar y auditar lo que corre.
//  2. **Registro propio.** Todas las imágenes se replican a nuestro registro en
//     la UE. Depender de Docker Hub en el camino de aprovisionamiento es
//     depender de su rate limit y de su disponibilidad para que un cliente pueda
//     crear un servicio.
//  3. **Licencia permisiva de verdad.** AGPL queda fuera por la misma razón que
//     en la ADR 0022, y también las licencias «source available» que se anuncian
//     como open source y no lo son (SSPL, BSL, Sustainable Use, Elastic v2):
//     ofrecerlas como servicio gestionado es justo lo que prohíben.
//  4. **Nada de privilegios.** El manifiesto no tiene forma de pedir
//     `privileged`, `hostPath`, `hostNetwork` ni capacidades: no es que se
//     validen, es que no existen como campo.

// AppNeedPostgres es el único requisito que hoy se puede declarar: el Sytue
// Postgres del proyecto. Se enumera —en vez de aceptar cualquier cadena— porque
// cada valor exige código que sepa resolverlo, y un requisito que nadie resuelve
// dejaría la app esperando para siempre a algo que no va a llegar.
const AppNeedPostgres = "postgres"

// AppCategory agrupa el catálogo en la interfaz.
type AppCategory string

const (
	AppCategoryCache  AppCategory = "cache"
	AppCategorySearch AppCategory = "search"
	AppCategoryQueue  AppCategory = "queue"
	AppCategoryCMS    AppCategory = "cms"
	AppCategoryTools  AppCategory = "tools"
	// Las tres siguientes llegan con los productos COMPLETOS —los que se abren en
	// un navegador y los usa una persona, no otra máquina—. Sin ellas todo lo que
	// no fuera caché, cola o buscador caía en «herramientas», y un catálogo donde
	// la mitad de las entradas comparten categoría no está agrupado: está
	// ordenado por casualidad.
	AppCategoryDevTools      AppCategory = "devtools"
	AppCategoryObservability AppCategory = "observability"
	AppCategoryProductivity  AppCategory = "productivity"
)

// AllowedAppLicenses son las licencias que se aceptan en el catálogo.
//
// Es una allowlist a propósito: una denylist deja pasar cualquier licencia nueva
// que alguien se invente, y ese es exactamente el problema —las licencias
// «source available» aparecen más rápido de lo que se actualizaría una lista de
// prohibidas—. SSPL, BUSL, Elastic v2 y Sustainable Use siguen fuera, y ahí sí
// que el motivo es la licencia misma: prohíben exactamente esto, ofrecerlas como
// servicio gestionado.
var AllowedAppLicenses = append([]string{
	"Apache-2.0",
	"BSD-2-Clause",
	"BSD-3-Clause",
	"ISC",
	"MIT",
	"MPL-2.0",
	"PostgreSQL",
}, CopyleftAppLicenses...)

// CopyleftAppLicenses son las licencias con copyleft fuerte que el catálogo
// acepta, y que exigen `Source` (ADR 0032).
//
// # Por qué entran, si la ADR 0022 sacó los motores AGPL de la suite
//
// Porque son dos cosas distintas y la diferencia es jurídica, no de gusto. La
// 0022 retira motores AGPL de DENTRO de Sytue: ahí el código propietario se
// integra con ellos, y esa combinación es lo que el copyleft alcanza. Aquí la
// imagen corre TAL CUAL en el namespace del cliente, sin que nada nuestro se
// enlace con ella: no hay obra derivada, solo agregación.
//
// Y «tal cual» no es una promesa: es lo que ya comprueba validateAppMirror
// —`mirrored_digest` tiene que ser IDÉNTICO a `upstream_digest`—. Una copia con
// un solo byte cambiado no cumple esa igualdad y no entra en el catálogo, así que
// la condición de la que depende todo lo anterior está verificada por una máquina
// y no por quien escribe el JSON.
//
// La obligación que sí queda —AGPL §13 y GPL §6: quien usa el servicio puede
// obtener el fuente— se cumple con `Source`, que la UI enseña junto a la licencia.
var CopyleftAppLicenses = []string{
	"AGPL-3.0-only",
	"AGPL-3.0-or-later",
	"GPL-2.0-only",
	"GPL-2.0-or-later",
	"GPL-3.0-only",
	"GPL-3.0-or-later",
	"LGPL-2.1-or-later",
	"LGPL-3.0-only",
	"LGPL-3.0-or-later",
}

// AppPort es un puerto que el servicio publica DENTRO del namespace del
// proyecto. Nunca sale a internet: quien quiera exponerlo pone su propia app
// delante.
type AppPort struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

// AppEnvExport es una variable que este servicio inyecta en la APP del cliente
// cuando se conectan. Es la mitad que hace real lo de «se integran aunque los
// crees de uno en uno».
//
// `Value` es una plantilla con dos sustituciones y ninguna más: `{host}` (el
// nombre DNS del servicio dentro del namespace) y `{port}`. Deliberadamente no
// es un lenguaje: cuanto más pueda expresar, más cosas puede filtrar.
type AppEnvExport struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// AppIntegration es lo que esta app sabe hacer con OTRA del catálogo cuando las
// dos están levantadas en el mismo proyecto.
//
// Es la mitad que faltaba de «se integran aunque los elijas de uno en uno»:
// `Exports` conecta un servicio con la APP DEL CLIENTE, y esto conecta dos
// servicios ENTRE SÍ —el CMS que usa la caché, el buscador que consume la cola—
// sin que nadie copie una URL de un panel a otro.
//
// `Docs` es obligatorio y es lo que separa una integración de una conjetura: la
// URL de la documentación OFICIAL del proyecto donde se dice que esa variable
// existe y qué significa. Sin ese ancla, «Sytue las conecta solas» acabaría
// siendo Sytue inventando configuración ajena, y el fallo aparecería en el
// arranque de un contenedor de un cliente.
//
// `Env` usa la MISMA plantilla acotada que Exports —`{host}` y `{port}`, nada
// más—, pero referidos a la OTRA app: es su Service y su primer puerto.
type AppIntegration struct {
	// App es el id de la otra entrada del catálogo. Tiene que existir: se
	// comprueba al cargar el catálogo, no al desplegar en casa de un cliente.
	App string `json:"app"`
	// Docs es la página oficial que documenta esta integración (https).
	Docs string `json:"docs"`
	// Env son las variables que se le inyectan a ESTA app apuntando a la otra.
	Env []AppEnvExport `json:"env"`
}

// AppEnvVar es una variable que necesita el PROPIO servicio para arrancar.
//
// `Generate` pide un secreto aleatorio por instalación —una clave maestra, una
// contraseña de administración— en vez de un literal en el catálogo, que sería
// el mismo para todos los clientes.
//
// `Export` la inyecta ADEMÁS en la app del cliente con el mismo nombre. Es la
// única forma de que un secreto generado llegue a quien tiene que usarlo sin
// convertir la plantilla de `exports` en un lenguaje capaz de leer secretos.
//
// `Value` admite UN marcador: `{web_url}`, la dirección pública con la que se
// publica la app. Existe porque los productos completos la necesitan para
// funcionar bien —Gitea la escribe en los enlaces de clonado, Grafana en los
// correos— y sin ella arrancan apuntando a `localhost`: parece que va, y lo que
// se copia no sirve. Se resuelve al aprovisionar, que es cuando se conoce el
// hostname; si la app no se publica, queda en cadena vacía.
type AppEnvVar struct {
	Key      string `json:"key"`
	Value    string `json:"value,omitempty"`
	Generate bool   `json:"generate,omitempty"`
	Export   bool   `json:"export,omitempty"`
}

// AppEnvPlaceholders son los marcadores que admite un valor de `env`, y los
// únicos. Los `{db_*}` solo tienen sentido si la app declara `needs: postgres`,
// y eso se comprueba al cargar el catálogo: usarlos sin declararlo dejaría a la
// app arrancando con la cadena literal `{db_host}` como servidor.
var AppEnvPlaceholders = map[string]bool{
	"{web_url}": true, "{db_host}": true, "{db_port}": true, "{db_name}": true,
	"{db_user}": true, "{db_password}": true, "{db_url}": true,
}

// RenderEnvValue sustituye los marcadores de un valor del catálogo.
//
// Deliberadamente NO es un lenguaje de plantillas, igual que `exports`: cuanto
// más pueda expresar, más cosas puede filtrar del entorno del controlador.
func RenderEnvValue(value string, vars map[string]string) string {
	for k, v := range vars {
		value = strings.ReplaceAll(value, k, v)
	}
	return value
}

// AppConfigFile es un fichero de configuración que el CATÁLOGO aporta a la app.
//
// Existe porque hay software que no se configura con variables: Prometheus, Loki
// o Traefik quieren un YAML, y sin poder dárselo la única opción era dejarlos
// fuera —o peor, ofrecerlos sabiendo que arrancan con la configuración de
// ejemplo y no miden nada—.
//
// El contenido lo escribe el catálogo, NUNCA el cliente: es la misma frontera
// que la imagen. Un fichero de configuración es código que el proceso ejecuta a
// su manera, y aceptarlo del cliente sería volver a abrir la puerta que el
// catálogo cerró (ADR 0029).
//
// Admite los mismos marcadores que `env` —`{web_url}`, `{db_*}`—: una
// configuración que necesita saber su propia URL es lo normal.
type AppConfigFile struct {
	// Path es la ruta ABSOLUTA del fichero dentro del contenedor.
	Path string `json:"path"`
	// Content es su contenido literal.
	Content string `json:"content"`
}

// MaxAppConfigBytes acota lo que puede pesar la configuración de una entrada.
// Un ConfigMap no admite más de 1 MiB en total, y llegar a ese límite se
// descubriría al desplegar en casa de un cliente; 64 KiB por fichero deja sitio
// de sobra para cualquier configuración razonable y avisa mucho antes.
const MaxAppConfigBytes = 64 * 1024

// CatalogApp es una entrada del catálogo de apps open-source.
type CatalogApp struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Category    AppCategory `json:"category"`
	// License es el identificador SPDX. Tiene que estar en AllowedAppLicenses.
	License string `json:"license"`
	// Homepage es la web del proyecto original. Se enseña: quien despliega
	// software de otro tiene derecho a saber de quién es.
	Homepage string `json:"homepage"`
	// Source es el repositorio del código fuente. Obligatorio con copyleft fuerte
	// (AGPL/GPL/LGPL) y opcional en el resto.
	//
	// No es documentación: es cómo se cumple el AGPL §13 —quien interactúa con el
	// servicio por red tiene derecho a su fuente— sin que nadie tenga que acordarse
	// de publicarlo. Va aquí, y no en un texto legal, porque la UI lo enseña al
	// lado de la licencia; y es obligatorio en la validación porque la alternativa
	// era confiar en que quien añade una entrada AGPL lo recuerde.
	Source string `json:"source,omitempty"`
	// Upstream es de DÓNDE se replica, con su etiqueta: es lo que hace auditable
	// el espejo — sin ella nadie puede comprobar qué se copió.
	Upstream string `json:"upstream"`
	// UpstreamDigest es el digest EXACTO que se replica. Se resuelve contra el
	// registro de origen al escribir la entrada, no se adivina.
	//
	// Una copia fiel de un manifiesto (`crane copy`, `skopeo copy`) CONSERVA el
	// digest, así que este valor tiene que ser también el de nuestra imagen. Que
	// no coincidan significa que la copia no fue fiel —recomprimida, una sola
	// arquitectura— y eso hay que verlo, no descubrirlo en producción.
	UpstreamDigest string `json:"upstream_digest"`
	// MirroredDigest es el digest de NUESTRA copia; vacío mientras no se haya
	// replicado. Cuando está, tiene que ser el mismo que UpstreamDigest —una copia
	// fiel lo conserva—, y eso se comprueba.
	//
	// Aquí NO se guarda el registro. La referencia completa la compone
	// ImageRef() con el REGISTRY_URL del despliegue, porque el namespace es
	// distinto en dev y en producción: escribirlo en el catálogo lo dejaba
	// apuntando a un sitio que en producción no existe, y el fallo no aparecía
	// hasta que un cliente intentaba crear el servicio.
	MirroredDigest string `json:"mirrored_digest,omitempty"`
	// Version es la versión legible de lo que hay en ese digest, solo para
	// enseñarla. El digest manda.
	Version string    `json:"version"`
	Ports   []AppPort `json:"ports"`
	// Exports son las variables que se inyectan en la app del cliente.
	Exports []AppEnvExport `json:"exports"`
	// Integrations son las otras apps del catálogo con las que esta se conecta
	// sola cuando conviven en un proyecto.
	Integrations []AppIntegration `json:"integrations,omitempty"`
	// Env es lo que el propio servicio necesita para arrancar.
	Env []AppEnvVar `json:"env,omitempty"`
	// Needs son los servicios GESTIONADOS que la app necesita en el proyecto para
	// poder arrancar. Hoy solo `postgres`.
	//
	// Existe porque el catálogo se quedaba en las apps que guardan su estado en su
	// propio disco: casi todo lo grande —una wiki, una analítica, un gestor de
	// identidad— necesita base de datos, y sin esto la única salida era no
	// ofrecerlo. `Integrations` no vale para esto: conecta apps del catálogo entre
	// sí, y el Postgres gestionado no es una de ellas.
	//
	// Lo que hace el declararlo: el alta EXIGE que el proyecto ya tenga ese
	// servicio —con un mensaje que dice cuál falta, en vez de un contenedor que se
	// reinicia sin explicación— y el aprovisionamiento resuelve los marcadores
	// `{db_*}` de `env` con sus credenciales reales.
	Needs []string `json:"needs,omitempty"`
	// ConfigFiles son ficheros de configuración que aporta el catálogo. Se montan
	// de solo lectura desde un ConfigMap.
	ConfigFiles []AppConfigFile `json:"config_files,omitempty"`
	// DiskGiB es el disco persistente del servicio; 0 = sin estado.
	DiskGiB int `json:"disk_gib"`
	// DataPath es dónde monta ese disco. Obligatorio si DiskGiB > 0: cada imagen
	// guarda sus datos en un sitio distinto y montar en el que no es deja al
	// servicio arrancando en vacío cada vez, sin que nada falle.
	DataPath string `json:"data_path,omitempty"`
	// WritablePaths son directorios que la imagen necesita escribir FUERA de su
	// disco de datos, y que con la raíz de solo lectura no puede.
	//
	// Existe porque es el caso normal, no la excepción: el arranque de RabbitMQ
	// genera su configuración en /etc/rabbitmq. La alternativa —renunciar a
	// readOnlyRootFilesystem— sería pagar el sistema de ficheros entero por un
	// directorio. Se montan como emptyDir: efímeros, y por tanto sin nada que
	// persista entre reinicios que nadie haya declarado.
	WritablePaths []string `json:"writable_paths,omitempty"`
	// RunAsUser es el UID con el que corre. Obligatorio y distinto de 0: el
	// namespace del cliente exige PodSecurity `restricted`, así que una imagen
	// que por defecto corra como root no arrancaría — y descubrirlo en el primer
	// despliegue de un cliente es tarde.
	//
	// # Qué imagen vale, y qué no (el criterio, escrito para no volver a dudarlo)
	//
	// Que el manifiesto de la imagen diga `USER root` NO la descarta: aquí se le
	// fija el UID y el pod monta el disco con `fsGroup` igual, así que el volumen
	// es suyo. Lo que de verdad decide es su ENTRYPOINT:
	//
	//   - Un binario directo (`/opt/typesense-server`, `ntfy`) vale: no hay nada
	//     que quiera cambiar dueños ni escribir fuera de lo declarado.
	//   - Un script (`docker-entrypoint.sh`, `/start.sh`) casi nunca: esos scripts
	//     existen justo para hacer `chown` y bajar privilegios con `su-exec`, y con
	//     un UID no-root fallan en la primera línea. Ahí están Nextcloud, Ghost,
	//     Vaultwarden y Uptime Kuma.
	//
	// Se mira con `crane config <imagen>`, no se supone. Y lo que la imagen
	// escriba fuera de su disco va en WritablePaths o no arranca.
	RunAsUser int64 `json:"run_as_user"`
	// MemoryMiB y CPUMilli son el techo de recursos. Obligatorios: un servicio
	// sin límite se come la cuota del proyecto y tumba la app que venía a ayudar.
	MemoryMiB int `json:"memory_mib"`
	CPUMilli  int `json:"cpu_milli"`
	// HealthPath es la ruta de salud HTTP; vacío = solo sonda de TCP.
	HealthPath string `json:"health_path,omitempty"`
	// WebPort es el puerto por el que la app se publica en internet, con su propio
	// hostname y TLS. 0 —el defecto— significa que NO se publica y solo se alcanza
	// desde dentro del proyecto.
	//
	// Que lo diga el catálogo y no el cliente es la decisión entera. Un CMS sin URL
	// pública no sirve para nada, pero el panel de administración de RabbitMQ
	// publicado —con la contraseña que la plataforma acaba de generar— es una
	// brecha a un clic de distancia, y ese clic lo daría alguien que no sabe que
	// 15672 es un panel. Aquí se revisa una vez, por entrada, y ya no hay
	// interruptor que equivocarse.
	WebPort int `json:"web_port,omitempty"`
}

// Límites del catálogo. No son de rendimiento: son el techo de lo que un
// servicio gestionado puede pedirle a la cuota de un proyecto sin que nadie lo
// revise.
const (
	MaxAppDiskGiB   = 256
	MaxAppMemoryMiB = 16384
	MaxAppCPUMilli  = 8000
)

var (
	appIDRe  = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)
	digestRe = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	// exportKeyRe es más estricta que envKeyRe (domain.go): una variable que se
	// inyecta en la app de un cliente tiene que ser un nombre reconocible, no un
	// `_` suelto.
	exportKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,62}$`)
)

// ValidateCatalogApp comprueba los invariantes de una entrada.
//
// Se ejecuta sobre el catálogo EMBEBIDO, en un test, no sobre entrada de
// usuario: el cliente nunca envía uno de estos. Existe porque el catálogo lo
// escriben personas, y la diferencia entre `valkey:8` y `valkey@sha256:…` no se
// ve al revisar un diff.
func ValidateCatalogApp(a CatalogApp, allowedRegistry string) error {
	if !appIDRe.MatchString(a.ID) {
		return fmt.Errorf("id %q: minúsculas, dígitos y guiones, entre 3 y 32", a.ID)
	}
	if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Description) == "" {
		return fmt.Errorf("%s: nombre y descripción son obligatorios", a.ID)
	}
	switch a.Category {
	case AppCategoryCache, AppCategorySearch, AppCategoryQueue, AppCategoryCMS, AppCategoryTools,
		AppCategoryDevTools, AppCategoryObservability, AppCategoryProductivity:
	default:
		return fmt.Errorf("%s: categoría %q desconocida", a.ID, a.Category)
	}
	if !licenseAllowed(a.License) {
		return fmt.Errorf(
			"%s: licencia %q no permitida (permitidas: %s). Las «source available» —SSPL, BSL, "+
				"Elastic v2, Sustainable Use— prohíben justo esto: ofrecerlas como servicio gestionado",
			a.ID, a.License, strings.Join(AllowedAppLicenses, ", "),
		)
	}
	if !strings.HasPrefix(a.Homepage, "https://") {
		return fmt.Errorf("%s: homepage tiene que ser https", a.ID)
	}
	if err := validateAppSource(a); err != nil {
		return err
	}
	if err := validateAppMirror(a, allowedRegistry); err != nil {
		return err
	}
	if err := validateAppPorts(a); err != nil {
		return err
	}
	if err := validateAppExports(a); err != nil {
		return err
	}
	if err := validateAppIntegrations(a); err != nil {
		return err
	}
	if err := validateAppEnv(a); err != nil {
		return err
	}
	if err := validateAppConfigFiles(a); err != nil {
		return err
	}
	for _, n := range a.Needs {
		if n != AppNeedPostgres {
			return fmt.Errorf(
				"%s: requisito %q desconocido (hoy solo %q). Uno que nadie resuelve dejaría la "+
					"app esperando a algo que no va a llegar", a.ID, n, AppNeedPostgres,
			)
		}
	}
	return validateAppResources(a)
}

// validateAppIntegrations: una integración apunta a OTRA app, cita su documentación
// oficial y declara al menos una variable.
//
// Que `docs` sea obligatorio no es papeleo: es lo que impide que el catálogo
// invente configuración de un proyecto ajeno. Quien añade la entrada tiene que
// haber leído dónde lo dice el fabricante, y quien la revisa puede comprobarlo
// sin desplegar nada.
func validateAppIntegrations(a CatalogApp) error {
	seen := map[string]bool{}
	// claves: variable -> app que la aporta, para cazar dos integraciones que se
	// pisen entre sí.
	claves := map[string]string{}
	for _, in := range a.Integrations {
		switch {
		case !appIDRe.MatchString(in.App):
			return fmt.Errorf("%s: integración con id inválido %q", a.ID, in.App)
		case in.App == a.ID:
			return fmt.Errorf("%s: se integra consigo misma", a.ID)
		case seen[in.App]:
			return fmt.Errorf("%s: integración con %q declarada dos veces", a.ID, in.App)
		case !strings.HasPrefix(in.Docs, "https://"):
			return fmt.Errorf(
				"%s: la integración con %q necesita la URL https de la documentación OFICIAL que la "+
					"describe: sin ella el catálogo estaría inventando la configuración de otro proyecto",
				a.ID, in.App,
			)
		case len(in.Env) == 0:
			return fmt.Errorf("%s: la integración con %q no inyecta ninguna variable", a.ID, in.App)
		}
		seen[in.App] = true
		for _, e := range in.Env {
			if !exportKeyRe.MatchString(e.Key) {
				return fmt.Errorf("%s: integración con %q: %q no es un nombre de variable válido",
					a.ID, in.App, e.Key)
			}
			if err := validExportTemplate(e.Value); err != nil {
				return fmt.Errorf("%s: integración con %q: %s: %w", a.ID, in.App, e.Key, err)
			}
			if err := integrationKeyLibre(a, in, e.Key, claves); err != nil {
				return err
			}
			claves[e.Key] = in.App
		}
	}
	return nil
}

// integrationKeyLibre resuelve las dos colisiones que una integración puede
// provocar, y las resuelve PROHIBIÉNDOLAS en vez de eligiendo una ganadora.
//
//   - Contra un secreto generado: pisarlo con un literal del catálogo dejaría al
//     servicio arrancando con la contraseña de todos los clientes en vez de la
//     suya, y no fallaría nada.
//   - Entre dos integraciones: el valor dependería de qué app se levantó primero
//     —o del orden de un mapa—, así que el mismo proyecto arrancaría distinto en
//     dos despliegues iguales.
//
// Pisar un literal de `env` sí se permite, y es el caso normal: `CACHE=memoria`
// pasa a ser `CACHE=redis` justo cuando hay una caché que usar.
func integrationKeyLibre(a CatalogApp, in AppIntegration, key string, claves map[string]string) error {
	for _, propia := range a.Env {
		if propia.Key == key && propia.Generate {
			return fmt.Errorf(
				"%s: la integración con %q pisa %s, que es un secreto generado por instalación",
				a.ID, in.App, key,
			)
		}
	}
	if otra, dup := claves[key]; dup {
		return fmt.Errorf(
			"%s: %s la declaran las integraciones con %q y con %q: el valor dependería de cuál se "+
				"levantara antes", a.ID, key, otra, in.App,
		)
	}
	return nil
}

// validateAppPorts: al menos uno, en rango, sin repetir y con nombre (el nombre
// lo usa el Service de Kubernetes y una colisión ahí es un fallo de despliegue).
func validateAppPorts(a CatalogApp) error {
	if len(a.Ports) == 0 {
		return fmt.Errorf("%s: hace falta al menos un puerto", a.ID)
	}
	seen := map[int]bool{}
	for _, p := range a.Ports {
		if p.Port < 1 || p.Port > 65535 {
			return fmt.Errorf("%s: puerto %d fuera de rango", a.ID, p.Port)
		}
		if seen[p.Port] {
			return fmt.Errorf("%s: puerto %d repetido", a.ID, p.Port)
		}
		seen[p.Port] = true
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("%s: el puerto %d necesita nombre", a.ID, p.Port)
		}
	}
	// Publicar un puerto que no existe dejaría un Ingress apuntando al vacío: la
	// URL sale en el panel, el TLS se emite y la app devuelve 502.
	if a.WebPort != 0 && !seen[a.WebPort] {
		return fmt.Errorf("%s: web_port %d no está entre los puertos que declara", a.ID, a.WebPort)
	}
	return nil
}

// validateAppExports: nombre de variable reconocible y plantilla acotada.
func validateAppExports(a CatalogApp) error {
	for _, e := range a.Exports {
		if !exportKeyRe.MatchString(e.Key) {
			return fmt.Errorf("%s: %q no es un nombre de variable válido", a.ID, e.Key)
		}
		if err := validExportTemplate(e.Value); err != nil {
			return fmt.Errorf("%s: export %s: %w", a.ID, e.Key, err)
		}
	}
	return nil
}

// validateAppMirror comprueba la trazabilidad del espejo: de dónde viene, qué
// digest exacto se copió, y que nuestra imagen tenga ESE digest y no otro.
func validateAppMirror(a CatalogApp, allowedRegistry string) error {
	if !strings.Contains(a.Upstream, ":") || strings.HasSuffix(a.Upstream, ":") {
		return fmt.Errorf(
			"%s: upstream tiene que decir de dónde se replica CON su etiqueta (p. ej. "+
				"docker.io/valkey/valkey:9.1.1-alpine), y llegó %q", a.ID, a.Upstream,
		)
	}
	if allowedRegistry != "" && strings.HasPrefix(a.Upstream, allowedRegistry+"/") {
		return fmt.Errorf("%s: el upstream no puede ser nuestro propio registro", a.ID)
	}
	if !digestRe.MatchString(a.UpstreamDigest) && !digestRe.MatchString("@"+a.UpstreamDigest) {
		return fmt.Errorf("%s: upstream_digest mal formado: %q", a.ID, a.UpstreamDigest)
	}
	// La comprobación que de verdad protege: si ya está replicada, su digest tiene
	// que ser EL MISMO que el de origen. Una copia fiel lo conserva; que difiera
	// significa que se copió otra cosa —casi siempre, un `docker pull` + `push`
	// que reconstruyó el manifiesto y se dejó una arquitectura—.
	if a.MirroredDigest != "" && a.MirroredDigest != a.UpstreamDigest {
		return fmt.Errorf(
			"%s: la copia tiene el digest %s pero el origen es %s. Una copia fiel conserva el "+
				"digest: si no coincide, no se copió lo mismo",
			a.ID, a.MirroredDigest, a.UpstreamDigest,
		)
	}
	return nil
}

// ImageRef compone la referencia con la que se tira de la imagen: el registro
// del DESPLIEGUE más el id y el digest de nuestra copia.
//
// Que el registro venga de la configuración y no del catálogo es lo que impide
// que las dos verdades se separen: el namespace del PaaS se llama distinto en
// dev y en producción, y un catálogo con el host escrito a mano apuntaba a un
// sitio que en producción no existe.
func (a CatalogApp) ImageRef(registry string) string {
	return fmt.Sprintf("%s/%s@%s", strings.TrimSuffix(registry, "/"), a.ID, a.MirroredDigest)
}

// validateAppEnv: nombres reconocibles, y `generate` y `value` se excluyen.
// validateAppConfigFiles: rutas absolutas, sin repetir, con contenido y sin
// pisar el disco de datos ni un directorio efímero.
//
// Lo de «sin pisar» no es teórico: montar un fichero dentro de `data_path` con
// subPath lo escondería en cada arranque, y el usuario vería que sus cambios
// desaparecen sin que nada falle.
func validateAppConfigFiles(a CatalogApp) error {
	vistas := map[string]bool{}
	for _, c := range a.ConfigFiles {
		if !strings.HasPrefix(c.Path, "/") {
			return fmt.Errorf("%s: config_files necesita ruta absoluta: %q", a.ID, c.Path)
		}
		if vistas[c.Path] {
			return fmt.Errorf("%s: config_files repite %q", a.ID, c.Path)
		}
		vistas[c.Path] = true
		if strings.TrimSpace(c.Content) == "" {
			return fmt.Errorf("%s: config_files %q sin contenido", a.ID, c.Path)
		}
		if len(c.Content) > MaxAppConfigBytes {
			return fmt.Errorf("%s: config_files %q pesa %d B (tope %d)", a.ID, c.Path, len(c.Content), MaxAppConfigBytes)
		}
		if a.DataPath != "" && strings.HasPrefix(c.Path, strings.TrimSuffix(a.DataPath, "/")+"/") {
			return fmt.Errorf(
				"%s: config_files %q cae dentro del disco de datos (%s): al montarlo taparía lo que "+
					"haya ahí en cada arranque, sin que nada falle", a.ID, c.Path, a.DataPath,
			)
		}
		// `/tmp` va PRIMERO y aparte de la lista declarada: lo monta siempre el
		// aprovisionador, y desde que declararlo es un error de validación ya no
		// puede estar en `writable_paths`. Sin esta línea, un `config_files` en
		// `/tmp/...` dejó de rechazarlo nadie — el fichero se monta y el emptyDir
		// lo tapa en cada arranque, que es justo el fallo mudo que esta función
		// existe para impedir.
		if strings.HasPrefix(c.Path, "/tmp/") {
			return fmt.Errorf("%s: config_files %q cae dentro de /tmp, que es efímero y se monta siempre", a.ID, c.Path)
		}
		for _, w := range a.WritablePaths {
			if strings.HasPrefix(c.Path, strings.TrimSuffix(w, "/")+"/") {
				return fmt.Errorf("%s: config_files %q cae dentro del directorio efímero %s", a.ID, c.Path, w)
			}
		}
		for _, m := range placeholderRe.FindAllString(c.Content, -1) {
			if !AppEnvPlaceholders[m] {
				return fmt.Errorf("%s: config_files %q usa %s, que no es un marcador conocido", a.ID, c.Path, m)
			}
		}
	}
	return nil
}

func validateAppEnv(a CatalogApp) error {
	pide := map[string]bool{}
	for _, n := range a.Needs {
		pide[n] = true
	}
	seen := map[string]bool{}
	for _, e := range a.Env {
		if !exportKeyRe.MatchString(e.Key) {
			return fmt.Errorf("%s: env %q no es un nombre de variable válido", a.ID, e.Key)
		}
		if seen[e.Key] {
			return fmt.Errorf("%s: env %q repetida", a.ID, e.Key)
		}
		seen[e.Key] = true
		if e.Generate && e.Value != "" {
			return fmt.Errorf(
				"%s: env %s pide `generate` Y trae `value`: uno de los dos sobra, y adivinar cuál "+
					"acabaría dando el mismo secreto a todos los clientes", a.ID, e.Key,
			)
		}
		if !e.Generate && e.Value == "" {
			return fmt.Errorf("%s: env %s no tiene valor ni pide generarlo", a.ID, e.Key)
		}
		for _, m := range placeholderRe.FindAllString(e.Value, -1) {
			if !AppEnvPlaceholders[m] {
				return fmt.Errorf("%s: env %s usa %s, que no es un marcador conocido", a.ID, e.Key, m)
			}
			// Un `{db_host}` sin `needs: postgres` arrancaría la app con esa cadena
			// LITERAL como servidor: no falla al desplegar, falla al conectarse, y
			// el motivo no aparece por ningún lado.
			if strings.HasPrefix(m, "{db_") && !pide[AppNeedPostgres] {
				return fmt.Errorf(
					"%s: env %s usa %s pero la app no declara `needs: [\"postgres\"]`; sin eso el "+
						"valor llega literal al contenedor", a.ID, e.Key, m,
				)
			}
		}
	}
	return nil
}

// validateAppResources: el techo es obligatorio, y tiene tope.
func validateAppResources(a CatalogApp) error {
	if a.DiskGiB < 0 || a.DiskGiB > MaxAppDiskGiB {
		return fmt.Errorf("%s: disco %d GiB fuera de 0..%d", a.ID, a.DiskGiB, MaxAppDiskGiB)
	}
	if a.MemoryMiB < 1 || a.MemoryMiB > MaxAppMemoryMiB {
		return fmt.Errorf("%s: memoria %d MiB fuera de 1..%d", a.ID, a.MemoryMiB, MaxAppMemoryMiB)
	}
	if a.CPUMilli < 1 || a.CPUMilli > MaxAppCPUMilli {
		return fmt.Errorf("%s: cpu %dm fuera de 1..%d", a.ID, a.CPUMilli, MaxAppCPUMilli)
	}
	if a.HealthPath != "" && !strings.HasPrefix(a.HealthPath, "/") {
		return fmt.Errorf("%s: health_path tiene que empezar por /", a.ID)
	}
	if a.DiskGiB > 0 && !strings.HasPrefix(a.DataPath, "/") {
		return fmt.Errorf(
			"%s: pide %d GiB de disco pero no dice dónde montarlo (data_path). Montarlo donde no es "+
				"deja al servicio arrancando en vacío cada vez sin que nada falle",
			a.ID, a.DiskGiB,
		)
	}
	if a.DiskGiB == 0 && a.DataPath != "" {
		return fmt.Errorf("%s: data_path sin disco que montar", a.ID)
	}
	for _, w := range a.WritablePaths {
		if !strings.HasPrefix(w, "/") {
			return fmt.Errorf("%s: writable_paths tiene que llevar rutas absolutas: %q", a.ID, w)
		}
		// `/tmp` lo monta SIEMPRE el aprovisionador. Declararlo otra vez produce dos
		// volumeMounts con el mismo mountPath, y eso el API server lo rechaza: el
		// Deployment no se crea y el servicio se queda en «provisioning» para
		// siempre, sin un error que lo explique en el panel.
		if w == "/tmp" || strings.HasPrefix(w, "/tmp/") {
			return fmt.Errorf(
				"%s: writable_paths incluye %q, y /tmp ya se monta siempre: dos montajes en la "+
					"misma ruta impiden crear el Deployment", a.ID, w,
			)
		}
		if w == a.DataPath {
			return fmt.Errorf(
				"%s: %q es a la vez el disco de datos y un directorio efímero: montar los dos "+
					"encima del mismo sitio haría que los datos se perdieran al reiniciar",
				a.ID, w,
			)
		}
	}
	// El 0 es root. El namespace del cliente exige `restricted`, así que un pod
	// que lo pida ni siquiera se admite: mejor descubrirlo aquí que en el primer
	// alta de un cliente.
	if a.RunAsUser <= 0 {
		return fmt.Errorf("%s: run_as_user tiene que ser un UID distinto de root", a.ID)
	}
	return nil
}

func licenseAllowed(spdx string) bool {
	for _, ok := range AllowedAppLicenses {
		if ok == spdx {
			return true
		}
	}
	return false
}

// LicenseIsCopyleft dice si la licencia obliga a poner el fuente a disposición de
// quien usa el servicio.
func LicenseIsCopyleft(spdx string) bool {
	for _, c := range CopyleftAppLicenses {
		if c == spdx {
			return true
		}
	}
	return false
}

// validateAppSource exige el repositorio de fuentes cuando la licencia lo obliga.
//
// Es el único punto donde el cumplimiento del AGPL deja de depender de que
// alguien se acuerde: sin `source`, la entrada no carga y el servicio no arranca.
func validateAppSource(a CatalogApp) error {
	if a.Source != "" && !strings.HasPrefix(a.Source, "https://") {
		return fmt.Errorf("%s: source tiene que ser https", a.ID)
	}
	if LicenseIsCopyleft(a.License) && a.Source == "" {
		return fmt.Errorf(
			"%s: %s obliga a poner el código fuente a disposición de quien usa el servicio "+
				"(AGPL §13 / GPL §6), así que la entrada necesita `source` con el repositorio",
			a.ID, a.License,
		)
	}
	return nil
}

// placeholderRe encuentra cualquier `{...}` de la plantilla de export.
var placeholderRe = regexp.MustCompile(`\{[^}]*\}`)

// validExportTemplate acepta SOLO `{host}` y `{port}`.
//
// Que no sea un lenguaje es la decisión: el valor de estas variables acaba
// dentro de la app del cliente, y cada capacidad que se añada aquí —leer otro
// servicio, componer una credencial, mirar el entorno— es una vía nueva para que
// una entrada del catálogo filtre algo que no le corresponde.
func validExportTemplate(v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("valor vacío")
	}
	for _, m := range placeholderRe.FindAllString(v, -1) {
		if m != "{host}" && m != "{port}" {
			return fmt.Errorf("%s no está permitido: solo {host} y {port}", m)
		}
	}
	return nil
}

// RenderExports resuelve las variables que este servicio inyecta en la app.
func RenderExports(a CatalogApp, host string) map[string]string {
	return renderEnvTemplates(a.Exports, host, MainPort(a))
}

// RenderIntegration resuelve las variables que recibe la app que DECLARA la
// integración, apuntando al servicio de la otra.
func RenderIntegration(in AppIntegration, host string, port int) map[string]string {
	return renderEnvTemplates(in.Env, host, port)
}

// MainPort es el puerto por el que se habla con una app: el primero que declara.
func MainPort(a CatalogApp) int {
	if len(a.Ports) == 0 {
		return 0
	}
	return a.Ports[0].Port
}

func renderEnvTemplates(list []AppEnvExport, host string, port int) map[string]string {
	out := make(map[string]string, len(list))
	for _, e := range list {
		v := strings.ReplaceAll(e.Value, "{host}", host)
		out[e.Key] = strings.ReplaceAll(v, "{port}", fmt.Sprint(port))
	}
	return out
}

// IntegratorsOf devuelve los ids de las apps del catálogo que declaran una
// integración con appID, o sea a quién le cambia el entorno que appID aparezca o
// desaparezca de un proyecto.
//
// Es la dirección CONTRARIA a la que declara el catálogo, y por eso hace falta
// recorrerlo: una entrada dice de quién sabe consumir, no quién la consume a ella.
func IntegratorsOf(cat AppCatalog, appID string) []string {
	if appID == "" {
		return nil
	}
	var out []string
	for _, a := range cat.Apps {
		if a.ID == appID {
			continue
		}
		for _, in := range a.Integrations {
			if in.App == appID {
				out = append(out, a.ID)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// SortCatalogApps ordena por categoría y nombre, para que la lista no baile
// entre peticiones (el orden de un mapa en Go es aleatorio a propósito).
func SortCatalogApps(apps []CatalogApp) {
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].Category != apps[j].Category {
			return apps[i].Category < apps[j].Category
		}
		return apps[i].Name < apps[j].Name
	})
}
