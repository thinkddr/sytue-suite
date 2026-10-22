// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ia es Sytue AI dentro del ERP: informes que se piden en una frase (un
// tablero o un Excel) y fichas que se rellenan pegando un texto.
//
// LA IA NO VE LOS DATOS DE UN INFORME. Recibe qué hay —las fuentes y sus campos—
// y devuelve una consulta. La consulta la valida y la ejecuta este servicio, con
// los permisos de quien la pide: la facturación no viaja a un modelo de lenguaje,
// y lo que el modelo se invente (un campo que no existe, una operación rara) se
// queda en una parte descartada, no en una consulta.
//
// Y NO SE GENERA SQL. La consulta se aplica en memoria sobre las filas de su
// fuente, que ya salen del store con su RLS. Así ninguna clave que escriba un
// modelo acaba dentro de una sentencia.
package ia

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
)

// Tipos de campo de un informe. Menos que los del catálogo a propósito: para
// filtrar, agrupar y sumar da igual que un texto sea un correo o un teléfono.
const (
	TipoTexto    = "texto"
	TipoNumero   = "numero"   // float64 en la fila
	TipoDinero   = "dinero"   // int64 en CÉNTIMOS en la fila, como en el resto del ERP
	TipoFecha    = "fecha"    // "AAAA-MM-DD" en la fila
	TipoBooleano = "booleano" // bool en la fila
)

// Vistas de una parte.
const (
	VistaCifra  = "cifra"
	VistaBarras = "barras"
	VistaTabla  = "tabla"
)

// Topes de un informe: lo que un modelo puede pedir sin convertir un tablero en
// una descarga de la base entera.
const (
	MaxPartes   = 12
	MaxColumnas = 30
	MaxFiltros  = 10
)

// Errores de una tabla que no se pudo llenar.
const (
	ErrorSinPermiso   = "sin_permiso"
	ErrorNoDisponible = "no_disponible"
	ErrorInvalida     = "invalida"
)

// ErrInforme es una respuesta del modelo que no es un informe.
var ErrInforme = errors.New("informe inválido")

// Informe es lo que se pide en una frase. Es la MISMA estructura para un tablero
// y para un Excel: un tablero se descarga como Excel, y cada parte es una hoja.
type Informe struct {
	Titulo string  `json:"titulo"`
	Partes []Parte `json:"partes"`
}

// Parte es un panel del tablero o una hoja del Excel.
type Parte struct {
	Titulo   string   `json:"titulo"`
	Vista    string   `json:"vista"`
	Consulta Consulta `json:"consulta"`
}

// Consulta dice qué sale de una fuente.
type Consulta struct {
	Fuente   string   `json:"fuente"`
	Columnas []string `json:"columnas,omitempty"`
	Filtros  []Filtro `json:"filtros,omitempty"`
	// Agrupar es un campo; si es una fecha, "campo:dia", "campo:mes" o "campo:anio".
	Agrupar string `json:"agrupar,omitempty"`
	// Medida es "cuenta", "suma:campo" o "media:campo".
	Medida string `json:"medida,omitempty"`
	// Orden es un campo (o "valor"/"grupo" si hay medida); con "-" delante, descendente.
	Orden  string `json:"orden,omitempty"`
	Limite int    `json:"limite,omitempty"`
}

// Filtro es una condición sobre un campo. Todas se cumplen a la vez.
type Filtro struct {
	Campo string `json:"campo"`
	Op    string `json:"op"`
	Valor string `json:"valor"`
}

// Campo es una columna de una fuente o de una tabla.
type Campo struct {
	Clave    string `json:"clave"`
	Tipo     string `json:"tipo"`
	Etiqueta string `json:"etiqueta"`
	// Valores traduce lo guardado a su rótulo (selecciones y estados): el modelo
	// filtra por el valor y la tabla sale con el rótulo.
	Valores map[string]string `json:"-"`
}

// Fuente es de dónde lee un informe: un modelo del ERP o un listado de otra app.
type Fuente struct {
	Clave    string
	Etiqueta string
	// App es la app de la que sale; vacía para el ERP.
	App    string
	Campos []Campo
}

func (f Fuente) campo(clave string) (Campo, bool) {
	for _, c := range f.Campos {
		if c.Clave == clave {
			return c, true
		}
	}
	return Campo{}, false
}

// Fila es un registro de una fuente con cada valor ya en su tipo (ver Tipo*).
type Fila map[string]any

