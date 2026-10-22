// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "math"

// Lógicas (functions/logical.ts) y de información (functions/information.ts).
//
// EVALUACIÓN PEREZOSA: IF, IFS y SWITCH evalúan la condición y SOLO la rama elegida;
// IFERROR e IFNA, la reserva solo si el primero falla. AND, OR y XOR son EAGER como en
// una hoja real: evalúan todo y propagan el primer #error (=AND(FALSO;1/0) → #DIV/0!).

// gatherBooleans recoge los booleanos de AND/OR/XOR. Un escalar directo se coacciona;
// dentro de una referencia se ignoran texto y blancos y los números cuentan como (≠0).
func gatherBooleans(args []*Arg) ([]bool, Error) {
	var out []bool
	for _, a := range args {
		if a.Ref() == nil {
			b, err := CoerceBoolean(a.Value())
			if err != "" {
				return nil, err
			}
			out = append(out, b)
			continue
		}
		for _, fila := range a.Matrix() {
			for _, v := range fila {
				switch x := v.(type) {
				case Error:
					return nil, x
				case bool:
					out = append(out, x)
				case float64:
					out = append(out, x != 0)
				}
			}
		}
	}
	return out, ""
}

// logica es AND, OR y XOR: sin ningún valor lógico → #VALUE!.
func logica(reduce func(ciertos, total int) bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		bs, err := gatherBooleans(args)
		if err != "" {
			return err
		}
		if len(bs) == 0 {
			return ErrValue
		}
		ciertos := 0
		for _, b := range bs {
			if b {
				ciertos++
			}
		}
		return reduce(ciertos, len(bs))
	}
}

// is1 envuelve un predicado de INSPECCIÓN: las IS* no propagan #errores, los examinan.
func is1(pred func(v Value) bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		return pred(args[0].Value())
	}
}

func esTexto(v Value) bool {
	s, ok := v.(string)
	return ok && s != ""
}

// errorType es ERROR.TYPE: código → número de clasificación (Excel / ECMA-376).
// ponytail: #CIRC! ≈ la #REF! circular de Excel.
var errorType = map[Error]float64{
	ErrNull: 1, ErrDiv0: 2, ErrValue: 3, ErrRef: 4, ErrName: 5, ErrNum: 6, ErrNA: 7, ErrSpill: 9, ErrCalc: 14, ErrCirc: 4,
}

// paridad es ISEVEN / ISODD: SÍ coaccionan y propagan; truncan antes de mirar.
func paridad(par bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		n, err := CoerceNumber(args[0].Value())
		if err != "" {
			return err
		}
		return (math.Mod(math.Trunc(n), 2) == 0) == par
	}
}

// funcionesLogicas es el registro de funciones de este fichero. Es un MAPA y no un init():
// el cuerpo de cada función vive dentro del literal, y medido como UNA función
// lo que sale es la suma de todos ellos, que no es una medida de nada.
var funcionesLogicas = map[string]fn{
	// Si falta el «si no», una condición falsa devuelve FALSE.
	"IF": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		cond, err := CoerceBoolean(args[0].Value())
		switch {
		case err != "":
			return err
		case cond:
			return args[1].Value()
		case len(args) == 3:
			return args[2].Value()
		}
		return false
	},
	// La primera condición verdadera gana; #N/A si ninguna.
	"IFS": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 {
			return ErrValue
		}
		for i := 0; i+1 < len(args); i += 2 {
			cond, err := CoerceBoolean(args[i].Value())
			if err != "" {
				return err
			}
			if cond {
				return args[i+1].Value()
			}
		}
		return ErrNA
	},
	"AND": logica(func(ciertos, total int) bool { return ciertos == total }),
	"OR":  logica(func(ciertos, _ int) bool { return ciertos > 0 }),
	"XOR": logica(func(ciertos, _ int) bool { return ciertos%2 == 1 }),
	"NOT": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		b, err := CoerceBoolean(args[0].Value())
		if err != "" {
			return err
		}
		return !b
	},
	"TRUE":  func(*FnCtx, []*Arg) any { return true },
	"FALSE": func(*FnCtx, []*Arg) any { return false },
	"IFERROR": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		if v := args[0].Value(); !IsError(v) {
			return v
		}
		return args[1].Value()
	},
	// Como IFERROR pero SOLO captura #N/A.
	"IFNA": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		if v := args[0].Value(); v != ErrNA {
			return v
		}
		return args[1].Value()
	},
	// SWITCH(expr, valor1, resultado1, …, [predeterminado]): igualdad de hoja.
	"SWITCH": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 {
			return ErrValue
		}
		sujeto := args[0].Value()
		if IsError(sujeto) {
			return sujeto
		}
		for i := 1; i+1 < len(args); i += 2 {
			cmp, err := CompareValues(sujeto, args[i].Value())
			if err != "" {
				return err
			}
			if cmp == 0 {
				return args[i+1].Value()
			}
		}
		if (len(args)-1)%2 == 1 {
			return args[len(args)-1].Value()
		}
		return ErrNA
	},

	// Una celda en blanco vale "" y no se distingue de la cadena vacía: por eso
	// ISBLANK("") es TRUE, ISTEXT("") FALSE y TYPE("") 1, como Excel con una vacía.
	"ISBLANK":   is1(func(v Value) bool { return v == "" }),
	"ISNUMBER":  is1(func(v Value) bool { _, ok := v.(float64); return ok }),
	"ISTEXT":    is1(esTexto),
	"ISNONTEXT": is1(func(v Value) bool { return !esTexto(v) }),
	"ISLOGICAL": is1(func(v Value) bool { _, ok := v.(bool); return ok }),
	"ISERROR":   is1(IsError),
	"ISERR":     is1(func(v Value) bool { return IsError(v) && v != ErrNA }),
	"ISNA":      is1(func(v Value) bool { return v == ErrNA }),
	// ISREF mira la FORMA del argumento sin evaluarlo: ISREF(1/0) es FALSE.
	// ponytail: ISREF(INDIRECT("A1")) da FALSE (Excel: TRUE); una llamada no expone ref.
	"ISREF": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		return args[0].Ref() != nil
	},
	"ISFORMULA": func(c *FnCtx, args []*Arg) any {
		if len(args) != 1 || args[0].Ref() == nil {
			return ErrValue
		}
		return c.IsFormula(*args[0].Ref())
	},
	"ISEVEN": paridad(true),
	"ISODD":  paridad(false),
	"NA":     func(*FnCtx, []*Arg) any { return ErrNA },
	// N: CUALQUIER texto → 0 (a diferencia de la coacción normal, no parsea «7»).
	"N": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		switch x := args[0].Value().(type) {
		case Error:
			return x
		case float64:
			return x
		case bool:
			return b2f(x)
		}
		return 0.0
	},
	// TYPE: 1 número, 2 texto, 4 lógico, 16 error, 64 matriz (rango de varias celdas).
	"TYPE": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		if r := args[0].Ref(); r != nil && (r.C2 > r.C1 || r.R2 > r.R1) {
			return 64.0
		}
		switch x := args[0].Value().(type) {
		case Error:
			return 16.0
		case float64:
			return 1.0
		case bool:
			return 4.0
		case string:
			if x != "" {
				return 2.0
			}
		}
		return 1.0
	},
	"ERROR.TYPE": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		e, ok := args[0].Value().(Error)
		if n, hay := errorType[e]; ok && hay {
			return n
		}
		return ErrNA
	},
}

func init() { registrar(funcionesLogicas) }
