// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/banca"
	"git.sytue.eu/sytue/one/services/syerp/internal/obligaciones"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

var (
	errYaJustificado = httpx.NewError(http.StatusConflict, "ya_justificado",
		"This bank transaction already has its invoice or explanation.")
	errNoJustificado = httpx.NewError(http.StatusConflict, "no_justificado",
		"This bank transaction is still pending.")
	errFacturaNoAbierta = httpx.NewError(http.StatusConflict, "factura_no_abierta",
		"The invoice is already paid or cancelled.")
	errSentidoFactura = httpx.NewError(http.StatusUnprocessableEntity, "sentido_factura",
		"Money coming in is matched to an issued invoice, and money going out to a received one.")
)

// errorDeBanco traduce los errores del banco y deja el resto a errorDeEscritura.
func errorDeBanco(err error) error {
	switch {
	case errors.Is(err, store.ErrYaJustificado):
		return errYaJustificado
	case errors.Is(err, store.ErrNoJustificado):
		return errNoJustificado
	case errors.Is(err, store.ErrFacturaNoAbierta):
		return errFacturaNoAbierta
	}
	return errorDeEscritura(err)
}

// resultadoN43 es lo que la interfaz enseña al terminar de subir un extracto.
type resultadoN43 struct {
	Cuentas    []string `json:"cuentas"`
	Nuevos     int      `json:"nuevos"`
	Repetidos  int      `json:"repetidos"`
	Casados    int      `json:"casados"`
	Explicados int      `json:"explicados"`
	Pendientes int      `json:"pendientes"`
}

