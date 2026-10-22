// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// Export entrega la agenda para «Exportar tus datos» (GET /api/v1/export): la
// tabla de contactos y un contacts.vcf con todos, que es lo que importan Google,
// Outlook y cualquier móvil. En CSV la tabla es la de Google Contacts y no la
// genérica: así ese fichero también se importa tal cual.
func (h *Handlers) Export(w http.ResponseWriter, r *http.Request) {
	p, err := portabilidad.Nuevo(w, r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}
	if err := h.exportar(r.Context(), subFromCtx(r), p); err != nil {
		_ = p.Fallo(err)
		return
	}
	_ = p.Cerrar()
}

func (h *Handlers) exportar(ctx context.Context, sub string, p *portabilidad.Paquete) error {
	ahora := time.Now()
	contacts, err := h.store.ListOwnerContacts(ctx, sub)
	if err != nil {
		return err
	}
	if p.Formato() == portabilidad.CSV {
		var b bytes.Buffer
		if err := domain.WriteContactsCSV(&b, contacts); err != nil {
			return err
		}
		err = p.Fichero("contacts.csv", ahora, &b)
	} else {
		err = portabilidad.Tabla(p, "contacts", "contact", portabilidad.Todas(contacts))
	}
	if err != nil {
		return err
	}

	// El vCard guardado, no uno regenerado: lleva los campos que Sytue no
	// interpreta (y la foto), que es justo lo que no se puede perder al irse.
	books, err := h.store.ListAddressBooks(ctx, sub)
	if err != nil {
		return err
	}
	var vcf strings.Builder
	for _, b := range books {
		objs, err := h.store.ListCardObjects(ctx, b.ID)
		if err != nil {
			return err
		}
		for _, o := range objs {
			vcf.WriteString(strings.TrimRight(o.Data, "\r\n"))
			vcf.WriteString("\r\n")
		}
	}
	return p.Fichero("contacts.vcf", ahora, strings.NewReader(vcf.String()))
}
