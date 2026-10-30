// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/portal/src/pages/dispositivos.astro

package hub

import (
	"fmt"
	"runtime"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Conecta tus dispositivos»: `ConnectDevices.tsx` dentro de `dispositivos.astro`. Un
// asistente de tres pasos —qué usas, cómo se hace, cómo saber que ha funcionado— que
// lleva la cuenta de Sytue al correo, el calendario y los contactos de otro programa.
// Ningún valor va escrito aquí: salen de Sytue ID (`api.CoordenadasDe`), y cada uno se
// copia con un botón.
//
// Medido en producción a 1280×800 (`medidas/portal/dispositivos.ancho.json`), con la
// topbar descontada (−56): el progreso en y = 118,56 (24,5 de alto), el encabezado del
// paso en y = 164,06 (19,25/26,25 · 600), su frase en y = 195,56, las siete opciones
// desde y = 235,81 en dos columnas de 330,75 con `gap-3` (99,78 de alto; la última,
// 115,42) y «Continuar» en (984, 718,08), 83 × 31,5. A 390, una columna de 362.
//
// ponytail: al cambiar de paso la web lleva el foco al encabezado para que el lector de
// pantalla lo anuncie. Fyne 2.8 no enfoca texto; el foco se queda en el botón pulsado,
// que sigue en su sitio.

// Plataforma es lo que la persona dice usar en el primer paso.
type Plataforma string

// Las siete, en el orden de la web.
const (
	IPhone      Plataforma = "iphone"
	Mac         Plataforma = "mac"
	Outlook     Plataforma = "outlook"
	Thunderbird Plataforma = "thunderbird"
	Android     Plataforma = "android"
	UnidadDrive Plataforma = "drive"
	APIs        Plataforma = "apis"
)

// Plataformas son las siete opciones del primer paso.
var Plataformas = []Plataforma{IPhone, Mac, Outlook, Thunderbird, Android, UnidadDrive, APIs}

// Medidas del asistente (`Wizard.tsx` y `ConnectDevices.tsx`).
const (
	// AnchoAsistente es el `max-w-3xl` del contenido de cada paso.
	AnchoAsistente float32 = 48 * tema.Rem
	// Los huecos de las instrucciones: `gap-4` entre el flujo y ellas, `gap-5` entre
	// sus bloques, `gap-3` entre pasos y `gap-2` entre filas de copiar.
	HuecoInstrucciones float32 = 4 * 0.25 * tema.Rem
	HuecoBloques       float32 = 5 * 0.25 * tema.Rem
	HuecoPasosNum      float32 = 3 * 0.25 * tema.Rem
	HuecoValores       float32 = 2 * 0.25 * tema.Rem
	// MargenListaPasos es el `mb-3` de la lista de pasos de Android, que se suma al
	// `gap-4` de su sección.
	MargenListaPasos float32 = 3 * 0.25 * tema.Rem
)

// ClientesExternos porta el asistente web en `/dispositivos/clientes`.
type ClientesExternos struct {
	pagina

	T        *i18n.Translator
	revision uint64
	// Estado es el de las coordenadas: sin ellas no hay instrucciones honestas que dar.
	Estado      Estado
	Coordenadas api.Coordenadas
	// Correo es la dirección de la sesión, que es el usuario en todos los programas.
	Correo string
	// Apex es el dominio de la suite, del que salen las direcciones de compatibilidad.
	Apex string

	Paso       int
	Plataforma Plataforma

	// AlCopiar recibe el valor y la clave del aviso que lo confirma.
	AlCopiar                   func(valor, clave string)
	AlAbrirURL                 func(string)
	AlReintentar               func()
	AlCambiarPaso              func()
	AlEnlacePerfilApple        func(Plataforma)
	PerfilCreando, PerfilFallo bool
	Montando, FalloMontaje     bool
	// AlMontar conecta Drive como disco en ESTE equipo con la dirección WebDAV. Solo en
	// escritorio, que es donde el explorador de ficheros lo sabe montar; nil lo esconde.
	AlMontar func(direccion string)
	// AlConfigurarApple instala en ESTE equipo de Apple (`EsteApple`: Mac o IPhone) el
	// perfil con su propia contraseña de aplicación: correo, calendario y contactos sin
	// escribir nada. Fuera de ese equipo, AlEnlacePerfilApple pide el enlace autenticado.
	AlConfigurarApple func()
	EsteApple         Plataforma
	// AlConfigurarCliente es «Configurar Thunderbird», «Configurar Outlook» y «Configurar
	// en Android»: crea una contraseña de aplicación para ese cliente, la copia y abre el
	// programa si está en este equipo. Lo que deja lo cuenta `Cliente`. nil lo esconde.
	AlConfigurarCliente func(Plataforma)
	Cliente             Cliente
	// AlAbrirDAVx5 abre el alta de DAVx⁵ con la base de CalDAV o, con `contactos`, la de
	// CardDAV. Solo en Android, que es donde está DAVx⁵; nil esconde los botones.
	AlAbrirDAVx5 func(base string, contactos bool)
}

// Cliente es lo que ha dejado el botón de un clic. La contraseña se enseña UNA vez:
// salir del paso de las instrucciones la olvida (`Ir`).
type Cliente struct {
	Plataforma Plataforma
	Creando    bool
	Fallo      bool
	Clave      string
	// Abierto es que el programa se ha abierto solo; si no, la pantalla dice «ábrelo tú».
	Abierto bool
}

// nombrePrograma es el nombre del programa en «no hemos podido abrir {program}»: marca,
// no se traduce (el título de la opción es «Windows (Outlook)»).
var nombrePrograma = map[Plataforma]string{Thunderbird: "Thunderbird", Outlook: "Outlook"}

// FichaDAVx5 es donde se instala DAVx⁵: F-Droid, gratis y sin cuenta de Google.
const FichaDAVx5 = "https://f-droid.org/packages/at.bitfire.davdroid/"

var _ fyne.Widget = (*ClientesExternos)(nil)

// NuevosClientesExternos crea la pantalla.
func NuevosClientesExternos(t *i18n.Translator) *ClientesExternos {
	d := &ClientesExternos{T: t, Estado: Cargando, Apex: api.ApexProd}
	d.ExtendBaseWidget(d)
	return d
}

// Resize recuerda el ancho: el alto depende de él.
func (d *ClientesExternos) Resize(s fyne.Size) {
	if d.recordar(s) {
		d.BaseWidget.Resize(s)
		d.Refresh()
		return
	}
	d.BaseWidget.Resize(s)
}

// AccessibilityRole: la pantalla agrupa.
func (d *ClientesExternos) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

// Ir cambia de paso. Volver al primero olvida lo elegido, que es lo que hace «Conectar
// otro dispositivo» en la web.
func (d *ClientesExternos) Ir(paso int) {
	if d.AlCambiarPaso != nil {
		d.AlCambiarPaso()
	}
	d.revision++
	d.PerfilCreando, d.PerfilFallo, d.Montando, d.FalloMontaje = false, false, false, false
	d.Paso = max(0, min(paso, 2))
	if d.Paso != 1 {
		d.Cliente = Cliente{}
	}
	d.Refresh()
}

// sinDatos es el estado en el que la web enseña «no podemos enseñarte los datos»: la
// petición a Sytue ID acabó mal. Mientras carga se ve el asistente, porque su primer
// paso no necesita las coordenadas.
func (d *ClientesExternos) sinDatos() bool {
	return d.Estado != Cargando && d.Estado != Listo
}

func (d *ClientesExternos) copiar(clave string) func(string) {
	revision := d.revision
	return func(valor string) {
		if revision == d.revision && d.AlCopiar != nil {
			d.AlCopiar(valor, clave)
		}
	}
}

// CreateRenderer construye la pantalla. Las opciones, el progreso y los botones se crean
// UNA vez: rehacerlos al pulsar le quitaría el foco a quien usa el teclado.
func (d *ClientesExternos) CreateRenderer() fyne.WidgetRenderer {
	t := d.T
	r := &clientesExternosRenderer{d: d, fondo: ui.NuevoFondoSuite(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))}
	for _, p := range Plataformas {
		o := ui.NuevaTarjetaEleccion(t.T("devices.platform."+string(p)+".title", nil),
			t.T("devices.platform."+string(p)+".body", nil), func() {
				if d.AlCambiarPaso != nil {
					d.AlCambiarPaso()
				}
				d.Plataforma, d.Cliente = p, Cliente{}
				d.revision++
				d.PerfilCreando, d.PerfilFallo, d.Montando, d.FalloMontaje = false, false, false, false
				d.Refresh()
			})
		o.Nota = t.T("devices.platform."+string(p)+".note", nil)
		r.opciones = append(r.opciones, o)
	}
	r.pasos = ui.NuevosPasos(t.T("devices.step1.title", nil), t.T("devices.step2.title", nil), t.T("devices.step3.title", nil))
	r.pasos.AlElegir = d.Ir
	r.pasos.RotuloIr = func(n int) string { return t.T("ui.wizard.goToStep", i18n.Vars{"n": n}) }
	r.atras = ui.NuevoBoton(t.T("ui.wizard.back", nil), func() { d.Ir(d.Paso - 1) }).Con(ui.Secundario, ui.TallaMD)
	r.atras.Icono, r.atras.TamIcono = iconos.ChevronIzquierda, 16
	r.siguiente = ui.NuevoBoton("", func() { d.Ir(d.Paso + 1) }).Con(ui.Primario, ui.TallaMD)
	r.rehacer()
	return r
}

