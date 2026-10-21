// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	"fmt"
	"strings"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// DetectFramework decide el framework efectivo de un proyecto. Si el proyecto lo
// declara explícitamente, se respeta; con 'auto' se deduce de la configuración
// (build_command/output_dir) y, en su defecto, el builder inspecciona el repo en
// el Job de Kaniko (heurística de ficheros) — esa parte la hace el script de
// preparación del contexto. Aquí resolvemos la parte determinable sin clonar.
func DetectFramework(p domain.Project) domain.Framework {
	if p.Framework != "" && p.Framework != domain.FrameworkDetect {
		return p.Framework
	}
	switch {
	case strings.Contains(strings.ToLower(p.BuildCommand), "astro"):
		return domain.FrameworkAstro
	case p.OutputDir != "" && p.BuildCommand != "":
		return domain.FrameworkNode
	default:
		// 'auto' real: el contexto de build ejecuta el detector por ficheros
		// (astro.config.*, go.mod, package.json, Dockerfile, index.html).
		return domain.FrameworkDetect
	}
}

// ResolveBuildStrategy decide CÓMO construir un proyecto. Si el proyecto declara
// una estrategia explícita se respeta (override del usuario); con 'auto' (vacío)
// aplica el default razonable: solo 'auto' (framework sin resolver) usa Cloud
// Native Buildpacks, que auto-detectan el lenguaje SIN Dockerfile. Los frameworks
// gestionados (docker/astro/static/node/go) conservan su Dockerfile afinado
// —propio para 'docker', generado por GenerateDockerfile para el resto— vía
// Kaniko, que es lo que ya construían; optar por CNB en ellos es explícito
// (build_strategy='buildpacks'). La decisión se toma aquí (sin clonar) porque
// selecciona la forma del Job.
func ResolveBuildStrategy(p domain.Project, fw domain.Framework) domain.BuildStrategy {
	if p.BuildStrategy == domain.BuildStrategyDockerfile || p.BuildStrategy == domain.BuildStrategyBuildpacks {
		return p.BuildStrategy
	}
	if fw == domain.FrameworkDetect {
		return domain.BuildStrategyBuildpacks
	}
	return domain.BuildStrategyDockerfile
}

// DetectScript es el fragmento shell que corre dentro del initContainer de
// preparación del contexto para deducir el framework por ficheros del repo
// cuando es 'auto'. Devuelve la cadena para SYTUE_FRAMEWORK. Clean-room: lógica
// propia, sin código de terceros.
const DetectScript = `
detect_framework() {
  if [ -f Dockerfile ]; then echo docker; return; fi
  if ls astro.config.* >/dev/null 2>&1; then echo astro; return; fi
  if [ -f go.mod ]; then echo go; return; fi
  if [ -f package.json ]; then echo node; return; fi
  if [ -f index.html ]; then echo static; return; fi
  echo static
}
`

// generatedDockerfileName es el nombre del Dockerfile que Sytue Cloud genera y
// materializa (ConfigMap) para frameworks gestionados; también es la clave del
// ConfigMap y el subPath del montaje.
const generatedDockerfileName = "Dockerfile.sytue"

// generatedDockerfilePath es la ruta absoluta —dentro del /workspace compartido—
// donde el builder materializa el Dockerfile generado y que Kaniko lee vía
// --dockerfile. Debe coincidir con el MountPath del ConfigMap en kubeclient.go.
const generatedDockerfilePath = "/workspace/" + generatedDockerfileName

// DockerfileFor devuelve la ruta o el contenido de Dockerfile a usar. Para
// proyectos 'docker' se usa el Dockerfile del repo; para el resto, Sytue Cloud
// genera un Dockerfile estándar por framework (build reproducible, rootless,
// imagen mínima). El builder materializa este contenido en el contexto.
func DockerfileFor(fw domain.Framework, p domain.Project) string {
	if fw == domain.FrameworkDocker {
		// Ruta relativa al contexto; respeta root_dir.
		if p.RootDir != "" {
			return strings.TrimRight(p.RootDir, "/") + "/Dockerfile"
		}
		return "Dockerfile"
	}
	// Para frameworks gestionados se materializa /workspace/Dockerfile.sytue.
	return generatedDockerfilePath
}

