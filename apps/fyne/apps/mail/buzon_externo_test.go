// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// buzonConGmail es un buzón de prueba con la cuenta de Gmail de cuentaGmailNueva, sus
// carpetas (sin Archivo) y dos mensajes en su Recibidos.
func buzonConGmail(t *testing.T) (*Buzon, *correoFalso) {
	t.Helper()
	s := &correoFalso{
		cuentas:  cuentaGmailNueva(),
		carpetas: map[string][]api.ExternalMailFolder{"nueva": carpetasDeGmail},
		paginas: map[string]api.ExternalMailPage{"nueva/INBOX/": {Items: []api.ExternalMailMessage{
			{ID: "g1", Subject: "Factura", Date: "2026-09-11T09:00:00Z", From: []api.ExternalMailAddress{{Name: "Ana", Email: "ana@proveedor.eu"}}},
			{ID: "g2", Subject: "Antigua", Date: "2026-09-10T09:00:00Z", Seen: true},
		}}},
		leido: api.ExternalMailMessage{ID: "g1", Text: "Adjunto la factura.", Attachments: []api.ExternalMailAttachment{{Name: "f.pdf", Type: "application/pdf", ContentBase64: "JVBERg=="}}},
	}
	b := correoDePrueba(t, s).b
	w := test.NewWindow(b.P.Objeto())
	t.Cleanup(w.Close)
	b.cargarExternas()
	return b, s
}

// Fase B, punto 4: una carpeta externa se abre con la Bandeja y el Lector de siempre;
// leer, tirar y mover van al proveedor de SU cuenta con su dirección, y lo que solo
// existe en Sytue (evento, baja, Rooms, Drive, el .eml, etiquetas, reglas) no sale.
//
// VISTO EN ROJO (2026-11-06): sin `ponerLector` el lector ofrecía «Crear evento» y el
// original con un id que Sytue no conoce; con `SetFlagMany` para todo, marcar leído un
// externo iba al servicio de Sytue; sin `ponerAccionesExternas`, «Mover a…» listaba las
// carpetas de Sytue. Deshecho.
func TestUnaCarpetaExternaSeAbreConLaListaYElLector(t *testing.T) {
	b, s := buzonConGmail(t)
	b.ElegirExterna("nueva", "INBOX")
	cs := b.P.Bandeja.Conversaciones
	if b.P.Bandeja.Estado != Listo || len(cs) != 2 || cs[0].Ultimo.Asunto != "Factura" || !cs[0].SinLeer || cs[1].SinLeer {
		t.Fatalf("la lista: estado %v, %+v", b.P.Bandeja.Estado, cs)
	}
	if b.P.Bandeja.SoloLectura {
		t.Fatal("una cuenta con acciones se abre en solo lectura")
	}
	a := b.P.Acciones
	if a.Archivar != nil || a.Papelera == nil || a.Etiquetar != nil || a.CrearRegla != nil {
		t.Fatal("el triaje externo ofrece lo que la cuenta no tiene (Archivo, etiquetas, reglas) o no deja tirar")
	}
	destinos := make([]string, 0, len(a.Destinos))
	for _, d := range a.Destinos {
		destinos = append(destinos, d.ID)
	}
	if !slices.Equal(destinos, []string{"SENT", "TRASH", "Label_1"}) {
		t.Fatalf("«Mover a…» lista %v; quiero las carpetas de la cuenta menos la actual", destinos)
	}

	b.Abrir(cs[0])
	fyne.DoAndWait(func() {})
	if !slices.Equal(s.leidos, []string{"nueva/INBOX/g1 <ana@example.eu>"}) {
		t.Fatalf("el cuerpo se pidió así: %v", s.leidos)
	}
	if !slices.Equal(s.acciones, []string{"nueva/INBOX/g1 read  <ana@example.eu>"}) {
		t.Fatalf("marcar leído fue %v", s.acciones)
	}
	c, ya := b.cuerpo(cs[0].Ultimo.ID)
	if !ya || c.Texto != "Adjunto la factura." || len(c.Adjuntos) != 1 || string(c.Adjuntos[0].Datos) != "%PDF" {
		t.Fatalf("cuerpo %+v", c)
	}
	l := b.P.Lector
	if l.AlCrearEvento != nil || l.AlOriginal != nil || l.AlDarseDeBaja != nil || l.AlCompartirEnRooms != nil || l.Baja {
		t.Fatal("el lector ofrece lo que solo existe en Sytue para un mensaje externo")
	}

	a.Mover(b.P.Lector.Conversacion, Carpeta{ID: "Label_1", Nombre: "Facturas"})
	a = b.P.Acciones
	a.Papelera(b.P.Bandeja.Conversaciones[0])
	if want := []string{"nueva/INBOX/g1 move Label_1 <ana@example.eu>", "nueva/INBOX/g2 trash  <ana@example.eu>"}; !slices.Equal(s.acciones[1:], want) {
		t.Fatalf("mover y tirar fueron %v", s.acciones[1:])
	}
	if len(b.P.Bandeja.Conversaciones) != 0 || b.hiloID != "" {
		t.Fatal("lo movido sigue en la lista o abierto")
	}

	// Volver a una carpeta de Sytue devuelve el lector entero.
	b.cargarCarpetas()
	b.Abrir(b.P.Bandeja.Conversaciones[0])
	if b.externa != nil || b.P.Lector.AlOriginal == nil || b.P.Menu.Externa != nil {
		t.Fatal("al volver a Sytue el lector sigue sin el original o la vista externa no se suelta")
	}
}

