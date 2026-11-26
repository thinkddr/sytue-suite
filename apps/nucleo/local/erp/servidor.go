// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/obligaciones"
	"git.sytue.eu/sytue/one/apps/nucleo/validation"
)

// Persona es el `sub` de quien usa el ERP local: no hay cuentas, hay una persona.
const Persona = "local"

// CodigoNoDisponible es el código de error de las rutas que esta entrega no sirve.
const CodigoNoDisponible = "no_disponible_local"

// Base es la raíz con la que se construye `api.ERP` sobre el transporte: no se
// resuelve nunca, porque la petición no sale del proceso.
const Base = "http://erp.sytue-office.invalid"

// maxCuerpo acota lo que se lee de una petición.
const maxCuerpo = 8 << 20

// Servidor atiende `/api/v1/…` del ERP sobre un Almacen.
type Servidor struct {
	cat *catalogo.Catalogo
	obl *obligaciones.Catalogo
	alm *Almacen
	mux *http.ServeMux
	// Nombre es el de la persona, para el historial.
	Nombre string

	mu sync.Mutex
	// ia es la IA de la persona (PonerIA); nil, sin IA.
	ia Asistente
	// envios son las cuentas de correo con que sale la factura (PonerCorreo); nil, ninguna.
	envios Correo
}

// Cargar abre los dos catálogos embebidos (módulos y obligaciones).
func Cargar() (*catalogo.Catalogo, *obligaciones.Catalogo, error) {
	cat, err := catalogo.Cargar()
	if err != nil {
		return nil, nil, err
	}
	obl, err := obligaciones.Cargar(func(clave string) bool { return cat.Rotulo(clave, "en") != "" })
	if err != nil {
		return nil, nil, err
	}
	return cat, obl, nil
}

// NuevoServidor monta las rutas de esta entrega.
func NuevoServidor(cat *catalogo.Catalogo, obl *obligaciones.Catalogo, alm *Almacen) *Servidor {
	s := &Servidor{cat: cat, obl: obl, alm: alm, mux: http.NewServeMux()}
	v := "/api/v1"
	h := func(patron string, f http.HandlerFunc) { s.mux.HandleFunc(patron, f) }
	h("GET "+v+"/catalogo", s.catalogo)
	h("GET "+v+"/mis-permisos", s.misPermisos)
	h("POST "+v+"/modulos/{modulo}", s.activarModulo)
	h("GET "+v+"/organizacion", s.organizacion)
	h("PUT "+v+"/organizacion", s.guardarOrganizacion)
	h("GET "+v+"/organizacion/paises", s.paises)
	h("GET "+v+"/organizacion/requisitos", s.requisitos)
	h("GET "+v+"/obligaciones", s.obligaciones)
	h("GET "+v+"/resumen", s.resumen)
	h("GET "+v+"/search", s.buscar)
	s.rutasLibro(h)
	s.rutasBanco(h)
	s.rutasSeguimiento(h)
	s.rutasInformes(h)
	s.rutasObligaciones(h)
	s.rutasTPV(h)
	s.rutasIA(h)
	h("GET "+v+"/importaciones", s.importaciones)
	h("POST "+v+"/importaciones/{id}/revertir", s.revertirImportacion)
	// El CRUD genérico va a mano: sus rutas comodín chocan con las literales en ServeMux.
	h(v+"/", s.generico)
	// Todo lo demás: no está en esta entrega.
	h("/", func(w http.ResponseWriter, r *http.Request) { noDisponible(w) })
	return s
}

func (s *Servidor) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// --- Respuestas ------------------------------------------------------------------

func responder(w http.ResponseWriter, estado int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(estado)
	_ = json.NewEncoder(w).Encode(v)
}

// errCampos son errores por campo de las rutas propias (el perfil, un cuerpo ilegible):
// salen en `details.fields` como los de `validation.Errors`.
type errCampos map[string]string

