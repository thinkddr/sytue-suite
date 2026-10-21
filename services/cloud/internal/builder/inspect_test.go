// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func pkgJSON(t *testing.T, v map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestQueSeDeduceDeUnRepositorio cubre lo que el formulario deja de preguntar.
// Cada caso es una pregunta que un novato no sabía contestar.
func TestQueSeDeduceDeUnRepositorio(t *testing.T) {
	casos := []struct {
		nombre string
		layout RepoLayout
		quiero Detected
	}{
		{
			// El Dockerfile manda sobre todo lo demás, igual que en DetectScript: si
			// el repo trae su receta, inventarle comandos sería pisarla.
			nombre: "Dockerfile gana a package.json",
			layout: RepoLayout{Files: []string{"package.json", "Dockerfile", "index.html"}},
			quiero: Detected{
				Framework: domain.FrameworkDocker, BuildStrategy: domain.BuildStrategyDockerfile,
				Confident: true, Evidence: "Dockerfile",
			},
		},
		{
			nombre: "Astro por su fichero de configuración",
			layout: RepoLayout{
				Files: []string{"astro.config.mjs", "package.json", "bun.lock"},
				PackageJSON: pkgJSON(t, map[string]any{
					"scripts": map[string]string{"build": "astro build"},
				}),
			},
			quiero: Detected{
				Framework: domain.FrameworkAstro, BuildStrategy: domain.BuildStrategyDockerfile,
				InstallCmd: "bun install --frozen-lockfile", BuildCmd: "npm run build",
				OutputDir: "dist", Confident: true, Evidence: "astro.config.mjs",
			},
		},
		{
			nombre: "Go por go.mod, sin comandos que inventar",
			layout: RepoLayout{Files: []string{"go.mod", "main.go"}},
			quiero: Detected{
				Framework: domain.FrameworkGo, BuildStrategy: domain.BuildStrategyDockerfile,
				Confident: true, Evidence: "go.mod",
			},
		},
		{
			// Vite deja los ficheros en dist: esa es la respuesta a `output_dir`.
			nombre: "Vite con pnpm",
			layout: RepoLayout{
				Files: []string{"package.json", "pnpm-lock.yaml", "vite.config.ts"},
				PackageJSON: pkgJSON(t, map[string]any{
					"scripts":         map[string]string{"build": "vite build"},
					"devDependencies": map[string]string{"vite": "^7.0.0"},
				}),
			},
			quiero: Detected{
				Framework: domain.FrameworkNode, BuildStrategy: domain.BuildStrategyDockerfile,
				InstallCmd: "pnpm install --frozen-lockfile", BuildCmd: "npm run build",
				OutputDir: "dist", Confident: true, Evidence: "package.json → vite",
			},
		},
		{
			// Create React App: la salida es `build`, no `dist`. Acertar aquí es la
			// diferencia entre una web y un 404.
			nombre: "Create React App deja la salida en build",
			layout: RepoLayout{
				Files: []string{"package.json", "package-lock.json"},
				PackageJSON: pkgJSON(t, map[string]any{
					"scripts":      map[string]string{"build": "react-scripts build"},
					"dependencies": map[string]string{"react-scripts": "5.0.1"},
				}),
			},
			quiero: Detected{
				Framework: domain.FrameworkNode, BuildStrategy: domain.BuildStrategyDockerfile,
				InstallCmd: "npm ci", BuildCmd: "npm run build",
				OutputDir: "build", Confident: true, Evidence: "package.json → react-scripts",
			},
		},
		{
			// Next necesita un servidor: servir su directorio de salida con nginx
			// daría una build verde y una web rota, así que va por buildpacks.
			nombre: "Next no se sirve como estático",
			layout: RepoLayout{
				Files: []string{"package.json", "yarn.lock"},
				PackageJSON: pkgJSON(t, map[string]any{
					"scripts":      map[string]string{"build": "next build"},
					"dependencies": map[string]string{"next": "15.0.0"},
				}),
			},
			quiero: Detected{
				Framework: domain.FrameworkNode, BuildStrategy: domain.BuildStrategyBuildpacks,
				InstallCmd: "yarn install --frozen-lockfile", BuildCmd: "npm run build",
				Evidence: "package.json → next",
			},
		},
		{
			nombre: "HTML suelto",
			layout: RepoLayout{Files: []string{"index.html", "estilo.css"}},
			quiero: Detected{
				Framework: domain.FrameworkStatic, BuildStrategy: domain.BuildStrategyDockerfile,
				OutputDir: ".", Confident: true, Evidence: "index.html",
			},
		},
		{
			// Python: aquí no se mira, pero buildpacks sí sabe. No se marca seguro
			// porque no se ha visto nada que lo respalde.
			nombre: "lo que no se reconoce cae en buildpacks y NO se da por seguro",
			layout: RepoLayout{Files: []string{"requirements.txt", "app.py"}},
			quiero: Detected{
				Framework: domain.FrameworkDetect, BuildStrategy: domain.BuildStrategyBuildpacks,
			},
		},
		{
			nombre: "repositorio vacío",
			layout: RepoLayout{},
			quiero: Detected{
				Framework: domain.FrameworkDetect, BuildStrategy: domain.BuildStrategyBuildpacks,
			},
		},
	}

	for _, c := range casos {
		got := DetectFromRepo(c.layout)
		if got != c.quiero {
			t.Errorf("%s:\n  got  %+v\n  want %+v", c.nombre, got, c.quiero)
		}
	}
}

