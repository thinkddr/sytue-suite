// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/ia"
)

// INFORMES, TABLEROS E IA DEL ERP (entrega 3 y E-08). Las fuentes, las consultas, el
// Excel y los prompts son los del servidor (apps/nucleo/erp/ia); aquí se leen las filas
// del SQLite y la IA es la que la persona haya configurado en Ajustes (`l.Asistir()`),
// que llega por PonerIA. Sin IA, las rutas que la piden contestan 503 `ia_no_disponible`
// como un servidor sin Sytue AI. Las fuentes de otras apps de la suite no existen en local.

const esquemaInformes = `
CREATE TABLE IF NOT EXISTS tablero (
  id      TEXT PRIMARY KEY,
  titulo  TEXT NOT NULL,
  informe TEXT NOT NULL,              -- el informe, JSON legible
  creado  TEXT NOT NULL
);
`

// Asistente es la IA de la persona: la de `local.App.Asistir()`.
type Asistente func(context.Context, api.PeticionIA) (string, error)

// Topes de lo que se lee para un informe, los del servidor.
const (
	filasPorParte   = 1000
	filasPorInforme = 60_000
	maxPeticionIA   = 2000
	maxTextoFicha   = 20_000
)

// PonerIA enchufa (o, con nil, desenchufa) la IA: se llama al arrancar y cada vez que la
// persona la cambia en Ajustes.
func (s *Servidor) PonerIA(f Asistente) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ia = f
}

func (s *Servidor) asistente() Asistente {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ia
}

// pedirIA llama a la IA; ya ha respondido si no hay o falla.
func (s *Servidor) pedirIA(w http.ResponseWriter, r *http.Request, p api.PeticionIA) (string, bool) {
	f := s.asistente()
	if f == nil {
		fallo(w, http.StatusServiceUnavailable, "ia_no_disponible", "Sytue AI is not available right now. Try again in a moment.", nil)
		return "", false
	}
	p.Idioma = idioma(r)
	texto, err := f(r.Context(), p)
	if err != nil {
		fallo(w, http.StatusServiceUnavailable, "ia_no_disponible", "Sytue AI is not available right now. Try again in a moment.", nil)
		return "", false
	}
	return texto, true
}

func (s *Servidor) rutasInformes(h func(string, http.HandlerFunc)) {
	v := "/api/v1"
	h("POST "+v+"/informes/datos", s.datosInforme)
	h("POST "+v+"/informes/excel", s.excelInforme)
	h("GET "+v+"/tableros", s.tableros)
	h("POST "+v+"/tableros", s.guardarTablero)
	h("DELETE "+v+"/tableros/{id}", s.borrarTablero)
	h("POST "+v+"/ia/informe", s.proponerInforme)
}

// fuentes son los modelos de los módulos encendidos y los apuntes del libro.
func (s *Servidor) fuentes(ctx context.Context, idioma string) ([]ia.Fuente, map[string]ia.Fuente, error) {
	activos, err := s.alm.ModulosActivos(ctx)
	if err != nil {
		return nil, nil, err
	}
	var out []ia.Fuente
	indice := map[string]ia.Fuente{}
	anadir := func(f ia.Fuente) {
		if _, repetida := indice[f.Clave]; !repetida {
			indice[f.Clave] = f
			out = append(out, f)
		}
	}
	for _, mod := range s.cat.Modulos() {
		if !slices.Contains(activos, mod.Clave) {
			continue
		}
		for _, m := range mod.Modelos {
			anadir(ia.FuenteDeModelo(s.cat, m, idioma))
		}
	}
	anadir(ia.FuenteDelLibro(s.cat, idioma))
	return out, indice, nil
}

