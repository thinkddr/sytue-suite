// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/basesconn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/internal/ia"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// Fuente «Sytue Bases» de los informes (ADR 0068, bases-2): UNA vista de una base, de solo
// lectura, por una conexión que concedió el propietario de la base para SyERP.
//
// A diferencia de las fuentes de las otras apps (`otrasApps`), aquí NO viaja el token de la
// persona: viaja el de la conexión, sellado en `conexiones`. Por eso quien ve la fuente no
// lo decide Bases sino este fichero: solo administración (owner/admin), que es también quien
// puede guardarla. Bases comprueba el token en cada lectura: revocar allí corta aquí en el
// acto, y el informe guardado enseña «no disponible».
//
// ponytail: una vista por organización (la clave de `conexiones` es el proveedor). Varias
// vistas piden una tabla propia con id; la fuente ya se llamaría `bases:<id>`.
const (
	proveedorBases = "bases"
	fuenteBases    = "bases"
	appBases       = "Sytue Bases"
	// Topes de UNA lectura: lo que un informe puede cargar de una vista.
	basesMaxBytes = 16 << 20
	basesMaxFilas = 10_000
	basesMaxCols  = 100
)

var errConexionBases = httpx.NewError(http.StatusUnprocessableEntity, "conexion_bases_invalida",
	"That Sytue Bases connection is not valid for SyERP: it may be revoked, expired, granted to another product or to another organisation.")

// esAdministracion dice si quien pide es owner o admin de la organización.
func esAdministracion(ctx context.Context) bool {
	c, ok := authn.FromContext(ctx)
	return ok && c.HasAnyRole("owner", "admin")
}

// leerBases hace una lectura con el token dado, para la organización de la petición.
func (h *Handlers) leerBases(ctx context.Context, token string) (*basesconn.Lectura, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	return basesconn.Leer(ctx, clienteApps, basesconn.Peticion{
		Raiz: h.cfg.BasesBaseURL, Token: token, Consumidor: basesconn.ERP, Tenant: t.Slug,
		MaxBytes: basesMaxBytes, MaxFilas: basesMaxFilas, MaxCols: basesMaxCols,
	})
}

// clavesDeColumnas da a cada columna de la vista la clave con la que el informe la nombra:
// su nombre, sin `:` (que en una consulta separa función y campo) y sin repetirse.
func clavesDeColumnas(cols []basesconn.Columna) []string {
	out := make([]string, len(cols))
	vistas := map[string]int{}
	for i, c := range cols {
		k := strings.TrimSpace(strings.ReplaceAll(c.Name, ":", " "))
		if k == "" {
			k = fmt.Sprintf("columna %d", i+1)
		}
		vistas[k]++
		if n := vistas[k]; n > 1 {
			k = fmt.Sprintf("%s (%d)", k, n)
		}
		out[i] = k
	}
	return out
}

