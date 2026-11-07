// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// filasDeTexto son los textos de las filas de una sección: el rótulo de una fila del
// lateral, el de un aviso o el de un botón.
func filasDeTexto(s *ui.SeccionLateral) []string {
	var out []string
	for _, o := range s.Filas {
		switch f := o.(type) {
		case *ui.FilaLateral:
			out = append(out, f.Rotulo)
		case *ui.Rotulo:
			out = append(out, f.Texto)
		case *ui.Boton:
			out = append(out, f.Texto)
		}
	}
	return out
}

// B8: «Cuentas de correo externas» es la PRIMERA sección, también sin cuentas y sin
// conexión, y su última fila es «Añadir cuenta de correo», como
// `ExternalAccountsSection` en MailApp.tsx. VISTO EN ROJO (2026-11-06): con el orden
// de antes la primera era «Carpetas» y, sin cuentas, la sección no existía.
func TestLaSeccionDeCuentasExternasEsLaDeLaWeb(t *testing.T) {
	tr := traductor(t)
	for _, m := range []*Menu{
		{T: tr, Locale: "es", EstadoCarpetas: Listo, EstadoExternas: Listo},
		{T: tr, Locale: "es", EstadoCarpetas: Listo, EstadoExternas: Listo, SinConexion: true},
		{
			T: tr, Locale: "es", EstadoCarpetas: Listo, EstadoExternas: Listo,
			Cuentas: []CuentaDeBuzones{{ID: "g", Titulo: "ana@gmail.com"}},
		},
	} {
		secciones := m.secciones(tema.Claro)
		primera, ok := secciones[0].(*ui.SeccionLateral)
		if !ok || primera.Titulo != tr.T("externalMail.title", nil) {
			t.Fatalf("la primera sección no es la de cuentas externas (%d cuentas)", len(m.Cuentas))
		}
		filas := filasDeTexto(primera)
		if want := len(m.Cuentas) + 1; len(filas) != want || filas[len(filas)-1] != tr.T("externalMail.add", nil) {
			t.Fatalf("filas %q: quiero las %d cuentas y «Añadir» al final", filas, len(m.Cuentas))
		}
		if segunda := secciones[1].(*ui.SeccionLateral); segunda.Titulo != tr.T("nav.folders", nil) {
			t.Fatalf("después va %q y en la web va «Carpetas»", segunda.Titulo)
		}
	}
	// El alta ya no se esconde en los ajustes plegados: la web no la tiene ahí.
	for _, a := range ajustes {
		if a.texto == "externalMail.title" {
			t.Fatal("«Cuentas de correo externas» sigue también en «Ajustes»")
		}
	}
}

// B4: si las cuentas no cargan, la sección lo dice y deja reintentar, en vez de
// quedarse vacía. VISTO EN ROJO (2026-11-06): con el `if err == nil` de antes el
// estado seguía en Cargando y la sección solo tenía «Añadir».
func TestLasCuentasExternasQueNoCarganSeDicen(t *testing.T) {
	s := &correoFalso{resumenErr: errors.New("caído")}
	b := correoDePrueba(t, s).b
	b.cargarExternas()
	if b.P.Menu.EstadoExternas != Error {
		t.Fatalf("estado %v tras un fallo; quiero Error", b.P.Menu.EstadoExternas)
	}
	filas := filasDeTexto(b.P.Menu.seccionExternas(tema.Claro))
	if !slices.Contains(filas, b.P.T.T("externalMail.accountsError", nil)) || !slices.Contains(filas, b.P.T.T("common.retry", nil)) {
		t.Fatalf("la sección no avisa del fallo: %q", filas)
	}
	s.resumenErr, s.cuentas = nil, cuentaGmailNueva()
	b.P.Menu.AlReintentarExternas()
	if b.P.Menu.EstadoExternas != Listo || len(b.P.Menu.Cuentas) != 1 {
		t.Fatalf("reintentar no ha traído la cuenta: estado %v, %v", b.P.Menu.EstadoExternas, b.P.Menu.Cuentas)
	}
	if filas := filasDeTexto(b.P.Menu.seccionExternas(tema.Claro)); slices.Contains(filas, b.P.T.T("externalMail.accountsError", nil)) {
		t.Fatalf("el aviso sigue tras cargar: %q", filas)
	}
}