// Fase B, punto 5: «Todos los buzones de entrada» mezcla por fecha la Entrada de Sytue
// y la de cada cuenta, pide más donde hace falta y dice qué cuenta no contestó.
//
// VISTO EN ROJO (2026-11-06): con las páginas una detrás de otra en vez de mezcladas,
// el de Gmail salía al final; sin `mezclaCaida`, la cuenta caída desaparecía de la
// lista sin decir nada. Deshecho.
func TestTodosLosBuzonesDeEntradaMezclaPorFecha(t *testing.T) {
	b, s := buzonConGmail(t)
	s.cuentas.Items = append(s.cuentas.Items, api.ExternalMailAccount{InstanceID: "caida", Provider: "imap", Address: "ana@caida.eu", Connected: true, Services: []string{"mail"}})
	s.caidas = map[string]bool{"caida": true}
	s.paginas["nueva/INBOX/"] = api.ExternalMailPage{Items: []api.ExternalMailMessage{{ID: "g1", Subject: "Gmail", Date: "2026-09-11T09:00:00Z"}}}
	b.cargarCarpetas()
	b.cargarExternas()
	tr := b.P.T
	todas, _ := ramaDe(t, b.P.Menu.seccionBuzones(tema.Claro), tr.T("nav.allInboxes", nil))
	// Su triángulo dice qué despliega: «Cuentas», a secas, no lo decía.
	if want := tr.T("nav.accountsIn", i18n.Vars{"mailbox": tr.T("nav.allInboxes", nil)}); todas.Accion == nil || todas.Accion.Nombre != want {
		t.Fatalf("el triángulo de la Entrada mezclada no dice qué despliega; quiero %q", want)
	}
	todas.AlPulsar()
	if b.externa == nil || b.externa.Unificado != "inbox" || b.P.Bandeja.Titulo != tr.T("nav.allInboxes", nil) {
		t.Fatalf("la fila no abre la Entrada mezclada: %+v", b.externa)
	}
	asuntos := func() []string {
		out := make([]string, 0, len(b.P.Bandeja.Conversaciones))
		for _, c := range b.P.Bandeja.Conversaciones {
			out = append(out, c.Ultimo.Asunto)
		}
		return out
	}
	if want := []string{"Calendario", "Gmail", "Encuesta"}; !slices.Equal(asuntos(), want) || !b.P.Bandeja.HayMas {
		t.Fatalf("mezcla %q (hay más %v); quiero %q con más por pedir", asuntos(), b.P.Bandeja.HayMas, want)
	}
	if b.P.Franja == nil || !strings.Contains(textosDe(b.P.Franja), tr.T("externalMail.someUnavailable", nil)) {
		t.Fatal("la cuenta que no contestó no se dice")
	}
	todas, _ = ramaDe(t, b.P.Menu.seccionBuzones(tema.Claro), tr.T("nav.allInboxes", nil))
	if !todas.Activa {
		t.Fatal("«Todos los buzones de entrada» no se marca a la vista")
	}
	b.MasPagina()
	if want := []string{"Calendario", "Gmail", "Encuesta", "Factura"}; !slices.Equal(asuntos(), want) || b.P.Bandeja.HayMas {
		t.Fatalf("tras pedir más: %q (hay más %v); quiero %q", asuntos(), b.P.Bandeja.HayMas, want)
	}
}

