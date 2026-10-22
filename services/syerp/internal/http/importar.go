// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/banca"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/conectores"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

const (
	// maxImportBytes acota el cuerpo. Ocho megas de CSV son decenas de miles de
	// filas: más que eso se parte en varios ficheros, como en cualquier ERP.
	maxImportBytes = 8 << 20
	// maxImportFilas acota lo que se procesa en UNA petición, que tiene 30 s.
	maxImportFilas = 5000
)

var (
	errCSVInvalido = httpx.NewError(http.StatusUnprocessableEntity, "csv_invalido",
		"The file could not be read as CSV.")
	errSinColumnas = httpx.NewError(http.StatusUnprocessableEntity, "csv_sin_columnas",
		"No column header matches a field of this model.")
	uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

type falloDeFila struct {
	Fila  int    `json:"fila"`
	Error string `json:"error"`
}

// Importar — POST /api/v1/{modelo}/importar — carga un CSV en el modelo. Si lo
// que llega es XML, es una factura electrónica recibida (ver efactura.go).
//
// ES LA INTEROPERABILIDAD UNIVERSAL: todo ERP exporta CSV, así que esto es lo
// que hace que quien llega de Holded, Sage, SAP o una hoja de cálculo tenga sus
// datos el primer día sin esperar a un conector. Acepta el mismo fichero que
// produce la exportación de este ERP (mismas columnas, mismos rótulos), y casi
// cualquier otro:
//
//   - la cabecera puede ir por clave de campo (`nombre`) o por rótulo en el
//     idioma de la petición (`Nombre`); las columnas que no casan se ignoran y
//     se devuelven, para que se vea qué se quedó fuera;
//   - el separador se deduce de la cabecera (coma, punto y coma o tabulador),
//     porque Excel en español exporta con punto y coma;
//   - los importes admiten «1.234,56» y «1234.56»; las fechas, ISO y dd/mm/aaaa;
//   - una relación admite el identificador o el NOMBRE del registro apuntado.
//
// Con columnaEntrante `externo_id` rellena, la fila se crea o actualiza por ese id
// (importar dos veces no duplica); sin ella, se crea. Una fila mala no aborta el
// resto: se cuenta y se devuelve con su número de línea.
func (h *Handlers) Importar(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	cuerpo, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImportBytes))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The file is too large (8 MiB max)."))
		return
	}
	cuerpo = bytes.TrimPrefix(cuerpo, []byte("\xef\xbb\xbf"))
	if esXML(cuerpo) {
		h.importarFacturas(w, r, m, cuerpo)
		return
	}
	if (m.Clave == "linea_extracto" || m.Clave == "extracto_bancario") && banca.EsN43(cuerpo) {
		h.importarN43(w, r, cuerpo)
		return
	}
	h.importarCSV(w, r, m, cuerpo)
}