// B4: al volver del proveedor, la cuenta sale en el lateral sin esperar a nada; y la
// fila «Añadir» abre el diálogo. Fase B, punto 4: la cuenta se abre en el panel
// principal y el diálogo se cierra.
func TestLaCuentaNuevaSaleEnElLateralAlVolverDelProveedor(t *testing.T) {
	s := &correoFalso{cuentas: api.ExternalMailAccounts{Providers: cuentaGmailNueva().Providers}}
	b := correoDePrueba(t, s).b
	w := test.NewWindow(b.P.Objeto())
	t.Cleanup(w.Close)
	b.P.Menu.AlAnadirExterna()
	t.Cleanup(func() {
		if b.cerrarCorreoExterno != nil {
			b.cerrarCorreoExterno()
		}
	})
	c := b.correoAbierto
	if c == nil {
		t.Fatal("«Añadir cuenta de correo» no ha abierto el diálogo")
	}
	c.proveedor, c.asistente, c.paso = "gmail", true, 1
	b.AutorizarCorreo = func(context.Context, string, string, ...string) (string, error) {
		s.cuentas = cuentaGmailNueva()
		return "nueva", nil
	}
	c.autorizar()
	fyne.DoAndWait(func() {})
	if len(b.P.Menu.Cuentas) != 1 || b.P.Menu.Cuentas[0].ID != "nueva" {
		t.Fatalf("el lateral no tiene la cuenta nueva: %v", b.P.Menu.Cuentas)
	}
	if b.correoAbierto != nil || b.externa == nil || b.externa.Cuenta != "nueva" || b.externa.Carpeta != "INBOX" {
		t.Fatalf("la cuenta nueva no se abre en el panel: diálogo %v, vista %+v", b.correoAbierto != nil, b.externa)
	}
}

// El lateral lee el resumen de las cuentas (`MailSummary`), no la lista del diálogo, y
// pinta las cuentas del árbol de buzones: con buzón y conectadas.
// VISTO EN ROJO (2026-11-06) con `MailAccounts` en `cargarExternas`.
func TestElLateralLeeElResumenDeLasCuentas(t *testing.T) {
	s := &correoFalso{cuentas: api.ExternalMailAccounts{Items: []api.ExternalMailAccount{
		{InstanceID: "trabajo", Provider: "imap", Address: "ana@trabajo.eu", Connected: true, Services: []string{"mail"}},
		{InstanceID: "calendario", Provider: "gmail", Address: "ana@calendario.eu", Connected: true, Services: []string{"calendar"}},
		{InstanceID: "caducada", Provider: "gmail", Address: "ana@gmail.com", Services: []string{"mail"}},
	}}}
	b := correoDePrueba(t, s).b
	b.cargarExternas()
	if s.resumenes != 1 || s.listados != 0 {
		t.Fatalf("el lateral pidió %d resúmenes y %d listas; quiero solo el resumen", s.resumenes, s.listados)
	}
	// La caducada sale también, pidiendo volver a iniciar sesión (fase B).
	if len(b.P.Menu.Cuentas) != 2 || b.P.Menu.Cuentas[0].ID != "trabajo" || b.P.Menu.Cuentas[0].Titulo != "ana@trabajo.eu" ||
		b.P.Menu.Cuentas[1].ID != "caducada" || b.P.Menu.Cuentas[1].Estado != api.ExternalMailReconnect {
		t.Fatalf("lateral = %+v; quiero las dos cuentas con buzón, la caducada por reconectar", b.P.Menu.Cuentas)
	}
}