// importarN43 mete un extracto Norma 43 y hace a la vez el trabajo que se puede
// hacer sin preguntar: casa cada ingreso o cargo con su factura cuando no hay
// duda, y justifica solos los movimientos que se explican solos (una comisión, un
// cargo de Hacienda o de la Seguridad Social). Lo demás queda como tarea.
func (h *Handlers) importarN43(w http.ResponseWriter, r *http.Request, cuerpo []byte) {
	cuentas, err := banca.LeerN43(bytes.NewReader(cuerpo))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnprocessable.Wrap(err).WithMessage(err.Error()))
		return
	}
	ctx := r.Context()
	personaFisica := h.esPersonaFisica(ctx)
	abiertas, err := h.store.FacturasAbiertas(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	idioma := httpx.LanguageFromRequest(r)
	res := resultadoN43{Cuentas: []string{}}
	for _, c := range cuentas {
		huellas := banca.Huellas(c)
		e := store.ExtractoBanco{
			CuentaExterno: c.Clave(), CuentaNombre: c.Nombre(),
			Externo: c.Clave() + ":" + c.Desde.Format("20060102") + "-" + c.Hasta.Format("20060102"),
			Hasta:   c.Hasta, SaldoInicial: c.SaldoInicial, SaldoFinal: c.SaldoFinal,
		}
		for i, m := range c.Movimientos {
			e.Movimientos = append(e.Movimientos, store.MovimientoBanco{
				Externo: huellas[i], Fecha: m.Fecha, Descripcion: m.Texto(), Centimos: m.Centimos,
			})
		}
		nuevos, err := h.store.ImportarExtracto(ctx, e, sub(r))
		if err != nil {
			httpx.Fail(w, r, errorDeEscritura(err))
			return
		}
		res.Cuentas = append(res.Cuentas, c.Nombre())
		res.Repetidos += len(c.Movimientos) - len(nuevos)
		for i, m := range c.Movimientos {
			id, nuevo := nuevos[i]
			if !nuevo {
				continue
			}
			res.Nuevos++
			if f, ok := facturaSegura(m, abiertas); ok {
				if _, err := h.conciliar(ctx, id, f, sub(r)); err == nil {
					res.Casados++
					abiertas = slices.DeleteFunc(abiertas, func(x store.FacturaAbierta) bool { return x.ID == f.ID })
					continue
				}
			}
			if motivo := banca.MotivoEvidente(m); motivo != "" {
				if _, err := h.explicar(ctx, id, motivo, m.Centimos > 0, personaFisica, m.Texto(), idioma, sub(r)); err == nil {
					res.Explicados++
					continue
				}
			}
			res.Pendientes++
		}
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

// facturaSegura devuelve la factura de un movimiento solo si no hay duda Y ya está
// confirmada: casar con un borrador lo confirmaría, y eso lo decide una persona.
func facturaSegura(m banca.Movimiento, abiertas []store.FacturaAbierta) (store.FacturaAbierta, bool) {
	p, porID := proponer(m, abiertas)
	if p.Automatica == nil {
		return store.FacturaAbierta{}, false
	}
	f := porID[p.Automatica.ID]
	return f, f.Estado == "confirmado"
}

// proponer compara un movimiento con las facturas abiertas de su sentido: un
// ingreso, con las emitidas; un cargo, con las recibidas.
func proponer(m banca.Movimiento, abiertas []store.FacturaAbierta) (banca.Propuesta, map[string]store.FacturaAbierta) {
	modelo := "factura_venta"
	if m.Centimos < 0 {
		modelo = "factura_compra"
	}
	porID := map[string]store.FacturaAbierta{}
	var pendientes []banca.Pendiente
	for _, f := range abiertas {
		if f.Modelo != modelo || f.Total <= 0 {
			continue
		}
		fecha, _ := time.Parse(time.DateOnly, f.Fecha)
		venc, _ := time.Parse(time.DateOnly, f.Vencimiento)
		porID[f.ID] = f
		pendientes = append(pendientes, banca.Pendiente{
			ID: f.ID, Ref: f.Ref, Tercero: f.Tercero, NIF: f.NIF, Fecha: fecha, Vencimiento: venc, Centimos: f.Total,
		})
	}
	return banca.Emparejar(m, pendientes), porID
}

// conciliar casa un movimiento con una factura. Si la factura está en borrador,
// primero se confirma: pagar algo que no está en el libro dejaría al cliente o al
// proveedor con un saldo que no sale de ninguna factura.
func (h *Handlers) conciliar(ctx context.Context, lineaID string, f store.FacturaAbierta, owner string) (store.Asiento, error) {
	m, ok := h.cat.Modelo(f.Modelo)
	if !ok || m.Contable == nil {
		return store.Asiento{}, store.ErrNoEncontrado
	}
	if f.Estado == "borrador" {
		if _, err := h.store.Asentar(ctx, m, f.ID, owner); err != nil {
			return store.Asiento{}, err
		}
	}
	concepto := "Cobro " + f.Ref
	if !m.Contable.Cobro {
		concepto = "Pago " + f.Ref
	}
	return h.store.Liquidar(ctx, store.Liquidacion{
		LineaID: lineaID, FacturaID: f.ID, ModeloFactura: f.Modelo,
		Contrapartida: m.Contable.CuentaContrapartida, Concepto: concepto,
	}, owner)
}

// explicar justifica un movimiento sin factura con su motivo y lo asienta contra
// la cuenta que le corresponde.
//
// ponytail: la Seguridad Social de un autónomo va entera a gasto (642) aunque el
// cargo incluya también la de sus trabajadores; separarla pide leer la
// liquidación de la Tesorería, que hoy no entra.
func (h *Handlers) explicar(ctx context.Context, lineaID, motivo string, ingreso, personaFisica bool,
	texto, idioma, owner string,
) (store.Asiento, error) {
	cuenta, ok := banca.CuentaDeMotivo(motivo, ingreso, personaFisica)
	if !ok {
		return store.Asiento{}, errMotivoDesconocido
	}
	return h.store.Liquidar(ctx, store.Liquidacion{
		LineaID: lineaID, Motivo: motivo, Contrapartida: cuenta,
		Concepto: validation.TruncateRunes(h.cat.Rotulo("valor."+motivo, idioma)+" · "+texto, 200),
	}, owner)
}

var errMotivoDesconocido = httpx.NewError(http.StatusUnprocessableEntity, "motivo_desconocido",
	"Unknown reason for a transaction without an invoice.")

func (h *Handlers) esPersonaFisica(ctx context.Context) bool {
	o, err := h.store.Organizacion(ctx)
	if err != nil {
		return false
	}
	pais, ok := h.obligaciones.Pais(o.Pais)
	if !ok {
		return false
	}
	f, ok := pais.Forma(o.Forma)
	return ok && f.Tipo == obligaciones.TipoPersonaFisica
}

// --- Tareas del banco ---------------------------------------------------------

type candidataVista struct {
	store.FacturaAbierta
	Segura bool `json:"segura"`
}

type tareaBanco struct {
	store.LineaBanco
	Sugerencia string           `json:"sugerencia,omitempty"`
	Candidatas []candidataVista `json:"candidatas"`
}

// Banco — GET /api/v1/banco — las tareas del banco: cada movimiento sin
// justificar con las facturas que podrían ser la suya y el motivo que parece, y
// los últimos justificados, para poder deshacer.
func (h *Handlers) Banco(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pendientes, total, err := h.store.LineasBanco(ctx, "pendiente", 200)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	hechas, _, err := h.store.LineasBanco(ctx, "conciliado", 20)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	abiertas, err := h.store.FacturasAbiertas(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	tareas := make([]tareaBanco, 0, len(pendientes))
	for _, l := range pendientes {
		fecha, _ := time.Parse(time.DateOnly, l.Fecha)
		m := banca.Movimiento{Fecha: fecha, Centimos: l.Centimos, Descripcion: l.Descripcion}
		p, porID := proponer(m, abiertas)
		t := tareaBanco{LineaBanco: l, Sugerencia: banca.MotivoProbable(m), Candidatas: []candidataVista{}}
		for _, c := range p.Candidatas {
			t.Candidatas = append(t.Candidatas, candidataVista{
				FacturaAbierta: porID[c.ID], Segura: p.Automatica != nil && p.Automatica.ID == c.ID,
			})
		}
		tareas = append(tareas, t)
	}
	idioma := httpx.LanguageFromRequest(r)
	type motivoVista struct {
		Clave  string `json:"clave"`
		Rotulo string `json:"rotulo"`
	}
	motivos := make([]motivoVista, 0, len(banca.Motivos))
	for _, m := range banca.Motivos {
		motivos = append(motivos, motivoVista{m, h.cat.Rotulo("valor."+m, idioma)})
	}
	if hechas == nil {
		hechas = []store.LineaBanco{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"pendientes": tareas, "total_pendientes": total, "hechas": hechas, "motivos": motivos,
	})
}

// ConciliarMovimiento — POST /api/v1/banco/{id}/conciliar {"factura": id} — casa
// el movimiento con una factura emitida (si entra dinero) o recibida (si sale).
func (h *Handlers) ConciliarMovimiento(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Factura string `json:"factura"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := validation.New().Required("factura", cuerpo.Factura).Err(); err != nil {
		httpx.Fail(w, r, errorDeValidacion(err))
		return
	}
	ctx := r.Context()
	linea, err := h.store.Obtener(ctx, "linea_extracto", chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	importe, _ := linea.Data["importe"].(float64)
	var factura store.Registro
	for _, modelo := range []string{"factura_venta", "factura_compra"} {
		if factura, err = h.store.Obtener(ctx, modelo, cuerpo.Factura); err == nil {
			break
		}
	}
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	if (factura.Modelo == "factura_venta") != (importe > 0) {
		httpx.Fail(w, r, errSentidoFactura)
		return
	}
	if factura.Estado != "borrador" && factura.Estado != "confirmado" {
		httpx.Fail(w, r, errFacturaNoAbierta)
		return
	}
	asiento, err := h.conciliar(ctx, linea.ID, store.FacturaAbierta{
		ID: factura.ID, Modelo: factura.Modelo, Ref: factura.Ref, Estado: factura.Estado,
	}, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeBanco(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, asiento)
}

// ExplicarMovimiento — POST /api/v1/banco/{id}/explicar {"motivo": clave} — da
// por justificado un movimiento que no lleva factura.
func (h *Handlers) ExplicarMovimiento(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Motivo string `json:"motivo"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	linea, err := h.store.Obtener(ctx, "linea_extracto", chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	importe, _ := linea.Data["importe"].(float64)
	texto, _ := linea.Data["descripcion"].(string)
	asiento, err := h.explicar(ctx, linea.ID, cuerpo.Motivo, importe > 0, h.esPersonaFisica(ctx),
		texto, httpx.LanguageFromRequest(r), sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeBanco(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, asiento)
}

// ReabrirMovimiento — POST /api/v1/banco/{id}/reabrir — deshace la justificación.
func (h *Handlers) ReabrirMovimiento(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Reabrir(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.Fail(w, r, errorDeBanco(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