func (e errCampos) Error() string {
	partes := make([]string, 0, len(e))
	for k, v := range e {
		partes = append(partes, k+": "+v)
	}
	slices.Sort(partes)
	return strings.Join(partes, "; ")
}

func (e errCampos) campos() []validation.FieldError {
	fs := make([]validation.FieldError, 0, len(e))
	for k, v := range e {
		fs = append(fs, validation.FieldError{Field: k, Message: v})
	}
	slices.SortFunc(fs, func(a, b validation.FieldError) int { return strings.Compare(a.Field, b.Field) })
	return fs
}

// fallo escribe el sobre de error de platform/pkg/httpx: `{"error": {code, message, details}}`.
func fallo(w http.ResponseWriter, estado int, codigo, mensaje string, campos []validation.FieldError) {
	cuerpo := map[string]any{"code": codigo, "message": mensaje}
	if len(campos) > 0 {
		cuerpo["details"] = map[string]any{"fields": campos}
	}
	responder(w, estado, map[string]any{"error": cuerpo})
}

func noDisponible(w http.ResponseWriter) {
	fallo(w, http.StatusNotImplemented, CodigoNoDisponible,
		"This is not available yet in the local edition of Sytue ERP.", nil)
}

// falloDe traduce los errores del almacén y de la validación.
func falloDe(w http.ResponseWriter, err error) {
	var campos errCampos
	verrs, esValidacion := validation.AsErrors(err)
	switch {
	case errors.As(err, &campos):
		fallo(w, http.StatusUnprocessableEntity, "unprocessable", err.Error(), campos.campos())
	case esValidacion:
		fallo(w, http.StatusUnprocessableEntity, "unprocessable", err.Error(), verrs.Fields)
	case errors.Is(err, ErrNoEncontrado):
		fallo(w, http.StatusNotFound, "not_found", "Resource not found.", nil)
	case errors.Is(err, ErrYaAsentado):
		fallo(w, http.StatusConflict, "ya_asentado", "This document has already been posted to the ledger.", nil)
	case errors.Is(err, ErrPeriodoCerrado):
		fallo(w, http.StatusConflict, "periodo_cerrado", "That accounting period is closed. Reopen it first, or post with a later date.", nil)
	case errors.Is(err, ErrCierreHaciaAtras):
		fallo(w, http.StatusConflict, "cierre_hacia_atras",
			"The book is already closed up to a later date. Use the reopening route, which asks for a reason.", nil)
	case errors.Is(err, ErrYaJustificado):
		fallo(w, http.StatusConflict, "ya_justificado", "This bank transaction already has its invoice or explanation.", nil)
	case errors.Is(err, ErrNoJustificado):
		fallo(w, http.StatusConflict, "no_justificado", "This bank transaction is still pending.", nil)
	case errors.Is(err, ErrFacturaNoAbierta):
		fallo(w, http.StatusConflict, "factura_no_abierta", "The invoice is already paid or cancelled.", nil)
	case errors.Is(err, errSentidoFactura):
		fallo(w, http.StatusUnprocessableEntity, "sentido_factura",
			"Money coming in is matched to an issued invoice, and money going out to a received one.", nil)
	case errors.Is(err, errAdjuntoTipo):
		fallo(w, http.StatusUnsupportedMediaType, "adjunto_tipo", "Upload a PDF, an image (JPEG, PNG or WebP) or an e-invoice XML.", nil)
	case errors.Is(err, errAdjuntoGrande):
		fallo(w, http.StatusRequestEntityTooLarge, "adjunto_grande", "The file is larger than 10 MB.", nil)
	case errors.Is(err, errMotivo):
		fallo(w, http.StatusUnprocessableEntity, "motivo_desconocido", "Unknown reason for a transaction without an invoice.", nil)
	case errors.Is(err, ErrDatos), errors.Is(err, dominio.ErrDatos):
		fallo(w, http.StatusUnprocessableEntity, "unprocessable", err.Error(), nil)
	default:
		fallo(w, http.StatusInternalServerError, "internal", err.Error(), nil)
	}
}

