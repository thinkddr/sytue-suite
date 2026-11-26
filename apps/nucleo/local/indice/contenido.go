// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package indice

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	sqlfotos "git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
)

// MaxTextoDocumento es lo que se guarda para buscar del contenido de un documento: lo
// demás no cambia el resultado y sí la memoria del Hub.
const MaxTextoDocumento = 64 << 10

// ----- El contenido de los documentos -----

// estadoDoc es lo que se lee del estado CRDT de un documento de la biblioteca: solo
// sus registros (`office/model.State`), que llevan el contenido entero bajo
// `api.DocContentKey` como una cadena JSON.
type estadoDoc struct {
	Registers map[string]struct {
		Value json.RawMessage `json:"value"`
	} `json:"registers"`
}

// textoDeDocumento es el texto que se ve del documento: el de su HTML, el de las celdas
// de sus hojas (no las fórmulas) o el de las cajas y notas de sus diapositivas.
func textoDeDocumento(kind api.DocKind, e estadoDoc) string {
	reg, ok := e.Registers[api.DocContentKey(kind)]
	if !ok {
		return ""
	}
	var valor string
	if json.Unmarshal(reg.Value, &valor) != nil || valor == "" {
		return ""
	}
	var partes []string
	switch kind {
	case api.DocSheet:
		var m struct {
			Sheets []struct {
				Name  string            `json:"name"`
				Cells map[string]string `json:"cells"`
			} `json:"sheets"`
		}
		if json.Unmarshal([]byte(valor), &m) != nil {
			return ""
		}
		for _, h := range m.Sheets {
			partes = append(partes, h.Name)
			celdas := make([]string, 0, len(h.Cells))
			for k := range h.Cells {
				celdas = append(celdas, k)
			}
			slices.Sort(celdas)
			for _, k := range celdas {
				if v := h.Cells[k]; v != "" && !strings.HasPrefix(v, "=") {
					partes = append(partes, v)
				}
			}
		}
	case api.DocSlides:
		var m struct {
			Slides []struct {
				Notes string `json:"notes"`
				Boxes []struct {
					HTML string `json:"html"`
					Alt  string `json:"alt"`
				} `json:"boxes"`
			} `json:"slides"`
		}
		if json.Unmarshal([]byte(valor), &m) != nil {
			return ""
		}
		for _, d := range m.Slides {
			for _, c := range d.Boxes {
				partes = append(partes, htmltext.Text(c.HTML), c.Alt)
			}
			partes = append(partes, htmltext.Text(d.Notes))
		}
	default:
		partes = append(partes, htmltext.Text(valor))
	}
	return recortar(strings.Join(noVacios(partes...), "\n"), MaxTextoDocumento)
}

// recortar deja s en n bytes como mucho, sin partir una letra.
func recortar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// ----- Fotos y álbumes -----

// mesesES y mesesEN son los nombres de los meses con los que se busca una foto por su
// fecha («agosto 2024», «august»).
var (
	mesesES = []string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	mesesEN = []string{"january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december"}
)

// textoDeFecha es lo que se busca de una fecha: la fecha, el año y el mes con su nombre.
func textoDeFecha(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	m := t.Month() - 1
	return fmt.Sprintf("%s %d %s %s", t.Format("2006-01-02"), t.Year(), mesesES[m], mesesEN[m])
}

// fototeca son las fotos y los álbumes de Photos de la carpeta dir: de su fototeca de
// SQLite (`local/fototeca`, leída sin abrirla para escribir) o, si aún no se ha migrado,
// del almacén de registros de antes.
func fototeca(dir string) ([]Elemento, error) {
	l, err := sqlfotos.LeerSinAbrir(dir)
	if err != nil {
		return nil, err
	}
	if !l.Migrada {
		filas, err := almacen.Leer(dir)
		return fotos(filas), err
	}
	out := make([]Elemento, 0, len(l.Albumes)+len(l.Fotos))
	for _, a := range l.Albumes {
		out = append(out, Elemento{Producto: "photos", Clase: Album, ID: a.ID, Titulo: a.Titulo, Fecha: a.Editado})
	}
	for _, x := range l.Fotos {
		out = append(out, elementoDeFoto(x.ID, x.Nombre, x.Descripcion, x.Camara, x.Etiquetas, x.Tomada, x.Creada))
	}
	return out, nil
}

// elementoDeFoto es una foto buscable (fotos).
func elementoDeFoto(id, nombre, descripcion, camara string, etiquetas []string, tomada *time.Time, creada time.Time) Elemento {
	cuando := creada
	if tomada != nil {
		cuando = *tomada
	}
	return Elemento{
		Producto: "photos", Clase: Foto, ID: id, Titulo: nombre, Detalle: descripcion,
		Texto: strings.Join(noVacios(append(slices.Clone(etiquetas), camara, textoDeFecha(cuando))...), "\n"),
		Fecha: cuando,
	}
}