// Tabla es el resultado de una parte.
type Tabla struct {
	Columnas []Campo `json:"columnas"`
	// Filas lleva números como número (los importes, en unidades y con dos
	// decimales exactos), fechas AAAA-MM-DD y selecciones con su rótulo.
	Filas     [][]any `json:"filas"`
	Recortada bool    `json:"recortada,omitempty"`
	// Error dice por qué no hay datos: sin_permiso, no_disponible o invalida.
	Error string `json:"error,omitempty"`
	// Procedencia dice de dónde salen los datos cuando no son del ERP ni de la sesión de
	// quien pide: hoy, una vista de Sytue Bases (base, tabla, vista, quién la concedió y
	// cuándo, versión del token y hora de la lectura).
	Procedencia map[string]any `json:"procedencia,omitempty"`
}

// InformeDe lee la respuesta del modelo.
func InformeDe(texto string) (Informe, error) {
	var inf Informe
	if err := json.Unmarshal([]byte(asistente.JSON(texto)), &inf); err != nil {
		return Informe{}, fmt.Errorf("%w: la respuesta no es un informe", ErrInforme)
	}
	return inf, nil
}

// Comprobar valida cada parte contra las fuentes y devuelve el error de cada una
// (nil si vale), en el mismo orden. Normaliza al paso lo que tiene arreglo: la
// vista vacía es una tabla y agrupar sin medida es contar.
func (inf *Informe) Comprobar(fuentes map[string]Fuente) []error {
	inf.Titulo = recortar(inf.Titulo, 120)
	errs := make([]error, len(inf.Partes))
	for i := range inf.Partes {
		p := &inf.Partes[i]
		p.Titulo = recortar(p.Titulo, 120)
		if p.Vista == "" {
			p.Vista = VistaTabla
		}
		f, ok := fuentes[p.Consulta.Fuente]
		switch {
		case p.Vista != VistaCifra && p.Vista != VistaBarras && p.Vista != VistaTabla:
			errs[i] = fmt.Errorf("vista %q desconocida", p.Vista)
		case !ok:
			errs[i] = fmt.Errorf("fuente %q desconocida", p.Consulta.Fuente)
		default:
			errs[i] = p.Consulta.comprobar(f, p.Vista)
		}
	}
	return errs
}

var operaciones = []string{"=", "!=", ">", ">=", "<", "<=", "contiene", "empieza"}

func (c *Consulta) comprobar(f Fuente, vista string) error {
	if len(c.Columnas) > MaxColumnas || len(c.Filtros) > MaxFiltros {
		return errors.New("demasiadas columnas o filtros")
	}
	for _, paso := range []func(Fuente) error{c.comprobarCampos, c.comprobarAgrupar, c.comprobarMedida, c.comprobarOrden} {
		if err := paso(f); err != nil {
			return err
		}
	}
	c.Limite = max(c.Limite, 0)
	switch {
	case vista == VistaCifra && (c.Medida == "" || c.Agrupar != ""):
		return errors.New("una cifra es una medida sin agrupar")
	case vista == VistaBarras && c.Agrupar == "":
		return errors.New("unas barras necesitan agrupar")
	}
	return nil
}