// falloValidacion es el 422 de `errorDeValidacion` del servidor: con los campos, si los hay.
func falloValidacion(w http.ResponseWriter, err error) {
	var fs []validation.FieldError
	if v, ok := validation.AsErrors(err); ok {
		fs = v.Fields
	}
	fallo(w, http.StatusUnprocessableEntity, "unprocessable", err.Error(), fs)
}

func leer(r *http.Request, v any) error {
	cuerpo, err := io.ReadAll(io.LimitReader(r.Body, maxCuerpo))
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(cuerpo))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return errCampos{"body": "JSON inválido"}
	}
	return nil
}

// idioma es el de Accept-Language si el catálogo lo tiene; si no, inglés.
func idioma(r *http.Request) string {
	for _, parte := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(parte), ";")
		base, _, _ := strings.Cut(strings.ToLower(tag), "-")
		if slices.Contains(catalogo.Idiomas, base) {
			return base
		}
	}
	return "en"
}

// --- Configuración -----------------------------------------------------------------

func (s *Servidor) catalogo(w http.ResponseWriter, r *http.Request) {
	activos, err := s.alm.ModulosActivos(r.Context())
	if err != nil {
		falloDe(w, err)
		return
	}
	l := idioma(r)
	responder(w, http.StatusOK, map[string]any{"modulos": s.cat.VistaJSON(l), "activos": activos, "idioma": l})
}

// verbosLocales son los gestos que se conceden. Aprobar es de varias personas y su
// interfaz no se ofrece (E-09); conciliar, desde la entrega del banco.
var verbosLocales = []string{"crear", "asentar", "pagar", "conciliar", "administrar"}

func (s *Servidor) misPermisos(w http.ResponseWriter, _ *http.Request) {
	p := api.ERPPermisos{Administrador: true, Modulos: map[string]string{}, Verbos: map[string][]string{}}
	for _, m := range s.cat.Modulos() {
		p.Modulos[m.Clave] = api.ERPAdministrador
		p.Verbos[m.Clave] = verbosLocales
	}
	// El calendario fiscal es un área aparte en los permisos del servidor.
	p.Modulos["obligaciones"] = api.ERPAdministrador
	p.Verbos["obligaciones"] = verbosLocales
	responder(w, http.StatusOK, p)
}

func (s *Servidor) activarModulo(w http.ResponseWriter, r *http.Request) {
	modulo := r.PathValue("modulo")
	if !slices.ContainsFunc(s.cat.Modulos(), func(m catalogo.Modulo) bool { return m.Clave == modulo }) {
		fallo(w, http.StatusNotFound, "modelo_desconocido", "That model does not exist in the catalogue.", nil)
		return
	}
	var cuerpo struct {
		Activo bool `json:"activo"`
	}
	if err := leer(r, &cuerpo); err != nil {
		falloDe(w, err)
		return
	}
	ctx := r.Context()
	previos, err := s.alm.ModulosActivos(ctx)
	if err != nil {
		falloDe(w, err)
		return
	}
	res := api.ERPModulosActivos{}
	cambiar := func(m string, activo bool) bool {
		if err := s.alm.ActivarModulo(ctx, m, activo); err != nil {
			falloDe(w, err)
			return false
		}
		return true
	}
	if cuerpo.Activo {
		for _, d := range s.cat.DependenciasTransitivas(modulo) {
			if !cambiar(d, true) {
				return
			}
			if !slices.Contains(previos, d) {
				res.Activados = append(res.Activados, d)
			}
		}
	} else {
		for _, d := range s.cat.DependientesTransitivas(modulo) {
			if slices.Contains(previos, d) {
				if !cambiar(d, false) {
					return
				}
				res.Desactivados = append(res.Desactivados, d)
			}
		}
	}
	if !cambiar(modulo, cuerpo.Activo) {
		return
	}
	if res.Activos, err = s.alm.ModulosActivos(ctx); err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, res)
}