// Una contraseña de app revocada en iCloud: la franja de la carpeta lo dice y «Conectar
// cuenta» lleva a su inicio de sesión (`AvisoExterno` en la web). VISTO EN ROJO
// (2026-11-06) sin `ponerAvisoExterno`: la lista decía «no se pudo» y ofrecía
// «Reintentar», que nunca lo arregla.
func TestLaFranjaExternaPideVolverAIniciarSesion(t *testing.T) {
	icloud := cuentaICloud()
	s := &correoFalso{
		cuentas:     api.ExternalMailAccounts{Items: []api.ExternalMailAccount{icloud}, Providers: []api.ExternalMailProvider{{Provider: "imap", Available: true}}},
		mensajesErr: &api.Error{Status: 422, Body: `{"error":{"code":"mail_auth"}}`},
	}
	b := correoDePrueba(t, s).b
	w := test.NewWindow(b.P.Objeto())
	t.Cleanup(w.Close)
	b.cargarExternas()
	b.ElegirExterna("nube", "INBOX")
	if b.errorExterno != "externalMail.icloudAuthError" || b.P.Bandeja.Estado != Error {
		t.Fatalf("aviso %q, estado %v", b.errorExterno, b.P.Bandeja.Estado)
	}
	conectar := botonDe(b.P.Franja, b.P.T.T("externalMail.connect", nil))
	if conectar == nil {
		t.Fatal("la franja no deja volver a conectar")
	}
	conectar.AlPulsar()
	t.Cleanup(func() {
		if b.cerrarCorreoExterno != nil {
			b.cerrarCorreoExterno()
		}
	})
	if c := b.correoAbierto; c == nil || !c.asistente || c.instancia != "nube" || !c.icloud {
		t.Fatal("«Conectar cuenta» no abre el inicio de sesión de esa cuenta")
	}
}

// Responder a un mensaje externo sale de su cuenta, con su dirección como la que se
// espera; «Redactar» con su carpeta a la vista, también. Las dos abren el redactor de
// siempre con esa cuenta en el «De» (fase B, punto 6). VISTO EN ROJO (2026-11-06) sin
// `responderExterno`: la respuesta salía del buzón de Sytue, a quien no le escribieron.
func TestResponderAUnExternoSaleDeSuCuenta(t *testing.T) {
	b, s := buzonConGmail(t)
	b.ElegirExterna("nueva", "INBOX")
	b.Abrir(b.P.Bandeja.Conversaciones[0])
	b.responder(b.P.Lector.Conversacion.Mensajes[0], Responder)
	rd := b.redaccion
	if rd == nil || rd.r.B.Cuenta != "nueva" || rd.r.B.De != "ana@example.eu" || rd.r.B.Para != "Ana <ana@proveedor.eu>" && rd.r.B.Para != "ana@proveedor.eu" || !strings.HasPrefix(rd.r.B.Asunto, "Re:") {
		t.Fatalf("el redactor de la respuesta: %+v", rd)
	}
	b.enviar(rd, false)
	if s.sends != 1 || s.out.ExpectedAddress != "ana@example.eu" || s.out.To[0].Email != "ana@proveedor.eu" || b.redaccion != nil {
		t.Fatalf("enviar: %d envíos, %+v", s.sends, s.out)
	}
	b.Redactar()
	if b.redaccion == nil || b.redaccion.r.B.Cuenta != "nueva" {
		t.Fatal("«Redactar» con la carpeta externa a la vista no escribe desde su cuenta")
	}
}