// La lista del diálogo cambia el lateral: lo que se fue sale ya, y una cuenta nueva
// vuelve a pedir el resumen; la misma lista otra vez no lo pide.
// VISTO EN ROJO (2026-11-06): sin `resumenCaducado` en `fijarExternas`, la cuenta
// conectada con el diálogo abierto no salía en el lateral.
func TestLaListaDelDialogoPideElResumenSoloSiCambia(t *testing.T) {
	trabajo := api.ExternalMailAccount{InstanceID: "trabajo", Provider: "imap", Address: "ana@trabajo.eu", Connected: true, Services: []string{"mail"}}
	s := &correoFalso{cuentas: api.ExternalMailAccounts{Items: []api.ExternalMailAccount{trabajo}}}
	b := correoDePrueba(t, s).b
	b.cargarExternas()
	s.cuentas = cuentaGmailNueva()
	s.cuentas.Items = append([]api.ExternalMailAccount{trabajo}, s.cuentas.Items...)
	b.fijarExternas(s.cuentas)
	if s.resumenes != 2 || len(b.P.Menu.Cuentas) != 2 {
		t.Fatalf("tras una cuenta nueva: %d resúmenes, lateral %+v", s.resumenes, b.P.Menu.Cuentas)
	}
	b.fijarExternas(s.cuentas)
	if s.resumenes != 2 {
		t.Fatalf("la misma lista volvió a pedir el resumen (%d)", s.resumenes)
	}
	s.cuentas.Items = s.cuentas.Items[:1]
	b.fijarExternas(s.cuentas)
	if s.resumenes != 2 || len(b.P.Menu.Cuentas) != 1 || b.P.Menu.Cuentas[0].ID != "trabajo" {
		t.Fatalf("desconectar: %d resúmenes, lateral %+v", s.resumenes, b.P.Menu.Cuentas)
	}
}

// Con un resumen en camino, la lista del diálogo no pide otro: espera a ver si el que
// llega ya la trae (la vuelta del proveedor), y solo si no, lo pide.
// VISTO EN ROJO (2026-11-06): sin `externasPendientes`, al volver del proveedor se
// pedían dos resúmenes.
func TestUnResumenEnCaminoNoSePideDosVeces(t *testing.T) {
	s := &correoFalso{cuentas: cuentaGmailNueva()}
	b := correoDePrueba(t, s).b
	var cola []func()
	b.Lanzar = func(f func()) { cola = append(cola, f) }
	uno := func() {
		f := cola[0]
		cola = cola[1:]
		f()
		fyne.DoAndWait(func() {})
	}
	correr := func() {
		for len(cola) > 0 {
			uno()
		}
	}
	b.cargarExternas()
	b.fijarExternas(s.cuentas)
	correr()
	if s.resumenes != 1 || len(b.P.Menu.Cuentas) != 1 {
		t.Fatalf("vuelta del proveedor: %d resúmenes, lateral %+v", s.resumenes, b.P.Menu.Cuentas)
	}
	// Si el servidor contestó al que estaba en camino antes de conectar la cuenta, se
	// pide otra vez.
	nueva := cuentaGmailNueva()
	nueva.Items = append(nueva.Items, api.ExternalMailAccount{InstanceID: "otra", Provider: "imap", Address: "otra@example.eu", Connected: true})
	s.cuentas = api.ExternalMailAccounts{}
	b.cargarExternas()
	b.fijarExternas(nueva)
	uno()
	s.cuentas = nueva
	correr()
	if s.resumenes != 3 || len(b.P.Menu.Cuentas) != 2 {
		t.Fatalf("resumen viejo: %d resúmenes, lateral %+v", s.resumenes, b.P.Menu.Cuentas)
	}
}

// ramaDe busca la rama `rotulo` de una sección y devuelve su fila y los textos de lo
// que cuelga de ella (rótulo y contador), que es lo que pinta la sección abierta.
func ramaDe(t *testing.T, s *ui.SeccionLateral, rotulo string) (*ui.FilaLateral, []*ui.FilaLateral) {
	t.Helper()
	for i, o := range s.Filas {
		f, ok := o.(*ui.FilaLateral)
		if !ok || f.Rotulo != rotulo {
			continue
		}
		var hijos []*ui.FilaLateral
		if i+1 < len(s.Filas) {
			if sangria, ok := s.Filas[i+1].(*fyne.Container); ok {
				for _, h := range sangria.Objects[0].(*fyne.Container).Objects {
					hijos = append(hijos, h.(*ui.FilaLateral))
				}
			}
		}
		return f, hijos
	}
	t.Fatalf("no hay rama %q en %q", rotulo, filasDeTexto(s))
	return nil, nil
}

