// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/certificados"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
FIRMAR CON EL CERTIFICADO DEL SISTEMA (ADR 0051), dentro del modo «Con certificado»: lo que
la web no puede hacer, porque el navegador no llega al almacén del sistema ni a una tarjeta.

Tres pasos a la vista, en este orden y sin saltarse ninguno:

 1. elegir el certificado (los de las tarjetas por PKCS#11 y los del sistema);
 2. PREPARAR en el servicio y enseñar lo que dice él —no la app— de lo que se firma: quién,
    con qué emisor, hasta cuándo, qué nivel y qué autoridad sella—;
 3. el PIN (si la tarjeta no lo pide en su teclado) y firmar: el sistema firma el resumen y
    el servicio completa. Si la autoridad de sellado no responde, NO se firma sin sello a
    escondidas: la firma ya hecha vale para reintentar o para seguir sin sello.

Aquí no hay criptografía: el almacén le pide la firma al sistema y el servicio monta el
resto. La norma de certipdf.go se levanta solo para esa llamada.
*/

// firmaSistema es el estado de esos tres pasos.
type firmaSistema struct {
	buscando, buscado bool
	certs             []certificados.Certificado
	almacenes         []certificados.Almacen
	elegido           int
	prep              *api.PDFFirmaPreparada
	pin               *ui.Entrada
	// firma y formato se guardan tras firmar: con la autoridad caída, reintentar no
	// vuelve a pedir el PIN.
	firma      []byte
	formato    string
	selloCaido bool
	trabajando bool
	error      string
}

// Firma con el certificado del sistema: el paso de la Pantalla que da Almacenes.
func (d *dialogoFirmar) conSistema(ancho float32) fyne.CanvasObject {
	a, s := d.a, &d.sistema
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if !s.buscado {
		rotulo := a.t("pdf.dialog.firmar.sistema.buscar")
		if s.buscando {
			rotulo = a.t("pdf.dialog.firmar.sistema.buscando")
		}
		b := botonPanel(rotulo, tema.TextoXS, tema.Espacio(1.5), d.buscarCertificados)
		b.PX, b.Inerte = tema.Espacio(3), s.buscando
		return container.New(alPrincipio{}, b)
	}
	if len(s.certs) == 0 {
		return notaDialogo(a.t("pdf.dialog.firmar.sistema.ninguno"), ancho)
	}
	rotulos := make([]string, len(s.certs))
	for i, c := range s.certs {
		rotulos[i] = c.Nombre() + " · " + c.Almacen
	}
	partes := []fyne.CanvasObject{
		conRotulo(a.t("pdf.dialog.firmar.sistema.buscar"), selectorDialogo(rotulos, s.elegido, ancho, func(i int) {
			// Otro certificado es otra preparación: la anterior nombra al otro.
			s.elegido, s.prep, s.firma, s.selloCaido, s.error = i, nil, nil, false, ""
			a.rehacer()
		})),
	}
	if s.prep == nil {
		b := botonPrimario(a.t("pdf.dialog.firmar.sistema.preparar"), tema.TextoXS, tema.Espacio(3), tema.Espacio(1.5), d.prepararSistema)
		b.Inerte = s.trabajando
		partes = append(partes, container.New(alPrincipio{}, b))
	} else {
		partes = append(partes, d.loQueSeFirma(ancho))
		if s.certs[s.elegido].PidePIN && s.firma == nil {
			if s.pin == nil {
				s.pin = entradaDialogo("", ancho)
				s.pin.Contrasena(true)
			}
			s.pin.Ancho = ancho
			partes = append(partes, conRotulo(a.t("pdf.dialog.firmar.sistema.pin"), s.pin))
		}
		partes = append(partes, d.botonesSistema())
	}
	if s.error != "" {
		partes = append(partes, ui.NuevoParrafo(s.error, tema.TextoXS, ui.Normal, "danger-text", v).A(ancho))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, partes...)
}

// loQueSeFirma es lo que el SERVICIO dice de la firma preparada, antes del PIN.
func (d *dialogoFirmar) loQueSeFirma(ancho float32) fyne.CanvasObject {
	a, p := d.a, d.sistema.prep
	lineas := []string{
		a.p.tv("pdf.dialog.firmar.sistema.resumen", i18n.Vars{
			"pages": p.Paginas, "signer": p.Firmante.Nombre, "issuer": p.Firmante.Emisor,
			"until": p.Firmante.ValidoHasta.Format("02/01/2006"),
		}),
	}
	if p.Nivel == "cualificada" {
		lineas = append(lineas, a.t("pdf.dialog.firmar.sistema.cualificada"))
	} else {
		lineas = append(lineas, a.t("pdf.dialog.firmar.sistema.avanzada"))
	}
	if p.Sello != "" {
		lineas = append(lineas, a.p.tv("pdf.dialog.firmar.sistema.sello", i18n.Vars{"tsa": p.Sello}))
	} else {
		lineas = append(lineas, a.t("pdf.dialog.firmar.sistema.sinSello"))
	}
	if d.sistema.selloCaido {
		lineas = append(lineas, a.t("pdf.dialog.firmar.sistema.selloCaido"))
	}
	return avisoAmbar(strings.Join(lineas, " "), ancho)
}

// botonesSistema: «Firmar», o con la autoridad caída «Volver a intentarlo» y «Firmar
// sin sello».
func (d *dialogoFirmar) botonesSistema() fyne.CanvasObject {
	a, s := d.a, &d.sistema
	var botones []fyne.CanvasObject
	if s.selloCaido {
		sin := botonPanel(a.t("pdf.dialog.firmar.sistema.sinSelloBoton"), tema.TextoXS, tema.Espacio(1.5), func() { d.completarSistema(true) })
		sin.PX, sin.Inerte = tema.Espacio(3), s.trabajando
		otra := botonPrimario(a.t("pdf.dialog.firmar.sistema.reintentar"), tema.TextoXS, tema.Espacio(3), tema.Espacio(1.5), func() { d.completarSistema(false) })
		otra.Inerte = s.trabajando
		botones = append(botones, sin, otra)
	} else {
		f := botonPrimario(a.t("pdf.dialog.firmar.sistema.firmar"), tema.TextoXS, tema.Espacio(3), tema.Espacio(1.5), d.firmarSistema)
		f.Inerte = s.trabajando
		botones = append(botones, f)
	}
	return container.New(alPrincipio{}, container.New(ui.Fila{Hueco: tema.Espacio(2)}, botones...))
}

func (d *dialogoFirmar) buscarCertificados() {
	a, s := d.a, &d.sistema
	if a.p.Almacenes == nil || s.buscando {
		return
	}
	s.buscando = true
	a.rehacer()
	almacenes := a.p.Almacenes
	go func() {
		ctx, parar := context.WithTimeout(a.p.ctx, plazo)
		defer parar()
		var certs []certificados.Certificado
		var de []certificados.Almacen
		for _, alm := range almacenes() {
			cs, err := alm.Certificados(ctx)
			if err != nil {
				continue // un almacén que falla no esconde los demás
			}
			for range cs {
				de = append(de, alm)
			}
			certs = append(certs, cs...)
		}
		fyne.Do(func() {
			s.buscando, s.buscado, s.certs, s.almacenes, s.elegido = false, true, certs, de, 0
			a.rehacer()
		})
	}()
}

// opcionesDeFirma son las del recuadro y el motivo, como las de la firma con .p12.
func (d *dialogoFirmar) opcionesDeFirma() api.PDFFirmaOpciones {
	rect := rectFirma
	if !d.visible {
		rect = [4]float64{}
	}
	pagina, _ := strconv.Atoi(strings.TrimSpace(d.pagina.Texto()))
	return api.PDFFirmaOpciones{
		Motivo: d.motivo.Texto(), Lugar: d.lugar.Texto(), Pagina: pagina - 1, Rect: rect, Bloquear: d.bloquear,
	}
}

func (d *dialogoFirmar) prepararSistema() {
	a, s := d.a, &d.sistema
	doc := a.p.doc
	if doc == nil || s.trabajando || s.elegido >= len(s.certs) {
		return
	}
	s.trabajando, s.error = true, ""
	a.rehacer()
	cadena, opts := s.certs[s.elegido].DER(), d.opcionesDeFirma()
	go func() {
		ctx, parar := context.WithTimeout(a.p.ctx, plazo)
		defer parar()
		prep, err := doc.PrepararFirma(ctx, opts, cadena)
		fyne.Do(func() {
			s.trabajando = false
			if err != nil {
				s.error = a.p.mensajeDeError(err, "pdf.error.apply")
			} else {
				s.prep, s.firma, s.selloCaido = &prep, nil, false
			}
			a.rehacer()
		})
	}()
}

func (d *dialogoFirmar) firmarSistema() {
	a, s := d.a, &d.sistema
	if s.prep == nil || s.trabajando {
		return
	}
	pin := ""
	if s.pin != nil {
		pin = s.pin.Texto()
	}
	s.trabajando, s.error = true, ""
	a.rehacer()
	alm, cert, prep := s.almacenes[s.elegido], s.certs[s.elegido], *s.prep
	go func() {
		ctx, parar := context.WithTimeout(a.p.ctx, plazo)
		defer parar()
		firma, formato, err := alm.Firmar(ctx, cert, prep, pin)
		fyne.Do(func() {
			if s.pin != nil {
				s.pin.PonerTexto("") // el PIN no se queda escrito
			}
			if err != nil {
				s.trabajando, s.error = false, d.errorDeAlmacen(err)
				a.rehacer()
				return
			}
			s.firma, s.formato, s.trabajando = firma, formato, false
			d.completarSistema(false)
		})
	}()
}

// completarSistema manda la firma al servicio y guarda la copia firmada en Drive.
func (d *dialogoFirmar) completarSistema(sinSello bool) {
	a, s := d.a, &d.sistema
	doc := a.p.doc
	if doc == nil || s.prep == nil || s.firma == nil || s.trabajando {
		return
	}
	s.trabajando, s.error = true, ""
	a.rehacer()
	prep, firma, formato, sufijo := *s.prep, s.firma, s.formato, a.t("pdf.suffix.signed")
	go func() {
		ctx, parar := context.WithTimeout(a.p.ctx, plazo)
		defer parar()
		firmado, _, err := doc.pdf.CompletarFirma(ctx, prep, firma, formato, sinSello)
		var f api.File
		if err == nil {
			f, err = doc.GuardarFirmado(ctx, firmado, sufijo)
		}
		fyne.Do(func() {
			s.trabajando = false
			switch {
			case api.PDFSelloCaido(err):
				s.selloCaido = true
			case api.PDFFirmaCaducada(err):
				s.prep, s.firma, s.selloCaido, s.error = nil, nil, false, a.t("pdf.dialog.firmar.sistema.caducada")
			case err != nil:
				s.error = a.p.mensajeDeError(err, "pdf.error.apply")
			default:
				a.cerrar()
				a.p.firmadoConElSistema(doc, f)
				return
			}
			a.rehacer()
		})
	}()
}

// errorDeAlmacen traduce lo que dice la tarjeta o el sistema. Cancelar en el diálogo
// del sistema no es un error: no se dice nada.
func (d *dialogoFirmar) errorDeAlmacen(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return ""
	case errors.Is(err, certificados.ErrPIN):
		return d.a.t("pdf.dialog.firmar.sistema.pinMalo")
	case errors.Is(err, certificados.ErrPINBloqueado):
		return d.a.t("pdf.dialog.firmar.sistema.pinBloqueado")
	case errors.Is(err, certificados.ErrNoEsta):
		return d.a.t("pdf.dialog.firmar.sistema.noEsta")
	}
	return d.a.p.mensajeDeError(err, "pdf.error.apply")
}

// firmadoConElSistema es lo mismo que tras firmar con .p12: el aviso con la copia y, si
// alguien había pedido firmar este PDF, la entrega.
func (p *Pantalla) firmadoConElSistema(doc *Documento, f api.File) {
	if p.doc != doc {
		return
	}
	p.Error, p.Aviso, p.Copia = "", p.tv("pdf.saved.copy", i18n.Vars{"name": f.Name}), doc.RutaDe(f)
	if p.peticion != nil {
		p.entregarFirma(*p.peticion, p.Copia)
	}
	p.Recomponer()
}
