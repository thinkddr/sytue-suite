// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/config"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// filasPorTramo es lo que se le pide al store en cada vuelta del cursor. Es el
// máximo que admite `Listar`: cuantas menos vueltas, menos consultas para la
// misma descarga, y en memoria solo vive el tramo que se está escribiendo.
const filasPorTramo = 200

// maxIDsExportar es cuántas filas marcadas se pueden pedir por su identificador: las
// de varias páginas de la lista. Más no caben en una URL que atraviese un proxy.
const maxIDsExportar = 500

var errIDsExportar = httpx.NewError(http.StatusBadRequest, "ids_invalidos",
	"The ids must be record ids separated by commas, at most 500.")

// idsDe lee `?ids=` (identificadores separados por comas): las filas marcadas de la
// lista. Cada uno tiene que ser un UUID, que es lo que la consulta compara.
func idsDe(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	crudo := r.URL.Query().Get("ids")
	if crudo == "" {
		return nil, true
	}
	ids := strings.Split(crudo, ",")
	if len(ids) > maxIDsExportar {
		httpx.Fail(w, r, errIDsExportar)
		return nil, false
	}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			httpx.Fail(w, r, errIDsExportar)
			return nil, false
		}
	}
	return ids, true
}

// Exportar devuelve la lista del modelo en CSV: las mismas columnas que la vista
// de lista, los mismos filtros (`q`, `estado`, `campo`/`valor`) y los rótulos en
// el idioma de la petición. Con `ids`, solo esas filas: las marcadas en la lista.
//
// SE ESCRIBE CONFORME SE LEE. El cursor va trayendo tramos de 200 filas que se
// vuelcan al ResponseWriter y se olvidan, así que lo que gasta el pod no depende
// del tamaño de la organización. Lo que sí crece con ella es el TIEMPO, y por
// eso hay un tope duro de filas (`EXPORT_MAX_FILAS`): el router le da 30 s a
// cada petición, y una descarga que se pasa de ahí no acaba en un fichero
// grande, acaba en un fichero cortado por la mitad.
//
// La contrapartida de escribir a chorro es que el 200 sale con la primera fila:
// a partir de ese byte un fallo ya no se puede convertir en un error HTTP, solo
// en un fichero corto y una línea de log. Por eso el primer tramo se pide ANTES
// de tocar la respuesta, que es cuando fallan casi todos (base caída, filtro
// imposible, cursor inválido).
func (h *Handlers) Exportar(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	f, ok := filtroDe(w, r, m)
	if !ok {
		return
	}
	if f.IDs, ok = idsDe(w, r); !ok {
		return
	}
	f.Limite = filasPorTramo

	idioma := httpx.LanguageFromRequest(r)
	vista := h.cat.VistaModelo(m, idioma)
	cols := h.columnas(vista, idioma)
	valores := rotulosDeValor(vista)
	tope := h.cfg.ExportMaxFilas
	if tope <= 0 {
		tope = config.ExportMaxFilasDefecto
	}

	pag, err := h.store.Listar(r.Context(), f)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	escritas := 0
	registro := log.From(r.Context())
	cortado := func(err error) {
		registro.Warn("exportación CSV cortada a media escritura",
			"modelo", m.Clave, "filas", escritas, "error", err)
	}

	// Sin Content-Length: el fichero se va componiendo y no se sabe cuánto va a
	// medir. El nombre lleva modelo y día para que diez descargas no se pisen en
	// la carpeta de quien exporta; `m.Clave` es [a-z0-9_] por el catálogo, así
	// que no puede romper las comillas de la cabecera.
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+m.Clave+"-"+time.Now().UTC().Format(time.DateOnly)+`.csv"`)

	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write(rotulos(cols)); err != nil {
		cortado(err)
		return
	}

	for {
		// Los nombres legibles de las relaciones del tramo, en UNA consulta: sin
		// esto, exportar mil pedidos serían mil consultas para poner el nombre
		// del cliente.
		etiquetas, err := h.store.ResolverEtiquetas(r.Context(), idsReferenciados(m, pag.Registros))
		if err != nil {
			cortado(err)
			return
		}
		for i := range pag.Registros {
			if escritas == tope {
				// Se corta, pero NO en silencio: quien abra el fichero tiene que
				// ver que le faltan filas ahí mismo, no descubrirlo al cuadrar
				// los totales. Como el 200 ya salió, el aviso solo puede ir
				// dentro del propio CSV.
				if err := cw.Write([]string{h.cat.Rotulo("aviso.truncado", idioma) + " " + strconv.Itoa(tope)}); err != nil {
					cortado(err)
				}
				return
			}
			if err := cw.Write(fila(cols, pag.Registros[i], etiquetas, valores)); err != nil {
				cortado(err)
				return
			}
			escritas++
		}
		if pag.Siguiente == "" {
			return
		}
		f.Cursor = pag.Siguiente
		// Se suelta lo escrito antes de ir a por el siguiente tramo: así el
		// navegador va recibiendo fichero mientras la base trabaja.
		cw.Flush()
		siguiente, err := h.store.Listar(r.Context(), f)
		if err != nil {
			cortado(err)
			return
		}
		pag = siguiente
	}
}