// importarCSV lee el fichero, valida fila a fila y escribe el LOTE ENTERO de
// una vez (store.Importar), que es lo que lo deja revertible.
//
// Con `?simular=1` hace exactamente lo mismo contra la base y lo deshace: el
// informe que devuelve es lo que pasaría, comprobado contra las filas y las
// reglas que hay de verdad, no contra una copia en memoria de las reglas que se
// desalinearía a la primera.
func (h *Handlers) importarCSV(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo, cuerpo []byte) {
	lector := csv.NewReader(bytes.NewReader(cuerpo))
	lector.Comma = separadorDe(cuerpo)
	lector.FieldsPerRecord = -1
	lector.LazyQuotes = true
	lector.TrimLeadingSpace = true
	cabecera, err := lector.Read()
	if err != nil {
		httpx.Fail(w, r, errCSVInvalido)
		return
	}

	idioma := httpx.LanguageFromRequest(r)
	vista := h.cat.VistaModelo(m, idioma)
	columnas, ignoradas := casarColumnas(cabecera, m, vista)
	if len(columnas) == 0 {
		httpx.Fail(w, r, errSinColumnas)
		return
	}

	conv := &conversor{h: h, vista: vista}
	ctx := r.Context()
	var filas []store.FilaImportada
	previos := []store.FalloDeFila{}
	leidas := 0
	for n := 2; ; n++ {
		campos, err := lector.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			previos = append(previos, store.FalloDeFila{Fila: n, Error: "línea ilegible"})
			continue
		}
		if len(campos) == 0 || (len(campos) == 1 && strings.TrimSpace(campos[0]) == "") {
			continue
		}
		leidas++
		if leidas > maxImportFilas {
			previos = append(previos, store.FalloDeFila{Fila: n, Error: fmt.Sprintf("tope de %d filas por importación", maxImportFilas)})
			break
		}
		crudo, externo, estado, err := conv.fila(ctx, columnas, campos)
		if err != nil {
			previos = append(previos, store.FalloDeFila{Fila: n, Error: err.Error()})
			continue
		}
		data, etiqueta, err := domain.Validar(m, crudo)
		if err != nil {
			previos = append(previos, store.FalloDeFila{Fila: n, Error: err.Error()})
			continue
		}
		filas = append(filas, store.FilaImportada{
			Linea: n, Data: data, Etiqueta: etiqueta, Externo: externo, Estado: estado,
		})
	}

	simular := r.URL.Query().Get("simular") == "1"
	var res store.ResultadoImportacion
	if simular {
		res, err = h.store.SimularImportacion(ctx, m, filas, sub(r))
	} else {
		res, err = h.store.Importar(ctx, m, filas, nombreDelFichero(r), sub(r))
	}
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	// Los fallos de lectura y validación van con los de escritura, en orden de
	// línea: para quien manda el fichero es una sola lista de qué no entró.
	res.Fallos = append(previos, res.Fallos...)
	sort.Slice(res.Fallos, func(i, j int) bool { return res.Fallos[i].Fila < res.Fallos[j].Fila })
	res.Filas = leidas
	httpx.JSON(w, r, http.StatusOK, struct {
		store.ResultadoImportacion
		Simulacion bool     `json:"simulacion"`
		Ignoradas  []string `json:"ignoradas"`
	}{res, simular, ignoradas})
}

// nombreDelFichero es el nombre que el cliente diga que tiene el fichero. Es
// con lo que quien lo subió reconoce su lote en la lista; si no lo manda, el
// lote sigue existiendo y se identifica por su hora.
func nombreDelFichero(r *http.Request) string {
	n := strings.TrimSpace(r.URL.Query().Get("fichero"))
	if len(n) > 300 {
		n = n[:300]
	}
	return n
}

// Importaciones — GET /api/v1/importaciones — los lotes recientes.
func (h *Handlers) Importaciones(w http.ResponseWriter, r *http.Request) {
	lotes, err := h.store.Importaciones(r.Context(), 50)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Un lote dice qué fichero entró en qué modelo y cuántas filas: de los módulos
	// que quien pide no puede leer, nada (NEG-17).
	visibles := make([]store.LoteImportado, 0, len(lotes))
	for _, l := range lotes {
		if h.accesoRegistro(r.Context(), l.Modelo, permisos.Lectura) {
			visibles = append(visibles, l)
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"importaciones": visibles})
}

// RevertirImportacion — POST /api/v1/importaciones/{id}/revertir — deshace un
// lote entero.
func (h *Handlers) RevertirImportacion(w http.ResponseWriter, r *http.Request) {
	res, err := h.store.Revertir(r.Context(), chi.URLParam(r, "id"), sub(r))
	switch {
	case errors.Is(err, store.ErrNoEncontrado):
		httpx.Fail(w, r, httpx.ErrNotFound)
	case errors.Is(err, store.ErrYaRevertida):
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "ya_revertida",
			"That import was already undone."))
	case err != nil:
		httpx.Fail(w, r, errorDeEscritura(err))
	default:
		httpx.JSON(w, r, http.StatusOK, res)
	}
}