// fotos son las fotos vivas, buscables por su nombre, su descripción, sus etiquetas, la
// cámara y la fecha («agosto 2024»), y los álbumes por su título (que es como la gente
// nombra los lugares: «Bilbao 2024»). Las coordenadas no tienen nombre sin un servicio
// que las traduzca, y quién está en cada álbum va en un objeto aparte del motor
// (`private_members`) que este lector no resuelve: ninguna de las dos cosas se busca.
func fotos(filas []almacen.Fila) []Elemento {
	type foto struct {
		ID          string     `json:"id"`
		Name        string     `json:"name"`
		Description string     `json:"description"`
		Tags        []string   `json:"tags"`
		Camera      string     `json:"camera"`
		TakenAt     *time.Time `json:"taken_at"`
		CreatedAt   time.Time  `json:"created_at"`
		DeletedAt   *time.Time `json:"deleted_at"`
		PurgedAt    *time.Time `json:"purged_at"`
	}
	type album struct {
		ID        string     `json:"id"`
		Title     string     `json:"title"`
		UpdatedAt time.Time  `json:"updated_at"`
		DeletedAt *time.Time `json:"deleted_at"`
	}
	var fs []foto
	albumes := map[string]album{}
	var orden []string
	for _, f := range filas {
		switch f.Entidad {
		case "asset":
			var x foto
			if json.Unmarshal(f.JSON, &x) == nil && x.DeletedAt == nil && x.PurgedAt == nil && x.ID != "" {
				fs = append(fs, x)
			}
		case "album":
			var x album
			if json.Unmarshal(f.JSON, &x) == nil && x.DeletedAt == nil && x.ID != "" {
				albumes[x.ID] = x
				orden = append(orden, x.ID)
			}
		}
	}
	out := make([]Elemento, 0, len(orden)+len(fs))
	for _, id := range orden {
		a := albumes[id]
		out = append(out, Elemento{Producto: "photos", Clase: Album, ID: a.ID, Titulo: a.Title, Fecha: a.UpdatedAt})
	}
	for _, x := range fs {
		out = append(out, elementoDeFoto(x.ID, x.Name, x.Description, x.Camera, x.Tags, x.TakenAt, x.CreatedAt))
	}
	return out
}

// ----- Bases -----

// MaxRegistrosBases acota los registros que se leen para buscar: el Hub busca, no
// replica una base de datos entera.
const MaxRegistrosBases = 20000

// filaBases es una base, una tabla, un campo o un registro del almacén de Bases.
type filaBases struct {
	ID        string          `json:"id"`
	BaseID    string          `json:"base_id"`
	TableID   string          `json:"table_id"`
	Name      string          `json:"name"`
	Desc      string          `json:"description"`
	Kind      string          `json:"kind"`
	Position  int             `json:"position"`
	Data      map[string]any  `json:"data"`
	UpdatedAt time.Time       `json:"updated_at"`
	DeletedAt json.RawMessage `json:"deleted_at"`
}

// bases son las bases y sus tablas por su nombre, y los registros por el texto de sus
// celdas (las de texto; un número o una fecha no se buscan como palabras).
func bases(filas []almacen.Fila) []Elemento {
	vivo := func(f filaBases) bool { return len(f.DeletedAt) == 0 || string(f.DeletedAt) == "null" }
	nombresBase := map[string]string{}
	tablas := map[string]filaBases{}
	campos := map[string][]filaBases{} // por tabla
	var regs []filaBases
	var out []Elemento
	for _, f := range filas {
		var x filaBases
		if json.Unmarshal(f.JSON, &x) != nil || !vivo(x) || x.ID == "" {
			continue
		}
		switch f.Entidad {
		case "base":
			nombresBase[x.ID] = x.Name
			out = append(out, Elemento{Producto: "bases", Clase: Base, ID: x.ID, Titulo: x.Name, Detalle: x.Desc, Fecha: cmp.Or(x.UpdatedAt, f.Fecha)})
		case "table":
			tablas[x.ID] = x
		case "field":
			campos[x.TableID] = append(campos[x.TableID], x)
		case "record":
			if len(regs) < MaxRegistrosBases {
				regs = append(regs, x)
			}
		}
	}
	donde := func(tabla string) string {
		t := tablas[tabla]
		return strings.Join(noVacios(nombresBase[t.BaseID], t.Name), " · ")
	}
	for _, t := range tablas {
		if _, ok := nombresBase[t.BaseID]; !ok {
			continue // de una base borrada
		}
		// Bases abre una tabla por `<base>/<tabla>` y un registro por `<base>/<tabla>/<registro>`.
		out = append(out, Elemento{Producto: "bases", Clase: Tabla, ID: t.BaseID + "/" + t.ID, Titulo: t.Name, Detalle: nombresBase[t.BaseID]})
	}
	for _, cs := range campos {
		slices.SortFunc(cs, func(a, b filaBases) int { return a.Position - b.Position })
	}
	for _, r := range regs {
		t, ok := tablas[r.TableID]
		if !ok {
			continue
		}
		if _, ok := nombresBase[t.BaseID]; !ok {
			continue
		}
		textos := textosDeRegistro(r, campos[r.TableID])
		if len(textos) == 0 {
			continue
		}
		out = append(out, Elemento{
			Producto: "bases", Clase: Registro, ID: t.BaseID + "/" + r.TableID + "/" + r.ID, Titulo: textos[0], Detalle: donde(r.TableID),
			Texto: strings.Join(textos[1:], "\n"), Fecha: r.UpdatedAt,
		})
	}
	return out
}

// textosDeRegistro son los textos no vacíos de las celdas de texto de un registro, en
// el orden de sus campos.
func textosDeRegistro(r filaBases, campos []filaBases) []string {
	var textos []string
	for _, c := range campos {
		if s, ok := r.Data[c.ID].(string); ok && deTexto(c.Kind) {
			textos = append(textos, s)
		}
	}
	return noVacios(textos...)
}

// deTexto dice si un campo de esa clase se busca como palabras.
func deTexto(clase string) bool {
	switch clase {
	case api.BasesKindText, "longtext", "email", "url", "phone", api.BasesKindSelect:
		return true
	}
	return false
}
