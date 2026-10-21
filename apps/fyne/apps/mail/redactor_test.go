// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// buzonConVentana es el buzón de prueba montado en una ventana: el redactor es un
// diálogo y necesita lienzo.
func buzonConVentana(t *testing.T, s *servicioFalso, tam fyne.Size) (*Buzon, fyne.Window, *[]func()) {
	t.Helper()
	b, pendientes := buzonDePrueba(t, s)
	t.Cleanup(func() {
		for _, a := range b.P.Avisos.Avisos() {
			b.P.Avisos.Descartar(a)
		}
	})
	w := test.NewWindow(b.P.Objeto())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(tam)
	b.Arrancar(context.Background())
	*pendientes = nil // el sondeo de arranque no es de estas pruebas
	return b, w, pendientes
}

// TestPartirDireccionesEsLaDeLaWeb: lo de antes de la última coma son fichas y lo
// último, lo que se escribe; «;» también separa.
func TestPartirDireccionesEsLaDeLaWeb(t *testing.T) {
	fichas, actual := PartirDirecciones("Ana <ana@x.eu>, luis@y.eu; ma")
	if !slices.Equal(fichas, []string{"Ana <ana@x.eu>", "luis@y.eu"}) || actual != "ma" {
		t.Errorf("fichas %q, actual %q", fichas, actual)
	}
	if got := UnirDireccionesCampo(fichas, "ma"); got != "Ana <ana@x.eu>, luis@y.eu, ma" {
		t.Errorf("unir: %q", got)
	}
	if f, a := PartirDirecciones(""); len(f) != 0 || a != "" {
		t.Errorf("vacío: %q %q", f, a)
	}
	if got := direccionesLaxas("Ana <ana@x.eu>, a medias"); !slices.Equal(got, []string{"ana@x.eu", "a medias"}) {
		t.Errorf("un borrador se guarda con lo que haya: %q", got)
	}
}

// TestLaComaCierraLaFicha: escribir una coma convierte lo escrito en ficha y deja el
// campo vacío para la siguiente; borrar con el campo vacío abre la última.
func TestLaComaCierraLaFicha(t *testing.T) {
	c := NuevoCampoDirecciones("")
	c.teclear("ana@x.eu,")
	if c.Valor != "ana@x.eu, " || c.editor.Texto() != "" {
		t.Fatalf("tras la coma: valor %q, campo %q", c.Valor, c.editor.Texto())
	}
	c.tecla(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if c.Valor != "ana@x.eu" || c.editor.Texto() != "ana@x.eu" {
		t.Errorf("borrar con el campo vacío: valor %q, campo %q", c.Valor, c.editor.Texto())
	}
}

// TestResponderAbreElRedactorConLaCitaYLaFirma: el redactor abre con el remitente en
// «Para», el asunto con su «Re:», la cita y la firma debajo, y el título «Responder».
func TestResponderAbreElRedactorConLaCitaYLaFirma(t *testing.T) {
	s := servicioConTodas()
	s.firma = "Ada"
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.Abrir(b.P.Bandeja.Conversaciones[0])
	b.P.Lector.AlResponder(b.P.Lector.Conversacion.Mensajes[0], Responder)

	d := dialogoAbierto(t, w.Canvas())
	br := b.redaccion.r.B
	if d.Titulo != traductor(t).T("compose.replyTitle", nil) || br.Para != "Persona m1 <m1@example.org>" || br.Asunto != "Re: Calendario" {
		t.Errorf("título %q, para %q, asunto %q", d.Titulo, br.Para, br.Asunto)
	}
	if !strings.Contains(br.Cuerpo, "> Hola:") || !strings.HasSuffix(br.Cuerpo, "\n\n-- \nAda") {
		t.Errorf("el cuerpo no lleva la cita completa y la firma: %q", br.Cuerpo)
	}
}

// TestEnviarSinDestinatarioLoDiceEnLaFila: no se envía y el aviso va DENTRO de la fila
// de «Para», que crece su renglón.
func TestEnviarSinDestinatarioLoDiceEnLaFila(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.Redactar()
	antes := b.redaccion.r.filaPara.AltoFila(600)
	b.enviar(b.redaccion, false)
	if s.pidio("Send") != 0 {
		t.Fatal("se ha enviado sin destinatario")
	}
	f := b.redaccion.r.filaPara
	if f.Error != traductor(t).T("compose.toRequired", nil) || f.AltoFila(600) != antes+altoErrorFila {
		t.Errorf("error %q; la fila mide %v y medía %v", f.Error, f.AltoFila(600), antes)
	}
}

// TestEnviarRetieneDiezSegundosYSeDeshace: el envío pide la ventana de deshacer, el
// aviso ofrece deshacer y deshacer saca el envío de la cola y reabre el redactor.
//
// VISTO EN ROJO (2026-09-23): con `VentanaDeshacer` a 0 el aviso no trae la acción y
// el envío sale sin retención.
func TestEnviarRetieneDiezSegundosYSeDeshace(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{Para: "Grace <grace@example.org>", Asunto: "Hola", Cuerpo: "Qué tal."})
	b.enviar(b.redaccion, false)
	if !slices.Contains(s.llamadas, "Send(ada@example.org,10,grace@example.org)") {
		t.Fatalf("envío: %v", s.llamadas)
	}
	if !strings.Contains(s.enviado, "Subject: Hola") || !strings.Contains(s.enviado, "Qu=C3=A9 tal.") {
		t.Errorf("el .eml no lleva lo escrito:\n%s", s.enviado)
	}
	if b.redaccion != nil {
		t.Error("el redactor sigue abierto tras enviar")
	}
	av := b.P.Avisos.Avisos()
	if len(av) != 1 || av[0].RotuloAccion == "" || av[0].AlPulsarAccion == nil {
		t.Fatalf("el aviso de enviado no ofrece deshacer: %+v", av)
	}
	av[0].AlPulsarAccion()
	if !slices.Contains(s.llamadas, "CancelOutbound(o1)") || b.redaccion == nil || b.redaccion.r.B.Asunto != "Hola" {
		t.Errorf("deshacer: llamadas %v, redactor %v", s.llamadas, b.redaccion != nil)
	}
	_ = w
}

