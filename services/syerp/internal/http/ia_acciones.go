// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/ia"
	"git.sytue.eu/sytue/one/services/syerp/internal/obligaciones"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
	"git.sytue.eu/sytue/one/services/syerp/internal/web"
)

// Sytue AI que HACE: una frase se convierte en registros.
//
//   - POST /ia/acciones: la frase → un plan (qué se crea, qué ya existe, qué falta),
//     con lo comprobable ya comprobado en internet.
//   - POST /ia/acciones/crear: el plan que la persona ha visto → los registros, en
//     una transacción, y la tarea de subir la factura si es un gasto.
//   - POST /banco/pistas: dónde conseguir la factura de cada cargo del banco.

var errIASinPlan = httpx.NewError(http.StatusUnprocessableEntity, "ia_sin_plan",
	"Sytue AI could not turn that into records. Try saying it another way.")

// placeholderRelacion ocupa, al validar, el sitio de una relación a un registro
// del plan que aún no existe.
const placeholderRelacion = "00000000-0000-4000-8000-000000000000"

// registroCorto es un registro que ya existe, tal como se enseña en el plan.
type registroCorto struct {
	ID       string `json:"id"`
	Etiqueta string `json:"etiqueta"`
}

// pasoPlan es un paso del plan ya comprobado.
type pasoPlan struct {
	Ref       string            `json:"ref"`
	Modelo    string            `json:"modelo"`
	Etiqueta  string            `json:"etiqueta,omitempty"`
	Existente *registroCorto    `json:"existente,omitempty"`
	Data      map[string]any    `json:"data"`
	Enlaces   map[string]string `json:"enlaces,omitempty"`
	Etiquetas map[string]string `json:"etiquetas,omitempty"`
	Faltan    []string          `json:"faltan,omitempty"`
	Avisos    []string          `json:"avisos,omitempty"`
	// Tarea es el resumen de la tarea que se creará con el registro.
	Tarea string `json:"tarea,omitempty"`
}

// ProponerAcciones — POST /api/v1/ia/acciones {"peticion": "…"}.
func (h *Handlers) ProponerAcciones(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(plazoPropuesta))
	var c struct {
		Peticion string `json:"peticion"`
	}
	if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c.Peticion = strings.TrimSpace(c.Peticion)
	if c.Peticion == "" || len([]rune(c.Peticion)) > maxPeticion {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Say what you need in up to 2000 characters."))
		return
	}
	ctx, idioma := r.Context(), httpx.LanguageFromRequest(r)
	var vistas []catalogo.ModeloVista
	for _, mod := range h.cat.Modulos() {
		if !h.puede(ctx, mod.Clave, permisos.Usuario) {
			continue
		}
		for _, m := range mod.Modelos {
			vistas = append(vistas, h.cat.VistaModelo(m, idioma))
		}
	}
	if len(vistas) == 0 {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	texto, err := h.ia.Pedir(ctx, tokenDe(r), asistente.Peticion{
		Action: "acciones", Kind: "erp", Modo: "calidad", Esfuerzo: "high",
		Instruction: ia.InstruccionAcciones(c.Peticion, h.organizacionIA(ctx), time.Now().UTC()),
		Context:     ia.ContextoModelos(vistas),
	})
	if err != nil {
		log.From(ctx).Warn("sytue ai: acciones sin respuesta", "error", err)
		httpx.Fail(w, r, errorDeIA(err))
		return
	}
	plan, err := ia.PlanDe(texto)
	if err != nil {
		httpx.Fail(w, r, errIASinPlan)
		return
	}
	pasos := h.prepararPlan(ctx, plan, idioma)
	if len(pasos) == 0 {
		httpx.Fail(w, r, errIASinPlan)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"resumen": plan.Resumen, "avisos": plan.Avisos, "pasos": pasos})
}

