// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdffuente

import (
	"embed"
	"sync"
)

// Las dos familias que Sytue incrusta en los PDF que exporta, ambas SIL OFL 1.1 (los
// textos de licencia van al lado: datos/OFL-*.txt) y con fsType 0, es decir, incrustación
// libre:
//
//   - Figtree, la letra de la marca (las mismas instancias estáticas que apps/nucleo/fuentes;
//     un test vigila que no diverjan). Cubre latín y latín extendido.
//   - Liberation Sans 2.1.5 como RESERVA: añade griego y cirílico, que Figtree no trae, y
//     cubre así los 24 idiomas oficiales de la UE (el búlgaro es cirílico y el griego es
//     griego). Quedan fuera, sin glifo, las escrituras no europeas (CJK, árabe, hebreo,
//     devanagari…): el exportador sustituye esos caracteres y lo cuenta, no pinta .notdef.
//
//go:embed datos/*.ttf datos/OFL-*.txt
var datos embed.FS

var (
	unaVez sync.Once
	caras  [2][4]*Fuente
)

// Pila devuelve las fuentes en orden de preferencia para un estilo: primero la de la marca
// y luego la de reserva. Quien maqueta usa la primera que trae el carácter.
func Pila(negrita, cursiva bool) []*Fuente {
	unaVez.Do(func() {
		for fam, nombres := range [2][4]string{
			{"Figtree-Regular", "Figtree-Bold", "Figtree-Italic", "Figtree-BoldItalic"},
			{"LiberationSans-Regular", "LiberationSans-Bold", "LiberationSans-Italic", "LiberationSans-BoldItalic"},
		} {
			for i, n := range nombres {
				d, err := datos.ReadFile("datos/" + n + ".ttf")
				if err != nil {
					panic(err) // recursos fijos del binario: un nombre que falta es un error de programación
				}
				if caras[fam][i], err = Abrir(d); err != nil {
					panic(n + ": " + err.Error())
				}
			}
		}
	})
	i := 0
	if negrita {
		i |= 1
	}
	if cursiva {
		i |= 2
	}
	// Orden de `nombres`: regular, negrita, cursiva, negrita cursiva = los bits 0 y 1 de i.
	return []*Fuente{caras[0][i], caras[1][i]}
}
