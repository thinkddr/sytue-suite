// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// La llamada local de ci.yml usa el mismo commit; las acciones remotas siguen
// obligadas a usar la whitelist y los SHA de versions.env. Ejercitar la función
// real evita mantener una segunda versión del clasificador en el guardián.

// toolchainWorkflowUsesGuion carga `fail`, `env_val` y el clasificador real de
// scripts/check-toolchain-versions.sh y ejecuta lo que se le pase: el
// clasificador sobre un directorio, o env_val con una variable.
const toolchainWorkflowUsesGuion = `set -euo pipefail
ROOT="$PWD"
funciones="$(sed -n '/^fail() {$/,/^}$/p; /^env_val() {/p; /^comprobar_usos_workflows() {$/,/^}$/p' scripts/check-toolchain-versions.sh)"
grep -F 'comprobar_usos_workflows() {' <<<"$funciones" >/dev/null || exit 97
eval "$funciones"
"$@"
`

// toolchainWorkflowUsesBanco es un repositorio de mentira con un workflow que
// llama (parent.yml) y otro llamado (child.yml).
type toolchainWorkflowUsesBanco struct {
	t       *testing.T
	fixture string
	flujos  string
	casos   int
}

func (b *toolchainWorkflowUsesBanco) real(args ...string) guiontest.Resultado {
	b.t.Helper()
	r := guiontest.Bash(b.t, guiontest.Opciones{}, toolchainWorkflowUsesGuion, args...)
	if r.Codigo == 97 {
		b.t.Fatal("no se encontró el clasificador real de uses")
	}
	return r
}

// envVal lee una variable de versions.env con el env_val real.
func (b *toolchainWorkflowUsesBanco) envVal(variable string) string {
	b.t.Helper()
	r := b.real("env_val", variable)
	if r.Codigo != 0 || r.Texto() == "" {
		b.t.Fatalf("versions.env no declara %s: %s", variable, r.Todo())
	}
	return r.Texto()
}

func (b *toolchainWorkflowUsesBanco) git(args ...string) {
	b.t.Helper()
	if r := guiontest.Ejecutar(b.t, guiontest.Opciones{Dir: b.fixture}, "git", args...); r.Codigo != 0 {
		b.t.Fatalf("git %v: %s", args, r.Todo())
	}
}

func (b *toolchainWorkflowUsesBanco) escribir(nombre, contenido string) {
	b.t.Helper()
	guiontest.Escribir(b.t, filepath.Join(b.flujos, nombre), contenido)
}

func (b *toolchainWorkflowUsesBanco) borrar(nombre string) {
	b.t.Helper()
	if err := os.Remove(filepath.Join(b.flujos, nombre)); err != nil {
		b.t.Fatal(err)
	}
}

// llamar deja en parent.yml un job que usa esa referencia.
func (b *toolchainWorkflowUsesBanco) llamar(uses string) {
	b.t.Helper()
	b.escribir("parent.yml", "on: workflow_dispatch\npermissions:\n  contents: read\njobs:\n  child:\n    uses: "+uses+"\n")
}

// declarar deja en child.yml un workflow con ese evento bajo `on:`.
func (b *toolchainWorkflowUsesBanco) declarar(evento string) {
	b.t.Helper()
	b.escribir("child.yml", "on:\n  "+evento+
		"\npermissions:\n  contents: read\njobs:\n  demo:\n    runs-on: sytue-arm64\n    steps:\n      - run: echo fixture\n")
}

// bien: el clasificador tiene que aceptar el banco tal como está.
func (b *toolchainWorkflowUsesBanco) bien(que string) {
	b.t.Helper()
	b.casos++
	r := b.real("comprobar_usos_workflows", b.fixture)
	b.t.Run(que, func(t *testing.T) {
		if r.Codigo != 0 {
			t.Fatalf("%s: el clasificador lo rechazó: %s", que, r.Todo())
		}
	})
}

// mal: el clasificador tiene que rechazarlo, y por ese motivo.
func (b *toolchainWorkflowUsesBanco) mal(que, motivo string) {
	b.t.Helper()
	b.casos++
	r := b.real("comprobar_usos_workflows", b.fixture)
	b.t.Run(que, func(t *testing.T) {
		if r.Codigo == 0 {
			t.Fatalf("el clasificador aceptó %s", que)
		}
		if !strings.Contains(r.Todo(), motivo) {
			t.Fatalf("%s falló por otro motivo: %s", que, r.Todo())
		}
	})
}