func rotulosDe(fs []*ui.FilaLateral) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Rotulo+f.Contador)
	}
	return out
}

var carpetasDeGmail = []api.ExternalMailFolder{
	{ID: "TRASH", Name: "TRASH", Role: "trash"},
	{ID: "Label_1", Name: "Facturas"},
	{ID: "INBOX", Name: "INBOX", Role: "inbox", Unread: 4},
	{ID: "SENT", Name: "SENT", Role: "sent"},
}

// Fase B, punto 3: cada cuenta es una rama plegable con sus carpetas, como en Apple
// Mail y como `ExternalAccountsSection`. VISTO EN ROJO (2026-11-06) con `f.Name` en
// vez de `RotuloCarpeta` («INBOX» en el lateral) y con el contador también abierta.
func TestCadaCuentaSeDespliegaConSusCarpetas(t *testing.T) {
	s := &correoFalso{cuentas: cuentaGmailNueva(), carpetas: map[string][]api.ExternalMailFolder{"nueva": carpetasDeGmail}}
	b := correoDePrueba(t, s).b
	w := test.NewWindow(b.P.Objeto())
	t.Cleanup(w.Close)
	b.cargarExternas()
	tr := b.P.T
	cuenta, hijos := ramaDe(t, b.P.Menu.seccionExternas(tema.Claro), "ana@example.eu")
	if want := []string{tr.T("folder.inbox", nil) + "4", tr.T("folder.sent", nil), tr.T("folder.trash", nil), "Facturas"}; !slices.Equal(rotulosDe(hijos), want) {
		t.Fatalf("carpetas %q; quiero %q", rotulosDe(hijos), want)
	}
	if cuenta.Contador != "" || !cuenta.MantieneCajon {
		t.Fatalf("abierta, la cuenta lleva contador %q (lo dice su Entrada) o recoge el cajón", cuenta.Contador)
	}
	cuenta.AlPulsar()
	cuenta, hijos = ramaDe(t, b.P.Menu.seccionExternas(tema.Claro), "ana@example.eu")
	if len(hijos) != 0 || cuenta.Contador != "4" || b.correoAbierto != nil {
		t.Fatalf("plegada: hijos %q, contador %q, diálogo abierto %v", rotulosDe(hijos), cuenta.Contador, b.correoAbierto != nil)
	}
	cuenta.AlPulsar()
	_, hijos = ramaDe(t, b.P.Menu.seccionExternas(tema.Claro), "ana@example.eu")
	hijos[1].AlPulsar()
	fyne.DoAndWait(func() {})
	// Fase B, punto 4: en el panel principal, no en un diálogo, y marcada en el lateral.
	// VISTO EN ROJO (2026-11-06) con el mini-buzón de antes: se abría un diálogo.
	if v := b.externa; v == nil || v.Cuenta != "nueva" || v.Carpeta != "SENT" || b.correoAbierto != nil {
		t.Fatalf("pulsar Enviados no abre esa carpeta de la cuenta en el panel: %+v", v)
	}
	_, hijos = ramaDe(t, b.P.Menu.seccionExternas(tema.Claro), "ana@example.eu")
	if !hijos[1].Activa || hijos[0].Activa {
		t.Fatal("la carpeta abierta no se marca en el lateral")
	}
}