func (s *Servidor) organizacion(w http.ResponseWriter, r *http.Request) {
	o, err := s.alm.Organizacion(r.Context())
	if errors.Is(err, ErrNoEncontrado) {
		fallo(w, http.StatusNotFound, "organizacion_no_configurada", "The organisation profile has not been set up yet.", nil)
		return
	}
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, o)
}

func recortar(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	return string(r[:min(len(r), n)])
}

func (s *Servidor) guardarOrganizacion(w http.ResponseWriter, r *http.Request) {
	var o api.ERPOrganizacion
	if err := leer(r, &o); err != nil {
		falloDe(w, err)
		return
	}
	o.Nombre, o.NIF, o.Direccion = recortar(o.Nombre, 200), recortar(o.NIF, 40), recortar(o.Direccion, 200)
	o.CP, o.Ciudad, o.Pais = recortar(o.CP, 20), recortar(o.Ciudad, 100), strings.ToUpper(strings.TrimSpace(o.Pais))
	errs := errCampos{}
	if o.Nombre == "" {
		errs["nombre"] = "obligatorio"
	}
	if pais, ok := s.obl.Pais(o.Pais); !ok {
		errs["pais"] = "país no admitido"
	} else if _, ok := pais.Forma(o.Forma); !ok {
		errs["forma"] = "forma jurídica desconocida para ese país"
	}
	for k := range o.Condiciones {
		if !slices.Contains(obligaciones.Condiciones, k) {
			errs["condiciones"] = "condición desconocida " + k
		}
	}
	if len(errs) > 0 {
		falloDe(w, errs)
		return
	}
	if err := s.alm.GuardarOrganizacion(r.Context(), o); err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, o)
}

func (s *Servidor) paises(w http.ResponseWriter, _ *http.Request) {
	type vista struct {
		Pais       string                         `json:"pais"`
		Verificado bool                           `json:"verificado"`
		Moneda     string                         `json:"moneda"`
		Formas     []obligaciones.Forma           `json:"formas"`
		Portales   map[string]obligaciones.Portal `json:"portales"`
		Total      int                            `json:"obligaciones"`
	}
	paises := s.obl.Paises()
	out := make([]vista, 0, len(paises))
	for _, p := range paises {
		out = append(out, vista{p.Pais, p.Verificado, p.Moneda, p.Formas, p.Portales, len(p.Obligaciones)})
	}
	responder(w, http.StatusOK, map[string]any{"paises": out, "condiciones": obligaciones.Condiciones})
}

func (s *Servidor) requisitos(w http.ResponseWriter, r *http.Request) {
	o, err := s.alm.Organizacion(r.Context())
	if err != nil {
		responder(w, http.StatusOK, map[string]any{})
		return
	}
	pais, ok := s.obl.Pais(o.Pais)
	if !ok || pais.RequisitosFactura == nil {
		responder(w, http.StatusOK, map[string]any{})
		return
	}
	responder(w, http.StatusOK, pais.RequisitosFactura)
}