// guardarConexionBases comprueba el token LEYENDO con él (así se sabe que es para SyERP y
// para esta organización) y guarda, con el secreto sellado, las columnas y la procedencia.
// Lo que mande el cliente en `config` se ignora: lo escribe el servidor.
func (h *Handlers) guardarConexionBases(w http.ResponseWriter, r *http.Request, cuerpo cuerpoConexion) {
	ctx := r.Context()
	token := cuerpo.Secreto
	if token == "" {
		// Sin token nuevo solo se puede apagar o encender la que hay.
		previa, err := h.store.Conexion(ctx, proveedorBases, h.cifra)
		if err != nil {
			httpx.Fail(w, r, errConexionBases)
			return
		}
		token = previa.Secreto
	}
	l, err := h.leerBases(ctx, token)
	if err != nil {
		log.From(ctx).Warn("syerp: conexión de Sytue Bases rechazada", "error", err)
		httpx.Fail(w, r, errConexionBases)
		return
	}
	cols := make([]map[string]string, len(l.Columns))
	for i, k := range clavesDeColumnas(l.Columns) {
		cols[i] = map[string]string{"clave": k, "tipo": l.Columns[i].Type}
	}
	p := l.Provenance
	err = h.store.GuardarConexion(ctx, store.Conexion{
		Proveedor: proveedorBases, Instancia: p.BaseName + " › " + p.TableName + " › " + p.ViewName,
		Activo: cuerpo.Activo, Secreto: cuerpo.Secreto,
		Config: map[string]any{"columnas": cols, "procedencia": procedenciaDe(p)},
	}, h.cifra, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.NoContent(w)
}

// procedenciaDe es lo que el informe enseña de dónde salen los datos. Sin el token.
func procedenciaDe(p basesconn.Procedencia) map[string]any {
	return map[string]any{
		"origen": appBases, "base": p.BaseName, "tabla": p.TableName, "vista": p.ViewName,
		"conexion": p.ConnectionID, "version": p.Version, "concedida_por": p.GrantedBy,
		"concedida_el": p.GrantedAt.UTC().Format(time.RFC3339), "caduca_el": p.ExpiresAt.UTC().Format(time.RFC3339),
		"leida_el": p.ReadAt.UTC().Format(time.RFC3339),
	}
}

// fuenteDeBases es la fuente que ve administración si hay una conexión activa. Sale de lo
// guardado (columnas de la última lectura), sin llamar a Bases: listar fuentes no lee datos.
func (h *Handlers) fuenteDeBases(ctx context.Context) (ia.Fuente, bool) {
	if !esAdministracion(ctx) || h.cfg.BasesBaseURL == "" {
		return ia.Fuente{}, false
	}
	cx, err := h.store.ListarConexiones(ctx)
	if err != nil {
		return ia.Fuente{}, false
	}
	for _, c := range cx {
		if c.Proveedor != proveedorBases || !c.Activo {
			continue
		}
		f := ia.Fuente{Clave: fuenteBases, Etiqueta: c.Instancia, App: appBases}
		cols, _ := c.Config["columnas"].([]any)
		for _, x := range cols {
			m, _ := x.(map[string]any)
			clave, _ := m["clave"].(string)
			tipo, _ := m["tipo"].(string)
			if clave != "" {
				f.Campos = append(f.Campos, ia.Campo{Clave: clave, Tipo: tipoDeBases(tipo), Etiqueta: clave})
			}
		}
		return f, len(f.Campos) > 0
	}
	return ia.Fuente{}, false
}

func tipoDeBases(t string) string {
	switch t {
	case "number":
		return ia.TipoNumero
	case "boolean":
		return ia.TipoBooleano
	}
	return ia.TipoTexto
}

// filasDeBases lee la vista AHORA (sin caché: revocar en Bases corta aquí) y la pasa a
// filas del informe. Devuelve además la procedencia de ESTA lectura.
func (h *Handlers) filasDeBases(ctx context.Context, f ia.Fuente, tope int) ([]ia.Fila, bool, string, map[string]any) {
	// Se vuelve a exigir aquí: un informe guardado por administración no enseña la vista a
	// quien no lo es, aunque llegue con la clave de la fuente.
	if !esAdministracion(ctx) {
		return nil, false, ia.ErrorSinPermiso, nil
	}
	c, err := h.store.Conexion(ctx, proveedorBases, h.cifra)
	if err != nil || !c.Activo || c.Secreto == "" {
		return nil, false, ia.ErrorNoDisponible, nil
	}
	l, err := h.leerBases(ctx, c.Secreto)
	if err != nil {
		if !errors.Is(err, basesconn.ErrRevocada) {
			log.From(ctx).Warn("syerp: Sytue Bases no contesta", "error", err)
		}
		return nil, false, ia.ErrorNoDisponible, nil
	}
	claves := clavesDeColumnas(l.Columns)
	// La vista pudo cambiar de columnas desde que se guardó: las que el informe conoce y ya
	// no vienen quedan vacías; las nuevas no se enseñan hasta volver a guardar la conexión.
	conocidas := map[string]bool{}
	for _, campo := range f.Campos {
		conocidas[campo.Clave] = true
	}
	recortada := len(l.Rows) > tope
	if recortada {
		l.Rows = l.Rows[:tope]
	}
	filas := make([]ia.Fila, len(l.Rows))
	for i, r := range l.Rows {
		fila := ia.Fila{}
		for j, v := range r {
			if conocidas[claves[j]] && v != nil {
				fila[claves[j]] = v
			}
		}
		filas[i] = fila
	}
	return filas, recortada, "", procedenciaDe(l.Provenance)
}
