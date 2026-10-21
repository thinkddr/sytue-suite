// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

// # Sytue AI en el ERP (`SytueIA.tsx` y `services/syerp/internal/http/handlers_ia.go`)
//
// Las reglas del ERP, ya decididas: la IA solo ve el ESQUEMA (los nombres de los campos,
// nunca los datos), cada informe lo ejecuta el servicio en memoria y sin SQL con los
// permisos de quien lo pide, y lo que se crea pasa antes por la persona. Por eso aquí
// hay DOS llamadas por cosa —proponer y ejecutar—, como en la web: la propuesta tarda
// del orden de diez segundos y la persona ve qué va a salir antes de guardar nada.
//
// Las formas son las de `services/syerp/internal/ia` y las ata
// `TestERPIAHablaElIdiomaDelServicio` leyendo el código del servicio.

// ERPParaTablero y ERPParaExcel son el `para` de un informe: qué se va a hacer con él.
const (
	ERPParaTablero = "tablero"
	ERPParaExcel   = "excel"
)

// Las vistas de una parte de un informe.
const (
	ERPVistaCifra  = "cifra"
	ERPVistaBarras = "barras"
	ERPVistaTabla  = "tabla"
)

// ERPInforme es lo que Sytue AI propone: un título y sus partes, todavía sin datos.
type ERPInforme struct {
	Titulo string            `json:"titulo"`
	Partes []ERPParteInforme `json:"partes"`
}

// ERPParteInforme es una cifra, unas barras o una tabla. La consulta viaja TAL CUAL: la
// ejecuta el servicio, no el cliente, y reescribirla aquí perdería lo que no conociera.
type ERPParteInforme struct {
	Titulo   string          `json:"titulo"`
	Vista    string          `json:"vista"`
	Consulta json.RawMessage `json:"consulta"`
}

// ERPColumnaInforme es una columna de una tabla de informe. Tipo es texto, numero,
// dinero, fecha o booleano.
type ERPColumnaInforme struct {
	Clave    string `json:"clave"`
	Tipo     string `json:"tipo"`
	Etiqueta string `json:"etiqueta"`
}

// Los tipos de columna que se formatean distinto.
const (
	ERPColDinero   = "dinero" // en UNIDADES, no en céntimos (a diferencia de los registros)
	ERPColNumero   = "numero"
	ERPColFecha    = "fecha"
	ERPColBooleano = "booleano"
)

// ERPTablaInforme son las filas de una parte. Error dice por qué no las hay:
// `sin_permiso`, `no_disponible` o `invalida`.
type ERPTablaInforme struct {
	Columnas  []ERPColumnaInforme `json:"columnas"`
	Filas     [][]any             `json:"filas"`
	Recortada bool                `json:"recortada,omitempty"`
	Error     string              `json:"error,omitempty"`
}

// ERPTablero es un tablero guardado de Sytue AI.
type ERPTablero struct {
	ID      string     `json:"id"`
	Titulo  string     `json:"titulo"`
	Informe ERPInforme `json:"informe"`
}

// Tableros lista los tableros guardados de la organización (`GET /api/v1/tableros`).
func (e *ERP) Tableros(ctx context.Context) ([]ERPTablero, error) {
	r, err := JSON[struct {
		Tableros []ERPTablero `json:"tableros"`
	}](ctx, e.c, e.pedir("/tableros", nil))
	return r.Tableros, err
}

// postJSON manda `cuerpo` en JSON y lee la respuesta en T.
func postJSON[T any](ctx context.Context, e *ERP, ruta string, cuerpo any) (T, error) {
	var cero T
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return cero, err
	}
	r := e.pedir(ruta, nil)
	r.Method, r.Body = http.MethodPost, datos
	r.Header.Set("Content-Type", "application/json")
	return JSON[T](ctx, e.c, r)
}

// ProponerInforme convierte una frase en un informe (`POST /api/v1/ia/informe`). `para`
// es ERPParaTablero o ERPParaExcel y `modelo` la lista abierta, si hay una: la petición
// se entiende sobre ella.
func (e *ERP) ProponerInforme(ctx context.Context, peticion, para, modelo string) (ERPInforme, error) {
	r, err := postJSON[struct {
		Informe ERPInforme `json:"informe"`
	}](ctx, e, "/ia/informe", map[string]string{"peticion": peticion, "para": para, "modelo": modelo})
	return r.Informe, err
}

// DatosInforme ejecuta el informe con los datos de AHORA y devuelve una tabla por parte
// (`POST /api/v1/informes/datos`).
func (e *ERP) DatosInforme(ctx context.Context, inf ERPInforme) ([]ERPTablaInforme, error) {
	r, err := postJSON[struct {
		Datos []ERPTablaInforme `json:"datos"`
	}](ctx, e, "/informes/datos", map[string]ERPInforme{"informe": inf})
	return r.Datos, err
}