// separadorDe elige el separador que MÁS aparece en la primera línea. Excel en
// español exporta con punto y coma y en inglés con coma; un fichero de una
// herramienta de datos suele traer tabuladores.
func separadorDe(cuerpo []byte) rune {
	primera, _, _ := bytes.Cut(cuerpo, []byte("\n"))
	mejor, cuenta := ',', bytes.Count(primera, []byte(","))
	for _, c := range []struct {
		r rune
		b []byte
	}{{';', []byte(";")}, {'\t', []byte("\t")}} {
		if n := bytes.Count(primera, c.b); n > cuenta {
			mejor, cuenta = c.r, n
		}
	}
	return mejor
}

// columnaEntrante es una columnaEntrante del CSV casada con un campo del modelo. `estado` y
// `externo_id` son columnas especiales: no van a `data`.
type columnaEntrante struct {
	indice int
	campo  *catalogo.CampoVista
	// especial: "estado" o "externo_id"; vacío para un campo normal.
	especial string
}

func normalizar(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func casarColumnas(cabecera []string, m *catalogo.Modelo, vista catalogo.ModeloVista) ([]columnaEntrante, []string) {
	porNombre := map[string]*catalogo.CampoVista{}
	for i := range vista.Campos {
		c := &vista.Campos[i]
		porNombre[c.Clave] = c
		porNombre[normalizar(c.Etiqueta)] = c
	}
	var cols []columnaEntrante
	var ignoradas []string
	vistos := map[string]bool{}
	for i, titulo := range cabecera {
		n := normalizar(titulo)
		switch {
		// `externo_id` solo sirve de clave de actualización si el modelo lo
		// declara: es donde se guarda, y sin él no habría por dónde reconocer
		// la fila la próxima vez.
		case n == "externo_id" && porNombre["externo_id"] != nil:
			cols = append(cols, columnaEntrante{indice: i, especial: "externo_id"})
			continue
		case n == "estado" && len(m.Estados) > 0:
			cols = append(cols, columnaEntrante{indice: i, especial: "estado"})
			continue
		}
		c, ok := porNombre[n]
		if !ok || vistos[c.Clave] {
			ignoradas = append(ignoradas, titulo)
			continue
		}
		vistos[c.Clave] = true
		cols = append(cols, columnaEntrante{indice: i, campo: c})
	}
	if ignoradas == nil {
		ignoradas = []string{}
	}
	return cols, ignoradas
}

// conversor pasa el texto de cada celda a la forma JSON que espera `domain.Validar`,
// y resuelve nombres de relaciones contra el store (con caché por importación:
// el mismo cliente aparece en cien facturas).
type conversor struct {
	h     *Handlers
	vista catalogo.ModeloVista
	cache map[string]string
}

func (c *conversor) fila(ctx context.Context, cols []columnaEntrante, campos []string) (json.RawMessage, string, string, error) {
	data := map[string]any{}
	externo, estado := "", ""
	for _, col := range cols {
		if col.indice >= len(campos) {
			continue
		}
		texto := strings.TrimSpace(campos[col.indice])
		switch col.especial {
		case "externo_id":
			externo = texto
			if texto != "" {
				data["externo_id"] = texto
			}
			continue
		case "estado":
			estado = c.estado(texto)
			if texto != "" && estado == "" {
				return nil, "", "", fmt.Errorf("estado: %q no es un estado del modelo", texto)
			}
			continue
		}
		if texto == "" {
			continue
		}
		valor, err := c.valor(ctx, col.campo, texto)
		if err != nil {
			return nil, "", "", fmt.Errorf("%s: %w", col.campo.Etiqueta, err)
		}
		data[col.campo.Clave] = valor
	}
	crudo, err := json.Marshal(data)
	return crudo, externo, estado, err
}

func (c *conversor) estado(texto string) string {
	n := normalizar(texto)
	for _, e := range c.vista.Estados {
		if e.Valor == n || normalizar(e.Etiqueta) == n {
			return e.Valor
		}
	}
	return ""
}

func (c *conversor) valor(ctx context.Context, campo *catalogo.CampoVista, texto string) (any, error) {
	switch campo.Tipo {
	case catalogo.TipoEntero:
		i, err := strconv.ParseInt(strings.ReplaceAll(texto, " ", ""), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("se esperaba un entero")
		}
		return i, nil
	case catalogo.TipoDecimal:
		return decimalCanonico(texto), nil
	case catalogo.TipoDinero:
		return conectores.CentimosDeDecimal(decimalCanonico(texto))
	case catalogo.TipoFecha:
		return fechaISO(texto)
	case catalogo.TipoFechaHora:
		for _, f := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02T15:04", "02/01/2006 15:04"} {
			if t, err := time.Parse(f, texto); err == nil {
				return t.UTC().Format(time.RFC3339), nil
			}
		}
		return nil, fmt.Errorf("fecha y hora ilegibles")
	case catalogo.TipoBooleano:
		switch normalizar(texto) {
		case "1", "si", "sí", "yes", "true", "x", "oui", "ja", "sì":
			return true, nil
		case "0", "no", "false", "non", "nein":
			return false, nil
		}
		return nil, fmt.Errorf("se esperaba sí o no")
	case catalogo.TipoSeleccion:
		n := normalizar(texto)
		for _, o := range campo.Opciones {
			if o.Valor == n || normalizar(o.Etiqueta) == n {
				return o.Valor, nil
			}
		}
		return nil, fmt.Errorf("%q no es una opción", texto)
	case catalogo.TipoRelacion:
		return c.relacion(ctx, campo.Modelo, texto)
	}
	return texto, nil
}

