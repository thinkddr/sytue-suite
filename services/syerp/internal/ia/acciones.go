// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
)

// Acciones: una frase («apunta un gasto de Claude Code de 180 €») se convierte en
// los registros que hay que crear, con sus relaciones. El modelo PROPONE un plan;
// quien llama lo comprueba contra el catálogo y la base, lo enseña y lo crea en
// una transacción cuando la persona dice que sí.

// MaxPasos acota un plan: una petición que pide más de diez registros es una
// importación, y para eso está el CSV.
const MaxPasos = 10

// Plan es lo que propone Sytue AI.
type Plan struct {
	Resumen string   `json:"resumen"`
	Pasos   []PasoIA `json:"pasos"`
	Avisos  []string `json:"avisos,omitempty"`
}

// PasoIA es un registro por crear (o por encontrar, si ya existe).
type PasoIA struct {
	// Ref es el nombre del paso para que otro apunte a él: «$p1».
	Ref    string `json:"ref"`
	Modelo string `json:"modelo"`
	// Buscar es con qué nombre o NIF mirar si ya existe. Vacío = siempre se crea.
	Buscar string          `json:"buscar,omitempty"`
	Datos  json.RawMessage `json:"datos"`
}

// Organizacion es lo que el modelo necesita saber de quien pide: cambia el IVA
// que toca y qué documento es un gasto.
type Organizacion struct {
	Nombre, NIF, Pais, Forma string
	PersonaFisica            bool
	Intracomunitario         bool
}

// ErrSinPlan es una respuesta que no es un plan.
var ErrSinPlan = errors.New("ia: la respuesta no es un plan")

var refRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,15}$`)

// ContextoModelos describe los modelos en los que se puede escribir: claves,
// tipos, a qué apunta cada relación, qué es obligatorio y los valores de las
// selecciones. Ningún dato.
func ContextoModelos(vistas []catalogo.ModeloVista) string {
	var b strings.Builder
	b.WriteString("MODELOS (clave «nombre»: campo:tipo «nombre»; *obligatorio; →modelo al que apunta; [valor=rótulo|…])\n")
	for _, v := range vistas {
		b.WriteString(v.Clave + " «" + v.Etiqueta + "»:")
		for i, c := range v.Campos {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(" " + c.Clave + ":" + c.Tipo)
			if c.Requerido {
				b.WriteString("*")
			}
			if c.Modelo != "" {
				b.WriteString("→" + c.Modelo)
			}
			b.WriteString(" «" + c.Etiqueta + "»")
			if len(c.Opciones) > 0 {
				pares := make([]string, 0, len(c.Opciones))
				for _, o := range c.Opciones {
					pares = append(pares, o.Valor+"="+o.Etiqueta)
				}
				b.WriteString(" [" + strings.Join(pares, "|") + "]")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// InstruccionAcciones es la orden para convertir una petición en un plan.
//
// Las reglas de IVA son las de una gestoría española y van escritas para el
// modelo: es la diferencia entre un gasto bien apuntado y uno que descuadra el
// 303. Lo que el modelo no sabe con seguridad se deja vacío y se avisa.
func InstruccionAcciones(peticion string, org Organizacion, ahora time.Time) string {
	lineas := []string{
		"Convierte la petición en los registros que hay que crear en el ERP con los MODELOS del contexto y devuelve SOLO un objeto JSON así:",
		`{"resumen":"…","pasos":[{"ref":"p1","modelo":"clave","buscar":"…","datos":{"campo":"valor"}}],"avisos":["…"]}`,
		"Usa SOLO modelos y campos del contexto, con su clave exacta. Como mucho 10 pasos, en el orden en que se crean.",
		`En un campo que apunta a otro modelo (→), escribe el nombre del registro que ya existe, o "$ref" si es otro paso de este plan (p. ej. "proveedor":"$p1").`,
		`"buscar": en contactos, productos y empleados, el nombre o NIF con el que ver si ya existe; si existe no se crea. Omítelo en los documentos (facturas, gastos, pedidos).`,
		"Importes en unidades con punto decimal (180 o 180.50), fechas AAAA-MM-DD. Sin fecha, hoy. Rellena todos los campos obligatorios (*).",
		"Un gasto o compra con proveedor es una factura recibida (factura_compra) contra ese proveedor, creado antes como contacto de tipo organización si hace falta. Un gasto que adelantó un empleado es un gasto (gasto).",
		"Un importe sin más es lo pagado: el total con IVA incluido.",
		"IVA: proveedor en España, 21 % salvo que se diga otro (base = total / 1,21, iva = total − base, redondeado a céntimos). " +
			"Servicio de una empresa de otro país de la UE o de fuera (p. ej. software de EE. UU. o de Irlanda) a una empresa española: la factura no lleva IVA " +
			"(inversión del sujeto pasivo), así que base = total e iva = 0, y avisa de que ese IVA se declara en el 303" +
			func() string {
				if org.Intracomunitario {
					return " y, si es de la UE, en el 349."
				}
				return " y de que para no pagar el IVA extranjero hay que estar en el Registro de Operadores Intracomunitarios."
			}(),
		"De una empresa conocida puedes poner su razón social, NIF-IVA, país, web y correo de facturación SOLO si los sabes con seguridad; si no, déjalos vacíos. No inventes NIF ni correos.",
		`En "avisos", en frases cortas para alguien que no sabe de impuestos: lo que la persona tiene que comprobar o hacer (por ejemplo, subir la factura en PDF).`,
		`"resumen" y "avisos" en el idioma de la petición.`,
		"Quien pide: " + strings.TrimSpace(org.Nombre+" · NIF "+org.NIF+" · "+org.Pais+" · "+org.Forma),
		"Hoy es " + ahora.Format(time.DateOnly) + ".",
		"Petición: " + peticion,
	}
	return strings.Join(lineas, "\n")
}

// PlanDe lee la respuesta del modelo. Descarta los pasos sin modelo o con una
// referencia ilegible y recorta a MaxPasos; comprobar que los modelos y campos
// existen es de quien llama, que tiene el catálogo y los permisos.
func PlanDe(texto string) (Plan, error) {
	var p Plan
	if json.Unmarshal([]byte(asistente.JSON(texto)), &p) != nil {
		return Plan{}, ErrSinPlan
	}
	pasos := p.Pasos[:0]
	vistas := map[string]bool{}
	for _, paso := range p.Pasos {
		paso.Ref = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(paso.Ref)), "$")
		paso.Modelo = strings.TrimSpace(paso.Modelo)
		if paso.Modelo == "" || !refRe.MatchString(paso.Ref) || vistas[paso.Ref] || len(pasos) == MaxPasos {
			continue
		}
		vistas[paso.Ref] = true
		if len(paso.Datos) == 0 || string(paso.Datos) == "null" {
			paso.Datos = json.RawMessage(`{}`)
		}
		pasos = append(pasos, paso)
	}
	p.Pasos = pasos
	if len(p.Pasos) == 0 {
		return Plan{}, ErrSinPlan
	}
	p.Resumen = strings.TrimSpace(p.Resumen)
	return p, nil
}

// Referencia dice si el valor de un campo relación apunta a otro paso («$p1»).
func Referencia(valor string) (string, bool) {
	valor = strings.TrimSpace(valor)
	if !strings.HasPrefix(valor, "$") {
		return "", false
	}
	ref := strings.ToLower(valor[1:])
	return ref, refRe.MatchString(ref)
}