// La cache conserva sólo estado estructurado, nunca el secreto de aplicación.
type claveClientesExternos struct {
	revision                            uint64
	paso                                int
	plataforma                          Plataforma
	estado                              Estado
	estrecha                            bool
	variante                            tema.Variante
	marco                               bool
	color                               string
	correo                              string
	coordenadas                         api.Coordenadas
	creando, fallo, tieneClave, abierto bool
	perfilCreando, perfilFallo          bool
	montando, falloMontaje              bool
}

type clientesExternosRenderer struct {
	d *ClientesExternos

	fondo           *ui.FondoSuite
	titulo, entrada *ui.Parrafo
	pasos           *ui.Pasos
	encabezado      *ui.Parrafo
	frase           *ui.Parrafo
	opciones        []*ui.TarjetaEleccion
	cuerpo          bloque
	filete          *canvas.Rectangle
	atras           *ui.Boton
	siguiente       *ui.Boton
	// clave es de qué están hechos los bloques del cuerpo: se rehacen solo si cambia.
	clave claveClientesExternos

	objetos []fyne.CanvasObject
}

func (r *clientesExternosRenderer) rehacer() {
	d, t := r.d, r.d.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p := func(texto string, e tema.Escalon, peso ui.Peso, token string) *ui.Parrafo {
		return ui.NuevoParrafo(texto, e, peso, token, v)
	}
	r.fondo.PonerVariante(v)
	r.titulo = p(t.T("devices.title", nil), ui.EscalonTitulo(2), ui.PesoTitulo(2), "text")
	lead := "devices.lead"
	if d.sinDatos() {
		lead = "devices.leadShort"
	}
	r.entrada = p(t.T(lead, nil), baseRelajada, ui.Normal, "text-muted")

	r.pasos.Actual, r.pasos.Ventana = d.Paso, d.Ancho()
	// El paso terminal no deja volver atrás desde el progreso.
	if d.Paso == 2 {
		r.pasos.AlElegir = nil
	} else {
		r.pasos.AlElegir = d.Ir
	}
	r.pasos.Refresh()
	encabezados := [...][2]string{
		{t.T("devices.step1.heading", nil), t.T("devices.step1.description", nil)},
		{t.T("devices.step2.headingFallback", nil), t.T("devices.step2.description", nil)},
		{t.T("devices.step3.heading", nil), t.T("devices.step3.description", nil)},
	}
	enc := encabezados[d.Paso]
	if d.Paso == 1 && d.Plataforma != "" {
		enc[0] = t.T("devices.step2.heading", i18n.Vars{"platform": t.T("devices.platform."+string(d.Plataforma)+".title", nil)})
	}
	r.encabezado = p(enc[0], ui.EscalonTitulo(3), ui.PesoTitulo(3), "text")
	r.frase = p(enc[1], tema.TextoBase, ui.Normal, "text-muted")

	for i, o := range r.opciones {
		o.Elegida = Plataformas[i] == d.Plataforma
		o.Inerte = d.Estado != Listo
		o.Refresh()
	}
	r.filete = canvas.NewRectangle(tema.Color("border-subtle", v))
	r.siguiente.Texto = t.T("ui.wizard.next", nil)
	if d.Paso == 1 {
		r.siguiente.Texto = t.T("devices.step2.next", nil)
	}
	r.siguiente.Inerte = d.Estado != Listo || d.Paso == 0 && d.Plataforma == ""
	r.siguiente.Refresh()

	clave := claveClientesExternos{
		revision: d.revision, paso: d.Paso, plataforma: d.Plataforma,
		estado: d.Estado, estrecha: d.Ancho() < ui.CorteSM,
		variante: v, marco: tema.EsMarco(), color: tema.ColorElegido(),
		correo: d.Correo, coordenadas: d.Coordenadas,
		creando: d.Cliente.Creando, fallo: d.Cliente.Fallo, tieneClave: d.Cliente.Clave != "", abierto: d.Cliente.Abierto,
		perfilCreando: d.PerfilCreando, perfilFallo: d.PerfilFallo, montando: d.Montando, falloMontaje: d.FalloMontaje,
	}
	if r.cuerpo == nil || clave != r.clave {
		r.clave = clave
		r.cuerpo = r.hacerCuerpo()
	}
	r.recomponer()
}

