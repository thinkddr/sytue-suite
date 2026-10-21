// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"cmp"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// MENCIONES
//
// Mencionar a alguien en una nota es escribir `@` y su dirección —`@ana@empresa.es`—,
// igual que en un comentario de Docs (hubnotify.Mencionados). No hay nada que guardar:
// la mención es el texto, y lo que hace el servicio es AVISAR a esa persona.
//
// Dos reglas, y las dos están aquí y no en el cliente:
//   - Se avisa UNA VEZ, cuando la dirección aparece: una nota se guarda sola cada
//     pocos segundos, y avisar en cada guardado sería llenarle la campana. Por eso se
//     compara con el texto de ANTES, leído con la fila bloqueada (store.Antes).
//   - Solo a quien PUEDE LEER la nota. El aviso lleva su título y un trozo de su
//     texto: mandárselo a quien no la ve sería enseñársela.

// textoDeMenciones es el texto del que se leen las menciones: título y contenido llano.
func textoDeMenciones(titulo, contenido string) string {
	return titulo + "\n" + domain.TextoDeHTML(contenido)
}

// avisarMenciones avisa a quienes la nota menciona y `antes` no mencionaba. La nota ya
// está guardada: aquí nada falla hacia fuera, lo que no se pueda avisar se anota.
func (h *Handlers) avisarMenciones(r *http.Request, n domain.Note, antes string) {
	texto := textoDeMenciones(n.Title, n.Content)
	ya := hubnotify.Mencionados(antes)
	var nuevas []string
	for _, e := range hubnotify.Mencionados(texto) {
		if !slices.Contains(ya, e) {
			nuevas = append(nuevas, e)
		}
	}
	if len(nuevas) == 0 {
		return
	}
	ctx := r.Context()
	org := tenancy.MustFrom(ctx).Slug
	c, _ := authn.FromContext(ctx)
	actor := cmp.Or(strings.TrimSpace(c.Name), c.Email)
	nombre := cmp.Or(strings.TrimSpace(n.Title), "Notepads")
	for _, e := range nuevas {
		// De la dirección a la persona, con la sesión de quien escribe: Sytue ID solo
		// contesta por gente de SU organización, y a medio teclear no es de nadie.
		sujeto, err := peticiones.PorCorreo(ctx, h.cfg.IDBaseURL, authn.Token(r), e)
		if err != nil || sujeto == "" || sujeto == sub(r) {
			continue
		}
		porCuaderno, porNota, err := h.store.LecturaDeNota(ctx, n.ID, sujeto)
		if err != nil {
			log.From(ctx).Warn("mención: mirar si la persona lee la nota", "error", err)
			continue
		}
		enlace := hubnotify.EnlaceNota(h.cfg.PublicURL, org, n.NotebookID.String(), n.ID.String())
		switch {
		case porCuaderno:
		case porNota:
			// No ve el cuaderno: la abre en «Compartido conmigo».
			enlace = hubnotify.EnlaceNotaCompartida(h.cfg.PublicURL, org, n.ID.String())
		default:
			continue
		}
		h.hub.SendAsync(ctx, hubnotify.Notification{
			Tenant: org, Subject: sujeto, Source: "notes", Type: "notes.mention", Level: "info",
			Title: hubnotify.RecortarTitulo(fmt.Sprintf("%s te ha mencionado en «%s»", actor, nombre)),
			Body:  validation.TruncateRunes(strings.TrimSpace(domain.TextoDeHTML(n.Content)), 300),
			URL:   enlace, Params: map[string]string{"name": nombre, "actor": actor},
		})
	}
}