func (h *Handlers) organizacionIA(ctx context.Context) ia.Organizacion {
	o, err := h.store.Organizacion(ctx)
	if err != nil {
		return ia.Organizacion{}
	}
	org := ia.Organizacion{Nombre: o.Nombre, NIF: o.NIF, Pais: o.Pais, Forma: o.Forma, Intracomunitario: o.Condiciones["intracomunitario"]}
	if pais, ok := h.obligaciones.Pais(o.Pais); ok {
		if f, ok := pais.Forma(o.Forma); ok {
			org.Forma, org.PersonaFisica = f.Nombre, f.Tipo == obligaciones.TipoPersonaFisica
		}
	}
	return org
}

// maestros son los modelos en los que se busca antes de crear aunque el modelo
// no lo pida: un segundo «Anthropic» en contactos es un error que se ve en el 347.
var maestros = []string{"contacto", "producto", "empleado"}

// prepararPlan comprueba cada paso contra el catálogo, los permisos, la base e
// internet. Los pasos de un modelo que no existe o no se puede escribir se caen.
func (h *Handlers) prepararPlan(ctx context.Context, plan ia.Plan, idioma string) []pasoPlan {
	refs := map[string]bool{}
	out := make([]pasoPlan, 0, len(plan.Pasos))
	for _, p := range plan.Pasos {
		m, ok := h.cat.Modelo(p.Modelo)
		if !ok || !h.puede(ctx, m.Modulo, permisos.Usuario) {
			continue
		}
		out = append(out, h.prepararPaso(ctx, m, p, idioma, refs))
		refs[p.Ref] = true
	}
	return out
}

func (h *Handlers) prepararPaso(ctx context.Context, m *catalogo.Modelo, p ia.PasoIA, idioma string, refs map[string]bool) pasoPlan {
	vista := h.cat.VistaModelo(m, idioma)
	data, relaciones := ia.FichaDe(m, vista, string(p.Datos))
	paso := pasoPlan{Ref: p.Ref, Modelo: m.Clave, Etiqueta: vista.Etiqueta, Data: data}
	for campo, nombre := range relaciones {
		if ref, ok := ia.Referencia(nombre); ok {
			if refs[ref] {
				paso.enlazar(campo, ref)
			}
			continue
		}
		f, _ := campoDe(m, campo)
		pag, err := h.store.Listar(ctx, store.Filtro{Modelo: f.Modelo, Texto: nombre, Limite: 1})
		if err != nil || len(pag.Registros) == 0 {
			paso.Avisos = append(paso.Avisos, "No encuentro «"+nombre+"»: elígelo después en la ficha.")
			continue
		}
		reg := pag.Registros[0]
		paso.Data[campo] = reg.ID
		paso.etiquetar(reg.ID, cmp.Or(reg.Etiqueta, reg.Ref))
	}

	buscar := strings.TrimSpace(p.Buscar)
	if buscar == "" && slices.Contains(maestros, m.Clave) {
		buscar, _ = data[m.Titulo].(string)
	}
	if buscar != "" {
		if reg, ok := h.buscarExistente(ctx, m, buscar, data); ok {
			paso.Existente = &registroCorto{ID: reg.ID, Etiqueta: cmp.Or(reg.Etiqueta, reg.Ref)}
			return paso
		}
	}

	h.comprobarEnInternet(ctx, m, &paso)
	enlaces := map[string]any{}
	for campo := range paso.Enlaces {
		enlaces[campo] = placeholderRelacion
	}
	if _, _, err := domain.Validar(m, jsonDe(paso.Data, enlaces)); err != nil {
		if v, ok := validation.AsErrors(err); ok {
			for _, f := range v.Fields {
				paso.Faltan = append(paso.Faltan, f.Field)
			}
		}
	}
	if llevaFactura(m) {
		paso.Tarea = tareaFactura(m)
	}
	return paso
}

func (p *pasoPlan) enlazar(campo, ref string) {
	if p.Enlaces == nil {
		p.Enlaces = map[string]string{}
	}
	p.Enlaces[campo] = ref
}

