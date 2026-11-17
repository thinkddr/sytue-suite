// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Las cuatro reglas de `avisoDeLlamada` (lib/llamadaEntrante.ts), una por fila: ni el
// canal abierto, ni ya en llamada, ni un canal silenciado, ni uno que no es suyo. Lo
// que sí avisa dice si es un directo, que es lo que decide si suena.
func TestAvisoDeLlamadaEntranteSusCuatroReglas(t *testing.T) {
	canales := []Canal{
		{ID: "c1", Nombre: "equipo"},
		{ID: "d1", Nombre: "Ana Ruiz", Clase: Directo},
		{ID: "mudo", Nombre: "ruido", Silenciado: true},
	}
	casos := []struct {
		nombre string
		s      SituacionLlamada
		quiero *AvisoEntrante
	}{
		{"un directo que no miras", SituacionLlamada{CanalID: "d1", Abierto: "c1"}, &AvisoEntrante{CanalID: "d1", Etiqueta: "Ana Ruiz", Directo: true}},
		{"un canal que no miras", SituacionLlamada{CanalID: "c1", Abierto: "d1"}, &AvisoEntrante{CanalID: "c1", Etiqueta: "equipo"}},
		{"sin canal abierto", SituacionLlamada{CanalID: "d1"}, &AvisoEntrante{CanalID: "d1", Etiqueta: "Ana Ruiz", Directo: true}},
		{"el canal abierto", SituacionLlamada{CanalID: "d1", Abierto: "d1"}, nil},
		{"ya en una llamada", SituacionLlamada{CanalID: "d1", Abierto: "c1", EnLlamada: true}, nil},
		{"un canal silenciado", SituacionLlamada{CanalID: "mudo", Abierto: "c1"}, nil},
		{"un canal ajeno", SituacionLlamada{CanalID: "otro", Abierto: "c1"}, nil},
		{"sin canal", SituacionLlamada{Abierto: "c1"}, nil},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			c.s.Canales = canales
			got, ok := AvisoDeLlamada(c.s)
			switch {
			case c.quiero == nil && ok:
				t.Errorf("avisa (%+v) y no debía", got)
			case c.quiero != nil && (!ok || got != *c.quiero):
				t.Errorf("da %+v (%v) y era %+v", got, ok, *c.quiero)
			}
		})
	}
}

// timbreDePrueba cuenta cuántas veces suena y cuántas se para.
type timbreDePrueba struct{ suena, paradas int }

func (tb *timbreDePrueba) sonar() func() {
	tb.suena++
	return func() { tb.paradas++ }
}

func (tb *timbreDePrueba) sonando() bool { return tb.suena > tb.paradas }

