// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El valor por defecto de un campo (`lib/defectos.ts` y `DialogoDefectoCampo`)
//
// Con el que nace un registro que no lo trae: un alta desde la rejilla, una tarjeta del
// tablero, una respuesta de formulario. Lo aplica el servicio; aquí solo se decide qué
// se escribe en el diálogo y qué se envía. Lo de una fecha puede ser «el día del alta»
// (`api.BasesDefaultToday`) en vez de un día fijo.

// admiteDefecto: solo los campos que se escriben; un enlace por defecto ataría cada alta
// a un registro, una fórmula se calcula sola y un fichero no se copia.
func admiteDefecto(c api.BasesField) bool {
	return c.Kind != api.BasesKindLink && c.Kind != api.BasesKindFormula && c.Kind != api.BasesKindAttachment
}

// borradorDefecto es lo que lleva escrito el diálogo (`BorradorDefecto`).
type borradorDefecto struct {
	hoy     bool
	texto   string
	marcado bool
	varios  []string
}

// borradorDeDefecto abre el diálogo con el defecto que el campo ya tiene.
func borradorDeDefecto(c api.BasesField) borradorDefecto {
	b := borradorDefecto{marcado: c.Default == true, varios: cadenasDe(c.Default)}
	switch {
	case c.Kind == api.BasesKindDate && c.Default == api.BasesDefaultToday:
		b.hoy = true
	case c.Kind != api.BasesKindCheckbox && c.Kind != api.BasesKindMultiselect:
		b.texto = textoParaEditar(c, c.Default)
	}
	if c.Kind != api.BasesKindMultiselect {
		b.varios = nil
	}
	return b
}

// valorDeDefecto es lo que se envía: el defecto, nil para quitarlo, u `ok` falso si lo
// escrito no encaja en el campo (no se deja guardar).
func valorDeDefecto(c api.BasesField, b borradorDefecto) (valor any, ok bool) {
	switch c.Kind {
	case api.BasesKindCheckbox:
		if b.marcado {
			return true, true
		}
		return nil, true
	case api.BasesKindMultiselect:
		if len(b.varios) > 0 {
			return b.varios, true
		}
		return nil, true
	case api.BasesKindDate:
		if b.hoy {
			return api.BasesDefaultToday, true
		}
	}
	return valorEscrito(c, b.texto)
}

// dialogoDefectoCampo fija el valor por defecto de un campo (solo el propietario). No
// toca los registros que ya hay, y la ayuda lo dice: es lo primero que alguien pregunta
// al ver que la columna sigue vacía.
func (p *Pantalla) dialogoDefectoCampo(campo api.BasesField) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := borradorDeDefecto(campo)
	fallo := ""
	entrada := ui.NuevaEntrada("")
	entrada.Rotulo = campo.Name
	entrada.PonerTexto(b.texto)
	var d *ui.Dialogo
	var guardar *ui.Boton
	ajustar := func() {
		_, vale := valorDeDefecto(campo, b)
		guardar.Inerte = !vale
		guardar.Refresh()
	}
	control := func() fyne.CanvasObject {
		switch campo.Kind {
		case api.BasesKindCheckbox:
			c := ui.NuevaCasilla(p.t("view.checked"), func(si bool) { b.marcado = si; ajustar() })
			c.Marcada = b.marcado
			return c
		case api.BasesKindMultiselect:
			hijos := make([]fyne.CanvasObject, 0, len(campo.Choices))
			for _, o := range campo.Choices {
				c := ui.NuevaCasilla(o, func(si bool) {
					b.varios = slices.DeleteFunc(b.varios, func(x string) bool { return x == o })
					if si {
						b.varios = append(b.varios, o)
					}
					ajustar()
				})
				c.Marcada = slices.Contains(b.varios, o)
				hijos = append(hijos, c)
			}
			return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, hijos...)
		case api.BasesKindSelect:
			opciones := append([]string{p.t("view.none")}, campo.Choices...)
			s := ui.NuevoSelector(opciones, func(i int) {
				b.texto = ""
				if i > 0 {
					b.texto = opciones[i]
				}
				ajustar()
			})
			s.Rotulo, s.Elegida = campo.Name, max(slices.Index(campo.Choices, b.texto)+1, 0)
			return s
		case api.BasesKindDate:
			hoy := ui.NuevaCasilla(p.t("field.defaultToday"), func(si bool) {
				b.hoy = si
				entrada.Inerte(si)
				ajustar()
			})
			hoy.Marcada = b.hoy
			entrada.Inerte(b.hoy)
			return container.New(ui.Columna{Hueco: tema.Espacio(2)}, hoy, entrada)
		}
		return entrada
	}
	cuerpo := func() fyne.CanvasObject {
		hijos := []fyne.CanvasObject{ui.GrupoCampo(campo.Name, false, control(), p.t("field.defaultHelp"), "")}
		if fallo != "" {
			hijos = append(hijos, alerta(fallo, v))
		}
		return container.New(ui.Columna{Hueco: tema.Espacio(4)}, hijos...)
	}
	enviar := func() {
		valor, vale := valorDeDefecto(campo, b)
		if !vale {
			return
		}
		p.enDialogo(d, func() error {
			ctx, cancelar := p.ctx()
			defer cancelar()
			return p.svc.SetFieldDefault(ctx, campo.ID, valor)
		}, func(m string) { fallo = m; d.CambiarCuerpo(cuerpo()) }, p.recargarCampos)
	}
	d, guardar = p.dialogo(p.t("field.defaultTitle", i18n.Vars{"name": campo.Name}), cuerpo(), p.t("common.save"), enviar)
	entrada.AlCambiar(func(s string) { b.texto = s; ajustar() })
	ajustar()
	if campo.Kind != api.BasesKindCheckbox && campo.Kind != api.BasesKindMultiselect && campo.Kind != api.BasesKindSelect {
		d.Foco = entrada
	}
	d.Mostrar(p.ventana.Canvas())
}