// Una cuenta POP3 no tiene acciones en el proveedor: su carpeta se lee sin triaje, y al
// volver a Sytue vuelve todo. VISTO EN ROJO (2026-11-06) sin mirar `Capabilities`.
func TestUnaCuentaPOP3SeLeeSinTriaje(t *testing.T) {
	s := &correoFalso{cuentas: api.ExternalMailAccounts{Items: []api.ExternalMailAccount{
		{InstanceID: "pop", Provider: "pop3", Address: "ana@pop.eu", Connected: true, Services: []string{"mail"}, Capabilities: api.ExternalMailCapabilities{Read: true, Send: true}},
	}}}
	b := correoDePrueba(t, s).b
	b.cargarExternas()
	b.ElegirExterna("pop", "INBOX")
	if !b.P.Bandeja.SoloLectura || b.P.Franja == nil || !strings.Contains(textosDe(b.P.Franja), b.P.T.T("externalMail.pop3Note", nil)) {
		t.Fatal("la carpeta POP3 ofrece triaje o no avisa")
	}
	b.cargarCarpetas()
	if b.P.Bandeja.SoloLectura || b.P.Franja != nil {
		t.Fatal("al volver a Sytue sigue en solo lectura o con la franja")
	}
}

// Un mensaje POP3 mezclado en «Todos los buzones de entrada» se lee sin triaje: ni se
// marca leído al abrirlo, ni se destaca, ni se archiva, que el proveedor no lo admite.
// Los de una cuenta con acciones, sí.
//
// VISTO EN ROJO (2026-11-06): sin `sinAcciones`, abrirlo pedía «read» a la cuenta POP3
// y el lector ofrecía el triaje. Deshecho.
func TestUnPOP3MezcladoEnLasEntradasSeLeeSinTriaje(t *testing.T) {
	b, s := buzonConGmail(t)
	s.cuentas.Items = append(s.cuentas.Items, api.ExternalMailAccount{InstanceID: "pop", Provider: "pop3", Address: "ana@pop.eu", Connected: true, Services: []string{"mail"}, Capabilities: api.ExternalMailCapabilities{Read: true, Send: true}})
	s.paginas["pop/INBOX/"] = api.ExternalMailPage{Items: []api.ExternalMailMessage{{ID: "p1", Subject: "POP", Date: "2026-09-12T09:00:00Z"}}}
	b.cargarCarpetas()
	b.cargarExternas()
	b.ElegirUnificado("inbox")
	conv := func(asunto string) Conversacion {
		for _, c := range b.P.Bandeja.Conversaciones {
			if c.Ultimo.Asunto == asunto {
				return c
			}
		}
		t.Fatalf("no está %q en la mezcla", asunto)
		return Conversacion{}
	}
	pop := conv("POP")
	b.Abrir(pop)
	if !b.P.Lector.SoloLectura || b.P.Bandeja.SoloLectura {
		t.Fatal("el POP3 mezclado se abre con triaje, o la mezcla entera queda en solo lectura")
	}
	b.alternarDestacado(pop)
	b.moverPorRol([]Conversacion{pop}, "archive")
	if len(s.acciones) != 0 || !conv("POP").SinLeer {
		t.Fatalf("al POP3 se le pidieron acciones: %q", s.acciones)
	}
	b.Abrir(conv("Factura"))
	if b.P.Lector.SoloLectura || len(s.acciones) != 1 || !strings.HasPrefix(s.acciones[0], "nueva/INBOX/g1 read") {
		t.Fatalf("el de Gmail no se marca leído al abrirlo: %q", s.acciones)
	}
}

