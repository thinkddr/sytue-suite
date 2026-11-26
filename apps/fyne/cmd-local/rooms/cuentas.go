// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/chat"
	"git.sytue.eu/sytue/one/apps/nucleo/local/cuentas"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// textos son los de esta pantalla, que no está en el catálogo de Rooms (es solo de la
// edición local), ya en el idioma de la app: salen de `tablaTextos`.
type textos struct {
	notas, primerCanal, primerAsunto       string
	cuentas, titulo, sinCuentas, quitar    string
	anadir, servidor, usuario, clave       string
	token, tokenAyuda, aviso, conectando   string
	notasAyuda, quitarPregunta, cerrar, ok string
	yo                                     string
	// Slack (D-20): la app propia del espacio.
	slack, slackToken, slackApp, slackAyuda, copiar, copiado string
	// cuenta es el rótulo del selector de «Crear canal».
	cuenta string
	// Teams y Google Chat (RO-05, RO-06): entran por OAuth en el navegador.
	teams, teamsAyuda, teamsCanales, cliente, clienteAyuda string
	google, googleAyuda, secreto, navegador, volver        string
	// Lo que dicen las conversaciones cuando el proveedor no deja leer.
	sinConsentimiento, sinLectura string
	// Los permisos que se piden si se marcan (adjuntos de Teams, directos nuevos de
	// Google Chat) y quién escribe en Google Chat cuando Google no lo dice.
	teamsFicheros, googleDirectos, personaGoogle string
}

