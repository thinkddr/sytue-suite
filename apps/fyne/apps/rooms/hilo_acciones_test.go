// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioConHiloPropio es servicioConHilo con una respuesta reaccionada y otra mía,
// que es la que se puede editar y borrar.
func servicioConHiloPropio() *servicioFalso {
	s := servicioConHilo()
	raiz := s.hilo.Root
	s.hilo.Replies[0].Reactions = []api.RoomsReaction{{Emoji: "🎉", Count: 1, Subs: []string{"ana"}}}
	s.hilo.Replies = append(s.hilo.Replies, api.RoomsMessage{
		ID: "r3", AuthorSub: "yo", AuthorName: "Yo", Body: "Lo miro", CreatedAt: raiz.CreatedAt.Add(3 * time.Minute),
	})
	return s
}

// hiloAbierto arranca la sala y abre el hilo de «vale».
func hiloAbierto(t *testing.T, s *servicioFalso) (*Sala, fyne.Window) {
	t.Helper()
	sa, _, w := salaEn(t, s, 1280)
	sa.Arrancar(context.Background())
	sa.AbrirHilo(sa.P.Canal.Historial.Mensajes[3])
	if sa.hilo == nil || sa.hilo.Estado != Listo {
		t.Fatal("el hilo no se abrió")
	}
	return sa, w
}

func filaDelHilo(t *testing.T, h *PanelHilo, id string) *FilaMensaje {
	t.Helper()
	f := h.Fila(id)
	if f == nil {
		t.Fatalf("el hilo no pinta %q", id)
	}
	test.WidgetRenderer(f)
	return f
}

// TestElHiloLlevaBarraYReaccionesEnCadaMensaje es `ThreadPanel` tras ROOM-10: la raíz y
// cada respuesta llevan la barra de acciones (reacciones rápidas, emoji y «…», sin
// «Responder en un hilo») y sus chips de reacción con el «+», como en el canal.
//
// VISTO EN ROJO (2026-11-12): las filas del hilo nacían sin barra y sin tira de
// reacciones (`EnHilo` las quitaba) y `PanelHilo` no tenía acciones que darles.
func TestElHiloLlevaBarraYReaccionesEnCadaMensaje(t *testing.T) {
	sa, _ := hiloAbierto(t, servicioConHiloPropio())
	h := sa.hilo
	for _, id := range []string{"m4", "r1", "r2", "r3"} {
		f := filaDelHilo(t, h, id)
		if f.barra == nil || f.barra.AlReaccionar == nil || f.barra.AlEmojis == nil || f.barra.Menu == nil {
			t.Fatalf("la fila %q del hilo no lleva la barra entera: %+v", id, f.barra)
		}
		if f.barra.AlHilo != nil {
			t.Errorf("la fila %q ofrece «Responder en un hilo» dentro del propio hilo", id)
		}
	}
	r1 := filaDelHilo(t, h, "r1")
	if len(r1.reacciones) != 1 || r1.masReaccion == nil {
		t.Fatalf("la respuesta reaccionada pinta %d chips y «+» %v", len(r1.reacciones), r1.masReaccion != nil)
	}
	if raiz := filaDelHilo(t, h, "m4"); len(raiz.reacciones) != 1 {
		t.Errorf("la raíz del hilo pinta %d chips y tiene una reacción", len(raiz.reacciones))
	}
	// La barra sale en su sitio del hilo: `-top-2 right-3`, no el del canal.
	if p, ancho := r1.barra.Position(), r1.Size().Width; p.Y != -ArribaBarraHilo ||
		p.X != ancho-DerechaBarraHilo-r1.barra.MinSize().Width {
		t.Errorf("la barra del hilo va en %v sobre una fila de %v", p, ancho)
	}
	// Apagada hasta que el puntero entra, y con el fondo del `hover`.
	if !r1.barra.Apagada {
		t.Error("la barra del hilo se ve sin el puntero encima")
	}
	r1.MouseIn(nil)
	if r1.barra.Apagada || r1.fondo.FillColor == nil {
		t.Error("con el puntero encima la barra del hilo sigue apagada")
	}
	r1.MouseOut()
}

