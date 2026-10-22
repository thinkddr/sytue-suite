// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// pdfVerificar atiende POST /api/v1/pdf/verify: el informe de cada firma de un PDF
// recibido —integridad, cambios posteriores, sello y confianza—. Conversión pura
// como el resto de `/pdf/*`: entra el fichero y nada se guarda.
//
// Va aparte de `/pdf/content` porque cuesta más —cada firma resume el fichero
// entero y compara las revisiones— y solo hace falta cuando alguien abre el panel
// de firmas, no cada vez que se abre un documento.
func (h *Handler) pdfVerificar(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"firmas": doc.VerificarFirmas(domain.OpcionesVerificacion{
			Anclas: h.firmaAnclas, AnclasSello: h.sello.Anclas,
		}),
		// Para que la pantalla diga POR QUÉ no juzga la identidad: no es el
		// documento, es que el despliegue no tiene en quién confiar.
		"anclas": h.firmaAnclas != nil,
	})
}