// Los casos comparten el banco y van en el orden del guion original: cada uno
// parte de cómo dejó los ficheros el anterior.
func TestToolchainWorkflowUsesLocalesYAccionesRemotas(t *testing.T) {
	fixture := t.TempDir()
	b := &toolchainWorkflowUsesBanco{t: t, fixture: fixture, flujos: filepath.Join(fixture, ".github", "workflows")}
	if err := os.MkdirAll(b.flujos, 0o750); err != nil {
		t.Fatal(err)
	}
	b.git("init", "-q")

	b.declarar("workflow_call:")
	b.llamar("./.github/workflows/child.yml")
	b.git("add", ".github")
	b.bien("workflow local versionado con workflow_call nulo")
	b.llamar(`"./.github/workflows/child.yml"`)
	b.bien("workflow local entre comillas dobles")
	b.llamar(`'./.github/workflows/child.yml'`)
	b.bien("workflow local entre comillas simples")
	b.llamar("./.github/workflows/child.yml")
	b.declarar("workflow_call: {inputs: {verbose: {type: boolean, default: false}}}")
	b.bien("workflow local con interfaz workflow_call")
	b.escribir("child.yml", "on: workflow_call\n")
	b.bien("evento workflow_call escalar")
	b.escribir("child.yml", "on: [workflow_call, workflow_dispatch]\n")
	b.bien("lista de eventos con workflow_call")
	b.escribir("child.yaml", "on: [workflow_call, workflow_dispatch]\n")
	b.git("add", ".github/workflows/child.yaml")
	b.llamar("./.github/workflows/child.yaml")
	b.bien("workflow local con extensión yaml")

	b.llamar("./.github/workflows/absent.yml")
	b.mal("workflow ausente", "no existe")
	b.escribir("untracked.yml", "on: [workflow_call, workflow_dispatch]\n")
	b.llamar("./.github/workflows/untracked.yml")
	b.mal("workflow fuera del commit", "no está versionado")
	b.borrar("untracked.yml")
	b.llamar("./.github/workflows/child.yml")
	b.declarar("workflow_dispatch:")
	b.mal("workflow sin workflow_call", "workflow_call válido")
	b.declarar("workflow_call: false")
	b.mal("workflow_call con tipo inválido", "workflow_call válido")
	b.escribir("child.yml", "on: workflow_call\non: workflow_call\n")
	b.mal("YAML con claves duplicadas", "workflow_call válido")
	b.declarar("workflow_call:")
	b.escribir("child.yml", guiontest.Leer(t, filepath.Join(b.flujos, "child.yml"))+"\n---\non: workflow_call\n")
	b.mal("más de un documento YAML", "workflow_call válido")
	b.declarar("workflow_call:")

	for _, referencia := range []string{
		"./.github/workflows/../workflows/child.yml",
		"./scripts/child.yml",
		`"./.github/workflows/ch\"ild.yml"`,
		"./.github/workflows/child.yml@main",
		"./.github/workflows/child.yml@0123456789abcdef0123456789abcdef01234567",
	} {
		b.llamar(referencia)
		b.mal(referencia, "ruta de workflow local no permitida")
	}
	if err := os.Symlink(filepath.Join(guiontest.Raiz(t), ".github", "workflows", "ci-go.yml"), filepath.Join(b.flujos, "link.yml")); err != nil {
		t.Fatal(err)
	}
	b.git("add", ".github/workflows/link.yml")
	b.llamar("./.github/workflows/link.yml")
	b.mal("symlink a otro árbol", "enlace simbólico")
	b.borrar("link.yml")

	checkout := b.envVal("ACTION_CHECKOUT_SHA")
	b.llamar("actions/checkout@" + checkout)
	b.bien("acción checkout fijada por SHA")
	b.llamar("actions/upload-artifact@" + b.envVal("ACTION_UPLOAD_ARTIFACT_SHA"))
	b.bien("acción upload-artifact fijada por SHA")
	for _, accion := range []string{"restore", "save"} {
		b.llamar("actions/cache/" + accion + "@" + b.envVal("ACTION_CACHE_SHA"))
		b.bien("acción cache/" + accion + " fijada por SHA")
	}
	for _, referencia := range []string{"actions/checkout@v7", "actions/checkout@latest"} {
		b.llamar(referencia)
		b.mal(referencia, "no va fijada por SHA")
	}
	b.llamar("unknown/action@v1 # uses: ./.github/workflows/child.yml")
	b.mal("acción desconocida oculta tras uses en comentario", "acción no prevista")
	b.llamar("actions/checkout@v7 # uses: ./.github/workflows/child.yml")
	b.mal("tag externo oculto tras uses en comentario", "no va fijada por SHA")
	b.llamar("actions/checkout@" + checkout + " # uses: unknown/action@v1")
	b.bien("un comentario no cambia el uses real")
	b.llamar("actions/checkout@0000000000000000000000000000000000000000")
	b.mal("SHA no fijado en versions.env", "no coincide con ACTION_CHECKOUT_SHA")
	b.llamar("unknown/action@0000000000000000000000000000000000000000")
	b.mal("acción ajena a la whitelist", "acción no prevista")

	if b.casos != 30 {
		t.Fatalf("se ejercitaron %d casos y el guion original tenía 30", b.casos)
	}
}