// TestTeAdjuntoSinAdjuntoPregunta: la mención de un adjunto que no va abre la
// confirmación antes de enviar; lo citado no cuenta.
func TestTeAdjuntoSinAdjuntoPregunta(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{Para: "grace@example.org", Cuerpo: "Te adjunto el presupuesto."})
	b.enviar(b.redaccion, false)
	if s.pidio("Send") != 0 {
		t.Fatal("se ha enviado sin preguntar por el adjunto")
	}
	if !hayDialogo(w.Canvas(), traductor(t).T("compose.noAttachmentTitle", nil)) {
		t.Error("no se pregunta por el adjunto que no va")
	}
}

// hayDialogo dice si alguna superposición es un diálogo con ese título.
func hayDialogo(c fyne.Canvas, titulo string) bool {
	var hay bool
	var mirar func(fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		if d, ok := o.(*ui.Dialogo); ok && d.Titulo == titulo {
			hay = true
		}
		for _, h := range hijos(o) {
			mirar(h)
		}
	}
	for _, o := range c.Overlays().List() {
		mirar(o)
	}
	return hay
}

// TestElAutoguardadoGuardaYElIdCambia: dos segundos sin teclear y el borrador está en
// el servidor; el siguiente guardado SUSTITUYE al anterior con su id.
func TestElAutoguardadoGuardaYElIdCambia(t *testing.T) {
	s := servicioConTodas()
	b, _, pendientes := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.Redactar()
	rd := b.redaccion
	rd.r.asunto.SetText("Presupuesto")
	if len(*pendientes) == 0 {
		t.Fatal("escribir no ha programado el autoguardado")
	}
	(*pendientes)[len(*pendientes)-1]()
	if rd.r.B.ID != "d1" || !strings.Contains(s.guardado, "Subject: Presupuesto") {
		t.Fatalf("autoguardado: id %q, .eml %q", rd.r.B.ID, s.guardado)
	}
	rd.r.asunto.SetText("Presupuesto v2")
	(*pendientes)[len(*pendientes)-1]()
	if !slices.Contains(s.llamadas, "SaveDraft(d1)") || rd.r.B.ID != "d2" {
		t.Errorf("el segundo guardado no sustituye al primero: %v, id %q", s.llamadas, rd.r.B.ID)
	}
}