// hacerCuerpo construye lo que va debajo del encabezado del paso, o el aviso de que no
// hay datos.
func (r *clientesExternosRenderer) hacerCuerpo() bloque {
	d, t := r.d, r.d.T
	if d.sinDatos() {
		vacio := ui.NuevoVacio(t.T("devices.unavailable.title", nil), t.T("devices.unavailable.body", nil))
		if d.AlReintentar != nil {
			vacio.Accion = ui.NuevoBoton(t.T("devices.retry", nil), d.AlReintentar).Con(ui.Secundario, ui.TallaMD)
		}
		return nuevaBSeccion(t.T("devices.unavailable.section", nil), "", vacioB{vacio})
	}
	switch d.Paso {
	case 0:
		return &rejillaOpciones{opciones: r.opciones, ventana: d.Ancho()}
	case 1:
		if d.Plataforma == "" {
			return col(0)
		}
		// El `DataFlow compact` de `@sytue/ui` en los dos sentidos: lo que se sincroniza
		// entra y sale a la vez (`incoming` y `outgoing` a la vez, como AppPasswordsCard).
		flujo := fijo{ui.FlujoDatos(t, "", []string{"sync"}, []string{"sync"}, true, func() {
			if d.AlAbrirURL != nil {
				d.AlAbrirURL(PoliticaDePrivacidad(d.Apex, t.Locale()))
			}
		}, AnchoAsistente)}
		return tope{AnchoAsistente, col(HuecoInstrucciones, flujo, r.instrucciones(d.Plataforma))}
	}
	otra := ui.NuevoBoton(t.T("devices.step3.again", nil), func() {
		d.Plataforma = ""
		d.Ir(0)
	}).Con(ui.Secundario, ui.TallaMD)
	return tope{AnchoAsistente, col(HuecoInstrucciones,
		nuevoResumen(d.Ancho(), r.comprobaciones(d.Plataforma)),
		nuevaAyuda(ui.AyudaInfo, t.T("devices.step3.helpTitle", nil), t.T("devices.step3.helpBody", nil)),
		fijo{otra})}
}

