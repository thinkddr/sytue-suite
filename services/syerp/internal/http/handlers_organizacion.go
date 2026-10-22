// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/obligaciones"
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
		if desde, hasta, ok := obligaciones.RangoDePeriodo(i.Periodo); ok && pendientes != nil {
			for _, prep := range i.Preparacion {
				n := 0
				for mes, cuantos := range pendientes[prep] {
					if mes >= desde.Format("2006-01") && mes <= hasta.Format("2006-01") {
						n += cuantos
					}
				}
				if v.Pendientes == nil {
					v.Pendientes = map[string]int{}
				}
				v.Pendientes[prep] = n
			}
		}
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
	eventos := []obligaciones.Evento{{
		UID:         tenant + "-revision-semanal@erp.sytue.eu",
		Fecha:       proximoLunes(time.Now().UTC()),
		Resumen:     rotulo("aviso.revision_semanal"),
		Descripcion: rotulo("aviso.revision_semanal_pasos"),
		Repetir:     "FREQ=WEEKLY;BYDAY=MO",
	}}
	for _, i := range h.vistas(inst, idioma, nil) {
		eventos = append(eventos, obligaciones.Evento{
			UID:         fmt.Sprintf("%s-%s-%s@erp.sytue.eu", tenant, i.Clave, i.Periodo),
			Fecha:       i.Recomendada,
			Resumen:     i.Nombre + " · " + obligaciones.EtiquetaPeriodo(i.Periodo),
			Descripcion: descripcionICS(i, rotulo),
			URL:         i.Portal.URL,
			Categoria:   i.AmbitoRotulo,
			AvisoDias:   7,
		})
	}
	nombre := "ERP · " + o.Nombre
	cuerpo := obligaciones.ICS(nombre, eventos, time.Now())
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="obligaciones-%s-%d.ics"`, strings.ToLower(o.Pais), anio))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cuerpo)
}

// descripcionICS es lo que se lee al abrir el evento: qué es, las fechas que
// cuentan, qué hay que tener y dónde se presenta.
func descripcionICS(i instanciaVista, rotulo func(string) string) string {
	var b strings.Builder
	b.WriteString(i.ConceptoRotulo + " · " + i.AmbitoRotulo)
	b.WriteString("\n" + rotulo("aviso.tu_fecha") + ": " + i.Recomendada.Format(time.DateOnly))
	if i.LimiteDomiciliacion != nil {
		b.WriteString("\n" + rotulo("aviso.limite_domiciliacion") + ": " + i.LimiteDomiciliacion.Format(time.DateOnly))
	}
	b.WriteString("\n" + rotulo("aviso.limite_legal") + ": " + i.Limite.Format(time.DateOnly))
	if g := i.Guia; g != nil {
		b.WriteString("\n\n" + g.Que)
		for _, n := range g.Necesitas {
			b.WriteString("\n• " + n)
		}
	}
	b.WriteString("\n\n" + i.Portal.Nombre + ": " + i.Portal.URL)
	if i.Sancion != nil && i.Sancion.Nota != "" {
		b.WriteString("\n" + i.Sancion.Nota)
	}
	return b.String()
}

// proximoLunes es el lunes de esta semana si hoy es lunes, o el siguiente.
func proximoLunes(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return d.AddDate(0, 0, (8-int(d.Weekday()))%7)
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
	// Acumulado (el 130): el resultado desde el 1 de enero al cierre de cada
	// periodo del año hasta este, para restar lo que ya se pagó en los anteriores.
	cierres := []time.Time{hasta}
	if ob.Calculo.Acumulado {
		cierres = cierres[:0]
		// El día 0 de un mes es el último del anterior: 31 de marzo, 30 de junio…
		for q := 1; !time.Date(desde.Year(), time.Month(3*q+1), 0, 0, 0, 0, 0, time.UTC).After(hasta); q++ {
			cierres = append(cierres, time.Date(desde.Year(), time.Month(3*q+1), 0, 0, 0, 0, 0, time.UTC))
		}
		desde = time.Date(desde.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	}
	var haber, debe int64
	acumulados := make([]int64, 0, len(cierres))
	asentado := false
	for _, cierre := range cierres {
		saldos, err := h.store.BalanceEntre(r.Context(), desde, cierre)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		// Saldo acreedor de las cuentas «haber» menos saldo deudor de las «debe»:
		// para el IVA, 477 (repercutido) menos 472 (soportado). Sobre ingresos y
		// gastos (7 y 6) es el resultado del periodo.
		haber, debe = 0, 0
		for _, s := range saldos {
			if tienePrefijo(s.Cuenta, ob.Calculo.Haber) {
				haber += s.Haber - s.Debe
			}
			if tienePrefijo(s.Cuenta, ob.Calculo.Debe) {
				debe += s.Debe - s.Haber
			}
		}
		acumulados = append(acumulados, haber-debe)
		asentado = len(saldos) > 0
	}
	resultado := haber - debe
	switch {
	case ob.Calculo.Acumulado:
		resultado = obligaciones.PagoFraccionado(acumulados, ob.Calculo.Porcentaje)
	case ob.Calculo.Tipo == "porcentaje_resultado":
		resultado = int64(float64(resultado) * ob.Calculo.Porcentaje / 100)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"clave": clave, "periodo": periodo,
		"desde": desde.Format("2006-01-02"), "hasta": hasta.Format("2006-01-02"),
		"tipo": ob.Calculo.Tipo, "porcentaje": ob.Calculo.Porcentaje,
		"haber_centimos": haber, "debe_centimos": debe, "resultado_centimos": resultado,
		"asentado": asentado,
	})
}

func tienePrefijo(cuenta string, prefijos []string) bool {
	for _, p := range prefijos {
		if strings.HasPrefix(cuenta, p) {
			return true
		}
	}
	return false
}