// leerFuente trae hasta `tope` filas de una fuente.
func (s *Servidor) leerFuente(ctx context.Context, f ia.Fuente, tope int) ([]ia.Fila, bool, error) {
	if tope <= 0 {
		return nil, true, nil
	}
	if f.Clave == ia.FuenteLibro {
		return s.alm.filasDelLibro(ctx, tope)
	}
	m, ok := s.cat.Modelo(f.Clave)
	if !ok {
		return nil, false, ErrNoEncontrado
	}
	var out []ia.Fila
	filtro := api.ERPFiltro{Limite: 200}
	for {
		pag, err := s.alm.Listar(ctx, m, filtro)
		if err != nil {
			return nil, false, err
		}
		for _, reg := range pag.Registros {
			if len(out) == tope {
				return out, true, nil
			}
			out = append(out, ia.FilaDe(m, reg.Ref, reg.Estado, reg.Data, pag.Etiquetas))
		}
		if pag.Siguiente == "" {
			return out, false, nil
		}
		filtro.Cursor = pag.Siguiente
	}
}

// filasDelLibro son los apuntes de lo asentado, lo último primero.
func (a *Almacen) filasDelLibro(ctx context.Context, tope int) ([]ia.Fila, bool, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT m.fecha, m.diario, m.ref, m.concepto, l.cuenta, l.debe, l.haber
		FROM apunte l JOIN asiento m ON m.id = l.asiento WHERE m.estado = 'asentado'
		ORDER BY m.fecha DESC, l.id DESC LIMIT ?`, tope+1)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = filas.Close() }()
	var out []ia.Fila
	for filas.Next() {
		var fecha, diario, ref, concepto, cuenta string
		var debe, haber int64
		if err := filas.Scan(&fecha, &diario, &ref, &concepto, &cuenta, &debe, &haber); err != nil {
			return nil, false, err
		}
		out = append(out, ia.Fila{"fecha": fecha, "diario": diario, "ref": ref, "concepto": concepto, "cuenta": cuenta, "debe": debe, "haber": haber})
	}
	recortada := len(out) > tope
	return out[:min(len(out), tope)], recortada, filas.Err()
}

// informeDelCuerpo lee {informe} y lo comprueba contra las fuentes de hoy.
func (s *Servidor) informeDelCuerpo(w http.ResponseWriter, r *http.Request) (ia.Informe, []error, map[string]ia.Fuente, bool) {
	var c struct {
		Informe ia.Informe `json:"informe"`
	}
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return ia.Informe{}, nil, nil, false
	}
	if n := len(c.Informe.Partes); n == 0 || n > ia.MaxPartes {
		fallo(w, http.StatusUnprocessableEntity, "informe_invalido", "That report is not valid.", nil)
		return ia.Informe{}, nil, nil, false
	}
	_, indice, err := s.fuentes(r.Context(), idioma(r))
	if err != nil {
		falloDe(w, err)
		return ia.Informe{}, nil, nil, false
	}
	return c.Informe, c.Informe.Comprobar(indice), indice, true
}

// tablas ejecuta cada parte; cada fuente se lee una vez aunque la usen varias.
func (s *Servidor) tablas(r *http.Request, inf ia.Informe, errs []error, indice map[string]ia.Fuente, tope int) []ia.Tabla {
	type carga struct {
		filas     []ia.Fila
		recortada bool
		error     string
	}
	leidas := map[string]carga{}
	restantes := filasPorInforme
	out := make([]ia.Tabla, len(inf.Partes))
	op := ia.Opciones{Ahora: s.alm.ahora().UTC(), Tope: tope, RotuloCuenta: s.cat.Rotulo("medida.cuenta", idioma(r))}
	for i, p := range inf.Partes {
		fuente, ok := indice[p.Consulta.Fuente]
		if errs[i] != nil || !ok {
			out[i] = ia.Tabla{Error: ia.ErrorInvalida}
			continue
		}
		c, hay := leidas[fuente.Clave]
		if !hay {
			var err error
			c.filas, c.recortada, err = s.leerFuente(r.Context(), fuente, min(restantes, maxExportar))
			if err != nil {
				c.error = ia.ErrorNoDisponible
			}
			restantes = max(restantes-len(c.filas), 0)
			leidas[fuente.Clave] = c
		}
		if c.error != "" {
			out[i] = ia.Tabla{Error: c.error}
			continue
		}
		t := p.Consulta.Ejecutar(fuente, c.filas, op)
		t.Recortada = t.Recortada || c.recortada
		out[i] = t
	}
	return out
}

func (s *Servidor) datosInforme(w http.ResponseWriter, r *http.Request) {
	inf, errs, indice, ok := s.informeDelCuerpo(w, r)
	if !ok {
		return
	}
	responder(w, http.StatusOK, map[string]any{"datos": s.tablas(r, inf, errs, indice, filasPorParte)})
}

func (s *Servidor) excelInforme(w http.ResponseWriter, r *http.Request) {
	inf, errs, indice, ok := s.informeDelCuerpo(w, r)
	if !ok {
		return
	}
	l := idioma(r)
	tablas := s.tablas(r, inf, errs, indice, maxExportar)
	hojas := make([]ia.Hoja, len(inf.Partes))
	for i, p := range inf.Partes {
		t := tablas[i]
		switch {
		case t.Error != "":
			t = ia.Tabla{Columnas: []ia.Campo{{Clave: "aviso", Tipo: ia.TipoTexto, Etiqueta: s.cat.Rotulo("aviso.no_disponible", l)}}}
		case t.Recortada:
			t.Filas = append(t.Filas, []any{s.cat.Rotulo("aviso.truncado", l) + " " + strconv.Itoa(maxExportar)})
		}
		hojas[i] = ia.Hoja{Nombre: cmp.Or(p.Titulo, inf.Titulo), Tabla: t}
	}
	var buf bytes.Buffer
	if err := ia.EscribirXLSX(&buf, hojas); err != nil {
		falloDe(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="informe-`+s.alm.ahora().UTC().Format(time.DateOnly)+`.xlsx"`)
	_, _ = w.Write(buf.Bytes())
}

