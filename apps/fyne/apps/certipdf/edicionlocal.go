// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
Lo que solo monta la EDICIÓN LOCAL (apps/fyne/PLAN-EDICION-LOCAL.md §6.3): sin servicio que
decida, la autoridad de sellado (P-05) y las anclas de confianza (P-06) las elige la persona.
Los dos campos de la Pantalla valen cero en la edición conectada, que sigue igual.

Los textos son propios de la edición y no tienen web: van en `textosLocales` (es, en) y
`traduccionesLocales` (los otros veinticuatro idiomas), con claves sin punto para que no se
confundan con las del catálogo.
*/

// AutoridadSello es una autoridad de sellado de la lista: cómo se llama y su dirección.
type AutoridadSello struct {
	Nombre, URL string
}

// AjusteSello es la autoridad de sellado que elige la persona en este equipo. Leer da la
// elegida ("" = sin sello) y la lista que se ofrece; Poner guarda las dos.
type AjusteSello struct {
	Leer  func() (actual string, lista []AutoridadSello)
	Poner func(actual string, lista []AutoridadSello)
}

// AjusteAnclas es la carpeta de certificados de confianza de la persona. Carpeta y Cuantos
// dicen dónde está y cuántos se han cargado; Elegir abre el selector y, al acabar, llama a
// listo en el hilo de la interfaz.
type AjusteAnclas struct {
	Carpeta func() string
	Cuantos func() int
	Elegir  func(listo func())
}

var textosLocales = map[string][2]string{ // clave: {es, en}
	"firmaAqui":     {"El certificado y la clave no salen de este equipo: la firma se hace aquí y no se guardan.", "The certificate and the key do not leave this device: the signature is made here and they are not stored."},
	"selloTitulo":   {"Sello de tiempo", "Time stamp"},
	"selloNinguno":  {"Sin sello de tiempo", "No time stamp"},
	"selloOtra":     {"Otra autoridad (dirección)", "Another authority (address)"},
	"selloAnadir":   {"Usar y añadir a la lista", "Use and add to the list"},
	"selloQuitar":   {"Quitar de la lista", "Remove from the list"},
	"selloAyuda":    {"Solo viaja a la autoridad la huella de la firma, nunca el documento. Sin autoridad, la firma no lleva sello: prueba quién firmó, pero no cuándo.", "Only the fingerprint of the signature travels to the authority, never the document. Without an authority the signature carries no stamp: it proves who signed, not when."},
	"selloMala":     {"Escribe una dirección http:// o https://.", "Type an http:// or https:// address."},
	"sinSello":      {"Sin sello de tiempo: no has elegido autoridad de sellado.", "No time stamp: you have not chosen a time-stamping authority."},
	"anclasTitulo":  {"Certificados de confianza", "Trusted certificates"},
	"anclasCuantos": {"{n} certificados de confianza en {carpeta}.", "{n} trusted certificates in {carpeta}."},
	"anclasUno":     {"1 certificado de confianza en {carpeta}.", "1 trusted certificate in {carpeta}."},
	"anclasNinguno": {"No hay certificados de confianza en {carpeta}: se comprueba si el documento cambió, pero no quién firmó. Añade ahí los certificados raíz (.pem, .crt, .cer) en que confías.", "There are no trusted certificates in {carpeta}: the check says whether the document changed, but not who signed it. Add the root certificates (.pem, .crt, .cer) you trust there."},
	"anclasElegir":  {"Elegir carpeta…", "Choose folder…"},
	"revocacion":    {"No se comprueba si un certificado ha sido revocado (OCSP, CRL): no se conecta a nadie para verificar.", "Whether a certificate has been revoked (OCSP, CRL) is not checked: nothing is contacted to verify."},
}

// propio es un texto de la edición local, en el idioma de la interfaz: español e inglés
// de `textosLocales`, los otros veinticuatro de `traduccionesLocales`
// (edicionlocal_idiomas.go); lo que falte cae al inglés.
func (p *Pantalla) propio(clave string, vars ...string) string {
	s := i18n.Propios{Base: textosLocales, Traducciones: traduccionesLocales}.En(i18n.Locale(p.Idioma), clave)
	for i := 0; i+1 < len(vars); i += 2 {
		s = strings.ReplaceAll(s, "{"+vars[i]+"}", vars[i+1])
	}
	return s
}

// direccionDeSello acepta solo http(s) con host.
func direccionDeSello(s string) (string, bool) {
	s = strings.TrimSpace(s)
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", false
	}
	return s, true
}

