// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import "testing"

// Lo que se fija aquí es el ESTADO POR DEFECTO, que es el que decide si una
// petición que llega antes de Resolve —o después de que Resolve fallara— asume el
// rol o no.
//
// Tiene que ser «no asumirlo». Al revés, un servicio que arrancara con el rol
// marcado como activo intentaría un SET ROLE que no puede hacer y fallaría en
// todas las consultas, que es exactamente el fallo del que se viene: en
// producción el catálogo se sembró con cero de 32 plantillas por eso.
func TestElRolNoSeAsumeMientrasNoSeCompruebe(t *testing.T) {
	r := NewAppRole("integrations_app")
	if r.Activo() {
		t.Error("el rol se da por asumible sin haberlo comprobado")
	}
	if r.Nombre() != "integrations_app" {
		t.Errorf("nombre inesperado: %q", r.Nombre())
	}
}

// Un AppRole recién creado no comparte estado con otro: dos servicios en el mismo
// proceso (o dos tests seguidos) no pueden contaminarse la decisión.
func TestCadaRolLlevaSuPropiaDecision(t *testing.T) {
	a, b := NewAppRole("docs_app"), NewAppRole("integrations_app")
	a.usar.Store(true)
	if b.Activo() {
		t.Error("la decisión de un rol se filtró a otro")
	}
	if !a.Activo() {
		t.Error("el rol no conservó su propia decisión")
	}
}

// Un AppRole nil NO puede panicar: significa «no hay rol que asumir».
//
// Un Store construido sin su AppRole —un constructor que se quedó sin
// actualizar— hacía que el servicio arrancara y muriera con un nil pointer
// DENTRO de la primera transacción, y el panic señalaba a este paquete en vez de
// al constructor culpable. Le costó a Sytue Docs un despliegue entero: los pods
// nuevos en CrashLoopBackOff y `helm upgrade` agotando su plazo.
func TestUnRolNilNoPanica(t *testing.T) {
	var nulo *AppRole
	if nulo.Activo() {
		t.Error("un AppRole nil se declara activo")
	}
	if _, err := nulo.Resolve(t.Context(), nil); err == nil {
		t.Error("Resolve sobre un AppRole nil debería explicar que falta en el constructor")
	}
}

// Todo Store que asuma un rol lo construye. Es la comprobación que faltaba: la de
// arriba prueba que nil no rompe, esta que no llega a haber ningún nil.
func TestLosConstructoresCreanSuAppRole(t *testing.T) {
	// Comprobado por inspección en el gate de charts (helm-render-test.sh busca
	// `pg.NewAppRole(appRole)` en cada store con `const appRole`), porque aquí no
	// se pueden importar los stores de los servicios sin un ciclo de paquetes.
	t.Skip("lo verifica deploy/production/tests/helm-render-test.sh sobre el código fuente")
}