func (c *conversor) relacion(ctx context.Context, modelo, texto string) (string, error) {
	if uuidRe.MatchString(texto) {
		return texto, nil
	}
	clave := modelo + "\x00" + normalizar(texto)
	if id, ok := c.cache[clave]; ok {
		return id, nil
	}
	id, err := c.h.store.BuscarPorEtiqueta(ctx, modelo, texto)
	if errors.Is(err, store.ErrNoEncontrado) {
		return "", fmt.Errorf("no existe ningún registro llamado %q", texto)
	}
	if err != nil {
		return "", err
	}
	if c.cache == nil {
		c.cache = map[string]string{}
	}
	c.cache[clave] = id
	return id, nil
}

// decimalCanonico pasa «1.234,56», «1,234.56» o «1234,56» a «1234.56». Cuando
// hay coma y punto, el ÚLTIMO es el separador decimal y el otro, de miles. Con
// uno solo, es de miles si se repite (1.234.567) y decimal en el resto de casos:
// «1,234» se lee como uno coma doscientos treinta y cuatro, que es lo que
// escribe una persona en castellano; quien exporta miles con coma exporta
// también decimales con punto, y cae en el primer caso.
func decimalCanonico(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), " ", ""), "€", "")
	coma, punto := strings.LastIndex(s, ","), strings.LastIndex(s, ".")
	switch {
	case coma >= 0 && punto >= 0:
		if coma > punto {
			s = strings.ReplaceAll(s, ".", "")
			s = strings.Replace(s, ",", ".", 1)
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}
	case strings.Count(s, ",") > 1:
		s = strings.ReplaceAll(s, ",", "")
	case coma >= 0:
		s = strings.Replace(s, ",", ".", 1)
	case strings.Count(s, ".") > 1:
		s = strings.ReplaceAll(s, ".", "")
	}
	return s
}

func fechaISO(texto string) (string, error) {
	for _, f := range []string{"2006-01-02", "02/01/2006", "02-01-2006", "02.01.2006", "2006/01/02"} {
		if t, err := time.Parse(f, texto); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("fecha ilegible (AAAA-MM-DD o DD/MM/AAAA)")
}
