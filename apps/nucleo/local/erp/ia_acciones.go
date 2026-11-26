// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/ia"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/obligaciones"
	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
	"git.sytue.eu/sytue/one/apps/nucleo/validation"
)

// La IA del ERP que HACE, con la de la persona (PonerIA) y los mismos prompts y
// lectores de apps/nucleo/erp/ia que el servidor:
//
//   - POST /ia/acciones: una frase → un plan de registros (qué se crea, qué ya existe,
//     qué falta). Sin comprobaciones en internet: la edición local no sale a la red por
//     su cuenta (R-RED).
//   - POST /ia/acciones/crear: el plan que la persona ha visto → los registros en una
//     transacción, y la tarea de subir la factura de cada gasto.
//   - POST /banco/pistas: dónde conseguir la factura de cada cargo sin justificar.
//   - POST /{modelo}/ia/rellenar con un PDF: su texto con el motor PDF del núcleo.

// placeholderRelacion ocupa, al validar, el sitio de una relación a un paso del plan
// que aún no existe.
const placeholderRelacion = "00000000-0000-4000-8000-000000000000"

// maxPDFFicha es el tope del PDF que se lee para rellenar una ficha, el del servidor.
const maxPDFFicha = 10 << 20

// maestros son los modelos en los que se busca antes de crear aunque el plan no lo
// pida: un segundo «Anthropic» en contactos es un error que se ve en el 347.
var maestros = []string{"contacto", "producto", "empleado"}

func iaSinPlan(w http.ResponseWriter) {
	fallo(w, http.StatusUnprocessableEntity, "ia_sin_plan", "Sytue AI could not turn that into records. Try saying it another way.", nil)
}

func (s *Servidor) rutasIA(h func(string, http.HandlerFunc)) {
	h("POST /api/v1/ia/acciones", s.proponerAcciones)
	h("POST /api/v1/ia/acciones/crear", s.crearAcciones)
	h("POST /api/v1/banco/pistas", s.pistasBanco)
}

// textoDePDF es el texto de un PDF en orden de lectura, con el motor local. Un PDF sin
// texto (un escaneo) es un error, no un texto vacío que la IA intentaría rellenar.
func textoDePDF(ctx context.Context, datos []byte) (string, error) {
	doc, err := localpdf.LeerPDF(datos)
	if err != nil {
		return "", err
	}
	paginas, err := localpdf.TextSpans(ctx, doc)
	if err != nil {
		return "", err
	}
	frag := make([][]ia.FragmentoPDF, 0, len(paginas))
	for _, p := range paginas {
		fs := make([]ia.FragmentoPDF, 0, len(p))
		for _, t := range p {
			fs = append(fs, ia.FragmentoPDF{Texto: t.Texto, X: t.X, Y: t.Y})
		}
		frag = append(frag, fs)
	}
	texto := ia.TextoDePDF(frag)
	if texto == "" {
		return "", errors.New("erp: el PDF no tiene texto")
	}
	return texto, nil
}

// leerPDFFicha lee el cuerpo como PDF; si devuelve false, ya ha respondido.
func leerPDFFicha(w http.ResponseWriter, r *http.Request) (string, bool) {
	datos, err := io.ReadAll(io.LimitReader(r.Body, maxPDFFicha+1))
	if err != nil || len(datos) > maxPDFFicha {
		fallo(w, http.StatusBadRequest, "bad_request", "Send a PDF of up to 10 MB.", nil)
		return "", false
	}
	texto, err := textoDePDF(r.Context(), datos)
	if err != nil {
		fallo(w, http.StatusUnprocessableEntity, "pdf_sin_texto", "This PDF could not be read. If it is a photo or a scan, paste its text instead.", nil)
		return "", false
	}
	return validation.TruncateRunes(texto, maxTextoFicha), true
}

// modelosEscribibles son las vistas de los modelos de los módulos encendidos (todos,
// si aún no hay ninguno encendido).
func (s *Servidor) modelosEscribibles(ctx context.Context, idioma string) ([]catalogo.ModeloVista, error) {
	activos, err := s.alm.ModulosActivos(ctx)
	if err != nil {
		return nil, err
	}
	var vistas []catalogo.ModeloVista
	for _, mod := range s.cat.Modulos() {
		if len(activos) > 0 && !slices.Contains(activos, mod.Clave) {
			continue
		}
		for _, m := range mod.Modelos {
			vistas = append(vistas, s.cat.VistaModelo(m, idioma))
		}
	}
	return vistas, nil
}