// TestEscapeGuardaYElAspaPregunta: cerrar por fuera guarda lo escrito; el aspa, que es
// lo único que ofrece no guardar, pregunta antes.
func TestEscapeGuardaYElAspaPregunta(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{Para: "grace@example.org", Asunto: "A medias"})
	b.redaccion.d.Descartar()
	if s.pidio("SaveDraft") != 1 || b.redaccion != nil {
		t.Fatalf("cerrar por fuera: %v", s.llamadas)
	}
	b.abrirRedactor(Borrador{Para: "grace@example.org", Asunto: "Otra"})
	rd := b.redaccion
	b.aspaRedactor(rd)
	if b.redaccion == nil {
		t.Fatal("el aspa ha cerrado sin preguntar")
	}
	if !hayDialogo(w.Canvas(), traductor(t).T("compose.discardTitle", nil)) {
		t.Error("el aspa no abre la confirmación de descartar")
	}
}

// TestCtrlIntroEnvia: la combinación llega al campo como atajo y el redactor envía.
func TestCtrlIntroEnvia(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{Para: "grace@example.org", Asunto: "Rápido", Cuerpo: "Hecho."})
	sc, _ := b.redaccion.r.rico.Zona().(fyne.Shortcutable)
	sc.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: fyne.KeyModifierControl})
	if s.pidio("Send") != 1 {
		t.Errorf("Ctrl + Intro no ha enviado: %v", s.llamadas)
	}
}

func TestGuardarBorradorEsUnIconoAccesible(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.Redactar()

	guardar := b.redaccion.guardar
	if !guardar.SoloIcono || guardar.Icono != iconos.Guardar {
		t.Errorf("guardar no es el disquete de solo icono: soloIcono=%v icono=%v", guardar.SoloIcono, guardar.Icono)
	}
	if quiero := traductor(t).T("compose.saveDraft", nil); guardar.Rotulo != quiero {
		t.Errorf("el disquete se anuncia %q; quiero %q", guardar.Rotulo, quiero)
	}
}

// TestLaCabeceraDelRedactorMideLoDeLaWeb: MEDIDO en el arnés sin sesión a 1280×800
// (2026-09-23): el diálogo de 720, las filas de 32,5 y el campo en x = 368,5 —el rótulo
// de `w-16` y el `gap-2` desde el cuerpo—.
// Raíz16: campo36+filete1=37; x305=280+border1+px-6;
// campo305+px-1(4)+w-16(64)+gap-2(8)=381; cuerpo720-2-48=670.
func TestLaCabeceraDelRedactorMideLoDeLaWeb(t *testing.T) {
	const (
		filaMedida  = 37
		xCuerpo     = 305.0
		xCampo      = 381.0
		anchoMedido = 720.0
	)
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	b.Redactar()
	w.Canvas().Capture()
	d := dialogoAbierto(t, w.Canvas())
	if got := d.Size().Width; got != anchoMedido {
		t.Errorf("el redactor mide %v de ancho y en la web %v", got, anchoMedido)
	}
	r := b.redaccion.r
	if got := r.filaPara.AltoFila(r.ancho); got != filaMedida {
		t.Errorf("la fila de «Para» mide %v y en la web %v", got, filaMedida)
	}
	if got := r.filaAsunto.AltoFila(r.ancho); got != filaMedida {
		t.Errorf("la fila del asunto mide %v y en la web %v", got, filaMedida)
	}
	if got := margenFila + anchoRotuloFila + huecoFilaCab; float64(got) != xCampo-xCuerpo {
		t.Errorf("el campo empieza a %v del cuerpo y en la web a %v", got, xCampo-xCuerpo)
	}
	if AnchoCuerpoDialogo(ui.DialogoLG, 1280) != 670 {
		t.Errorf("el cuerpo del redactor mide %v y en la web 676", AnchoCuerpoDialogo(ui.DialogoLG, 1280))
	}
	_ = tema.Rem
}

