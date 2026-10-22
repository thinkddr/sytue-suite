// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// ---------------------------------------------------------------------------
// Mirar un repositorio antes de crearlo, para no preguntar lo que se puede ver
// ---------------------------------------------------------------------------
//
// El alta de un proyecto pedía framework, estrategia de build, comando de
// instalación, comando de build, directorio de salida, rama y subdirectorio.
// Siete decisiones, y las siete están escritas en el repositorio que el usuario
// acaba de pegar. Para alguien que «sabe lo justo de programación», `output_dir`
// no es una pregunta: es un muro.
//
// El detector por ficheros ya existía (DetectScript), pero corre DENTRO del Job
// de build, o sea después del alta y cuando ya no hay nadie mirando. Aquí se
// adelanta al formulario.
//
// Dos decisiones de fondo:
//
//   - **Se mira, no se clona.** Un `git clone` en el camino del alta es minutos,
//     disco y un repositorio ajeno dentro del control-plane. La API de contenidos
//     de la forja da los nombres de la raíz y un fichero suelto en dos peticiones.
//   - **Cuando no se sabe, se dice.** `Confident=false` no es un fallo: es la
//     diferencia entre rellenar el formulario con algo razonable y rellenarlo con
//     algo inventado que el usuario descubrirá cuando su primer despliegue sirva
//     una página en blanco.

// RepoLayout es lo que se ve de un repositorio sin clonarlo.
type RepoLayout struct {
	// Files son los nombres de fichero de la raíz (o del subdirectorio elegido).
	Files []string
	// PackageJSON es el contenido de package.json, si lo hay.
	PackageJSON []byte
}

// Detected es la configuración de despliegue deducida del repositorio.
type Detected struct {
	Framework     domain.Framework     `json:"framework"`
	BuildStrategy domain.BuildStrategy `json:"build_strategy,omitempty"`
	InstallCmd    string               `json:"install_command,omitempty"`
	BuildCmd      string               `json:"build_command,omitempty"`
	OutputDir     string               `json:"output_dir,omitempty"`
	// Confident dice si esto se puede aplicar sin preguntar. En falso, el
	// formulario tiene que enseñar los campos: es mejor una pregunta que un
	// despliegue que sirve una página en blanco.
	Confident bool `json:"confident"`
	// Evidence es el fichero que lo delató, para ENSEÑARLO. «Detectado: Astro»
	// obliga a creerse la magia; «astro.config.mjs → Astro» se puede comprobar.
	Evidence string `json:"evidence,omitempty"`
}

// packageJSON es lo poco que se lee del manifiesto de Node: qué construir y con
// qué. Campos sueltos y no el fichero entero porque el resto no se usa, y un
// `map[string]any` invita a leer cosas que no se han pensado.
//
// Los nombres en camelCase no son un despiste: este esquema lo define npm, no
// nosotros, y renombrarlos a snake_case dejaría los campos vacíos en silencio.
//
//nolint:tagliatelle // el formato es de package.json (npm), no de nuestra API
type packageJSON struct {
	Scripts      map[string]string `json:"scripts"`
	Dependencies map[string]string `json:"dependencies"`
	DevDeps      map[string]string `json:"devDependencies"`
	PackageMgr   string            `json:"packageManager"`
}