func (s *Servidor) organizacionIA(ctx context.Context) ia.Organizacion {
	o, err := s.alm.Organizacion(ctx)
	if err != nil {
		return ia.Organizacion{}
	}
	org := ia.Organizacion{Nombre: o.Nombre, NIF: o.NIF, Pais: o.Pais, Forma: o.Forma, Intracomunitario: o.Condiciones["intracomunitario"]}
	if pais, ok := s.obl.Pais(o.Pais); ok {
		if f, ok := pais.Forma(o.Forma); ok {
			org.Forma, org.PersonaFisica = f.Nombre, f.Tipo == obligaciones.TipoPersonaFisica
		}
	}
	return org
}

// proponerAcciones — POST /ia/acciones {"peticion": "…"}. No crea nada.
func (s *Servidor) proponerAcciones(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Peticion string `json:"peticion"`
	}
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	c.Peticion = strings.TrimSpace(c.Peticion)
	if c.Peticion == "" || len([]rune(c.Peticion)) > maxPeticionIA {
		fallo(w, http.StatusBadRequest, "bad_request", "Say what you need in up to 2000 characters.", nil)
		return
	}
	ctx, l := r.Context(), idioma(r)
	vistas, err := s.modelosEscribibles(ctx, l)
	if err != nil {
		falloDe(w, err)
		return
	}
	texto, ok := s.pedirIA(w, r, api.PeticionIA{
		Accion: "acciones", Tipo: "erp", Modo: "calidad", Esfuerzo: "high",
		Instruccion: ia.InstruccionAcciones(c.Peticion, s.organizacionIA(ctx), s.alm.ahora().UTC()), Contexto: ia.ContextoModelos(vistas),
	})
	if !ok {
		return
	}
	plan, err := ia.PlanDe(texto)
	if err != nil {
		iaSinPlan(w)
		return
	}
	permitidos := map[string]bool{}
	for _, v := range vistas {
		permitidos[v.Clave] = true
	}
	refs := map[string]bool{}
	pasos := make([]api.ERPPasoPlan, 0, len(plan.Pasos))
	for _, p := range plan.Pasos {
		m, ok := s.cat.Modelo(p.Modelo)
		if !ok || !permitidos[m.Clave] {
			continue
		}
		paso, err := s.prepararPaso(ctx, m, p, l, refs)
		if err != nil {
			falloDe(w, err)
			return
		}
		pasos = append(pasos, paso)
		refs[p.Ref] = true
	}
	if len(pasos) == 0 {
		iaSinPlan(w)
		return
	}
	responder(w, http.StatusOK, api.ERPPlan{Resumen: plan.Resumen, Avisos: plan.Avisos, Pasos: pasos})
}

func etiquetar(p *api.ERPPasoPlan, id, nombre string) {
	if p.Etiquetas == nil {
		p.Etiquetas = map[string]string{}
	}
	p.Etiquetas[id] = nombre
}

// prepararPaso comprueba un paso contra el catálogo y la base: resuelve las
// relaciones por nombre, encuentra lo que ya existe y dice qué falta.
func (s *Servidor) prepararPaso(ctx context.Context, m *catalogo.Modelo, p ia.PasoIA, l string, refs map[string]bool) (api.ERPPasoPlan, error) {
	vista := s.cat.VistaModelo(m, l)
	data, relaciones := ia.FichaDe(m, vista, string(p.Datos))
	paso := api.ERPPasoPlan{Ref: p.Ref, Modelo: m.Clave, Etiqueta: vista.Etiqueta, Data: data}
	if err := s.resolverRelaciones(ctx, m, &paso, relaciones, refs); err != nil {
		return paso, err
	}
	buscar := strings.TrimSpace(p.Buscar)
	if buscar == "" && slices.Contains(maestros, m.Clave) {
		buscar, _ = data[m.Titulo].(string)
	}
	if buscar != "" {
		reg, hay, err := s.alm.existente(ctx, m.Clave, buscar, data)
		if err != nil {
			return paso, err
		}
		if hay {
			paso.Existente = &api.ERPRegistroCorto{ID: reg.ID, Etiqueta: cmp.Or(reg.Etiqueta, reg.Ref)}
			return paso, nil
		}
	}
	enlaces := map[string]any{}
	for campo := range paso.Enlaces {
		enlaces[campo] = placeholderRelacion
	}
	if _, _, err := dominio.Validar(m, jsonDe(paso.Data, enlaces)); err != nil {
		if v, ok := validation.AsErrors(err); ok {
			for _, f := range v.Fields {
				paso.Faltan = append(paso.Faltan, f.Field)
			}
		}
	}
	if llevaFactura(m) {
		paso.Tarea = tareaFactura(m)
	}
	return paso, nil
}