// tablaTextos son los textos de `textos`, en español e inglés; los otros veinticuatro
// idiomas están en textos_idiomas.go (`TestLaTablaDeRoomsCuadra` exige las mismas claves
// en todos). La clave es el nombre del campo.
var tablaTextos = map[string][2]string{ // campo: {es, en}
	"notas":             {"Notas", "Notes"},
	"primerCanal":       {"notas", "notes"},
	"primerAsunto":      {"Notas para mí, guardadas en este equipo", "Notes to self, stored on this device"},
	"cuentas":           {"Cuentas de mensajería", "Messaging accounts"},
	"titulo":            {"Cuentas de mensajería", "Messaging accounts"},
	"sinCuentas":        {"No hay cuentas conectadas. Las notas funcionan sin ninguna.", "No accounts connected. Notes work without any."},
	"quitar":            {"Quitar", "Remove"},
	"anadir":            {"Añadir cuenta Matrix", "Add Matrix account"},
	"servidor":          {"Servidor (https://matrix.example.org)", "Server (https://matrix.example.org)"},
	"usuario":           {"Usuario", "User"},
	"clave":             {"Contraseña", "Password"},
	"token":             {"Token de acceso (opcional)", "Access token (optional)"},
	"tokenAyuda":        {"Con token no hace falta contraseña. La contraseña no se guarda: solo el token, en el llavero del sistema.", "With a token no password is needed. The password is not stored: only the token, in the system keychain."},
	"aviso":             {"Sytue Rooms online y cualquier servidor Matrix. Solo salas SIN cifrado de extremo a extremo: las salas cifradas se ven en la lista pero Sytue Office no puede leerlas ni escribir en ellas.", "Sytue Rooms online and any Matrix server. Only rooms WITHOUT end-to-end encryption: encrypted rooms are listed but Sytue Office cannot read or write them."},
	"conectando":        {"Conectando…", "Connecting…"},
	"notasAyuda":        {"Notas: canales para ti en este equipo, en claro, sin red.", "Notes: channels for yourself on this device, in plain text, offline."},
	"quitarPregunta":    {"¿Quitar esta cuenta de Sytue Office? Sus mensajes siguen en el servidor.", "Remove this account from Sytue Office? Its messages stay on the server."},
	"cerrar":            {"Cerrar", "Close"},
	"ok":                {"Conectar", "Connect"},
	"yo":                {"Yo", "Me"},
	"slack":             {"Añadir espacio de Slack", "Add Slack workspace"},
	"slackToken":        {"Token de usuario", "User token"},
	"slackApp":          {"Token de la app", "App token"},
	"slackAyuda":        {"En api.slack.com/apps crea una app «From a manifest» con el manifiesto de abajo, instálala en tu espacio y pega su «User OAuth Token». Con el token de la app (Socket Mode) los mensajes llegan al instante; sin él, se revisan cada 20 s. Es una app interna de TU espacio: Slack no le aplica los límites de las apps distribuidas. Si la creaste antes de que hubiera adjuntos, añádele los permisos files:read, files:write y users:read.email y reinstálala.", "At api.slack.com/apps create an app «From a manifest» with the manifest below, install it in your workspace and paste its «User OAuth Token». With the app token (Socket Mode) messages arrive instantly; without it, they are checked every 20 s. It is an internal app of YOUR workspace: Slack does not apply the limits of distributed apps to it. If you created it before attachments existed, add the files:read, files:write and users:read.email scopes and reinstall it."},
	"copiar":            {"Copiar el manifiesto", "Copy the manifest"},
	"copiado":           {"Manifiesto copiado.", "Manifest copied."},
	"cuenta":            {"Cuenta", "Account"},
	"teams":             {"Añadir Microsoft Teams", "Add Microsoft Teams"},
	"teamsAyuda":        {"Cuentas de trabajo o de centro educativo (Microsoft 365); Teams personal no está en la API de Microsoft. Se abre el navegador para entrar. Los chats se leen y se escriben aquí, y los mensajes nuevos se miran cada poco porque Microsoft no avisa a una app sin servidor. Las reuniones se crean aquí y se entra en ellas con la app de Teams.", "Work or school accounts (Microsoft 365); personal Teams is not available in Microsoft's API. The browser opens to sign in. Chats are read and written here, and new messages are checked every so often because Microsoft does not notify an app without a server. Meetings are created here and joined in the Teams app."},
	"teamsCanales":      {"También los canales de los equipos (leerlos pide el consentimiento del administrador de la organización; si no lo da, desmárcalo para entrar)", "Also team channels (reading them needs the organization administrator's consent; if it is not given, untick this to sign in)"},
	"cliente":           {"ID de cliente OAuth (opcional)", "OAuth client ID (optional)"},
	"clienteAyuda":      {"Vacío, el de Sytue Office. Si tu organización no admite apps de terceros, pon el de una app que haya registrado ella.", "Leave empty to use Sytue Office's. If your organization does not allow third-party apps, enter one it has registered."},
	"google":            {"Añadir Google Chat y Meet", "Add Google Chat and Meet"},
	"googleAyuda":       {"Cuentas de Google Workspace. Con la app de Sytue Office puedes escribir en tus espacios y crear enlaces de Meet. Google solo deja leer los mensajes a las apps de terceros tras una auditoría: para leerlos, crea en la consola de Google Cloud de tu Workspace un cliente OAuth de escritorio interno, activa la API de Google Chat y pon aquí su ID y su secreto.", "Google Workspace accounts. With Sytue Office's app you can write in your spaces and create Meet links. Google only lets third-party apps read messages after an audit: to read them, create an internal desktop OAuth client in your Workspace's Google Cloud console, enable the Google Chat API and enter its ID and secret here."},
	"secreto":           {"Secreto del cliente (opcional)", "Client secret (optional)"},
	"navegador":         {"Se ha abierto el navegador: entra en la cuenta y acepta los permisos.", "The browser has opened: sign in and accept the permissions."},
	"volver":            {"Volver a entrar", "Sign in again"},
	"sinConsentimiento": {"Leer los canales de Teams pide que el administrador de tu organización consienta el permiso ChannelMessage.Read.All. Los chats funcionan sin él.", "Reading Teams channels needs your organization's administrator to consent to the ChannelMessage.Read.All permission. Chats work without it."},
	"sinLectura":        {"Desde aquí puedes escribir en este espacio. Para leer sus mensajes, da de alta la cuenta con un cliente OAuth propio de tu Google Workspace en «Cuentas de mensajería».", "From here you can write in this space. To read its messages, add the account with your own Google Workspace OAuth client in «Messaging accounts»."},
	"teamsFicheros":     {"Adjuntos: mandar ficheros (se suben a tu OneDrive, como hace Teams) y abrir los que te manden. Pide a Microsoft un permiso más (Files.ReadWrite.All)", "Attachments: send files (they are uploaded to your OneDrive, as Teams does) and open the ones you receive. Asks Microsoft for one more permission (Files.ReadWrite.All)"},
	"googleDirectos":    {"Mensajes directos nuevos: escribir a quien aún no tiene una conversación contigo. Pide a Google un permiso más (chat.spaces.create)", "New direct messages: write to someone you do not have a conversation with yet. Asks Google for one more permission (chat.spaces.create)"},
	"personaGoogle":     {"Persona de Google", "Google user"},
}

