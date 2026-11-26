// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command mail es Sytue Mail de Sytue Office: un cliente de correo para las cuentas
// externas de la persona (Outlook, Gmail, iCloud, Yahoo, cualquier IMAP y el IMAP de
// Sytue Mail como una cuenta más), con el correo descargado en este equipo y en claro
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.9).
//
// Comparado con `cmd/mail`: no hay `sesion.Abrir` ni `cifrado.NewPrivateApplication`
// (ni OIDC, ni Matrix, ni repositorio cifrado). La pantalla es la misma
// (`apps/mail`), enchufada a `nucleo/local/correo`, que lee y escribe en local y
// sincroniza por IMAP y SMTP. Sin cuentas, la app enseña la bienvenida y el alta, y no
// abre ninguna conexión.
package main

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/apps/mail"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	cuentas "git.sytue.eu/sytue/one/apps/fyne/local/correo"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// La pantalla pide `mail.Servicio`; el almacén local lo cumple entero.
var _ mail.Servicio = (*correo.Buzon)(nil)

// ocultas son las filas de Ajustes que en local no existen (§6.9, M-11). «Vacaciones» no
// está: es la respuesta automática de cada cuenta en su servidor (ManageSieve, M-10), y la
// pantalla la enseña solo cuando alguna cuenta la ofrece (`api.MailRespuestas`).
var ocultas = map[string]bool{
	"aliases": true, "shared": true,
	"smime-anchor": true, "smime-identity": true, "mail-clients": true,
}

// La respuesta automática por cuenta (M-10) la da el correo local.
var _ api.MailRespuestas = (*correo.Buzon)(nil)