// Tablero es un tablero guardado (la forma de `store.Tablero`).
type Tablero struct {
	ID        string          `json:"id"`
	Titulo    string          `json:"titulo"`
	Informe   json.RawMessage `json:"informe"`
	CreadoPor string          `json:"creado_por"`
	CreadoEn  time.Time       `json:"creado_en"`
}

// Tableros son los cien más recientes.
func (a *Almacen) Tableros(ctx context.Context) ([]Tablero, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT id, titulo, informe, creado FROM tablero ORDER BY creado DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := []Tablero{}
	for filas.Next() {
		var t Tablero
		var inf, creado string
		if err := filas.Scan(&t.ID, &t.Titulo, &inf, &creado); err != nil {
			return nil, err
		}
		t.Informe, t.CreadoPor = json.RawMessage(inf), Persona
		t.CreadoEn, _ = time.Parse(marca, creado)
		out = append(out, t)
	}
	return out, filas.Err()
}

// GuardarTablero guarda un informe como tablero.
func (a *Almacen) GuardarTablero(ctx context.Context, titulo string, informe []byte) (Tablero, error) {
	t := Tablero{ID: uuid.NewString(), Titulo: titulo, Informe: informe, CreadoPor: Persona}
	ahora := a.marcaAhora()
	t.CreadoEn, _ = time.Parse(marca, ahora)
	_, err := a.db.ExecContext(ctx, `INSERT INTO tablero (id, titulo, informe, creado) VALUES (?, ?, ?, ?)`, t.ID, titulo, string(informe), ahora)
	return t, err
}