// GenerateDockerfile produce un Dockerfile estándar para el framework dado. Son
// plantillas propias (clean-room) con imágenes base permisivas y contenedor
// final rootless/distroless o nginx estático.
func GenerateDockerfile(fw domain.Framework, p domain.Project) (string, error) {
	build := firstNonEmpty(p.BuildCommand, defaultBuildCmd(fw))
	install := firstNonEmpty(p.InstallCmd, defaultInstallCmd(fw))
	out := firstNonEmpty(p.OutputDir, defaultOutputDir(fw))

	switch fw {
	case domain.FrameworkAstro, domain.FrameworkStatic, domain.FrameworkNode:
		// SSG/estático: build con Bun y se sirve con un servidor estático mínimo.
		return fmt.Sprintf(`# Generado por Sytue Cloud — NO editar
FROM oven/bun:1.4.3@sha256:ec06c3b6cea04192ae6770c434f668ca41d343ad19fa6472216c7b48be39c598 AS build
WORKDIR /app
COPY . .
RUN %s
RUN %s
%s
FROM nginxinc/nginx-unprivileged:stable AS run
COPY --from=build /app/%s /usr/share/nginx/html
EXPOSE 8080
`, install, build, analyticsInjectStep("/app/"+strings.Trim(out, "/")), strings.TrimLeft(out, "/")), nil
	case domain.FrameworkGo:
		return `# Generado por Sytue Cloud — NO editar
FROM golang:1.27.3@sha256:f24164268fc1e741bbdc0066dbadcf1c8426c5a8489f0729c02c3905a53032fd AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/app ./...
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
`, nil
	default:
		return "", fmt.Errorf("builder: sin plantilla de Dockerfile para framework %q", fw)
	}
}

// AnalyticsTag es la etiqueta que mide un sitio con Sytue Analytics. Se sirve
// desde el dominio del propio sitio —el Ingress del proyecto enruta /_sytue al
// colector (controller.applyIngress)—, así que no hay CORS, no hay dominio que
// un bloqueador pueda vetar y el cliente no toca su CSP.
const AnalyticsTag = `<script defer src="/_sytue/s.js"></script>`

// analyticsInjectStep es el paso de build que cablea Sytue Analytics en TODO el
// HTML que produce la app, sin que nadie pegue nada en su plantilla.
//
// Se inyecta SIEMPRE, esté la analítica activa o no, y eso es lo que hace que el
// complemento se encienda con un clic: quien decide si se mide es el colector
// —devuelve 404 en /_sytue/s.js si el hostname no está dado de alta
// (analyticshttp.serveScript)—, no esta etiqueta. Al revés, inyectar solo cuando
// está activa convertiría «activar la analítica» en «reconstruir y redesplegar la
// app», que es justo la fricción que el complemento viene a quitar.
//
// Tres detalles que no son de estilo:
//
//   - **Idempotente.** El `grep -q` salta el fichero que ya la trae: un usuario
//     que la pegó a mano en su plantilla contaría cada visita DOS veces, y una
//     analítica que miente es peor que no tenerla.
//   - **POSIX, no GNU.** Ni `sed -i` (BSD lo escribe distinto) ni `xargs -r` (BSD
//     no lo tiene): así el paso se puede EJECUTAR en el test desde cualquier
//     máquina en vez de solo leerse, y un test que solo se salta no comprueba nada.
//   - **El texto viaja como argumento**, no incrustado en el script: lleva comillas
//     dobles, y meterlas dentro del `sh -c` es la vía directa a un Dockerfile que
//     no parsea.
//
// Un HTML sin `</head>` se queda sin medir y no falla la build: es HTML legal,
// pero romper el despliegue de alguien por una etiqueta de analítica sería
// exactamente la prioridad al revés.
func analyticsInjectStep(dir string) string {
	return "RUN " + analyticsInjectCmd(dir)
}

// analyticsInjectCmd es el comando suelto, sin el `RUN`, para poder ejecutarlo tal
// cual en el test.
//
// El directorio sale de `output_dir`, que lo escribe el cliente, así que va
// entrecomillado: uno con un espacio —«mi salida»— partiría el `find` y dejaría
// la build rota por una etiqueta de analítica. Que el cliente ya pueda ejecutar
// lo que quiera en SU build (`RUN <build_command>`) no es razón para interpolar
// sin comillas: el día que este Dockerfile se genere para otra cosa, la costumbre
// es lo que queda.
func analyticsInjectCmd(dir string) string {
	return fmt.Sprintf(
		`find %s -name '*.html' -exec sh -c `+
			`'grep -q "$2" "$1" || { sed "s#</head>#$3</head>#" "$1" > "$1.sytue" && mv "$1.sytue" "$1"; }' `+
			`_ {} '/_sytue/s.js' '%s' ';'`,
		shellQuote(dir), AnalyticsTag,
	)
}

// shellQuote entrecomilla para `sh`: comillas simples, y las comillas simples de
// dentro se cierran, se escapan y se reabren ('\” es el idioma POSIX).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func defaultBuildCmd(fw domain.Framework) string {
	switch fw {
	case domain.FrameworkAstro:
		return "bun run build"
	case domain.FrameworkNode:
		return "bun run build"
	default:
		return "true"
	}
}

func defaultInstallCmd(fw domain.Framework) string {
	switch fw {
	case domain.FrameworkAstro, domain.FrameworkNode:
		return "bun install --frozen-lockfile"
	default:
		return "true"
	}
}

func defaultOutputDir(fw domain.Framework) string {
	switch fw {
	case domain.FrameworkAstro:
		return "dist"
	case domain.FrameworkNode:
		return "dist"
	default:
		return "."
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
