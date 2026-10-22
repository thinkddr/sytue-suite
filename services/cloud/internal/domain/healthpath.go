// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

// ---------------------------------------------------------------------------
// Detección de la ruta de salud
// ---------------------------------------------------------------------------
//
// La ruta de salud NO la elige la plataforma: la implementa el código del cliente.
// Por eso aquí no se «recomienda» ni se inventa nada —una ruta propuesta que la app
// no sirva devuelve 404, y entonces la sonda da el contenedor por muerto y lo
// reinicia en bucle—. Lo que se hace es SONDEAR la app que ya está corriendo con
// las rutas de convención y quedarse con las que responden de verdad.
//
// Es la diferencia entre sugerir y comprobar: lo que sale de aquí está medido.

// HealthPathCandidates son las rutas que se prueban, en el orden en que se
// prefieren. El criterio del orden es la INTENCIÓN de cada una:
//
//  1. Las que existen SOLO para esto (`/healthz` es la convención de Kubernetes;
//     `/livez` y `/readyz`, las del propio kube-apiserver). Si una responde, es una
//     respuesta deliberada del programador: dice «estoy sano», no «he servido algo».
//  2. Las mismas bajo un prefijo de API, habituales en backends con router montado.
//  3. La raíz, la ÚLTIMA. Casi siempre responde 200, así que como sonda es la peor:
//     puede estar sirviendo la portada con la base de datos caída. Solo se propone
//     cuando no hay nada mejor, y la interfaz lo dice.
var HealthPathCandidates = []string{
	"/healthz",
	"/health",
	"/livez",
	"/readyz",
	"/_health",
	"/api/health",
	"/api/healthz",
	"/",
}

// HealthProbeResult es lo que devolvió UNA ruta al sondearla.
type HealthProbeResult struct {
	Path string `json:"path"`
	// Status es el código HTTP, o 0 si no hubo respuesta (red, timeout, TLS).
	Status int `json:"status"`
	// Healthy = respondió 2xx. Un 3xx NO cuenta: una redirección al login es una
	// app que arranca, no una app sana, y la sonda de Kubernetes tampoco lo
	// aceptaría en la ruta que se está eligiendo.
	Healthy bool `json:"healthy"`
	// LatencyMs es lo que tardó en responder.
	LatencyMs int `json:"latency_ms"`
}

// RecommendHealthPath elige la ruta a proponer entre las sondeadas: la primera
// SANA en el orden de preferencia del catálogo. Devuelve "" si ninguna respondió,
// que es el caso en el que hay que callarse y decir que no se pudo comprobar en vez
// de proponer algo que rompería la app.
func RecommendHealthPath(results []HealthProbeResult) string {
	healthy := make(map[string]bool, len(results))
	for _, r := range results {
		if r.Healthy {
			healthy[r.Path] = true
		}
	}
	for _, candidate := range HealthPathCandidates {
		if healthy[candidate] {
			return candidate
		}
	}
	return ""
}

// IsWeakHealthPath marca las rutas que sirven pero son mala señal de salud. La raíz
// responde en casi cualquier app —también en una que tenga la base de datos caída—,
// así que se puede usar, pero quien la elija debe saber lo que está eligiendo.
func IsWeakHealthPath(path string) bool {
	return path == "/"
}