// resolverRelaciones rellena las relaciones del paso: con otro paso del plan (refs) si
// lo nombra por su referencia, con el registro que se le parece si no, y con un aviso
// si no encuentra ninguno.
func (s *Servidor) resolverRelaciones(ctx context.Context, m *catalogo.Modelo, paso *api.ERPPasoPlan, relaciones map[string]string, refs map[string]bool) error {
	for campo, nombre := range relaciones {
		if ref, ok := ia.Referencia(nombre); ok {
			if refs[ref] {
				if paso.Enlaces == nil {
					paso.Enlaces = map[string]string{}
				}
				paso.Enlaces[campo] = ref
			}
			continue
		}
		destino := ""
		for _, f := range m.Campos {
			if f.Clave == campo {
				destino = f.Modelo
			}
		}
		reg, hay, err := s.alm.parecido(ctx, destino, nombre)
		if err != nil {
			return err
		}
		if !hay {
			paso.Avisos = append(paso.Avisos, "No encuentro «"+nombre+"»: elígelo después en la ficha.")
			continue
		}
		paso.Data[campo] = reg.ID
		etiquetar(paso, reg.ID, cmp.Or(reg.Etiqueta, reg.Ref))
	}
	return nil
}

// parecido es el registro vivo de `modelo` cuyo nombre contiene el texto (o al revés),
// el más antiguo primero; como la búsqueda de la lista.
func (a *Almacen) parecido(ctx context.Context, modelo, texto string) (api.ERPRegistro, bool, error) {
	if modelo == "" || strings.TrimSpace(texto) == "" {
		return api.ERPRegistro{}, false, nil
	}
	regs, err := a.listarTx(ctx, a.db, modelo, false, "")
	if err != nil {
		return api.ERPRegistro{}, false, err
	}
	q := plegar(strings.TrimSpace(texto))
	for i := len(regs) - 1; i >= 0; i-- {
		if coincide(regs[i], q) {
			return regs[i], true, nil
		}
	}
	return api.ERPRegistro{}, false, nil
}

// existente: por NIF si lo trae, por nombre exacto y, si no, por un nombre que
// contenga al otro («Anthropic» y «Anthropic Ireland, Limited»).
func (a *Almacen) existente(ctx context.Context, modelo, buscar string, data map[string]any) (api.ERPRegistro, bool, error) {
	if nif, _ := data["nif"].(string); nif != "" && modelo == "contacto" {
		if c, hay, err := a.contactoPorNIF(ctx, nif); err != nil || hay {
			return c, hay, err
		}
	}
	if id, err := a.BuscarPorEtiqueta(ctx, modelo, buscar); err == nil {
		r, err := a.Obtener(ctx, modelo, id)
		return r, err == nil, err
	} else if !errors.Is(err, ErrNoEncontrado) {
		return api.ERPRegistro{}, false, err
	}
	r, hay, err := a.parecido(ctx, modelo, buscar)
	if err != nil || !hay {
		return r, false, err
	}
	x, y := strings.ToLower(r.Etiqueta), strings.ToLower(buscar)
	return r, x != "" && (strings.Contains(x, y) || strings.Contains(y, x)), nil
}

// llevaFactura: los documentos de gasto, que sin su factura no desgravan.
func llevaFactura(m *catalogo.Modelo) bool { return m.Contable != nil && !m.Contable.Cobro }

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

// crearPasos crea en la transacción los pasos del plan, en orden; de los que ya estaban
// solo se toma el id, para los que los citan. Devuelve lo creado y las tareas de subir la
// factura de lo que la lleva (con vencimiento hoy).
func (s *Servidor) crearPasos(ctx context.Context, tx *sql.Tx, pasos []api.ERPPasoPlan, hoy string) ([]api.ERPRegistro, []Actividad, error) {
	var creados []api.ERPRegistro
	var tareas []Actividad
	ids := map[string]string{} // ref → id, de lo que ya estaba y de lo creado
	for _, p := range pasos {
		m, _ := s.cat.Modelo(p.Modelo)
		if p.Existente != nil {
			reg, err := obtener(ctx, tx, m.Clave, p.Existente.ID)
			if err != nil {
				return creados, tareas, err
			}
			ids[p.Ref] = reg.ID
			continue
		}
		relleno := map[string]any{}
		for campo, ref := range p.Enlaces {
			if id, ok := ids[ref]; ok {
				relleno[campo] = id
			}
		}
		data, etiqueta, err := dominio.Validar(m, jsonDe(p.Data, relleno))
		if err != nil {
			return creados, tareas, err
		}
		reg, err := s.alm.insertar(ctx, tx, m, data, etiqueta)
		if err != nil {
			return creados, tareas, err
		}
		ids[p.Ref] = reg.ID
		creados = append(creados, reg)
		if llevaFactura(m) {
			tareas = append(tareas, Actividad{Modelo: m.Clave, Registro: reg.ID, Tipo: "subir_documento", Resumen: tareaFactura(m), Vence: hoy})
		}
	}
	return creados, tareas, nil
}

