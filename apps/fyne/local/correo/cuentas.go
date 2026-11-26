// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package correo es la pantalla de cuentas de Sytue Mail en Sytue Office: la
// bienvenida sin cuentas, el alta de una cuenta IMAP/SMTP (con los servidores de
// Gmail, Outlook, iCloud, Yahoo y Sytue Mail ya puestos) y la lista para quitarlas o
// volver a entrar (§5.10, M-12). Las contraseñas van al llavero (`l.Secretos`).
package correo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	almacen "git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	cuentasLocal "git.sytue.eu/sytue/one/apps/nucleo/local/cuentas"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Gestor son las cuentas de correo de la app.
type Gestor struct {
	L *local.App
	B *almacen.Buzon
	W fyne.Window
	// AlCambiar avisa de que se añadió o se quitó una cuenta.
	AlCambiar func()
}

// t es el texto propio de clave en el idioma de la app: español e inglés de `textos`, y
// los otros veinticuatro de `traducciones` (textos_idiomas.go).
func (g *Gestor) t(clave string) string {
	return local.Propios{Base: textos, Traducciones: traducciones}.En(g.L.Idioma(), clave)
}

// nombreDe es el nombre de un proveedor en pantalla: el de Microsoft lleva una conjunción
// y sale de `textos`; los demás son nombres de producto y van tal cual.
func (g *Gestor) nombreDe(p almacen.Preajuste) string {
	if _, ok := textos["preajuste."+p.Proveedor]; ok {
		return g.t("preajuste." + p.Proveedor)
	}
	return p.Nombre
}

func ayuda(texto string) *widget.Label {
	l := widget.NewLabel(texto)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	return l
}