// columna es una columna de la exportación con todo lo suyo ya resuelto: se
// calcula una vez por descarga y no una vez por fila.
type columna struct {
	clave  string
	tipo   string
	rotulo string
}

// columnas traduce las columnas de la vista de lista al idioma de la petición.
//
// Los rótulos salen del MISMO sitio que los de la pantalla (`VistaModelo`), así
// que el CSV y la interfaz llaman a las cosas igual. "ref" y "estado" no son
// campos del modelo —son columnas que el catálogo admite aparte—, así que su
// rótulo viene del diccionario común.
func (h *Handlers) columnas(v catalogo.ModeloVista, idioma string) []columna {
	campos := make(map[string]catalogo.CampoVista, len(v.Campos))
	for _, c := range v.Campos {
		campos[c.Clave] = c
	}
	out := make([]columna, 0, len(v.Lista))
	for _, clave := range v.Lista {
		col := columna{clave: clave, rotulo: h.cat.Rotulo("campo."+clave, idioma)}
		if f, ok := campos[clave]; ok {
			col.tipo, col.rotulo = f.Tipo, f.Etiqueta
		}
		out = append(out, col)
	}
	return out
}

func rotulos(cols []columna) []string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		out = append(out, c.rotulo)
	}
	return out
}

// rotulosDeValor recoge los rótulos ya traducidos de las selecciones y de los
// estados, para que el CSV diga "Ganada" y no "ganada". Un mapa plano basta
// porque el diccionario del catálogo indexa los valores por sí mismos
// (`valor.<clave>`) y no por el campo que los usa.
func rotulosDeValor(v catalogo.ModeloVista) map[string]string {
	out := map[string]string{}
	for _, c := range v.Campos {
		for _, op := range c.Opciones {
			out[op.Valor] = op.Etiqueta
		}
	}
	for _, e := range v.Estados {
		out[e.Valor] = e.Etiqueta
	}
	return out
}

// fila convierte un registro en su línea de CSV.
//
// Toda celda pasa por `validation.CSVSafe` salvo las que son un número y nada
// más: lo que va aquí lo escribió alguien en el ERP, y un nombre de cliente que
// empieza por "=" es una fórmula en cuanto se abre el fichero con Excel o
// LibreOffice (CWE-1236). Los rótulos no lo necesitan porque salen del catálogo,
// que va horneado en la imagen.
func fila(cols []columna, reg store.Registro, etiquetas, valores map[string]string) []string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		v := c.valor(reg, etiquetas, valores)
		if !numerico(v) {
			v = validation.CSVSafe(v)
		}
		out = append(out, v)
	}
	return out
}