func main() {
	a := local.NuevaApp("")
	l, err := local.Abrir(a, "mail")
	if err != nil {
		slog.Error("mail", "error", err)
		os.Exit(1)
	}
	tema.ElegirCromo(tema.CromoPapel)
	t, err := mail.Traductor(l.Idioma())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}
	// El correo vive en `<Datos>/Correo`: un índice SQLite y los .eml de cada cuenta.
	buzon, err := correo.Abrir(filepath.Join(l.Espacio.Rutas.Datos, "Correo"), l.Secretos,
		// Por el portero (§5.8): solo los servidores de las cuentas dadas de alta. Las
		// imágenes remotas y la baja de un clic, con un permiso puntual de cada petición,
		// que Mail solo hace cuando la persona lo pide (ver ese mensaje o ese remitente,
		// «Darse de baja»).
		correo.Red{
			Marcar: l.Red.Marcador(red.Correo), HTTP: l.Red.Cliente(red.OAuth),
			Web: l.Red.Puntual(red.ImagenRemota, ""), Baja: l.Red.Puntual(red.BajaDeLista, ""),
		})
	if err != nil {
		slog.Error("no se pudo abrir el correo", "error", err)
		os.Exit(1)
	}
	l.AlCerrar(buzon.Cerrar)
	// La copia de seguridad copia el índice abierto en caliente (§5.13); se quita antes
	// de cerrarlo (AlCerrar va del último al primero).
	l.AlCerrar(l.Copia.BaseAbierta(buzon.RutaIndice(), buzon.CopiarIndice))

	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	ctx := context.Background()

	p := mail.NuevaPantalla(t, l.Idioma())
	box := mail.NuevoBuzon(p, buzon)
	box.Abridor = a
	box.AjustesOcultos = ocultas
	p.Menu.DesplazadorNuevo = true
	// La IA del redactor es la que la persona configuró en Ajustes (M-15), y se
	// vuelve a enchufar si la cambia.
	// Lo propio de Sytue Office: el lector con formato (imágenes remotas bloqueadas por
	// defecto, M-16), las invitaciones con Sí/Quizá/No (M-17), «Crear evento» en Calendar
	// local (M-14) y la libreta de Contacts de este equipo en solo lectura (M-13).
	abridor := cuentas.Abridor{A: a}
	box.ConFormato()
	box.ConInvitaciones(cuentas.Invitaciones{B: buzon, A: a, Dir: filepath.Join(os.TempDir(), "sytue-office-mail")})
	box.ConHub(cuentas.Intents{}, abridor)
	p.Lector.AlCompartirEnRooms = nil // sin Rooms de la suite a la que compartir
	// La libreta es la de Contacts, en solo lectura: la misma que autocompleta los
	// invitados en Calendar (`lector.Libreta`). Con la libreta SQLite de Contacts
	// (`local/libreta`) cada letra es una consulta con índice a la base, que ve lo último
	// que Contacts guardó; mientras Contacts siga en su almacén de antes, se lee detrás
	// desde ya (con 20.000 contactos, unos segundos) y la primera letra no espera.
	libreta := lector.Nuevo(l.Espacio.Rutas.Producto("contacts"), l.Espacio.ID).Libreta()
	libreta.Precargar()
	box.ConContactos(libreta)
	box.ConLibreta(libreta, "")
	box.Asistir = l.Asistir()
	l.AlCambiarIA(func() { fyne.Do(func() { box.Asistir = l.Asistir() }) })
	l.AlCerrar(box.Parar)

	gestor := &cuentas.Gestor{L: l, B: buzon, W: w}
	// «Añadir cuenta de correo» del lateral abre las cuentas locales (no las
	// externas del servidor de Sytue, que aquí no existe).
	p.Menu.AlAnadirExterna = func() { p.CerrarCajon(); gestor.Gestionar() }
	// «Actualizar» sincroniza con los servidores además de releer la lista.
	p.Menu.AlRefrescar = func() { buzon.SincronizarYa(); box.Refrescar() }
	buzon.AlFallar = func(c correo.Cuenta, err error) {
		fyne.Do(func() { p.Avisar(ui.AvisoPeligro, c.Titulo(), err.Error()) })
	}
	// El correo nuevo, en cuanto llega por IDLE: aviso del sistema y número en el icono.
	buzon.AlLlegar = func(ls []correo.Llegada) {
		gestor.AvisarLlegadas(ls, func(hilo string) { fyne.Do(func() { box.AbrirHilo(hilo) }) })
	}
	buzon.AlCambiar(func() {
		go func() {
			if cs, err := buzon.Mailboxes(context.Background()); err == nil {
				_ = escritorio.Insignia(a.Metadata().ID, api.MailInboxUnread(cs))
			}
		}()
	})

	marco := container.NewStack()
	mostrar := func() {
		if len(buzon.Cuentas()) == 0 {
			marco.Objects = []fyne.CanvasObject{gestor.Bienvenida()}
		} else {
			marco.Objects = []fyne.CanvasObject{p.Objeto()}
		}
		marco.Refresh()
	}
	gestor.AlCambiar = func() {
		mostrar()
		// Las direcciones «De» y las carpetas cambian con las cuentas.
		box.Arrancar(ctx)
	}

	// Un .eml del equipo («Abrir con», soltarlo en la ventana) se lee en «Importados».
	l.AlAbrirFichero(func(ctx context.Context, ap api.Apertura) error {
		_, datos, err := local.LeerLocal(ap.Local)
		if err != nil {
			return err
		}
		m, err := buzon.ImportarEML(ctx, datos)
		if err != nil {
			return err
		}
		//nolint:contextcheck // refrescar y abrir el hilo es de la pantalla, no de la importación
		fyne.Do(func() {
			if len(buzon.Cuentas()) == 0 {
				marco.Objects = []fyne.CanvasObject{p.Objeto()}
				marco.Refresh()
			}
			box.Refrescar()
			box.AbrirHilo(m.ThreadID)
		})
		return nil
	}, func(err error) { l.Fallo(w, err) })
	l.AceptarFicheros(w)
	soltar.AlSoltar(w, func(x soltar.Soltado) { box.Soltar(x.ComoTexto(), x.Posicion) })

	w.SetContent(l.Montar(w, iconos.Mail, marco))
	w.Resize(fyne.NewSize(1280, 800))
	mostrar()
	box.Atajos(w.Canvas())
	// Una sola «sesión»: la de este equipo. Arranca la pantalla (carpetas, firma,
	// direcciones) y, debajo, la sincronización y la bandeja de salida.
	box.CambiarSesion(ctx, "local")
	buzon.Arrancar()
	// `mailto:` y `sytueoffice-mail:/redactar?…` abren el redactor: llegan por los
	// argumentos (el sistema abre así el programa de correo en Linux y Windows) o por
	// `SetOnOpenURL` (macOS, iOS, Android), y la cáscara los entrega aquí.
	// Una conversación (`?clase=hilo&id=…`): el Hub, otra app.
	l.AlAbrirElemento(func(clase, id string) {
		if clase == "hilo" {
			box.AbrirHilo(id)
		}
	})
	// Ajustes → Cuentas de cualquier app de la edición: añadir, volver a entrar y quitar.
	gestor.Registrar()
	// Lo que otras apps dejan para adjuntar («para-enviar», el PDF de una factura del
	// ERP) se recoge por su identificador al leer el enlace; lo que nadie recogió en un
	// día se barre al arrancar.
	salida := correo.ParaEnviar{Dir: l.Espacio.Rutas.ParaEnviar}
	go salida.Barrer()
	l.AlAbrirEnlace(func(u *url.URL) {
		if gestor.AtenderEnlace(u) {
			return
		}
		go func() { // recoger lee ficheros
			if r, ok := cuentas.LeerEnlace(u, salida); ok {
				fyne.Do(func() {
					box.RedactarDesde(r)
					// Sin cuentas se ve la bienvenida, no la pantalla de Mail con sus
					// avisos: que un adjunto no llegó se dice encima del redactor.
					if r.SinAdjuntar > 0 && len(buzon.Cuentas()) == 0 {
						local.Info(w, t.T("compose.attach", nil), t.T("compose.attachFailed", nil))
					}
				})
			}
		}()
	})
	w.ShowAndRun()
}