// VISTO EN ROJO (2026-11-06): con el filtro de `Connected` de antes, la cuenta que
// perdió el permiso desaparecía del lateral sin decir por qué.
func TestUnaCuentaQuePerdioElPermisoLoDiceEnSuSitio(t *testing.T) {
	s := &correoFalso{
		cuentas: api.ExternalMailAccounts{Items: []api.ExternalMailAccount{
			{InstanceID: "caducada", Provider: "gmail", Address: "ana@gmail.com", Services: []string{"mail"}},
			{InstanceID: "lenta", Provider: "imap", Address: "ana@trabajo.eu", Connected: true, Services: []string{"mail"}},
		}},
		estados: map[string]string{"lenta": api.ExternalMailUnavailable},
	}
	b := correoDePrueba(t, s).b
	b.cargarExternas()
	tr := b.P.T
	sec := b.P.Menu.seccionExternas(tema.Claro)
	_, caducada := ramaDe(t, sec, "ana@gmail.com")
	_, lenta := ramaDe(t, sec, "ana@trabajo.eu")
	if want := []string{tr.T("externalMail.reconnectNeeded", nil)}; !slices.Equal(rotulosDe(caducada), want) {
		t.Fatalf("la caducada dice %q; quiero %q", rotulosDe(caducada), want)
	}
	if want := []string{tr.T("externalMail.accountUnavailable", nil)}; !slices.Equal(rotulosDe(lenta), want) {
		t.Fatalf("la que no contestó dice %q; quiero %q", rotulosDe(lenta), want)
	}
	var abierta string
	b.P.Menu.AlAbrirExterna = func(id, carpeta string) { abierta = id + "/" + carpeta }
	caducada[0].AlPulsar()
	lenta[0].AlPulsar()
	if abierta != "caducada/" || s.resumenes != 2 {
		t.Fatalf("reconectar abre %q y reintentar pide %d resúmenes", abierta, s.resumenes)
	}
}

// «Buzones», arriba: solo con alguna cuenta externa, con la suma de no leídos y cada
// buzón desplegable en el de cada cuenta (`MailboxesSection`). VISTO EN ROJO
// (2026-11-06) sin el `len(m.Cuentas) == 0` (salía sin cuentas) y con las ramas de
// «Buzones» abiertas de serie.
func TestBuzonesJuntaLaEntradaDeSytueYLaDeCadaCuenta(t *testing.T) {
	s := &correoFalso{cuentas: api.ExternalMailAccounts{}, carpetas: map[string][]api.ExternalMailFolder{"nueva": carpetasDeGmail}}
	b := correoDePrueba(t, s).b
	b.ponerCarpetas([]api.Mailbox{{ID: "sytue-inbox", Name: "Inbox", Role: "inbox", UnreadEmails: 2}})
	b.carpetaID = "sytue-inbox"
	b.P.Menu.CarpetaID = "sytue-inbox"
	b.cargarExternas()
	if sec := b.P.Menu.seccionBuzones(tema.Claro); sec != nil {
		t.Fatalf("«Buzones» sale sin cuentas externas: %q", filasDeTexto(sec))
	}
	s.cuentas = cuentaGmailNueva()
	b.cargarExternas()
	tr := b.P.T
	secciones := b.P.Menu.secciones(tema.Claro)
	if primera := secciones[0].(*ui.SeccionLateral); primera.Titulo != tr.T("nav.mailboxes", nil) {
		t.Fatalf("la primera sección es %q; quiero «Buzones»", primera.Titulo)
	}
	todas, hijos := ramaDe(t, b.P.Menu.seccionBuzones(tema.Claro), tr.T("nav.allInboxes", nil))
	if todas.Contador != "6" || len(hijos) != 0 {
		t.Fatalf("«Todos los buzones de entrada»: contador %q, abierta con %q", todas.Contador, rotulosDe(hijos))
	}
	// Se despliega con su triángulo, aparte de la fila.
	todas.Accion.AlPulsar()
	_, hijos = ramaDe(t, b.P.Menu.seccionBuzones(tema.Claro), tr.T("nav.allInboxes", nil))
	// Cada fuente con su contador, Sytue también: 2 + 4 es el 6 de la rama. VISTO EN
	// ROJO (2026-11-06) con `fuenteSytue` sin Contador.
	if want := []string{tr.T("nav.sytueMailbox", nil) + "2", "ana@example.eu4"}; !slices.Equal(rotulosDe(hijos), want) {
		t.Fatalf("fuentes %q; quiero %q", rotulosDe(hijos), want)
	}
	if !hijos[0].Activa {
		t.Fatal("la Entrada de Sytue que se está viendo no se marca")
	}
	var elegida string
	b.P.Menu.AlElegirCarpeta = func(c Carpeta) { elegida = c.ID }
	var abierta string
	b.P.Menu.AlAbrirExterna = func(id, carpeta string) { abierta = id + "/" + carpeta }
	hijos[0].AlPulsar()
	hijos[1].AlPulsar()
	if elegida != "sytue-inbox" || abierta != "nueva/INBOX" {
		t.Fatalf("Sytue elige %q y la cuenta abre %q", elegida, abierta)
	}
	// Con la dirección principal, el buzón de Sytue se llama por ella.
	b.principal = "ana@sytue.eu"
	b.ponerArbol()
	_, hijos = ramaDe(t, b.P.Menu.seccionBuzones(tema.Claro), tr.T("nav.allInboxes", nil))
	if hijos[0].Rotulo != "ana@sytue.eu" {
		t.Fatalf("el buzón de Sytue se llama %q", hijos[0].Rotulo)
	}
}