var tabla = local.Propios{Base: tablaTextos, Traducciones: traducciones}

// textosDe son los textos de la pantalla en el idioma dado (o en inglés, si no lo tienen).
func textosDe(idioma string) textos {
	t := func(campo string) string { return tabla.En(i18n.Locale(idioma), campo) }
	return textos{
		notas: t("notas"), primerCanal: t("primerCanal"), primerAsunto: t("primerAsunto"),
		cuentas: t("cuentas"), titulo: t("titulo"), sinCuentas: t("sinCuentas"), quitar: t("quitar"),
		anadir: t("anadir"), servidor: t("servidor"), usuario: t("usuario"), clave: t("clave"),
		token: t("token"), tokenAyuda: t("tokenAyuda"), aviso: t("aviso"), conectando: t("conectando"),
		notasAyuda: t("notasAyuda"), quitarPregunta: t("quitarPregunta"), cerrar: t("cerrar"), ok: t("ok"),
		yo: t("yo"), cuenta: t("cuenta"),
		slack: t("slack"), slackToken: t("slackToken"), slackApp: t("slackApp"), slackAyuda: t("slackAyuda"),
		copiar: t("copiar"), copiado: t("copiado"),
		teams: t("teams"), teamsAyuda: t("teamsAyuda"), teamsCanales: t("teamsCanales"), cliente: t("cliente"), clienteAyuda: t("clienteAyuda"),
		google: t("google"), googleAyuda: t("googleAyuda"), secreto: t("secreto"), navegador: t("navegador"), volver: t("volver"),
		sinConsentimiento: t("sinConsentimiento"), sinLectura: t("sinLectura"),
		teamsFicheros: t("teamsFicheros"), googleDirectos: t("googleDirectos"), personaGoogle: t("personaGoogle"),
	}
}

// avisos son los textos que las cuentas ponen en las conversaciones.
func (tx textos) avisos() chat.Avisos {
	return chat.Avisos{SinConsentimiento: tx.sinConsentimiento, SinLectura: tx.sinLectura, PersonaGoogle: tx.personaGoogle}
}

func ayuda(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	return l
}

