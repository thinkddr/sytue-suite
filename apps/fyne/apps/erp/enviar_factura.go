// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La factura en PDF y por correo (`EnviarFactura.tsx`)
//
// El PDF de la factura de cliente (`GET /pdf`) y su envío al cliente con el PDF adjunto
// (`GET|POST /enviar`): las rutas del servicio, que la edición local sirve igual en
// proceso. La ficha no los ofrece si la pantalla no los pide (Salida), que es lo de
// siempre en la edición conectada; los pide Sytue Office, que además dice dónde se
// guarda el PDF y con qué se abre, y qué hacer si no hay con qué enviarlo.

// SalidaFactura son los gestos con los que la factura de cliente sale de la app además
// de imprimirla. Su valor cero es el de siempre: la ficha no ofrece ni «PDF» ni «Enviar
// por correo».
type SalidaFactura struct {
	// PDF enseña el botón «PDF» en la ficha de una factura de cliente.
	PDF bool
	// Enviar enseña «Enviar por correo» (el sobre de la web) y su diálogo.
	Enviar bool
	// Guardar deja `datos` donde elija la persona (el selector de guardado del sistema,
	// con `titulo` y el nombre propuesto) y llama a listo en el hilo de la interfaz con
	// la ruta, o con "" si cancela. Nil: a la carpeta de descargas, como el navegador.
	Guardar func(titulo, nombre string, datos []byte, listo func(ruta string, err error))
	// Abrir abre el PDF guardado; nil, con la app del sistema.
	Abrir func(ruta string)
	// Redactar abre el programa de correo con el mensaje escrito, cuando la app no tiene
	// con qué enviarlo (sin cuenta de correo): el PDF ya está guardado en `ruta` para
	// adjuntarlo a mano. Nil: no se ofrece.
	Redactar func(para, asunto, mensaje, ruta string) error
	// RedactarConPDF es «Abrir en el correo» con el PDF YA ADJUNTO (Sytue Office: el
	// redactor de Sytue Mail de este equipo, que recoge el PDF que se le deja). Se
	// prueba antes que Redactar y se llama fuera del hilo de la interfaz; si devuelve un
	// error (Mail no está, este equipo no tiene dónde dejárselo), se hace lo de Redactar.
	// Nil: siempre Redactar.
	RedactarConPDF func(para, asunto, mensaje, nombre string, pdf []byte) error
}

// TextosEnvio son los textos del envío que solo dice Sytue Office (es, en; los otros
// veinticuatro idiomas, en `traduccionesEnvio`, textos_idiomas.go): elegir la cuenta de
// correo y lo que se hace sin ninguna. No están en el catálogo de la web. «{ruta}» se
// sustituye a mano por dónde quedó el PDF.
var TextosEnvio = map[string][2]string{
	"desde": {"Desde", "From"},
	"sinCuenta": {
		"No hay ninguna cuenta de correo en Sytue Mail de este equipo. Añade una en Mail, o abre el mensaje en tu programa de correo y adjunta el PDF.",
		"There is no email account in Sytue Mail on this device. Add one in Mail, or open the message in your email app and attach the PDF.",
	},
	"abrirCorreo": {"Abrir en el correo", "Open in email app"},
	"adjuntaPDF":  {"Adjunta al mensaje el PDF guardado en {ruta}", "Attach the PDF saved at {ruta} to the message"},
	"enMail":      {"El mensaje está en Sytue Mail con el PDF adjunto", "The message is in Sytue Mail with the PDF attached"},
}

// textoEnvio es el de TextosEnvio en el idioma de tr; lo que falte cae al inglés.
func textoEnvio(clave string, tr *i18n.Translator) string {
	return i18n.Propios{Base: TextosEnvio, Traducciones: traduccionesEnvio}.En(tr.Locale(), clave)
}

// Topes de lo que se escribe, los del servicio (`maxAsuntoEnvio`, `maxMensajeEnvio`).
const (
	MaxAsuntoEnvio  = 300
	MaxMensajeEnvio = 10_000
	// FilasMensajeEnvio es el `rows={6}` del texto.
	FilasMensajeEnvio = 6
	// maxPDFFactura acota lo que se lee del PDF: una factura no llega a 1 MiB.
	maxPDFFactura = 32 << 20
)

