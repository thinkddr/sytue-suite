// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package obligaciones

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// El proceso de una obligación: cuatro pasos que se recorren en orden (revisar lo
// pendiente, preparar las cifras, presentar en el portal y guardar el justificante).
// Las reglas son las mismas en el servidor y en el ERP de Sytue Office; cada almacén
// guarda la fase donde puede.

// Proceso es el avance guardado de una obligación en un periodo.
type Proceso struct {
	Fase       int    `json:"fase"`
	Referencia string `json:"referencia"`
}

// FasesProceso es el número de pasos del proceso.
const FasesProceso = 4

// ErrPaso es un avance que no se admite (fuera de rango, saltándose un paso, sin
// referencia o sin documento). Los errores de Avanzar lo envuelven y su texto es
// solo el porqué («completa el paso anterior»).
var ErrPaso = errors.New("paso del proceso no admitido")

type errPaso string

func (e errPaso) Error() string          { return string(e) }
func (e errPaso) Is(objetivo error) bool { return objetivo == ErrPaso }

// DocumentosProceso son las claves de los documentos que pide la obligación para
// terminar; sin ninguno declarado, el justificante.
func (i Instancia) DocumentosProceso() []string {
	docs := make([]string, 0, len(i.Documentos))
	for _, d := range i.Documentos {
		docs = append(docs, d.Clave)
	}
	if len(docs) == 0 {
		docs = []string{"justificante"}
	}
	return docs
}

// RegistroDocumento es el `registro` con que se adjunta un documento del proceso
// (modelo `obligacion`).
func RegistroDocumento(clave, periodo, doc string) string { return clave + ":" + periodo + ":" + doc }

// ComprobarFase valida un avance antes de abrir la escritura.
func ComprobarFase(fase int, referencia string) error {
	if fase < 1 || fase > FasesProceso || utf8.RuneCountInString(referencia) > 200 {
		return ErrPaso
	}
	return nil
}

// Avanzar aplica un avance sobre el actual. Volver a un paso ya hecho no cambia nada
// (cambia es false); saltarse uno es un error. El último pide la referencia del
// justificante y que cada documento esté guardado (guardado lo comprueba el almacén).
func Avanzar(actual Proceso, fase int, referencia string, documentos []string, guardado func(doc string) (bool, error)) (Proceso, bool, error) {
	if err := ComprobarFase(fase, referencia); err != nil {
		return actual, false, err
	}
	if fase <= actual.Fase {
		return actual, false, nil
	}
	if fase != actual.Fase+1 {
		return actual, false, errPaso("completa el paso anterior")
	}
	if fase == FasesProceso {
		if strings.TrimSpace(referencia) == "" {
			return actual, false, errPaso("indica la referencia del justificante")
		}
		for _, doc := range documentos {
			hay, err := guardado(doc)
			if err != nil {
				return actual, false, err
			}
			if !hay {
				return actual, false, errPaso(fmt.Sprintf("guarda el documento %s antes de terminar", doc))
			}
		}
	}
	return Proceso{Fase: fase, Referencia: strings.TrimSpace(referencia)}, true, nil
}

// PendientesDe suma, por cada comprobación que pide la instancia (banco, facturas,
// nóminas), lo pendiente de los meses de su periodo. pendientes va por comprobación y
// mes (AAAA-MM). nil si la instancia no pide nada o no hay recuento.
func PendientesDe(i Instancia, pendientes map[string]map[string]int) map[string]int {
	desde, hasta, ok := RangoDePeriodo(i.Periodo)
	if !ok || pendientes == nil {
		return nil
	}
	var out map[string]int
	for _, prep := range i.Preparacion {
		n := 0
		for mes, cuantos := range pendientes[prep] {
			if mes >= desde.Format("2006-01") && mes <= hasta.Format("2006-01") {
				n += cuantos
			}
		}
		if out == nil {
			out = map[string]int{}
		}
		out[prep] = n
	}
	return out
}
