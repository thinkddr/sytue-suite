// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Combinar PDF» desde la selección
//
// Es el `onCombinarPdfs` de FileBrowser.tsx con el puente que allí hace el BFF
// (pages/api/pdf-pages.ts): se descargan los PDF marcados en el orden de la lista, el
// motor los une y el resultado se sube como fichero NUEVO junto al primero, con la
// palabra del idioma de la persona («contrato (combinado).pdf»). Los originales no se
// tocan: una operación de páginas no tiene deshacer.

// PlazoCombinar es cuánto se espera a la combinación entera: bajar hasta cien megas,
// unirlos y volver a subirlos no cabe en el plazo de una petición suelta.
const PlazoCombinar = 10 * time.Minute

// ConPDF monta «Combinar PDF» con el cliente de PDF: sin él, la acción no se ofrece.
func (p *Pantalla) ConPDF(pdf *api.PDF) {
	p.pdf = pdf
	if pdf == nil {
		p.explor.AlCombinarPDF = nil
		return
	}
	p.explor.AlCombinarPDF = p.combinarPDF
}

// combinarPDF lanza la combinación de lo marcado. Se llama en la rutina de la interfaz.
func (p *Pantalla) combinarPDF() {
	if p.pdf == nil || !p.explor.PuedeCombinar() {
		return
	}
	sel := p.explor.PDFMarcados()
	destino := sel[0].Padre
	if destino == "" {
		destino = p.carpetaActual()
	}
	if destino == "" {
		destino = p.raizID
	}
	p.enSegundoPlano(func() { p.Combinar(sel, destino) })
}

// Combinar une `pdfs`, en ese orden, y deja el resultado en la carpeta `destino` con el
// nombre del primero. Avisa de cómo ha ido y, si ha ido bien, suelta la selección y
// recarga la carpeta para que el nuevo se vea. Se llama desde cualquier rutina.
func (p *Pantalla) Combinar(pdfs []Elemento, destino string) {
	if p.pdf == nil || len(pdfs) < 2 || !p.ocupar() {
		return
	}
	enHilo(func() { p.explor.Combinando = true; p.Repintar() })
	defer func() {
		enHilo(func() { p.explor.Combinando = false })
		p.soltar()
	}()
	vida := p.Vida
	if vida == nil {
		vida = context.Background()
	}
	ctx, cancelar := context.WithTimeout(vida, PlazoCombinar)
	defer cancelar()
	// En la raíz, sin padre ni `raizID` aún, «/folders//files» no es una carpeta: se
	// pregunta cuál es, como `crearCarpeta`.
	if destino == "" {
		if p.svc == nil {
			p.avisar(ui.AvisoPeligro, p.tr.T("pdf.failed", nil), "")
			return
		}
		raiz, err := p.svc.Root(ctx)
		if err != nil {
			p.avisar(ui.AvisoPeligro, p.tr.T("pdf.failed", nil), "")
			return
		}
		destino = raiz.ID
	}

	creado, err := p.unirPDF(ctx, pdfs, destino)
	if err != nil {
		if errors.Is(err, api.ErrPDFGrande) {
			p.avisar(ui.AvisoPeligro, p.tr.T("pdf.combineTooLarge", nil), "")
			return
		}
		p.avisar(ui.AvisoPeligro, p.tr.T("pdf.failed", nil), "")
		return
	}
	p.avisar(ui.AvisoExito, p.tr.T("pdf.done", i18n.Vars{"name": creado.Name}),
		p.tr.T("pdf.doneWhere", i18n.Vars{"name": pdfs[0].Nombre}))
	enHilo(p.explor.limpiarSeleccion)
	// Lo que se está viendo, no la carpeta: con una búsqueda delante, recargar la carpeta
	// la quitaba, y leer la ruta desde esta rutina no es seguro.
	p.recargarVista()
}

// unirPDF baja, combina y sube. El tope del motor es del CUERPO entero de la petición,
// no de cada fichero: se mira con los tamaños que ya dio la lista antes de bajar nada,
// y otra vez con lo bajado, que es lo que cuenta.
func (p *Pantalla) unirPDF(ctx context.Context, pdfs []Elemento, destino string) (api.File, error) {
	var previsto int64
	for _, el := range pdfs {
		previsto += max(el.Tamano, 0)
	}
	if previsto > api.PDFMaxBytes {
		return api.File{}, api.ErrPDFGrande
	}
	docs := make([]api.PDFFichero, 0, len(pdfs))
	defer func() {
		for _, d := range docs {
			clear(d.Datos)
		}
	}()
	total := 0
	for _, el := range pdfs {
		datos, err := p.pdf.Descargar(ctx, el.ID)
		if err != nil {
			return api.File{}, err
		}
		docs = append(docs, api.PDFFichero{Nombre: el.Nombre, Datos: datos})
		if total += len(datos); total > api.PDFMaxBytes {
			return api.File{}, api.ErrPDFGrande
		}
	}
	unido, err := p.pdf.Paginas(ctx, api.PDFCombinar, "", 0, "", docs...)
	if err != nil {
		return api.File{}, err
	}
	original := api.File{ID: pdfs[0].ID, FolderID: destino, Name: pdfs[0].Nombre}
	return p.pdf.GuardarCopia(ctx, original, p.tr.T("pdf.name.combinar", nil), unido)
}
