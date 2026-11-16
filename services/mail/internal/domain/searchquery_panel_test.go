// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// TestLaConsultaDelPanelEsLaQueEntiendeElServicio recorre los casos del panel
// «Opciones de búsqueda» de Mail (apps/fyne/apps/mail/testdata/consulta-busqueda.json,
// que pasan también la web y la app nativa): lo que el panel lee de una consulta es lo
// que este parser entiende de ella. Si aquí cambia un operador o un sinónimo y el panel
// no, el panel rellenaría campos con criterios que la búsqueda no aplica.
func TestLaConsultaDelPanelEsLaQueEntiendeElServicio(t *testing.T) {
	t.Parallel()
	crudo, err := os.ReadFile("../../../../apps/fyne/apps/mail/testdata/consulta-busqueda.json")
	if err != nil {
		t.Fatal(err)
	}
	type opciones struct {
		De, Para, Asunto     string
		ConAdjuntos, SinLeer bool
		Desde, Hasta         string
	}
	var v struct {
		Desmontar []struct {
			Nombre   string
			Consulta string
			Opciones opciones
		}
	}
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Desmontar) < 10 {
		t.Fatalf("solo hay %d casos", len(v.Desmontar))
	}
	dia := func(d time.Time, desplazar int) string {
		if d.IsZero() {
			return ""
		}
		return d.AddDate(0, 0, desplazar).Format(time.DateOnly)
	}
	for _, c := range v.Desmontar {
		q := domain.ParseSearchQuery(c.Consulta)
		// El panel muestra «Hasta» inclusivo: el día ANTERIOR al `before` del servicio.
		got := opciones{
			De: q.From, Para: q.To, Asunto: q.Subject,
			ConAdjuntos: q.HasAttachment, SinLeer: q.Unseen,
			Desde: dia(q.After, 0), Hasta: dia(q.Before, -1),
		}
		if got != c.Opciones {
			t.Errorf("%s:\n servicio %+v\n    panel %+v", c.Nombre, got, c.Opciones)
		}
	}
}