// numerico exime a los números del saneado, y es la única excepción.
//
// POR QUÉ HACE FALTA: `CSVSafe` protege el "-" inicial, que es un arranque de
// fórmula legítimo ("-1+cmd|..."), así que un importe negativo saldría como
// "'-0.05" y la hoja de cálculo lo trataría como TEXTO. Media columna en texto y
// media en número es una columna que no se puede sumar, y sumar la columna es
// justo para lo que se exporta la lista de un ERP.
//
// POR QUÉ ES SEGURO: se mira el RESULTADO y no el tipo declarado del campo, así
// que no depende de que lo que hay en `data` case con el catálogo. Y una cadena
// que `ParseFloat` acepta entera no puede ser una fórmula: no hay fórmula que
// sea a la vez un literal numérico válido —en cuanto aparece un operador, una
// referencia o un paréntesis, esto devuelve error y la celda se neutraliza—.
func numerico(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// valor saca de un registro lo que va en esta columna, ya legible: el importe en
// unidades, la relación por su nombre y el estado o la selección con su rótulo.
func (c columna) valor(reg store.Registro, etiquetas, valores map[string]string) string {
	switch c.clave {
	case "ref":
		return reg.Ref
	case "estado":
		return legible(valores, reg.Estado)
	}
	v := reg.Data[c.clave]
	switch c.tipo {
	case catalogo.TipoDinero:
		return importe(v)
	case catalogo.TipoRelacion:
		id, _ := v.(string)
		return legible(etiquetas, id)
	case catalogo.TipoSeleccion:
		s, _ := v.(string)
		return legible(valores, s)
	}
	return texto(v)
}

// legible devuelve el nombre traducido cuando se conoce y el valor crudo cuando
// no: una relación a una fila borrada o un estado sin rótulo salen como están,
// que se lee mal pero se lee, y es mejor que dejar la celda vacía.
func legible(nombres map[string]string, valor string) string {
	if s := nombres[valor]; s != "" {
		return s
	}
	return valor
}

// texto pinta un valor de `data` tal como vino del jsonb. Los números llegan
// como float64 —el store deserializa sin `UseNumber`—, así que se formatean sin
// exponente para que un 20000 no acabe en la celda como 2e+04.
func texto(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return fmt.Sprint(x)
	}
}

// importe pasa los céntimos que guarda el ERP a lo que espera quien abre el
// fichero: UNIDADES con dos decimales y PUNTO decimal (123456 → "1234.56").
//
// Punto y no coma porque el separador del CSV es la coma: con coma decimal,
// "1234,56" parte la celda en dos en cuanto alguien reabre el fichero. Y porque
// es la misma forma canónica que lee `conectores.CentimosDeDecimal` al importar,
// así que lo que sale de aquí puede volver a entrar sin perder un céntimo.
//
// La conversión es ENTERA, sin float por el medio, igual que en el resto del
// servicio: dividir por 100.0 devuelve 1234.5599999999999 para importes
// perfectamente normales, y ese céntimo aparece meses después sin saber de dónde
// sale.
func importe(v any) string {
	c, ok := centimos(v)
	if !ok {
		return ""
	}
	// Se divide con signo y se corrige solo la parte decimal: negar `c` fallaría
	// justo en el mínimo de int64, y -5 céntimos tienen que salir "-0.05" y no
	// "0.-5".
	unidades, resto := c/100, c%100
	if resto < 0 {
		resto = -resto
	}
	signo := ""
	if c < 0 && unidades == 0 {
		signo = "-"
	}
	return fmt.Sprintf("%s%d.%02d", signo, unidades, resto)
}

// centimos lee un importe de `data`. Llega como float64 porque el store
// deserializa el jsonb sin `UseNumber`, y se acepta solo si es entero exacto:
// `domain` no deja guardar decimales en un campo de dinero, así que un valor con
// parte fraccionaria es un dato corrupto y sale la celda vacía en vez de un
// redondeo inventado.
func centimos(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case float64:
		if x != float64(int64(x)) {
			return 0, false
		}
		return int64(x), true
	default:
		return 0, false
	}
}
