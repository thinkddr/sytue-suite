// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// conVigia es el doble con la llamada y el recuento de presentes por canal.
type conVigia struct {
	*conLlamada
	presentes map[string]int
	errVigia  error
}

func (s *conVigia) LlamadaActiva(_ context.Context, canal string) (int, error) {
	s.apuntar("LlamadaActiva", canal)
	return s.presentes[canal], s.errVigia
}

func salaConVigia(t *testing.T, presentes map[string]int) (*Sala, *conVigia, func(), fyne.Window) {
	t.Helper()
	base := servicioConDos()
	s := &conVigia{conLlamada: &conLlamada{conReuniones: &conReuniones{servicioFalso: base}}, presentes: presentes}
	sa, relojes, w := salaEn(t, base, 1280)
	sa.S = s
	sa.Conectar = func(context.Context, api.RoomsCall, llamada.Opciones) (EnCurso, error) { return &enCursoFalsa{}, nil }
	t.Cleanup(func() {
		if sa.ventanaLlamada != nil {
			sa.ventanaLlamada.Close()
		}
	})
	sa.Arrancar(context.Background())
	return sa, s, func() { relojes[CadaLlamadaActiva]() }, w
}

func canalDelCarril(t *testing.T, sa *Sala, id string) Canal {
	t.Helper()
	for _, c := range sa.P.Lateral.Canales {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("el canal %s no está en el carril", id)
	return Canal{}
}

// Una trama `call_started` de OTRO canal enciende el punto de su fila sin abrirlo, y
// `call_ended` lo apaga: es la parte de la trama `call` de `ChatApp.tsx` que el
// nativo descartaba (solo recargaba los mensajes del abierto).
//
// VISTO EN ROJO (2026-11-09): quitando la llamada a alEmpezarOAcabarLlamada de
// alLlegar, el directo sigue sin punto tras `call_started`.
func TestLaLlamadaDeOtroCanalEnciendeElPuntoDelCarril(t *testing.T) {
	sa, _, _, _ := salaConVigia(t, nil)
	if sa.abierto != "c1" {
		t.Fatalf("se abrió %q", sa.abierto)
	}
	sa.alLlegar(api.RoomsEvent{Type: "call_started", ChannelID: "d1"})
	if !canalDelCarril(t, sa, "d1").EnLlamada {
		t.Fatal("la llamada del directo no enciende el punto de su fila")
	}
	if canalDelCarril(t, sa, "c1").EnLlamada || sa.P.Canal.ConFranjaLlamada() {
		t.Error("la llamada de otro canal se pinta en el abierto")
	}
	// La fila que se pinta lleva el punto (no solo el dato).
	f := NuevaFilaCanal(sa.P.T, canalDelCarril(t, sa, "d1"), nil, false, func() {}, func() {})
	test.WidgetRenderer(f)
	if f.vivo == nil {
		t.Error("la fila del directo no pinta el punto vivo")
	}
	sa.alLlegar(api.RoomsEvent{Type: "call_ended", ChannelID: "d1"})
	if canalDelCarril(t, sa, "d1").EnLlamada {
		t.Error("`call_ended` no apaga el punto")
	}
}

// En el canal abierto, la franja dice «Llamada en curso · N participantes» con lo que
// cuenta el servicio, «Unirse» entra por la misma puerta que «Llamar» y, ya dentro, la
// franja se esconde. `call_ended` la apaga en seco; el sondeo es de 60 s, se para con la
// app de fondo y pregunta en el acto al volver.
//
// VISTO EN ROJO (2026-11-09): sin abrirLlamadaActiva en `abrir` no se pregunta nada al
// abrir el canal y la franja no sale; sin pararSondeoLlamada en EnSegundoPlano el
// sondeo sigue programado con la ventana de fondo.
func TestLaFranjaDeLlamadaEnCursoDelCanalAbierto(t *testing.T) {
	sa, s, sondeo, w := salaConVigia(t, map[string]int{"c1": 3})
	c := sa.P.Canal
	if !s.pidio("LlamadaActiva(c1)") || !c.ConFranjaLlamada() || !c.Llamada.Visible() {
		t.Fatalf("la franja no sale: %v · %d presentes", s.llamadas, c.Llamada.Participantes)
	}
	quiero := sa.P.T.T("call.inProgress", map[string]any{"count": 3})
	if got := c.Llamada.Rotulo(); got != quiero || got != "Llamada en curso · 3 participantes" {
		t.Errorf("la franja dice %q", got)
	}
	if c.Llamada.AccessibilityLabel() != quiero {
		t.Error("la franja no se anuncia con su texto (`role=status`)")
	}
	if c.Historial.Position().Y != AltoCabecera+MedidaFranjaLlamada().Height {
		t.Errorf("el historial empieza en %v: la franja lo tapa", c.Historial.Position().Y)
	}
	capturarCompositor(t, w, "canal.llamada-en-curso")
	if !canalDelCarril(t, sa, "c1").EnLlamada {
		t.Error("con gente dentro, el punto del canal abierto sigue apagado")
	}

	// El sondeo ajusta el número; un fallo deja lo que había.
	s.presentes["c1"] = 1
	sondeo()
	if got := c.Llamada.Rotulo(); got != "Llamada en curso · 1 participante" {
		t.Errorf("tras el sondeo dice %q", got)
	}
	s.errVigia = errors.New("caído")
	sondeo()
	if c.Llamada.Participantes != 1 {
		t.Errorf("un fallo del servicio cambia la franja a %d", c.Llamada.Participantes)
	}
	s.errVigia = nil

	test.Tap(c.Llamada.Unirse())
	if sa.ventanaLlamada == nil {
		t.Fatal("«Unirse» no abre la llamada del canal")
	}
	if c.ConFranjaLlamada() {
		t.Error("dentro de la llamada se sigue ofreciendo unirse")
	}
	sa.ventanaLlamada.Close()
	if !c.ConFranjaLlamada() {
		t.Error("al colgar no vuelve la franja con la llamada aún viva")
	}

	sa.alLlegar(api.RoomsEvent{Type: "call_ended", ChannelID: "c1"})
	if c.ConFranjaLlamada() || c.Llamada.Visible() || canalDelCarril(t, sa, "c1").EnLlamada {
		t.Error("`call_ended` no apaga la franja ni el punto")
	}
	sa.alLlegar(api.RoomsEvent{Type: "call_started", ChannelID: "c1"})
	if c.Llamada.Participantes != 1 || !c.ConFranjaLlamada() {
		t.Errorf("`call_started` en el abierto no la enciende al momento: %d", c.Llamada.Participantes)
	}

	sa.EnSegundoPlano()
	if sa.pararLlamadaActiva != nil {
		t.Error("con la app de fondo el sondeo sigue programado")
	}
	antes := s.veces("LlamadaActiva(c1)")
	sa.EnPrimerPlano()
	if s.veces("LlamadaActiva(c1)") != antes+1 || sa.pararLlamadaActiva == nil {
		t.Error("al volver no se pregunta en el acto ni se reprograma el sondeo")
	}
}

// Al cambiar a un canal sin llamada (el 404 de su sala es 0 en el cliente) la franja
// de c1 no se queda puesta en el nuevo.
func TestLaFranjaDeLlamadaSeVaAlCambiarDeCanal(t *testing.T) {
	sa, _, _, _ := salaConVigia(t, map[string]int{"c1": 2})
	if !sa.P.Canal.ConFranjaLlamada() {
		t.Fatal("la franja no sale en c1")
	}
	sa.Elegir(canalDelCarril(t, sa, "d1"))
	if sa.P.Canal.ConFranjaLlamada() {
		t.Error("el directo sin llamada hereda la franja de c1")
	}
}

// Con el dedo, «Unirse» sube a 44 px (el `pointer-coarse` de Button.tsx) y la franja
// crece con él: conserva su `py-1.5` y el botón no se sale por abajo encima del
// historial.
//
// VISTO EN ROJO (2026-11-09): con el alto fijo del botón `sm` (32 px) la franja medía
// 45 y el botón de 44 llenaba la línea entera, sin relleno.
func TestLaFranjaDeLlamadaDejaSitioAlDedo(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	f := NuevaFranjaLlamada(traductor(t))
	f.Participantes = 2
	test.WidgetRenderer(f)
	f.Resize(fyne.NewSize(400, MedidaFranjaLlamada().Height))
	b := f.Unirse()
	if b.Size().Height < 44 {
		t.Fatalf("«Unirse» mide %v de alto con el dedo", b.Size().Height)
	}
	if b.Position().Y < RellenoFranjaLlamadaY || b.Position().Y+b.Size().Height > f.Size().Height-tema.Borde-RellenoFranjaLlamadaY {
		t.Errorf("«Unirse» (%v, alto %v) no deja el `py-1.5` en una franja de %v", b.Position().Y, b.Size().Height, f.Size().Height)
	}
}
