// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestBloquearCreaLaReglaYMueveElTriajeASpam: «Bloquear a …» en «Más acciones» crea
// la regla «de esta dirección → Spam» con un nombre legible, mueve el hilo entero a
// Spam en una llamada y lo saca de la vista; «Deshacer» quita la regla y devuelve el
// hilo a Recibidos.
//
// VISTO EN ROJO (2026-11-13): sin la entrada en `menuMas` no se ofrece, y sin el
// `DeleteFilter` de `deshacerBloqueo` la regla sobrevive al «Deshacer». Deshecho.
func TestBloquearCreaLaReglaYMueveElTriajeASpam(t *testing.T) {
	s := servicioConTodas()
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())
	b.Abrir(b.P.Bandeja.Conversaciones[0])

	bloquear := elementoDelMenu(b, "Bloquear a Persona m1")
	if bloquear == nil {
		t.Fatal("«Más acciones» no ofrece «Bloquear a …»")
	}
	bloquear.AlPulsar()
	if !slices.Contains(s.llamadas, "CreateFilter(Bloquear a m1@example.org,m1@example.org,,jk,false,false)") ||
		!slices.Contains(s.llamadas, "MoveMany(jk,m1,m3)") {
		t.Fatalf("bloquear: %v", s.llamadas)
	}
	if len(s.reglas) != 1 || !s.reglas[0].Enabled {
		t.Errorf("la regla no ha quedado activa: %+v", s.reglas)
	}
	for _, c := range b.P.Bandeja.Conversaciones {
		if c.HiloID == "h1" {
			t.Fatal("el hilo bloqueado sigue en Recibidos")
		}
	}
	av := b.P.Avisos.Avisos()
	if len(av) != 1 || av[0].Tono != ui.AvisoExito || av[0].Titulo != "m1@example.org bloqueado: sus correos irán a Spam" ||
		av[0].AlPulsarAccion == nil {
		t.Fatalf("aviso: %+v", av)
	}
	av[0].AlPulsarAccion()
	if len(s.reglas) != 0 || !slices.Contains(s.llamadas, "DeleteFilter(r1)") {
		t.Errorf("deshacer no quita la regla: %+v", s.reglas)
	}
	if !slices.Contains(s.llamadas, "MoveMany(in,m1,m3)") {
		t.Errorf("deshacer no devuelve el hilo a Recibidos: %v", s.llamadas)
	}
}

// TestBloquearSinPoderMoverNoDejaLaRegla: si el hilo no llega a Spam, la regla recién
// creada se borra —un aviso de error con la regla puesta engaña—, el hilo vuelve a la
// lista y se avisa.
func TestBloquearSinPoderMoverNoDejaLaRegla(t *testing.T) {
	s := servicioConTodas()
	s.errMover = &api.Error{Status: http.StatusBadGateway}
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())

	b.P.Acciones.Bloquear(b.P.Bandeja.Conversaciones[0])
	if len(s.reglas) != 0 || !slices.Contains(s.llamadas, "DeleteFilter(r1)") {
		t.Errorf("la regla se ha quedado: %+v %v", s.reglas, s.llamadas)
	}
	if len(b.P.Bandeja.Conversaciones) != 2 {
		t.Errorf("tras el rechazo hay %d conversaciones y eran 2", len(b.P.Bandeja.Conversaciones))
	}
	if tono, titulo := ultimoAviso(b); tono != ui.AvisoPeligro || titulo != traductor(t).T("reader.blockFailed", nil) {
		t.Errorf("aviso: %v %q", tono, titulo)
	}
}

// TestBloquearSoloDondeHayTriajeYReglas: ni dentro de Spam, ni a uno mismo; en una
// búsqueda sí, que los resultados pueden venir de cualquier carpeta.
func TestBloquearSoloDondeHayTriajeYReglas(t *testing.T) {
	b := buzonConAvisos(t, servicioConTodas())
	b.Arrancar(context.Background())
	a := b.P.Acciones
	if a.Bloquear == nil || a.PuedeBloquear == nil {
		t.Fatal("en Recibidos no se ofrece bloquear")
	}
	if a.PuedeBloquear("Ada@Example.org") || !a.PuedeBloquear("no-reply@boletin.eu") || a.PuedeBloquear("") {
		t.Error("a uno mismo no, a un no-reply sí, sin dirección no")
	}
	b.ElegirCarpeta(carpetaPorRol(b, "junk"))
	if a.Bloquear != nil {
		t.Error("dentro de Spam se ofrece bloquear")
	}
}
