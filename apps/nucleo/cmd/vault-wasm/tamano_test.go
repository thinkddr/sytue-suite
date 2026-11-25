// SPDX-License-Identifier: Apache-2.0
//go:build !js

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// presupuestoBoveda es lo que puede pesar el núcleo de la bóveda (`vault`), que toda
// app con bóveda descarga antes de pintar nada. Medido el 2026-11-24 con
// `scripts/client-core-assets.sh` (go1.27.4, -trimpath -ldflags "-s -w"): 7.743.303
// bytes, frente a los 11.343.873 del núcleo completo de antes de partirlo. El margen
// (~8 %) deja crecer el código de la bóveda, no volver a meter el SDK de Matrix: eso
// son 2,7 MB de golpe (docs/decisions.md#0086).
const presupuestoBoveda = 8 << 20

// Paquetes que el núcleo de la bóveda no puede enlazar. Cualquiera de ellos devuelve
// el peso del núcleo completo: el paquete raíz de mautrix trae su cliente HTTP y
// zerolog, y net/http trae TLS y http2.
var vetadosEnBoveda = []string{
	"maunium.net/go/mautrix",
	"maunium.net/go/mautrix/event",
	"github.com/rs/zerolog",
	"net/http",
	"crypto/tls",
	"git.sytue.eu/sytue/one/apps/nucleo/webport",
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof",
}

func wasmEnv() []string {
	return append(os.Environ(), "CGO_ENABLED=0", "GOOS=js", "GOARCH=wasm", "GOFLAGS=")
}

func TestElNucleoDeBovedaNoArrastraMatrix(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Env = wasmEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	deps := strings.Fields(string(out))
	for _, vetado := range vetadosEnBoveda {
		if slices.Contains(deps, vetado) {
			t.Errorf("el núcleo de la bóveda enlaza %s; su sitio es el núcleo completo (cmd/pqc-wasm)", vetado)
		}
	}
}

func TestElNucleoDeBovedaCabeEnSuPresupuesto(t *testing.T) {
	if testing.Short() {
		t.Skip("compila el módulo wasm")
	}
	salida := filepath.Join(t.TempDir(), "core.wasm")
	// Las mismas opciones que scripts/client-core-assets.sh: es lo que se sirve.
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-s -w", "-o", salida, ".")
	cmd.Env = wasmEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	info, err := os.Stat(salida)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > presupuestoBoveda {
		t.Fatalf("el núcleo de la bóveda pesa %d bytes y su presupuesto es %d: algo pesado ha entrado en cmd/internal/kernelops", info.Size(), presupuestoBoveda)
	}
	t.Logf("núcleo de la bóveda: %d bytes de %d", info.Size(), presupuestoBoveda)
}