// DetectFromRepo deduce la configuración de despliegue de lo que se ve del repo.
//
// El ORDEN es el mismo que el de DetectScript (el detector que corre dentro del
// build) y eso no es casualidad: dos detectores con reglas distintas darían un
// framework en el alta y otro al construir, y el usuario vería cambiar su
// proyecto solo.
func DetectFromRepo(l RepoLayout) Detected {
	tiene := func(nombre string) bool {
		for _, f := range l.Files {
			if strings.EqualFold(f, nombre) {
				return true
			}
		}
		return false
	}
	tienePrefijo := func(pref string) string {
		for _, f := range l.Files {
			if strings.HasPrefix(strings.ToLower(f), pref) {
				return f
			}
		}
		return ""
	}

	switch {
	case tiene("Dockerfile"):
		// El repo trae su receta: no hay comandos que adivinar ni salida que servir.
		return Detected{
			Framework: domain.FrameworkDocker, BuildStrategy: domain.BuildStrategyDockerfile,
			Confident: true, Evidence: "Dockerfile",
		}
	case tienePrefijo("astro.config.") != "":
		return conNode(l, domain.FrameworkAstro, "dist", tienePrefijo("astro.config."))
	case tiene("go.mod"):
		return Detected{
			Framework: domain.FrameworkGo, BuildStrategy: domain.BuildStrategyDockerfile,
			Confident: true, Evidence: "go.mod",
		}
	case tiene("package.json"):
		return detectNode(l)
	case tiene("index.html"):
		return Detected{
			Framework: domain.FrameworkStatic, BuildStrategy: domain.BuildStrategyDockerfile,
			OutputDir: ".", Confident: true, Evidence: "index.html",
		}
	}
	// Sin ninguna pista: buildpacks es lo que detecta lenguajes que aquí no se
	// miran (Python, Ruby, PHP, Java) sin pedirle un Dockerfile a nadie. Pero no
	// se marca como seguro, porque no se ha visto nada que lo respalde.
	return Detected{Framework: domain.FrameworkDetect, BuildStrategy: domain.BuildStrategyBuildpacks}
}

// ssrFrameworks son los metaframeworks que necesitan un SERVIDOR en marcha, no
// un directorio de ficheros.
//
// Importa porque el Dockerfile que Sytue Cloud genera sirve estáticos con nginx:
// darle el directorio de salida de un Next con SSR produciría una build verde y
// una web rota. Para estos se elige buildpacks, que arranca el `start` del
// proyecto — es más lento de construir, y sirve.
var ssrFrameworks = []string{"next", "nuxt", "@remix-run/dev", "@angular/ssr"}

// outputPorDependencia mapea el generador de sitios estáticos a DÓNDE deja los
// ficheros. Es la pregunta `output_dir` del formulario, respondida.
var outputPorDependencia = []struct{ dep, out string }{
	{"@sveltejs/kit", "build"},
	{"vite", "dist"},
	{"react-scripts", "build"},
	{"@11ty/eleventy", "_site"},
	{"vuepress", ".vuepress/dist"},
}

func detectNode(l RepoLayout) Detected {
	var pkg packageJSON
	if len(l.PackageJSON) > 0 {
		// Un package.json ilegible no es un error del alta: se trata como si no
		// estuviera, y el usuario rellena lo que falte.
		_ = json.Unmarshal(l.PackageJSON, &pkg)
	}
	deps := map[string]string{}
	for k, v := range pkg.Dependencies {
		deps[k] = v
	}
	for k, v := range pkg.DevDeps {
		deps[k] = v
	}

	for _, ssr := range ssrFrameworks {
		if _, hay := deps[ssr]; hay {
			// Node + buildpacks: el CNB de Node instala, construye y arranca el
			// `start` del proyecto. No se marca `Confident` porque el puerto y el
			// arranque son suyos, y conviene que el usuario lo vea.
			return Detected{
				Framework: domain.FrameworkNode, BuildStrategy: domain.BuildStrategyBuildpacks,
				InstallCmd: installCmd(l, pkg), BuildCmd: buildCmd(pkg),
				Evidence: "package.json → " + ssr,
			}
		}
	}
	if _, hay := deps["astro"]; hay {
		return conNode(l, domain.FrameworkAstro, "dist", "package.json → astro")
	}
	for _, m := range outputPorDependencia {
		if _, hay := deps[m.dep]; hay {
			return conNode(l, domain.FrameworkNode, m.out, "package.json → "+m.dep)
		}
	}
	// package.json sin generador reconocido: se sabe que es Node y con qué
	// instalar, pero no dónde deja los ficheros. Eso último se pregunta.
	return Detected{
		Framework: domain.FrameworkNode, BuildStrategy: domain.BuildStrategyDockerfile,
		InstallCmd: installCmd(l, pkg), BuildCmd: buildCmd(pkg),
		Evidence: "package.json",
	}
}

func conNode(l RepoLayout, fw domain.Framework, out, evidencia string) Detected {
	var pkg packageJSON
	if len(l.PackageJSON) > 0 {
		_ = json.Unmarshal(l.PackageJSON, &pkg)
	}
	return Detected{
		Framework: fw, BuildStrategy: domain.BuildStrategyDockerfile,
		InstallCmd: installCmd(l, pkg), BuildCmd: buildCmd(pkg),
		OutputDir: out, Confident: true, Evidence: evidencia,
	}
}

