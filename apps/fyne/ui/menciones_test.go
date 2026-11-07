// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLasMenciones: los casos de shell/src/menciones.test.ts. La mención que se escribe
// es la que el servicio reconoce para avisar: `@correo`.
//
// VISTO EN ROJO (2026-11-05, en el frontal): con el `\s` de RE2 en la expresión, «enlace
// @an» con el espacio duro que el editor de notas pone tras un enlace no mencionaba.
func TestLasMenciones(t *testing.T) {
	for texto, quiero := range map[string]string{"Hola @an": "an", "@": "", "Hola @AN": "an", "enlace @an": "an"} {
		if q, ok := ConsultaMencion(texto); !ok || q != quiero {
			t.Errorf("ConsultaMencion(%q) = %q %v, quiero %q", texto, q, ok, quiero)
		}
	}
	for _, x := range []string{"escribe a ana@x.es", "Hola @ana@x.es ya", "sin arroba"} {
		if q, ok := ConsultaMencion(x); ok {
			t.Errorf("%q no es una mención a medias y da %q", x, q)
		}
	}
	if got := PonerMencion("Hola @an", "ana@x.es"); got != "Hola @ana@x.es " {
		t.Errorf("PonerMencion = %q", got)
	}
	g := []api.Miembro{{Email: "ana@x.es", Nombre: "Ana Pérez"}, {Email: "luis@x.es"}}
	if c := CandidatosMencion(g, "per"); len(c) != 1 || c[0].Email != "ana@x.es" {
		t.Errorf("por nombre y sin acentos: %+v", c)
	}
	if c := CandidatosMencion(g, "luis"); len(c) != 1 || c[0].Email != "luis@x.es" {
		t.Errorf("por dirección: %+v", c)
	}
	muchos := make([]api.Miembro, 9)
	for i := range muchos {
		muchos[i] = api.Miembro{Email: "x@x.es"}
	}
	if len(CandidatosMencion(muchos, "x")) != 6 {
		t.Error("como mucho seis")
	}
	for _, c := range []struct {
		activa, total int
		tecla         fyne.KeyName
		quiero        int
	}{{0, 3, fyne.KeyDown, 1}, {2, 3, fyne.KeyDown, 0}, {0, 3, fyne.KeyUp, 2}, {1, 3, fyne.KeyA, 1}, {0, 0, fyne.KeyDown, 0}} {
		if got := MoverActiva(c.activa, c.total, c.tecla); got != c.quiero {
			t.Errorf("MoverActiva(%d, %d, %s) = %d, quiero %d", c.activa, c.total, c.tecla, got, c.quiero)
		}
	}
}

// TestLaAgendaSePideUnaVez: a la primera arroba se pide la agenda, las siguientes usan
// lo que llegó, y un fallo se vuelve a pedir en la siguiente.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin volver `pedida` a false en el fallo,
// la segunda arroba no volvía a pedir y la lista se quedaba vacía para siempre.
func TestLaAgendaSePideUnaVez(t *testing.T) {
	var peticiones atomic.Int32
	falla := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		peticiones.Add(1)
		if falla {
			http.Error(w, "no", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"user_id":"u1","email":"ana@x.es","name":"Ana"}]}`))
	}))
	defer srv.Close()
	dir := api.NewDirectorio(srv.URL, nil, nil)
	sincrona := func(f func()) { f() }
	var a AgendaMenciones
	llegadas := 0
	if g := a.Gente(dir, sincrona, func() { llegadas++ }); len(g) != 0 || llegadas != 0 {
		t.Fatalf("un fallo trae %v y avisa %d veces", g, llegadas)
	}
	falla = false
	a.Gente(dir, sincrona, func() { llegadas++ })
	if g := a.Gente(dir, sincrona, func() { llegadas++ }); len(g) != 1 || g[0].Email != "ana@x.es" || llegadas != 1 {
		t.Errorf("tras el fallo, la agenda es %v (avisos %d)", g, llegadas)
	}
	if n := peticiones.Load(); n != 2 {
		t.Errorf("la agenda se ha pedido %d veces; una por el fallo y otra buena", n)
	}
	var sin AgendaMenciones
	if sin.Gente(nil, sincrona, nil) != nil {
		t.Error("sin directorio no hay gente")
	}
}
