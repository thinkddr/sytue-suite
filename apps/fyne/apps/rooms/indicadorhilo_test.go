// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El hilo visto desde el canal (D-ROOM-05): el indicador con avatares y «Última
// respuesta hace…», la línea «Respondió en un hilo» de una respuesta difundida y la
// casilla «Enviar también a #canal» del hilo. Es IndicadorHilo.tsx y ThreadPanel.tsx.

// TestElIndicadorDeHiloLlevaAvataresYLaUltimaRespuesta: con quienes respondieron, el
// botón lleva sus avatares en vez del chevron, crece a su alto y dice cuándo fue la
// última respuesta; sin ellos, el de siempre.
func TestElIndicadorDeHiloLlevaAvataresYLaUltimaRespuesta(t *testing.T) {
	antes := relojHilo
	relojHilo = func() time.Time { return time.Date(2026, 11, 10, 10, 0, 0, 0, time.Local) }
	t.Cleanup(func() { relojHilo = antes })
	tr := traductor(t)
	m := Mensaje{
		ID: "m", Autor: Autor{ID: "a", Nombre: "Ana"}, Cuerpo: "hola", Respuestas: 4,
		Creado:          time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local),
		UltimaRespuesta: time.Date(2026, 11, 10, 9, 55, 0, 0, time.Local),
		Participantes:   []string{"Bea Gil", "Carl Ruiz"},
	}
	f := NuevaFilaMensaje(tr, "es", m, false, nil)
	f.AlAbrirHilo = func(Mensaje) {}
	w := test.NewWindow(f)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(600, 140))
	f.Resize(fyne.NewSize(600, MedidaFilaMensaje(f, 600).Height))

	e := f.enlaceHilo
	if e == nil || len(e.caras) != 2 || e.glifo != nil {
		t.Fatalf("el indicador no lleva los dos avatares en vez del chevron: %+v", e)
	}
	if e.Ultima != "Última respuesta hace 5 min" || e.ultima == nil {
		t.Errorf("la última respuesta dice %q", e.Ultima)
	}
	if e.Nombre != "4 respuestas Última respuesta hace 5 min" {
		t.Errorf("el nombre accesible es %q: la cuenta y la hora, sin los avatares", e.Nombre)
	}
	if got, quiero := MedidaFilaMensaje(f, 600).Height, AltoFilaMensaje(false)+AireReacciones+AltoEnlaceHiloCaras; got != quiero {
		t.Errorf("la fila con avatares mide %v y son %v", got, quiero)
	}
	// Solapados: el segundo empieza antes de que acabe el primero, y el texto después.
	a, b := e.caras[0].Position(), e.caras[1].Position()
	lado := e.caras[0].Size().Width
	if b.X >= a.X+lado || b.X <= a.X {
		t.Errorf("los avatares no se solapan: %v y %v con lado %v", a, b, lado)
	}
	if e.texto.Position().X < b.X+lado {
		t.Error("«4 respuestas» pisa los avatares")
	}

	sin := NuevaFilaMensaje(tr, "es", Mensaje{ID: "n", Autor: m.Autor, Cuerpo: "x", Respuestas: 1, Creado: m.Creado}, false, nil)
	sin.AlAbrirHilo = func(Mensaje) {}
	test.WidgetRenderer(sin)
	if sin.enlaceHilo == nil || sin.enlaceHilo.glifo == nil || sin.enlaceHilo.Ultima != "" ||
		MedidaFilaMensaje(sin, 600).Height != AltoFilaMensaje(false)+AireReacciones+AltoEnlaceHilo {
		t.Error("sin indicador del servicio el enlace tiene que ser el de siempre")
	}
}

