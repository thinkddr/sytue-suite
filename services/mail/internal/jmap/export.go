// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// buzonExportado es una fila de mailboxes.<formato>: el buzón y su ruta de carpetas,
// que es la que llevan los .eml y la que usa quien se lo lleva a otro servicio.
type buzonExportado struct {
	Path string `json:"path"`
	domain.Mailbox
}

// mensajeExportado es una fila de messages.<formato>: el mensaje y dónde está su .eml.
type mensajeExportado struct {
	Path    string `json:"path"`
	Mailbox string `json:"mailbox"`
	domain.Message
}

// exportData entrega el buzón para «Exportar tus datos» (GET /api/v1/export): cada
// mensaje en su .eml ORIGINAL, en la carpeta de su buzón, y las tablas de carpetas,
// mensajes, etiquetas y filtros. El .eml es el mismo que llegó: con sus cabeceras,
// sus adjuntos y su firma, que es lo que otro cliente de correo sabe importar.
func (a *API) exportData(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	p, err := portabilidad.Nuevo(w, r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}
	if err := a.exportar(r, t, acc.ID, p); err != nil {
		_ = p.Fallo(err)
		return
	}
	_ = p.Cerrar()
}

func (a *API) exportar(r *http.Request, t tenancy.Tenant, cuenta uuid.UUID, p *portabilidad.Paquete) error {
	ctx := r.Context()
	buzones, err := a.store.ListMailboxes(ctx, cuenta)
	if err != nil {
		return err
	}
	filasBuzon := make([]buzonExportado, len(buzones))
	for i, b := range buzones {
		filasBuzon[i] = buzonExportado{Path: rutaDeBuzon(b, buzones), Mailbox: b}
	}
	if err := portabilidad.Tabla(p, "mailboxes", "mailbox", portabilidad.Todas(filasBuzon)); err != nil {
		return err
	}
	var filas []mensajeExportado
	for _, b := range buzones {
		carpeta := rutaDeBuzon(b, buzones)
		for antes, antesID := (time.Time{}), uuid.Nil; ; {
			pagina, err := a.store.ListMessages(ctx, b.ID, 200, antes, antesID)
			if err != nil {
				return err
			}
			for _, m := range pagina {
				ruta := "messages/" + carpeta + "/" + nombreEML(m)
				if err := a.exportarMensaje(r, t, p, ruta, m); err != nil {
					return err
				}
				filas = append(filas, mensajeExportado{Path: ruta, Mailbox: carpeta, Message: m})
			}
			if len(pagina) < 200 {
				break
			}
			ultimo := pagina[len(pagina)-1]
			antes, antesID = ultimo.ReceivedAt, ultimo.ID
		}
	}
	if err := portabilidad.Tabla(p, "messages", "message", portabilidad.Todas(filas)); err != nil {
		return err
	}
	return a.exportarAjustes(ctx, cuenta, p)
}

func (a *API) exportarMensaje(r *http.Request, t tenancy.Tenant, p *portabilidad.Paquete, ruta string, m domain.Message) error {
	blob, err := a.blobsGet(r, t, m.BlobKey)
	if err != nil {
		return fmt.Errorf("mensaje %s: %w", m.ID, err)
	}
	defer blob.Close()
	return p.Fichero(ruta, m.ReceivedAt, blob)
}

func (a *API) exportarAjustes(ctx context.Context, cuenta uuid.UUID, p *portabilidad.Paquete) error {
	etiquetas, err := a.store.ListLabels(ctx, cuenta)
	if err != nil {
		return err
	}
	if err := portabilidad.Tabla(p, "labels", "label", portabilidad.Todas(etiquetas)); err != nil {
		return err
	}
	filtros, err := a.store.ListFilters(ctx, cuenta)
	if err != nil {
		return err
	}
	return portabilidad.Tabla(p, "filters", "filter", portabilidad.Todas(filtros))
}

// rutaDeBuzon es la ruta de carpetas de un buzón («Clientes/Acme»).
func rutaDeBuzon(b domain.Mailbox, todos []domain.Mailbox) string {
	porID := make(map[uuid.UUID]domain.Mailbox, len(todos))
	for _, x := range todos {
		porID[x.ID] = x
	}
	ruta := b.Name
	// El tope evita girar para siempre si un dato corrupto hiciera un ciclo.
	for i := 0; b.ParentID != nil && i < 32; i++ {
		padre, ok := porID[*b.ParentID]
		if !ok {
			break
		}
		ruta, b = path.Join(padre.Name, ruta), padre
	}
	return ruta
}

// nombreEML ordena por fecha al listar la carpeta y deja reconocer el mensaje por
// su asunto; el trozo de id lo hace único aunque dos lleguen en el mismo segundo.
func nombreEML(m domain.Message) string {
	asunto := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' {
			return '-'
		}
		return r
	}, strings.TrimSpace(m.Subject))
	if runas := []rune(asunto); len(runas) > 60 {
		asunto = string(runas[:60])
	}
	return fmt.Sprintf("%s %s (%s).eml", m.ReceivedAt.UTC().Format("2006-01-02 150405"), asunto, m.ID.String()[:8])
}
