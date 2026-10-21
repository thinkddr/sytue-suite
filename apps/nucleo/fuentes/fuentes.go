// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package fuentes son los ficheros de letra de la marca, en crudo.
//
// Bytes de TTF y nada más: quien los quiera los convierte con lo que use su toolkit
// —`opentype.ParseCollection` en Gio, `fyne.NewStaticResource` en Fyne—. Están aquí, y
// no dentro del tema de uno de los dos, porque son 280 KB por copia y la copia que se
// queda atrás es la que un día enseña otra letra.
//
// Figtree (SIL OFL 1.1, ver OFL.txt), instancias ESTÁTICAS de
// github.com/erikdkennedy/figtree. Estáticas y no la variable que usa la web: ni Gio ni
// Fyne instancian los ejes de variación, así que una variable saldría siempre en su peso
// por defecto y «SemiBold» sería un nombre sin efecto.
//
// Y la cara monoespaciada, [Mono]: JetBrains Mono Regular v2.304 (SIL OFL 1.1, ver
// OFL-JetBrainsMono.txt), estática, de github.com/JetBrains/JetBrainsMono. Es la primera
// familia del `--font-mono` de la web (`packages/config/src/theme.css`); la web no la
// autoaloja y el navegador cae a la del sistema, pero aquí no hay pila de respaldo que
// valga: sin ella un PEM o un identificador salían en Figtree, sin columnas.
//
// [Matematica] es Noto Sans Math Regular v3.000 (SIL OFL 1.1, ver
// OFL-NotoSansMath.txt), el TTF hinted original de la release
// github.com/notofonts/math/releases/tag/NotoSansMath-v3.000. La reserva web de las
// flechas usa esta cara; traerla evita que su avance dependa de la versión del SO.
package fuentes

import (
	"embed"
	"io/fs"
)

//go:embed OFL.txt Figtree-Regular.ttf Figtree-Medium.ttf Figtree-SemiBold.ttf Figtree-Bold.ttf Figtree-ExtraBold.ttf Figtree-Italic.ttf Figtree-BoldItalic.ttf JetBrainsMono-Regular.ttf NotoSansMath-Regular.ttf OFL-NotoSansMath.txt
var ttf embed.FS

// FS contiene las caras .ttf y las licencias embebidas, por su nombre de fichero.
var FS fs.FS = ttf

// ExtraNegra es Figtree ExtraBold (800), de la misma procedencia que las demás: el peso
// de los títulos de la hoja en la forma Marco (`--sytue-title-weight`). No está en
// Nombres, que son las seis caras del texto corriente.
const ExtraNegra = "Figtree-ExtraBold.ttf"

// Mono es el fichero de la cara monoespaciada. No está en Nombres, que es la familia
// de la marca: quien recorre Nombres para armar su colección de Figtree no la quiere.
const Mono = "JetBrainsMono-Regular.ttf"

// Matematica es la cara de reserva para símbolos. No pertenece a Nombres, que
// enumera sólo la familia Figtree de la marca.
const Matematica = "NotoSansMath-Regular.ttf"

// Nombres son las seis caras de Figtree, en el orden en que se declaran.
var Nombres = []string{
	"Figtree-Regular.ttf",
	"Figtree-Medium.ttf",
	"Figtree-SemiBold.ttf",
	"Figtree-Bold.ttf",
	"Figtree-Italic.ttf",
	"Figtree-BoldItalic.ttf",
}

// Leer devuelve una cara o licencia embebida. Un nombre que no está es un error de
// programación —los recursos son fijos—, así que entra en pánico.
func Leer(nombre string) []byte {
	datos, err := fs.ReadFile(ttf, nombre)
	if err != nil {
		panic("fuentes: " + err.Error())
	}
	return datos
}