// mostrarCuentas es «Cuentas de mensajería»: las conectadas, quitar una y dar de alta
// una Matrix. listo recarga los canales de la sala.
func mostrarCuentas(w fyne.Window, l *local.App, reg *chat.Registro, agg *chat.Agregador, tx textos, listo func()) {
	var d *ui.Dialogo
	reabrir := func() {
		d.Cerrar()
		listo()
		mostrarCuentas(w, l, reg, agg, tx, listo)
	}
	lista := container.NewVBox(ayuda(tx.notasAyuda))
	conectadas := reg.Cuentas()
	if len(conectadas) == 0 {
		lista.Add(ayuda(tx.sinCuentas))
	}
	for _, c := range conectadas {
		quitar := widget.NewButton(tx.quitar, func() {
			local.Confirmar(w, tx.quitar, tx.quitarPregunta, tx.cerrar, tx.quitar, true, func() {
				if err := reg.Baja(c.ID); err != nil {
					l.Fallo(w, err)
					return
				}
				agg.Quitar(c.ID)
				reabrir()
			})
		})
		botones := container.NewHBox(quitar)
		if chat.EsOAuth(c) && c.Estado == chat.EstadoReconectar {
			volver := widget.NewButton(tx.volver, func() {
				d.Cerrar()
				reconectarCuenta(w, l, reg, agg, tx, c.ID, func() { listo(); mostrarCuentas(w, l, reg, agg, tx, listo) })
			})
			volver.Importance = widget.HighImportance
			botones.Objects = append([]fyne.CanvasObject{volver}, botones.Objects...)
		}
		rotulo := c.Rotulo
		switch {
		case c.Servidor != "":
			rotulo += " · " + c.Servidor
		case c.Usuario != "" && c.Usuario != c.Rotulo:
			rotulo += " · " + c.Usuario
		}
		lista.Add(container.NewBorder(nil, nil, nil, botones, widget.NewLabel(rotulo)))
	}
	otra := func(alta func(fyne.Window, *local.App, *chat.Registro, *chat.Agregador, textos, func())) func() {
		return func() {
			d.Cerrar()
			alta(w, l, reg, agg, tx, func() { listo(); mostrarCuentas(w, l, reg, agg, tx, listo) })
		}
	}
	anadir := widget.NewButton(tx.anadir, otra(altaMatrix))
	anadir.Importance = widget.HighImportance
	altas := container.NewGridWithColumns(2, anadir, widget.NewButton(tx.slack, otra(altaSlack)),
		widget.NewButton(tx.teams, otra(altaTeams)), widget.NewButton(tx.google, otra(altaGoogle)))
	cuerpo := container.NewVBox(lista, widget.NewSeparator(), ayuda(tx.aviso), altas)
	d = local.Ventana(w, tx.titulo, cuerpo, nil)
}

// quitarCuenta pregunta y quita la cuenta id (desde Ajustes → Cuentas o su enlace).
func quitarCuenta(w fyne.Window, l *local.App, reg *chat.Registro, agg *chat.Agregador, tx textos, id string, listo func()) {
	for _, c := range reg.Cuentas() {
		if c.ID != id {
			continue
		}
		local.Confirmar(w, tx.quitar, c.Rotulo+"\n\n"+tx.quitarPregunta, tx.cerrar, tx.quitar, true, func() {
			if err := reg.Baja(c.ID); err != nil {
				l.Fallo(w, err)
				return
			}
			agg.Quitar(c.ID)
			listo()
		})
		return
	}
}

func altaMatrix(w fyne.Window, l *local.App, reg *chat.Registro, agg *chat.Agregador, tx textos, listo func()) {
	servidor, usuario, token := widget.NewEntry(), widget.NewEntry(), widget.NewPasswordEntry()
	clave := widget.NewPasswordEntry()
	servidor.SetPlaceHolder("https://")
	usuario.SetPlaceHolder("@ana:matrix.org")
	form := []*widget.FormItem{
		widget.NewFormItem(tx.servidor, servidor),
		widget.NewFormItem(tx.usuario, usuario),
		widget.NewFormItem(tx.clave, clave),
		widget.NewFormItem(tx.token, token),
	}
	form[3].HintText = tx.tokenAyuda
	local.Formulario(w, tx.anadir, tx.cerrar, tx.ok, form, func(d *ui.Dialogo) {
		d.Cerrar()
		espera := local.Espera(w, tx.anadir, tx.conectando)
		go func() {
			ctx, cancelar := context.WithTimeout(context.Background(), time.Minute)
			defer cancelar()
			// Mientras se prueba, el portero deja ir al servidor escrito.
			defer l.Red.PermitirURL(red.Chat, servidor.Text, servidor.Text)()
			c, p, err := reg.AltaMatrix(ctx, servidor.Text, usuario.Text, clave.Text, token.Text)
			fyne.Do(func() {
				espera.Cerrar()
				if err != nil {
					l.Fallo(w, err)
					return
				}
				agg.Poner(c.ID, p)
				listo()
			})
		}()
	})
}

