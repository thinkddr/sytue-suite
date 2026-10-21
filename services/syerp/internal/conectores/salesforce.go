// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package conectores

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Mapeo declara qué objeto de Salesforce alimenta qué modelo de SyERP y cómo se
// corresponden sus campos. Es DATO, igual que el catálogo: mapear un objeto más
// es una entrada en esta lista, no un importador nuevo.
type Mapeo struct {
	// Nombre con el que se pide el mapeo; vacío = el objeto. Existe porque un
	// mismo objeto de origen puede alimentar dos cosas (empresas y personas de
	// `res.partner`).
	Nombre string
	// Objeto de origen (Account, Contact, Opportunity, res.partner).
	Objeto string
	// Filtro es el dominio que acota la consulta en el origen (Odoo). Nulo = todo.
	Filtro any
	// Etapa es la columna de origen que decide el estado; vacía = "StageName".
	Etapa string
	// Modelo de SyERP al que entra.
	Modelo string
	// Campos traduce columna de Salesforce → campo de SyERP.
	Campos map[string]string
	// Dinero son los campos que llegan en unidades de la divisa y se guardan en
	// céntimos.
	Dinero map[string]bool
	// Fijos son valores que el modelo exige y Salesforce no trae.
	Fijos map[string]any
	// Estados traduce el valor de Salesforce al estado del flujo de SyERP. Lo que
	// no esté en el mapa se queda con el estado inicial: inventar un estado que
	// el modelo no declara rompería sus propias reglas.
	Estados map[string]string
}

// MapeosSalesforce son los objetos que SyERP sabe importar.
//
// Los tres que importan de verdad al empezar: la empresa, la persona y el trato.
// Con eso, quien llega desde Salesforce ve sus datos el primer día; el resto
// (productos, pedidos) llega por su conector cuando alguien lo pida, no antes.
var MapeosSalesforce = []Mapeo{
	{
		Objeto: "Account",
		Modelo: "contacto",
		Campos: map[string]string{
			"Id": "externo_id", "Name": "nombre", "Phone": "telefono",
			"Website": "web", "BillingCity": "ciudad", "BillingCountry": "pais",
			"BillingStreet": "direccion", "BillingPostalCode": "cp",
		},
		Fijos: map[string]any{"tipo": "organizacion"},
	},
	{
		Objeto: "Contact",
		Modelo: "contacto",
		Campos: map[string]string{
			"Id": "externo_id", "Name": "nombre", "Email": "correo",
			"Phone": "telefono", "MailingCity": "ciudad", "MailingCountry": "pais",
		},
		Fijos: map[string]any{"tipo": "persona"},
	},
	{
		Objeto: "Opportunity",
		Modelo: "oportunidad",
		Campos: map[string]string{
			"Id": "externo_id", "Name": "nombre", "Amount": "importe",
			"Probability": "probabilidad", "CloseDate": "vencimiento",
		},
		Dinero: map[string]bool{"importe": true},
		Estados: map[string]string{
			"Closed Won":          "ganada",
			"Closed Lost":         "perdida",
			"Qualification":       "calificada",
			"Proposal":            "propuesta",
			"Value Proposal":      "propuesta",
			"Negotiation":         "propuesta",
			"Prospecting":         "nuevo",
			"Needs Analysis":      "calificada",
			"Id. Decision Makers": "calificada",
		},
	},
}

func (m Mapeo) columnaEtapa() string {
	if m.Etapa != "" {
		return m.Etapa
	}
	return "StageName"
}

// Clave es el nombre con el que se pide el mapeo.
func (m Mapeo) Clave() string {
	if m.Nombre != "" {
		return m.Nombre
	}
	return m.Objeto
}

// MapeoDe busca un mapeo por su clave en la lista dada.
func MapeoDe(mapeos []Mapeo, objeto string) (Mapeo, bool) {
	for _, m := range mapeos {
		if strings.EqualFold(m.Clave(), objeto) {
			return m, true
		}
	}
	return Mapeo{}, false
}

