// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// ultimogo comprueba que el repositorio usa la ÚLTIMA versión estable de Go
// (CONVENTIONS §3: siempre la última estable). Sale con 1 si `go.mod` va por
// detrás de la última publicada, y con 2 si no se pudo saber: callar un fallo de
// red lo convertiría en «estamos al día».
//
// Pregunta al proxy de módulos por `golang.org/toolchain`, que es de donde el
// propio `go` baja sus toolchains: no añade ningún origen a la cadena de
// suministro. Las rc y las betas no cuentan.
//
//	go run ./tools/ultimogo            # desde la raíz del repositorio
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	estable   = regexp.MustCompile(`^v0\.0\.1-go(\d+)\.(\d+)\.(\d+)\.[a-z0-9]+-[a-z0-9]+$`)
	directiva = regexp.MustCompile(`(?m)^go (\d+)\.(\d+)\.(\d+)$`)
)

type version [3]int

func (v version) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }

func (v version) menor(o version) bool {
	for i := range v {
		if v[i] != o[i] {
			return v[i] < o[i]
		}
	}
	return false
}

func numeros(m []string) version {
	var v version
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v
}

// ultimaEstable es la mayor versión estable de la lista de `golang.org/toolchain`
// (`v0.0.1-go1.27.4.linux-amd64 …`). Ignora rc, betas y lo que no reconoce.
func ultimaEstable(lista string) (version, bool) {
	var mejor version
	hay := false
	for _, campo := range strings.Fields(lista) {
		m := estable.FindStringSubmatch(campo)
		if m == nil {
			continue
		}
		if v := numeros(m); !hay || mejor.menor(v) {
			mejor, hay = v, true
		}
	}
	return mejor, hay
}

// versionDeGoMod es la directiva `go X.Y.Z` de un go.mod.
func versionDeGoMod(gomod string) (version, bool) {
	m := directiva.FindStringSubmatch(gomod)
	if m == nil {
		return version{}, false
	}
	return numeros(m), true
}

// listaDeToolchains pregunta al proxy de módulos con el propio `go`.
func listaDeToolchains() ([]byte, error) {
	ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelar()
	return exec.CommandContext(ctx, "go", "list", "-m", "-versions", "golang.org/toolchain").Output()
}

func main() {
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ultimogo: %v\n", err)
		os.Exit(2)
	}
	usamos, ok := versionDeGoMod(string(gomod))
	if !ok {
		fmt.Fprintln(os.Stderr, "ultimogo: go.mod no fija una versión completa de Go (go X.Y.Z)")
		os.Exit(2)
	}
	lista, err := listaDeToolchains()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ultimogo: no se pudo consultar el proxy de módulos: %v\n", err)
		os.Exit(2)
	}
	ultima, ok := ultimaEstable(string(lista))
	if !ok {
		fmt.Fprintln(os.Stderr, "ultimogo: el proxy no listó ninguna versión estable de Go")
		os.Exit(2)
	}
	if usamos.menor(ultima) {
		fmt.Fprintf(os.Stderr, "ultimogo: hay Go %s publicado y el repositorio sigue en %s — súbelo en go.mod, los Dockerfile y CONVENTIONS §3 (make toolchain-check dice dónde)\n", ultima, usamos)
		os.Exit(1)
	}
	fmt.Printf("ultimogo: Go %s es la última estable\n", usamos)
}