// Desconectar en el diálogo la cuenta cuya carpeta está a la vista vuelve a la Entrada
// de Sytue. VISTO EN ROJO (2026-11-06): sin mirarlo en fijarExternas, el panel seguía
// con los correos de la cuenta que ya no existe. Deshecho.
func TestDesconectarLaCuentaALaVistaVuelveASytue(t *testing.T) {
	b, _ := buzonConGmail(t)
	b.cargarCarpetas()
	b.ElegirExterna("nueva", "INBOX")
	b.fijarExternas(cuentaGmailNueva())
	if b.externa == nil {
		t.Fatal("una lista que aún la trae soltó la carpeta")
	}
	b.fijarExternas(api.ExternalMailAccounts{})
	entrada, _ := b.carpetaDeRol("inbox")
	if b.externa != nil || b.carpetaID != entrada.ID || b.P.Bandeja.Titulo != RotuloCarpeta(b.P.T, entrada.Nombre, entrada.Rol) {
		t.Fatalf("tras desconectarla sigue a la vista: %+v, carpeta %q", b.externa, b.carpetaID)
	}
}

// textosDe son los textos de las ayudas y botones de un árbol de objetos.
func textosDe(o fyne.CanvasObject) string {
	var b strings.Builder
	var andar func(fyne.CanvasObject)
	andar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.AyudaEnLinea:
			b.WriteString(x.Cuerpo + "\n")
		case *ui.Boton:
			b.WriteString(x.Texto + "\n")
		case *fyne.Container:
			for _, h := range x.Objects {
				andar(h)
			}
		}
	}
	andar(o)
	return b.String()
}

// botonDe es el botón con ese texto dentro de un árbol de objetos.
func botonDe(o fyne.CanvasObject, texto string) *ui.Boton {
	switch x := o.(type) {
	case *ui.Boton:
		if x.Texto == texto {
			return x
		}
	case *fyne.Container:
		for _, h := range x.Objects {
			if b := botonDe(h, texto); b != nil {
				return b
			}
		}
	}
	return nil
}

// buzonConEnviados es buzonConGmail con Enviados y Borradores en Sytue y en la cuenta,
// cada carpeta de la cuenta con un mensaje suyo, y el reloj del refresco a mano.
func buzonConEnviados(t *testing.T) (*Buzon, *correoFalso, *servicioFalso, *[]func()) {
	t.Helper()
	b, s := buzonConGmail(t)
	sv := b.S.(*servicioFalso)
	sv.carpetas = append(sv.carpetas, api.Mailbox{ID: "se", Name: "Sent", Role: "sent"}, api.Mailbox{ID: "dr", Name: "Drafts", Role: "drafts"})
	s.carpetas["nueva"] = append(slices.Clone(carpetasDeGmail), api.ExternalMailFolder{ID: "DRAFT", Name: "DRAFT", Role: "drafts"})
	s.paginas["nueva/SENT/"] = api.ExternalMailPage{Items: []api.ExternalMailMessage{{ID: "e1", Subject: "Enviado desde Gmail", Date: "2026-09-11T09:00:00Z", Seen: true}}}
	s.paginas["nueva/DRAFT/"] = api.ExternalMailPage{Items: []api.ExternalMailMessage{{ID: "b1", Subject: "Borrador de Gmail", Date: "2026-09-11T09:00:00Z", Seen: true}}}
	var relojes []func()
	b.Esperar = func(_ time.Duration, f func()) func() {
		relojes = append(relojes, f)
		return func() {}
	}
	b.cargarCarpetas()
	b.cargarExternas()
	s.paginasPedidas()
	sv.llamadas = nil
	return b, s, sv, &relojes
}

// sytuePedidas son las carpetas de Sytue que se listaron, y las olvida.
func sytuePedidas(sv *servicioFalso) []string {
	var out []string
	for _, l := range sv.llamadas {
		if strings.HasPrefix(l, "Messages(") {
			out = append(out, l)
		}
	}
	sv.llamadas = nil
	return out
}