// TestElMenuDelHiloEsElDelCanalSinLoDelCanal: las mismas entradas y en el mismo
// orden, sin «Seguir mensaje», «Marcar como no leído» ni «Visto por»; editar y borrar
// solo en lo propio; denunciar y bloquear en lo ajeno.
func TestElMenuDelHiloEsElDelCanalSinLoDelCanal(t *testing.T) {
	sa, _ := hiloAbierto(t, servicioConHiloPropio())
	tr := sa.P.T
	mio := rotulos(sa.hilo.Menu(sa.hilo.Respuestas[2]))
	quiero := []string{
		tr.T("message.pin", nil), tr.T("action.save", nil), tr.T("message.copyText", nil),
		tr.T("message.copyLink", nil), tr.T("message.saveToNotes", nil), tr.T("message.forward", nil),
		tr.T("action.edit", nil), tr.T("action.delete", nil),
	}
	if !slices.Equal(mio, quiero) {
		t.Errorf("el «…» de una respuesta mía es %q y tenía que ser %q", mio, quiero)
	}
	ajeno := rotulos(sa.hilo.Menu(sa.hilo.Respuestas[0]))
	for _, no := range []string{tr.T("action.edit", nil), tr.T("action.delete", nil), tr.T("message.markUnread", nil), tr.T("message.follow", nil)} {
		if slices.Contains(ajeno, no) {
			t.Errorf("el «…» de una respuesta ajena ofrece %q: %q", no, ajeno)
		}
	}
	// El del canal sigue teniendo lo suyo.
	if canal := rotulos(sa.menuDe(sa.P.Canal.Historial.Mensajes[0])); !slices.Contains(canal, tr.T("message.markUnread", nil)) {
		t.Errorf("el «…» del canal perdió «Marcar como no leído»: %q", canal)
	}
}

// TestReaccionarEnElHilo: la reacción rápida de la barra y el chip conmutan contra el
// servicio y el hilo se relee con el resultado.
func TestReaccionarEnElHilo(t *testing.T) {
	s := servicioConHiloPropio()
	sa, _ := hiloAbierto(t, s)
	h := sa.hilo
	filaDelHilo(t, h, "r2").barra.AlReaccionar("👍")
	if !s.pidio("React(r2,👍,poner)") {
		t.Fatalf("reaccionar en el hilo no llegó al servicio: %v", s.llamadas)
	}
	r2 := filaDelHilo(t, h, "r2")
	if len(r2.Mensaje.Reacciones) != 1 || !r2.Mensaje.Reacciones[0].Mia || len(r2.reacciones) != 1 {
		t.Fatalf("tras reaccionar, la respuesta tiene %+v", r2.Mensaje.Reacciones)
	}
	// Pulsar el chip propio la quita.
	chip := r2.reacciones[0]
	if p, ok := chip.(*ui.Pista); ok {
		chip = p.Hijo // con «quién reaccionó», la píldora va dentro de su pista
	}
	test.Tap(chip.(fyne.Tappable))
	if !s.pidio("React(r2,👍,quitar)") || len(filaDelHilo(t, h, "r2").Mensaje.Reacciones) != 0 {
		t.Errorf("el chip propio no quita la reacción: %v", s.llamadas)
	}
	// Reaccionar a la raíz desde el hilo relee también el canal, que la enseña.
	antes := s.veces("Messages(c1)")
	filaDelHilo(t, h, "m4").barra.AlReaccionar("🎉")
	if !s.pidio("React(m4,🎉,poner)") || s.veces("Messages(c1)") <= antes {
		t.Errorf("reaccionar a la raíz en el hilo no relee el canal: %v", s.llamadas)
	}
}