// Bienvenida es lo que se ve sin ninguna cuenta: qué es esto y el botón de añadir.
func (g *Gestor) Bienvenida() fyne.CanvasObject {
	titulo := widget.NewLabelWithStyle(g.t("bienvenida.titulo"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	titulo.SizeName = theme.SizeNameSubHeadingText
	texto := widget.NewLabel(g.t("bienvenida.texto"))
	texto.Wrapping, texto.Alignment = fyne.TextWrapWord, fyne.TextAlignCenter
	anadir := widget.NewButtonWithIcon(g.t("anadir"), theme.ContentAddIcon(), func() { g.Anadir("") })
	anadir.Importance = widget.HighImportance
	pres := almacen.Preajustes()
	botones := make([]fyne.CanvasObject, 0, len(pres))
	for _, p := range pres {
		botones = append(botones, widget.NewButton(g.nombreDe(p), func() { g.Anadir(p.Proveedor) }))
	}
	icono := widget.NewIcon(theme.MailComposeIcon())
	caja := container.NewVBox(
		container.NewCenter(container.NewGridWrap(fyne.NewSize(64, 64), icono)),
		titulo, texto,
		container.NewCenter(anadir),
		container.NewCenter(container.NewGridWithColumns(3, botones...)),
		ayuda(g.t("bienvenida.privado")),
	)
	return container.NewCenter(container.NewGridWrap(fyne.NewSize(min(560, max(g.W.Canvas().Size().Width-32, 280)), 420), caja))
}

// Gestionar abre la lista de cuentas: estado, volver a entrar, quitar y añadir.
func (g *Gestor) Gestionar() {
	var d *ui.Dialogo
	lista := container.NewVBox()
	for _, c := range g.B.Cuentas() {
		estado := g.t("estado.ok")
		if !c.Sincronizada.IsZero() {
			estado += " · " + c.Sincronizada.Local().Format("02/01 15:04")
		}
		if c.Estado == almacen.EstadoReconectar {
			estado = g.t("estado.reconectar")
		} else if c.Error != "" {
			estado = g.t("estado.sinred")
		}
		nombre := widget.NewLabelWithStyle(c.Titulo(), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		quitar := widget.NewButtonWithIcon(g.t("quitar"), theme.DeleteIcon(), func() {
			local.Confirmar(g.W, g.t("quitar.titulo"), g.t("quitar.texto"), g.t("cancelar"), g.t("quitar"), true, func() {
				if err := g.B.Quitar(c.ID); err != nil {
					g.L.Fallo(g.W, err)
				}
				d.Cerrar()
				g.cambio()
			})
		})
		fila := container.NewBorder(nil, nil, nil, container.NewHBox(
			widget.NewButtonWithIcon(g.t("reconectar"), theme.ViewRefreshIcon(), func() { d.Cerrar(); g.editar(c) }), quitar),
			container.NewVBox(nombre, ayuda(estado)))
		lista.Add(fila)
		lista.Add(widget.NewSeparator())
	}
	if len(g.B.Cuentas()) == 0 {
		lista.Add(ayuda(g.t("ninguna")))
	}
	anadir := widget.NewButtonWithIcon(g.t("anadir"), theme.ContentAddIcon(), func() { d.Cerrar(); g.Anadir("") })
	anadir.Importance = widget.HighImportance
	sincronizar := widget.NewButtonWithIcon(g.t("sincronizar"), theme.ViewRefreshIcon(), func() { g.B.SincronizarYa(); d.Cerrar() })
	pie := container.NewVBox(container.NewHBox(anadir, sincronizar), ayuda(g.t("privado")))
	d = local.Ventana(g.W, g.t("cuentas"), lista, pie)
}

func (g *Gestor) cambio() {
	if g.AlCambiar != nil {
		g.AlCambiar()
	}
}

// Anadir abre el alta de una cuenta; proveedor vacío lo deduce de la dirección.
func (g *Gestor) Anadir(proveedor string) {
	g.formulario(almacen.Cuenta{Proveedor: proveedor}, false)
}

// editar vuelve a pedir la contraseña (o a entrar en Microsoft) de una cuenta.
func (g *Gestor) editar(c almacen.Cuenta) { g.formulario(c, true) }

// Reconectar abre el formulario de la cuenta id para volver a entrar (Ajustes → Cuentas).
func (g *Gestor) Reconectar(id string) {
	for _, c := range g.B.Cuentas() {
		if c.ID == id {
			g.editar(c)
			return
		}
	}
}

// Quitar pregunta y quita la cuenta id (Ajustes → Cuentas, o el enlace
// `sytueoffice-mail:/cuentas?accion=quitar&id=…`, que lo puede escribir cualquiera: por
// eso se pregunta siempre).
func (g *Gestor) Quitar(id string) {
	for _, c := range g.B.Cuentas() {
		if c.ID != id {
			continue
		}
		local.Confirmar(g.W, g.t("quitar.titulo"), c.Titulo()+"\n\n"+g.t("quitar.texto"), g.t("cancelar"), g.t("quitar"), true, func() {
			if err := g.B.Quitar(c.ID); err != nil {
				g.L.Fallo(g.W, err)
			}
			g.cambio()
		})
		return
	}
}

// Registrar pone este gestor en Ajustes → Cuentas de la cáscara y atiende los enlaces de
// cuentas que mandan las demás apps.
func (g *Gestor) Registrar() {
	g.L.GestionarCuentas(cuentasLocal.OrigenCorreo, local.GestorCuentas{
		Anadir: func() { g.Anadir("") }, Reconectar: g.Reconectar, Quitar: g.Quitar,
	})
}

// AtenderEnlace hace lo que pide un enlace de cuentas; false si no es uno.
func (g *Gestor) AtenderEnlace(u *url.URL) bool {
	accion, id, ok := local.LeerEnlaceCuentas(u)
	if !ok {
		return false
	}
	switch accion {
	case local.AccionAnadir:
		g.Anadir("")
	case local.AccionReconectar:
		g.Reconectar(id)
	case local.AccionQuitar:
		g.Quitar(id)
	}
	return true
}

var seguridades = []string{almacen.SeguridadTLS, almacen.SeguridadSTARTTLS, almacen.SeguridadNinguna}

// selectorSeguridad es la lista de seguridad de un servidor con rótulos que se leen
// («SSL/TLS», «STARTTLS» y «Sin cifrar» en el idioma de la app) y los valores de la cuenta
// (`almacen.Seguridad*`) por debajo.
type selectorSeguridad struct {
	*widget.Select
	rotulos []string
}

func (g *Gestor) nuevaSeguridad() *selectorSeguridad {
	rotulos := []string{"SSL/TLS", "STARTTLS", g.t("seguridad.ninguna")}
	return &selectorSeguridad{Select: widget.NewSelect(rotulos, nil), rotulos: rotulos}
}

// Valor es el de la cuenta (`almacen.Seguridad*`) de lo elegido; "" si no hay nada.
func (s *selectorSeguridad) Valor() string {
	if i := s.SelectedIndex(); i >= 0 && i < len(seguridades) {
		return seguridades[i]
	}
	return ""
}

// Poner elige el rótulo de un valor de la cuenta.
func (s *selectorSeguridad) Poner(v string) {
	for i, x := range seguridades {
		if x == v {
			s.SetSelectedIndex(i)
			return
		}
	}
}

// formulario es el alta (o la reconexión, con existente) de una cuenta: proveedor,
// dirección y contraseña, y los servidores en «avanzado».
//
//nolint:gocognit,gocyclo // el formulario se monta de una vez: cada campo se cablea con los demás en el mismo sitio
func (g *Gestor) formulario(c almacen.Cuenta, existente bool) {
	pres := almacen.Preajustes()
	nombres := make([]string, len(pres))
	for i, p := range pres {
		nombres[i] = g.nombreDe(p)
	}
	direccion, nombre, contrasena := widget.NewEntry(), widget.NewEntry(), widget.NewPasswordEntry()
	direccion.SetPlaceHolder("ana@example.org")
	direccion.SetText(c.Direccion)
	nombre.SetText(c.Nombre)
	nombre.SetPlaceHolder(g.t("nombre.ayuda"))
	imapHost, imapPuerto, smtpHost, smtpPuerto, usuarioE := widget.NewEntry(), widget.NewEntry(), widget.NewEntry(), widget.NewEntry(), widget.NewEntry()
	imapSeg, smtpSeg := g.nuevaSeguridad(), g.nuevaSeguridad()
	// El servidor de la respuesta automática (ManageSieve, M-10): opcional. En blanco, el
	// de IMAP en el 4190 si la cuenta es de un IMAP cualquiera; los proveedores conocidos
	// no lo tienen. Va con STARTTLS (sin cifrar solo en este equipo): no hay que elegir.
	sieveHost, sievePuerto := widget.NewEntry(), widget.NewEntry()
	sieveHost.SetPlaceHolder(g.t("sieve.ayuda"))
	sievePuerto.SetPlaceHolder(strconv.Itoa(almacen.PuertoSieve))
	usuarioE.SetPlaceHolder(g.t("usuario.ayuda"))
	cliente := widget.NewEntry()
	cliente.SetPlaceHolder(g.t("cliente.ayuda"))
	nota := ayuda("")
	sinCifrar := ayuda(g.t("sinCifrar"))
	sinCifrar.Hide()
	enlace := widget.NewHyperlink("", nil)
	enlace.Hide()
	proveedor := widget.NewSelect(nombres, nil)
	elegido := false // la persona eligió proveedor: la dirección ya no lo cambia
	var actual almacen.Preajuste

	ponerServidores := func(p almacen.Preajuste) {
		actual = p
		imapHost.SetText(p.IMAP.Host)
		smtpHost.SetText(p.SMTP.Host)
		if p.IMAP.Puerto > 0 {
			imapPuerto.SetText(strconv.Itoa(p.IMAP.Puerto))
			smtpPuerto.SetText(strconv.Itoa(p.SMTP.Puerto))
		}
		imapSeg.Poner(cmpOr(p.IMAP.Seguridad, almacen.SeguridadTLS))
		smtpSeg.Poner(cmpOr(p.SMTP.Seguridad, almacen.SeguridadSTARTTLS))
		oauth := p.Acceso == almacen.AccesoMicrosoft
		if oauth {
			contrasena.Disable()
			nota.SetText(g.t("microsoft.nota"))
		} else {
			contrasena.Enable()
			nota.SetText(g.t("contrasena.nota"))
		}
		if oauth && nube.ClienteMicrosoft == "" {
			cliente.Show()
		} else {
			cliente.Hide()
		}
		enlace.Hide()
		if p.ContrasenaApp != "" {
			if u, err := url.Parse(p.ContrasenaApp); err == nil {
				enlace.SetText(g.t("crearContrasena"))
				enlace.SetURL(u)
				enlace.Show()
			}
		}
	}
	elegir := func(prov string) {
		for i, p := range pres {
			if p.Proveedor == prov {
				proveedor.SetSelectedIndex(i)
				return
			}
		}
	}
	proveedor.OnChanged = func(string) {
		i := proveedor.SelectedIndex()
		if i < 0 {
			return
		}
		p := pres[i]
		if p.Proveedor == almacen.ProveedorIMAP {
			p = almacen.Detectar(direccion.Text)
			p.Proveedor = almacen.ProveedorIMAP
		}
		ponerServidores(p)
	}
	direccion.OnChanged = func(s string) {
		if !elegido && !existente {
			elegir(almacen.Detectar(s).Proveedor)
			if actual.Proveedor == almacen.ProveedorIMAP {
				ponerServidores(almacen.Detectar(s))
			}
		}
	}
	switch {
	case existente:
		direccion.Disable()
		p, _ := almacen.PreajusteDe(c.Proveedor)
		p.Proveedor = c.Proveedor
		elegir(c.Proveedor)
		p.IMAP, p.SMTP, p.Acceso = c.IMAP, c.SMTP, c.Acceso
		ponerServidores(p)
		usuarioE.SetText(c.IMAP.Usuario)
		cliente.SetText(c.ClienteOAuth)
		sieveHost.SetText(c.Sieve.Host)
		if c.Sieve.Puerto > 0 {
			sievePuerto.SetText(strconv.Itoa(c.Sieve.Puerto))
		}
	case c.Proveedor != "":
		elegir(c.Proveedor)
		elegido = true
	default:
		elegir(almacen.ProveedorIMAP)
	}
	proveedor.OnChanged = func(prev func(string)) func(string) {
		return func(s string) { elegido = true; prev(s) }
	}(proveedor.OnChanged)

	// «Sin cifrar» solo vale para un servidor de este equipo: se dice al elegirla.
	verSinCifrar := func(string) {
		if imapSeg.Valor() == almacen.SeguridadNinguna || smtpSeg.Valor() == almacen.SeguridadNinguna {
			sinCifrar.Show()
		} else {
			sinCifrar.Hide()
		}
	}
	imapSeg.OnChanged, smtpSeg.OnChanged = verSinCifrar, verSinCifrar
	// «Buscar los servidores» del dominio (autoconfig y SRV), solo al pulsarlo.
	hallado := ayuda("")
	var buscar *widget.Button
	buscar = widget.NewButton(g.t("buscar"), func() {
		dir := strings.TrimSpace(direccion.Text)
		if g.L.Red.SinConexion() {
			hallado.SetText(g.t("buscar.sinred"))
			return
		}
		_, dominio, _ := strings.Cut(dir, "@")
		buscar.Disable()
		hallado.SetText(g.t("buscar.buscando"))
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), 30*time.Second)
			defer parar()
			// El fichero de configuración, con el permiso puntual; los SRV, con el DNS del
			// portero (con «Sin conexión», ninguno de los dos sale).
			r, err := almacen.Descubridor{HTTP: g.L.Red.Puntual(red.Descubrimiento, dominio), Resolver: g.L.Red.Resolvedor(red.Descubrimiento, dominio)}.Descubrir(ctx, dir)
			fyne.Do(func() {
				buscar.Enable()
				if err != nil {
					hallado.SetText(g.t("buscar.nada"))
					return
				}
				imapHost.SetText(r.IMAP.Host)
				imapPuerto.SetText(strconv.Itoa(r.IMAP.Puerto))
				imapSeg.Poner(r.IMAP.Seguridad)
				smtpHost.SetText(r.SMTP.Host)
				smtpPuerto.SetText(strconv.Itoa(r.SMTP.Puerto))
				smtpSeg.Poner(r.SMTP.Seguridad)
				hallado.SetText(g.t("buscar.hallado." + r.Origen))
			})
		}()
	})
	imapHost.SetPlaceHolder("imap.example.org")
	smtpHost.SetPlaceHolder("smtp.example.org")
	avanzado := widget.NewAccordion(widget.NewAccordionItem(g.t("servidores"), container.NewVBox(container.NewHBox(buscar), hallado, widget.NewForm(
		widget.NewFormItem("IMAP", filaServidor(imapHost, imapPuerto, imapSeg.Select)),
		widget.NewFormItem("SMTP", filaServidor(smtpHost, smtpPuerto, smtpSeg.Select)),
		widget.NewFormItem(g.t("usuario"), usuarioE),
	), sinCifrar,
		// Debajo y con su rótulo encima: en el formulario, un rótulo largo ensancharía la
		// columna de los rótulos y estrecharía los servidores de IMAP y SMTP.
		widget.NewLabelWithStyle(g.t("sieve"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, sized(sievePuerto, AnchoPuerto), sieveHost))))
	form := widget.NewForm(
		widget.NewFormItem(g.t("proveedor"), proveedor),
		widget.NewFormItem(g.t("direccion"), direccion),
		widget.NewFormItem(g.t("nombre"), nombre),
		widget.NewFormItem(g.t("contrasena"), contrasena),
		widget.NewFormItem("", container.NewVBox(nota, enlace)),
		widget.NewFormItem("", cliente),
	)
	cuerpo := container.NewVBox(form, avanzado)
	titulo := g.t("anadir")
	if existente {
		titulo = g.t("reconectar")
	}
	var d *ui.Dialogo
	d = ui.Confirmacion(titulo, g.t("cancelar"), g.t("conectar"), false, cuerpo, nil, func() {
		d.Cerrar()
		puerto := func(e *widget.Entry) int { n, _ := strconv.Atoi(strings.TrimSpace(e.Text)); return n }
		nueva := c
		nueva.Proveedor = actual.Proveedor
		nueva.Direccion, nueva.Nombre = strings.TrimSpace(direccion.Text), strings.TrimSpace(nombre.Text)
		nueva.IMAP = almacen.Servidor{Host: strings.TrimSpace(imapHost.Text), Puerto: puerto(imapPuerto), Seguridad: imapSeg.Valor(), Usuario: strings.TrimSpace(usuarioE.Text)}
		nueva.SMTP = almacen.Servidor{Host: strings.TrimSpace(smtpHost.Text), Puerto: puerto(smtpPuerto), Seguridad: smtpSeg.Valor(), Usuario: strings.TrimSpace(usuarioE.Text)}
		nueva.Sieve = almacen.Servidor{Host: strings.TrimSpace(sieveHost.Text), Puerto: puerto(sievePuerto)}
		nueva.Acceso = cmpOr(actual.Acceso, almacen.AccesoContrasena)
		nueva.GuardaEnviados = actual.GuardaEnviados
		if nueva.Acceso == almacen.AccesoMicrosoft {
			nueva.ClienteOAuth = cmpOr(strings.TrimSpace(cliente.Text), nube.ClienteMicrosoft)
		}
		g.conectar(nueva, contrasena.Text)
	})
	d.Mostrar(g.W.Canvas())
}

// AnchoPuerto es el del campo del puerto: cinco cifras holgadas.
const AnchoPuerto float32 = 80

// filaServidor es el servidor con todo el ancho de la fila y, debajo, el puerto y la
// seguridad. Los tres en una fila dejaban el servidor en unos 60 píxeles en cualquier
// idioma: el desplegable de la seguridad se llevaba el resto.
func filaServidor(host, puerto *widget.Entry, seguridad *widget.Select) fyne.CanvasObject {
	return container.NewVBox(host, container.NewBorder(nil, nil, sized(puerto, AnchoPuerto), nil, seguridad))
}

func sized(o fyne.CanvasObject, ancho float32) fyne.CanvasObject {
	return container.New(layout.NewGridWrapLayout(fyne.NewSize(ancho, o.MinSize().Height)), o)
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// conectar prueba la cuenta (IMAP y SMTP) y, si entra, la da de alta. Una que ya estaba
// (volver a entrar) se actualiza: Anadir la reconoce por su ID.
func (g *Gestor) conectar(c almacen.Cuenta, contrasena string) {
	if c.Acceso != almacen.AccesoMicrosoft && contrasena == "" {
		g.L.Fallo(g.W, errors.New(g.t("falta.contrasena")))
		return
	}
	if c.Acceso == almacen.AccesoMicrosoft && c.ClienteOAuth == "" {
		g.L.Fallo(g.W, errors.New(g.t("falta.cliente")))
		return
	}
	aviso := local.Espera(g.W, g.t("conectando"), g.t("conectando.texto"))
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), 10*time.Minute)
		defer parar()
		err := g.probarYAnadir(ctx, c, contrasena)
		fyne.Do(func() {
			aviso.Cerrar()
			if err != nil {
				g.L.Fallo(g.W, err)
				return
			}
			g.cambio()
		})
	}()
}

