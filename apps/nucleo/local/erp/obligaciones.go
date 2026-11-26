// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/obligaciones"
)

// Las obligaciones fiscales más allá del calendario: lo que falta en el ERP para
// presentarlas, la cifra que sale del libro (`obligaciones.Calculo`), el proceso de
// cuatro pasos y el calendario descargable (ICS). Las reglas son las de
// apps/nucleo/erp/obligaciones, las mismas del servidor; aquí solo se guarda la fase.
// En el servidor el proceso es de la organización; aquí, de la persona (E-09).

const esquemaObligaciones = `
CREATE TABLE IF NOT EXISTS proceso_obligacion (
  clave      TEXT NOT NULL,
  periodo    TEXT NOT NULL,
  fase       INTEGER NOT NULL DEFAULT 0,
  referencia TEXT NOT NULL DEFAULT '',
  documentos TEXT NOT NULL DEFAULT '',
  editado    TEXT NOT NULL,
  PRIMARY KEY (clave, periodo)
);
`

func (s *Servidor) rutasObligaciones(h func(string, http.HandlerFunc)) {
	v := "/api/v1"
	h("GET "+v+"/obligaciones/calculo", s.calculoObligacion)
	h("GET "+v+"/obligaciones/ics", s.obligacionesICS)
	h("GET "+v+"/obligaciones/proceso", s.procesoObligacion)
	h("POST "+v+"/obligaciones/proceso", s.procesoObligacion)
	h("GET "+v+"/obligaciones/proceso/pendientes", s.pendientesProceso)
}

// facturaPendiente: un borrador, o emitida sin asiento vivo en el libro.
func (a *Almacen) facturaPendiente(ctx context.Context, r api.ERPRegistro) (bool, error) {
	switch r.Estado {
	case "borrador":
		return true, nil
	case "confirmado", "pagado":
		var n int
		err := a.db.QueryRowContext(ctx, `SELECT count(*) FROM asiento WHERE origen = ? AND estado = 'asentado'`, r.ID).Scan(&n)
		return n == 0, err
	}
	return false, nil
}

// Pendientes cuenta, por comprobación (banco, facturas, nominas) y mes (AAAA-MM), lo
// que falta en el ERP entre dos fechas: movimientos del banco sin justificar y
// facturas o nóminas sin llevar al libro.
func (a *Almacen) Pendientes(ctx context.Context, desde, hasta time.Time) (map[string]map[string]int, error) {
	out := map[string]map[string]int{}
	d, h := desde.Format(time.DateOnly), hasta.Format(time.DateOnly)
	for modelo, prep := range map[string]string{"linea_extracto": "banco", "factura_venta": "facturas", "factura_compra": "facturas", "nomina": "nominas"} {
		regs, err := a.listarTx(ctx, a.db, modelo, false, "")
		if err != nil {
			return nil, err
		}
		for _, r := range regs {
			fecha, _ := r.Data["fecha"].(string)
			if len(fecha) < 7 || fecha < d || fecha > h {
				continue
			}
			pendiente := r.Estado == "pendiente"
			if modelo != "linea_extracto" {
				if pendiente, err = a.facturaPendiente(ctx, r); err != nil {
					return nil, err
				}
			}
			if !pendiente {
				continue
			}
			if out[prep] == nil {
				out[prep] = map[string]int{}
			}
			out[prep][fecha[:7]]++
		}
	}
	return out, nil
}

