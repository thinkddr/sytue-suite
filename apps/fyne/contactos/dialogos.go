// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los diálogos pequeños de Sytue Contacts: dar nombre a una etiqueta o a una libreta
// (`LabelModal` y `AddressBookModal`) y las confirmaciones de borrarlas (`useConfirm`).
//
// Medido en producción: «Nueva etiqueta» y «Nueva libreta» son diálogos de 400 × 207 con
// un cuerpo de 87,5 (el rótulo, el campo y el `py-5`) y el pie de 60,5 con «Cancelar»
// (79) y «Crear» (58).

// anchoCuerpo es lo que le queda al cuerpo de un diálogo de esa talla en ese lienzo:
// el panel (su `max-w`, o el lienzo menos el `p-4`; en la hoja del teléfono, todo el
// ancho y sin filetes a los lados) sin los bordes ni el `px-6`. Es ui.AnchoCuerpoTalla.
func anchoCuerpo(lienzo fyne.Size, ancho float32) float32 {
	talla := ui.DialogoMD
	switch ancho {
	case 400:
		talla = ui.DialogoSM
	case 720:
		talla = ui.DialogoLG
	}
	return ui.AnchoCuerpoTalla(talla, lienzo)
}

// recolocar vuelve a medir y a centrar un diálogo abierto cuyo cuerpo ha cambiado de
// alto (una fila de correo más, la línea de un error): en la web lo hace el navegador,
// porque el panel no tiene alto fijo. Es `ui.Dialogo.Recolocar`.
func recolocar(_ fyne.Canvas, d *ui.Dialogo) {
	if d != nil {
		d.Recolocar()
	}
}

// pieDialogo es el de LabelModal y los suyos: «Cancelar» secundario y la acción
// primaria, a la derecha.
func pieDialogo(cancelar, accion *ui.Boton) fyne.CanvasObject {
	return container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, accion)
}

// dialogoNombre es LabelModal / AddressBookModal: un campo de nombre obligatorio, con
// Intro para guardar. `inicial` vacío es un alta («Crear»); si no, un cambio de nombre
// («Guardar»). `guardar` recibe el nombre ya recortado y avisa con `listo`.
func (p *Pantalla) dialogoNombre(lienzo fyne.Canvas, titulo, marca, obligatorio, inicial string,
	guardar func(nombre string, listo func(error)),
) *ui.Dialogo {
	campo := ui.NuevaEntrada(p.t(marca, nil))
	campo.PonerTexto(inicial)
	accion := "common.create"
	if inicial != "" {
		accion = "common.save"
	}
	var d *ui.Dialogo
	cuerpo := container.New(ui.Columna{})
	errorNombre := ""
	pintar := func() {
		campo.Invalida = errorNombre != ""
		campo.Refresh()
		cuerpo.Objects = []fyne.CanvasObject{ui.GrupoCampo(p.t("common.name", nil), true, campo, "", errorNombre)}
		cuerpo.Refresh()
		recolocar(lienzo, d)
	}
	cancelar := ui.NuevoBoton(p.t("common.cancel", nil), nil).Con(ui.Secundario, ui.TallaMD)
	crear := ui.NuevoBoton(p.t(accion, nil), nil).Con(ui.Primario, ui.TallaMD)
	enviar := func() {
		nombre := strings.TrimSpace(campo.Texto())
		if nombre == "" {
			errorNombre = p.t(obligatorio, nil)
			pintar()
			return
		}
		crear.Cargando, cancelar.Inerte = true, true
		crear.Refresh()
		cancelar.Refresh()
		guardar(nombre, func(err error) {
			crear.Cargando, cancelar.Inerte = false, false
			crear.Refresh()
			cancelar.Refresh()
			if err == nil {
				d.Cerrar()
			}
		})
	}
	cancelar.AlPulsar, crear.AlPulsar = func() { d.Descartar() }, enviar
	campo.AlCambiar(func(string) {
		if errorNombre != "" {
			errorNombre = ""
			pintar()
		}
	})
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			enviar()
			return true
		}
		return false
	}
	pintar()
	d = ui.NuevoDialogo(p.t(titulo, nil), cuerpo).Con(ui.DialogoSM, pieDialogo(cancelar, crear))
	d.RotuloCerrar = p.t("ui.close", nil)
	d.AlCerrar = func() {
		if !crear.Cargando {
			d.Cerrar()
		}
	}
	return d
}

// ParrafoConfirmacion es el `<p class="text-text text-sm leading-relaxed">` de
// useConfirm: `text-sm` con el interlineado de 1,625.
var ParrafoConfirmacion = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}