// TestLaRespuestaDifundidaDiceQueEsDeUnHilo: una respuesta mandada también al canal
// lleva «Respondió en un hilo · Ver el hilo» encima del texto, y «Ver el hilo» abre el
// hilo de su raíz con la raíz del feed.
func TestLaRespuestaDifundidaDiceQueEsDeUnHilo(t *testing.T) {
	s := servicioConHilo()
	raiz := s.mensajes["c1"][3]
	padre := raiz.ID
	s.mensajes["c1"] = append(s.mensajes["c1"], api.RoomsMessage{
		ID: "r3", ChannelID: "c1", ParentID: &padre, AlsoInChannel: true, AuthorSub: "beto", AuthorName: "Beto",
		Body: "Lo cuento aquí también", CreatedAt: raiz.CreatedAt.Add(3 * time.Minute),
	})
	ultima := raiz.CreatedAt.Add(3 * time.Minute)
	s.mensajes["c1"][3].LastReplyAt = &ultima
	s.mensajes["c1"][3].ThreadParticipants = []api.RoomsThreadParticipant{{Sub: "beto", Name: "Beto"}, {Sub: "ana", Name: "Ana"}}
	sa, _, ventana := salaEn(t, s, 1280)
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial
	difundida := h.Mensajes[len(h.Mensajes)-1]
	if difundida.RaizHilo != padre {
		t.Fatalf("la respuesta difundida no sabe su raíz: %+v", difundida)
	}
	f := NuevaFilaMensaje(sa.P.T, "es", difundida, false, nil)
	f.AlAbrirHilo = sa.AbrirHilo
	w := test.NewWindow(f)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(600, 160))
	f.Resize(fyne.NewSize(600, MedidaFilaMensaje(f, 600).Height))
	if f.enHilo == nil {
		t.Fatal("la respuesta difundida no lleva «Respondió en un hilo»")
	}
	if f.enHilo.Position().Y >= f.cuerpo.Position().Y {
		t.Error("«Respondió en un hilo» va debajo del texto y no encima")
	}
	normal := NuevaFilaMensaje(sa.P.T, "es", h.Mensajes[0], false, nil)
	normal.AlAbrirHilo = sa.AbrirHilo
	if got, quiero := MedidaFilaMensaje(f, 600).Height-MedidaFilaMensaje(normal, 600).Height, f.altoEnHilo(); got < quiero {
		t.Errorf("la línea no suma su alto a la fila: %v < %v", got, quiero)
	}

	// Lo que se ve de verdad: el canal con el indicador y la difundida, y el hilo abierto.
	sa.AbrirHilo(difundida)
	capturarCompositorEn(t, ventana, os.Getenv("SYTUE_CAPTURAS"), "rooms-hilo-tambien-al-canal")
	sa.CerrarHilo()

	test.Tap(f.enHilo.ver)
	if sa.raizHilo != padre || sa.hilo == nil || sa.hilo.Raiz.ID != padre || sa.hilo.Raiz.Cuerpo != raiz.Body {
		t.Fatalf("«Ver el hilo» abre %q y no el hilo de la raíz %q", sa.raizHilo, padre)
	}
}

// TestEnviarTambienAlCanalViajaYSeDesmarca: la casilla de debajo de la caja del hilo
// manda la respuesta también al canal y se desmarca tras enviar; otro hilo la trae
// limpia.
func TestEnviarTambienAlCanalViajaYSeDesmarca(t *testing.T) {
	s := servicioConHilo()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.AbrirHilo(sa.P.Canal.Historial.Mensajes[3])
	h := sa.hilo
	if h.casilla == nil || h.casilla.Etiqueta != "Enviar también a #equipo" || !h.casilla.Visible() {
		t.Fatalf("el hilo no lleva «Enviar también a #equipo»: %+v", h.casilla)
	}
	if y := h.casilla.Position().Y; y < h.Compositor.Position().Y+h.Compositor.Size().Height {
		t.Error("la casilla va encima de la caja y no debajo")
	}
	test.Tap(h.casilla)
	if !h.TambienAlCanal {
		t.Fatal("marcar la casilla no la marca")
	}
	sa.Responder("lo digo también en el canal")
	if n := len(s.envios); n == 0 || !s.envios[n-1].AlsoInChannel {
		t.Fatalf("la respuesta no viaja con also_in_channel: %+v", s.envios)
	}
	if h.TambienAlCanal || h.casilla.Marcada {
		t.Error("tras enviar, la casilla sigue marcada")
	}
	sa.Responder("y esta solo en el hilo")
	if n := len(s.envios); s.envios[n-1].AlsoInChannel {
		t.Error("sin marcar, la respuesta va también al canal")
	}

	// Si el envío falla, vuelve el texto y también la casilla: reintentar es pulsar Enviar.
	s.errReply = errors.New("sin red")
	h.PonerTambienAlCanal(true)
	sa.Responder("esta no sale")
	if !h.TambienAlCanal || !h.casilla.Marcada || h.Compositor.Texto() != "esta no sale" {
		t.Errorf("tras fallar, casilla=%v texto=%q", h.TambienAlCanal, h.Compositor.Texto())
	}
	s.errReply = nil

	h.PonerTambienAlCanal(true)
	sa.AbrirHilo(sa.P.Canal.Historial.Mensajes[3])
	if sa.hilo.TambienAlCanal {
		t.Error("abrir otro hilo arrastra la casilla marcada")
	}
}