// correoValido es el `CORREO` de la web: una dirección con pinta de dirección. La
// comprobación de verdad es la del servicio.
var correoValido = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// EnviarFactura es el estado del diálogo de enviar la factura.
type EnviarFactura struct {
	tr *i18n.Translator
	// Referencia es el número de la factura, para el título y el asunto.
	Referencia string
	// Datos es lo que dijo el servicio al abrir; nil, aún no ha llegado (la rueda).
	Datos *api.ERPEnvioFactura
	// Tocado: se intentó enviar, y los errores de los campos ya se enseñan.
	Tocado, Enviando bool
	// Fallo es el `InlineHelp` rojo de abajo.
	Fallo string

	AlCambio func()
	// AlEnviar manda el correo; AlPDF es el enlace «Descargar factura en PDF»; AlRedactar,
	// «Abrir en el correo» (nil, no se ofrece).
	AlEnviar   func(api.ERPCorreoFactura)
	AlPDF      func()
	AlRedactar func()

	para, asunto *ui.Entrada
	mensaje      *ui.AreaTexto
	desde        *ui.Selector
	podia        bool
}

// NuevoEnviarFactura abre el diálogo con el asunto y el texto del catálogo; los del
// servicio, si los propone, llegan con Poner.
func NuevoEnviarFactura(tr *i18n.Translator, referencia string) *EnviarFactura {
	x := &EnviarFactura{tr: tr, Referencia: referencia}
	vars := i18n.Vars{"ref": referencia}
	x.para = ui.NuevaEntrada("")
	x.asunto = ui.NuevaEntrada("")
	x.asunto.PonerTexto(tr.T("send.defaultSubject", vars))
	x.mensaje = ui.NuevaAreaTexto("")
	x.mensaje.Filas = FilasMensajeEnvio
	x.mensaje.PonerTexto(tr.T("send.defaultMessage", vars))
	x.para.AlCambiar(func(string) { x.siCambiaPuede() })
	x.asunto.AlCambiar(func(s string) {
		if utf8.RuneCountInString(s) > MaxAsuntoEnvio {
			x.asunto.PonerTexto(string([]rune(s)[:MaxAsuntoEnvio]))
			return
		}
		x.siCambiaPuede()
	})
	x.mensaje.AlCambiar(func(s string) {
		if utf8.RuneCountInString(s) > MaxMensajeEnvio {
			x.mensaje.PonerTexto(string([]rune(s)[:MaxMensajeEnvio]))
		}
	})
	return x
}

func (x *EnviarFactura) t(clave string) string { return x.tr.T(clave, nil) }

func (x *EnviarFactura) cambio() {
	x.podia = x.Puede()
	if x.AlCambio != nil {
		x.AlCambio()
	}
}

// siCambiaPuede repinta solo cuando lo tecleado cambia si se puede enviar (el error
// del campo y el botón), no a cada letra.
func (x *EnviarFactura) siCambiaPuede() {
	if x.Puede() != x.podia || x.Tocado {
		x.cambio()
	}
}

// Poner es lo que contestó el servicio: la dirección del cliente (si no se ha escrito
// otra), las cuentas desde las que sale y el asunto y el texto que propone.
func (x *EnviarFactura) Poner(d api.ERPEnvioFactura) {
	x.Datos = &d
	if strings.TrimSpace(x.para.Texto()) == "" {
		x.para.PonerTexto(d.Para)
	}
	if d.Asunto != "" {
		x.asunto.PonerTexto(d.Asunto)
	}
	if d.Mensaje != "" {
		x.mensaje.PonerTexto(d.Mensaje)
	}
	opciones := make([]string, 0, len(d.Cuentas))
	for _, c := range d.Cuentas {
		o := c.Direccion
		if c.Nombre != "" && c.Nombre != c.Direccion {
			o = c.Nombre + " · " + c.Direccion
		}
		opciones = append(opciones, o)
	}
	x.desde = ui.NuevoSelector(opciones, func(int) {})
	x.desde.Rotulo = textoEnvio("desde", x.tr)
	x.cambio()
}

// Para, Asunto y Mensaje son lo escrito.
func (x *EnviarFactura) Para() string    { return strings.TrimSpace(x.para.Texto()) }
func (x *EnviarFactura) Asunto() string  { return strings.TrimSpace(x.asunto.Texto()) }
func (x *EnviarFactura) Mensaje() string { return x.mensaje.Texto() }

// ParaValido es `paraValido`.
func (x *EnviarFactura) ParaValido() bool { return correoValido.MatchString(x.Para()) }