// obligaciones es el calendario del año para el perfil, con lo que falta en el ERP
// para presentar cada una y la fase de su proceso (obligaciones.go).
func (s *Servidor) obligaciones(w http.ResponseWriter, r *http.Request) {
	o, perfil, ok := s.perfil(w, r)
	if !ok {
		return
	}
	anio := time.Now().Year()
	if a, err := strconv.Atoi(r.URL.Query().Get("anio")); err == nil && a >= 2000 && a <= 2100 {
		anio = a
	}
	inst, err := s.obl.Calendario(perfil, anio)
	if err != nil {
		fallo(w, http.StatusUnprocessableEntity, "unprocessable", err.Error(), nil)
		return
	}
	ctx := r.Context()
	pendientes, err := s.alm.Pendientes(ctx, time.Date(anio, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(anio, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		falloDe(w, err)
		return
	}
	procesos, err := s.alm.Procesos(ctx)
	if err != nil {
		falloDe(w, err)
		return
	}
	type vista struct {
		obligaciones.Instancia
		ConceptoRotulo string                `json:"concepto_rotulo"`
		AmbitoRotulo   string                `json:"ambito_rotulo"`
		Pendientes     map[string]int        `json:"pendientes,omitempty"`
		Proceso        *obligaciones.Proceso `json:"proceso,omitempty"`
	}
	l := idioma(r)
	vistas := make([]vista, 0, len(inst))
	for _, i := range inst {
		v := vista{
			Instancia: i, ConceptoRotulo: s.cat.Rotulo("obligacion."+i.Concepto, l), AmbitoRotulo: s.cat.Rotulo("ambito."+i.Ambito, l),
			Pendientes: obligaciones.PendientesDe(i, pendientes),
		}
		if p, ok := procesos[i.Clave+":"+i.Periodo]; ok {
			v.Proceso = &p
		}
		vistas = append(vistas, v)
	}
	pais, _ := s.obl.Pais(o.Pais)
	res := map[string]any{
		"anio": anio, "pais": o.Pais, "verificado": pais.Verificado, "moneda": pais.Moneda,
		"region": perfil.Region, "obligaciones": vistas,
	}
	if t, ok := pais.Territorios[perfil.Region]; ok {
		res["territorio"] = t
	}
	responder(w, http.StatusOK, res)
}

func (s *Servidor) resumen(w http.ResponseWriter, r *http.Request) {
	res, err := s.alm.Resumen(r.Context())
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, res)
}

func (s *Servidor) buscar(w http.ResponseWriter, r *http.Request) {
	limite := 20
	for _, k := range []string{"limit", "limite"} {
		if n, err := strconv.Atoi(r.URL.Query().Get(k)); err == nil && n > 0 {
			limite = min(n, 100)
			break
		}
	}
	regs, err := s.alm.Buscar(r.Context(), r.URL.Query().Get("q"), limite)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"items": regs})
}

// --- El CRUD genérico ----------------------------------------------------------------

type manejadorModelo func(http.ResponseWriter, *http.Request, *catalogo.Modelo)

// generico reparte `/{modelo}/…` entre los manejadores del CRUD. Lo que no es un
// modelo del catálogo es una ruta del servidor que no se sirve aquí (`/adjuntos/{id}`,
// `/libro/{id}/anular`, `/banco`…): 501.
//
//nolint:gocyclo // un enrutador: un caso por ruta, en el orden en que se prueban
func (s *Servidor) generico(w http.ResponseWriter, r *http.Request) {
	partes := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	m, ok := s.cat.Modelo(partes[0])
	if !ok {
		noDisponible(w)
		return
	}
	resto := partes[1:]
	if len(resto) >= 1 && resto[0] != "" {
		r.SetPathValue("id", resto[0])
	}
	var f manejadorModelo
	switch clave := r.Method + " " + strings.Join(resto, "/"); {
	case len(resto) == 0:
		// `/factura_venta` sin barra: el servidor tampoco lo sirve.
	case clave == "GET ":
		f = s.listar
	case clave == "POST ":
		f = s.crear
	case clave == "GET estadisticas":
		f = s.estadisticas
	case clave == "POST ia/rellenar":
		f = s.rellenarFicha
	case clave == "GET export":
		f = s.exportarCSV
	case clave == "POST lote":
		f = s.lote
	case clave == "POST importar":
		f = s.importar
	case clave == "POST asistencia/leer":
		f = s.leerFactura
	case len(resto) == 1 && r.Method == http.MethodGet:
		f = s.obtener
	case len(resto) == 1 && r.Method == http.MethodPut:
		f = s.actualizar
	case len(resto) == 1 && r.Method == http.MethodDelete:
		f = s.borrar
	case len(resto) == 2:
		f = map[string]manejadorModelo{
			"POST estado": s.cambiarEstado, "POST restaurar": s.restaurar, "POST duplicar": s.duplicar,
			"POST convertir": s.convertir, "GET historial": s.historial, "POST historial": s.anotar,
			"POST asentar": s.asentar, "GET asientos": s.asientosDe,
			"GET facturae": s.facturae, "GET ubl": s.ubl, "GET imprimir": s.imprimir, "GET pdf": s.pdfFactura,
			"GET enviar": s.envioFactura, "POST enviar": s.enviarFactura,
		}[r.Method+" "+resto[1]]
	}
	if f == nil {
		noDisponible(w)
		return
	}
	f(w, r, m)
}