func (p *pasoPlan) etiquetar(id, nombre string) {
	if p.Etiquetas == nil {
		p.Etiquetas = map[string]string{}
	}
	p.Etiquetas[id] = nombre
}

// buscarExistente: por NIF si lo trae, por nombre exacto y, si no, por un nombre
// que contenga al otro («Anthropic» y «Anthropic Ireland, Limited»).
func (h *Handlers) buscarExistente(ctx context.Context, m *catalogo.Modelo, buscar string, data map[string]any) (store.Registro, bool) {
	if nif, _ := data["nif"].(string); nif != "" {
		if id, err := h.store.BuscarPorNIF(ctx, m.Clave, nif); err == nil {
			if reg, err := h.store.Obtener(ctx, m.Clave, id); err == nil {
				return reg, true
			}
		}
	}
	if id, err := h.store.BuscarPorEtiqueta(ctx, m.Clave, buscar); err == nil {
		if reg, err := h.store.Obtener(ctx, m.Clave, id); err == nil {
			return reg, true
		}
	}
	pag, err := h.store.Listar(ctx, store.Filtro{Modelo: m.Clave, Texto: buscar, Limite: 1})
	if err != nil || len(pag.Registros) == 0 {
		return store.Registro{}, false
	}
	a, b := strings.ToLower(pag.Registros[0].Etiqueta), strings.ToLower(buscar)
	if a != "" && (strings.Contains(a, b) || strings.Contains(b, a)) {
		return pag.Registros[0], true
	}
	return store.Registro{}, false
}

// comprobarEnInternet quita del paso lo que internet dice que no existe: un
// NIF-IVA que el VIES no conoce, una web que no responde, un correo sin buzón.
func (h *Handlers) comprobarEnInternet(ctx context.Context, m *catalogo.Modelo, paso *pasoPlan) {
	if h.web == nil {
		return
	}
	ctx, cancelar := context.WithTimeout(ctx, 20*time.Second)
	defer cancelar()
	for _, f := range m.Campos {
		valor, _ := paso.Data[f.Clave].(string)
		if valor == "" {
			continue
		}
		switch {
		case f.Clave == "nif":
			if pais, _, ok := web.PartirNIFIVA(valor); ok && pais != "ES" {
				r := h.web.IVA(ctx, valor)
				switch {
				case r.Comprobado && !r.Valido:
					delete(paso.Data, f.Clave)
					paso.Avisos = append(paso.Avisos, "El NIF-IVA "+valor+" no existe en el censo europeo (VIES): lo he quitado. Cópialo de la factura.")
				case r.Valido && r.Nombre != "":
					paso.Avisos = append(paso.Avisos, "NIF-IVA comprobado en el VIES: "+r.Nombre+".")
				}
			}
		case f.Tipo == catalogo.TipoCorreo && !h.web.Correo(ctx, valor):
			delete(paso.Data, f.Clave)
			paso.Avisos = append(paso.Avisos, "El correo "+valor+" no tiene dónde recibir: lo he quitado.")
		case f.Tipo == catalogo.TipoURL && !h.web.URL(ctx, valor):
			delete(paso.Data, f.Clave)
			paso.Avisos = append(paso.Avisos, "La web "+valor+" no responde: la he quitado.")
		}
	}
}

// llevaFactura: los documentos de gasto, que sin su factura no desgravan.
func llevaFactura(m *catalogo.Modelo) bool {
	return m.Contable != nil && !m.Contable.Cobro
}

func tareaFactura(m *catalogo.Modelo) string {
	if m.Clave == "nomina" {
		return "Sube la nómina firmada en PDF"
	}
	return "Sube la factura en PDF"
}

func jsonDe(partes ...map[string]any) json.RawMessage {
	todo := map[string]any{}
	for _, p := range partes {
		for k, v := range p {
			todo[k] = v
		}
	}
	crudo, _ := json.Marshal(todo)
	return crudo
}