// installCmd elige el gestor de paquetes DEL REPOSITORIO, no el nuestro.
//
// Manda el lockfile: instalar un proyecto con pnpm-lock.yaml usando npm ignora
// las versiones fijadas y puede traer dependencias distintas a las que el autor
// probó. `packageManager` (Corepack) se mira después, y solo si no hay lockfile.
func installCmd(l RepoLayout, pkg packageJSON) string {
	lock := map[string]string{
		"bun.lock":          "bun install --frozen-lockfile",
		"bun.lockb":         "bun install --frozen-lockfile",
		"pnpm-lock.yaml":    "pnpm install --frozen-lockfile",
		"yarn.lock":         "yarn install --frozen-lockfile",
		"package-lock.json": "npm ci",
	}
	for _, f := range l.Files {
		if cmd, hay := lock[strings.ToLower(f)]; hay {
			return cmd
		}
	}
	switch {
	case strings.HasPrefix(pkg.PackageMgr, "pnpm"):
		return "pnpm install"
	case strings.HasPrefix(pkg.PackageMgr, "yarn"):
		return "yarn install"
	case strings.HasPrefix(pkg.PackageMgr, "bun"):
		return "bun install"
	}
	// Sin lockfile ni declaración: npm es el que trae Node de serie, así que es el
	// único que se puede dar por disponible.
	return "npm install"
}

// buildCmd usa el script `build` del propio proyecto; si no lo tiene, no hay nada
// que construir y decirlo con un comando inventado sería peor.
func buildCmd(pkg packageJSON) string {
	if _, hay := pkg.Scripts["build"]; !hay {
		return ""
	}
	switch {
	case strings.HasPrefix(pkg.PackageMgr, "pnpm"):
		return "pnpm run build"
	case strings.HasPrefix(pkg.PackageMgr, "yarn"):
		return "yarn build"
	case strings.HasPrefix(pkg.PackageMgr, "bun"):
		return "bun run build"
	}
	return "npm run build"
}

// ---------------------------------------------------------------------------
// Leer el repositorio de la forja
// ---------------------------------------------------------------------------

// forgeHosts son las forjas cuya API se sabe hablar. Es una ALLOWLIST y no una
// heurística sobre la URL: la escribe el usuario, así que sin lista cerrada esto
// sería una petición saliente a donde él diga —el manual de SSRF—.
//
// Gitea y GitHub comparten la forma de la API de contenidos; lo único que cambia
// es el prefijo. Una forja que no esté aquí no impide crear el proyecto: devuelve
// «no se pudo mirar» y el formulario pregunta, que es como funcionaba antes.
var forgeHosts = map[string]string{
	"git.gay":      "/api/v1/repos/%s/%s/contents/%s",
	"git.sytue.eu": "/api/v1/repos/%s/%s/contents/%s", // self-hosted, retirado 2026-09-04; queda mientras existan proyectos ya creados desde ahí
	"github.com":   "https://api.github.com/repos/%s/%s/contents/%s",
	"codeberg.org": "/api/v1/repos/%s/%s/contents/%s",
}

// ErrForgeDesconocida es lo que devuelve InspectRepo cuando no sabe leer esa
// forja. Es una respuesta legítima, no un fallo: el alta sigue.
var ErrForgeDesconocida = fmt.Errorf("builder: la forja no está entre las que se saben leer")

// maxPackageJSON acota lo que se descarga. Un package.json real no llega a 100 KB;
// el límite está para que un fichero enorme —o un servidor hostil que finge serlo—
// no se lea entero en memoria del control-plane.
const maxPackageJSON = 512 * 1024

// forgeClient es el cliente HTTP con el que se habla con la forja: con guardián
// de SSRF (no alcanza redes privadas ni el metadata del proveedor) y con tope de
// redirecciones, porque un 302 hacia dentro es la forma clásica de rodear una
// allowlist de hosts.
//
// Es una variable y no una constante para que el test pueda sustituirla: httptest
// sirve en 127.0.0.1 con un certificado propio, o sea las dos cosas exactas que
// este cliente está para rechazar.
var forgeClient = func() *http.Client { return ssrfhttp.NewClient(10*time.Second, 3) }