// selloLocal es la elección de la autoridad de sellado de la firma con certificado (de
// fichero o del sistema). Cambiarla tira la preparación: dice qué autoridad sellará.
func (d *dialogoFirmar) selloLocal(ancho float32) fyne.CanvasObject {
	a, s, aj := d.a, &d.sistema, d.a.p.SelloDeTiempo
	actual, lista := aj.Leer()
	rotulos := make([]string, 0, 1+len(lista))
	rotulos = append(rotulos, a.p.propio("selloNinguno"))
	elegida := 0
	for i, au := range lista {
		r := au.Nombre
		if r == "" {
			r = au.URL
		}
		rotulos = append(rotulos, r)
		if au.URL == actual {
			elegida = i + 1
		}
	}
	olvidar := func() { s.prep, s.firma, s.selloCaido, s.error = nil, nil, false, "" }
	selector := selectorDialogo(rotulos, elegida, ancho, func(i int) {
		nueva := ""
		if i > 0 {
			nueva = lista[i-1].URL
		}
		aj.Poner(nueva, lista)
		olvidar()
		a.rehacer()
	})
	if s.urlSello == nil {
		s.urlSello = entradaDialogo("https://", ancho)
	}
	s.urlSello.Ancho = ancho
	anadir := botonPanel(a.p.propio("selloAnadir"), tema.TextoXS, tema.Espacio(1.5), func() {
		nueva, ok := direccionDeSello(s.urlSello.Texto())
		if !ok {
			s.error = a.p.propio("selloMala")
			a.rehacer()
			return
		}
		esta := false
		for _, au := range lista {
			esta = esta || au.URL == nueva
		}
		if !esta {
			u, _ := url.Parse(nueva)
			lista = append(lista, AutoridadSello{Nombre: u.Hostname(), URL: nueva})
		}
		aj.Poner(nueva, lista)
		s.urlSello.PonerTexto("")
		olvidar()
		a.rehacer()
	})
	anadir.PX = tema.Espacio(3)
	botones := []fyne.CanvasObject{anadir}
	if elegida > 0 {
		quitar := botonPanel(a.p.propio("selloQuitar"), tema.TextoXS, tema.Espacio(1.5), func() {
			resto := append(append([]AutoridadSello(nil), lista[:elegida-1]...), lista[elegida:]...)
			aj.Poner("", resto)
			olvidar()
			a.rehacer()
		})
		quitar.PX = tema.Espacio(3)
		botones = append(botones, quitar)
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)},
		conRotulo(a.p.propio("selloTitulo"), selector, notaDialogo(a.p.propio("selloAyuda"), ancho)),
		conRotulo(a.p.propio("selloOtra"), s.urlSello),
		container.New(alPrincipio{}, container.New(ui.Fila{Hueco: tema.Espacio(2)}, botones...)))
}