func (c *Consulta) comprobarCampos(f Fuente) error {
	for _, col := range c.Columnas {
		if _, ok := f.campo(col); !ok {
			return fmt.Errorf("campo %q desconocido", col)
		}
	}
	for _, fl := range c.Filtros {
		campo, ok := f.campo(fl.Campo)
		switch {
		case !ok:
			return fmt.Errorf("campo %q desconocido", fl.Campo)
		case !slices.Contains(operaciones, fl.Op):
			return fmt.Errorf("operación %q desconocida", fl.Op)
		}
		if _, err := objetivo(campo, fl.Valor, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

func (c *Consulta) comprobarAgrupar(f Fuente) error {
	if c.Agrupar == "" {
		return nil
	}
	clave, unidad, _ := strings.Cut(c.Agrupar, ":")
	campo, ok := f.campo(clave)
	if !ok {
		return fmt.Errorf("campo %q desconocido", clave)
	}
	if unidad != "" && (campo.Tipo != TipoFecha || !slices.Contains([]string{"dia", "mes", "anio"}, unidad)) {
		return fmt.Errorf("agrupación %q imposible", c.Agrupar)
	}
	if c.Medida == "" {
		c.Medida = "cuenta"
	}
	return nil
}

func (c *Consulta) comprobarMedida(f Fuente) error {
	op, clave, _ := strings.Cut(c.Medida, ":")
	switch op {
	case "":
		return nil
	case "cuenta":
		c.Medida = "cuenta"
		return nil
	case "suma", "media":
		if campo, ok := f.campo(clave); ok && (campo.Tipo == TipoNumero || campo.Tipo == TipoDinero) {
			return nil
		}
		return fmt.Errorf("%s necesita un campo numérico", op)
	}
	return fmt.Errorf("medida %q desconocida", c.Medida)
}

func (c *Consulta) comprobarOrden(f Fuente) error {
	orden := strings.TrimPrefix(c.Orden, "-")
	_, esCampo := f.campo(orden)
	switch {
	case orden == "":
	case c.Medida != "" && orden != "valor" && orden != "grupo":
		return fmt.Errorf("orden %q imposible con medida", c.Orden)
	case c.Medida == "" && !esCampo:
		return fmt.Errorf("campo %q desconocido", orden)
	}
	return nil
}

// Opciones de una ejecución.
type Opciones struct {
	// Ahora resuelve las fechas relativas: un tablero de «este mes» es siempre
	// del mes en curso, no del día en que se guardó.
	Ahora time.Time
	// Tope es cuántas filas puede tener la tabla; más, y se marca Recortada.
	Tope int
	// RotuloCuenta es el nombre de la columna de una cuenta («Número»).
	RotuloCuenta string
}

// Ejecutar aplica una consulta ya comprobada a las filas de su fuente.
func (c Consulta) Ejecutar(f Fuente, filas []Fila, o Opciones) Tabla {
	type condicion struct {
		campo Campo
		op    string
		valor any
	}
	conds := make([]condicion, 0, len(c.Filtros))
	for _, fl := range c.Filtros {
		campo, _ := f.campo(fl.Campo)
		v, err := objetivo(campo, fl.Valor, o.Ahora)
		if err != nil {
			return Tabla{Error: ErrorInvalida}
		}
		conds = append(conds, condicion{campo, fl.Op, v})
	}
	pasan := make([]Fila, 0, len(filas))
	for _, fila := range filas {
		if !slices.ContainsFunc(conds, func(k condicion) bool { return !cumple(k.campo, fila[k.campo.Clave], k.op, k.valor) }) {
			pasan = append(pasan, fila)
		}
	}
	if c.Medida != "" {
		return c.agregar(f, pasan, o)
	}
	return c.listar(f, pasan, o)
}

// objetivo convierte el valor de un filtro al tipo de su campo.
func objetivo(c Campo, valor string, ahora time.Time) (any, error) {
	valor = strings.TrimSpace(valor)
	switch c.Tipo {
	case TipoNumero, TipoDinero:
		x, err := strconv.ParseFloat(strings.ReplaceAll(valor, ",", "."), 64)
		if err != nil {
			return nil, fmt.Errorf("%q no es un número", valor)
		}
		if c.Tipo == TipoDinero {
			return int64(math.Round(x * 100)), nil
		}
		return x, nil
	case TipoFecha:
		return fechaDe(valor, ahora)
	case TipoBooleano:
		b, err := strconv.ParseBool(valor)
		if err != nil {
			return nil, fmt.Errorf("%q no es verdadero o falso", valor)
		}
		return b, nil
	}
	return strings.ToLower(valor), nil
}

// fechaDe resuelve una fecha de filtro: AAAA-MM-DD o relativa a hoy (hoy, hoy-30d,
// hoy+7d, inicio_mes, inicio_trimestre, inicio_anio). Un inicio con «-N» es el de
// N periodos atrás: inicio_trimestre-1 abre el trimestre pasado.
func fechaDe(s string, ahora time.Time) (string, error) {
	s = strings.ToLower(s)
	a, m, _ := ahora.Date()
	dia := func(anio int, mes time.Month) string {
		// time.Date normaliza: el mes 0 es el diciembre del año anterior.
		return time.Date(anio, mes, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	}
	base, atras := s, 0
	if i := strings.LastIndexByte(s, '-'); strings.HasPrefix(s, "inicio_") && i > 0 {
		n, err := strconv.Atoi(s[i+1:])
		if err != nil || n < 0 {
			return "", fmt.Errorf("fecha %q ilegible", s)
		}
		base, atras = s[:i], n
	}
	switch {
	case s == "hoy":
		return ahora.Format(time.DateOnly), nil
	case base == "inicio_mes":
		return dia(a, m-time.Month(atras)), nil
	case base == "inicio_trimestre":
		return dia(a, (m-1)/3*3+1-time.Month(3*atras)), nil
	case base == "inicio_anio":
		return dia(a-atras, time.January), nil
	case len(s) > 5 && strings.HasPrefix(s, "hoy") && (s[3] == '-' || s[3] == '+') && strings.HasSuffix(s, "d"):
		n, err := strconv.Atoi(s[4 : len(s)-1])
		if err != nil {
			break
		}
		if s[3] == '-' {
			n = -n
		}
		return ahora.AddDate(0, 0, n).Format(time.DateOnly), nil
	}
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t.Format(time.DateOnly), nil
	}
	return "", fmt.Errorf("fecha %q ilegible", s)
}

// cumple compara el valor de una fila con el de un filtro. Un valor ausente o de
// otro tipo no cumple nada: un importe vacío no es «menor que 100».
func cumple(c Campo, v any, op string, obj any) bool {
	switch x := v.(type) {
	case float64:
		o, ok := obj.(float64)
		return ok && segun(op, compararNum(x, o))
	case int64:
		o, ok := obj.(int64)
		return ok && segun(op, compararNum(x, o))
	case bool:
		o, ok := obj.(bool)
		return ok && ((op == "=" && x == o) || (op == "!=" && x != o))
	case string:
		o, ok := obj.(string)
		return ok && cumpleTexto(c, x, op, o)
	}
	return false
}

func cumpleTexto(c Campo, x, op, o string) bool {
	s := strings.ToLower(x)
	if c.Tipo == TipoFecha && len(s) > 10 {
		s = s[:10]
	}
	// En una selección vale el valor guardado y también su rótulo.
	if r := strings.ToLower(c.Valores[x]); r != "" && (r == o || (op == "contiene" && strings.Contains(r, o))) {
		return op != "!="
	}
	switch op {
	case "contiene":
		return strings.Contains(s, o)
	case "empieza":
		return strings.HasPrefix(s, o)
	}
	return segun(op, strings.Compare(s, o))
}

// segun dice si una comparación (-1, 0, 1) cumple la operación.
func segun(op string, cmp int) bool {
	switch op {
	case "=":
		return cmp == 0
	case "!=":
		return cmp != 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	}
	return false
}

func compararNum[T int64 | float64](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

type grupo struct {
	clave  string
	n      int
	numero float64
	cents  int64
}

// agrupar reparte las filas por el campo de agrupación (y su unidad, si es una
// fecha) y acumula la medida de cada grupo, en el orden en que aparecen.
func (c Consulta) agrupar(filas []Fila) []*grupo {
	op, claveMedida, _ := strings.Cut(c.Medida, ":")
	claveGrupo, unidad, _ := strings.Cut(c.Agrupar, ":")
	corte := map[string]int{"dia": 10, "mes": 7, "anio": 4}[unidad]
	var grupos []*grupo
	porClave := map[string]*grupo{}
	for _, fila := range filas {
		k := ""
		if claveGrupo != "" {
			k = textoDe(fila[claveGrupo])
		}
		if corte > 0 && len(k) >= 10 {
			k = k[:corte]
		}
		g := porClave[k]
		if g == nil {
			g = &grupo{clave: k}
			porClave[k] = g
			grupos = append(grupos, g)
		}
		switch x := fila[claveMedida].(type) {
		case float64:
			g.numero += x
			g.n++
		case int64:
			g.cents += x
			g.n++
		default:
			if op == "cuenta" {
				g.n++
			}
		}
	}
	return grupos
}

func (c Consulta) agregar(f Fuente, filas []Fila, o Opciones) Tabla {
	op, claveMedida, _ := strings.Cut(c.Medida, ":")
	medida, _ := f.campo(claveMedida)
	claveGrupo, unidad, _ := strings.Cut(c.Agrupar, ":")
	agrupado, _ := f.campo(claveGrupo)

	valorCol := Campo{Clave: "valor", Tipo: TipoNumero, Etiqueta: o.RotuloCuenta}
	if op != "cuenta" {
		valorCol = Campo{Clave: "valor", Tipo: medida.Tipo, Etiqueta: medida.Etiqueta}
	}
	valor := func(g *grupo) float64 {
		switch {
		case op == "cuenta":
			return float64(g.n)
		case g.n == 0:
			return 0
		case op == "media" && medida.Tipo == TipoDinero:
			return math.Round(float64(g.cents) / float64(g.n))
		case op == "media":
			return g.numero / float64(g.n)
		case medida.Tipo == TipoDinero:
			return float64(g.cents)
		}
		return g.numero
	}

	grupos := c.agrupar(filas)
	if claveGrupo == "" {
		if len(grupos) == 0 {
			grupos = []*grupo{{}}
		}
		return Tabla{Columnas: []Campo{valorCol}, Filas: [][]any{{celda(valorCol, valor(grupos[0]))}}}
	}

	orden := c.Orden
	if orden == "" {
		orden = "-valor"
		if unidad != "" || agrupado.Tipo == TipoFecha {
			orden = "grupo"
		}
	}
	slices.SortStableFunc(grupos, func(a, b *grupo) int {
		r := strings.Compare(a.clave, b.clave)
		if strings.TrimPrefix(orden, "-") == "valor" {
			r = compararNum(valor(a), valor(b))
		}
		if strings.HasPrefix(orden, "-") {
			return -r
		}
		return r
	})

	colGrupo := agrupado
	if unidad != "" {
		colGrupo.Tipo = TipoTexto
	}
	t := Tabla{Columnas: []Campo{colGrupo, valorCol}}
	for _, g := range recorte(grupos, c.Limite, o.Tope, &t.Recortada) {
		t.Filas = append(t.Filas, []any{celda(colGrupo, g.clave), celda(valorCol, valor(g))})
	}
	return t
}

func (c Consulta) listar(f Fuente, filas []Fila, o Opciones) Tabla {
	cols := f.Campos
	if len(c.Columnas) > 0 {
		cols = make([]Campo, 0, len(c.Columnas))
		for _, clave := range c.Columnas {
			campo, _ := f.campo(clave)
			cols = append(cols, campo)
		}
	}
	if orden := strings.TrimPrefix(c.Orden, "-"); orden != "" {
		desc := strings.HasPrefix(c.Orden, "-")
		slices.SortStableFunc(filas, func(a, b Fila) int {
			r := compararCeldas(a[orden], b[orden])
			if desc {
				return -r
			}
			return r
		})
	}
	t := Tabla{Columnas: cols[:min(len(cols), MaxColumnas)]}
	for _, fila := range recorte(filas, c.Limite, o.Tope, &t.Recortada) {
		out := make([]any, len(t.Columnas))
		for i, col := range t.Columnas {
			out[i] = celda(col, fila[col.Clave])
		}
		t.Filas = append(t.Filas, out)
	}
	return t
}

// recorte aplica el límite pedido y el tope; solo el tope cuenta como recortar.
func recorte[T any](xs []T, limite, tope int, recortada *bool) []T {
	if limite > 0 && len(xs) > limite {
		xs = xs[:limite]
	}
	if tope > 0 && len(xs) > tope {
		*recortada = true
		xs = xs[:tope]
	}
	return xs
}

// compararCeldas ordena valores de una misma columna; los ausentes, al final.
func compararCeldas(a, b any) int {
	switch x := a.(type) {
	case nil:
		if b == nil {
			return 0
		}
		return 1
	case float64:
		if y, ok := b.(float64); ok {
			return compararNum(x, y)
		}
	case int64:
		if y, ok := b.(int64); ok {
			return compararNum(x, y)
		}
	}
	if b == nil {
		return -1
	}
	return strings.Compare(strings.ToLower(textoDe(a)), strings.ToLower(textoDe(b)))
}

// celda prepara un valor para la tabla: el importe en unidades exactas, la
// selección con su rótulo y la fecha sin hora.
func celda(c Campo, v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case int64:
		if c.Tipo == TipoDinero {
			return Unidades(x)
		}
		return float64(x)
	case float64:
		if c.Tipo == TipoDinero {
			return Unidades(int64(math.Round(x)))
		}
		return x
	case string:
		if r := c.Valores[x]; r != "" {
			return r
		}
		if c.Tipo == TipoFecha && len(x) > 10 {
			return x[:10]
		}
		return x
	}
	return v
}

// Unidades pasa céntimos a un número JSON exacto con dos decimales (123456 →
// 1234.56), sin float por el medio: sumar la columna tiene que cuadrar al céntimo.
func Unidades(c int64) json.Number {
	signo := ""
	if c < 0 {
		signo, c = "-", -c
	}
	return json.Number(fmt.Sprintf("%s%d.%02d", signo, c/100, c%100))
}

func textoDe(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return fmt.Sprint(v)
}

func recortar(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	return string(r[:min(len(r), n)])
}