// crearAcciones — POST /ia/acciones/crear {"pasos": [...]}: crea lo que la persona ha
// visto, todo o nada. El plan vuelve de la pantalla, así que se valida otra vez.
func (s *Servidor) crearAcciones(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Pasos []api.ERPPasoPlan `json:"pasos"`
	}
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	if len(c.Pasos) == 0 || len(c.Pasos) > ia.MaxPasos {
		iaSinPlan(w)
		return
	}
	ctx := r.Context()
	for _, p := range c.Pasos {
		if _, ok := s.cat.Modelo(p.Modelo); !ok {
			fallo(w, http.StatusNotFound, "modelo_desconocido", "That model does not exist in the catalogue.", nil)
			return
		}
	}
	hoy := s.alm.ahora().Format(time.DateOnly)
	res := api.ERPCreados{Creados: []api.ERPRegistro{}, Actividades: []api.ERPActividad{}}
	var tareas []Actividad
	err := s.alm.tx(ctx, func(tx *sql.Tx) error {
		creados, t, err := s.crearPasos(ctx, tx, c.Pasos, hoy)
		res.Creados = append(res.Creados, creados...)
		tareas = t
		return err
	})
	if v, ok := validation.AsErrors(err); ok {
		fallo(w, http.StatusUnprocessableEntity, "unprocessable", err.Error(), v.Fields)
		return
	}
	if err != nil {
		falloDe(w, err)
		return
	}
	for _, t := range tareas {
		a, err := s.alm.CrearActividad(ctx, t)
		if err != nil {
			falloDe(w, err)
			return
		}
		var vista api.ERPActividad
		crudo, _ := json.Marshal(a)
		_ = json.Unmarshal(crudo, &vista)
		res.Actividades = append(res.Actividades, vista)
	}
	responder(w, http.StatusCreated, res)
}

// pistasBanco — POST /banco/pistas: la IA mira hasta quince cargos sin factura ni
// pista y dice dónde conseguirla. Lo que no contesta queda marcado como mirado.
func (s *Servidor) pistasBanco(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lineas, _, err := s.alm.LineasBanco(ctx, "pendiente", 1<<20)
	if err != nil {
		falloDe(w, err)
		return
	}
	var cargos []LineaBanco
	for _, l := range lineas {
		if reg, err := s.alm.Obtener(ctx, "linea_extracto", l.ID); err == nil {
			if _, mirado := reg.Data["empresa"]; mirado {
				continue
			}
		}
		if l.Centimos < 0 && len(cargos) < 15 {
			cargos = append(cargos, l)
		}
	}
	sort.SliceStable(cargos, func(i, j int) bool { return cargos[i].Fecha < cargos[j].Fecha })
	if len(cargos) == 0 {
		responder(w, http.StatusOK, map[string]any{"buscadas": 0})
		return
	}
	movs := make([]ia.MovimientoPista, len(cargos))
	porClave, pedidas := map[string]LineaBanco{}, map[string]bool{}
	for i, c := range cargos {
		clave := "m" + strconv.Itoa(i+1)
		movs[i] = ia.MovimientoPista{ID: clave, Fecha: c.Fecha, Descripcion: c.Descripcion, Centimos: c.Centimos}
		porClave[clave], pedidas[clave] = c, true
	}
	texto, ok := s.pedirIA(w, r, api.PeticionIA{
		Accion: "pistas_factura", Tipo: "erp", Modo: "calidad", Esfuerzo: "high",
		Instruccion: ia.InstruccionPistas(), Contexto: ia.ContextoPistas(movs),
	})
	if !ok {
		return
	}
	pistas := ia.PistasDe(texto, pedidas)
	respondidas := map[string]bool{}
	for _, p := range pistas {
		respondidas[p.ID] = true
		if err := s.alm.GuardarPista(ctx, porClave[p.ID].ID, p.Empresa, p.Web, p.URLFactura, p.Email); err != nil {
			falloDe(w, err)
			return
		}
	}
	for clave, c := range porClave {
		if !respondidas[clave] {
			if err := s.alm.GuardarPista(ctx, c.ID, "", "", "", ""); err != nil {
				falloDe(w, err)
				return
			}
		}
	}
	responder(w, http.StatusOK, map[string]any{"buscadas": len(cargos)})
}

// GuardarPista apunta en un movimiento dónde conseguir su factura (vacía: mirado,
// sin pista), sin tocar su estado.
func (a *Almacen) GuardarPista(ctx context.Context, lineaID, empresa, web, urlFactura, email string) error {
	return a.tx(ctx, func(tx *sql.Tx) error {
		r, err := obtener(ctx, tx, "linea_extracto", lineaID)
		if err != nil {
			return err
		}
		for k, v := range map[string]string{"empresa": empresa, "web": web, "url_factura": urlFactura, "email_facturas": email} {
			r.Data[k] = v
		}
		return a.guardarDatos(ctx, tx, r.Modelo, r.ID, r.Estado, r.Data)
	})
}