// BorrarTablero lo borra.
func (a *Almacen) BorrarTablero(ctx context.Context, id string) error {
	res, err := a.db.ExecContext(ctx, `DELETE FROM tablero WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoEncontrado
	}
	return nil
}

func (s *Servidor) tableros(w http.ResponseWriter, r *http.Request) {
	ts, err := s.alm.Tableros(r.Context())
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"tableros": ts})
}

func (s *Servidor) guardarTablero(w http.ResponseWriter, r *http.Request) {
	inf, errs, _, ok := s.informeDelCuerpo(w, r)
	if !ok {
		return
	}
	if slices.ContainsFunc(errs, func(e error) bool { return e != nil }) {
		fallo(w, http.StatusUnprocessableEntity, "informe_invalido", "That report is not valid.", nil)
		return
	}
	crudo, err := json.Marshal(inf)
	if err != nil {
		falloDe(w, err)
		return
	}
	t, err := s.alm.GuardarTablero(r.Context(), cmp.Or(inf.Titulo, "Sytue AI"), crudo)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusCreated, map[string]any{"tablero": t})
}

func (s *Servidor) borrarTablero(w http.ResponseWriter, r *http.Request) {
	if err := s.alm.BorrarTablero(r.Context(), r.PathValue("id")); err != nil {
		falloDe(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// proponerInforme — POST /ia/informe — una frase → un informe, con la IA de la persona.
func (s *Servidor) proponerInforme(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Peticion string `json:"peticion"`
		Para     string `json:"para"`
		Modelo   string `json:"modelo"`
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
	fuentes, indice, err := s.fuentes(r.Context(), idioma(r))
	if err != nil {
		falloDe(w, err)
		return
	}
	sobre := ""
	if f, ok := indice[c.Modelo]; ok {
		sobre = f.Clave + " «" + f.Etiqueta + "»"
	}
	para := ia.ParaTablero
	if c.Para == ia.ParaExcel {
		para = ia.ParaExcel
	}
	texto, ok := s.pedirIA(w, r, api.PeticionIA{
		Accion: "informe", Tipo: "erp", Modo: "calidad", Esfuerzo: "high",
		Instruccion: ia.InstruccionInforme(para, c.Peticion, sobre, s.alm.ahora().UTC()), Contexto: ia.ContextoFuentes(fuentes),
	})
	if !ok {
		return
	}
	inf, err := ia.InformeDe(texto)
	if err != nil {
		fallo(w, http.StatusUnprocessableEntity, "ia_sin_informe", "Sytue AI could not turn that into a report. Try saying it another way.", nil)
		return
	}
	errs := inf.Comprobar(indice)
	validas := make([]ia.Parte, 0, len(inf.Partes))
	for i, p := range inf.Partes {
		if errs[i] == nil && len(validas) < ia.MaxPartes {
			validas = append(validas, p)
		}
	}
	if len(validas) == 0 {
		fallo(w, http.StatusUnprocessableEntity, "ia_sin_informe", "Sytue AI could not turn that into a report. Try saying it another way.", nil)
		return
	}
	inf.Partes = validas
	responder(w, http.StatusOK, map[string]any{"informe": inf})
}

// rellenarFicha — POST /{modelo}/ia/rellenar — un texto pegado, o el PDF de una
// factura (leído con el motor PDF del núcleo, ia_acciones.go), → los campos de una ficha.
func (s *Servidor) rellenarFicha(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	var c struct {
		Texto string `json:"texto"`
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/pdf") {
		texto, ok := leerPDFFicha(w, r)
		if !ok {
			return
		}
		c.Texto = texto
	} else if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	c.Texto = strings.TrimSpace(c.Texto)
	if c.Texto == "" || len(c.Texto) > maxTextoFicha {
		fallo(w, http.StatusBadRequest, "bad_request", "Paste the text to read, up to 20000 characters.", nil)
		return
	}
	vista := s.cat.VistaModelo(m, idioma(r))
	texto, ok := s.pedirIA(w, r, api.PeticionIA{
		Accion: "rellenar", Tipo: "erp", Modo: "interactivo",
		Instruccion: ia.InstruccionFicha(vista, s.alm.ahora().UTC()), Contexto: c.Texto,
	})
	if !ok {
		return
	}
	data, relaciones := ia.FichaDe(m, vista, texto)
	etiquetas := map[string]string{}
	for clave, nombre := range relaciones {
		for _, f := range m.Campos {
			if f.Clave != clave || f.Tipo != catalogo.TipoRelacion {
				continue
			}
			if id, err := s.alm.BuscarPorEtiqueta(r.Context(), f.Modelo, nombre); err == nil {
				data[clave], etiquetas[id] = id, nombre
			} else if !errors.Is(err, ErrNoEncontrado) {
				falloDe(w, err)
				return
			}
		}
	}
	responder(w, http.StatusOK, map[string]any{"data": data, "etiquetas": etiquetas})
}