func (s *Servidor) listar(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	q := r.URL.Query()
	f := api.ERPFiltro{
		Q: q.Get("q"), Estado: q.Get("estado"), Orden: q.Get("orden"), Asc: q.Get("dir") == "asc",
		Papelera: q.Get("papelera") == "1", Cursor: q.Get("cursor"),
	}
	f.Limite, _ = strconv.Atoi(q.Get("limite"))
	if f.Estado != "" && !slices.Contains(m.Estados, f.Estado) {
		estadoInvalido(w)
		return
	}
	switch f.Orden {
	case "", "creado", "etiqueta", "ref", "estado":
	default:
		if !slices.ContainsFunc(m.Campos, func(c catalogo.Campo) bool { return c.Clave == f.Orden }) {
			fallo(w, http.StatusBadRequest, "bad_request", "unknown sort field: "+f.Orden, nil)
			return
		}
	}
	pag, err := s.alm.Listar(r.Context(), m, f)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, pag)
}

func estadoInvalido(w http.ResponseWriter) {
	fallo(w, http.StatusUnprocessableEntity, "estado_invalido", "That status is not part of the document's workflow.", nil)
}

type cuerpoRegistro struct {
	Data json.RawMessage `json:"data"`
}

func (s *Servidor) validarCuerpo(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) (map[string]any, string, bool) {
	var c cuerpoRegistro
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return nil, "", false
	}
	if len(c.Data) == 0 {
		c.Data = json.RawMessage("{}")
	}
	data, etiqueta, err := dominio.Validar(m, c.Data)
	if err != nil {
		falloValidacion(w, err)
		return nil, "", false
	}
	return data, etiqueta, true
}

func (s *Servidor) crear(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	data, etiqueta, ok := s.validarCuerpo(w, r, m)
	if !ok {
		return
	}
	reg, err := s.alm.Crear(r.Context(), m, data, etiqueta)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusCreated, reg)
}

func (s *Servidor) actualizar(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	data, etiqueta, ok := s.validarCuerpo(w, r, m)
	if !ok {
		return
	}
	reg, err := s.alm.Actualizar(r.Context(), m, r.PathValue("id"), data, etiqueta)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, reg)
}

func (s *Servidor) obtener(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	reg, err := s.alm.Obtener(r.Context(), m.Clave, r.PathValue("id"))
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, reg)
}

func (s *Servidor) estadisticas(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	st, err := s.alm.Estadisticas(r.Context(), m)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, st)
}

func (s *Servidor) borrar(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	if err := s.alm.Borrar(r.Context(), m.Clave, r.PathValue("id")); err != nil {
		falloDe(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) restaurar(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	reg, err := s.alm.Restaurar(r.Context(), m.Clave, r.PathValue("id"))
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, reg)
}

func (s *Servidor) cambiarEstado(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	var c struct {
		Estado string `json:"estado"`
	}
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	if !slices.Contains(m.Estados, c.Estado) {
		estadoInvalido(w)
		return
	}
	reg, err := s.alm.CambiarEstado(r.Context(), m.Clave, r.PathValue("id"), c.Estado)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, reg)
}

func (s *Servidor) duplicar(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	reg, err := s.alm.Duplicar(r.Context(), m, r.PathValue("id"))
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusCreated, reg)
}

