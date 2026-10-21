// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: «Importar una copia…» —`importar` y `confirmarImportacion` de
// NotesApp.tsx, con `NotasSoloLectura` de PublicNotebook.tsx—.
//
// Importar SIEMPRE enseña antes lo que trae, y lo enseña ENTERO: un volcado llega por
// correo o de una copia de hace meses, y «Mudanza, 40 notas» no dice si es la copia buena.
// Lo que se pinta es el ensayo que devuelve el servicio (ya saneado), no el fichero. Y lo
// que no va a entrar se dice ANTES, cuando todavía se puede no importar.

// Medidas de la vista previa, de la hoja.
const (
	// RellenoNotaLectura es el `p-4` de cada nota de solo lectura.
	RellenoNotaLectura = 4 * 0.25 * tema.Rem
)

// importar elige el fichero con el selector del sistema, lo lee y enseña su vista previa.
func (p *Pantalla) importar() {
	if p.svc == nil || !p.hayRed() {
		return
	}
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil || len(rutas) == 0 {
			return // cancelar el selector no es un error
		}
		if info, err := os.Stat(rutas[0]); err != nil || info.Size() > api.MaxVolcado {
			p.avisar(ui.AvisoPeligro, p.tr.T("import.failed", nil), "")
			return
		}
		volcado, err := os.ReadFile(rutas[0])
		if err != nil {
			p.avisar(ui.AvisoPeligro, p.tr.T("import.failed", nil), "")
			return
		}
		ctx, cancelar := p.ctx()
		defer cancelar()
		vp, err := p.svc.PreviewNotebook(ctx, volcado)
		if err != nil {
			p.avisar(ui.AvisoPeligro, p.tr.T("import.failed", nil), "")
			return
		}
		enHilo(func() { p.dialogoImportar(volcado, vp) })
	}()
}

// dialogoImportar es el modal `lg` con el cuaderno tal como quedaría.
func (p *Pantalla) dialogoImportar(volcado []byte, vp api.VistaPreviaVolcado) {
	if p.win == nil {
		return
	}
	t, v := p.tr.T, p.variante()
	titulo := vp.Notebook.Title
	if titulo == "" {
		titulo = t("notebook.untitled", nil)
	}
	cabecera := []fyne.CanvasObject{}
	if vp.Notebook.Emoji != "" {
		cabecera = append(cabecera, lineaTok(vp.Notebook.Emoji, tema.TextoLG, ui.SemiNegrita, "text", v))
	}
	cabecera = append(cabecera, lineaTok(titulo, tema.TextoLG, ui.SemiNegrita, "text", v))
	arriba := []fyne.CanvasObject{
		container.New(ui.Fila{Hueco: 2 * 0.25 * tema.Rem}, cabecera...),
		// Parte de línea como en la web: en una, en el teléfono se cortaba a media frase.
		ui.NuevoParrafo(t("import.preview.count", i18n.Vars{"count": len(vp.Notas)}), tema.TextoXS, ui.Normal, "text-subtle", v),
	}
	if vp.Descartadas > 0 {
		arriba = append(arriba, lineaTok(t("import.preview.discarded", i18n.Vars{"count": vp.Descartadas}),
			tema.TextoBase, ui.Normal, "warning-text", v))
	}
	cuerpo := container.New(ui.Columna{Hueco: 3 * 0.25 * tema.Rem},
		container.New(ui.Columna{Hueco: 1 * 0.25 * tema.Rem}, arriba...),
		p.notasSoloLectura(DeNotas(vp.Notas), v))

	var d *ui.Dialogo
	cancelar := ui.NuevoBoton(t("common.cancel", nil), func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	importar := ui.NuevoBoton(t("import.action", nil), nil)
	importar.AlPulsar = func() {
		d.Cerrar()
		go p.confirmarImportacion(volcado)
	}
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, importar)
	d = ui.NuevoDialogo(t("import.label", nil), cuerpo).Con(ui.DialogoLG, pie)
	p.mostrar(d)
}

// notasSoloLectura es `NotasSoloLectura`: cada nota en su caja (`rounded-lg border p-4`),
// el título y el cuerpo; una cifrada se DICE con el candado, no se omite.
//
// ponytail: sin la `shadow-sm` de cada caja (la sombra la lleva `ui.Tarjeta`, que no es
// esta pieza) y sin volver anclajes los enlaces entre notas de la misma copia.
func (p *Pantalla) notasSoloLectura(notas []Nota, v tema.Variante) fyne.CanvasObject {
	t := p.tr.T
	if len(notas) == 0 {
		return lineaTok(t("empty.noNotes.title", nil), tema.TextoBase, ui.Normal, "text-muted", v)
	}
	cajas := make([]fyne.CanvasObject, 0, len(notas))
	for _, n := range notas {
		var piezas []fyne.CanvasObject
		if n.Titulo != "" {
			piezas = append(piezas, container.New(ui.Margen{Abj: 1 * 0.25 * tema.Rem},
				lineaTok(n.Titulo, tema.TextoBase, ui.Medio, "text", v)))
		}
		switch {
		case n.Cifrada:
			piezas = append(piezas, container.New(ui.Fila{Hueco: 1 * 0.25 * tema.Rem},
				Icono(iconos.Candado, 14, "text-subtle", v),
				lineaTok(t("crypto.locked", nil), tema.TextoBase, ui.Normal, "text-subtle", v)))
		case n.Tipo == "text" || n.Tipo == "todo":
			piezas = append(piezas, NuevaProsa(n.Doc))
		case n.Contenido != "":
			piezas = append(piezas, ui.NuevoParrafo(n.Contenido, tema.TextoBase, ui.Normal, "text-muted", v))
		}
		caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG)
		g := RellenoNotaLectura + tema.Borde
		cajas = append(cajas, container.New(&enCaja{caja: caja}, append(caja.Objetos(),
			container.New(ui.Margen{Izq: g, Der: g, Arr: g, Abj: g}, container.New(ui.Columna{}, piezas...)))...))
	}
	return container.New(ui.Columna{Hueco: 3 * 0.25 * tema.Rem}, cajas...)
}

// confirmarImportacion crea el cuaderno, lo abre y dice cuántas entraron, y SIEMPRE cuántas
// se quedaron fuera: una importación que se come notas en silencio deja un hueco que se
// descubre semanas después.
func (p *Pantalla) confirmarImportacion(volcado []byte) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	r, err := p.svc.ImportNotebook(ctx, volcado)
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("import.failed", nil), "")
		return
	}
	p.cargarCuadernos()
	enHilo(func() {
		p.abrirCuaderno(DeCuaderno(r.Notebook))
		if r.Descartadas > 0 {
			p.avisos.Notificar(ui.NuevoAviso(ui.AvisoAdvertencia, p.tr.T("import.someSkipped",
				i18n.Vars{"count": r.Importadas, "skipped": r.Descartadas}), ""))
			return
		}
		p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, p.tr.T("import.done", i18n.Vars{"count": r.Importadas}), ""))
	})
}