// Fase B, punto 5 completo: «Todos los enviados» es un buzón en sí, como «Todos los
// buzones de entrada»: la fila abre los Enviados de Sytue y de cada cuenta mezclados, y
// se marca; el refresco de 30 s pide solo esa carpeta, la visible, y ninguna Entrada.
// La papelera unificada solo se despliega.
//
// VISTO EN ROJO (2026-11-07): con `u.Rol == "inbox"` de antes en seccionBuzones, la
// fila de los enviados solo plegaba; con fuentesDe pidiendo siempre «inbox», «Todos
// los enviados» enseñaba las Entradas y el refresco volvía a pedir INBOX; y con
// `m.Externa.Mezclada()` en vez del rol, la Entrada mezclada seguía marcada. Deshecho.
func TestTodosLosEnviadosMezclaSoloLosEnviados(t *testing.T) {
	b, s, sv, relojes := buzonConEnviados(t)
	tr := b.P.T
	enviados, _ := ramaDe(t, b.P.Menu.seccionBuzones(tema.Claro), tr.T("nav.allSent", nil))
	if enviados.Accion == nil || enviados.Accion.Nombre != tr.T("nav.accountsIn", i18n.Vars{"mailbox": tr.T("nav.allSent", nil)}) {
		t.Fatal("«Todos los enviados» no lleva su triángulo aparte")
	}
	enviados.AlPulsar()
	if b.externa == nil || b.externa.Unificado != "sent" || b.P.Bandeja.Titulo != tr.T("nav.allSent", nil) {
		t.Fatalf("la fila no abre los enviados mezclados: %+v, %q", b.externa, b.P.Bandeja.Titulo)
	}
	if got := s.paginasPedidas(); !slices.Equal(got, []string{"nueva/SENT/"}) {
		t.Fatalf("se pidió %q a la cuenta; quiero solo sus Enviados", got)
	}
	if got := sytuePedidas(sv); !slices.Equal(got, []string{"Messages(se,)"}) {
		t.Fatalf("se pidió %q a Sytue; quiero solo sus Enviados", got)
	}
	cs := b.P.Bandeja.Conversaciones
	if len(cs) == 0 || !slices.ContainsFunc(cs, func(c Conversacion) bool { return c.Ultimo.Asunto == "Enviado desde Gmail" }) {
		t.Fatalf("la mezcla no trae el enviado de la cuenta: %+v", cs)
	}
	s1 := b.P.Menu.seccionBuzones(tema.Claro)
	if f, _ := ramaDe(t, s1, tr.T("nav.allSent", nil)); !f.Activa {
		t.Fatal("«Todos los enviados» no se marca a la vista")
	}
	if f, _ := ramaDe(t, s1, tr.T("nav.allInboxes", nil)); f.Activa {
		t.Fatal("con los enviados a la vista, «Todos los buzones de entrada» sigue marcado")
	}

	// El refresco: solo la carpeta visible.
	(*relojes)[len(*relojes)-1]()
	if got := s.paginasPedidas(); !slices.Equal(got, []string{"nueva/SENT/"}) {
		t.Fatalf("el refresco pidió %q a la cuenta; quiero solo sus Enviados", got)
	}
	if got := sytuePedidas(sv); !slices.Equal(got, []string{"Messages(se,)"}) {
		t.Fatalf("el refresco pidió %q a Sytue", got)
	}

	// La papelera unificada no se abre: su fila pliega y despliega.
	papelera, _ := ramaDe(t, b.P.Menu.seccionBuzones(tema.Claro), tr.T("nav.allTrash", nil))
	if papelera.Accion != nil {
		t.Fatal("«Toda la papelera» lleva triángulo aparte, como si se abriera")
	}
	papelera.AlPulsar()
	b.ElegirUnificado("trash")
	if b.externa == nil || b.externa.Unificado != "sent" || len(s.paginasPedidas()) != 0 {
		t.Fatalf("la papelera unificada abrió una mezcla: %+v", b.externa)
	}
}