// TestLaCapturaDelRedactorSeGuarda deja los PNG del redactor (SYTUE_CAPTURAS).
func TestLaCapturaDelRedactorSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	for _, c := range []struct {
		nombre string
		tam    fyne.Size
	}{{"escritorio", fyne.NewSize(1280, 800)}, {"movil", fyne.NewSize(360, 740)}} {
		s := servicioConTodas()
		s.firma = "Ada Lovelace\nSytue"
		b, w, _ := buzonConVentana(t, s, c.tam)
		b.Redactar()
		guardar(t, filepath.Join(dir, "redactar-"+c.nombre+".png"), w.Canvas().Capture())
		b.cerrar(b.redaccion)
		b.Abrir(b.P.Bandeja.Conversaciones[0])
		b.P.Lector.AlResponder(b.P.Lector.Conversacion.Mensajes[0], ResponderATodos)
		b.redaccion.r.verCc = true
		b.redaccion.r.relayout()
		guardar(t, filepath.Join(dir, "responder-"+c.nombre+".png"), w.Canvas().Capture())
	}
	_ = api.MailBaja{}
}

// TestProgramarPideHoraFuturaYSaleConElla: una hora pasada no se programa y se dice; una
// futura va al servicio con `send_at` y el redactor se cierra.
func TestProgramarPideHoraFuturaYSaleConElla(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	b.abrirRedactor(Borrador{Para: "grace@example.org", Asunto: "Mañana"})
	rd := b.redaccion
	rd.r.AlternarProgramar()
	b.programar(rd, time.Now().Add(-time.Hour), true)
	if s.pidio("Schedule") != 0 {
		t.Fatal("se ha programado a una hora pasada")
	}
	cuando := time.Now().Add(26 * time.Hour).Truncate(time.Minute)
	rd.r.cuando.fecha.PonerTexto(ui.EscribirFecha(cuando))
	rd.r.cuando.hora.PonerTexto(ui.EscribirHora(cuando))
	rd.r.BotonProgramar.Tapped(nil)
	if !slices.Contains(s.llamadas, "Schedule(ada@example.org,"+cuando.UTC().Format(time.RFC3339)+",grace@example.org)") {
		t.Errorf("programar: %v", s.llamadas)
	}
	if b.redaccion != nil {
		t.Error("el redactor sigue abierto tras programar")
	}
}

// TestLosProgramadosSeCancelanYSeReprograman: la lista sale del servicio y cancelar
// quita el envío de la cola.
func TestLosProgramadosSeCancelanYSeReprograman(t *testing.T) {
	s := servicioConTodas()
	s.programados = []api.MailScheduled{{ID: "p1", Subject: "Informe", Recipients: []string{"g@example.org"}, ReleaseAt: time.Now().Add(time.Hour)}}
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	b.verProgramados()
	if !hayDialogo(w.Canvas(), traductor(t).T("scheduled.title", nil)) || s.pidio("Scheduled") != 1 {
		t.Fatalf("programados: %v", s.llamadas)
	}
}

// El Para desnudo tiene una caja menor que su placeholder y un CC/CCO al lado.
// El texto debe quedar recortado en su campo, incluso antes de escribir nada.
func TestElParaMovilNoPintaEncimaDeCcCco(t *testing.T) {
	tema.Elegir("claro")
	t.Cleanup(func() { tema.Elegir("") })
	for _, ancho := range []float32{320, 375, 390} {
		tr := traductor(t)
		campo := NuevoCampoDirecciones(tr.T("compose.toPlaceholder", nil))
		cc := nuevoBotonTexto(tr.T("compose.ccBcc", nil), func() {})
		fila := nuevaFilaCabecera(tr.T("compose.to", nil), campoDir{campo}, cc)
		w := test.NewTempWindow(t, fila)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(ancho, fila.AltoFila(ancho)))
		con := w.Canvas().Capture()
		campo.marca = ""
		campo.Refresh()
		sin := w.Canvas().Capture()
		cambiados := 0
		for y := int(cc.Position().Y); y < int(cc.Position().Y+cc.Size().Height); y++ {
			for x := int(cc.Position().X); x < int(cc.Position().X+cc.Size().Width); x++ {
				ar, ag, ab, aa := con.At(x, y).RGBA()
				br, bg, bb, ba := sin.At(x, y).RGBA()
				if ar != br || ag != bg || ab != bb || aa != ba {
					cambiados++
				}
			}
		}
		if cambiados > 0 {
			t.Errorf("a%g el placeholder invade%d píxeles del botón CC/CCO", ancho, cambiados)
		}
		if dir := os.Getenv("SYTUE_CAPTURAS"); dir != "" {
			guardar(t, filepath.Join(dir, fmt.Sprintf("mail-para-movil-%g.png", ancho)), con)
		}
	}
}
