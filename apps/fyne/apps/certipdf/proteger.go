// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// permisosPDF son los de `PERMISOS`, en su orden.
var permisosPDF = []string{"imprimir", "copiar", "modificar", "anotar", "rellenar", "ensamblar"}

// dialogoProteger es `Proteger`: contraseñas y permisos, la verdad sobre los permisos
// dicha antes, y lo que el fichero lleva encima (limpiarlo o corregirlo).
type dialogoProteger struct {
	a          *dialogoAbierto
	usuario    *ui.Entrada
	dueno      *ui.Entrada
	permitidos []string
	// metadatos son los cuatro campos de «Corregir las propiedades», por su nombre.
	metadatos map[string]*ui.Entrada
}

func nuevoDialogoProteger(a *dialogoAbierto) *dialogoProteger {
	return &dialogoProteger{a: a, permitidos: append([]string{}, permisosPDF...)}
}

// camposDeMetadatos son los del formulario de propiedades: nombre, clave y valor.
func (d *dialogoProteger) camposDeMetadatos() [][3]string {
	h := d.a.p.Contenido.Huella
	return [][3]string{
		{"titulo", "pdf.meta.field.title", h.Titulo},
		{"autor", "pdf.meta.field.author", h.Autor},
		{"asunto", "pdf.meta.field.subject", h.Asunto},
		{"palabras", "pdf.meta.field.keywords", h.Palabras},
	}
}

func (d *dialogoProteger) vista(ancho float32) fyne.CanvasObject {
	a := d.a
	c := a.p.Contenido
	partes := make([]fyne.CanvasObject, 0, 3)
	if c.Cifrado {
		partes = append(partes, d.yaProtegido(ancho))
	}
	if d.usuario == nil {
		d.usuario = entradaDialogo("", ancho)
		d.usuario.Contrasena(true)
		d.dueno = entradaDialogo("", ancho)
		d.dueno.Contrasena(true)
	}
	casillas := make([]fyne.CanvasObject, len(permisosPDF))
	for i, p := range permisosPDF {
		permiso := p
		k := nuevaCasillaNativa(a.t("pdf.permission."+p), d.permite(p), func(si bool) { d.alternar(permiso, si) })
		k.esc, k.hueco, k.tinta = tema.TextoXS, tema.Espacio(1.5), "text"
		casillas[i] = k
	}
	formulario := container.New(ui.Columna{Hueco: tema.Espacio(3)},
		conRotulo(a.t("pdf.dialog.proteger.userPassword"), d.usuario, notaDialogo(a.t("pdf.dialog.proteger.userHelp"), ancho)),
		conRotulo(a.t("pdf.dialog.proteger.ownerPassword"), d.dueno),
		// `<legend class="mb-1">` y las casillas en `flex-wrap gap-2`.
		container.New(ui.Columna{Hueco: tema.Espacio(1)},
			ui.NuevoRotulo(a.t("pdf.dialog.proteger.allow"), tema.TextoXS, ui.Normal, "text-muted"),
			container.New(&filaQueEnvuelve{hueco: tema.Espacio(2), ancho: ancho}, casillas...)),
		// La verdad, dicha antes y no en la letra pequeña.
		avisoAmbar(a.t("pdf.dialog.proteger.truth"), ancho),
	)
	a.botonesDialogo(d.proteger)
	partes = append(partes, formulario, d.loQueLlevaEncima(ancho))
	return container.New(ui.Columna{Hueco: tema.Espacio(4)}, partes...)
}

// yaProtegido es la banda informativa de un documento cifrado, con «Quitar la
// protección»: `bg-info-bg text-info-text flex justify-between gap-3 rounded-md p-2`.
func (d *dialogoProteger) yaProtegido(ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a := d.a
	quitar := NuevaAccion(a.t("pdf.dialog.proteger.remove"), func() {
		a.aplicar(api.PDFCambio{Op: "desproteger", Sufijo: a.t("pdf.suffix.unprotected")})
	})
	quitar.Peso, quitar.PX, quitar.PY, quitar.Tinta, quitar.FondoEncima, quitar.Subrayado = ui.Normal, 0, 0, "info-text", "", true
	quitar.Inerte = a.p.Trabajando
	fondo := canvas.NewRectangle(tema.Color("info-bg", v))
	fondo.CornerRadius = tema.RadioMD
	p := tema.Espacio(2)
	fila := container.New(aLosLados{}, ui.NuevoRotulo(a.t("pdf.dialog.proteger.already"), tema.TextoXS, ui.Normal, "info-text"), quitar)
	return container.NewStack(fondo, container.New(ui.Margen{Izq: p, Arr: p, Der: p, Abj: p}, container.New(anchoFijo{ancho: ancho - 2*p}, fila)))
}