// Una llamada por el directo que no miras saca la tarjeta arriba a la derecha sin
// quitarle el foco a la caja de escribir, suena y, con la ventana de fondo, sale al
// sistema. «Contestar» abre el directo, entra en su llamada y calla el timbre.
//
// VISTO EN ROJO (2026-11-09): sin alLlamadaEntrante en alLlegar no sale nada, y fallan
// las tres pruebas de la sala.
func TestLaLlamadaEntranteDeUnDirectoSuenaYSeContesta(t *testing.T) {
	sa, _, _, w := salaConVigia(t, nil)
	tb := &timbreDePrueba{}
	sa.Timbre = tb.sonar
	var sistema []string
	sa.AvisarSistema = func(titulo, cuerpo string) { sistema = append(sistema, titulo+" · "+cuerpo) }
	foco := sa.P.Canal.Compositor.area
	w.Canvas().Focus(foco)
	sa.EnSegundoPlano()

	sa.alLlegar(api.RoomsEvent{Type: "call_started", ChannelID: "d1"})
	e := sa.P.Entrante
	if !e.Visible() || e.Aviso.CanalID != "d1" || !e.Aviso.Directo {
		t.Fatalf("la llamada del directo no se avisa: %+v", e.Aviso)
	}
	if got := e.AccessibilityLabel(); got != "Llamada de Ana Ruiz" {
		t.Errorf("la tarjeta se anuncia como %q", got)
	}
	if !tb.sonando() {
		t.Error("un directo que llama no suena")
	}
	if len(sistema) != 1 || sistema[0] != "Llamada de Ana Ruiz · te está llamando" {
		t.Errorf("con la ventana de fondo el sistema avisa %q", sistema)
	}
	if w.Canvas().Focused() != foco {
		t.Error("la llamada le roba el foco a lo que estabas escribiendo")
	}
	// Arriba a la derecha, con su `top-4 right-4` y su `w-72`.
	tam := w.Canvas().Size()
	if p := e.Position(); p.X+e.Size().Width != tam.Width-MargenLlamadaEntrante || p.Y != MargenLlamadaEntrante ||
		e.Size().Width != AnchoLlamadaEntrante {
		t.Errorf("la tarjeta está en %v y mide %v en una ventana de %v", p, e.Size(), tam)
	}
	sa.EnPrimerPlano()
	capturarCompositor(t, w, "canal.llamada-entrante")

	test.Tap(e.Contestar())
	if sa.abierto != "d1" || sa.ventanaLlamada == nil {
		t.Fatalf("«Contestar» no abre el directo (%q) ni entra en la llamada", sa.abierto)
	}
	if e.Visible() || tb.sonando() {
		t.Error("tras contestar sigue el aviso o el timbre")
	}
}

// `call_ended` retira el aviso y calla el timbre; «Rechazar» también. Una llamada de
// canal se ve pero no suena ni sale al sistema; y la del canal abierto, ni se ve.
//
// VISTO EN ROJO (2026-11-09): sin retirarEntrante en `abrir`, abrir a mano el canal que
// llama deja la tarjeta puesta encima de su propia franja.
func TestLaLlamadaEntranteSeVaYLaDeCanalNoSuena(t *testing.T) {
	sa, _, _, _ := salaConVigia(t, nil)
	tb := &timbreDePrueba{}
	sa.Timbre = tb.sonar
	avisos := 0
	sa.AvisarSistema = func(string, string) { avisos++ }
	sa.EnSegundoPlano()
	e := sa.P.Entrante

	sa.alLlegar(api.RoomsEvent{Type: "call_started", ChannelID: "d1"})
	sa.alLlegar(api.RoomsEvent{Type: "call_ended", ChannelID: "c1"})
	if !e.Visible() || !tb.sonando() {
		t.Fatal("el fin de OTRA llamada retira el aviso del directo")
	}
	sa.alLlegar(api.RoomsEvent{Type: "call_ended", ChannelID: "d1"})
	if e.Visible() || tb.sonando() {
		t.Error("`call_ended` no retira el aviso ni para el timbre")
	}

	sa.alLlegar(api.RoomsEvent{Type: "call_started", ChannelID: "d1"})
	test.Tap(e.Rechazar())
	if e.Visible() || tb.sonando() || sa.abierto != "c1" || sa.ventanaLlamada != nil {
		t.Error("«Rechazar» no retira el aviso, no calla o te mete en la llamada")
	}

	// Del canal abierto no se avisa: ya lo dice su franja.
	sa.alLlegar(api.RoomsEvent{Type: "call_started", ChannelID: "c1"})
	if e.Visible() {
		t.Error("la llamada del canal abierto también salta como entrante")
	}
	sa.alLlegar(api.RoomsEvent{Type: "call_ended", ChannelID: "c1"})

	sa.Elegir(canalDelCarril(t, sa, "d1"))
	antes, sistema := tb.suena, avisos
	sa.alLlegar(api.RoomsEvent{Type: "call_started", ChannelID: "c1"})
	if !e.Visible() || e.Aviso.Directo || e.AccessibilityLabel() != "Llamada en equipo" {
		t.Fatalf("la llamada del canal no se ve como tal: %+v", e.Aviso)
	}
	if tb.suena != antes || avisos != sistema {
		t.Error("una llamada de canal suena o sale al sistema")
	}
	// Abrir el canal que llama es darse por enterado.
	sa.Elegir(canalDelCarril(t, sa, "c1"))
	if e.Visible() {
		t.Error("abrir el canal que llama deja el aviso puesto")
	}
}