// Bloqueo es el aviso que impide enviar, o "": sin servidor de correo, una factura sin
// emitir y, en Sytue Office, sin ninguna cuenta de correo. En la suite manda el orden de
// la web (el despliegue antes que la factura); por cuenta, la factura antes, porque sin
// cuenta aún se puede abrir en el programa de correo, y un borrador no se manda por
// ningún lado.
func (x *EnviarFactura) Bloqueo() string {
	d := x.Datos
	switch {
	case d == nil:
		return ""
	case d.PorCuenta && !d.Enviable:
		return x.t("send.notIssued")
	case d.PorCuenta && !d.Disponible:
		return textoEnvio("sinCuenta", x.tr)
	case !d.Disponible:
		return x.t("send.unavailable")
	case !d.Enviable:
		return x.t("send.notIssued")
	}
	return ""
}

// PuedeRedactar: sin cuenta en Sytue Office, la factura emitida se abre en el programa
// de correo del sistema.
func (x *EnviarFactura) PuedeRedactar() bool {
	d := x.Datos
	return x.AlRedactar != nil && d != nil && d.PorCuenta && !d.Disponible && d.Enviable
}

// Puede es `puede`: hay datos, nada lo impide, la dirección vale y hay asunto.
func (x *EnviarFactura) Puede() bool {
	return x.Datos != nil && x.Bloqueo() == "" && x.ParaValido() && x.Asunto() != "" && !x.Enviando
}

// Peticion es el correo que se manda, desde la cuenta elegida.
func (x *EnviarFactura) Peticion() api.ERPCorreoFactura {
	c := api.ERPCorreoFactura{Para: x.Para(), Asunto: x.Asunto(), Mensaje: x.Mensaje()}
	if x.Datos != nil && x.desde != nil && x.desde.Elegida >= 0 && x.desde.Elegida < len(x.Datos.Cuentas) {
		c.Cuenta = x.Datos.Cuentas[x.desde.Elegida].ID
	}
	return c
}

// Enviar es el `enviar` de la web: marca los campos y, si se puede, manda.
func (x *EnviarFactura) Enviar() {
	x.Tocado = true
	if !x.Puede() {
		x.cambio()
		return
	}
	if x.AlEnviar != nil {
		x.AlEnviar(x.Peticion())
	}
}

// Cuerpo es el del Modal: la rueda hasta que llegan los datos, el aviso que bloquea,
// los campos (`flex flex-col gap-3`) con la línea del adjunto y el fallo.
func (x *EnviarFactura) Cuerpo(v tema.Variante, ancho float32) fyne.CanvasObject {
	p := &pila{}
	hueco := float32(0)
	add := func(o fyne.CanvasObject, h float32) {
		p.add(o, h, hueco)
		hueco = HuecoIA3
	}
	if x.Datos == nil && x.Fallo == "" {
		alto := 2*tema.Espacio(6) + 24
		add(centrado(ui.NuevaRueda(24, "text-muted", v), alto), alto)
	}
	if b := x.Bloqueo(); b != "" {
		a := ui.NuevaAyudaEnLinea(b).ConTono(ui.AyudaAviso)
		add(a, ui.AltoAyudaEnLinea(a, ancho))
	}
	if x.Datos != nil {
		inerte := (x.Bloqueo() != "" && !x.PuedeRedactar()) || x.Enviando
		if x.Datos.PorCuenta && len(x.Datos.Cuentas) > 0 && x.desde != nil {
			x.desde.Ancho = ancho
			x.desde.Inerte(inerte)
			g := ui.GrupoCampo(textoEnvio("desde", x.tr), false, x.desde, "", "")
			add(g, g.MinSize().Height)
		}
		errPara := ""
		if x.Tocado && !x.ParaValido() {
			errPara = x.t("send.invalidTo")
		}
		x.para.Ancho, x.para.Invalida = ancho, errPara != ""
		x.para.Inerte(inerte)
		g := ui.GrupoCampo(x.t("send.to"), true, x.para, "", errPara)
		add(g, g.MinSize().Height)
		x.asunto.Ancho = ancho
		x.asunto.Inerte(inerte)
		g = ui.GrupoCampo(x.t("send.subject"), true, x.asunto, "", "")
		add(g, g.MinSize().Height)
		x.mensaje.Ancho = ancho
		x.mensaje.Inerte(inerte)
		g = ui.GrupoCampo(x.t("send.message"), false, x.mensaje, "", "")
		add(g, g.MinSize().Height)
		add(x.lineaAdjunto(v, ancho))
	}
	if x.Fallo != "" {
		add(ayudaPeligro(x.Fallo, ancho))
	}
	c, _ := p.objeto()
	return c
}

