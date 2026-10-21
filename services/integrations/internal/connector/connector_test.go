// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connector

import (
	"context"
	"encoding/json"
	"testing"
)

// fakeDriver implementa solo la capacidad obligatoria (Test).
type fakeDriver struct{ kind string }

func (f fakeDriver) Kind() string                                  { return f.kind }
func (f fakeDriver) Test(context.Context, Instance, Runtime) error { return nil }

// tabularDriver además implementa la capacidad opcional TabularSource.
type tabularDriver struct{ fakeDriver }

func (t tabularDriver) Query(context.Context, Instance, Runtime, json.RawMessage) (Table, error) {
	return Table{Columns: []Column{{Name: "a", Type: "text"}}, Rows: [][]any{{"x"}}}, nil
}

func TestRegistryGetAndKinds(t *testing.T) {
	reg := NewRegistry(fakeDriver{"slack"}, tabularDriver{fakeDriver{"salesforce"}}, nil)

	if _, ok := reg.Get("slack"); !ok {
		t.Fatal("driver 'slack' no recuperado")
	}
	if _, ok := reg.Get("inexistente"); ok {
		t.Fatal("un Kind no registrado devolvió un driver")
	}
	if got := len(reg.Kinds()); got != 2 {
		t.Fatalf("Kinds() = %d, quiero 2 (el nil se ignora)", got)
	}
}

func TestCapabilityTypeAssert(t *testing.T) {
	reg := NewRegistry(fakeDriver{"slack"}, tabularDriver{fakeDriver{"salesforce"}})

	// 'slack' NO expone capacidades opcionales.
	slack, _ := reg.Get("slack")
	if _, ok := slack.(TabularSource); ok {
		t.Fatal("'slack' no debería satisfacer TabularSource")
	}
	if _, ok := slack.(Provisioner); ok {
		t.Fatal("'slack' no debería satisfacer Provisioner")
	}

	// 'salesforce' SÍ es TabularSource; se detecta por type-assert.
	sf, _ := reg.Get("salesforce")
	ts, ok := sf.(TabularSource)
	if !ok {
		t.Fatal("'salesforce' debería satisfacer TabularSource")
	}
	tbl, err := ts.Query(context.Background(), Instance{}, Runtime{}, nil)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(tbl.Columns) != 1 || len(tbl.Rows) != 1 {
		t.Fatalf("Table inesperada: %+v", tbl)
	}
}