func altaSlack(w fyne.Window, l *local.App, reg *chat.Registro, agg *chat.Agregador, tx textos, listo func()) {
	token, app := widget.NewPasswordEntry(), widget.NewPasswordEntry()
	token.SetPlaceHolder("xoxp-…")
	app.SetPlaceHolder("xapp-… (opcional / optional)")
	manifiesto := widget.NewMultiLineEntry()
	manifiesto.SetText(chat.ManifiestoSlack)
	manifiesto.SetMinRowsVisible(6)
	copiar := widget.NewButton(tx.copiar, func() {
		l.A.Clipboard().SetContent(chat.ManifiestoSlack)
		local.Info(w, tx.slack, tx.copiado)
	})
	campos := widget.NewForm(widget.NewFormItem(tx.slackToken, token), widget.NewFormItem(tx.slackApp, app))
	cuerpo := container.NewVBox(ayuda(tx.slackAyuda), manifiesto, container.NewHBox(copiar), widget.NewSeparator(), campos)
	var d *ui.Dialogo
	d = ui.Confirmacion(tx.slack, tx.cerrar, tx.ok, false, cuerpo, nil, func() {
		d.Cerrar()
		espera := local.Espera(w, tx.slack, tx.conectando)
		go func() {
			ctx, cancelar := context.WithTimeout(context.Background(), time.Minute)
			defer cancelar()
			defer l.Red.Permitir(cuentas.DestinosSlack(tx.slack)...)()
			c, p, err := reg.AltaSlack(ctx, token.Text, app.Text)
			fyne.Do(func() {
				espera.Cerrar()
				if err != nil {
					l.Fallo(w, err)
					return
				}
				agg.Poner(c.ID, p)
				listo()
			})
		}()
	})
	d.Mostrar(w.Canvas())
}

// conectarOAuth hace el alta (o el volver a entrar) de una cuenta de proveedor en el
// navegador del sistema: el portero deja ir a su servidor de tokens y a su API solo
// mientras dura, y al volver la cuenta entra en la sala.
func conectarOAuth(w fyne.Window, l *local.App, agg *chat.Agregador, tx textos, titulo, proveedor string, listo func(),
	alta func(ctx context.Context, abrir func(string) error) (chat.Cuenta, chat.Proveedor, error),
) {
	espera := local.Espera(w, titulo, tx.navegador)
	go func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancelar()
		defer l.Red.Permitir(chat.DestinosAlta(proveedor, titulo)...)()
		c, p, err := alta(ctx, func(destino string) error { return api.AbrirFuera(l.A, destino) })
		fyne.Do(func() {
			espera.Cerrar()
			if err != nil {
				l.Fallo(w, err)
				return
			}
			agg.Poner(c.ID, p)
			listo()
		})
	}()
}

// campo es un rótulo con su caja debajo (y su pista, partida en líneas): un formulario
// de Fyne pone la caja al lado y, en un diálogo estrecho, la corta.
func campo(rotulo string, caja fyne.CanvasObject, pista string) fyne.CanvasObject {
	c := container.NewVBox(widget.NewLabelWithStyle(rotulo, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), caja)
	if pista != "" {
		c.Add(ayuda(pista))
	}
	return c
}