// confirmar es `useConfirm` con el tono de peligro: la pregunta en un párrafo y el
// verbo en rojo. `hacer` lanza la acción; el diálogo se cierra al pulsar, como allí.
func (p *Pantalla) confirmar(lienzo fyne.Canvas, titulo, mensaje, verbo string, hacer func()) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := anchoCuerpo(lienzo.Size(), 400)
	cuerpo := ui.NuevoParrafo(mensaje, ParrafoConfirmacion, ui.Normal, "text", v).A(ancho)
	var d *ui.Dialogo
	d = ui.Confirmacion(titulo, p.t("ui.cancel", nil), verbo, true, cuerpo,
		func() { d.Cerrar() },
		func() { d.Cerrar(); hacer() })
	d.RotuloCerrar = p.t("ui.close", nil)
	d.Mostrar(lienzo)
}

// botonAcciones es el disparador de LabelActions y BookActions: `ghost sm iconOnly`,
// con el glifo que toque y su nombre accesible.
func botonAcciones(rotulo string, glifo *iconos.Dibujo, icono iconos.Icono, tam float32) *ui.Boton {
	b := ui.NuevoBoton("", nil).Con(ui.Fantasma, ui.TallaSM)
	b.SoloIcono, b.Rotulo, b.Glifo, b.Icono, b.TamIcono = true, rotulo, glifo, icono, tam
	return b
}

// menuDe abre un menú con esos elementos bajo su disparador, alineado a la derecha.
func menuDe(disparador fyne.CanvasObject, rotulo string, elementos ...fyne.CanvasObject) {
	m := ui.NuevoMenu(elementos...)
	m.Rotulo = rotulo
	m.Mostrar(disparador)
}

// accionesEtiqueta es LabelActions: cambiar el nombre y eliminar la etiqueta por la que
// se filtra. Solo con red y con una etiqueta activa.
func (p *Pantalla) accionesEtiqueta() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	rotulo := p.t("label.actions", nil)
	b := botonAcciones(rotulo, Glifo(EtiquetaD, iconos.TallaMD, "text", v), iconos.Ninguno, iconos.TallaMD)
	b.AlPulsar = func() {
		renombrar := ui.NuevoElementoMenu(p.t("label.rename", nil), iconos.Ninguno, p.AlRenombrarEtiqueta)
		borrar := ui.NuevoElementoMenu(p.t("label.delete", nil), iconos.Ninguno, p.AlBorrarEtiqueta)
		borrar.Peligro = true
		menuDe(b, rotulo, renombrar, borrar)
	}
	return b
}

// accionesLibreta es BookActions: cambiar el nombre y, si no es la predeterminada (que el
// servicio protege), eliminarla. Sin id no se ofrece nada: mejor no enseñar el botón que
// enseñarlo y que falle.
func (p *Pantalla) accionesLibreta() fyne.CanvasObject {
	if p.Activa == nil || p.Activa.ID == "" {
		return nil
	}
	rotulo := p.t("book.actions", nil)
	b := botonAcciones(rotulo, nil, iconos.Usuario, iconos.TallaSM)
	predeterminada := p.Activa.IsDefault
	b.AlPulsar = func() {
		elementos := []fyne.CanvasObject{
			ui.NuevoElementoMenu(p.t("book.rename", nil), iconos.Ninguno, p.AlRenombrarLibreta),
		}
		if !predeterminada {
			borrar := ui.NuevoElementoMenu(p.t("book.delete", nil), iconos.Ninguno, p.AlBorrarLibreta)
			borrar.Peligro = true
			elementos = append(elementos, borrar)
		}
		menuDe(b, rotulo, elementos...)
	}
	return b
}

// botonES es VcardToolbar: «Importar / Exportar» con su chevron, y el menú con las
// cuatro opciones. Sale también sin red; lo que necesita red se apaga.
func (p *Pantalla) botonES() fyne.CanvasObject {
	b := ui.NuevoBoton(p.t("io.button", nil), nil).Con(ui.Secundario, ui.TallaSM)
	b.IconoDetras, b.TamIcono = iconos.ChevronAbajo, 14
	if p.ImportOcupado {
		// El propio botón lleva la cuenta: es donde mira quien acaba de pulsarlo.
		b.Cargando = true
		if p.Importando != "" {
			b.Texto = p.Importando
		}
	}
	b.AlPulsar = func() {
		sinImportar := p.SinConexion || p.Activa == nil || !p.PuedeImportar
		elemento := func(clave string, inerte bool, f func()) fyne.CanvasObject {
			e := ui.NuevoElementoMenu(p.t(clave, nil), iconos.Ninguno, f)
			e.Inerte = inerte || f == nil
			return e
		}
		menuDe(b, p.t("io.menuLabel", nil),
			elemento("io.importVcard", sinImportar, p.AlImportarVcard),
			elemento("io.importCsv", sinImportar, p.AlImportarCSV),
			elemento("io.exportVcard", p.Total == 0, p.AlExportarVcard),
			elemento("io.exportCsv", p.SinConexion, p.AlExportarCSV),
		)
	}
	return b
}