// PoliticaDePrivacidad es el enlace del `DataFlow`: el de `ui`, que lo comparten todas
// las apps que enseñan el flujo de datos.
func PoliticaDePrivacidad(apex string, idioma i18n.Locale) string {
	return ui.PoliticaDePrivacidad(apex, idioma)
}

// fila crea una fila de copiar con su rótulo del catálogo.
func (r *clientesExternosRenderer) fila(rotulo, valor, copiado string) *FilaCopia {
	f := nuevaFilaCopia(r.d.T, r.d.T.T(rotulo, nil), valor, r.d.copiar(copiado))
	f.Ventana = r.d.Ancho()
	return f
}

// paso crea un paso numerado con sus dos claves.
func (r *clientesExternosRenderer) paso(n int, titulo, cuerpo string) *pasoNumerado {
	return nuevoPaso(n, r.d.T.T(titulo, nil), r.d.T.T(cuerpo, nil))
}

// valoresCorreo son los cinco del buzón con la pista de debajo.
func (r *clientesExternosRenderer) valoresCorreo() bloque {
	c, t := r.d.Coordenadas, r.d.T
	return col(HuecoValores,
		r.fila("devices.mail.address", r.d.Correo, "devices.copied.address"),
		r.fila("devices.mail.imapHost", c.IMAP.Host, "devices.copied.imap"),
		// El puerto es un identificador, no una cantidad: sin separador de millares.
		r.fila("devices.mail.imapPort", puerto(c.IMAP.Port), "devices.copied.port"),
		r.fila("devices.mail.smtpHost", c.SMTP.Host, "devices.copied.smtp"),
		r.fila("devices.mail.smtpPort", puerto(c.SMTP.Port), "devices.copied.port"),
		texto(t.T("devices.mail.hint", nil), xsAjustada))
}

// valoresDAV son los tres del calendario y los contactos.
func (r *clientesExternosRenderer) valoresDAV() bloque {
	c := r.d.Coordenadas
	return col(HuecoValores,
		r.fila("devices.dav.caldav", c.CalDAVURL, "devices.copied.url"),
		r.fila("devices.dav.carddav", c.CardDAVURL, "devices.copied.url"),
		r.fila("devices.dav.user", r.d.Correo, "devices.copied.address"))
}