// Ya en una llamada no se avisa de otra, y con «Pausar avisos» la del directo suena en
// la app pero no sale al sistema.
func TestLaLlamadaEntranteRespetaLaLlamadaYLaPausa(t *testing.T) {
	sa, _, _, _ := salaConVigia(t, nil)
	tb := &timbreDePrueba{}
	sa.Timbre = tb.sonar
	avisos := 0
	sa.AvisarSistema = func(string, string) { avisos++ }
	sa.LlamarEnCanal()
	if sa.ventanaLlamada == nil {
		t.Fatal("no se entra en la llamada de c1")
	}
	sa.alLlegar(api.RoomsEvent{Type: "call_started", ChannelID: "d1"})
	if sa.P.Entrante.Visible() || tb.suena != 0 {
		t.Error("dentro de una llamada se avisa de otra")
	}
	sa.ventanaLlamada.Close()
	sa.alLlegar(api.RoomsEvent{Type: "call_ended", ChannelID: "d1"})

	sa.pausa.Store(sa.ahora().Add(time.Hour).UnixNano())
	sa.EnSegundoPlano()
	sa.alLlegar(api.RoomsEvent{Type: "call_started", ChannelID: "d1"})
	if !sa.P.Entrante.Visible() || !tb.sonando() || avisos != 0 {
		t.Errorf("con la pausa: aviso %v, timbre %v, sistema %d", sa.P.Entrante.Visible(), tb.sonando(), avisos)
	}
	sa.Olvidar()
	if sa.P.Entrante.Visible() || tb.sonando() {
		t.Error("al salir de la sesión sigue el aviso o el timbre")
	}
}

// Con el dedo, los dos botones suben a 44 px y la tarjeta crece con ellos.
func TestLaLlamadaEntranteDejaSitioAlDedo(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	l := NuevaLlamadaEntrante(traductor(t))
	l.Aviso = AvisoEntrante{CanalID: "d1", Etiqueta: "Ana Ruiz con un nombre larguísimo que no cabe", Directo: true}
	l.Show()
	test.WidgetRenderer(l)
	l.Resize(fyne.NewSize(AnchoLlamadaEntrante, l.MinSize().Height))
	for _, b := range []*ui.Boton{l.Contestar(), l.Rechazar()} {
		if b.Size().Height < 44 || b.Position().Y+b.Size().Height > l.Size().Height-RellenoLlamadaEntrante {
			t.Errorf("un botón mide %v en %v en una tarjeta de %v", b.Size(), b.Position(), l.Size())
		}
	}
	if l.Rechazar().Position().X <= l.Contestar().Position().X {
		t.Error("«Contestar» no va primero")
	}
}

// Entrar en OTRA llamada mientras suena la entrante (el «Llamar» de la cabecera del
// canal abierto) la retira y calla el timbre: no se mezcla con la antesala.
//
// VISTO EN ROJO (2026-11-09): sin retirarEntrante en abrirLlamada, el directo sigue
// sonando dentro de la llamada del canal.
func TestEntrarEnOtraLlamadaCallaLaEntrante(t *testing.T) {
	sa, _, _, _ := salaConVigia(t, nil)
	tb := &timbreDePrueba{}
	sa.Timbre = tb.sonar
	sa.alLlegar(api.RoomsEvent{Type: "call_started", ChannelID: "d1"})
	if !sa.P.Entrante.Visible() || !tb.sonando() {
		t.Fatal("la llamada del directo no se avisa")
	}
	sa.LlamarEnCanal()
	if sa.ventanaLlamada == nil {
		t.Fatal("no se entra en la llamada de c1")
	}
	if sa.P.Entrante.Visible() || tb.sonando() {
		t.Error("dentro de la llamada del canal sigue el aviso o el timbre del directo")
	}
}