// lineaAdjunto es el `<p class="text-text-muted flex flex-wrap items-center gap-2
// text-sm">` con el recibo, «La factura va adjunta en PDF.» y el enlace al PDF.
func (x *EnviarFactura) lineaAdjunto(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	ic := iconos.Trazo(iconos.Recibo, 14, "text-muted", v).Objeto()
	frase := x.t("send.attachment")
	t := ui.Texto(frase, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
	enlace := x.t("form.downloadPdf")
	b := ui.NuevoEnlace(enlace, tema.TextoSM, ui.Normal, "primary-text", func() {
		if x.AlPDF != nil {
			x.AlPDF()
		}
	})
	b.Subrayado, b.Boton = true, true
	return botonera([]pieza{
		piezaDe(ic, fyne.NewSize(14, 14)),
		{obj: t, ancho: ui.Medir(frase, tema.TextoSM, ui.Normal).Width, alto: tema.TextoSM.Linea, texto: true},
		piezaDe(b, fyne.NewSize(ui.Medir(enlace, tema.TextoSM, ui.Normal).Width, tema.TextoSM.Linea)),
	}, ancho)
}

// Pie son «Cerrar» y «Enviar» (con el sobre) y, sin cuenta en Sytue Office, «Abrir en
// el correo» entre los dos.
func (x *EnviarFactura) Pie(alCerrar func()) fyne.CanvasObject {
	cerrar := ui.NuevoBoton(x.t("action.close"), alCerrar).Con(ui.Fantasma, ui.TallaMD)
	cerrar.Inerte = x.Enviando
	objs := []fyne.CanvasObject{cerrar}
	if x.PuedeRedactar() {
		r := ui.NuevoBoton(textoEnvio("abrirCorreo", x.tr), func() {
			if !x.Enviando {
				x.AlRedactar()
			}
		}).Con(ui.Secundario, ui.TallaMD)
		r.Icono, r.TamIcono, r.Cargando = iconos.Correo, 14, x.Enviando
		objs = append(objs, r)
	}
	enviar := ui.NuevoBoton(x.t("send.submit"), x.Enviar).Con(ui.Primario, ui.TallaMD)
	enviar.Icono, enviar.TamIcono = iconos.Correo, 14
	enviar.Cargando = x.Enviando && !x.PuedeRedactar()
	enviar.Inerte = x.Datos == nil || x.Bloqueo() != "" || x.Enviando
	objs = append(objs, enviar)
	return container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, objs...)
}

// --- Lo que hace la pantalla ---------------------------------------------------------

// bajarPDF pide el PDF de la factura y lo lee entero.
func (p *Pantalla) bajarPDF(ctx context.Context, modelo, id string) ([]byte, string, error) {
	cuerpo, nombre, err := p.svc.PDFFactura(ctx, modelo, id)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = cuerpo.Close() }()
	datos, err := io.ReadAll(io.LimitReader(cuerpo, maxPDFFactura))
	return datos, nombre, err
}

// pdfFactura baja la factura en PDF, la guarda donde diga la persona (o, sin selector,
// en la carpeta de descargas) y la abre.
func (p *Pantalla) pdfFactura(f *Ficha) {
	if p.svc == nil || f.Registro == nil {
		return
	}
	m, id := f.Modelo.Clave, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		datos, nombre, err := p.bajarPDF(ctx, m, id)
		if err != nil {
			enHilo(func() { p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr)) })
			return
		}
		if p.Salida.Guardar == nil {
			ruta, err := guardarDescarga(carpetaDescargas(), nombre, bytes.NewReader(datos))
			enHilo(func() { p.trasGuardarPDF(ruta, err) })
			return
		}
		enHilo(func() { p.Salida.Guardar(p.t("form.downloadPdf"), nombre, datos, p.trasGuardarPDF) })
	})
}

// trasGuardarPDF abre lo guardado; cancelar el selector no hace nada.
func (p *Pantalla) trasGuardarPDF(ruta string, err error) {
	switch {
	case err != nil:
		p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
	case ruta == "":
	case p.Salida.Abrir != nil:
		p.Salida.Abrir(ruta)
	default:
		abrirFichero(ruta)
	}
}

