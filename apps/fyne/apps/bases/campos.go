// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Gestionar un campo desde su cabecera (el menú ▾ de Rejilla.tsx y los diálogos de
// Dialogos.tsx): renombrarlo, cambiar su fórmula y borrarlo. Solo el propietario, que es
// quien puede cambiar el esquema; el servicio lo vuelve a comprobar.
//
// Los tres dejan el error del servicio DENTRO del diálogo, que sigue abierto con lo
// escrito: un nombre repetido o una fórmula que el servicio no acepta se arreglan ahí
// mismo, sin volver a abrir nada. Si sale bien se recargan la estructura (los nombres y
// las fórmulas) y los registros (los valores recalculados o ya sin ese campo).

// accionCampo es lo que se hace con un campo desde su cabecera.
type accionCampo int

const (
	accionRenombrar accionCampo = iota
	accionFormula
	accionOpciones
	accionDefecto
	accionBorrar
)

// alCampo es lo que la rejilla llama al elegir una acción del menú de una cabecera.
func (p *Pantalla) alCampo(accion accionCampo, campo api.BasesField) {
	switch accion {
	case accionRenombrar:
		p.dialogoRenombrarCampo(campo)
	case accionFormula:
		p.dialogoFormulaCampo(campo)
	case accionOpciones:
		p.dialogoOpcionesCampo(campo)
	case accionDefecto:
		p.dialogoDefectoCampo(campo)
	case accionBorrar:
		p.dialogoBorrarCampo(campo)
	}
}

// recargarCampos es lo que sigue a cambiar el esquema de la tabla.
func (p *Pantalla) recargarCampos() {
	p.cargarEstructura()
	p.cargarRegistros("inicio")
}

// dialogoRenombrarCampo es un diálogo propio y no `preguntar`: así el error del servicio
// se lee en él, que sigue abierto con lo escrito.
func (p *Pantalla) dialogoRenombrarCampo(campo api.BasesField) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	nombre := ui.NuevaEntrada("")
	nombre.Rotulo = p.t("field.name")
	nombre.PonerTexto(campo.Name)
	fallo := ""
	cuerpo := func() fyne.CanvasObject {
		hijos := []fyne.CanvasObject{ui.GrupoCampo(p.t("field.name"), true, nombre, "", "")}
		if fallo != "" {
			hijos = append(hijos, alerta(fallo, v))
		}
		return container.New(ui.Columna{Hueco: tema.Espacio(4)}, hijos...)
	}
	escrito := func() (string, bool) {
		n := strings.TrimSpace(nombre.Texto())
		return n, n != "" && n != campo.Name
	}
	var d *ui.Dialogo
	var guardar *ui.Boton
	enviar := func() {
		n, vale := escrito()
		if !vale {
			return
		}
		p.enDialogo(d, func() error {
			ctx, cancelar := p.ctx()
			defer cancelar()
			return p.svc.RenameField(ctx, campo.ID, n)
		}, func(m string) { fallo = m; d.CambiarCuerpo(cuerpo()) }, p.recargarCampos)
	}
	d, guardar = p.dialogo(p.t("field.renameTitle", i18n.Vars{"name": campo.Name}), cuerpo(), p.t("common.save"), enviar)
	ajustar := func() {
		_, vale := escrito()
		guardar.Inerte = !vale
		guardar.Refresh()
	}
	nombre.AlCambiar(func(string) { ajustar() })
	nombre.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		enviar()
		return true
	}
	ajustar()
	d.Foco = nombre
	d.Mostrar(p.ventana.Canvas())
}

// dialogoFormulaCampo cambia la expresión de un campo de fórmula con el mismo editor que
// «Añadir campo» (formCampo): se escribe con nombres —la guardada se enseña con los de
// ahora— y se envía con ids, solo si cambia. El tipo del resultado no se edita (la web
// tampoco: el servicio no deja cambiarlo).
func (p *Pantalla) dialogoFormulaCampo(campo api.BasesField) {
	b := p.abierta
	f := &formCampo{p: p, b: b, editar: &campo}
	f.formula = ui.NuevaAreaTexto("")
	f.formula.Rotulo, f.formula.Filas = p.t("field.kind.formula"), 3
	f.formula.PonerTexto(formulaANombres(campo.Formula, b.Campos, b.Tablas))
	f.d, f.crear = p.dialogo(p.t("field.formulaTitle", i18n.Vars{"name": campo.Name}), container.New(ui.Columna{}), p.t("common.save"), f.enviar)
	f.vigilarFormula()
	f.d.Foco = f.formula
	f.rehacer()
	f.d.Mostrar(p.ventana.Canvas())
}