// probarYAnadir entra en la cuenta (con Microsoft, pidiendo permiso en el navegador),
// prueba la entrada y la salida del correo y, si van, la da de alta. Fuera del hilo de
// la interfaz.
func (g *Gestor) probarYAnadir(ctx context.Context, c almacen.Cuenta, contrasena string) error {
	// El portero (§5.8) abre los servidores de esta cuenta solo mientras se prueba:
	// guardada, ya los da su fichero (local/cuentas).
	if raw, err := json.Marshal(c); err == nil {
		if mc, err := cuentasLocal.DeCorreo(raw); err == nil {
			defer g.L.Red.Permitir(cuentasLocal.DestinosDe(mc)...)()
		}
	}
	secreto := contrasena
	if c.Acceso == almacen.AccesoMicrosoft {
		defer g.L.Red.Permitir(red.Destino{Proposito: red.OAuth, Host: cuentasLocal.HostOAuthMicrosoft, Motivo: c.Direccion})()
		hc := &http.Client{Transport: g.L.Red.Transporte(red.OAuth), Timeout: time.Minute}
		t, err := almacen.OAuthMicrosoftCorreo(c.ClienteOAuth).Conectar(ctx, hc,
			func(destino string) error { return api.AbrirFuera(g.L.A, destino) })
		if err != nil {
			return err
		}
		if t.Refresco == "" {
			return errors.New(g.t("falta.refresco"))
		}
		secreto = t.Refresco
	}
	if err := g.B.ConSecreto(&c, secreto); err != nil {
		return err
	}
	if err := g.B.Probar(ctx, c); err != nil {
		return err
	}
	_, err := g.B.Anadir(c, "")
	return err
}

