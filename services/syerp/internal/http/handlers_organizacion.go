// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/obligaciones"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

var (
	errSinOrganizacion = httpx.NewError(http.StatusNotFound, "organizacion_no_configurada",
		"The organisation profile has not been set up yet.")
	errNoCalculable = httpx.NewError(http.StatusUnprocessableEntity, "no_calculable",
		"This obligation cannot be computed from the ledger.")
)

// --- Perfil de la organización ---------------------------------------------

// Organizacion devuelve el perfil. 404 con código propio si no existe: es la
// señal para que la interfaz abra el asistente en vez de un panel vacío.
func (h *Handlers) Organizacion(w http.ResponseWriter, r *http.Request) {
	o, err := h.store.Organizacion(r.Context())
	if errors.Is(err, store.ErrNoEncontrado) {
		httpx.Fail(w, r, errSinOrganizacion)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, o)
}

// Paises devuelve, para el asistente, los países con sus formas jurídicas y
// portales. Sin las obligaciones: eso es el calendario, y se pide aparte.
func (h *Handlers) Paises(w http.ResponseWriter, r *http.Request) {
	type paisVista struct {
		Pais       string                         `json:"pais"`
		Verificado bool                           `json:"verificado"`
		Moneda     string                         `json:"moneda"`
		Formas     []obligaciones.Forma           `json:"formas"`
		Portales   map[string]obligaciones.Portal `json:"portales"`
		Total      int                            `json:"obligaciones"`
	}
	paises := h.obligaciones.Paises()
	out := make([]paisVista, 0, len(paises))
	for _, p := range paises {
		out = append(out, paisVista{
			Pais: p.Pais, Verificado: p.Verificado, Moneda: p.Moneda,
			Formas: p.Formas, Portales: p.Portales, Total: len(p.Obligaciones),
		})
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"paises": out, "condiciones": obligaciones.Condiciones})
}

// GuardarOrganizacion crea o sustituye el perfil (solo administración).
func (h *Handlers) GuardarOrganizacion(w http.ResponseWriter, r *http.Request) {
	var o store.Organizacion
	if err := httpx.DecodeJSON(w, r, &o); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := validation.New()
	o.Nombre = validation.TruncateRunes(strings.TrimSpace(o.Nombre), 200)
	if o.Nombre == "" {
		v.Add("nombre", "obligatorio")
	}
	o.NIF = validation.TruncateRunes(strings.TrimSpace(o.NIF), 40)
	o.Direccion = validation.TruncateRunes(strings.TrimSpace(o.Direccion), 200)
	o.CP = validation.TruncateRunes(strings.TrimSpace(o.CP), 20)
	o.Ciudad = validation.TruncateRunes(strings.TrimSpace(o.Ciudad), 100)
	o.Pais = strings.ToUpper(strings.TrimSpace(o.Pais))
	pais, hayPais := h.obligaciones.Pais(o.Pais)
	if !hayPais {
		v.Add("pais", "país no admitido")
	} else if _, ok := pais.Forma(o.Forma); !ok {
		v.Add("forma", "forma jurídica desconocida para ese país")
	}
	for k := range o.Condiciones {
		conocida := false
		for _, c := range obligaciones.Condiciones {
			if c == k {
				conocida = true
				break
			}
		}
		if !conocida {
			v.Add("condiciones", "condición desconocida "+k)
		}
	}
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, errorDeValidacion(err))
		return
	}
	guardada, err := h.store.GuardarOrganizacion(r.Context(), o, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, guardada)
}