// CrearAcciones — POST /api/v1/ia/acciones/crear {"pasos": [...]}: crea lo que la
// persona ha visto. El plan vuelve del navegador, así que se valida entero otra
// vez: no es más de lo que la persona podría crear a mano con sus permisos.
func (h *Handlers) CrearAcciones(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Pasos []pasoPlan `json:"pasos"`
	}
	if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(c.Pasos) == 0 || len(c.Pasos) > ia.MaxPasos {
		httpx.Fail(w, r, errIASinPlan)
		return
	}
	ctx := r.Context()
	claims, _ := authn.FromContext(ctx)
	hoy := time.Now().In(madrid).Format(time.DateOnly)
	existentes := map[string]string{} // ref → id de un registro que ya estaba
	indices := map[string]int{}       // ref → posición entre los nuevos
	var nuevos []store.Nuevo
	var tareas []store.ActividadDe
	for _, p := range c.Pasos {
		m, ok := h.cat.Modelo(p.Modelo)
		if !ok || !h.puede(ctx, m.Modulo, permisos.Usuario) {
			httpx.Fail(w, r, errSinPermiso)
			return
		}
		if p.Existente != nil {
			reg, err := h.store.Obtener(ctx, m.Clave, p.Existente.ID)
			if err != nil {
				httpx.Fail(w, r, errorDeEscritura(err))
				return
			}
			existentes[p.Ref] = reg.ID
			continue
		}
		enlaces, relleno := map[string]int{}, map[string]any{}
		for campo, ref := range p.Enlaces {
			if id, ok := existentes[ref]; ok {
				relleno[campo] = id
			} else if i, ok := indices[ref]; ok {
				enlaces[campo], relleno[campo] = i, placeholderRelacion
			}
		}
		data, etiqueta, err := domain.Validar(m, jsonDe(p.Data, relleno))
		if err != nil {
			httpx.Fail(w, r, errorDeValidacion(err))
			return
		}
		indices[p.Ref] = len(nuevos)
		nuevos = append(nuevos, store.Nuevo{Modelo: m, Data: data, Enlaces: enlaces, Etiqueta: etiqueta})
		if llevaFactura(m) {
			tareas = append(tareas, store.ActividadDe{Indice: len(nuevos) - 1, Actividad: store.Actividad{
				Tipo: "subir_documento", Resumen: tareaFactura(m), Vence: hoy,
				ResponsableSub: claims.Subject, ResponsableNombre: cmp.Or(claims.Name, claims.Email),
			}})
		}
	}
	if len(nuevos) == 0 {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"creados": []store.Registro{}, "actividades": []store.Actividad{}})
		return
	}
	creados, actividades, err := h.store.CrearVarios(ctx, nuevos, tareas, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	for _, a := range actividades {
		h.avisarTarea(ctx, a)
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"creados": creados, "actividades": actividades})
}

var madrid = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Madrid"); err == nil {
		return l
	}
	return time.UTC
}()

// avisarTarea deja la tarea en la campana de quien la tiene, por la puerta interna del
// Hub (antes iba con el token de quien la pedía, y así solo podía avisarse a sí mismo).
// Si la pone otra persona, el aviso dice quién; si la crea Sytue AI para quien se la
// pidió, el título es la tarea. Si el Hub no responde, la tarea sigue ahí.
func (h *Handlers) avisarTarea(ctx context.Context, a store.Actividad) {
	if h.hub == nil || a.ResponsableSub == "" {
		return
	}
	t, _ := tenancy.From(ctx)
	actor := ""
	if c, ok := authn.FromContext(ctx); ok && c.Subject != a.ResponsableSub {
		actor = cmp.Or(strings.TrimSpace(c.Name), c.Email)
	}
	h.hub.SendAsync(ctx, avisoDeTarea(h.cfg.PublicURL, t.Slug, a, actor))
}