// InspectRepo mira un repositorio por la API de su forja y deduce cómo desplegarlo.
//
// `rootDir` acota a un subdirectorio (monorepos). El token es opcional: sin él
// solo se leen repositorios públicos, que es el caso normal en el alta —el
// privado se conecta después con la app de la forja—.
func InspectRepo(ctx context.Context, repoURL, rootDir, token string) (Detected, error) {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return Detected{}, fmt.Errorf("builder: URL de repositorio inválida")
	}
	plantilla, ok := forgeHosts[strings.ToLower(u.Host)]
	if !ok {
		return Detected{}, ErrForgeDesconocida
	}
	owner, repo, ok := repoPath(u.Path)
	if !ok {
		return Detected{}, fmt.Errorf("builder: la URL no apunta a un repositorio")
	}

	dir := strings.Trim(rootDir, "/")
	cliente := forgeClient()
	lista, err := forgeGet(ctx, cliente, endpoint(plantilla, u, owner, repo, dir), token)
	if err != nil {
		return Detected{}, err
	}
	var entradas []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if uerr := json.Unmarshal(lista, &entradas); uerr != nil {
		return Detected{}, fmt.Errorf("builder: la forja no devolvió un listado de ficheros")
	}
	layout := RepoLayout{Files: make([]string, 0, len(entradas))}
	for _, e := range entradas {
		if e.Type == "file" {
			layout.Files = append(layout.Files, e.Name)
		}
	}

	// El package.json solo se pide si el listado dice que existe: una petición
	// menos, y ningún 404 que interpretar.
	for _, f := range layout.Files {
		if f != "package.json" {
			continue
		}
		raw, ferr := forgeGet(ctx, cliente, endpoint(plantilla, u, owner, repo, path.Join(dir, f)), token)
		if ferr == nil {
			layout.PackageJSON = decodeContenido(raw)
		}
		break
	}
	return DetectFromRepo(layout), nil
}

// endpoint compone la URL de la API. Gitea la sirve en el propio host de la
// forja; GitHub, en api.github.com — de ahí que la plantilla pueda ser absoluta.
func endpoint(plantilla string, u *url.URL, owner, repo, p string) string {
	ruta := fmt.Sprintf(plantilla, url.PathEscape(owner), url.PathEscape(repo), p)
	if strings.HasPrefix(ruta, "https://") {
		return ruta
	}
	return "https://" + u.Host + ruta
}

// repoPath saca owner y repo de la ruta, tolerando el sufijo .git y lo que venga
// detrás (un enlace copiado de la barra del navegador trae /tree/main).
func repoPath(p string) (string, string, bool) {
	partes := strings.Split(strings.Trim(p, "/"), "/")
	if len(partes) < 2 || partes[0] == "" || partes[1] == "" {
		return "", "", false
	}
	return partes[0], strings.TrimSuffix(partes[1], ".git"), true
}

func forgeGet(ctx context.Context, c *http.Client, endpoint, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("builder: componer la petición a la forja: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("builder: la forja no responde")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("builder: la forja respondió %d al leer el repositorio", res.StatusCode)
	}
	// LimitReader y no ReadAll a secas: al otro lado hay un servidor que el usuario
	// eligió, y «devuelve un flujo infinito» es la forma barata de tumbar a quien lo
	// lee entero en memoria.
	return io.ReadAll(io.LimitReader(res.Body, maxPackageJSON))
}

// decodeContenido saca el fichero de la respuesta de la API de contenidos, que lo
// entrega en base64 dentro de un objeto.
func decodeContenido(raw []byte) []byte {
	var c struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(raw, &c); err != nil || c.Encoding != "base64" {
		return nil
	}
	// Los saltos de línea que mete GitHub cada 60 caracteres no los acepta el
	// decodificador estándar.
	limpio := strings.NewReplacer("\n", "", "\r", "").Replace(c.Content)
	out, err := base64.StdEncoding.DecodeString(limpio)
	if err != nil {
		return nil
	}
	return out
}
