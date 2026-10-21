// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// CambiarMiVoto es `toggleMyCardVote`: en respuesta única, votar otra opción retira el
// voto de la anterior; un valor sin votos desaparece; en varias, se suman.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin la condición `unica`, votar la «1» en
// una encuesta de respuesta única deja el voto en la «0» también.
func TestCambiarMiVoto(t *testing.T) {
	c := &Tarjeta{Tipo: "poll", Opciones: []string{"a", "b"}, Votos: []Voto{{Valor: "0", Veces: 1, Mio: true, Subs: []string{"yo"}}}}
	vs := CambiarMiVoto(c, "1", true, "yo")
	if len(vs) != 1 || vs[0].Value != "1" || vs[0].Count != 1 {
		t.Errorf("respuesta única: %+v", vs)
	}
	c.Multi = true
	vs = CambiarMiVoto(c, "1", true, "yo")
	if len(vs) != 2 || vs[0].Count != 1 || vs[1].Count != 1 {
		t.Errorf("varias respuestas: %+v", vs)
	}
	if vs := CambiarMiVoto(c, "0", false, "yo"); len(vs) != 0 {
		t.Errorf("retirar el único voto deja %+v", vs)
	}
}

// Una encuesta se pinta con sus opciones y se vota desde el historial: el voto se ve ya,
// se ajusta al recuento del servicio y vuelve si falla.
func TestLaEncuestaSeVota(t *testing.T) {
	s := servicioConDos()
	s.mensajes["c1"][0].Card = &api.RoomsCard{Kind: "poll", Options: []string{"Viernes", "Lunes"}}
	s.votos = []api.RoomsCardVote{{Value: "1", Count: 5, Subs: []string{"yo"}}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	f := filaDe(t, sa.P.Canal.Historial, "m1")
	test.WidgetRenderer(f)
	if f.tarjeta == nil {
		t.Fatal("la fila de la encuesta no lleva su tarjeta")
	}
	sa.Votar(sa.P.Canal.Historial.Mensajes[0], "1", true)
	m := sa.P.Canal.Historial.Mensajes[0]
	if !s.pidio("VoteCard(m1,1,sí)") || m.Tarjeta.VotoDe("1").Veces != 5 || !m.Tarjeta.VotoDe("1").Mio {
		t.Errorf("votar deja %+v: %v", m.Tarjeta, s.llamadas)
	}
	s.errAccion = errors.New("caído")
	sa.Votar(m, "0", true)
	if got := sa.P.Canal.Historial.Mensajes[0].Tarjeta; got.VotoDe("0").Veces != 0 || got.VotoDe("1").Veces != 5 {
		t.Errorf("con el servicio caído no vuelve: %+v", got.Votos)
	}
}

// «+» → «Encuesta» publica la pregunta con su tarjeta; con una sola opción no se puede.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin comprobar las dos opciones en `valida`,
// la encuesta de una opción sale y el servicio la rechaza.
func TestCrearUnaEncuesta(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.NuevaTarjeta("poll")
	d := sa.pregunta
	if d == nil {
		t.Fatal("no se abre el diálogo")
	}
	d.Cerrar()
	if TarjetaValida("poll", "¿Cuándo?", []string{"Viernes", " "}) || !TarjetaValida("poll", "¿Cuándo?", []string{"Viernes", "Lunes"}) ||
		!TarjetaValida("task", "Informe", nil) || TarjetaValida("task", " ", nil) {
		t.Error("TarjetaValida no es la de la web")
	}
	sa.EnviarTarjeta("¿Cuándo?", &api.RoomsCard{Kind: "poll", Options: []string{"Viernes", "Lunes"}})
	if len(s.envios) != 1 || s.envios[0].Card == nil || len(s.envios[0].Card.Options) != 2 || !s.pidio("Post(c1,¿Cuándo?)") {
		t.Errorf("la encuesta sale %+v: %v", s.envios, s.llamadas)
	}
}
