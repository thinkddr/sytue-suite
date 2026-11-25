// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// La cartera (store/cartera.go): la antigüedad de lo pendiente de cobro y de pago, el
// estado de cuenta de un contacto, los productos bajo su stock mínimo y el libro mayor
// de una cuenta. Cuatro lecturas; cada una pide leer el módulo del que salen sus cifras.

// maxCuenta es lo más largo que puede ser un código de cuenta en una consulta.
const maxCuenta = 32

// hoyDe es el día de la consulta: `?hoy=AAAA-MM-DD` (el de quien mira, en su huso) o el
// del servidor. Un vencimiento se cuenta en días, y a las 00:30 en Madrid el servidor,
// en UTC, sigue en el día anterior.
func hoyDe(r *http.Request) (time.Time, bool) {
	d, err := fechaOpcional(r.URL.Query().Get("hoy"))
	if err != nil {
		return d, false
	}
	if d.IsZero() {
		d = time.Now()
	}
	return d, true
}

// Antiguedad — GET /api/v1/cobros/antiguedad?tipo=cobros|pagos — lo pendiente de cobro
// (facturas de cliente) o de pago (de proveedor) en cinco tramos de días vencido.
func (h *Handlers) Antiguedad(w http.ResponseWriter, r *http.Request) {
	modelo := "factura_venta"
	switch r.URL.Query().Get("tipo") {
	case "", "cobros":
	case "pagos":
		modelo = "factura_compra"
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("tipo: cobros or pagos"))
		return
	}
	hoy, ok := hoyDe(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("hoy: YYYY-MM-DD"))
		return
	}
	a, err := h.store.Antiguedad(r.Context(), modelo, hoy)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, a)
}

// EstadoDeCuenta — GET /api/v1/contacto/{id}/estado-cuenta — lo facturado, pagado,
// pendiente y vencido con un contacto, a los dos lados, y sus facturas. Leer el contacto
// lo autoriza el modelo de la ruta; las cifras son de facturación, y sin poder leerla
// no se enseñan.
func (h *Handlers) EstadoDeCuenta(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if m.Clave != "contacto" {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if !h.puede(r.Context(), "facturacion", permisos.Lectura) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	hoy, ok := hoyDe(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("hoy: YYYY-MM-DD"))
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := h.store.Obtener(r.Context(), m.Clave, id); err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	e, err := h.store.EstadoDeCuenta(r.Context(), id, hoy)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, e)
}

// BajoMinimo — GET /api/v1/inventario/bajo-minimo — los productos cuyas existencias no
// llegan a su stock mínimo. Mezcla productos y existencias: pide leer los dos módulos.
func (h *Handlers) BajoMinimo(w http.ResponseWriter, r *http.Request) {
	if !h.puede(r.Context(), "productos", permisos.Lectura) || !h.puede(r.Context(), "inventario", permisos.Lectura) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	ps, err := h.store.BajoMinimo(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"productos": ps})
}

// Mayor — GET /api/v1/mayor?cuenta=430000&desde=&hasta= — el libro mayor de una cuenta:
// sus apuntes asentados del período con el saldo que deja cada uno.
func (h *Handlers) Mayor(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cuenta := strings.TrimSpace(q.Get("cuenta"))
	if cuenta == "" || utf8.RuneCountInString(cuenta) > maxCuenta {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("cuenta: required, up to 32 characters"))
		return
	}
	desde, err := fechaOpcional(q.Get("desde"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("desde: YYYY-MM-DD"))
		return
	}
	hasta, err := fechaOpcional(q.Get("hasta"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("hasta: YYYY-MM-DD"))
		return
	}
	m, err := h.store.Mayor(r.Context(), cuenta, desde, hasta)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, m)
}

// facturaAbierta dice si una factura está emitida y sin cobrar: la única a la que tiene
// sentido mandarle un recordatorio de pago.
func facturaAbierta(reg store.Registro) bool {
	for _, e := range store.EstadosAbiertos {
		if reg.Estado == e {
			return true
		}
	}
	return false
}