// anclasLocales es lo que el panel «Firmas» dice de las anclas de la persona, con el
// botón para elegir la carpeta y, si revocacion, que no se consulta revocación (el
// informe ya lo dice con su propio pie).
func (f *panelFirmas) anclasLocales(revocacion bool) []fyne.CanvasObject {
	p := f.l.pant
	if p == nil || p.Anclas == nil {
		return nil
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	carpeta, n := abreviarRuta(p.Anclas.Carpeta()), p.Anclas.Cuantos()
	texto := p.propio("anclasNinguno", "carpeta", carpeta)
	switch {
	case n == 1:
		texto = p.propio("anclasUno", "carpeta", carpeta)
	case n > 1:
		texto = p.propio("anclasCuantos", "n", strconv.Itoa(n), "carpeta", carpeta)
	}
	elegir := botonPanel(p.propio("anclasElegir"), tema.Texto2XS, tema.Espacio(1), func() {
		p.Anclas.Elegir(func() {
			// Otras anclas son otro veredicto: el informe de antes ya no vale.
			f.olvidar()
			f.rehacer()
		})
	})
	elegir.PX = tema.Espacio(2)
	partes := []fyne.CanvasObject{
		ui.NuevoRotulo(p.propio("anclasTitulo"), tema.TextoXS, ui.Medio, "text"),
		ui.NuevoParrafo(texto, tema.Texto2XS, ui.Normal, "text-muted", v).A(anchoUtil()),
		container.New(alPrincipio{}, elegir),
	}
	if revocacion {
		partes = append(partes, ui.NuevoParrafo(p.propio("revocacion"), tema.Texto2XS, ui.Normal, "text-muted", v).A(anchoUtil()))
	}
	return partes
}

// abreviarRuta deja una ruta larga en sus dos últimos tramos: el párrafo solo parte por
// los espacios, y una ruta entera desborda el panel.
func abreviarRuta(ruta string) string {
	if utf8.RuneCountInString(ruta) <= 40 {
		return ruta
	}
	tramos := strings.FieldsFunc(ruta, func(r rune) bool { return r == '/' || r == '\\' })
	if len(tramos) <= 2 {
		return ruta
	}
	return "…/" + strings.Join(tramos[len(tramos)-2:], "/")
}

// AjustesLocales es la pestaña de CertiPDF en los Ajustes de Sytue Office: la misma
// autoridad de sellado y la misma carpeta de anclas que se eligen al firmar y al
// verificar (P-05, P-06), con controles corrientes. nil sin SelloDeTiempo ni Anclas (la
// edición conectada).
func (p *Pantalla) AjustesLocales() fyne.CanvasObject {
	if p.SelloDeTiempo == nil && p.Anclas == nil {
		return nil
	}
	var apartados []fyne.CanvasObject
	if aj := p.SelloDeTiempo; aj != nil {
		apartados = append(apartados, p.ajustesSello(aj)...)
	}
	if an := p.Anclas; an != nil {
		if p.SelloDeTiempo != nil {
			apartados = append(apartados, widget.NewSeparator())
		}
		apartados = append(apartados, p.ajustesAnclas(an)...)
	}
	return container.NewVBox(apartados...)
}

// notaAjustes es un texto de ayuda de los Ajustes: en gris y partido en líneas.
func notaAjustes(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping, l.Importance = fyne.TextWrapWord, widget.LowImportance
	return l
}

// tituloAjustes es el rótulo en negrita de un apartado de los Ajustes.
func tituloAjustes(s string) *widget.Label {
	return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// ajustesSello es el apartado de la autoridad de sellado: la elegida, y añadir otra por
// su dirección.
func (p *Pantalla) ajustesSello(aj *AjusteSello) []fyne.CanvasObject {
	actual, lista := aj.Leer()
	rotulos := make([]string, 0, 1+len(lista))
	rotulos = append(rotulos, p.propio("selloNinguno"))
	elegida := 0
	for i, au := range lista {
		rotulos = append(rotulos, cmpOr(au.Nombre, au.URL))
		if au.URL == actual {
			elegida = i + 1
		}
	}
	sel := widget.NewSelect(rotulos, nil)
	sel.SetSelectedIndex(elegida)
	sel.OnChanged = func(string) {
		nueva := ""
		if i := sel.SelectedIndex(); i > 0 {
			nueva = lista[i-1].URL
		}
		aj.Poner(nueva, lista)
	}
	otra := widget.NewEntry()
	otra.SetPlaceHolder("https://")
	fallo := notaAjustes("")
	anadir := widget.NewButton(p.propio("selloAnadir"), func() {
		nueva, ok := direccionDeSello(otra.Text)
		if !ok {
			fallo.SetText(p.propio("selloMala"))
			return
		}
		fallo.SetText("")
		esta := false
		for _, au := range lista {
			esta = esta || au.URL == nueva
		}
		if !esta {
			u, _ := url.Parse(nueva)
			lista = append(lista, AutoridadSello{Nombre: u.Hostname(), URL: nueva})
			sel.Options = append(sel.Options, u.Hostname())
		}
		aj.Poner(nueva, lista)
		for i, au := range lista {
			if au.URL == nueva {
				sel.SetSelectedIndex(i + 1)
			}
		}
		otra.SetText("")
	})
	return []fyne.CanvasObject{
		tituloAjustes(p.propio("selloTitulo")),
		sel,
		notaAjustes(p.propio("selloAyuda")),
		widget.NewForm(widget.NewFormItem(p.propio("selloOtra"), otra)),
		container.NewHBox(anadir),
		fallo,
	}
}

// ajustesAnclas es el apartado de la carpeta de anclas: cuántos certificados hay en
// ella y elegir otra.
func (p *Pantalla) ajustesAnclas(an *AjusteAnclas) []fyne.CanvasObject {
	texto := notaAjustes("")
	pintar := func() {
		carpeta, n := an.Carpeta(), an.Cuantos()
		switch {
		case n == 1:
			texto.SetText(p.propio("anclasUno", "carpeta", carpeta))
		case n > 1:
			texto.SetText(p.propio("anclasCuantos", "n", strconv.Itoa(n), "carpeta", carpeta))
		default:
			texto.SetText(p.propio("anclasNinguno", "carpeta", carpeta))
		}
	}
	pintar()
	return []fyne.CanvasObject{
		tituloAjustes(p.propio("anclasTitulo")),
		texto,
		container.NewHBox(widget.NewButton(p.propio("anclasElegir"), func() { an.Elegir(pintar) })),
		notaAjustes(p.propio("revocacion")),
	}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
