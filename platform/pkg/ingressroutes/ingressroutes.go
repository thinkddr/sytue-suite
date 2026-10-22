// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ingressroutes contrasta las rutas que un servicio monta DE VERDAD con las
// que su chart de Helm publica en el Ingress.
//
// Existe porque el mismo fallo ha ocurrido tres veces: `ingress.paths` es una copia a
// mano del router de chi y nadie las compara. Cuando divergen, la ruta existe, tiene
// tests en verde y aun así no recibe un solo paquete en producción — el `/` del SPA se
// la queda. Le pasó a WebDAV (`/webdav` vs `/dav`), a los enlaces públicos de Drive
// (`/public`) y a los webhooks entrantes de Rooms (`/hooks`).
//
// Uso desde el test del paquete http de cada servicio (ver ingress_test.go):
//
//	routes, _ := NewRouter(Deps{}).(chi.Routes)
//	ingressroutes.AssertCovered(t, routes, "chat", ingressroutes.Internal)
package ingressroutes

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Internal son los prefijos que NO deben publicarse en el Ingress: sondas del kubelet
// y superficie servicio-a-servicio, que viaja por la red del clúster y nunca por
// Internet. Publicarlos sería el fallo contrario al que este paquete vigila.
var Internal = []string{"/healthz", "/readyz", "/metrics", "/internal", "/debug"}

// pathLine casa las entradas `- path: /loquesea` del values.yaml del chart. Se lee con
// una expresión y no con un parser de YAML a propósito: el fichero es nuestro, el
// formato lo fija el chart, y así este paquete no arrastra dependencias al binario.
var pathLine = regexp.MustCompile(`(?m)^\s*-\s*path:\s*(\S+)`)

// AssertCovered falla el test si el router monta algún prefijo de primer nivel que el
// Ingress del chart no publica. `chart` es el nombre del directorio bajo
// deploy/helm/charts. `allowed` son los prefijos deliberadamente NO publicados
// (normalmente Internal, más los que ese servicio justifique).
func AssertCovered(t *testing.T, routes chi.Routes, chart string, allowed []string) {
	t.Helper()

	published, err := chartPaths(chart)
	if err != nil {
		t.Fatalf("ingressroutes: leer el chart %q: %v", chart, err)
	}
	if len(published) == 0 {
		// Un chart que publica "/" (como id o hub) enruta todo el servicio: nada que
		// contrastar. Sin entradas `- path:` no hay nada que comparar tampoco.
		return
	}

	// Se contrasta la RUTA COMPLETA de cada endpoint, no su primer segmento.
	//
	// Antes se comparaba el primer segmento y eso empezó a dar falsos positivos en
	// cuanto un chart publicó algo más fino que `/api`: el commit e5752e9c pasó los
	// charts de `/api` a `/api/v1` —porque el BFF de las apps Astro sirve sus
	// propias rutas bajo /api/<recurso> y un Prefix en /api se las quedaba todas—,
	// y desde entonces cuatro servicios fallaban este test diciendo que «montan
	// /api y el chart no lo publica». Lo publicaba: publicaba /api/v1, que es
	// justo donde el servicio sirve.
	//
	// Comparar la ruta entera también es MÁS estricto, no menos: si mañana alguien
	// monta /api/v2 y el chart solo publica /api/v1, esto lo ve. Con el primer
	// segmento no lo habría visto.
	var missing []string
	seen := map[string]bool{}
	_ = chi.Walk(routes, func(_, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if firstSegment(route) == "" || covered(route, published) || covered(route, allowed) {
			return nil
		}
		if !seen[route] {
			seen[route] = true
			missing = append(missing, route)
		}
		return nil
	})
	sort.Strings(missing)
	if len(missing) == 0 {
		return
	}
	t.Errorf("el servicio monta %v pero deploy/helm/charts/%s/values.yaml no lo publica: en producción "+
		"esas rutas las sirve el SPA (404) aunque los tests pasen.\n"+
		"Publica el prefijo en ingress.paths, o añádelo a la lista de rutas internas del test si es "+
		"deliberado que no salga a Internet.", missing, chart)
}

// Prefixes devuelve los prefijos de PRIMER nivel que el router monta, ordenados y sin
// repetir. `/api/v1/documents/{id}` cuenta como `/api`: el Ingress enruta por prefijo,
// así que es la granularidad que decide si el tráfico llega o no.
func Prefixes(routes chi.Routes) []string {
	seen := map[string]bool{}
	_ = chi.Walk(routes, func(_, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if p := firstSegment(route); p != "" {
			seen[p] = true
		}
		return nil
	})
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// firstSegment reduce una ruta a su primer segmento: "/api/v1/x" => "/api". Devuelve ""
// para la raíz y para los comodines, que no aportan un prefijo enrutable.
func firstSegment(route string) string {
	trimmed := strings.TrimPrefix(route, "/")
	if i := strings.IndexByte(trimmed, '/'); i >= 0 {
		trimmed = trimmed[:i]
	}
	if trimmed == "" || trimmed == "*" || strings.HasPrefix(trimmed, "{") {
		return ""
	}
	return "/" + trimmed
}

// covered indica si `prefix` cae bajo alguno de los prefijos publicados. La comparación
// distingue mayúsculas igual que el matching de rutas del Ingress.
func covered(prefix string, published []string) bool {
	for _, p := range published {
		if p == "/" || p == prefix || strings.HasPrefix(prefix+"/", strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}

// chartPaths extrae los `ingress.paths` del values.yaml del chart indicado.
func chartPaths(chart string) ([]string, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(root, "deploy", "helm", "charts", chart, "values.yaml"))
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, m := range pathLine.FindAllStringSubmatch(string(raw), -1) {
		paths = append(paths, strings.Trim(m[1], `"'`))
	}
	return paths, nil
}

// repoRoot sube desde el directorio del test hasta encontrar el go.mod del módulo.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no se encontró go.mod subiendo desde el directorio del test")
		}
		dir = parent
	}
}