// enviarFactura abre el diálogo de enviar la factura por correo y pide con qué arranca.
func (p *Pantalla) enviarFactura(f *Ficha) {
	if p.lienzo == nil || p.svc == nil || f.Registro == nil {
		return
	}
	ref := cmp.Or(f.Registro.Ref, f.Titulo())
	x := NuevoEnviarFactura(p.tr, ref)
	p.envio = x
	var d *ui.Dialogo
	cerrar := func() {
		if !x.Enviando {
			d.Cerrar()
		}
	}
	ancho := anchoCuerpoDialogo(ui.DialogoMD, p.Ventana.Width)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo, pie := nuevoVivo(x.Cuerpo(v, ancho)), nuevoVivo(x.Pie(cerrar))
	d = ui.NuevoDialogo(p.t("send.title", i18n.Vars{"ref": ref}), cuerpo.c).Con(ui.DialogoMD, pie.c)
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	x.AlCambio = func() {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		cuerpo.poner(x.Cuerpo(v, ancho))
		pie.poner(x.Pie(cerrar))
		d.NoDescartable = x.Enviando
		d.Recolocar()
	}
	x.AlPDF = func() { p.pdfFactura(f) }
	x.AlEnviar = func(c api.ERPCorreoFactura) { p.mandarFactura(f, x, d, c) }
	if p.Salida.Redactar != nil {
		x.AlRedactar = func() { p.redactarFuera(f, x, d) }
	}
	d.Foco = x.para
	d.Mostrar(p.lienzo)
	m, id := f.Modelo.Clave, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		e, err := p.svc.EnvioFactura(ctx, m, id)
		enHilo(func() {
			if err != nil {
				x.Fallo = MensajeDeError(err, p.tr)
				x.cambio()
				return
			}
			x.Poner(e)
		})
	})
}

// mandarFactura la envía y, si sale, cierra, lo dice y vuelve a pedir el historial,
// donde ya está apuntada.
func (p *Pantalla) mandarFactura(f *Ficha, x *EnviarFactura, d *ui.Dialogo, c api.ERPCorreoFactura) {
	if x.Enviando || f.Registro == nil {
		return
	}
	x.Enviando, x.Fallo = true, ""
	x.cambio()
	m, id := f.Modelo.Clave, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.svc.EnviarCorreoFactura(ctx, m, id, c)
		enHilo(func() {
			x.Enviando = false
			if err != nil {
				x.Fallo = MensajeEnvio(err, p.tr)
				x.cambio()
				return
			}
			d.Cerrar()
			p.avisar(ui.AvisoExito, p.t("send.sent", i18n.Vars{"to": c.Para}))
			if p.esta(f) {
				p.cargarSeguimiento(f)
			}
		})
	})
}

// MensajeEnvio es el fallo de enviar como lo dice la web: un 502 es que el servidor de
// correo no lo aceptó (`send.failed`), salvo que el ERP local sepa decir por qué.
func MensajeEnvio(err error, tr *i18n.Translator) string {
	var e *api.Error
	if errors.As(err, &e) && e.Status == http.StatusBadGateway {
		if texto, ok := textoLocal(e.Code(), tr); ok {
			return texto
		}
		return tr.T("send.failed", nil)
	}
	return MensajeDeError(err, tr)
}

// redactarFuera abre el mensaje escrito en el correo: con el PDF adjunto si la app sabe
// (RedactarConPDF) y, si no o si falla, guardando el PDF en la carpeta de descargas y
// abriendo el programa de correo (Redactar), para adjuntarlo a mano.
func (p *Pantalla) redactarFuera(f *Ficha, x *EnviarFactura, d *ui.Dialogo) {
	if x.Enviando || f.Registro == nil || p.Salida.Redactar == nil {
		return
	}
	c := x.Peticion()
	x.Enviando, x.Fallo = true, ""
	x.cambio()
	m, id := f.Modelo.Clave, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		datos, nombre, err := p.bajarPDF(ctx, m, id)
		if err == nil && p.Salida.RedactarConPDF != nil && p.Salida.RedactarConPDF(c.Para, c.Asunto, c.Mensaje, nombre, datos) == nil {
			enHilo(func() {
				x.Enviando = false
				d.Cerrar()
				p.avisar(ui.AvisoInfo, textoEnvio("enMail", p.tr))
			})
			return
		}
		ruta := ""
		if err == nil {
			ruta, err = guardarDescarga(carpetaDescargas(), nombre, bytes.NewReader(datos))
		}
		enHilo(func() {
			if err == nil {
				err = p.Salida.Redactar(c.Para, c.Asunto, c.Mensaje, ruta)
			}
			x.Enviando = false
			if err != nil {
				x.Fallo = MensajeDeError(err, p.tr)
				x.cambio()
				return
			}
			d.Cerrar()
			p.avisar(ui.AvisoInfo, strings.ReplaceAll(textoEnvio("adjuntaPDF", p.tr), "{ruta}", ruta))
		})
	})
}
