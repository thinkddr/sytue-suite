// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

// maxImportCSVBytes acota el tamano del CSV subido (defensa: el cuerpo se
// envuelve en http.MaxBytesReader antes de parsear en streaming).
const maxImportCSVBytes = 5 << 20 // 5 MiB

// ---------------------------------------------------------------------------
// Favoritos (destacados)
// ---------------------------------------------------------------------------

// ListFavorites devuelve los UID de los contactos destacados del usuario. El
// frontend los cruza con su lista CardDAV para pintar la estrella y filtrar.
func (h *Handlers) ListFavorites(w http.ResponseWriter, r *http.Request) {
	uids, err := h.store.ListFavoriteUIDs(r.Context(), subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if uids == nil {
		uids = []string{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"uids": uids})
}

type favoriteRequest struct {
	UID      string `json:"uid"`
	Favorite bool   `json:"favorite"`
}

// ToggleFavorite marca/desmarca un contacto (por UID) como destacado. Mutacion
// por POST con cuerpo JSON (CSRF-safe: el navegador no puede enviarla
// cross-origin sin preflight). Solo actua sobre contactos del propio dueno.
func (h *Handlers) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
	var req favoriteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.UID) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The contact UID is required."))
		return
	}
	if err := h.store.SetFavorite(r.Context(), subFromCtx(r), req.UID, req.Favorite); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"uid": req.UID, "favorite": req.Favorite})
}

// ---------------------------------------------------------------------------
// Papelera (soft-delete existente + restaurar / eliminar definitivamente)
// ---------------------------------------------------------------------------

// ListTrash lista los contactos borrados del usuario (?limit=).
func (h *Handlers) ListTrash(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListTrash(r.Context(), subFromCtx(r), limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if items == nil {
		items = []store.TrashItem{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// RestoreContact revive un contacto de la papelera del usuario.
func (h *Handlers) RestoreContact(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.RestoreContact(r.Context(), subFromCtx(r), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// PurgeContact elimina definitivamente un contacto de la papelera del usuario.
func (h *Handlers) PurgeContact(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.PurgeContact(r.Context(), subFromCtx(r), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Import / Export CSV (Google Contacts / Outlook)
// ---------------------------------------------------------------------------

// ImportCSV importa contactos de un CSV de Google o de Outlook a una libreta del
// usuario. Detecta el dialecto por las cabeceras, salta filas invalidas sin
// abortar, deduplica por correo contra lo existente (y dentro del propio CSV) y
// crea cada contacto por el flujo normal (misma vCard canonica). Responde con el
// recuento importado/omitido. Solo importa a libretas del propio dueno.
func (h *Handlers) ImportCSV(w http.ResponseWriter, r *http.Request) {
	bookID, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub := subFromCtx(r)
	// El store ya es owner-scoped (libreta ajena => 404); el check explicito se
	// conserva como cinturon y documentacion del contrato.
	book, err := h.store.GetAddressBook(r.Context(), sub, bookID)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if book.OwnerSub != sub {
		httpx.Fail(w, r, httpx.ErrNotFound) // no importar a la libreta de otro
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxImportCSVBytes)
	parsed, skipped, err := domain.ParseContactsCSV(body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "csv_too_large",
				"El CSV supera el tamaño máximo permitido (5 MiB)."))
			return
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}

	existing, err := h.store.ContactEmailSet(r.Context(), bookID)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}

	imported := 0
	// ponytail: una transacción CreateContact por fila (reusa el flujo REST
	// completo: tarjeta + proyección + canales + bump). Cubre imports personales
	// típicos (cientos/miles) dentro del timeout de 30s; si hicieran falta imports
	// enormes, pasar a inserción por lotes / COPY en una sola transacción.
	for _, c := range parsed {
		if duplicateByEmail(c, existing) {
			skipped++ // dedupe contra lo ya existente / lo ya importado del CSV
			continue
		}
		c.UID = domain.NewUID()
		card, encErr := domain.EncodeContact(c, "")
		if encErr != nil {
			skipped++
			continue
		}
		if _, cErr := h.store.CreateContact(r.Context(), sub, store.UpsertContact{
			AddressBookID: bookID,
			Contact:       c,
			CardData:      card,
			ResourceName:  domain.ResourceName(c.UID),
		}); cErr != nil {
			skipped++
			continue
		}
		for _, e := range c.Emails {
			existing[strings.ToLower(strings.TrimSpace(e.Email))] = struct{}{}
		}
		imported++
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped})
}

// ExportCSV descarga todos los contactos del usuario como CSV (formato Google
// Contacts, el mas portable). Neutraliza la inyeccion de formulas CSV.
func (h *Handlers) ExportCSV(w http.ResponseWriter, r *http.Request) {
	contacts, err := h.store.ListOwnerContacts(r.Context(), subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="contactos.csv"`)
	w.WriteHeader(http.StatusOK)
	// Cabeceras ya enviadas: un error de escritura aqui no puede cambiar el
	// status; el cliente lo detecta como CSV truncado (raro con un buffer HTTP).
	_ = domain.WriteContactsCSV(w, contacts)
}

// duplicateByEmail indica si el contacto comparte algun correo (case-insensitive)
// con el conjunto dado (correos ya presentes en la libreta o ya importados).
func duplicateByEmail(c domain.Contact, existing map[string]struct{}) bool {
	for _, e := range c.Emails {
		if _, ok := existing[strings.ToLower(strings.TrimSpace(e.Email))]; ok {
			return true
		}
	}
	return false
}