// TestEditarYBorrarEnElHilo: «Editar» del «…» abre la caja en la propia fila del hilo;
// Intro guarda contra el servicio con «(editado)»; Esc cierra y devuelve el foco a la
// barra; «Borrar» confirma y la respuesta sale del hilo.
func TestEditarYBorrarEnElHilo(t *testing.T) {
	s := servicioConHiloPropio()
	sa, w := hiloAbierto(t, s)
	h := sa.hilo
	tr := sa.P.T
	entrada := func(m Mensaje, rotulo string) *ui.ElementoMenu {
		for _, e := range h.Menu(m) {
			if it, ok := e.(*ui.ElementoMenu); ok && it.Texto == rotulo {
				return it
			}
		}
		t.Fatalf("el «…» de %q no tiene %q", m.ID, rotulo)
		return nil
	}

	test.Tap(entrada(h.Respuestas[2], tr.T("action.edit", nil)))
	f := filaDelHilo(t, h, "r3")
	if h.EditandoID != "r3" || !f.Editando || f.edicion == nil {
		t.Fatalf("«Editar» no abre la caja en la fila del hilo: editando %q", h.EditandoID)
	}
	if foco := w.Canvas().Focused(); foco != f.edicion.area {
		t.Errorf("la caja de editar nace sin el foco: %T", foco)
	}
	// Esc: se cierra sin tocar nada y el foco vuelve a la barra de esa fila.
	sa.CancelarEdicionHilo()
	if h.EditandoID != "" || s.pidio("Edit(r3,Lo miro)") {
		t.Fatalf("cancelar deja editando %q", h.EditandoID)
	}
	if foco := w.Canvas().Focused(); foco != filaDelHilo(t, h, "r3").barra {
		t.Errorf("al cancelar el foco queda en %T y tenía que volver a la barra", foco)
	}

	test.Tap(entrada(h.Respuestas[2], tr.T("action.edit", nil)))
	filaDelHilo(t, h, "r3").AlGuardarEdicion(h.Respuestas[2], "Lo miro ahora")
	if !s.pidio("Edit(r3,Lo miro ahora)") {
		t.Fatalf("guardar no edita en el servicio: %v", s.llamadas)
	}
	if r := filaDelHilo(t, h, "r3").Mensaje; r.Cuerpo != "Lo miro ahora" || !r.Editado || h.EditandoID != "" {
		t.Errorf("tras guardar la respuesta dice %q, editado %v, editando %q", r.Cuerpo, r.Editado, h.EditandoID)
	}

	// Borrar pide confirmación; confirmada, la respuesta sale y el canal se relee.
	test.Tap(entrada(h.Respuestas[2], tr.T("action.delete", nil)))
	if s.pidio("Delete(r3)") {
		t.Fatal("«Borrar» borra sin preguntar")
	}
	antes := s.veces("Messages(c1)")
	sa.Borrar(h.Respuestas[2])
	if !s.pidio("Delete(r3)") || len(h.Respuestas) != 2 || h.Fila("r3") != nil {
		t.Fatalf("borrar en el hilo deja %d respuestas: %v", len(h.Respuestas), s.llamadas)
	}
	if s.veces("Messages(c1)") <= antes {
		t.Error("borrar una respuesta no relee el canal para bajar «N respuestas»")
	}
}

// TestEditarEnElHiloSeDeshaceSiElServicioDiceQueNo: el texto nuevo se pinta ya y
// vuelve al de antes si la edición falla.
func TestEditarEnElHiloSeDeshaceSiElServicioDiceQueNo(t *testing.T) {
	s := servicioConHiloPropio()
	sa, _ := hiloAbierto(t, s)
	h := sa.hilo
	s.errAccion = &api.Error{Status: 500}
	sa.EditarEnHilo(h.Respuestas[2])
	sa.GuardarEdicionHilo(h.Respuestas[2], "otra cosa")
	if r := filaDelHilo(t, h, "r3").Mensaje; r.Cuerpo != "Lo miro" || r.Editado {
		t.Errorf("con el servicio caído la respuesta dice %q, editado %v", r.Cuerpo, r.Editado)
	}
}

// TestRecorridoConHilo pulsa todo lo del canal con el hilo abierto —barras, chips y
// menús de sus mensajes incluidos— y comprueba que no deja capas abiertas.
func TestRecorridoConHilo(t *testing.T) {
	_, w := hiloAbierto(t, servicioConHiloPropio())
	recorrido.Comprobar(t, w, 400)
}

// TestElHiloConAccionesSeVe deja el PNG del hilo con la barra de una respuesta a la
// vista y sus reacciones, cuando se pide.
func TestElHiloConAccionesSeVe(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURA_SALA")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURA_SALA")
	}
	sa, w := hiloAbierto(t, servicioConHiloPropio())
	filaDelHilo(t, sa.hilo, "r1").MouseIn(nil)
	f, err := os.Create(filepath.Join(dir, "sala-hilo-acciones-1280.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}

// TestReleerElHiloNoBorraLaEdicionAMedias: el hilo se relee con cada evento del canal
// (una reacción ajena, una respuesta nueva); la caja de editar sigue con lo escrito y
// con el foco.
//
// VISTO EN ROJO (2026-11-12): la fila se rehacía con el cuerpo original y el foco se
// quedaba en una caja que ya no estaba en el lienzo.
func TestReleerElHiloNoBorraLaEdicionAMedias(t *testing.T) {
	sa, w := hiloAbierto(t, servicioConHiloPropio())
	h := sa.hilo
	sa.EditarEnHilo(h.Respuestas[2])
	f := filaDelHilo(t, h, "r3")
	f.edicion.area.PonerTexto("Lo miro mañana")
	sa.cargarHilo(false)
	g := filaDelHilo(t, h, "r3")
	if g == f || g.edicion == nil {
		t.Fatal("releer el hilo no rehízo la fila en edición")
	}
	if got := g.edicion.Texto(); got != "Lo miro mañana" {
		t.Errorf("tras releer el hilo la caja dice %q", got)
	}
	if foco := w.Canvas().Focused(); foco != g.edicion.area {
		t.Errorf("tras releer el hilo el foco queda en %T", foco)
	}
}
