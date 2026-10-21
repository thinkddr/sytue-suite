// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// anchoPanelProteger es el `w-64` de los dos paneles de protección.
var anchoPanelProteger = tema.Espacio(64)

// formProteger es lo que se va marcando en «Proteger hoja» antes de pulsarlo.
type formProteger struct {
	clave                 string
	sinSeleccion, formato bool
	permisos              Proteccion
}

// botonProtegerHoja es el escudo de `ProtectSheetMenu`, encendido con la hoja protegida.
func (lz *Lienzo) botonProtegerHoja(c *editor.Contexto) *editor.BotonCinta {
	var b *editor.BotonCinta
	b = editor.NuevoBotonCinta(iconos.EdEscudo, c.Traducir("data.protectSheet"), func() {
		lz.abrirProtegerHoja(c, b, &formProteger{})
	})
	b.Activo, b.Inerte = lz.Estado().Protegida, c.SoloLectura
	return b
}

// botonProtegerLibro es el libro de `ProtectWorkbookMenu`.
func (lz *Lienzo) botonProtegerLibro(c *editor.Contexto) *editor.BotonCinta {
	var b *editor.BotonCinta
	b = editor.NuevoBotonCinta(iconos.EdLibro, c.Traducir("data.protectBook"), func() { lz.abrirProtegerLibro(c, b) })
	b.Activo, b.Inerte = lz.Estado().ProtegidoLibro, c.SoloLectura
	return b
}

// abrirProtegerHoja es `ProtectSheetMenu`: ocultar la fórmula de la celda y, debajo,
// quitar la protección —con su contraseña, si la tiene— o ponerla con sus permisos.
func (lz *Lienzo) abrirProtegerHoja(c *editor.Contexto, disparador *editor.BotonCinta, f *formProteger) {
	var em *ui.Emergente
	// Cada cambio vuelve a abrir el panel con lo marcado (`g`). Desproteger tampoco
	// cierra: si la contraseña falla, el aviso lo dice y se reintenta; si acierta, el
	// panel pasa a ofrecer proteger.
	reabrir := func(g *formProteger) { em.Cerrar(); lz.abrirProtegerHoja(c, disparador, g) }
	st := lz.Estado()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	titulo := func(clave string) fyne.CanvasObject {
		return ui.NuevoParrafo(c.Traducir(clave), tema.TextoXS, ui.Medio, "text", v)
	}
	oculta := nuevaCasillaXS(c.Traducir("data.protect.hideFormula"), st.Estilo.Oculta, tema.Espacio(1.5), func(si bool) {
		b, _ := json.Marshal(map[string]bool{"hidden": si})
		c.Mandar(CmdEstilo, string(b))
		reabrir(f)
	})
	if st.Protegida {
		oculta.Inerte()
	}
	filas := []fyne.CanvasObject{oculta, container.New(bordeArriba{}, canvas.NewRectangle(tema.Color("border", v)), container.NewStack())}
	if st.Protegida {
		desproteger := func() {
			c.Mandar(CmdDesproteger, f.clave)
			reabrir(&formProteger{})
		}
		filas = append(filas, titulo("data.protect.sheetProtected"))
		if st.ConClave {
			filas = append(filas, conEtiqueta(c.Traducir("data.password"), campoClave(f, desproteger)))
		}
		filas = append(filas, anchoEntero(botonPop(c.Traducir("data.protect.unprotectSheet"), false, desproteger)))
	} else {
		casilla := func(clave string, marcada bool, poner func(bool)) fyne.CanvasObject {
			return nuevaCasillaXS(c.Traducir(clave), marcada, tema.Espacio(1.5), func(si bool) { poner(si); reabrir(f) })
		}
		permisos := make([]fyne.CanvasObject, 0, len(Permisos))
		for _, p := range Permisos {
			campo := p.Campo(&f.permisos)
			permisos = append(permisos, casilla(p.Rotulo, *campo, func(si bool) { *campo = si }))
		}
		proteger := botonPop(c.Traducir("data.protectSheet"), false, func() {
			p := f.permisos
			p.Password, p.FormatCells = f.clave, f.formato
			if f.sinSeleccion {
				no := false
				p.SelectCells = &no
			}
			b, _ := json.Marshal(p) //nolint:gosec // la contraseña es un cotejo en claro, como en la web (ver Proteccion)
			c.Mandar(CmdProteger, string(b))
			em.Cerrar()
		})
		filas = append(filas, titulo("data.protectSheet"),
			conEtiqueta(c.Traducir("data.passwordOptional"), campoClave(f, nil)),
			casilla("data.perm.selectCells", !f.sinSeleccion, func(si bool) { f.sinSeleccion = !si }),
			casilla("data.perm.formatCells", f.formato, func(si bool) { f.formato = si }),
			titulo("data.validation.allowAlso"),
			recuadro(topeAlto(tema.Espacio(32), container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, permisos...))),
			anchoEntero(proteger))
	}
	em = abrirEmergente(disparador, panel{ancho: anchoPanelProteger, hueco: tema.Espacio(1.5), filas: filas}.objeto())
}