// En «Todos los borradores» uno de Sytue reabre el redactor, como en Borradores; uno de
// la cuenta se lee (vive en su proveedor). Guardar un borrador con la mezcla a la vista
// la vuelve a pedir.
//
// VISTO EN ROJO (2026-11-07): con `b.rolVista() == "drafts"` de antes en Abrir, el de
// Sytue se abría en el lector; sin `!esExterno`, el de Gmail abría el redactor de Sytue
// con un id que Sytue no conoce; y sin mirar `Unificado` en alGuardarBorrador, el
// guardado no aparecía hasta el refresco. Deshecho.
func TestEnTodosLosBorradoresElDeSytueAbreElRedactor(t *testing.T) {
	b, s, sv, _ := buzonConEnviados(t)
	b.ElegirUnificado("drafts")
	if got := s.paginasPedidas(); !slices.Equal(got, []string{"nueva/DRAFT/"}) {
		t.Fatalf("se pidió %q a la cuenta; quiero solo sus Borradores", got)
	}
	if got := sytuePedidas(sv); !slices.Equal(got, []string{"Messages(dr,)"}) {
		t.Fatalf("se pidió %q a Sytue", got)
	}
	conv := func(asunto string) Conversacion {
		for _, c := range b.P.Bandeja.Conversaciones {
			if c.Ultimo.Asunto == asunto {
				return c
			}
		}
		t.Fatalf("no está %q en la mezcla", asunto)
		return Conversacion{}
	}
	b.Abrir(conv("Borrador de Gmail"))
	if b.redaccion != nil || b.hiloID == "" {
		t.Fatal("el borrador de la cuenta no se abre en el lector")
	}
	b.hiloID = ""
	b.Abrir(conv("Calendario"))
	if b.redaccion == nil || b.redaccion.r.B.ID != "m1" {
		t.Fatalf("el borrador de Sytue no reabre el redactor: %+v", b.redaccion)
	}
	b.alGuardarBorrador()
	if got := s.paginasPedidas(); !slices.Equal(got, []string{"nueva/DRAFT/"}) {
		t.Fatalf("guardar no volvió a pedir «Todos los borradores»: %q", got)
	}
}

// Una búsqueda es de todo el buzón y no quita «Todos los borradores»: un resultado de
// Sytue que es un recibido se lee, no se reabre en el redactor (guardarlo como
// borrador, PUT /drafts/{id}, borraría el original). Lo mismo en la web (`inSearch`).
//
// VISTO EN ROJO (2026-11-07): sin `b.consulta != ""` en Abrir, el recibido se abría
// en el redactor con su id. Deshecho.
func TestBuscandoEnTodosLosBorradoresUnRecibidoSeLee(t *testing.T) {
	b, _, sv, _ := buzonConEnviados(t)
	b.ElegirUnificado("drafts")
	sv.buscados = []api.MailMessage{{ID: "11111111-1111-4111-8111-111111111111", Subject: "Recibido normal", MailboxID: "in"}}
	b.buscarYa("recibido")
	for range 50 {
		fyne.DoAndWait(func() {})
	}
	var c *Conversacion
	for i := range b.P.Bandeja.Conversaciones {
		if b.P.Bandeja.Conversaciones[i].Ultimo.Asunto == "Recibido normal" {
			c = &b.P.Bandeja.Conversaciones[i]
		}
	}
	if c == nil {
		t.Fatalf("no sale el resultado: %+v", b.P.Bandeja.Conversaciones)
	}
	b.Abrir(*c)
	if b.redaccion != nil {
		t.Fatalf("un recibido buscado abre el redactor como borrador: id=%s", b.redaccion.r.B.ID)
	}
	if b.hiloID != c.HiloID {
		t.Fatalf("no se abre en el lector: hilo %q, quiero %q", b.hiloID, c.HiloID)
	}
}