// Con todas las cuentas pidiendo volver a iniciar sesión, ninguna aporta carpetas y
// «Buzones» sería «Carpetas» otra vez (`MailboxesSection`). VISTO EN ROJO (2026-11-06)
// con el `len(m.Cuentas) == 0` de antes.
func TestBuzonesNoSaleSiNingunaCuentaAportaCarpetas(t *testing.T) {
	s := &correoFalso{cuentas: api.ExternalMailAccounts{Items: []api.ExternalMailAccount{
		{InstanceID: "caducada", Provider: "gmail", Address: "ana@gmail.com", Services: []string{"mail"}},
	}}}
	b := correoDePrueba(t, s).b
	b.ponerCarpetas([]api.Mailbox{{ID: "sytue-inbox", Name: "Inbox", Role: "inbox", UnreadEmails: 2}})
	b.cargarExternas()
	if len(b.P.Menu.Cuentas) != 1 {
		t.Fatalf("la cuenta caducada no está en el lateral: %+v", b.P.Menu.Cuentas)
	}
	if sec := b.P.Menu.seccionBuzones(tema.Claro); sec != nil {
		t.Fatalf("«Buzones» sale solo con Sytue: %q", filasDeTexto(sec))
	}
}

// El diálogo ya no lee correo, así que cerrarlo no vuelve a pedir el resumen; lo que
// baja los contadores es leer en el panel principal. VISTO EN ROJO (2026-11-06): sin
// `refrescarContadoresDe` al marcar leído uno externo, «4» seguía en la cuenta y «6» en
// «Buzones» hasta reiniciar la app.
func TestLeerEnElPanelBajaLosContadoresExternos(t *testing.T) {
	s := &correoFalso{
		cuentas: cuentaGmailNueva(), carpetas: map[string][]api.ExternalMailFolder{"nueva": carpetasDeGmail},
		paginas: map[string]api.ExternalMailPage{"nueva/INBOX/": {Items: []api.ExternalMailMessage{{ID: "7", Subject: "Hola", Date: "2026-11-06T10:00:00Z"}}}},
	}
	b := correoDePrueba(t, s).b
	w := test.NewWindow(b.P.Objeto())
	t.Cleanup(w.Close)
	b.ponerCarpetas([]api.Mailbox{{ID: "sytue-inbox", Name: "Inbox", Role: "inbox", UnreadEmails: 2}})
	b.cargarExternas()
	b.abrirCorreoExterno()
	antes := s.resumenes
	b.correoAbierto.pedirCerrar()
	if b.correoAbierto != nil || s.resumenes != antes {
		t.Fatalf("cerrar: diálogo abierto %v, %d resúmenes (antes %d)", b.correoAbierto != nil, s.resumenes, antes)
	}
	b.ElegirExterna("nueva", "INBOX")
	// Lo que dice el proveedor tras leerlo.
	leidas := slices.Clone(carpetasDeGmail)
	for i := range leidas {
		leidas[i].Unread = 0
	}
	s.carpetas = map[string][]api.ExternalMailFolder{"nueva": leidas}
	b.Abrir(b.P.Bandeja.Conversaciones[0])
	tr := b.P.T
	todas, _ := ramaDe(t, b.P.Menu.seccionBuzones(tema.Claro), tr.T("nav.allInboxes", nil))
	if s.resumenes != antes+1 || todas.Contador != "2" || b.P.Menu.Cuentas[0].NoLeidos != 0 {
		t.Fatalf("después: %d resúmenes, «Buzones» %q, cuenta %d; quiero uno más, 2 y 0", s.resumenes, todas.Contador, b.P.Menu.Cuentas[0].NoLeidos)
	}
}
