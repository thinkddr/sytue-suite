// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

// Matriz ACL: cada fila es (rol, acción, permitido). Está escrita entera, no
// derivada de `minimo`, para que un cambio en la tabla de decisión rompa el test.
func TestMatrizACL(t *testing.T) {
	acciones := []Accion{AccLeer, AccComentar, AccEscribirReg, AccGestionVistas, AccEsquema, AccPermisos, AccAuditoria, AccBorrarBase, AccTransferir, AccConector, AccFormulario, AccRevisarScripts}
	esperado := map[Rol][]bool{
		//                    leer   coment reg    vistas esq    perm   audit  borrar transf conector formul scripts
		RolPropietario:  {true, true, true, true, true, true, true, true, true, true, true, true},
		RolEditor:       {true, true, true, true, false, false, false, false, false, false, false, false},
		RolComentarista: {true, true, false, false, false, false, false, false, false, false, false, false},
		RolLector:       {true, false, false, false, false, false, false, false, false, false, false, false},
		"":              {false, false, false, false, false, false, false, false, false, false, false, false},
		"admin":         {false, false, false, false, false, false, false, false, false, false, false, false},
	}
	for rol, fila := range esperado {
		for i, acc := range acciones {
			if got := Permite(rol, acc); got != fila[i] {
				t.Errorf("Permite(%q, %s) = %v, esperado %v", rol, acc, got, fila[i])
			}
		}
	}
	if Permite(RolPropietario, "inventada") {
		t.Error("una acción desconocida debe negarse")
	}
}

// Borrar un comentario: quien lo escribió (si aún puede comentar) y quien modera.
func TestPuedeBorrarComentario(t *testing.T) {
	casos := []struct {
		rol     Rol
		esAutor bool
		quiero  bool
	}{
		{RolPropietario, false, true},
		{RolPropietario, true, true},
		{RolEditor, false, true},
		{RolEditor, true, true},
		{RolComentarista, true, true},
		{RolComentarista, false, false},
		{RolLector, true, false},
		{RolLector, false, false},
		{"", true, false},
		{"admin", true, false},
	}
	for _, c := range casos {
		if got := PuedeBorrarComentario(c.rol, c.esAutor); got != c.quiero {
			t.Errorf("PuedeBorrarComentario(%q, autor=%v) = %v, esperado %v", c.rol, c.esAutor, got, c.quiero)
		}
	}
}

func TestLimpiarComentario(t *testing.T) {
	if s, err := LimpiarComentario("  hola\r\nmundo \n"); err != nil || s != "hola\nmundo" {
		t.Errorf("recorte: %q %v", s, err)
	}
	for _, malo := range []string{"", "   ", "a\x00b", "a\x1bb", string([]byte{0xff, 0xfe}), strings.Repeat("ñ", MaxLongComentario+1)} {
		if _, err := LimpiarComentario(malo); err == nil {
			t.Errorf("debería rechazar %q", malo)
		}
	}
	if _, err := LimpiarComentario(strings.Repeat("ñ", MaxLongComentario)); err != nil {
		t.Errorf("el máximo justo vale: %v", err)
	}
}

func TestResolverRol(t *testing.T) {
	a := Actor{Sub: "ana", Tenant: "acme", Groups: []string{"ventas"}}
	tabla := "t1"
	casos := []struct {
		nombre  string
		owner   string
		entrada []EntradaACL
		tabla   string
		rol     Rol
		ok      bool
	}{
		{"propietario", "ana", nil, "", RolPropietario, true},
		{"sin entradas: denegado", "otro", nil, "", "", false},
		{"usuario lector", "otro", []EntradaACL{{PrincipalKind: "user", PrincipalID: "ana", Role: RolLector}}, "", RolLector, true},
		{"otro usuario no aplica", "otro", []EntradaACL{{PrincipalKind: "user", PrincipalID: "bea", Role: RolEditor}}, "", "", false},
		{"toda la organización", "otro", []EntradaACL{{PrincipalKind: "tenant", PrincipalID: "acme", Role: RolComentarista}}, "", RolComentarista, true},
		{"otra organización no aplica", "otro", []EntradaACL{{PrincipalKind: "tenant", PrincipalID: "globex", Role: RolEditor}}, "", "", false},
		{"gana el máximo", "otro", []EntradaACL{
			{PrincipalKind: "tenant", PrincipalID: "acme", Role: RolLector},
			{PrincipalKind: "user", PrincipalID: "ana", Role: RolEditor},
		}, "", RolEditor, true},
		{"entrada de tabla suma en esa tabla", "otro", []EntradaACL{
			{PrincipalKind: "tenant", PrincipalID: "acme", Role: RolLector},
			{TableID: tabla, PrincipalKind: "user", PrincipalID: "ana", Role: RolEditor},
		}, tabla, RolEditor, true},
		{"entrada de tabla no sube la base", "otro", []EntradaACL{
			{TableID: tabla, PrincipalKind: "user", PrincipalID: "ana", Role: RolEditor},
		}, "", "", false},
		{"entrada de otra tabla no aplica", "otro", []EntradaACL{
			{TableID: "t2", PrincipalKind: "user", PrincipalID: "ana", Role: RolEditor},
		}, tabla, "", false},
		{"grupo del actor", "otro", []EntradaACL{{PrincipalKind: "group", PrincipalID: "ventas", Role: RolEditor}}, "", RolEditor, true},
		{"grupo que el actor no tiene", "otro", []EntradaACL{{PrincipalKind: "group", PrincipalID: "dirección", Role: RolEditor}}, "", "", false},
		{"un grupo que se llama como su sub no es ella", "otro", []EntradaACL{{PrincipalKind: "group", PrincipalID: "ana", Role: RolEditor}}, "", "", false},
		{"una persona que se llama como su grupo no es el grupo", "otro", []EntradaACL{{PrincipalKind: "user", PrincipalID: "ventas", Role: RolEditor}}, "", "", false},
		{"un grupo que se llama como su organización no es la organización", "otro", []EntradaACL{{PrincipalKind: "group", PrincipalID: "acme", Role: RolEditor}}, "", "", false},
		{"grupo de tabla suma en esa tabla", "otro", []EntradaACL{
			{PrincipalKind: "user", PrincipalID: "ana", Role: RolLector},
			{TableID: tabla, PrincipalKind: "group", PrincipalID: "ventas", Role: RolComentarista},
		}, tabla, RolComentarista, true},
		{"una entrada de fuera no aplica a quien es de la organización", "otro", []EntradaACL{{PrincipalKind: "external", PrincipalID: "ana", Role: RolEditor}}, "", "", false},
		{"clase de principal desconocida", "otro", []EntradaACL{{PrincipalKind: "public", PrincipalID: "ana", Role: RolEditor}}, "", "", false},
		{"owner no se concede por ACL", "otro", []EntradaACL{{PrincipalKind: "user", PrincipalID: "ana", Role: RolPropietario}}, "", "", false},
		{"rol inválido", "otro", []EntradaACL{{PrincipalKind: "user", PrincipalID: "ana", Role: "root"}}, "", "", false},
	}
	for _, c := range casos {
		rol, ok := ResolverRol(c.owner, c.entrada, a, c.tabla)
		if rol != c.rol || ok != c.ok {
			t.Errorf("%s: (%q, %v), esperado (%q, %v)", c.nombre, rol, ok, c.rol, c.ok)
		}
	}
	if _, ok := ResolverRol("", nil, Actor{}, ""); ok {
		t.Error("actor vacío sin propietario no debe tener acceso")
	}
	if _, ok := ResolverRol("", []EntradaACL{{PrincipalKind: "user", PrincipalID: "", Role: RolEditor}}, Actor{}, ""); ok {
		t.Error("un sub vacío no puede coincidir con una entrada vacía")
	}
}