// TestElGestorDePaquetesLoDiceElLockfileDelRepo. Instalar un proyecto con
// pnpm-lock.yaml usando npm ignora las versiones fijadas y puede traer
// dependencias distintas a las que su autor probó — un fallo que aparece en
// producción y no en la build.
func TestElGestorDePaquetesLoDiceElLockfileDelRepo(t *testing.T) {
	casos := map[string]string{
		"bun.lock":          "bun install --frozen-lockfile",
		"bun.lockb":         "bun install --frozen-lockfile",
		"pnpm-lock.yaml":    "pnpm install --frozen-lockfile",
		"yarn.lock":         "yarn install --frozen-lockfile",
		"package-lock.json": "npm ci",
	}
	for lock, quiero := range casos {
		got := DetectFromRepo(RepoLayout{
			Files:       []string{"package.json", lock},
			PackageJSON: pkgJSON(t, map[string]any{"scripts": map[string]string{"build": "x"}}),
		})
		if got.InstallCmd != quiero {
			t.Errorf("con %s: install = %q, quiero %q", lock, got.InstallCmd, quiero)
		}
	}

	// Sin lockfile manda `packageManager` (Corepack); sin ninguno de los dos, npm,
	// que es el único que se puede dar por instalado con Node.
	got := DetectFromRepo(RepoLayout{
		Files:       []string{"package.json"},
		PackageJSON: pkgJSON(t, map[string]any{"packageManager": "pnpm@10.1.0"}),
	})
	if got.InstallCmd != "pnpm install" {
		t.Errorf("con packageManager: install = %q", got.InstallCmd)
	}
	got = DetectFromRepo(RepoLayout{Files: []string{"package.json"}})
	if got.InstallCmd != "npm install" {
		t.Errorf("sin pistas: install = %q", got.InstallCmd)
	}
}

// TestSinScriptBuildNoSeInventaUnComando: un `npm run build` que no existe hace
// fallar la build entera con un error del gestor de paquetes, no del proyecto, y
// quien lo lea no sabrá que se lo pusimos nosotros.
func TestSinScriptBuildNoSeInventaUnComando(t *testing.T) {
	got := DetectFromRepo(RepoLayout{
		Files:       []string{"package.json", "index.html"},
		PackageJSON: pkgJSON(t, map[string]any{"scripts": map[string]string{"test": "vitest"}}),
	})
	if got.BuildCmd != "" {
		t.Errorf("build = %q; sin script `build` no hay nada que ejecutar", got.BuildCmd)
	}
}