// Procesos son las fases guardadas, por «clave:periodo».
func (a *Almacen) Procesos(ctx context.Context) (map[string]obligaciones.Proceso, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT clave, periodo, fase, referencia FROM proceso_obligacion`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := map[string]obligaciones.Proceso{}
	for filas.Next() {
		var clave, periodo string
		var p obligaciones.Proceso
		if err := filas.Scan(&clave, &periodo, &p.Fase, &p.Referencia); err != nil {
			return nil, err
		}
		out[clave+":"+periodo] = p
	}
	return out, filas.Err()
}

// Proceso es la fase guardada de una obligación; cero si no se ha empezado.
func (a *Almacen) Proceso(ctx context.Context, clave, periodo string) (obligaciones.Proceso, error) {
	var p obligaciones.Proceso
	err := a.db.QueryRowContext(ctx, `SELECT fase, referencia FROM proceso_obligacion WHERE clave = ? AND periodo = ?`, clave, periodo).
		Scan(&p.Fase, &p.Referencia)
	if errors.Is(err, sql.ErrNoRows) {
		return obligaciones.Proceso{}, nil
	}
	return p, err
}

// AvanzarProceso guarda el siguiente paso con las reglas de obligaciones.Avanzar; el
// último pide los documentos adjuntos al proceso (modelo `obligacion`).
func (a *Almacen) AvanzarProceso(ctx context.Context, clave, periodo string, fase int, referencia string, documentos []string) (obligaciones.Proceso, error) {
	if err := obligaciones.ComprobarFase(fase, referencia); err != nil {
		return obligaciones.Proceso{}, fmt.Errorf("%w: %w", err, ErrDatos)
	}
	var out obligaciones.Proceso
	err := a.tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT fase, referencia FROM proceso_obligacion WHERE clave = ? AND periodo = ?`, clave, periodo).
			Scan(&out.Fase, &out.Referencia)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		nuevo, cambia, err := obligaciones.Avanzar(out, fase, referencia, documentos, func(doc string) (bool, error) {
			var n int
			err := tx.QueryRowContext(ctx, `SELECT count(*) FROM adjunto WHERE modelo = 'obligacion' AND registro = ?`,
				obligaciones.RegistroDocumento(clave, periodo, doc)).Scan(&n)
			return n > 0, err
		})
		if errors.Is(err, obligaciones.ErrPaso) {
			return fmt.Errorf("%s: %w", err.Error(), ErrDatos)
		}
		if err != nil || !cambia {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO proceso_obligacion (clave, periodo, fase, referencia, documentos, editado)
			VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (clave, periodo) DO UPDATE SET fase = excluded.fase,
			referencia = excluded.referencia, documentos = excluded.documentos, editado = excluded.editado`,
			clave, periodo, nuevo.Fase, nuevo.Referencia, strings.Join(documentos, ","), a.marcaAhora()); err != nil {
			return err
		}
		out = nuevo
		return nil
	})
	return out, err
}

// perfil es el de la organización, o responde 404 si aún no hay.
func (s *Servidor) perfil(w http.ResponseWriter, r *http.Request) (api.ERPOrganizacion, obligaciones.Perfil, bool) {
	o, err := s.alm.Organizacion(r.Context())
	if errors.Is(err, ErrNoEncontrado) {
		fallo(w, http.StatusNotFound, "organizacion_no_configurada", "The organisation profile has not been set up yet.", nil)
		return o, obligaciones.Perfil{}, false
	}
	if err != nil {
		falloDe(w, err)
		return o, obligaciones.Perfil{}, false
	}
	return o, s.obl.PerfilDe(o.Pais, o.Forma, o.CP, o.Condiciones), true
}

// calculoObligacion — GET /obligaciones/calculo — «Preparar cifras».
func (s *Servidor) calculoObligacion(w http.ResponseWriter, r *http.Request) {
	o, perfil, ok := s.perfil(w, r)
	if !ok {
		return
	}
	clave, periodo := r.URL.Query().Get("clave"), r.URL.Query().Get("periodo")
	desde, hasta, ok := obligaciones.RangoDePeriodo(periodo)
	if !ok {
		fallo(w, http.StatusBadRequest, "bad_request", "Invalid period.", nil)
		return
	}
	pais, _ := s.obl.Pais(o.Pais)
	ob, hay := pais.Obligacion(clave)
	if !hay || ob.Calculo == nil {
		fallo(w, http.StatusUnprocessableEntity, "no_calculable", "This obligation cannot be computed from the ledger.", nil)
		return
	}
	if _, err := s.obl.Aplican(perfil); err != nil {
		fallo(w, http.StatusUnprocessableEntity, "unprocessable", err.Error(), nil)
		return
	}
	res, err := ob.Calculo.Calcular(r.Context(), clave, periodo, desde, hasta, func(ctx context.Context, desde, hasta time.Time) ([]obligaciones.Saldo, error) {
		saldos, err := s.alm.Balance(ctx, desde, hasta)
		out := make([]obligaciones.Saldo, 0, len(saldos))
		for _, x := range saldos {
			out = append(out, obligaciones.Saldo{Cuenta: x.Cuenta, Debe: x.Debe, Haber: x.Haber})
		}
		return out, err
	})
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, res)
}

// instancia es la obligación de la petición (clave y periodo) en el calendario del
// perfil; si devuelve false, ya ha respondido.
func (s *Servidor) instancia(w http.ResponseWriter, r *http.Request) (obligaciones.Instancia, time.Time, time.Time, bool) {
	_, perfil, ok := s.perfil(w, r)
	if !ok {
		return obligaciones.Instancia{}, time.Time{}, time.Time{}, false
	}
	clave, periodo := r.URL.Query().Get("clave"), r.URL.Query().Get("periodo")
	desde, hasta, ok := obligaciones.RangoDePeriodo(periodo)
	if !ok {
		fallo(w, http.StatusBadRequest, "bad_request", "Invalid period.", nil)
		return obligaciones.Instancia{}, time.Time{}, time.Time{}, false
	}
	inst, err := s.obl.Calendario(perfil, desde.Year())
	if err != nil {
		fallo(w, http.StatusUnprocessableEntity, "unprocessable", err.Error(), nil)
		return obligaciones.Instancia{}, time.Time{}, time.Time{}, false
	}
	for _, i := range inst {
		if i.Clave == clave && i.Periodo == periodo {
			return i, desde, hasta, true
		}
	}
	fallo(w, http.StatusNotFound, "not_found", "Resource not found.", nil)
	return obligaciones.Instancia{}, time.Time{}, time.Time{}, false
}

// procesoObligacion — GET|POST /obligaciones/proceso — leer y avanzar el proceso.
// Los tres primeros pasos piden que no quede nada pendiente en el periodo.
func (s *Servidor) procesoObligacion(w http.ResponseWriter, r *http.Request) {
	inst, desde, hasta, ok := s.instancia(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if r.Method == http.MethodGet {
		p, err := s.alm.Proceso(ctx, inst.Clave, inst.Periodo)
		if err != nil {
			falloDe(w, err)
			return
		}
		responder(w, http.StatusOK, p)
		return
	}
	var in obligaciones.Proceso
	if err := leer(r, &in); err != nil {
		falloDe(w, err)
		return
	}
	if in.Fase >= 1 && in.Fase <= obligaciones.FasesProceso {
		pendientes, err := s.alm.Pendientes(ctx, desde, hasta)
		if err != nil {
			falloDe(w, err)
			return
		}
		for _, n := range obligaciones.PendientesDe(inst, pendientes) {
			if n > 0 {
				fallo(w, http.StatusUnprocessableEntity, "unprocessable", "resuelve los pendientes antes de continuar", nil)
				return
			}
		}
	}
	p, err := s.alm.AvanzarProceso(ctx, inst.Clave, inst.Periodo, in.Fase, in.Referencia, inst.DocumentosProceso())
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, p)
}

// pendientesProceso — GET /obligaciones/proceso/pendientes — las facturas del periodo
// que faltan por llevar al libro, paginadas.
func (s *Servidor) pendientesProceso(w http.ResponseWriter, r *http.Request) {
	inst, desde, hasta, ok := s.instancia(w, r)
	if !ok {
		return
	}
	if !slices.Contains(inst.Preparacion, "facturas") {
		fallo(w, http.StatusBadRequest, "bad_request", "This obligation does not check invoices.", nil)
		return
	}
	limite := 30
	if v := r.URL.Query().Get("limite"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			fallo(w, http.StatusBadRequest, "bad_request", "Invalid limit.", nil)
			return
		}
		limite = n
	}
	ctx := r.Context()
	d, h := desde.Format(time.DateOnly), hasta.Format(time.DateOnly)
	var todas []api.ERPRegistro
	for _, modelo := range []string{"factura_venta", "factura_compra"} {
		regs, err := s.alm.listarTx(ctx, s.alm.db, modelo, false, "")
		if err != nil {
			falloDe(w, err)
			return
		}
		for _, reg := range regs {
			fecha, _ := reg.Data["fecha"].(string)
			if fecha < d || fecha > h {
				continue
			}
			pendiente, err := s.alm.facturaPendiente(ctx, reg)
			if err != nil {
				falloDe(w, err)
				return
			}
			if pendiente {
				todas = append(todas, reg)
			}
		}
	}
	slices.SortStableFunc(todas, func(a, b api.ERPRegistro) int {
		if c := b.CreadoEn.Compare(a.CreadoEn); c != 0 {
			return c
		}
		return strings.Compare(b.ID, a.ID)
	})
	// El cursor es el índice siguiente: la lista es de una persona y cabe entera.
	inicio, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
	inicio = min(max(inicio, 0), len(todas))
	fin := min(inicio+limite, len(todas))
	pag := api.ERPPagina{Registros: append([]api.ERPRegistro{}, todas[inicio:fin]...)}
	if fin < len(todas) {
		pag.Siguiente = strconv.Itoa(fin)
	}
	responder(w, http.StatusOK, pag)
}

// obligacionesICS — GET /obligaciones/ics — el calendario de este año y el siguiente
// como iCalendar, para importarlo con sus avisos.
func (s *Servidor) obligacionesICS(w http.ResponseWriter, r *http.Request) {
	o, perfil, ok := s.perfil(w, r)
	if !ok {
		return
	}
	anio := time.Now().Year()
	if a, err := strconv.Atoi(r.URL.Query().Get("anio")); err == nil && a >= 2000 && a <= 2100 {
		anio = a
	}
	var inst []obligaciones.Instancia
	for _, a := range []int{anio, anio + 1} {
		i, err := s.obl.Calendario(perfil, a)
		if err != nil {
			fallo(w, http.StatusUnprocessableEntity, "unprocessable", err.Error(), nil)
			return
		}
		inst = append(inst, i...)
	}
	l := idioma(r)
	eventos := obligaciones.EventosICS(inst, "local", func(clave string) string { return s.cat.Rotulo(clave, l) }, time.Now())
	cuerpo := obligaciones.ICS("ERP · "+o.Nombre, eventos, time.Now())
	adjuntar(w, "text/calendar; charset=utf-8", fmt.Sprintf("obligaciones-%s-%d.ics", strings.ToLower(o.Pais), anio), cuerpo)
}