func puerto(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// instrucciones son las de ESA plataforma, y solo las de esa.
func (r *clientesExternosRenderer) instrucciones(p Plataforma) bloque {
	d, t := r.d, r.d.T
	tt := func(k string) string { return t.T(k, nil) }
	manual := func(contenido bloque) *Detalles {
		det := nuevosDetalles(tt("devices.manualValues"), contenido)
		det.AlCambiar = d.Refresh
		return det
	}
	switch p {
	case IPhone, Mac:
		return r.instruccionesApple(p, manual)

	case Outlook, Thunderbird:
		cual := string(p)
		dav := bloque(nuevaAyuda(ui.AyudaNeutra, tt("devices.outlook.davTitle"), tt("devices.outlook.davBody")))
		if p == Thunderbird {
			dav = nuevaBSeccion(tt("devices.thunderbird.davTitle"), tt("devices.thunderbird.davBody"), r.valoresDAV())
		}
		return r.conUnClic(p,
			col(HuecoPasosNum,
				r.paso(1, "devices.desktop.step1.title", "devices.desktop.step1."+cual),
				r.paso(2, "devices.desktop.step2.title", "devices.desktop.step2."+cual),
				r.paso(3, "devices.desktop.step3.title", "devices.desktop.step3.body")),
			col(HuecoValores, r.fila("devices.desktop.emailLabel", d.Correo, "devices.copied.address")),
			nuevaAyuda(ui.AyudaAviso, tt("devices.desktop.notFoundTitle"), tt("devices.desktop.notFoundBody")),
			manual(r.valoresCorreo()),
			dav)

	case Android:
		return r.conUnClic(p,
			nuevaBSeccion(tt("devices.android.mailTitle"), tt("devices.android.mailBody"),
				col(HuecoSeccion+MargenListaPasos,
					col(HuecoPasosNum,
						r.paso(1, "devices.android.mail1.title", "devices.android.mail1.body"),
						r.paso(2, "devices.android.mail2.title", "devices.android.mail2.body")),
					r.valoresCorreo())),
			nuevaBSeccion(tt("devices.android.davTitle"), tt("devices.android.davBody"),
				col(HuecoSeccion+MargenListaPasos,
					col(HuecoPasosNum,
						r.paso(1, "devices.android.dav1.title", "devices.android.dav1.body"),
						r.paso(2, "devices.android.dav2.title", "devices.android.dav2.body"),
						r.paso(3, "devices.android.dav3.title", "devices.android.dav3.body")),
					r.valoresDAV())))

	case UnidadDrive:
		// Cuando este proceso puede abrir el proveedor existente de One por dueño,
		// lo ofrece antes de los pasos manuales. Nunca monta una segunda raíz.
		pasos := col(HuecoBloques,
			col(HuecoPasosNum,
				r.paso(1, "devices.drive.step1.title", "devices.drive.step1.body"),
				r.paso(2, "devices.drive.step2.title", "devices.drive.step2.body"),
				r.paso(3, "devices.drive.step3.title", "devices.drive.step3.body")),
			col(HuecoValores,
				r.fila("devices.drive.urlLabel", d.Coordenadas.WebDAVURL, "devices.copied.drive"),
				r.fila("devices.dav.user", d.Correo, "devices.copied.address")))
		esperar := nuevaAyuda(ui.AyudaInfo, tt("devices.drive.expectTitle"), tt(claveEsperarDrive(runtime.GOOS, d.AlMontar != nil)))
		if d.AlMontar == nil || d.Coordenadas.WebDAVURL == "" {
			return col(HuecoBloques, pasos, esperar)
		}
		// Con el botón, los pasos a mano (mapear una unidad de red) son el plan B y van
		// plegados en «Valores manuales», como los de Apple y Outlook: a la vista, justo
		// debajo del botón, parecían otra forma de hacer lo mismo (visto en Windows).
		boton := NuevaAccion(tt("shell.mountDrive"), tema.TextoMD, func() { d.AlMontar(d.Coordenadas.WebDAVURL) })
		boton.Inerte = d.Montando || d.Estado != Listo
		bloques := []bloque{fijo{boton}, esperar, manual(pasos)}
		if d.FalloMontaje {
			bloques = append(bloques, nuevaAyuda(ui.AyudaPeligro, tt("devices.oneClick.failTitle"), tt("devices.oneClick.failBody")))
		}
		return col(HuecoBloques, bloques...)

	case APIs:
		// Una dirección por producto y dialecto, derivadas del dominio de la suite: la
		// herramienta pide UNA base, y traducirla mal es media integración que falla en
		// silencio.
		bases := col(HuecoValores)
		for _, b := range [...]struct{ rotulo, producto, dialecto string }{
			{"devices.apis.identityGoogle", "id", "google"},
			{"devices.apis.driveGoogle", "drive", "google"},
			{"devices.apis.driveGraph", "drive", "graph"},
			{"devices.apis.calendarGoogle", "calendar", "google"},
			{"devices.apis.calendarGraph", "calendar", "graph"},
			{"devices.apis.contactsGoogle", "contacts", "google"},
			{"devices.apis.contactsGraph", "contacts", "graph"},
		} {
			bases.hijos = append(bases.hijos, r.fila(b.rotulo, api.ServiceURL(d.Apex, b.producto)+"/compat/"+b.dialecto, "devices.copied.apiBase"))
		}
		return col(HuecoBloques,
			col(HuecoPasosNum,
				r.paso(1, "devices.apis.step1.title", "devices.apis.step1.body"),
				r.paso(2, "devices.apis.step2.title", "devices.apis.step2.body"),
				r.paso(3, "devices.apis.step3.title", "devices.apis.step3.body")),
			bases,
			nuevaAyuda(ui.AyudaInfo, tt("devices.apis.expectTitle"), tt("devices.apis.expectBody")))
	}
	return col(0)
}

func (r *clientesExternosRenderer) instruccionesApple(p Plataforma, manual func(bloque) *Detalles) bloque {
	d := r.d
	tt := func(k string) string { return d.T.T(k, nil) }
	perfil := NuevaAccion(tt("devices.apple.download"), tema.TextoMD, func() {
		if p == d.EsteApple && d.AlConfigurarApple != nil {
			d.AlConfigurarApple()
			return
		}
		if d.AlEnlacePerfilApple != nil {
			d.AlEnlacePerfilApple(p)
		}
	})
	perfil.Inerte = d.PerfilCreando || d.Estado != Listo ||
		(p != d.EsteApple || d.AlConfigurarApple == nil) && d.AlEnlacePerfilApple == nil
	perfilBloques := []bloque{fijo{perfil}}
	if d.PerfilFallo {
		perfilBloques = append(perfilBloques, nuevaAyuda(ui.AyudaPeligro, tt("devices.oneClick.failTitle"), tt("devices.oneClick.failBody")))
	}
	segundo := r.paso(2, "devices.apple.step2.mac.title", "devices.apple.step2.mac.body")
	if p == IPhone {
		segundo = r.paso(2, "devices.apple.step2.iphone.title", "devices.apple.step2.iphone.body")
	}
	return col(HuecoBloques,
		col(HuecoValores, perfilBloques...),
		col(HuecoPasosNum,
			r.paso(1, "devices.apple.step1.title", "devices.apple.step1.body"), segundo,
			r.paso(3, "devices.apple.step3.title", "devices.apple.step3.body"),
			r.paso(4, "devices.apple.step4.title", "devices.apple.step4.body")),
		nuevaAyuda(ui.AyudaInfo, tt("devices.apple.manualTitle"), tt("devices.apple.manualBody")),
		manual(col(HuecoInstrucciones, r.valoresCorreo(), r.valoresDAV())))
}

// conUnClic pone delante de `resto` el botón de un clic de `p`, y debajo lo que ha
// dejado: la contraseña con su botón de copiar, qué hacer en el programa y, en Android,
// el alta de DAVx⁵. Sin `AlConfigurarCliente` quedan solo los pasos de siempre.
func (r *clientesExternosRenderer) conUnClic(p Plataforma, resto ...bloque) bloque {
	d, t := r.d, r.d.T
	tt := func(k string) string { return t.T(k, nil) }
	if d.AlConfigurarCliente == nil {
		return col(HuecoBloques, resto...)
	}
	boton := NuevaAccion(tt("devices.oneClick."+string(p)), tema.TextoMD, func() { d.AlConfigurarCliente(p) })
	c := d.Cliente
	boton.Inerte = c.Creando
	bloques := []bloque{col(HuecoValores, fijo{boton}, texto(tt("devices.oneClick.body"), xsAjustada))}
	switch {
	case c.Plataforma != p:
	case c.Fallo:
		bloques = append(bloques, nuevaAyuda(ui.AyudaPeligro, tt("devices.oneClick.failTitle"), tt("devices.oneClick.failBody")))
	case c.Clave != "":
		valores := col(HuecoValores,
			r.fila("devices.desktop.emailLabel", d.Correo, "devices.copied.address"),
			r.fila("devices.oneClick.passwordLabel", c.Clave, "devices.copied.password"),
			texto(tt("devices.oneClick.next."+string(p)), tema.TextoSM))
		bloques = append(bloques, nuevaAyuda(ui.AyudaExito, tt("devices.oneClick.onceTitle"), tt("devices.oneClick.onceBody")), valores)
		if !c.Abierto && !c.Creando && p != Android {
			bloques = append(bloques, nuevaAyuda(ui.AyudaAviso, "",
				t.T("devices.oneClick.notOpened", i18n.Vars{"program": nombrePrograma[p]})))
		}
		if p == Android && d.AlAbrirDAVx5 != nil {
			dav := func(clave, base string, contactos bool) bloque {
				return fijo{ui.NuevoBoton(tt(clave), func() { d.AlAbrirDAVx5(base, contactos) }).Con(ui.Secundario, ui.TallaMD)}
			}
			bloques = append(bloques, col(HuecoValores,
				dav("devices.android.davx5Calendars", d.Coordenadas.CalDAVURL, false),
				dav("devices.android.davx5Contacts", d.Coordenadas.CardDAVURL, true),
				fijo{ui.NuevoBoton(tt("devices.android.davx5Install"), func() {
					if d.AlAbrirURL != nil {
						d.AlAbrirURL(FichaDAVx5)
					}
				}).Con(ui.Fantasma, ui.TallaMD)}))
		}
	}
	return col(HuecoBloques, append(bloques, resto...)...)
}

// comprobaciones son las filas del último paso: cómo saber que ha funcionado.
func (r *clientesExternosRenderer) comprobaciones(p Plataforma) [][2]string {
	t := r.d.T
	fila := func(k string) [2]string {
		return [2]string{t.T("devices.check."+k+".label", nil), t.T("devices.check."+k+".value", nil)}
	}
	switch p {
	case IPhone, Mac:
		return [][2]string{fila("profile"), fila("mail"), fila("dav")}
	case Thunderbird, Android:
		return [][2]string{fila("mail"), fila("dav")}
	case Outlook:
		return [][2]string{fila("mail")}
	case UnidadDrive:
		return [][2]string{fila("drive"), fila("write")}
	case APIs:
		return [][2]string{fila("apis")}
	}
	return nil
}

func (r *clientesExternosRenderer) recomponer() {
	d := r.d
	r.objetos = []fyne.CanvasObject{r.fondo, r.titulo, r.entrada}
	if !d.sinDatos() {
		r.objetos = append(r.objetos, r.pasos, r.encabezado, r.frase)
	}
	r.objetos = append(r.objetos, r.cuerpo.objetos()...)
	if !d.sinDatos() && d.Paso < 2 {
		r.objetos = append(r.objetos, r.filete, r.siguiente)
		if d.Paso > 0 {
			r.objetos = append(r.objetos, r.atras)
		}
	}
}

func (r *clientesExternosRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, r.alto(r.d.Ancho())) }
func (r *clientesExternosRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *clientesExternosRenderer) Destroy()                     {}

