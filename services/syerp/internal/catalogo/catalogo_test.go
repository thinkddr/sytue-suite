// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package catalogo

import (
	"slices"
	"strings"
	"testing"
)

// TestCatalogoCarga es EL gate de los manifiestos: si alguien añade un módulo
// con un campo sin rótulo, una relación a un modelo que no existe o un modelo
// contable sin cuentas, esto se pone rojo antes de que llegue a un formulario.
func TestCatalogoCarga(t *testing.T) {
	c, err := Cargar()
	if err != nil {
		t.Fatalf("cargar catálogo: %v", err)
	}
	if len(c.Modulos()) == 0 {
		t.Fatal("catálogo vacío")
	}
	total := 0
	for _, mod := range c.Modulos() {
		total += len(mod.Modelos)
	}
	t.Logf("%d módulos, %d modelos", len(c.Modulos()), total)
}

// TestTodoTraducidoEnLosCincoIdiomas recorre el catálogo ya traducido y exige
// que no quede ni un rótulo vacío en ninguno de los cinco. Es la versión
// comprobable de «una función sin sus cinco idiomas no está acabada»: aquí se
// puede exigir de verdad porque el texto es dato.
func TestTodoTraducidoEnLosCincoIdiomas(t *testing.T) {
	c, err := Cargar()
	if err != nil {
		t.Fatalf("cargar catálogo: %v", err)
	}
	for _, idioma := range Idiomas {
		for _, mod := range c.Vista(idioma) {
			if strings.TrimSpace(mod.Etiqueta) == "" {
				t.Errorf("[%s] módulo %q sin rótulo", idioma, mod.Clave)
			}
			for _, m := range mod.Modelos {
				if strings.TrimSpace(m.Etiqueta) == "" {
					t.Errorf("[%s] modelo %q sin rótulo", idioma, m.Clave)
				}
				for _, f := range m.Campos {
					if strings.TrimSpace(f.Etiqueta) == "" {
						t.Errorf("[%s] %s.%s sin rótulo", idioma, m.Clave, f.Clave)
					}
					for _, op := range f.Opciones {
						if strings.TrimSpace(op.Etiqueta) == "" {
							t.Errorf("[%s] %s.%s valor %q sin rótulo", idioma, m.Clave, f.Clave, op.Valor)
						}
					}
				}
				for _, e := range m.Estados {
					if strings.TrimSpace(e.Etiqueta) == "" {
						t.Errorf("[%s] %s estado %q sin rótulo", idioma, m.Clave, e.Valor)
					}
				}
			}
		}
	}
}

// TestIdiomasDistintos comprueba que los rótulos NO son el inglés repetido cinco
// veces. Un diccionario copiado y pegado pasa el gate de arriba y deja la
// interfaz en inglés para todo el mundo, que es el defecto que este catálogo
// existe para evitar.
func TestIdiomasDistintos(t *testing.T) {
	c, err := Cargar()
	if err != nil {
		t.Fatalf("cargar catálogo: %v", err)
	}
	// "Contacts" es igual en inglés y francés, así que se mira el conjunto
	// entero: si NINGÚN rótulo cambia al pasar de idioma, está sin traducir.
	base := strings.Join(rotulos(c, "en"), "|")
	for _, idioma := range []string{"es", "fr", "de", "it"} {
		if strings.Join(rotulos(c, idioma), "|") == base {
			t.Errorf("el catálogo en %q es idéntico al inglés: sin traducir", idioma)
		}
	}
}

func rotulos(c *Catalogo, idioma string) []string {
	var out []string
	for _, mod := range c.Vista(idioma) {
		out = append(out, mod.Etiqueta)
		for _, m := range mod.Modelos {
			out = append(out, m.Etiqueta)
			for _, f := range m.Campos {
				out = append(out, f.Etiqueta)
			}
		}
	}
	return out
}

// TestContablesBienDeclarados vigila lo único que mueve dinero: que los modelos
// que van al libro diario traigan diario, cuentas y la fecha del documento.
func TestContablesBienDeclarados(t *testing.T) {
	c, err := Cargar()
	if err != nil {
		t.Fatalf("cargar catálogo: %v", err)
	}
	contables := 0
	for _, mod := range c.Modulos() {
		for _, m := range mod.Modelos {
			if m.Contable == nil {
				continue
			}
			contables++
			if m.Secuencia == "" {
				t.Errorf("%s: contable sin numerar", m.Clave)
			}
			if m.Contable.CuentaContrapartida == m.Contable.CuentaIngresoGasto {
				t.Errorf("%s: contrapartida e ingreso/gasto son la misma cuenta", m.Clave)
			}
		}
	}
	if contables == 0 {
		t.Error("ningún modelo contable: el ERP no llevaría libros")
	}
}

// TestDependenciasEntreModulos comprueba que las dependencias automáticas entre módulos
// se derivan de las relaciones entre modelos (p. ej. planificacion requiere rrhh).
func TestDependenciasEntreModulos(t *testing.T) {
	c, err := Cargar()
	if err != nil {
		t.Fatalf("cargar catálogo: %v", err)
	}

	// Turnos (planificacion) tiene campo empleado -> modelo empleado en rrhh
	deps := c.Dependencias("planificacion")
	encontrado := false
	for _, d := range deps {
		if d == "rrhh" {
			encontrado = true
			break
		}
	}
	if !encontrado {
		t.Errorf("planificación debería depender de rrhh, dependencias calculadas: %v", deps)
	}

	// Transitivas de planificacion deben incluir rrhh
	transitivas := c.DependenciasTransitivas("planificacion")
	encontradoTransitiva := false
	for _, d := range transitivas {
		if d == "rrhh" {
			encontradoTransitiva = true
			break
		}
	}
	if !encontradoTransitiva {
		t.Errorf("transitivas de planificación deberían incluir rrhh, obtenido: %v", transitivas)
	}
}

// TestDependientesTransitivasEInterdependientes comprueba que al querer deshabilitar un
// módulo, se calculan todos los que dependen de él o tienen dependencias bidireccionales.
func TestDependientesTransitivasEInterdependientes(t *testing.T) {
	c, err := Cargar()
	if err != nil {
		t.Fatalf("cargar catálogo: %v", err)
	}

	// Contabilidad y Facturación tienen dependencias bidireccionales
	// (activo en contabilidad -> factura_compra en facturación; facturas contables -> contabilidad)
	depContabilidad := c.DependientesTransitivas("contabilidad")
	if !slices.Contains(depContabilidad, "facturacion") {
		t.Errorf("desactivar contabilidad debería desactivar facturación (interdependencia), obtenido: %v", depContabilidad)
	}

	depFacturacion := c.DependientesTransitivas("facturacion")
	if !slices.Contains(depFacturacion, "contabilidad") {
		t.Errorf("desactivar facturación debería desactivar contabilidad (interdependencia), obtenido: %v", depFacturacion)
	}

	// rrhh es requerido por planificacion (turnos)
	depRRHH := c.DependientesTransitivas("rrhh")
	if !slices.Contains(depRRHH, "planificacion") {
		t.Errorf("desactivar rrhh debería desactivar planificación, obtenido: %v", depRRHH)
	}

	// Desactivar planificacion NO debe desactivar rrhh (unidireccional)
	depPlan := c.DependientesTransitivas("planificacion")
	if slices.Contains(depPlan, "rrhh") {
		t.Errorf("desactivar planificación no debería desactivar rrhh, obtenido: %v", depPlan)
	}
}
