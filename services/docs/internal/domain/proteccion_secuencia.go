// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// Secciones protegidas: la secuencia RGA por carácter (ROADMAP-OFFICE permisos-1).
//
// El editor de Docus guarda el contenido en el registro LWW `doc:html` y, ADEMÁS, lo
// refleja en una secuencia RGA con un elemento por carácter del HTML (coedicion.ts): el
// texto de la secuencia ES el HTML. Los co-editores conectados pintan lo que dice la
// secuencia, así que quien teclea directamente insert/delete —sin pasar por `doc:html`—
// podía enseñar a los demás una sección ajena cambiada, y el siguiente guardado de
// alguien con permiso en ella lo convertía en contenido real.
//
// CÓMO SE CIERRA. En los documentos con secciones (y solo en ellos) el almacén guarda
// una copia de la secuencia junto al estado de protección. Cada lote con insert/delete
// se aplica a una copia, se obtiene el texto resultante y se compara con las secciones
// con la MISMA regla que el registro `doc:html` (seccionesPermitidas): no se puede crear,
// quitar ni alterar el marcado de una sección, y solo cambia el contenido de una sección
// quien figura en ella. Así cae por igual teclear dentro de una sección ajena, partirla,
// borrarla o saltarla con un `set` de `doc:html` limpio acompañado de ops sucias.
//
// INVARIANTE. La copia es NULL (desconocida) o es exactamente la secuencia del documento
// en su seq actual. La mantienen todas las escrituras con ops por carácter (el
// propietario también: no se le valida, pero la copia no puede quedar atrás), se descarta
// cuando el documento deja de tener secciones, y si falta cuando un no propietario
// escribe por carácter el servicio la reconstruye (ErrSecuenciaPendiente) y reintenta.
//
// ponytail: coste lineal en el tamaño del documento por cada lote con ops por carácter,
// solo en los documentos con secciones (copia, índice por Dot, orden RGA y dos parseos de
// HTML). Si un documento protegido enorme lo notase, la mejora es guardar la copia por
// fragmentos y validar solo el tramo tocado.

// ErrSecuenciaPendiente: el documento tiene secciones y aún no tiene su copia de la
// secuencia; el servicio la reconstruye desde el estado completo y reintenta.
var ErrSecuenciaPendiente = errors.New("docs: secuencia de texto sin inicializar")

// TocaSecuencia indica si el lote trae operaciones por carácter.
func TocaSecuencia(ops []Op) bool {
	for i := range ops {
		if ops[i].Kind == OpInsert || ops[i].Kind == OpDelete {
			return true
		}
	}
	return false
}

// CopiaSecuencia es la copia de la secuencia de un estado completo (sin registros).
func CopiaSecuencia(st *State) *State {
	return &State{Seq: st.Seq, Elements: slices.Clone(st.Elements)}
}

// SecuenciaJSON serializa una copia para guardarla.
func SecuenciaJSON(s *State) ([]byte, error) {
	b, err := json.Marshal(s.Elements)
	if err != nil {
		return nil, fmt.Errorf("docs: serializar secuencia: %w", err)
	}
	return b, nil
}

// SecuenciaDeJSON es la inversa de SecuenciaJSON.
func SecuenciaDeJSON(raw []byte) (*State, error) {
	s := NewState()
	if err := json.Unmarshal(raw, &s.Elements); err != nil {
		return nil, fmt.Errorf("docs: secuencia guardada inválida: %w", err)
	}
	return s, nil
}

// AplicarSecuencia integra las operaciones por carácter en `s` con la semántica de
// State.Apply (idempotente; un delete de un elemento aún no visto deja su tombstone),
// pero con un índice por Dot: Apply busca linealmente y un lote grande sobre un
// documento grande sería cuadrático. Ignora los `set`. Un test la compara con Apply.
func AplicarSecuencia(s *State, ops []Op) {
	idx := make(map[Dot]int, len(s.Elements)+len(ops))
	for i, e := range s.Elements {
		if _, dup := idx[e.Dot]; !dup {
			idx[e.Dot] = i
		}
	}
	for _, op := range ops {
		switch op.Kind {
		case OpInsert:
			if _, ya := idx[op.Dot]; ya {
				continue
			}
			after := Dot{}
			if op.After != nil {
				after = *op.After
			}
			idx[op.Dot] = len(s.Elements)
			s.Elements = append(s.Elements, element{Dot: op.Dot, After: after, Value: op.Value, Lamport: op.Lamport})
		case OpDelete:
			if i, ya := idx[*op.Target]; ya {
				s.Elements[i].Deleted = true
				continue
			}
			idx[*op.Target] = len(s.Elements)
			s.Elements = append(s.Elements, element{Dot: *op.Target, Deleted: true})
		case OpSet, OpPatch:
			// los registros no forman parte de la secuencia
		}
	}
}

// secuenciaDelLote valida las operaciones por carácter del lote y deja en res la copia
// resultante. `est` es el estado ANTERIOR al lote; res.Estado.Modelo ya refleja el
// `doc:html` del propio lote.
func secuenciaDelLote(res *ResultadoProteccion, est EstadoProteccion, ops []Op, esDueno bool, quien Subject) error {
	if res.Estado.Modelo == "" {
		// Sin secciones tras el lote la copia no hace falta (y se reconstruirá si vuelven).
		if est.Secuencia != nil {
			res.Estado.Secuencia, res.SecuenciaCambia = nil, true
		}
		return nil
	}
	if !TocaSecuencia(ops) {
		return nil
	}
	if est.Secuencia == nil {
		if esDueno {
			return nil // no se le valida; la copia sigue por establecer
		}
		return ErrSecuenciaPendiente
	}
	copia := CopiaSecuencia(est.Secuencia)
	antes := copia.PlainText()
	AplicarSecuencia(copia, ops)
	if !esDueno {
		// La referencia es el marcado de la propia secuencia; si no lo tiene (secuencia sin
		// sembrar o ilegible) lo son las secciones del último `doc:html`.
		base, ok := seccionesDe(antes)
		if !ok || len(base) == 0 {
			base = leerHuellas(est.Modelo)
		}
		nuevas, validas := seccionesDe(copia.PlainText())
		if !seccionesPermitidas(base, nuevas, validas, quien) {
			return ErrProtegido
		}
	}
	res.Estado.Secuencia, res.SecuenciaCambia = copia, true
	return nil
}
