// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// El HTML nativo se puede DEVOLVER: su volcado sale entero, con los identificadores, las
// fechas y los estados de cada nota, aunque una nota hable de `</script>`, y se lee con
// la misma lectura estricta que un `.onex`.
func TestElHTMLNativoSeVuelveALeer(t *testing.T) {
	creada := time.Date(2025, 3, 4, 10, 0, 0, 0, time.UTC)
	hecha := time.Date(2025, 3, 5, 9, 30, 0, 0, time.UTC)
	nb := Notebook{ID: uuid.New(), Title: "Obra", Color: "#86198f", CreatedAt: creada}
	notas := []Note{
		{
			ID: uuid.New(), Kind: KindText, Title: "HTML", Content: "<p>cierra con &lt;/script&gt; y sigue</p>",
			CreatedAt: creada, UpdatedAt: creada,
			States: []NoteState{{TagID: uuid.New(), TagName: "Prioridad", StateID: uuid.New(), Name: "Alta", Color: "#ff0000"}},
		},
		{
			ID: uuid.New(), Kind: KindTodo, Title: "Pedir tejas", Checked: true, CheckedAt: &hecha, CreatedAt: creada, UpdatedAt: hecha,
			Data: []byte(`{"attachments":[{"id":"1","name":"a.txt","size":1,"type":"text/plain","dataUrl":"data:text/plain;base64,YQ=="}]}`),
		},
	}
	doc := NotebookAHTML(nb, notas)

	volcado, ok := VolcadoDeHTML([]byte(doc))
	if !ok {
		t.Fatal("no saca el volcado del HTML")
	}
	libro, leidas, err := LeerNativoTransferencia(volcado)
	if err != nil {
		t.Fatal(err)
	}
	if libro.ID != nb.ID || libro.Title != "Obra" || !libro.CreatedAt.Equal(creada) || len(leidas) != 2 {
		t.Fatalf("cuaderno = %+v, %d notas", libro, len(leidas))
	}
	porID := map[uuid.UUID]Note{}
	for _, n := range leidas {
		porID[n.ID] = n
	}
	a, b := porID[notas[0].ID], porID[notas[1].ID]
	if a.Content != notas[0].Content || len(a.States) != 1 || a.States[0].TagName != "Prioridad" || a.States[0].Name != "Alta" {
		t.Errorf("la nota de texto no vuelve igual: %+v", a)
	}
	var datos bytes.Buffer
	_ = json.Compact(&datos, b.Data) // el volcado va con sangría
	if !b.Checked || b.CheckedAt == nil || !b.CheckedAt.Equal(hecha) || !b.UpdatedAt.Equal(hecha) || datos.String() != string(notas[1].Data) {
		t.Errorf("la tarea no vuelve igual: %+v", b)
	}

	for nombre, html := range map[string]string{
		"sin volcado":      "<h1>Casa</h1>",
		"sin cerrar":       `<p>x</p>` + aperturaVolcado + `{"formato":"sytue.notepads"`,
		"no es JSON":       aperturaVolcado + "esto no\n</script>",
		"vacío del todo":   "",
		"solo la etiqueta": aperturaVolcado + "</script>",
	} {
		if _, ok := VolcadoDeHTML([]byte(html)); ok {
			t.Errorf("%s: dice que hay volcado", nombre)
		}
	}
}