// Refresh rehace la pantalla (ver el comentario del lanzador).
func (r *clientesExternosRenderer) Refresh() {
	r.rehacer()
	r.Layout(r.d.Size())
	canvas.Refresh(r.d)
}

func (r *clientesExternosRenderer) alto(ventana float32) float32 { return r.colocar(ventana, false) }

func (r *clientesExternosRenderer) Layout(t fyne.Size) {
	if t.Width <= 0 {
		return
	}
	r.fondo.Resize(fyne.NewSize(t.Width, max(t.Height, r.alto(t.Width))))
	r.colocar(t.Width, true)
}

// colocar recorre la pantalla de arriba abajo; con `mover` a false solo mide.
func (r *clientesExternosRenderer) colocar(ventana float32, mover bool) float32 {
	d := r.d
	x, ancho := CajaAjustes(ventana)
	y := ponerParrafo(r.titulo, x, MargenAjustesY, ancho, mover) + MargenEntradaAjustes
	y = ponerParrafo(r.entrada, x, y, min(ancho, AnchoIntro), mover) + MargenAjustesY
	if d.sinDatos() {
		if mover {
			r.cuerpo.poner(x, y, ancho)
		}
		return y + r.cuerpo.alto(ancho) + MargenAjustesY
	}
	y = ponerObjeto(r.pasos, x, y, fyne.NewSize(ancho, ui.AltoPasos), mover) + HuecoAsistente
	y = ponerParrafo(r.encabezado, x, y, ancho, mover) + MargenEntradaAjustes
	y = ponerParrafo(r.frase, x, y, min(ancho, AnchoIntro), mover) + HuecoAsistente
	if mover {
		r.cuerpo.poner(x, y, ancho)
	}
	y += r.cuerpo.alto(ancho)
	if d.Paso < 2 {
		y += HuecoAsistente
		y = ponerObjeto(r.filete, x, y, fyne.NewSize(ancho, tema.Borde), mover) + MargenPieAsistente
		// `ml-auto`: los botones a la derecha, «Atrás» delante de «Continuar».
		s := r.siguiente.MinSize()
		ponerObjeto(r.siguiente, x+ancho-s.Width, y, s, mover)
		if d.Paso > 0 {
			a := r.atras.MinSize()
			ponerObjeto(r.atras, x+ancho-s.Width-HuecoBotonesPie-a.Width, y, a, mover)
		}
		y += s.Height
	}
	return y + MargenAjustesY
}