// loQueLlevaEncima es la parte de los metadatos: `border-t pt-3`, lo que se sabe del
// fichero, «Quitar los metadatos» y el formulario para corregirlos.
func (d *dialogoProteger) loQueLlevaEncima(ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a := d.a
	h := a.p.Contenido.Huella
	lineas := make([]fyne.CanvasObject, 0, 4)
	if h.Autor != "" {
		lineas = append(lineas, ui.NuevoRotulo(a.p.tv("pdf.meta.author", i18n.Vars{"v": h.Autor}), tema.Texto2XS, ui.Normal, "text-subtle"))
	}
	if h.Creador != "" {
		lineas = append(lineas, ui.NuevoRotulo(a.p.tv("pdf.meta.creator", i18n.Vars{"v": h.Creador}), tema.Texto2XS, ui.Normal, "text-subtle"))
	}
	if h.ConJS {
		lineas = append(lineas, ui.NuevoRotulo(a.t("pdf.meta.js"), tema.Texto2XS, ui.Normal, "warning-text"))
	}
	if h.ConAdjuntos {
		lineas = append(lineas, ui.NuevoRotulo(a.t("pdf.meta.attachments"), tema.Texto2XS, ui.Normal, "text-subtle"))
	}
	limpiar := botonPanel(a.t("pdf.dialog.proteger.clean"), tema.TextoXS, tema.Espacio(1.5), func() {
		a.aplicar(api.PDFCambio{Op: "sanear", Sufijo: a.t("pdf.suffix.cleaned")})
	})
	limpiar.Inerte = a.p.Trabajando

	if d.metadatos == nil {
		d.metadatos = map[string]*ui.Entrada{}
		for _, m := range d.camposDeMetadatos() {
			e := nuevoCampoPanel("", tema.Espacio(1), tema.RadioMD, ancho)
			e.PonerTexto(m[2])
			// Es un `<form>` aparte dentro del diálogo: Intro guarda las propiedades,
			// no protege el documento.
			conIntro(e, func() {
				if !d.a.p.Trabajando {
					d.guardarMetadatos()
				}
			})
			d.metadatos[m[0]] = e
		}
	}
	metas := d.camposDeMetadatos()
	campos := make([]fyne.CanvasObject, 0, len(metas)+3)
	campos = append(campos, ui.NuevoRotulo(a.t("pdf.meta.editTitle"), tema.TextoXS, ui.Normal, "text-muted"))
	for _, m := range metas {
		campos = append(campos, container.New(ui.Columna{Hueco: tema.Espacio(0.5)},
			ui.NuevoRotulo(a.t(m[1]), tema.Texto2XS, ui.Normal, "text-muted"), d.metadatos[m[0]]))
	}
	guardar := botonPanel(a.t("pdf.meta.save"), tema.TextoXS, tema.Espacio(1.5), d.guardarMetadatos)
	guardar.Inerte = a.p.Trabajando
	campos = append(campos, notaDialogo(a.t("pdf.meta.emptyClears"), ancho), container.New(alPrincipio{}, guardar))

	dentro := container.New(ui.Columna{},
		container.New(ui.Margen{Abj: tema.Espacio(2)}, ui.NuevoRotulo(a.t("pdf.dialog.proteger.metadata"), tema.TextoXS, ui.Normal, "text-muted")),
		container.New(ui.Margen{Abj: tema.Espacio(2)}, container.New(ui.Columna{}, lineas...)),
		container.New(alPrincipio{}, limpiar),
		container.New(ui.Margen{Arr: tema.Espacio(3)}, container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, campos...)),
	)
	return container.New(conLineaArriba{sep: tema.Espacio(3)}, canvas.NewRectangle(tema.Color("border", v)), dentro)
}

func (d *dialogoProteger) permite(p string) bool {
	for _, x := range d.permitidos {
		if x == p {
			return true
		}
	}
	return false
}

// alternar es el `onChange` de una casilla: al marcar, el permiso va al FINAL de la
// lista, como el `[...prev, p]` de la web.
func (d *dialogoProteger) alternar(p string, si bool) {
	r := make([]string, 0, len(d.permitidos)+1)
	for _, x := range d.permitidos {
		if x != p {
			r = append(r, x)
		}
	}
	if si {
		r = append(r, p)
	}
	d.permitidos = r
}

// cambioProteger es lo que manda «Aplicar»: las dos contraseñas y los permisos.
func (d *dialogoProteger) cambioProteger() api.PDFCambio {
	return api.PDFCambio{Op: "proteger", Campos: map[string]string{
		"usuario": d.usuario.Texto(), "dueno": d.dueno.Texto(), "permisos": strings.Join(d.permitidos, ","),
	}, Sufijo: d.a.t("pdf.suffix.protected")}
}

func (d *dialogoProteger) proteger() { d.a.aplicar(d.cambioProteger()) }

// cambioMetadatos son las cuatro propiedades tal como están escritas: una vacía se borra.
func (d *dialogoProteger) cambioMetadatos() api.PDFCambio {
	datos := map[string]string{}
	for nombre, e := range d.metadatos {
		datos[nombre] = e.Texto()
	}
	return api.PDFCambio{Op: "metadatos", Datos: datos}
}

// guardarMetadatos sobrescribe (no deja copia) y lo dice con su frase.
func (d *dialogoProteger) guardarMetadatos() {
	a := d.a
	a.p.Aplicar(d.cambioMetadatos(), a.t("pdf.meta.saved"), func(bool) { a.cerrar() })
	a.rehacer()
}