// abrirProtegerLibro es `ProtectWorkbookMenu`: proteger la estructura del libro, con
// contraseña opcional, o quitarle la protección.
func (lz *Lienzo) abrirProtegerLibro(c *editor.Contexto, disparador *editor.BotonCinta) {
	var em *ui.Emergente
	st := lz.Estado()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f := &formProteger{}
	texto := func(clave string, e tema.Escalon, p ui.Peso, token string) fyne.CanvasObject {
		return ui.NuevoParrafo(c.Traducir(clave), e, p, token, v)
	}
	var filas []fyne.CanvasObject
	if st.ProtegidoLibro {
		desproteger := func() {
			c.Mandar(CmdDesprotegerLibro, f.clave)
			em.Cerrar()
			if lz.Libro.ProteccionLibro() != nil {
				lz.abrirProtegerLibro(c, disparador) // la contraseña no era: se reintenta
			}
		}
		filas = append(filas, texto("data.protect.bookProtected", tema.TextoXS, ui.Medio, "text"),
			texto("data.protect.bookLocked", tema.Texto2XS, ui.Normal, "text-subtle"))
		if st.ConClaveLibro {
			filas = append(filas, conEtiqueta(c.Traducir("data.password"), campoClave(f, desproteger)))
		}
		filas = append(filas, anchoEntero(botonPop(c.Traducir("data.protect.unprotectBook"), false, desproteger)))
	} else {
		filas = append(filas, texto("data.protectBook", tema.TextoXS, ui.Medio, "text"),
			texto("data.protect.lockStructure", tema.Texto2XS, ui.Normal, "text-subtle"),
			conEtiqueta(c.Traducir("data.passwordOptional"), campoClave(f, nil)),
			anchoEntero(botonPop(c.Traducir("data.protectBook"), false, func() {
				b, _ := json.Marshal(ProteccionLibro{Password: f.clave}) //nolint:gosec // ídem
				c.Mandar(CmdProtegerLibro, string(b))
				em.Cerrar()
			})))
	}
	em = abrirEmergente(disparador, panel{ancho: anchoPanelProteger, hueco: tema.Espacio(1.5), filas: filas}.objeto())
}

// campoClave es el `<input type="password">` del panel; Intro hace `alIntro`, si hay.
func campoClave(f *formProteger, alIntro func()) *ui.Entrada {
	e := ui.NuevaEntrada("").ConPropia(campoPanel)
	e.Contrasena(true)
	e.PonerTexto(f.clave)
	e.AlCambiar(func(s string) { f.clave = s })
	if alIntro != nil {
		e.AlTeclear = func(k *fyne.KeyEvent) bool {
			if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
				return false
			}
			alIntro()
			return true
		}
	}
	return e
}

// conEtiqueta es `<label class="block"><span class="sytue-meta-dense mb-0.5 block">`: el
// rótulo pequeño encima de su campo.
func conEtiqueta(rotulo string, o fyne.CanvasObject) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, ui.NuevoParrafo(rotulo, tema.Texto2XS, ui.Normal, "text-subtle", v), o)
}

// anchoEntero estira un botón a lo ancho del panel (`w-full`).
func anchoEntero(b fyne.CanvasObject) fyne.CanvasObject { return container.NewStack(b) }

// bloqueoPropio es la franja de la hoja protegida (`FranjaHojaProtegida.tsx`): el caso
// (a) de la franja de solo lectura. La protección es de quien edita, así que «Editar»
// la quita al momento, o pide antes la contraseña si la tiene; una contraseña que no es
// la dice `mandarProteccion` con su aviso de siempre.
func (lz *Lienzo) bloqueoPropio(c *editor.Contexto) (string, func()) {
	st := lz.Estado()
	if !st.Protegida {
		return "", nil
	}
	return c.Traducir("readOnly.protectedSheet"), func() {
		if !st.ConClave {
			c.Mandar(CmdDesproteger, "")
			return
		}
		f := &formProteger{}
		var d *ui.Dialogo
		quitar := func() { d.Cerrar(); c.Mandar(CmdDesproteger, f.clave) }
		// Título «Editar», como la web (`usePrompt` con `titulo`): la frase entera de
		// «Esta hoja está protegida…» no es un título.
		d = ui.Confirmacion(c.Traducir("readOnly.edit"), c.Traducir("ui.cancel"), c.Traducir("readOnly.edit"), false,
			conEtiqueta(c.Traducir("readOnly.password"), campoClave(f, quitar)), nil, quitar)
		if lienzo := fyne.CurrentApp().Driver().CanvasForObject(lz.Objeto()); lienzo != nil {
			d.Mostrar(lienzo)
		}
	}
}
