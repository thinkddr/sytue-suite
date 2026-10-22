// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/plantillas"
)

// semillaDeCreacion es con lo que nace un documento: su tipo, su título y el contenido
// inicial (las operaciones y el estado que resulta de aplicarlas).
type semillaDeCreacion struct {
	kind   domain.Kind
	titulo string
	ops    []domain.Op
	estado *domain.State
	// usoDoc y usoVersion: la plantilla de la organización de la que nace y su versión,
	// para anotarlo en su registro cuando el documento se cree.
	usoDoc     string
	usoVersion int
}

// semillaDeCreacion resuelve la plantilla pedida —de la galería de Sytue o de la
// organización («org:<id>»)— y el tema que se aplica a su contenido. Sin plantilla, el
// documento nace en blanco (y un tema no tiene nada que estilar).
func (h *Handler) semillaDeCreacion(r *http.Request, req createDocumentReq, sourceRef, titulo string) (semillaDeCreacion, error) {
	sem := semillaDeCreacion{kind: req.Kind, titulo: titulo, estado: domain.NewState()}
	clave := strings.TrimSpace(req.Template)
	if clave == "" {
		return sem, nil
	}
	if sourceRef != "" {
		// Una plantilla da un documento NUEVO cada vez; anclarlo a un fichero lo
		// haría idempotente y abrir la plantilla dos veces daría el mismo.
		return sem, httpx.ErrBadRequest.WithMessage(`"template" and "source_ref" cannot be combined.`)
	}
	var info plantillas.Info
	var err error
	if idOrg, esDeOrg := strings.CutPrefix(clave, prefijoPlantillaOrg); esDeOrg {
		// De la biblioteca de la organización: se copia el contenido de la versión.
		info, sem.usoVersion, sem.ops, err = h.desdePlantillaOrg(r, idOrg, req.TemplateVersion)
		sem.usoDoc = idOrg
	} else {
		var op domain.Op
		info, op, err = h.desdePlantilla(clave, req.Locale)
		sem.ops = []domain.Op{op}
	}
	if err != nil {
		return sem, err
	}
	sem.kind = info.Kind
	if sem.titulo == "" {
		sem.titulo = info.Name
	}
	for _, op := range sem.ops {
		if _, err := sem.estado.Apply(op); err != nil {
			return sem, err
		}
	}
	err = h.aplicarTemaAlCrear(r, req, &sem)
	return sem, err
}

// aplicarTemaAlCrear añade a la semilla la operación del tema pedido (`theme`), si lo hay.
func (h *Handler) aplicarTemaAlCrear(r *http.Request, req createDocumentReq, sem *semillaDeCreacion) error {
	th := strings.TrimSpace(req.Theme)
	if th == "" {
		return nil
	}
	tema, err := h.resolverTema(r.Context(), th, req.BrandVersion)
	if err != nil {
		return err
	}
	op, cambia, err := domain.OpTema(sem.kind, sem.estado, tema)
	if err != nil || !cambia {
		return err
	}
	sem.ops = append(sem.ops, op)
	_, err = sem.estado.Apply(op)
	return err
}
