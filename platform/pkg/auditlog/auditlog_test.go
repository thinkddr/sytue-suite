// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Lo que se vigila aquí es lo que ANTES estaba escrito dos veces y podía
// divergir sin que nadie lo viera: el corte de la página por keyset, el ida y
// vuelta del cursor y los límites del rango de fechas. Un fallo en cualquiera de
// los tres no se ve en pantalla —sale una página plausible— y en una traza de
// cumplimiento eso es peor que un error.
package auditlog_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

func momento(minutos int) time.Time {
	return time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC).Add(time.Duration(minutos) * time.Minute)
}

// entradas devuelve n filas en el orden en que las da el store: DESC por
// (occurred_at, id). La primera es la más reciente.
func entradas(n int) []auditlog.Entrada {
	out := make([]auditlog.Entrada, 0, n)
	for i := n; i > 0; i-- {
		out = append(out, auditlog.Entrada{ID: int64(i), OccurredAt: momento(i), Actor: "ana@acme", Action: "hold.create"})
	}
	return out
}

func TestPaginarNoRepiteNiSeSaltaFilas(t *testing.T) {
	todas := entradas(10)

	pag1, cursor := auditlog.Paginar(todas, nil, 4)
	if len(pag1) != 4 || cursor == "" {
		t.Fatalf("primera página: %d filas, cursor %q", len(pag1), cursor)
	}
	pos, err := auditlog.LeerCursor(cursor)
	if err != nil || pos == nil {
		t.Fatalf("LeerCursor: %v", err)
	}

	pag2, _ := auditlog.Paginar(entradas(10), pos, 4)
	if len(pag2) != 4 {
		t.Fatalf("segunda página: %d filas", len(pag2))
	}
	// La frontera es lo que se rompe al tocar el `<` o el `<=`: la segunda página
	// tiene que EMPEZAR justo después de la última de la primera, sin repetirla.
	if pag2[0].ID != pag1[3].ID-1 {
		t.Fatalf("la segunda página empieza en %d; la primera acabó en %d", pag2[0].ID, pag1[3].ID)
	}
	// Y juntas, sin huecos.
	vistas := map[int64]bool{}
	for _, e := range append(append([]auditlog.Entrada{}, pag1...), pag2...) {
		if vistas[e.ID] {
			t.Fatalf("fila %d repetida entre páginas", e.ID)
		}
		vistas[e.ID] = true
	}
	if len(vistas) != 8 {
		t.Fatalf("se esperaban 8 filas distintas, hubo %d", len(vistas))
	}
}

func TestPaginarUltimaPaginaNoDaCursor(t *testing.T) {
	if _, cursor := auditlog.Paginar(entradas(3), nil, 10); cursor != "" {
		t.Fatalf("sin más filas no debe haber cursor, hubo %q", cursor)
	}
	// Y el caso exacto: tantas filas como el límite tampoco deja siguiente.
	if _, cursor := auditlog.Paginar(entradas(4), nil, 4); cursor != "" {
		t.Fatalf("con len == limit tampoco hay siguiente, hubo %q", cursor)
	}
}

func TestCursorIdaYVuelta(t *testing.T) {
	ts := momento(7)
	pos, err := auditlog.LeerCursor(auditlog.FormatearCursor(ts, 42))
	if err != nil || pos == nil {
		t.Fatalf("ida y vuelta: %v", err)
	}
	if pos.ID != 42 || !pos.OccurredAt.Equal(ts) {
		t.Fatalf("se perdió información: %+v", pos)
	}
	// Vacío es «primera página», no un error.
	if p, err := auditlog.LeerCursor(""); err != nil || p != nil {
		t.Fatalf("token vacío: p=%+v err=%v", p, err)
	}
	// Corrupto es error, para que el llamante conteste 400 y no un 500.
	for _, malo := range []string{"no-es-base64!!", "c2luLXNlcGFyYWRvcg", "Zm9vfDEyMw"} {
		if _, err := auditlog.LeerCursor(malo); err == nil {
			t.Fatalf("el cursor %q debería fallar", malo)
		}
	}
}

func TestFiltrarPorAccionActorYRango(t *testing.T) {
	base := []auditlog.Entrada{
		{ID: 3, OccurredAt: momento(30), Actor: "ana@acme", Action: "hold.create"},
		{ID: 2, OccurredAt: momento(20), Actor: "BEA@acme", Action: "export.run"},
		{ID: 1, OccurredAt: momento(10), Actor: "ana@acme", Action: "hold.release"},
	}
	copia := func() []auditlog.Entrada { return append([]auditlog.Entrada{}, base...) }

	// La acción filtra por PREFIJO, el actor por SUBCADENA, y ninguno distingue
	// mayúsculas: son tres reglas distintas y es fácil intercambiarlas al tocar.
	if got := auditlog.Filtrar(copia(), auditlog.Consulta{Action: "HOLD."}); len(got) != 2 {
		t.Fatalf("prefijo de acción: %d filas", len(got))
	}
	if got := auditlog.Filtrar(copia(), auditlog.Consulta{Action: "create"}); len(got) != 0 {
		t.Fatalf("la acción es prefijo, no subcadena: %d filas", len(got))
	}
	if got := auditlog.Filtrar(copia(), auditlog.Consulta{Actor: "bea"}); len(got) != 1 {
		t.Fatalf("subcadena de actor sin mayúsculas: %d filas", len(got))
	}
	// El rango es [Since, Until): el borde inferior entra y el superior no.
	got := auditlog.Filtrar(copia(), auditlog.Consulta{Since: momento(20), Until: momento(30)})
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("rango semiabierto: %+v", got)
	}
}

func TestLeerConsultaValidaYRechaza(t *testing.T) {
	pide := func(qs string) (auditlog.Consulta, error) {
		return auditlog.LeerConsulta(httptest.NewRequest(http.MethodGet, "/admin/audit-log?"+qs, http.NoBody))
	}

	// Una fecha suelta como `until` es INCLUSIVA del día: el filtro usa `<`, así
	// que tiene que avanzar al día siguiente o «hasta el 10» se comería el 10.
	q, err := pide("until=2026-09-10")
	if err != nil {
		t.Fatalf("until=fecha: %v", err)
	}
	if !q.Until.Equal(time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("until de una fecha suelta debe avanzar un día, dio %v", q.Until)
	}
	// Un RFC3339 se toma tal cual, sin avanzar.
	q, err = pide("until=2026-09-10T08:00:00Z")
	if err != nil || !q.Until.Equal(time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("until RFC3339: %v (%v)", q.Until, err)
	}

	if q, err := pide(""); err != nil || q.Limit != 100 {
		t.Fatalf("límite por defecto 100, dio %d (%v)", q.Limit, err)
	}
	// Ojo con el rango invertido: `since=10&until=10` NO lo es, porque un `until`
	// de fecha suelta avanza al día siguiente y deja un rango de un día válido.
	// Para invertirlo de verdad hace falta que `since` pase de ese día siguiente.
	for _, malo := range []string{"limit=0", "limit=501", "limit=abc", "since=ayer", "cursor=???", "since=2026-09-11&until=2026-09-10"} {
		if _, err := pide(malo); err == nil {
			t.Fatalf("«%s» debería rechazarse", malo)
		}
	}
	if q, err := pide("format=csv"); err != nil || !q.CSV {
		t.Fatalf("format=csv: %+v (%v)", q, err)
	}
}
