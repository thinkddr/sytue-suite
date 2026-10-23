// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func TestAutoguardadoEncoladoPerteneceAlRedactorYALaSesion(t *testing.T) {
	for _, caso := range []string{"vigente", "otra cuenta", "cerrado", "sustituido", "enviando", "ventana cerrada"} {
		t.Run(caso, func(t *testing.T) {
			s := servicioConTodas()
			b, _, pendientes := buzonConVentana(t, s, fyne.NewSize(1280, 744))
			b.CambiarSesion(context.Background(), "emisor\x00persona-a\x00org")
			b.abrirRedactor(Borrador{Asunto: "Privado de A", Cuerpo: "Contenido de A"})
			rd := b.redaccion
			b.programarAutoguardado(rd)
			// Stop no puede retirar el callback que el reloj ya entregó a fyne.Do.
			encolado := (*pendientes)[len(*pendientes)-1]
			switch caso {
			case "otra cuenta":
				b.CambiarSesion(context.Background(), "emisor\x00persona-b\x00org")
			case "cerrado":
				b.cerrar(rd)
			case "sustituido":
				b.cerrar(rd)
				b.abrirRedactor(Borrador{Asunto: "Otro redactor"})
			case "enviando":
				rd.enviando = true
			case "ventana cerrada":
				b.Parar()
			}
			encolado()
			esperados := 0
			if caso == "vigente" {
				esperados = 1
			}
			if got := s.pidio("SaveDraft"); got != esperados {
				t.Fatalf("SaveDraft=%d, esperado %d: se ejecutó un autoguardado retirado", got, esperados)
			}
		})
	}
}

func confirmarEncolado(t *testing.T, c fyne.Canvas, rotulo string) func() {
	t.Helper()
	var pulsar func()
	var visitar func(fyne.CanvasObject)
	visitar = func(o fyne.CanvasObject) {
		if boton, ok := o.(*ui.Boton); ok && boton.Texto == rotulo {
			pulsar = boton.AlPulsar
		}
		for _, h := range hijos(o) {
			visitar(h)
		}
	}
	visitar(c.Overlays().Top())
	if pulsar == nil {
		t.Fatalf("no se encontró la confirmación %q", rotulo)
	}
	return pulsar
}

func TestConfirmacionesRetiradasNoEscribenConOtraSesion(t *testing.T) {
	for _, accion := range []string{"enviar", "descartar", "guardar", "cerrar guardando", "programar"} {
		t.Run(accion, func(t *testing.T) {
			s := servicioConTodas()
			b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
			b.CambiarSesion(context.Background(), "emisor\x00persona-a\x00org")
			b.abrirRedactor(Borrador{ID: "borrador-a", Para: "grace@example.org", Asunto: "De A", Cuerpo: "Te adjunto el presupuesto."})
			rd := b.redaccion
			var encolado func()
			switch accion {
			case "enviar":
				b.enviar(rd, false)
				encolado = confirmarEncolado(t, w.Canvas(), b.P.T.T("compose.noAttachmentSend", nil))
			case "descartar":
				b.aspaRedactor(rd)
				encolado = confirmarEncolado(t, w.Canvas(), b.P.T.T("compose.discardConfirm", nil))
			case "guardar":
				encolado = rd.guardar.AlPulsar
			case "cerrar guardando":
				encolado = rd.d.AlCerrar
			case "programar":
				rd.r.AlternarProgramar()
				encolado = func() { b.programar(rd, time.Now().Add(time.Hour), true) }
			}
			b.CambiarSesion(context.Background(), "emisor\x00persona-b\x00org")
			encolado()
			for _, llamada := range []string{"Send", "SaveDraft", "Delete", "Schedule"} {
				if s.pidio(llamada) != 0 {
					t.Fatalf("%s de A se ejecutó en B: %v", llamada, s.llamadas)
				}
			}
			b.CambiarSesion(context.Background(), "emisor\x00persona-a\x00org")
			if b.redaccion == nil || b.redaccion == rd || b.redaccion.r.B.Cuerpo != "Te adjunto el presupuesto." {
				t.Fatal("volver a A no restauró su redactor privado")
			}
		})
	}
}

func TestConfirmacionesVigentesSiguenEnviandoYDescartando(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{Para: "grace@example.org", Cuerpo: "Te adjunto el presupuesto."})
	b.enviar(b.redaccion, false)
	confirmarEncolado(t, w.Canvas(), b.P.T.T("compose.noAttachmentSend", nil))()
	if s.pidio("Send") != 1 {
		t.Fatal("la confirmación vigente no envió")
	}
	b.abrirRedactor(Borrador{ID: "borrador-vigente", Asunto: "Descartar"})
	b.aspaRedactor(b.redaccion)
	confirmarEncolado(t, w.Canvas(), b.P.T.T("compose.discardConfirm", nil))()
	if s.pidio("Delete") != 1 || b.redaccion != nil {
		t.Fatal("la confirmación vigente no descartó su borrador")
	}
}

func TestConfirmacionRetiradaNoBorraElBorradorDelNuevoRedactor(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{ID: "antiguo", Asunto: "Primero"})
	anterior := b.redaccion
	b.aspaRedactor(anterior)
	encolado := confirmarEncolado(t, w.Canvas(), b.P.T.T("compose.discardConfirm", nil))
	b.cerrar(anterior)
	b.abrirRedactor(Borrador{ID: "nuevo", Asunto: "Segundo"})
	actual := b.redaccion
	encolado()
	if s.pidio("Delete") != 0 || b.redaccion != actual {
		t.Fatal("la confirmación anterior borró datos o cerró el nuevo redactor")
	}
}

func TestLimpiezaProgramadaNoAdoptaLaSiguienteSesion(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.CambiarSesion(context.Background(), "emisor\x00persona-a\x00org")
	b.abrirRedactor(Borrador{ID: "borrador-a", Para: "grace@example.org", Cuerpo: "Mensaje programado de A"})
	b.redaccion.r.AlternarProgramar()
	var tareas []func()
	b.Lanzar = func(f func()) { tareas = append(tareas, f) }
	b.programar(b.redaccion, time.Now().Add(time.Hour), true)
	if len(tareas) != 1 {
		t.Fatal("programar no inició una única petición")
	}
	tareas[0]() // Schedule se confirma; Delete queda encolado antes de los contadores.
	if s.pidio("Schedule") != 1 || len(tareas) < 2 {
		t.Fatal("no quedó pendiente la limpieza del borrador")
	}
	limpiar := tareas[1]
	b.CambiarSesion(context.Background(), "emisor\x00persona-b\x00org")
	limpiar()
	if s.pidio("Delete") != 0 {
		t.Fatal("la limpieza del envío de A borró con la vida de B")
	}
}

func TestGuardarUnRedactorCerradoNoCierraElSiguiente(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{Asunto: "Primer borrador"})
	anterior := b.redaccion
	var guardar func()
	b.Lanzar = func(f func()) { guardar = f }
	b.guardarBorrador(anterior, false, true)
	b.cerrarGuardando(anterior)
	b.abrirRedactor(Borrador{Asunto: "Segundo borrador"})
	actual := b.redaccion
	guardar()
	if s.pidio("SaveDraft") != 1 || b.redaccion != actual || actual.r.B.Asunto != "Segundo borrador" {
		t.Fatal("el guardado confirmado del redactor cerrado perdió o cerró el siguiente")
	}
}