// ---- Piezas propias del asistente ----

// rejillaOpciones es `grid max-w-3xl gap-3 sm:grid-cols-2`: dos columnas desde `sm:`
// y una por debajo. Las dos tarjetas de una fila miden lo que la más alta, que es lo que
// hace una fila de rejilla con `align-items: stretch`.
type rejillaOpciones struct {
	opciones []*ui.TarjetaEleccion
	ventana  float32
}

func (g *rejillaOpciones) geometria(ancho float32) (cols int, columna float32) {
	ancho = min(ancho, AnchoAsistente)
	if g.ventana >= ui.CorteSM {
		return 2, (ancho - HuecoOpcionesAsistente) / 2
	}
	return 1, ancho
}

func (g *rejillaOpciones) filas(ancho float32) []float32 {
	cols, columna := g.geometria(ancho)
	var out []float32
	for i := 0; i < len(g.opciones); i += cols {
		var h float32
		for c := i; c < i+cols && c < len(g.opciones); c++ {
			h = max(h, ui.AltoTarjetaEleccion(g.opciones[c], columna))
		}
		out = append(out, h)
	}
	return out
}

func (g *rejillaOpciones) alto(ancho float32) float32 {
	filas := g.filas(ancho)
	h := float32(max(len(filas)-1, 0)) * HuecoOpcionesAsistente
	for _, f := range filas {
		h += f
	}
	return h
}