// TestUnPackageJSONIlegibleNoRompeElAlta: la respuesta de una forja puede venir
// truncada o no ser JSON. Eso degrada la detección, no el alta.
func TestUnPackageJSONIlegibleNoRompeElAlta(t *testing.T) {
	got := DetectFromRepo(RepoLayout{
		Files:       []string{"package.json", "bun.lock"},
		PackageJSON: []byte("{esto no es json"),
	})
	if got.Framework != domain.FrameworkNode {
		t.Errorf("framework = %q, quiero node por el propio package.json", got.Framework)
	}
	if got.InstallCmd != "bun install --frozen-lockfile" {
		t.Errorf("el lockfile se lee del listado, no del JSON: %q", got.InstallCmd)
	}
}

// --- Lectura de la forja ----------------------------------------------------

// TestSoloSeLeenLasForjasConocidas es la puerta contra el SSRF: la URL del
// repositorio la escribe el usuario, así que sin allowlist esto sería una
// petición saliente del control-plane a donde él diga.
func TestSoloSeLeenLasForjasConocidas(t *testing.T) {
	for _, u := range []string{
		"https://169.254.169.254/latest/meta-data",
		"https://evil.example.com/sytue/one",
		"https://gitlab.com/grupo/proyecto",
		"http://github.com/sytue/one", // sin TLS
		"github.com/sytue/one",        // sin esquema
		"https://github.com/sytue",    // sin repositorio
		"",
	} {
		if _, err := InspectRepo(context.Background(), u, "", ""); err == nil {
			t.Errorf("se intentó leer %q", u)
		}
	}
}

// TestSeLeeElRepositorioYSeDeduce recorre el camino entero contra una forja de
// mentira con la forma real de la API de Gitea/GitHub.
func TestSeLeeElRepositorioYSeDeduce(t *testing.T) {
	pkg := pkgJSON(t, map[string]any{
		"scripts":         map[string]string{"build": "astro build"},
		"devDependencies": map[string]string{"astro": "^5.0.0"},
	})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/contents/"):
			_, _ = fmt.Fprint(w, `[{"name":"astro.config.mjs","type":"file"},
				{"name":"package.json","type":"file"},{"name":"src","type":"dir"}]`)
		case strings.HasSuffix(r.URL.Path, "/contents/package.json"):
			// Con los saltos de línea cada 60 caracteres que mete GitHub, que el
			// decodificador estándar NO acepta.
			b64 := base64.StdEncoding.EncodeToString(pkg)
			_, _ = fmt.Fprintf(w, `{"encoding":"base64","content":%q}`, troceado(b64))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// El servidor de prueba escucha en 127.0.0.1 y con su propio certificado, que
	// son las dos cosas que el cliente de producción rechaza a propósito: se le
	// presta el suyo mientras dura el test.
	original := forgeClient
	forgeClient = srv.Client
	defer func() { forgeClient = original }()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	forgeHosts[u.Host] = "/api/v1/repos/%s/%s/contents/%s"
	defer delete(forgeHosts, u.Host)

	got, err := InspectRepo(context.Background(), srv.URL+"/acme/web.git", "", "")
	if err != nil {
		t.Fatalf("InspectRepo: %v", err)
	}
	if got.Framework != domain.FrameworkAstro || got.OutputDir != "dist" || !got.Confident {
		t.Errorf("detección = %+v", got)
	}
	if got.BuildCmd != "npm run build" {
		t.Errorf("build = %q (el package.json llegó y se leyó)", got.BuildCmd)
	}
}

// troceado parte en líneas de 60 como hace la API de GitHub.
func troceado(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i += 60 {
		fin := min(i+60, len(s))
		b.WriteString(s[i:fin])
		b.WriteString("\n")
	}
	return b.String()
}