// RequisitosFactura — GET /api/v1/organizacion/requisitos — qué tiene que tener
// una factura para valer en el país de la organización. Vacío donde no se ha escrito.
func (h *Handlers) RequisitosFactura(w http.ResponseWriter, r *http.Request) {
	o, err := h.store.Organizacion(r.Context())
	if err != nil {
		httpx.JSON(w, r, http.StatusOK, map[string]any{})
		return
	}
	pais, ok := h.obligaciones.Pais(o.Pais)
	if !ok || pais.RequisitosFactura == nil {
		httpx.JSON(w, r, http.StatusOK, map[string]any{})
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	httpx.JSON(w, r, http.StatusOK, pais.RequisitosFactura)
}

// --- Calendario de obligaciones --------------------------------------------

func (h *Handlers) perfil(w http.ResponseWriter, r *http.Request) (store.Organizacion, obligaciones.Perfil, bool) {
	o, err := h.store.Organizacion(r.Context())
	if errors.Is(err, store.ErrNoEncontrado) {
		httpx.Fail(w, r, errSinOrganizacion)
		return o, obligaciones.Perfil{}, false
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return o, obligaciones.Perfil{}, false
	}
	return o, h.obligaciones.PerfilDe(o.Pais, o.Forma, o.CP, o.Condiciones), true
}

func anioDe(r *http.Request) int {
	if s := r.URL.Query().Get("anio"); s != "" {
		if a, err := strconv.Atoi(s); err == nil && a >= 2000 && a <= 2100 {
			return a
		}
	}
	return time.Now().UTC().Year()
}

type instanciaVista struct {
	obligaciones.Instancia
	ConceptoRotulo string `json:"concepto_rotulo"`
	AmbitoRotulo   string `json:"ambito_rotulo"`
	// Pendientes es lo que falta en el ERP para presentarla, por comprobación
	// (banco, facturas, nóminas): el «te falta» de la fila.
	Pendientes map[string]int           `json:"pendientes,omitempty"`
	Proceso    *store.ProcesoObligacion `json:"proceso,omitempty"`
}

// vistas traduce las instancias y, si se dan, les pone lo que falta de su periodo.
func (h *Handlers) vistas(inst []obligaciones.Instancia, idioma string, pendientes map[string]map[string]int) []instanciaVista {
	out := make([]instanciaVista, 0, len(inst))
	for _, i := range inst {
		v := instanciaVista{
			Instancia:      i,
			ConceptoRotulo: h.cat.Rotulo("obligacion."+i.Concepto, idioma),
			AmbitoRotulo:   h.cat.Rotulo("ambito."+i.Ambito, idioma),
		}
		v.Pendientes = obligaciones.PendientesDe(i, pendientes)
		out = append(out, v)
	}
	return out
}

// Obligaciones devuelve el calendario del año para el perfil de la
// organización, con los rótulos en el idioma de la petición.
func (h *Handlers) Obligaciones(w http.ResponseWriter, r *http.Request) {
	o, perfil, ok := h.perfil(w, r)
	if !ok {
		return
	}
	anio := anioDe(r)
	inst, err := h.obligaciones.Calendario(perfil, anio)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnprocessable.Wrap(err).WithMessage(err.Error()))
		return
	}
	pais, _ := h.obligaciones.Pais(o.Pais)
	periodos := make([]string, 0, len(inst))
	for _, i := range inst {
		periodos = append(periodos, i.Periodo)
	}
	pendientes, procesos, err := h.store.PreparacionObligaciones(r.Context(),
		time.Date(anio, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(anio, 12, 31, 0, 0, 0, 0, time.UTC), periodos)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	vistas := h.vistas(inst, httpx.LanguageFromRequest(r), pendientes)
	for i := range vistas {
		if p, ok := procesos[vistas[i].Clave+":"+vistas[i].Periodo]; ok {
			vistas[i].Proceso = &p
		}
	}
	res := map[string]any{
		"anio":         anio,
		"pais":         o.Pais,
		"verificado":   pais.Verificado,
		"moneda":       pais.Moneda,
		"region":       perfil.Region,
		"obligaciones": vistas,
	}
	if t, ok := pais.Territorios[perfil.Region]; ok {
		res["territorio"] = t
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

// ObligacionesICS es el mismo calendario como fichero iCalendar, para importarlo
// en Sytue Calendar, Apple Calendar, Google u Outlook: este año y el siguiente, y
// una revisión semanal cada lunes con lo que hay que tener al día.
//
// Se IMPORTA, no se suscribe, a propósito: los calendarios suscritos por URL
// descartan las alarmas (Google siempre; Apple, por defecto), y lo que tiene que
// llegar a quien no sabe de impuestos es precisamente el aviso.
//
// Cada obligación va en SU fecha —la de la interfaz, días hábiles antes del
// límite legal—, con el aviso una semana antes y la víspera. El límite legal y el
// de domiciliación van en la descripción.
func (h *Handlers) ObligacionesICS(w http.ResponseWriter, r *http.Request) {
	o, perfil, ok := h.perfil(w, r)
	if !ok {
		return
	}
	anio := anioDe(r)
	var inst []obligaciones.Instancia
	for _, a := range []int{anio, anio + 1} {
		i, err := h.obligaciones.Calendario(perfil, a)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnprocessable.Wrap(err).WithMessage(err.Error()))
			return
		}
		inst = append(inst, i...)
	}
	idioma := httpx.LanguageFromRequest(r)
	tenant := ""
	if t, err := tenancy.From(r.Context()); err == nil {
		tenant = t.Slug
	}
	rotulo := func(clave string) string { return h.cat.Rotulo(clave, idioma) }
	eventos := obligaciones.EventosICS(inst, tenant, rotulo, time.Now())
	nombre := "ERP · " + o.Nombre
	cuerpo := obligaciones.ICS(nombre, eventos, time.Now())
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="obligaciones-%s-%d.ics"`, strings.ToLower(o.Pais), anio))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cuerpo)
}

// ObligacionCalculo saca del libro la cifra de una obligación en un periodo:
// el IVA a ingresar del trimestre, la base del pago fraccionado. Es la parte
// de «preparar la presentación» que el ERP puede hacer solo; la presentación en
// sí se hace en el portal, al que se enlaza.
func (h *Handlers) ObligacionCalculo(w http.ResponseWriter, r *http.Request) {
	o, perfil, ok := h.perfil(w, r)
	if !ok {
		return
	}
	clave := r.URL.Query().Get("clave")
	periodo := r.URL.Query().Get("periodo")
	desde, hasta, okPeriodo := obligaciones.RangoDePeriodo(periodo)
	if !okPeriodo {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid period."))
		return
	}
	pais, _ := h.obligaciones.Pais(o.Pais)
	ob, hay := pais.Obligacion(clave)
	if !hay || ob.Calculo == nil {
		httpx.Fail(w, r, errNoCalculable)
		return
	}
	if _, err := h.obligaciones.Aplican(perfil); err != nil {
		httpx.Fail(w, r, httpx.ErrUnprocessable.Wrap(err).WithMessage(err.Error()))
		return
	}
	res, err := ob.Calculo.Calcular(r.Context(), clave, periodo, desde, hasta, func(ctx context.Context, desde, hasta time.Time) ([]obligaciones.Saldo, error) {
		saldos, err := h.store.BalanceEntre(ctx, desde, hasta)
		out := make([]obligaciones.Saldo, 0, len(saldos))
		for _, x := range saldos {
			out = append(out, obligaciones.Saldo{Cuenta: x.Cuenta, Debe: x.Debe, Haber: x.Haber})
		}
		return out, err
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, res)
}