func (g *rejillaOpciones) poner(x, y, ancho float32) {
	cols, columna := g.geometria(ancho)
	for i, h := range g.filas(ancho) {
		for c := range cols {
			k := i*cols + c
			if k >= len(g.opciones) {
				break
			}
			o := g.opciones[k]
			o.Ancho = columna
			ponerObjeto(o, x+float32(c)*(columna+HuecoOpcionesAsistente), y, fyne.NewSize(columna, h), true)
		}
		y += h + HuecoOpcionesAsistente
	}
}

func (g *rejillaOpciones) objetos() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, len(g.opciones))
	for _, o := range g.opciones {
		out = append(out, o)
	}
	return out
}

// claveEsperarDrive es el «qué vas a encontrarte» de Drive: en Windows, con el botón de
// conectar, Drive es la carpeta de archivos a petición (`nucleo/nube`), que sí trabaja
// sin conexión y sube lo que se guarda; en los demás, el WebDAV, que no.
func claveEsperarDrive(goos string, conBoton bool) string {
	if goos == "windows" && conBoton {
		return "devices.drive.windowsExpectBody"
	}
	if conBoton {
		return "devices.native.driveBody"
	}
	return "devices.drive.expectBody"
}

// pasoNumerado es el `Step` de `ConnectDevices.tsx`: `flex gap-3`, un círculo de 21
// en `bg-bg-subtle` con el número (`text-xs font-semibold text-text-muted`), el título
// (`text-sm font-medium`) y el cuerpo (`mt-0.5 text-sm leading-snug text-text-muted`).
type pasoNumerado struct {
	circulo *canvas.Circle
	numero  *canvas.Text
	titulo  *ui.Parrafo
	cuerpo  *ui.Parrafo
}

// Medidas del paso numerado.
const (
	HuecoPasoNumerado float32 = 3 * 0.25 * tema.Rem
	MargenCuerpoPaso  float32 = 0.5 * 0.25 * tema.Rem
)

func nuevoPaso(n int, titulo, cuerpo string) *pasoNumerado {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &pasoNumerado{
		circulo: canvas.NewCircle(tema.Color("bg-subtle", v)),
		numero:  ui.Texto(fmt.Sprint(n), tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v)),
		titulo:  ui.NuevoParrafo(titulo, tema.TextoSM, ui.Medio, "text", v),
		cuerpo:  ui.NuevoParrafo(cuerpo, smAjustada, ui.Normal, "text-muted", v),
	}
}

func (p *pasoNumerado) textos(ancho float32) float32 {
	return max(ancho-ui.LadoCirculoPaso-HuecoPasoNumerado, 0)
}

func (p *pasoNumerado) alto(ancho float32) float32 {
	w := p.textos(ancho)
	return max(ui.LadoCirculoPaso, ui.AltoParrafo(p.titulo, w)+MargenCuerpoPaso+ui.AltoParrafo(p.cuerpo, w))
}

func (p *pasoNumerado) poner(x, y, ancho float32) {
	p.circulo.Move(fyne.NewPos(x, y))
	p.circulo.Resize(fyne.NewSize(ui.LadoCirculoPaso, ui.LadoCirculoPaso))
	w := ui.Medir(p.numero.Text, tema.TextoXS, ui.SemiNegrita).Width
	ui.ColocarTexto(p.numero, tema.TextoXS, x+(ui.LadoCirculoPaso-w)/2, y+(ui.LadoCirculoPaso-tema.TextoXS.Linea)/2)
	tx, tw := x+ui.LadoCirculoPaso+HuecoPasoNumerado, p.textos(ancho)
	ty := ponerParrafo(p.titulo, tx, y, tw, true) + MargenCuerpoPaso
	ponerParrafo(p.cuerpo, tx, ty, tw, true)
}

func (p *pasoNumerado) objetos() []fyne.CanvasObject {
	return []fyne.CanvasObject{p.circulo, p.numero, p.titulo, p.cuerpo}
}
