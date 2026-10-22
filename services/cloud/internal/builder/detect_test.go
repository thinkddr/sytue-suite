// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func TestDetectFramework(t *testing.T) {
	cases := []struct {
		name string
		p    domain.Project
		want domain.Framework
	}{
		{"explícito go", domain.Project{Framework: domain.FrameworkGo}, domain.FrameworkGo},
		{"explícito docker", domain.Project{Framework: domain.FrameworkDocker}, domain.FrameworkDocker},
		{"astro por build command", domain.Project{Framework: domain.FrameworkDetect, BuildCommand: "astro build"}, domain.FrameworkAstro},
		{"node por output+build", domain.Project{Framework: domain.FrameworkDetect, BuildCommand: "npm run build", OutputDir: "dist"}, domain.FrameworkNode},
		{"auto sin pistas", domain.Project{Framework: domain.FrameworkDetect}, domain.FrameworkDetect},
		{"vacío se trata como auto", domain.Project{}, domain.FrameworkDetect},
	}
	for _, c := range cases {
		if got := DetectFramework(c.p); got != c.want {
			t.Errorf("%s: DetectFramework = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDockerfileForDockerRespectsRootDir(t *testing.T) {
	if got := DockerfileFor(domain.FrameworkDocker, domain.Project{}); got != "Dockerfile" {
		t.Errorf("Dockerfile por defecto: %q", got)
	}
	if got := DockerfileFor(domain.FrameworkDocker, domain.Project{RootDir: "apps/web/"}); got != "apps/web/Dockerfile" {
		t.Errorf("Dockerfile con root_dir: %q", got)
	}
}

// TestGeneratedDockerfileNonDocker verifica que para frameworks gestionados se
// genera un Dockerfile (no vacío) con contenido de build.
func TestGeneratedDockerfileNonDocker(t *testing.T) {
	for _, fw := range []domain.Framework{domain.FrameworkAstro, domain.FrameworkGo, domain.FrameworkStatic, domain.FrameworkNode} {
		out := DockerfileFor(fw, domain.Project{})
		if strings.TrimSpace(out) == "" {
			t.Errorf("Dockerfile generado vacío para %s", fw)
		}
	}
}

func TestGeneratedDockerfilesPinStableToolchains(t *testing.T) {
	tests := []struct {
		framework domain.Framework
		image     string
	}{
		{framework: domain.FrameworkAstro, image: "FROM oven/bun:1.4.3@sha256:ec06c3b6cea04192ae6770c434f668ca41d343ad19fa6472216c7b48be39c598 AS build"},
		{framework: domain.FrameworkGo, image: "FROM golang:1.27.3@sha256:f24164268fc1e741bbdc0066dbadcf1c8426c5a8489f0729c02c3905a53032fd AS build"},
	}

	for _, test := range tests {
		dockerfile, err := GenerateDockerfile(test.framework, domain.Project{})
		if err != nil {
			t.Fatalf("GenerateDockerfile(%s): %v", test.framework, err)
		}
		if !strings.Contains(dockerfile, test.image) {
			t.Errorf("Dockerfile de %s no fija el toolchain estable %q:\n%s", test.framework, test.image, dockerfile)
		}
	}
}

// TestResolveBuildStrategy cubre la selección de estrategia según proyecto y
// framework: override explícito respetado; en 'auto', solo el framework sin
// resolver (detect) ⇒ buildpacks; los gestionados (docker + astro/static/node/go)
// conservan su Dockerfile ⇒ dockerfile.
func TestResolveBuildStrategy(t *testing.T) {
	cases := []struct {
		name string
		p    domain.Project
		fw   domain.Framework
		want domain.BuildStrategy
	}{
		{"override buildpacks en node", domain.Project{BuildStrategy: domain.BuildStrategyBuildpacks}, domain.FrameworkNode, domain.BuildStrategyBuildpacks},
		{"override dockerfile en detect", domain.Project{BuildStrategy: domain.BuildStrategyDockerfile}, domain.FrameworkDetect, domain.BuildStrategyDockerfile},
		{"auto + docker => dockerfile", domain.Project{}, domain.FrameworkDocker, domain.BuildStrategyDockerfile},
		{"auto + node => dockerfile", domain.Project{}, domain.FrameworkNode, domain.BuildStrategyDockerfile},
		{"auto + go => dockerfile", domain.Project{}, domain.FrameworkGo, domain.BuildStrategyDockerfile},
		{"auto + astro => dockerfile", domain.Project{}, domain.FrameworkAstro, domain.BuildStrategyDockerfile},
		{"auto + static => dockerfile", domain.Project{}, domain.FrameworkStatic, domain.BuildStrategyDockerfile},
		{"auto + detect => buildpacks", domain.Project{}, domain.FrameworkDetect, domain.BuildStrategyBuildpacks},
	}
	for _, c := range cases {
		if got := ResolveBuildStrategy(c.p, c.fw); got != c.want {
			t.Errorf("%s: ResolveBuildStrategy = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetectScriptCoversFrameworks(t *testing.T) {
	for _, tok := range []string{"astro", "go.mod", "package.json", "Dockerfile", "index.html"} {
		if !strings.Contains(DetectScript, tok) {
			t.Errorf("DetectScript no cubre %q", tok)
		}
	}
}

// TestAnalyticsInjectStepEjecutandolo comprueba el paso de cableado de Sytue
// Analytics EJECUTÁNDOLO sobre ficheros reales, no leyendo la cadena.
//
// Leer la cadena no verifica nada de lo que puede salir mal aquí: que la tubería
// se coma un fichero con espacios, que duplique la etiqueta en quien ya la tiene
// —dos pageviews por visita— o que un directorio sin HTML rompa la build. Lo que
// se prueba es exactamente lo que correrá dentro del contenedor de build.
func TestAnalyticsInjectStepEjecutandolo(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "sub carpeta"), 0o755); err != nil {
		t.Fatal(err)
	}
	ficheros := map[string]string{
		// El caso normal, y el mismo con el nombre que parte una tubería mal escrita.
		"index.html":            "<html><head><title>a</title></head><body>a</body></html>",
		"sub carpeta/otra.html": "<head></head>",
		"ya-medida.html":        "<head>" + AnalyticsTag + "</head>",
		"sin-head.html":         "<body>solo cuerpo</body>",
		"no-es-html.txt":        "</head>",
	}
	for nombre, cuerpo := range ficheros {
		if err := os.WriteFile(filepath.Join(dir, nombre), []byte(cuerpo), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Dos pasadas: la segunda es la que demuestra que reconstruir un proyecto no
	// duplica la etiqueta (el build corre entero en cada despliegue).
	for vuelta := 1; vuelta <= 2; vuelta++ {
		cmd := exec.Command("sh", "-c", analyticsInjectCmd(dir))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("vuelta %d: el paso falló: %v\n%s", vuelta, err, out)
		}
	}

	quiere := map[string]int{
		"index.html":            1,
		"sub carpeta/otra.html": 1,
		"ya-medida.html":        1, // ya la traía: ni se duplica ni se quita
		"sin-head.html":         0, // HTML legal sin </head>: se queda sin medir, no rompe
		"no-es-html.txt":        0, // --include acota a *.html
	}
	for nombre, n := range quiere {
		raw, err := os.ReadFile(filepath.Join(dir, nombre))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Count(string(raw), AnalyticsTag); got != n {
			t.Errorf("%s: %d etiquetas de Analytics, quiero %d (%s)", nombre, got, n, raw)
		}
	}

	// Un directorio sin un solo HTML no puede tumbar la build de nadie.
	cmd := exec.Command("sh", "-c", analyticsInjectCmd(t.TempDir()))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("directorio sin HTML: %v\n%s", err, out)
	}

	// `output_dir` lo escribe el cliente: uno con un espacio no puede romper la
	// build, y uno con una comilla no puede ejecutar nada.
	raro := filepath.Join(t.TempDir(), "mi salida")
	if err := os.MkdirAll(raro, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(raro, "i.html"), []byte("<head></head>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", "-c", analyticsInjectCmd(raro)).CombinedOutput(); err != nil {
		t.Errorf("directorio con espacio: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(raro, "i.html"))
	if err != nil || !strings.Contains(string(raw), AnalyticsTag) {
		t.Errorf("directorio con espacio: no se cableó (%s, %v)", raw, err)
	}
	testigo := filepath.Join(t.TempDir(), "ejecutado")
	inyeccion := "d'; touch " + testigo + "; echo '"
	_ = exec.Command("sh", "-c", analyticsInjectCmd(inyeccion)).Run() // el find fallará: no existe
	if _, err := os.Stat(testigo); err == nil {
		t.Errorf("un output_dir con comillas ejecutó un comando: %s", inyeccion)
	}

	// El paso viaja dentro del Dockerfile de los tres frameworks que Sytue Cloud
	// construye para servir HTML. Si alguien lo saca de ahí, el cableado deja de
	// ser automático sin que nada más se rompa.
	for _, fw := range []domain.Framework{domain.FrameworkAstro, domain.FrameworkStatic, domain.FrameworkNode} {
		df, err := GenerateDockerfile(fw, domain.Project{})
		if err != nil {
			t.Fatalf("GenerateDockerfile(%s): %v", fw, err)
		}
		if !strings.Contains(df, AnalyticsTag) {
			t.Errorf("el Dockerfile de %s no cablea Sytue Analytics:\n%s", fw, df)
		}
	}
}
