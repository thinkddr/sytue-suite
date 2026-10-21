// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El registro de avisos y errores, contra PostgreSQL real.
//
// Lo que hace útil esta pantalla no es guardar líneas, es AGRUPARLAS: mil veces
// el mismo error tienen que leerse como un problema que lleva pasando mil
// veces, no como mil problemas. Y el histograma tiene que enseñar las horas
// vacías, o dos picos separados por un día de calma parecen una crisis continua.
package store_test

import (
	"testing"
	"time"

	plog "git.sytue.eu/sytue/one/platform/pkg/log"
)

func TestServiceLogs(t *testing.T) {
	st, ctx := newStore(t)
	ahora := time.Now()

	lote := []plog.Entry{}
	// El mismo error de Mail cinco veces, en la última hora.
	for i := range 5 {
		lote = append(lote, plog.Entry{
			At: ahora.Add(-time.Duration(i) * time.Minute), Level: "ERROR",
			Service: "sytue-mail", Message: "enviar invitación falló",
			Attrs: map[string]string{"org": "acme", "intento": itoaTest(i)},
		})
	}
	// Un aviso de otro servicio y un error viejo, fuera de la ventana de una hora.
	lote = append(lote,
		plog.Entry{
			At: ahora.Add(-10 * time.Minute), Level: "WARN",
			Service: "sytue-id", Message: "SMTP no configurado",
		},
		plog.Entry{
			At: ahora.Add(-5 * time.Hour), Level: "ERROR",
			Service: "sytue-drive", Message: "no se pudo escribir el blob",
		},
	)
	if err := st.WriteServiceLogs(ctx, lote); err != nil {
		t.Fatalf("WriteServiceLogs: %v", err)
	}

	// Agrupado: las cinco de Mail son UNA fila con count=5.
	grupos, err := st.ListLogGroups(ctx, ahora.Add(-24*time.Hour), "", "", "", 50)
	if err != nil {
		t.Fatalf("ListLogGroups: %v", err)
	}
	if len(grupos) != 3 {
		t.Fatalf("%d grupos, quiero 3 (las cinco de Mail son una): %+v", len(grupos), grupos)
	}
	var mail bool
	for _, g := range grupos {
		if g.Service != "sytue-mail" {
			continue
		}
		mail = true
		if g.Count != 5 {
			t.Errorf("count=%d, quiero 5: sin recuento no se distingue un fallo suelto de una avería", g.Count)
		}
		if g.Sample["org"] != "acme" {
			t.Errorf("sin ejemplar: %v", g.Sample)
		}
		if !g.Last.After(g.First) {
			t.Errorf("primera=%v última=%v: el grupo no dice desde cuándo pasa", g.First, g.Last)
		}
	}
	if !mail {
		t.Fatal("el grupo de Mail no aparece")
	}

	// Filtro por nivel: el aviso no es un error.
	soloErrores, err := st.ListLogGroups(ctx, ahora.Add(-24*time.Hour), "error", "", "", 50)
	if err != nil {
		t.Fatalf("filtro nivel: %v", err)
	}
	for _, g := range soloErrores {
		if g.Level != "ERROR" {
			t.Errorf("el filtro de nivel deja pasar %q", g.Level)
		}
	}

	// Buscar por texto, con un '%' que si no se escapara devolvería todo.
	nada, err := st.ListLogGroups(ctx, ahora.Add(-24*time.Hour), "", "", "%", 50)
	if err != nil {
		t.Fatalf("busqueda: %v", err)
	}
	if len(nada) != 0 {
		t.Errorf("un '%%' devolvió %d grupos: el término no se escapa", len(nada))
	}

	// Cifras de cabecera y tendencia.
	stats, err := st.LogStatsSince(ctx, ahora)
	if err != nil {
		t.Fatalf("LogStatsSince: %v", err)
	}
	if stats.Errors24h != 6 {
		t.Errorf("errores 24h = %d, quiero 6", stats.Errors24h)
	}
	if stats.Errors1h != 5 {
		t.Errorf("errores 1h = %d, quiero 5: el viejo no cuenta como reciente", stats.Errors1h)
	}
	if stats.Warns24h != 1 {
		t.Errorf("avisos 24h = %d, quiero 1", stats.Warns24h)
	}
	if stats.Services != 2 {
		t.Errorf("servicios con errores = %d, quiero 2 (mail y drive)", stats.Services)
	}
	// 24 barras SIEMPRE, incluidas las horas sin nada.
	if len(stats.PorHora) != 24 {
		t.Errorf("%d barras, quiero 24: sin las horas vacías el gráfico miente", len(stats.PorHora))
	}

	// La purga se lleva lo viejo y respeta lo reciente.
	n, err := st.PurgeServiceLogs(ctx, ahora.Add(-time.Hour))
	if err != nil {
		t.Fatalf("PurgeServiceLogs: %v", err)
	}
	if n != 1 {
		t.Errorf("purgadas %d filas, quiero 1 (solo la de hace cinco horas)", n)
	}
}

func itoaTest(n int) string { return string(rune('0' + n)) }
