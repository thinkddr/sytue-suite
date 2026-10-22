// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// Un push que no toca el subdirectorio del proyecto no puede lanzar una
// compilación: en un monorepo con seis proyectos, tocar la documentación
// dispararía seis builds que producen las mismas imágenes que ya había.
//
// Y al revés importa MÁS: equivocarse hacia «no compiles» significa que un cambio
// real no llega a producción y nadie se entera. Por eso los dos casos de duda
// —sin directorio configurado, sin lista de rutas— devuelven true.
func TestGitEventTouchesDir(t *testing.T) {
	t.Parallel()
	ev := GitEvent{ChangedPaths: []string{"apps/web/src/index.ts", "README.md"}}
	casos := map[string]bool{
		"apps/web":       true,  // el suyo
		"apps/web/":      true,  // con barra final, misma cosa
		"/apps/web":      true,  // con barra inicial, ídem
		"apps/api":       false, // otro proyecto del monorepo
		"apps/web-admin": false, // NO es prefijo de cadena: es prefijo de RUTA
		"":               true,  // el proyecto es el repo entero
	}
	for dir, quiero := range casos {
		if got := ev.TouchesDir(dir); got != quiero {
			t.Errorf("TouchesDir(%q) = %v, quiero %v", dir, got, quiero)
		}
	}

	// Y al revés: un vecino con nombre más largo tampoco cuela en el proyecto
	// corto. Con una comparación de cadenas a secas, «apps/web-admin/x.ts»
	// empezaría por «apps/web» y recompilaría el proyecto equivocado.
	vecino := GitEvent{ChangedPaths: []string{"apps/web-admin/x.ts"}}
	if vecino.TouchesDir("apps/web") {
		t.Error("apps/web-admin no está dentro de apps/web")
	}

	// Sin rutas en el payload no se sabe: se despliega (fail-open).
	sinRutas := GitEvent{}
	if !sinRutas.TouchesDir("apps/web") {
		t.Error("sin lista de rutas hay que desplegar: no saber no es saber que no")
	}
}