// Un actor sin grupos nunca coincide con una entrada de grupo, tampoco con una vacía.
func TestGrupoVacioNoCoincide(t *testing.T) {
	for _, a := range []Actor{{Sub: "ana", Tenant: "acme"}, {Sub: "ana", Tenant: "acme", Groups: []string{""}}} {
		for _, e := range []EntradaACL{
			{PrincipalKind: "group", PrincipalID: "", Role: RolEditor},
			{PrincipalKind: "group", PrincipalID: "ventas", Role: RolEditor},
		} {
			if _, ok := ResolverRol("otro", []EntradaACL{e}, a, ""); ok {
				t.Errorf("actor %+v no debe coincidir con %+v", a, e)
			}
		}
	}
}

// Quien viene de otra organización solo es lo que dice su entrada `external`: ni
// propietaria aunque su sub coincida, ni «toda la organización», ni un grupo, ni una
// entrada de persona con su mismo sub.
func TestResolverRolDeFuera(t *testing.T) {
	gus := Actor{Sub: "gus", Tenant: "acme", Groups: []string{"ventas"}, Externo: true}
	casos := []struct {
		nombre  string
		owner   string
		entrada []EntradaACL
		tabla   string
		rol     Rol
		ok      bool
	}{
		{"su entrada de fuera", "ana", []EntradaACL{{PrincipalKind: "external", PrincipalID: "gus", Role: RolComentarista}}, "", RolComentarista, true},
		{"la entrada de fuera de otra persona", "ana", []EntradaACL{{PrincipalKind: "external", PrincipalID: "hal", Role: RolEditor}}, "", "", false},
		{"nunca propietaria, aunque el sub coincida", "gus", nil, "", "", false},
		{"propietaria por sub + entrada de fuera: manda la entrada", "gus", []EntradaACL{{PrincipalKind: "external", PrincipalID: "gus", Role: RolLector}}, "", RolLector, true},
		{"toda la organización anfitriona no la incluye", "ana", []EntradaACL{{PrincipalKind: "tenant", PrincipalID: "acme", Role: RolEditor}}, "", "", false},
		{"un grupo de la anfitriona con el nombre de uno suyo", "ana", []EntradaACL{{PrincipalKind: "group", PrincipalID: "ventas", Role: RolEditor}}, "", "", false},
		{"una entrada de persona con su sub", "ana", []EntradaACL{{PrincipalKind: "user", PrincipalID: "gus", Role: RolEditor}}, "", "", false},
		{"no suma lo de la organización a lo suyo", "ana", []EntradaACL{
			{PrincipalKind: "external", PrincipalID: "gus", Role: RolLector},
			{PrincipalKind: "tenant", PrincipalID: "acme", Role: RolEditor},
		}, "", RolLector, true},
		{"de fuera en una tabla", "ana", []EntradaACL{{TableID: "t1", PrincipalKind: "external", PrincipalID: "gus", Role: RolEditor}}, "t1", RolEditor, true},
		{"de fuera en una tabla no abre la base", "ana", []EntradaACL{{TableID: "t1", PrincipalKind: "external", PrincipalID: "gus", Role: RolEditor}}, "", "", false},
		{"propietaria no se concede a nadie de fuera", "ana", []EntradaACL{{PrincipalKind: "external", PrincipalID: "gus", Role: RolPropietario}}, "", "", false},
	}
	for _, c := range casos {
		rol, ok := ResolverRol(c.owner, c.entrada, gus, c.tabla)
		if rol != c.rol || ok != c.ok {
			t.Errorf("%s: (%q, %v), esperado (%q, %v)", c.nombre, rol, ok, c.rol, c.ok)
		}
	}
}
