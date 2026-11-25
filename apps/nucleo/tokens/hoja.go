// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"encoding/binary"
	"hash/crc32"
	"image/color"
)

// # Las paletas y las formas de arranque, generadas al compilar
//
// paletas_generadas.go trae ya resueltas las crudas de los juegos que se piden al
// arrancar —el de la casa y el del cromo «papel»— y los colores de la suite que declara
// la hoja; formas_generadas.go, las medidas de las dos formas que se leen de theme.css
// (forma.go). Los escribe `go generate ./tokens`, que corre los guardianes que los
// comparan con lo que sale de leer las hojas embebidas
// (TestLas{Paletas,Formas}GeneradasSonLasDeLaHoja con `-generar`): así el lector de la
// hoja sigue siendo UNO, el de colores.go y forma.go, y lo generado es su salida tal cual.
//
// Lo generado lleva la huella de las hojas de las que salió. Si scripts/sync-branding.sh
// trae una hoja nueva y nadie regenera, la huella no coincide y se lee la hoja como antes:
// más lento, nunca con colores ni medidas viejos. Y el guardián se pone en rojo.

//go:generate go test -run ^TestLas(Paletas|Formas)GeneradasSonLasDeLaHoja$ -generar

// tokenGenerado es un token en los juegos generados: su nombre y su color crudo, como
// 0xRRGGBBAA, en cada juego (en el orden de pedidosGenerados) y cada variante.
type tokenGenerado struct {
	nombre  string
	colores [len(pedidosGenerados)][variantes]uint32
}

// pedidosGenerados son los juegos que van generados: el de la casa y el del papel.
var pedidosGenerados = [...]pedido{{}, {papel: true}}

// huellaHojas es la huella de las dos hojas de color, en el orden en que se apilan.
func huellaHojas() uint64 { return huella(coloresCSS, aparienciaCSS) }

// huellaFormas es la de las dos hojas de las que salen las formas.
func huellaFormas() uint64 { return huella(temaCSS, aparienciaCSS) }

// castagnoli es la tabla de la segunda CRC de la huella.
var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// huella son dos CRC-32 de las hojas —la IEEE arriba, la Castagnoli abajo— separadas por
// un cero. No es para defenderse de nadie, sino de una hoja que cambia sin regenerar, y
// las dos van con instrucciones del procesador: unos 10 µs las tres hojas, donde la
// FNV-64a de antes eran 0,2 ms y una copia de cada hoja. Se pasan por un búfer de la pila
// para no copiar la hoja entera a un `[]byte`.
func huella(hojas ...string) uint64 {
	var (
		buf      [4096]byte
		ieee, ca uint32
	)
	for i, hoja := range hojas {
		if i > 0 {
			buf[0] = 0
			ieee, ca = crc32.Update(ieee, crc32.IEEETable, buf[:1]), crc32.Update(ca, castagnoli, buf[:1])
		}
		for hoja != "" {
			n := copy(buf[:], hoja)
			ieee, ca = crc32.Update(ieee, crc32.IEEETable, buf[:n]), crc32.Update(ca, castagnoli, buf[:n])
			hoja = hoja[n:]
		}
	}
	return uint64(ieee)<<32 | uint64(ca)
}

// leerHoja da los colores de la suite que declara la hoja y los juegos de arranque: los
// generados si son de estas hojas, y si no, leyendo la de la casa.
func leerHoja() (map[string]bool, map[pedido]*juego) {
	return leerHojaCon(huellaGenerada == huellaHojas())
}

func leerHojaCon(generadas bool) (map[string]bool, map[pedido]*juego) {
	if !generadas {
		return coloresDeclarados(coloresCSS), mustJuegos(coloresCSS, aparienciaCSS)
	}
	colores := make(map[string]bool, len(coloresGenerados))
	for _, c := range coloresGenerados {
		colores[c] = true
	}
	js := make(map[pedido]*juego, len(pedidosGenerados))
	for i, p := range pedidosGenerados {
		var crudas [variantes]map[string]color.NRGBA
		for v := range crudas {
			crudas[v] = make(map[string]color.NRGBA, len(tokensGenerados))
			for k := range tokensGenerados {
				tk := &tokensGenerados[k]
				var c [4]byte
				binary.BigEndian.PutUint32(c[:], tk.colores[i][v])
				crudas[v][tk.nombre] = color.NRGBA{R: c[0], G: c[1], B: c[2], A: c[3]}
			}
		}
		js[p] = juegoDe(crudas)
	}
	return colores, js
}

// juegoDe completa un juego a partir de sus crudas: los bordes aplanados y su índice.
func juegoDe(crudas [variantes]map[string]color.NRGBA) *juego {
	j := &juego{crudas: crudas, paletas: aplanar(crudas)}
	j.bordes = indiceDeBordes(j)
	return j
}

// leerFormas da las medidas de las dos formas: las generadas si son de estas hojas, y si
// no, leyendo theme.css.
func leerFormas() [2]map[string]Medida { return leerFormasCon(huellaFormasGenerada == huellaFormas()) }

func leerFormasCon(generadas bool) [2]map[string]Medida {
	if !generadas {
		return mustFormas(temaCSS, aparienciaCSS)
	}
	return formasGeneradas
}
