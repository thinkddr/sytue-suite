// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/basesconn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// Sección de un sitio con los registros de una VISTA de Sytue Bases (ADR 0068, bases-2).
//
// Es una INSTANTÁNEA: el editor pide aquí la vista y la guarda en el borrador como un nodo
// `table` corriente, que se pinta y se valida como cualquier otra tabla (cada celda pasa
// por el escapado del renderizador). No hay nodo nuevo en el IR, ni consulta al servir el
// sitio, ni token guardado: lo publicado es exactamente lo que quien edita vio y aprobó.
//
// Por eso mismo REVOCAR la conexión en Bases no retira lo ya publicado (es contenido del
// sitio, como si se hubiera tecleado): impide volver a traerlo. Y por eso solo trae datos
// quien puede publicar (owner/admin): lo que entra aquí acaba en una página pública.
//
// ponytail: los datos se refrescan volviendo a insertar la tabla. Un nodo enlazado que se
// actualice al publicar pide un tipo nuevo en el IR (TS y su espejo en Go), guardar el
// token sellado por sitio y decidir qué hace una publicación cuando la conexión ya no vale.

var errConexionBases = httpx.NewError(http.StatusUnprocessableEntity, "bases_connection_invalid",
	"That Sytue Bases connection is not valid for Sytue Web: it may be revoked, expired, granted to another product or to another organisation.")

// clienteBases habla con Sytue Bases por dentro del clúster.
var clienteBases = &http.Client{Timeout: 15 * time.Second}

// tablaDeBases es el cuerpo de un nodo `table` (textos planos; el editor les pone idioma)
// más la procedencia. Sin el token.
type tablaDeBases struct {
	Caption    string                `json:"caption"`
	Header     []string              `json:"header"`
	Rows       [][]string            `json:"rows"`
	Provenance basesconn.Procedencia `json:"provenance"`
}

// aTablaDeSitio pasa una lectura a tabla de sitio. Una vista que no cabe en una tabla del
// IR (50 filas × 8 columnas) se RECHAZA, no se recorta: publicar media vista sin decirlo es
// peor que pedir a quien administra la base que la acote.
func aTablaDeSitio(l *basesconn.Lectura) (tablaDeBases, error) {
	if len(l.Columns) == 0 || len(l.Columns) > ir.Limites.ColumnasTabla || len(l.Rows) > ir.Limites.FilasTabla {
		return tablaDeBases{}, httpx.NewError(http.StatusUnprocessableEntity, "bases_view_too_large",
			fmt.Sprintf("The view has %d columns and %d rows; a site table holds up to %d columns and %d rows. Narrow the view in Sytue Bases.",
				len(l.Columns), len(l.Rows), ir.Limites.ColumnasTabla, ir.Limites.FilasTabla))
	}
	out := tablaDeBases{Caption: l.Provenance.ViewName, Header: make([]string, len(l.Columns)), Rows: make([][]string, len(l.Rows)), Provenance: l.Provenance}
	for i, c := range l.Columns {
		out.Header[i] = c.Name
	}
	for i, r := range l.Rows {
		fila := make([]string, len(r))
		for j, v := range r {
			fila[j] = basesconn.Texto(v)
		}
		out.Rows[i] = fila
	}
	return out, nil
}

// tablaDesdeBases — POST /api/v1/sites/{id}/bases/table {token}.
func (h *Handlers) tablaDesdeBases(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Se requiere rol admin para traer datos de Sytue Bases a un sitio"))
		return
	}
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !basesconn.TokenValido(in.Token) {
		httpx.Fail(w, r, errConexionBases)
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	// El sitio ha de existir para quien pide (RLS): no se lee de Bases para un sitio ajeno.
	if _, err := h.Store.SitioCompleto(r.Context(), id); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	l, err := basesconn.Leer(r.Context(), clienteBases, basesconn.Peticion{
		Raiz: h.Cfg.BasesURL, Token: in.Token, Consumidor: basesconn.Web, Tenant: t.Slug,
		// Con holgura sobre lo que cabe en una tabla: lo que sobre se rechaza con su motivo.
		MaxBytes: 4 << 20, MaxFilas: 10_000, MaxCols: 100,
	})
	if err != nil {
		if errors.Is(err, basesconn.ErrRevocada) || errors.Is(err, basesconn.ErrToken) {
			httpx.Fail(w, r, errConexionBases)
			return
		}
		log.From(r.Context()).Warn("pages: Sytue Bases no contesta", "error", err)
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "bases_unavailable", "Sytue Bases is not available right now."))
		return
	}
	out, err := aTablaDeSitio(l)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