// ExcelInforme devuelve el informe como libro de Excel, una hoja por parte
// (`POST /api/v1/informes/excel`). Quien llama cierra el lector.
func (e *ERP) ExcelInforme(ctx context.Context, inf ERPInforme) (io.ReadCloser, error) {
	datos, err := json.Marshal(map[string]ERPInforme{"informe": inf})
	if err != nil {
		return nil, err
	}
	r := e.pedir("/informes/excel", nil)
	r.Method, r.Body = http.MethodPost, datos
	r.Header.Set("Content-Type", "application/json")
	resp, err := e.c.Do(ctx, r)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// GuardarTablero guarda el informe como tablero (`POST /api/v1/tableros`).
func (e *ERP) GuardarTablero(ctx context.Context, inf ERPInforme) (ERPTablero, error) {
	r, err := postJSON[struct {
		Tablero ERPTablero `json:"tablero"`
	}](ctx, e, "/tableros", map[string]ERPInforme{"informe": inf})
	return r.Tablero, err
}

// BorrarTablero lo borra (`DELETE /api/v1/tableros/{id}`). Los datos no se tocan.
func (e *ERP) BorrarTablero(ctx context.Context, id string) error {
	return e.sinCuerpo(ctx, http.MethodDelete, "/tableros/"+url.PathEscape(id), nil)
}

// ERPRegistroCorto es un registro que ya existía, como lo enseña el plan.
type ERPRegistroCorto struct {
	ID       string `json:"id"`
	Etiqueta string `json:"etiqueta"`
}

// ERPPasoPlan es un registro por crear, o uno que ya existía y se reutiliza.
type ERPPasoPlan struct {
	// Ref es el nombre del paso para que otro apunte a él («$p1»).
	Ref       string            `json:"ref"`
	Modelo    string            `json:"modelo"`
	Etiqueta  string            `json:"etiqueta,omitempty"`
	Existente *ERPRegistroCorto `json:"existente,omitempty"`
	Data      map[string]any    `json:"data"`
	// Enlaces son las relaciones a OTRO paso del plan: campo → ref.
	Enlaces   map[string]string `json:"enlaces,omitempty"`
	Etiquetas map[string]string `json:"etiquetas,omitempty"`
	Faltan    []string          `json:"faltan,omitempty"`
	Avisos    []string          `json:"avisos,omitempty"`
	// Tarea es el resumen de la tarea que se creará con el registro.
	Tarea string `json:"tarea,omitempty"`
}

// ERPPlan es lo que Sytue AI propone crear a partir de una frase.
type ERPPlan struct {
	Resumen string        `json:"resumen"`
	Avisos  []string      `json:"avisos,omitempty"`
	Pasos   []ERPPasoPlan `json:"pasos"`
}

// Incompleto dice si a algún paso nuevo le falta un campo obligatorio: entonces no se
// puede crear, y la web apaga «Crear todo».
func (p ERPPlan) Incompleto() bool {
	for _, x := range p.Pasos {
		if x.Existente == nil && len(x.Faltan) > 0 {
			return true
		}
	}
	return false
}

// ProponerAcciones convierte una frase en un plan de registros (`POST /api/v1/ia/acciones`).
// No crea nada.
func (e *ERP) ProponerAcciones(ctx context.Context, peticion string) (ERPPlan, error) {
	return postJSON[ERPPlan](ctx, e, "/ia/acciones", map[string]string{"peticion": peticion})
}

// ERPCreados es lo que salió de crear un plan: o entran todos, o ninguno.
type ERPCreados struct {
	Creados     []ERPRegistro  `json:"creados"`
	Actividades []ERPActividad `json:"actividades"`
}

// CrearAcciones crea los pasos del plan de una vez (`POST /api/v1/ia/acciones/crear`).
func (e *ERP) CrearAcciones(ctx context.Context, pasos []ERPPasoPlan) (ERPCreados, error) {
	return postJSON[ERPCreados](ctx, e, "/ia/acciones/crear", map[string][]ERPPasoPlan{"pasos": pasos})
}

// ERPRelleno son los campos que Sytue AI ha leído de un texto, y el nombre de las
// relaciones que ha encontrado. No se ha guardado nada.
type ERPRelleno struct {
	Data      map[string]any    `json:"data"`
	Etiquetas map[string]string `json:"etiquetas"`
}

// RellenarFicha lee un texto pegado y devuelve los campos de una ficha de `modelo`
// (`POST /api/v1/{modelo}/ia/rellenar`).
func (e *ERP) RellenarFicha(ctx context.Context, modelo, texto string) (ERPRelleno, error) {
	return postJSON[ERPRelleno](ctx, e, "/"+url.PathEscape(modelo)+"/ia/rellenar", map[string]string{"texto": texto})
}

// RellenarFichaPDF hace lo mismo con el PDF de una factura, tal cual: el servicio lo lee
// con Sytue PDF y sigue igual que con un texto. Un 422 es un PDF sin texto (un escaneo).
func (e *ERP) RellenarFichaPDF(ctx context.Context, modelo string, pdf []byte) (ERPRelleno, error) {
	r := e.pedir("/"+url.PathEscape(modelo)+"/ia/rellenar", nil)
	r.Method, r.Body = http.MethodPost, pdf
	r.Header.Set("Content-Type", "application/pdf")
	return JSON[ERPRelleno](ctx, e.c, r)
}
