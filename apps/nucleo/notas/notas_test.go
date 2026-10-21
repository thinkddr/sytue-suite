// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notas

import (
	"context"
	"errors"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// TestSubirLlevaLosBorradoresYLaCola: al cerrar sesión con «Subir», cada borrador sale
// como la nota que habría hecho Intro en su cuaderno (o en su día, si aún no existe),
// la cola también, y no queda nada. Sin red, todo se queda y se cuenta.
func TestSubirLlevaLosBorradoresYLaCola(t *testing.T) {
	st, err := offline.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const dueño = "ada/acme"
	for id, texto := range map[string]string{"nb-1": "[ ] comprar pan", PrefijoDia + "2026-10-17": "idea suelta", "nb-2": "   "} {
		if err := st.Write(dueño, EspacioBorradores, id, texto); err != nil {
			t.Fatal(err)
		}
	}
	cola := Captura{ClientID: "c1", Cuaderno: "nb-3", In: api.NoteInput{Kind: "text", Content: "<p>en cola</p>"}.ConClientID("c1")}
	if err := st.Write(dueño, EspacioCapturas, "c1", cola); err != nil {
		t.Fatal(err)
	}

	var enviadas []string
	sinRed := true
	e := Envio{
		Hoy: "2026-10-16",
		Crear: func(_ context.Context, nb string, in api.NoteInput) (api.Note, error) {
			if sinRed {
				return api.Note{}, errors.New("sin red")
			}
			if in.ClientID() == "" {
				t.Errorf("la nota a %s sale sin client_id: reintentar la duplicaría", nb)
			}
			enviadas = append(enviadas, nb+":"+in.Kind)
			return api.Note{}, nil
		},
		Diario: func(_ context.Context, date, _ string, _ bool) (api.Notebook, error) {
			return api.Notebook{ID: "dia-" + date}, nil
		},
	}
	n := 0
	nuevo := func() string { n++; return string(rune('a' + n)) }

	if quedan := Subir(context.Background(), st, dueño, e, nuevo); quedan != 3 {
		t.Fatalf("sin red quedan %d; quiero 3 (dos borradores, ya en la cola, y la captura)", quedan)
	}
	sinRed = false
	if quedan := Subir(context.Background(), st, dueño, e, nuevo); quedan != 0 {
		t.Fatalf("con red quedan %d; quiero 0", quedan)
	}
	slices.Sort(enviadas)
	if want := []string{"dia-2026-10-17:text", "nb-1:todo", "nb-3:text"}; !slices.Equal(enviadas, want) {
		t.Fatalf("enviadas %v; quiero %v", enviadas, want)
	}
}