// Columnas devuelve las columnas de Salesforce que hay que pedir, más la del
// estado si el mapeo lo traduce.
func (m Mapeo) Columnas() []string {
	out := make([]string, 0, len(m.Campos)+1)
	for col := range m.Campos {
		out = append(out, col)
	}
	if len(m.Estados) > 0 {
		out = append(out, m.columnaEtapa())
	}
	// Orden estable: la spec que se le manda al tercero no debe cambiar entre
	// ejecuciones solo porque Go recorra el mapa en otro orden.
	slices.Sort(out)
	return out
}

// Fila es un registro traducido, listo para el store.
type Fila struct {
	ExternoID string
	Data      map[string]any
	Estado    string
}

// Traducir convierte la tabla que devuelve la pasarela en filas de SyERP.
//
// Una fila que no se puede traducir NO detiene la importación: se cuenta y se
// sigue. Importar cinco mil contactos y abortar en el número 4.999 por un
// teléfono raro deja el trabajo a medias y sin forma de retomarlo.
func (m Mapeo) Traducir(t Tabla) ([]Fila, []error) {
	indice := map[string]int{}
	for i, c := range t.Columns {
		indice[c.Name] = i
	}
	var filas []Fila
	var fallos []error

	for n, cruda := range t.Rows {
		data := map[string]any{}
		for clave, valor := range m.Fijos {
			data[clave] = valor
		}
		fallo := false
		for col, campo := range m.Campos {
			i, ok := indice[col]
			if !ok || i >= len(cruda) {
				continue
			}
			valor, hay, err := celda(cruda[i], m.Dinero[campo])
			if err != nil {
				fallos = append(fallos, fmt.Errorf("fila %d, %s: %w", n+1, col, err))
				fallo = true
				break
			}
			if hay {
				data[campo] = valor
			}
		}
		if fallo {
			continue
		}
		externo, _ := data["externo_id"].(string)
		// Odoo numera con enteros; el id externo se guarda como texto siempre.
		if n, ok := data["externo_id"].(int64); ok {
			externo = strconv.FormatInt(n, 10)
			data["externo_id"] = externo
		}
		if externo == "" {
			fallos = append(fallos, fmt.Errorf("fila %d: sin identificador de origen", n+1))
			continue
		}
		f := Fila{ExternoID: externo, Data: data}
		if i, ok := indice[m.columnaEtapa()]; ok && i < len(cruda) {
			var etapa string
			_ = json.Unmarshal(cruda[i], &etapa)
			f.Estado = m.Estados[etapa]
		}
		filas = append(filas, f)
	}
	return filas, fallos
}

// celda traduce un valor crudo de la tabla. `hay` distingue «la columna vino
// vacía» de «vino con algo»: un nulo de Salesforce no puede convertirse en un
// cero, porque un importe desconocido y un importe de cero euros no son lo mismo.
//
// Los importes pasan por la conversión EXACTA a céntimos, sin float por el medio.
func celda(crudo json.RawMessage, esDinero bool) (valor any, hay bool, err error) {
	s := strings.TrimSpace(string(crudo))
	if s == "" || s == "null" {
		return nil, false, nil
	}
	if esDinero {
		var n json.Number
		if err := json.Unmarshal(crudo, &n); err != nil {
			var texto string
			if err := json.Unmarshal(crudo, &texto); err != nil {
				return nil, false, fmt.Errorf("importe ilegible")
			}
			n = json.Number(texto)
		}
		centimos, err := CentimosDeDecimal(n.String())
		if err != nil {
			return nil, false, err
		}
		return centimos, true, nil
	}
	var texto string
	if err := json.Unmarshal(crudo, &texto); err == nil {
		return texto, true, nil
	}
	var num json.Number
	if err := json.Unmarshal(crudo, &num); err == nil {
		// Los enteros (Probability) se guardan como número; los decimales que no
		// son dinero, como cadena, que es la forma canónica de este ERP.
		if i, err := num.Int64(); err == nil {
			return i, true, nil
		}
		return num.String(), true, nil
	}
	var b bool
	if err := json.Unmarshal(crudo, &b); err == nil {
		// ponytail: `false` cuenta como vacío. Es la convención de Odoo para un
		// campo sin valor, y ningún mapeo trae booleanos de verdad.
		if !b {
			return nil, false, nil
		}
		return b, true, nil
	}
	return nil, false, fmt.Errorf("valor no admitido")
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