// casilla es una casilla con su texto al lado que se parte en líneas (la de Fyne no lo
// parte y ensancharía el diálogo).
func casilla(texto string) (*widget.Check, fyne.CanvasObject) {
	c := widget.NewCheck("", nil)
	t := widget.NewLabel(texto)
	t.Wrapping = fyne.TextWrapWord
	return c, container.NewBorder(nil, nil, c, nil, t)
}

// altaTeams es «Añadir Microsoft Teams»: los chats y, si se marcan, los canales de los
// equipos y los adjuntos (cada uno, un permiso más); el cliente OAuth propio es para las
// organizaciones que no admiten apps de terceros.
func altaTeams(w fyne.Window, l *local.App, reg *chat.Registro, agg *chat.Agregador, tx textos, listo func()) {
	canales, filaCanales := casilla(tx.teamsCanales)
	ficheros, filaFicheros := casilla(tx.teamsFicheros)
	cliente := widget.NewEntry()
	cuerpo := container.NewVBox(ayuda(tx.teamsAyuda), filaCanales, filaFicheros, campo(tx.cliente, cliente, tx.clienteAyuda))
	var d *ui.Dialogo
	d = ui.Confirmacion(tx.teams, tx.cerrar, tx.ok, false, cuerpo, nil, func() {
		d.Cerrar()
		a := chat.AltaOAuth{Cliente: cliente.Text, Canales: canales.Checked, Ficheros: ficheros.Checked}
		conectarOAuth(w, l, agg, tx, tx.teams, chat.ProveedorTeams, listo, func(ctx context.Context, abrir func(string) error) (chat.Cuenta, chat.Proveedor, error) {
			return reg.AltaTeams(ctx, a, abrir)
		})
	})
	d.Mostrar(w.Canvas())
}

// altaGoogle es «Añadir Google Chat y Meet»: con el cliente de la compilación se escribe y
// se crean reuniones; con el propio del Workspace, además se lee (D-22). Abrir directos
// nuevos es un permiso más, que se pide si se marca.
func altaGoogle(w fyne.Window, l *local.App, reg *chat.Registro, agg *chat.Agregador, tx textos, listo func()) {
	cliente, secreto := widget.NewEntry(), widget.NewPasswordEntry()
	cliente.SetPlaceHolder("…apps.googleusercontent.com")
	directos, filaDirectos := casilla(tx.googleDirectos)
	cuerpo := container.NewVBox(ayuda(tx.googleAyuda), filaDirectos, campo(tx.cliente, cliente, ""), campo(tx.secreto, secreto, ""))
	var d *ui.Dialogo
	d = ui.Confirmacion(tx.google, tx.cerrar, tx.ok, false, cuerpo, nil, func() {
		d.Cerrar()
		a := chat.AltaOAuth{Cliente: cliente.Text, Secreto: secreto.Text, Directos: directos.Checked}
		conectarOAuth(w, l, agg, tx, tx.google, chat.ProveedorGoogle, listo, func(ctx context.Context, abrir func(string) error) (chat.Cuenta, chat.Proveedor, error) {
			return reg.AltaGoogle(ctx, a, abrir)
		})
	})
	d.Mostrar(w.Canvas())
}

// reconectarCuenta vuelve a entrar en una cuenta de Teams o de Google (desde «Cuentas de
// mensajería» o desde Ajustes → Cuentas); en las demás abre las cuentas.
func reconectarCuenta(w fyne.Window, l *local.App, reg *chat.Registro, agg *chat.Agregador, tx textos, id string, listo func()) {
	for _, c := range reg.Cuentas() {
		if c.ID != id {
			continue
		}
		if !chat.EsOAuth(c) {
			mostrarCuentas(w, l, reg, agg, tx, listo)
			return
		}
		conectarOAuth(w, l, agg, tx, tx.volver, c.Proveedor, listo, func(ctx context.Context, abrir func(string) error) (chat.Cuenta, chat.Proveedor, error) {
			return reg.Reconectar(ctx, id, abrir)
		})
		return
	}
}