// dialogoOpcionesCampo edita las opciones de un campo de selección, una por línea y en el
// orden en que se ofrecen (DialogoOpcionesCampo de Dialogos.tsx). Antes de guardar dice
// qué opciones quedan retiradas: los registros que las usan las conservan.
func (p *Pantalla) dialogoOpcionesCampo(campo api.BasesField) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	area := ui.NuevaAreaTexto("")
	area.Rotulo, area.Filas = p.t("field.choices"), 6
	area.PonerTexto(strings.Join(campo.Choices, "\n"))
	fallo := ""
	escrito := func() ([]string, bool) {
		l := listaDeOpciones(area.Texto())
		return l, len(l) > 0 && !slices.Equal(l, campo.Choices)
	}
	cuerpo := func() fyne.CanvasObject {
		hijos := []fyne.CanvasObject{ui.GrupoCampo(p.t("field.choices"), true, area, p.t("field.choicesEditHelp"), "")}
		l, _ := escrito()
		if r := opcionesQueSeRetiran(campo.Choices, l); len(r) > 0 {
			hijos = append(hijos, ui.NuevoParrafo(p.t("field.choicesRetiring", i18n.Vars{"options": strings.Join(r, ", ")}), tema.TextoSM, ui.Normal, "text-muted", v).A(p.anchoDialogo(400)))
		}
		if fallo != "" {
			hijos = append(hijos, alerta(fallo, v))
		}
		return container.New(ui.Columna{Hueco: tema.Espacio(4)}, hijos...)
	}
	var d *ui.Dialogo
	var guardar *ui.Boton
	enviar := func() {
		l, vale := escrito()
		if !vale {
			return
		}
		p.enDialogo(d, func() error {
			ctx, cancelar := p.ctx()
			defer cancelar()
			return p.svc.SetFieldChoices(ctx, campo.ID, l)
		}, func(m string) { fallo = m; d.CambiarCuerpo(cuerpo()) }, p.recargarCampos)
	}
	d, guardar = p.dialogo(p.t("field.choicesTitle", i18n.Vars{"name": campo.Name}), cuerpo(), p.t("common.save"), enviar)
	retiradasAntes := 0
	area.AlCambiar(func(string) {
		l, vale := escrito()
		guardar.Inerte = !vale
		guardar.Refresh()
		// El aviso de las retiradas aparece y desaparece: solo entonces se rehace el
		// cuerpo, que si no el área perdería el foco a cada letra.
		if n := len(opcionesQueSeRetiran(campo.Choices, l)); n != retiradasAntes {
			retiradasAntes = n
			d.CambiarCuerpo(cuerpo())
		}
	})
	_, vale := escrito()
	guardar.Inerte = !vale
	d.Foco = area
	d.Mostrar(p.ventana.Canvas())
}

// moverCampo pone `campo` en el sitio de `destino`. Se pinta ya —la columna no espera
// a la red— y se manda el orden de TODOS los campos de la tabla, no solo de los que se
// ven; si el servicio lo rechaza (otra persona cambió los campos), se recarga.
func (p *Pantalla) moverCampo(campo, destino api.BasesField) {
	b := p.abierta
	if b == nil {
		return
	}
	todos := b.Campos
	if t := b.tabla(); t != nil && len(t.Fields) > 0 {
		todos = t.Fields
	}
	ids := make([]string, len(todos))
	for i, c := range todos {
		ids[i] = c.ID
	}
	orden := moverCampoEn(ids, campo.ID, destino.ID)
	ordenarCampos(b.Campos, orden)
	if t := b.tabla(); t != nil {
		ordenarCampos(t.Fields, orden)
	}
	b.rejilla.Poner(b.Campos, b.rejilla.Registros)
	tabla := b.Activa
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		if err := p.svc.ReorderFields(ctx, tabla, orden); err != nil {
			enHilo(func() {
				b.rejilla.Aviso = p.mensajeDeError(err)
				p.recargarCampos()
			})
		}
	})
}

// dialogoBorrarCampo avisa de que borrar se lleva los valores de todos los registros. Si
// una fórmula usa el campo (aquí o a través de un enlace), dice cuál y no deja borrar: el
// servicio lo niega con un «referenciado» genérico que no dice qué lo usa.
func (p *Pantalla) dialogoBorrarCampo(campo api.BasesField) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	usan := formulasQueUsan(campo, p.abierta.Tablas)
	mensaje := p.t("field.deleteConfirm", i18n.Vars{"name": campo.Name})
	if len(usan) > 0 {
		mensaje = p.t("field.deleteBlocked", i18n.Vars{"name": campo.Name, "formulas": strings.Join(usan, ", ")})
	}
	fallo := ""
	cuerpo := func() fyne.CanvasObject {
		hijos := []fyne.CanvasObject{ui.NuevoParrafo(mensaje, tema.TextoSM, ui.Normal, "text", v).A(p.anchoDialogo(400))}
		if fallo != "" {
			hijos = append(hijos, alerta(fallo, v))
		}
		return container.New(ui.Columna{Hueco: tema.Espacio(3)}, hijos...)
	}
	var d *ui.Dialogo
	cancelar := ui.NuevoBoton(p.t("common.cancel"), func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	borrar := ui.NuevoBoton(p.t("common.delete"), func() {
		if len(usan) > 0 {
			return
		}
		p.enDialogo(d, func() error {
			ctx, cancelar := p.ctx()
			defer cancelar()
			return p.svc.DeleteField(ctx, campo.ID)
		}, func(m string) { fallo = m; d.CambiarCuerpo(cuerpo()) }, p.recargarCampos)
	}).Con(ui.Peligro, ui.TallaMD)
	borrar.Inerte = len(usan) > 0
	d = ui.NuevoDialogo(p.t("field.delete"), cuerpo()).Con(ui.DialogoSM, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, borrar))
	d.RotuloCerrar = p.t("ui.close")
	d.Mostrar(p.ventana.Canvas())
}
