// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

const tablaA = "11111111-1111-4111-8111-111111111111"

// La superficie es una lista ESCRITA ENTERA aquí: añadir una operación al mapa sin
// tocar esta prueba la pone en rojo, que es cuando alguien tiene que decidirlo.
func TestSuperficieDeScriptsEsCerrada(t *testing.T) {
	quiero := map[string]bool{
		"tables.list": false, "fields.list": false, "views.list": false,
		"records.list": false, "records.get": false,
		"records.create": true, "records.update": true, "records.delete": true,
	}
	if len(OperacionesDeScript) != len(quiero) {
		t.Fatalf("la superficie tiene %d operaciones, esperadas %d", len(OperacionesDeScript), len(quiero))
	}
	for op, escribe := range quiero {
		if got, ok := OperacionesDeScript[op]; !ok || got != escribe {
			t.Errorf("%s: existe=%v escribe=%v, esperado escribe=%v", op, ok, got, escribe)
		}
	}
}

func TestParseManifiesto(t *testing.T) {
	m, err := ParseManifiesto([]byte(`{"v":1,"ops":["records.list","records.create","records.list"],"tables":["` + strings.ToUpper(tablaA) + `"],"write":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Ops, []string{"records.create", "records.list"}) || !slices.Equal(m.Tables, []string{tablaA}) ||
		m.MaxOps != OpsPorDefecto || m.MaxMs != 5000 {
		t.Errorf("normalizado: %+v", m)
	}
	// El canónico es estable: volver a leerlo da lo mismo.
	canon, _ := json.Marshal(m)
	m2, err := ParseManifiesto(canon)
	canon2, _ := json.Marshal(m2)
	if err != nil || !bytes.Equal(canon, canon2) {
		t.Errorf("el canónico no es estable: %s / %s (%v)", canon, canon2, err)
	}
	// Sin listas: ni operaciones ni tablas, y sale con listas vacías (no null).
	if v, _ := ParseManifiesto([]byte(`{"v":1,"write":false}`)); string(mustJSON(v)) != `{"v":1,"ops":[],"tables":[],"write":false,"max_ops":100,"max_ms":5000}` {
		t.Errorf("vacío: %s", mustJSON(v))
	}

	malos := map[string]string{
		"vacío":                        ``,
		"no es un objeto":              `[]`,
		"versión":                      `{"v":2,"write":false}`,
		"campo desconocido":            `{"v":1,"write":false,"unattended":true}`,
		"datos de más":                 `{"v":1,"write":false}{}`,
		"operación inventada":          `{"v":1,"ops":["records.purge"],"write":true}`,
		"operación de esquema":         `{"v":1,"ops":["fields.create"],"write":true}`,
		"permisos":                     `{"v":1,"ops":["permissions.grant"],"write":true}`,
		"escribe sin declararlo":       `{"v":1,"ops":["records.delete"],"write":false}`,
		"tabla que no es un id":        `{"v":1,"tables":["../otra"],"write":false}`,
		"demasiadas operaciones":       `{"v":1,"write":false,"max_ops":501}`,
		"operaciones negativas":        `{"v":1,"write":false,"max_ops":-1}`,
		"demasiado tiempo":             `{"v":1,"write":false,"max_ms":30001}`,
		"muy poco tiempo":              `{"v":1,"write":false,"max_ms":50}`,
		"demasiadas tablas":            `{"v":1,"write":false,"tables":[` + strings.TrimSuffix(strings.Repeat(`"`+tablaA+`",`, MaxTablasDeScript+1), ",") + `]}`,
		"demasiado grande":             `{"v":1,"write":false,"ops":[` + strings.Repeat(" ", maxManifiesto) + `]}`,
		"lista de operaciones inflada": `{"v":1,"write":false,"ops":[` + strings.TrimSuffix(strings.Repeat(`"records.get",`, 17), ",") + `]}`,
	}
	for nombre, crudo := range malos {
		if _, err := ParseManifiesto([]byte(crudo)); !errors.Is(err, ErrInvalido) {
			t.Errorf("%s: error %v, esperado ErrInvalido", nombre, err)
		}
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestVeScriptYAccionParaEjecutar(t *testing.T) {
	privado := Script{AuthorSub: "ana", Scope: ScriptPrivado, ApprovedVersion: 3}
	compartido := Script{AuthorSub: "ana", Scope: ScriptCompartido}
	aprobado := Script{AuthorSub: "ana", Scope: ScriptCompartido, ApprovedVersion: 1}
	casos := []struct {
		s      Script
		sub    string
		revisa bool
		quiero bool
	}{
		{privado, "ana", false, true},
		{privado, "bea", false, false},
		{privado, "bea", true, false}, // ni la propietaria de la base ve un privado ajeno
		{compartido, "ana", false, true},
		{compartido, "bea", false, false}, // sin versión aprobada no existe para los demás
		{compartido, "bea", true, true},   // quien revisa sí lo ve
		{aprobado, "bea", false, true},
	}
	for i, c := range casos {
		if got := VeScript(c.s, c.sub, c.revisa); got != c.quiero {
			t.Errorf("caso %d: %v, esperado %v", i, got, c.quiero)
		}
	}
	if AccionParaEjecutar(ManifiestoScript{Write: true}) != AccEscribirReg || AccionParaEjecutar(ManifiestoScript{}) != AccLeer {
		t.Error("la acción que pide ejecutar")
	}
}
