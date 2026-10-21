// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package permisos

import (
	"slices"
	"testing"
)

func TestNivel(t *testing.T) {
	t.Parallel()
	puestos := map[string]string{"crm": Lectura, "contabilidad": Usuario, "ventas": "inventado"}
	for _, c := range []struct {
		roles  []string
		modulo string
		quiero string
	}{
		{[]string{"owner"}, "nominas", Administrador},
		{[]string{"member", "admin"}, Obligaciones, Administrador},
		{[]string{"member"}, "crm", Lectura},
		{[]string{"member"}, "contabilidad", Usuario},
		{[]string{"member"}, "nominas", Ninguno},
		{[]string{"member"}, Obligaciones, Ninguno},
		{[]string{"member"}, "proyectos", Usuario},
		// Un nivel que no existe no da ni quita: manda el defecto.
		{[]string{"member"}, "ventas", Usuario},
	} {
		if got := Nivel(c.roles, puestos, c.modulo); got != c.quiero {
			t.Errorf("Nivel(%v, %s) = %s, quiero %s", c.roles, c.modulo, got, c.quiero)
		}
	}
	if !Alcanza(Administrador, Lectura) || Alcanza(Lectura, Usuario) || !Alcanza(Usuario, Usuario) || Alcanza("", Lectura) {
		t.Error("el orden de los niveles no se respeta")
	}
}

// TestVerbosSeparanFunciones comprueba lo único que justifica que los verbos
// existan: que una lista puesta a mano rompe la escalera del nivel —se puede
// crear sin poder asentar— y que vincula también a la administración de la
// organización, que es de donde se escapaba antes.
func TestVerbosSeparanFunciones(t *testing.T) {
	t.Parallel()
	puestos := map[string]string{"facturacion": Usuario}

	// Sin listas, todo sigue como antes: el nivel implica los verbos.
	if !PuedeVerbo([]string{"member"}, puestos, nil, "facturacion", Asentar) {
		t.Error("sin lista, «usuario» tiene que seguir pudiendo asentar (nada cambia al desplegar)")
	}
	if PuedeVerbo([]string{"member"}, puestos, nil, "facturacion", Aprobar) {
		t.Error("aprobar no lo implica «usuario»")
	}

	// Con lista: crea pero no asienta. Esto es imposible con niveles solos.
	listas := map[string][]string{"facturacion": {Crear, Auditar}}
	if !PuedeVerbo([]string{"member"}, puestos, listas, "facturacion", Crear) {
		t.Error("la lista concede crear")
	}
	if PuedeVerbo([]string{"member"}, puestos, listas, "facturacion", Asentar) {
		t.Error("la lista NO concede asentar y el nivel no debe colarlo")
	}

	// Y ata al owner, que es lo que hace que la separación sirva de algo.
	if PuedeVerbo([]string{"owner"}, nil, listas, "facturacion", Asentar) {
		t.Error("una lista puesta también vincula al owner")
	}
	if !PuedeVerbo([]string{"owner"}, nil, nil, "facturacion", Asentar) {
		t.Error("sin lista el owner sigue pudiendo todo")
	}

	// Una lista VACÍA no es «no hay lista»: retira todos los gestos.
	vacia := map[string][]string{"facturacion": {}}
	for _, v := range Verbos {
		if PuedeVerbo([]string{"owner"}, nil, vacia, "facturacion", v) {
			t.Errorf("una lista vacía no concede %q", v)
		}
	}

	if d := VerbosPorDefecto(Lectura); !slices.Equal(d, []string{Auditar}) {
		t.Errorf("VerbosPorDefecto(lectura) = %v, quiero solo auditar", d)
	}
	if VerboValido("inventado") || !VerboValido(Pagar) {
		t.Error("el catálogo de verbos no filtra")
	}
}