// avisoDeTarea es el aviso de una tarea a quien la tiene: el mismo en vivo que en la
// reconstrucción de avisos (AvisosHistoricos). Sin `actor`, el título es la tarea.
func avisoDeTarea(publicURL, tenant string, a store.Actividad, actor string) hubnotify.Notification {
	titulo, params := a.Resumen, map[string]string{"title": a.Resumen}
	if actor != "" {
		titulo, params["actor"] = actor+" te ha asignado «"+a.Resumen+"»", actor
	}
	return hubnotify.Notification{
		Tenant:  tenant,
		Subject: a.ResponsableSub,
		Source:  "erp",
		Type:    "erp.task.assigned",
		Level:   "info",
		Title:   titulo,
		Body:    strings.Trim(cmp.Or(a.RegistroEtiqueta, a.Modelo)+" · "+a.Vence, " ·"),
		URL:     hubnotify.EnlaceTareas(publicURL, tenant),
		Params:  params,
	}
}

// PistasBanco — POST /api/v1/banco/pistas: Sytue AI (el modelo de calidad) mira
// los cargos sin factura y dice dónde conseguirla; lo que sugiere se comprueba en
// internet antes de guardarlo.
func (h *Handlers) PistasBanco(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(plazoPropuesta))
	ctx := r.Context()
	cargos, err := h.store.CargosSinPista(ctx, 15)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(cargos) == 0 {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"buscadas": 0})
		return
	}
	movs := make([]ia.MovimientoPista, len(cargos))
	porClave, pedidas := map[string]store.LineaBanco{}, map[string]bool{}
	for i, c := range cargos {
		clave := "m" + strconv.Itoa(i+1)
		movs[i] = ia.MovimientoPista{ID: clave, Fecha: c.Fecha, Descripcion: c.Descripcion, Centimos: c.Centimos}
		porClave[clave], pedidas[clave] = c, true
	}
	texto, err := h.ia.Pedir(ctx, tokenDe(r), asistente.Peticion{
		Action: "pistas_factura", Kind: "erp", Modo: "calidad", Esfuerzo: "high",
		Instruction: ia.InstruccionPistas(), Context: ia.ContextoPistas(movs),
	})
	if err != nil {
		log.From(ctx).Warn("sytue ai: pistas sin respuesta", "error", err)
		httpx.Fail(w, r, errorDeIA(err))
		return
	}
	pistas := ia.PistasDe(texto, pedidas)
	h.comprobarPistas(ctx, pistas)
	respondidas := map[string]bool{}
	for _, p := range pistas {
		respondidas[p.ID] = true
		if err := h.store.GuardarPista(ctx, porClave[p.ID].ID, p.Empresa, p.Web, p.URLFactura, p.Email); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	// Los que el modelo no contestó quedan marcados como mirados, sin pista.
	for clave, c := range porClave {
		if !respondidas[clave] {
			_ = h.store.GuardarPista(ctx, c.ID, "", "", "", "")
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"buscadas": len(cargos)})
}

// comprobarPistas deja en cada pista solo lo que existe, en paralelo y con
// tope: quince cargos con tres comprobaciones cada uno no pueden tardar un minuto.
func (h *Handlers) comprobarPistas(ctx context.Context, pistas []ia.Pista) {
	if h.web == nil {
		return
	}
	ctx, cancelar := context.WithTimeout(ctx, 25*time.Second)
	defer cancelar()
	var wg sync.WaitGroup
	turno := make(chan struct{}, 6)
	for i := range pistas {
		wg.Add(1)
		go func(p *ia.Pista) {
			defer wg.Done()
			turno <- struct{}{}
			defer func() { <-turno }()
			if p.URLFactura != "" && !h.web.URL(ctx, p.URLFactura) {
				p.URLFactura = ""
			}
			if p.Web != "" && !h.web.URL(ctx, p.Web) {
				p.Web = ""
			}
			if p.Email != "" && !h.web.Correo(ctx, p.Email) {
				p.Email = ""
			}
		}(&pistas[i])
	}
	wg.Wait()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		log.From(ctx).Info("pistas: comprobaciones cortadas por tiempo")
	}
}