func (s *Servidor) convertir(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	if m.Clave != "presupuesto" && m.Clave != "pedido_venta" {
		fallo(w, http.StatusBadRequest, "accion_invalida", "Only quotes and sales orders can be converted.", nil)
		return
	}
	reg, err := s.alm.Convertir(r.Context(), m.Clave, r.PathValue("id"))
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, reg)
}

func (s *Servidor) lote(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	var c struct {
		Accion string   `json:"accion"`
		IDs    []string `json:"ids"`
		Estado string   `json:"estado"`
	}
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	if len(c.IDs) == 0 {
		fallo(w, http.StatusBadRequest, "sin_ids", "At least one record id is required.", nil)
		return
	}
	switch c.Accion {
	case "estado":
		if !slices.Contains(m.Estados, c.Estado) {
			estadoInvalido(w)
			return
		}
	case "borrar", "restaurar":
	default:
		fallo(w, http.StatusBadRequest, "accion_invalida", "Unsupported batch action (supported: estado, borrar, restaurar).", nil)
		return
	}
	n, err := s.alm.Lote(r.Context(), m.Clave, c.Accion, c.IDs, c.Estado)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"afectados": n})
}

func (s *Servidor) historial(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	if _, err := s.alm.Obtener(r.Context(), m.Clave, r.PathValue("id")); err != nil && !errors.Is(err, ErrNoEncontrado) {
		falloDe(w, err)
		return
	}
	hs, err := s.alm.Historial(r.Context(), r.PathValue("id"), s.Nombre)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"historial": hs})
}

func (s *Servidor) anotar(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	var c struct {
		Texto string `json:"texto"`
	}
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	if err := s.alm.Anotar(r.Context(), m.Clave, r.PathValue("id"), c.Texto); err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusCreated, map[string]any{"tipo": "nota", "texto": strings.TrimSpace(c.Texto)})
}

// --- El transporte ---------------------------------------------------------------------

// Transporte es un http.RoundTripper que entrega cada petición al Servidor en el mismo
// proceso. No abre sockets ni resuelve nombres.
type Transporte struct{ S *Servidor }

// RoundTrip atiende la petición y devuelve la respuesta ya completa.
func (t Transporte) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body != nil {
		defer func() { _ = r.Body.Close() }()
	}
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	// El cliente pone la ruta con su base delante; el servidor solo conoce `/api/v1/…`.
	sr := r.Clone(r.Context())
	sr.RequestURI = sr.URL.RequestURI()
	if sr.Body == nil {
		sr.Body = http.NoBody
	}
	rec := &grabadora{cabecera: http.Header{}, estado: http.StatusOK}
	func() {
		defer func() {
			if p := recover(); p != nil {
				rec = &grabadora{cabecera: http.Header{}, estado: http.StatusOK}
				fallo(rec, http.StatusInternalServerError, "internal", fmt.Sprint(p), nil)
			}
		}()
		t.S.ServeHTTP(rec, sr)
	}()
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", rec.estado, http.StatusText(rec.estado)),
		StatusCode:    rec.estado,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rec.cabecera,
		Body:          io.NopCloser(bytes.NewReader(rec.cuerpo.Bytes())),
		ContentLength: int64(rec.cuerpo.Len()),
		Request:       r,
	}, nil
}

// grabadora es el http.ResponseWriter en memoria del transporte. No es `httptest`, que
// registra banderas de línea de órdenes al importarse.
type grabadora struct {
	cabecera http.Header
	cuerpo   bytes.Buffer
	estado   int
	escrito  bool
}

func (g *grabadora) Header() http.Header { return g.cabecera }

func (g *grabadora) WriteHeader(estado int) {
	if !g.escrito {
		g.estado, g.escrito = estado, true
	}
}

func (g *grabadora) Write(b []byte) (int, error) {
	g.WriteHeader(http.StatusOK)
	return g.cuerpo.Write(b)
}

// Cliente es el *http.Client para `api.NewERP(Base, nil, cliente, idioma)`.
func (s *Servidor) Cliente() *http.Client { return &http.Client{Transport: Transporte{S: s}} }