// Los textos propios de Mail en la edición (§5.14): español e inglés aquí, los otros
// veinticuatro idiomas en textos_idiomas.go; TestLosTextosDeMailEstanEnTodosLosIdiomas
// exige las mismas claves en todos.
var textos = map[string][2]string{ // clave: {es, en}
	"bienvenida.titulo":         {"Tu correo, en este equipo", "Your email, on this device"},
	"bienvenida.texto":          {"Conecta tus cuentas de Outlook, Gmail, iCloud, Yahoo, Sytue Mail o cualquier IMAP. El correo se descarga aquí y puedes leerlo y escribir sin conexión: se envía cuando vuelva la red.", "Connect your Outlook, Gmail, iCloud, Yahoo, Sytue Mail or any IMAP account. Mail is downloaded here so you can read and write offline: it is sent when you are back online."},
	"bienvenida.privado":        {"Sin cuenta de Sytue y sin servidores de Sytue: la app solo habla con los servidores de las cuentas que añadas.", "No Sytue account and no Sytue servers: the app only talks to the servers of the accounts you add."},
	"privado":                   {"Las contraseñas se guardan en el llavero del sistema; el correo, en claro en tu carpeta de Sytue Office.", "Passwords are kept in the system keychain; mail is stored unencrypted in your Sytue Office folder."},
	"anadir":                    {"Añadir cuenta de correo", "Add mail account"},
	"buscar":                    {"Buscar los servidores", "Find the servers"},
	"buscar.buscando":           {"Preguntando al dominio de la dirección…", "Asking the address’s domain…"},
	"buscar.nada":               {"El dominio no publica sus servidores: escríbelos a mano (te los da tu proveedor).", "The domain does not publish its servers: type them in (your provider gives them to you)."},
	"buscar.sinred":             {"«Sin conexión» está encendido en Ajustes → Red.", "“Offline” is on in Settings → Network."},
	"buscar.hallado.autoconfig": {"Servidores tomados de la configuración que publica el dominio. Revísalos antes de conectar.", "Servers taken from the configuration the domain publishes. Check them before connecting."},
	"buscar.hallado.srv":        {"Servidores tomados de los registros DNS del dominio. Revísalos antes de conectar.", "Servers taken from the domain’s DNS records. Check them before connecting."},
	"cuentas":                   {"Cuentas de correo", "Mail accounts"},
	"cerrar":                    {"Cerrar", "Close"},
	"ninguna":                   {"Aún no hay ninguna cuenta.", "No accounts yet."},
	"quitar":                    {"Quitar", "Remove"},
	"quitar.titulo":             {"¿Quitar la cuenta?", "Remove account?"},
	"quitar.texto":              {"Se borra de este equipo el correo descargado de esta cuenta y su contraseña. En el servidor no se toca nada.", "The mail downloaded from this account and its password are removed from this device. Nothing changes on the server."},
	"reconectar":                {"Volver a entrar", "Sign in again"},
	"sincronizar":               {"Sincronizar ahora", "Sync now"},
	"estado.ok":                 {"Conectada", "Connected"},
	"estado.reconectar":         {"El servidor rechazó la contraseña: vuelve a entrar", "The server rejected the password: sign in again"},
	"estado.sinred":             {"Sin conexión con el servidor; se reintentará", "Cannot reach the server; will retry"},
	"proveedor":                 {"Proveedor", "Provider"},
	"direccion":                 {"Correo", "Email"},
	"nombre":                    {"Tu nombre", "Your name"},
	"nombre.ayuda":              {"Como lo verán los destinatarios", "As recipients will see it"},
	"contrasena":                {"Contraseña", "Password"},
	"contrasena.nota":           {"Con Gmail, iCloud, Yahoo y Sytue Mail usa una contraseña de aplicación, no la de tu cuenta.", "With Gmail, iCloud, Yahoo and Sytue Mail use an app password, not your account password."},
	"microsoft.nota":            {"Outlook y Microsoft 365 se conectan en el navegador: no hace falta contraseña aquí.", "Outlook and Microsoft 365 sign in through the browser: no password needed here."},
	"crearContrasena":           {"Crear una contraseña de aplicación", "Create an app password"},
	"cliente.ayuda":             {"Identificador de cliente de Microsoft (tu registro de app)", "Microsoft client ID (your app registration)"},
	"servidores":                {"Servidores (avanzado)", "Servers (advanced)"},
	"usuario":                   {"Usuario", "Username"},
	"usuario.ayuda":             {"Si no es la dirección de correo", "If it is not the email address"},
	"conectar":                  {"Conectar", "Connect"},
	"cancelar":                  {"Cancelar", "Cancel"},
	"conectando":                {"Conectando…", "Connecting…"},
	"conectando.texto":          {"Comprobando la entrada y la salida del correo.", "Checking incoming and outgoing mail."},
	"falta.contrasena":          {"Escribe la contraseña de la cuenta.", "Enter the account password."},
	"falta.cliente":             {"Esta compilación no trae identificador de cliente de Microsoft: escribe el tuyo.", "This build has no Microsoft client ID: enter your own."},
	"llegadas":                  {"{n} correos nuevos", "{n} new emails"},
	"sinAsunto":                 {"(sin asunto)", "(no subject)"},
	"sinCifrar":                 {"«Sin cifrar» solo funciona con un servidor de este equipo, como localhost o el puente de Proton Mail.", "“Unencrypted” only works with a server on this device, such as localhost or the Proton Mail bridge."},
	"seguridad.ninguna":         {"Sin cifrar", "Unencrypted"},
	"falta.refresco":            {"Microsoft no dio acceso sin conexión; vuelve a intentarlo.", "Microsoft did not grant offline access; try again."},
	"preajuste.microsoft":       {"Outlook y Microsoft 365", "Outlook and Microsoft 365"},
	"sieve":                     {"Respuesta automática (Sieve)", "Auto-reply (Sieve)"},
	"sieve.ayuda":               {"Como el de IMAP", "Same as IMAP"},
}
