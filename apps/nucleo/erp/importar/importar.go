// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package importar lee un CSV para un modelo del ERP: la cabecera por clave o por
// rótulo, el separador por la cabecera, importes «1.234,56» o «1234.56», fechas ISO o
// dd/mm/aaaa y relaciones por su identificador o por su NOMBRE. Cada fila sale ya
// validada con dominio.Validar o como fallo con su número de línea. Lo usan
// services/syerp y el ERP de Sytue Office, que importan el mismo fichero igual.
package importar

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
)

var (
	// ErrCSV es un fichero que no se puede leer como CSV.
	ErrCSV = errors.New("importar: el fichero no se puede leer como CSV")
	// ErrSinColumnas es una cabecera en la que ninguna columna es un campo del modelo.
	ErrSinColumnas = errors.New("importar: ninguna columna es un campo del modelo")
	// ErrSinRegistro lo devuelve Buscar cuando no hay registro con ese nombre.
	ErrSinRegistro = errors.New("importar: no hay registro con ese nombre")

	uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// Buscar resuelve el nombre de un registro de `modelo` a su id (ErrSinRegistro si no hay).
type Buscar func(ctx context.Context, modelo, texto string) (string, error)

// Fila es una fila ya validada, lista para escribir.
type Fila struct {
	Linea    int
	Data     map[string]any
	Etiqueta string
	// Externo es el `externo_id` de la fila: con él se actualiza en vez de crear.
	Externo string
	// Estado es el de la columna «estado», si la hay.
	Estado string
}

// Fallo es una fila que no entró, por su línea.
type Fallo struct {
	Fila  int    `json:"fila"`
	Error string `json:"error"`
}

// Lectura es lo que sale de leer el fichero.
type Lectura struct {
	Filas     []Fila
	Fallos    []Fallo
	Ignoradas []string
	// Leidas son las filas con algo, buenas o malas.
	Leidas int
}

// Leer lee el CSV entero (hasta `tope` filas) y valida cada fila contra el modelo.
func Leer(ctx context.Context, cuerpo []byte, m *catalogo.Modelo, vista catalogo.ModeloVista, tope int, buscar Buscar) (Lectura, error) {
	lector := csv.NewReader(bytes.NewReader(cuerpo))
	lector.Comma = separadorDe(cuerpo)
	lector.FieldsPerRecord = -1
	lector.LazyQuotes = true
	lector.TrimLeadingSpace = true
	cabecera, err := lector.Read()
	if err != nil {
		return Lectura{}, ErrCSV
	}
	columnas, ignoradas := casarColumnas(cabecera, m, vista)
	if len(columnas) == 0 {
		return Lectura{}, ErrSinColumnas
	}
	conv := &conversor{vista: vista, buscar: buscar}
	out := Lectura{Fallos: []Fallo{}, Ignoradas: ignoradas}
	for n := 2; ; n++ {
		campos, err := lector.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			out.Fallos = append(out.Fallos, Fallo{Fila: n, Error: "línea ilegible"})
			continue
		}
		if len(campos) == 0 || (len(campos) == 1 && strings.TrimSpace(campos[0]) == "") {
			continue
		}
		out.Leidas++
		if out.Leidas > tope {
			out.Fallos = append(out.Fallos, Fallo{Fila: n, Error: fmt.Sprintf("tope de %d filas por importación", tope)})
			break
		}
		crudo, externo, estado, err := conv.fila(ctx, columnas, campos)
		if err != nil {
			out.Fallos = append(out.Fallos, Fallo{Fila: n, Error: err.Error()})
			continue
		}
		data, etiqueta, err := dominio.Validar(m, crudo)
		if err != nil {
			out.Fallos = append(out.Fallos, Fallo{Fila: n, Error: err.Error()})
			continue
		}
		out.Filas = append(out.Filas, Fila{Linea: n, Data: data, Etiqueta: etiqueta, Externo: externo, Estado: estado})
	}
	return out, nil
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
	vista  catalogo.ModeloVista
	buscar Buscar
	cache  map[string]string
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
		return CentimosDeDecimal(decimalCanonico(texto))
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
	if c.buscar == nil {
		return "", fmt.Errorf("no existe ningún registro llamado %q", texto)
	}
	id, err := c.buscar(ctx, modelo, texto)
	if errors.Is(err, ErrSinRegistro) {
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

// CentimosDeDecimal convierte "1234.56" en 123456 SIN pasar por float64.
//
// Con `strconv.ParseFloat` y una multiplicación, importes perfectamente normales
// se van un céntimo: 1234.56*100 da 123455.99999999999 en binario, y truncar da
// 123455. Un céntimo por factura, en miles de facturas, es un descuadre que
// aparece meses después y no se sabe de dónde sale.
func CentimosDeDecimal(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	negativo := false
	switch s[0] {
	case '-':
		negativo, s = true, s[1:]
	case '+':
		s = s[1:]
	}
	entera, decimal, _ := strings.Cut(s, ".")
	if entera == "" {
		entera = "0"
	}
	// Se redondea a dos cifras al MODO COMERCIAL (medio hacia arriba), que es lo
	// que hace cualquier hoja de cálculo y lo que espera quien mira el total.
	redondeo := int64(0)
	if len(decimal) > 2 {
		if decimal[2] >= '5' {
			redondeo = 1
		}
		decimal = decimal[:2]
	}
	for len(decimal) < 2 {
		decimal += "0"
	}
	var total int64
	for _, r := range entera + decimal {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("%q no es un importe decimal", s)
		}
		// 92 billones de euros en céntimos es el techo de int64; por encima, el
		// dato de entrada está mal y decirlo es mejor que desbordar en silencio.
		if total > (1<<62)/10 {
			return 0, fmt.Errorf("importe fuera de rango")
		}
		total = total*10 + int64(r-'0')
	}
	total += redondeo
	if negativo {
		total = -total
	}
	return total, nil
}
