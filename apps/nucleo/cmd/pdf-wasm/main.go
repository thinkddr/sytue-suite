// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

// Command pdf-wasm es el motor de PDF de apps/nucleo/pdf en el navegador (ADR 0086,
// «Firmar en el dispositivo», tercera entrega). Lo carga el estudio de CertiPDF SOLO
// para los PDF de la bóveda post-cuántica: el servidor no puede ver esos PDF, así que
// dibujarlos, anotarlos, rellenarlos u organizarlos tiene que pasar aquí, en el
// dispositivo.
//
// No tiene red, ni claves, ni almacenamiento: recibe los bytes ya descifrados por el
// host, los transforma en memoria con las MISMAS operaciones que el servicio de Docs
// (`ApplyAnnotationOperation`, `ApplyFormOperation`, `ApplyPageOperation`,
// `ApplyLayerOperation`, `ApplyToolOperation`, `ApplyProtectionOperation`,
// `ApplyEditOperation`, `ApplyCompareOperation`, `RenderView`, `Summary`, el texto,
// la verificación y los adjuntos) y devuelve bytes. Cifrar y guardar el
// resultado es cosa del host (la bóveda), nunca de este módulo.
package main

import (
	"context"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func main() {
	docs := newDocumentos()
	webport.Serve(webport.Module{
		ID:      "pdf",
		Version: 1,
		Execute: func(string, js.Value) (any, error) { return nil, webport.ErrRejected },
		Async: map[string]func(context.Context, js.Value) (any, error){
			"pdf.abrir":       docs.abrirJS,
			"pdf.cerrar":      docs.cerrarJS,
			"pdf.render":      docs.renderJS,
			"pdf.anotar":      docs.anotarJS,
			"pdf.formulario":  docs.formularioJS,
			"pdf.paginas":     docs.paginasJS,
			"pdf.herramienta": docs.herramientaJS,
			"pdf.proteger":    docs.protegerJS,
			"pdf.editar":      docs.editarJS,
			"pdf.comparar":    docs.compararJS,
			"pdf.texto":       docs.textoJS,
			"pdf.verificar":   docs.verificarJS,
			"pdf.adjunto":     docs.adjuntoJS,
		},
	})
}
