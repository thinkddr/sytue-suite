// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"log/slog"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// MotivoNoSabeEditar es el MotivoSoloLectura de un documento que el lienzo nativo no
// entiende entero. No es un problema de permisos y la franja no lo dice como si lo fuera:
// explica QUÉ no sabe editar este editor todavía y ofrece seguir en la web, que sí sabe.
const MotivoNoSabeEditar = "editor.readOnly.unsupported"

// queNoSabe traduce los motivos técnicos de los lienzos (los nombres de HTML de
// `rico.Documento.NoEntendido`, las claves de JSON que `diapos` no conserva) a lo que
// una persona reconoce en su documento. El orden es el de la frase: lo más concreto
// primero, y «parte de su contenido» para todo lo demás, que no tiene nombre llano.
var queNoSabe = []que{
	{"editor.readOnly.what.drawing", func(m string) bool { return strings.Contains(m, "data-dibujo") }},
	{"editor.readOnly.what.embed", func(m string) bool {
		return m == "<iframe>" || m == "<object>" || m == "<embed>" || strings.Contains(m, "data-embed") || strings.Contains(m, "data-sytue-embed")
	}},
	{"editor.readOnly.what.formFields", func(m string) bool {
		return strings.Contains(m, "data-formfield") || m == "<input>" || m == "<select>" || m == "<textarea>"
	}},
	{"editor.readOnly.what.protection", func(m string) bool { return m == "protect" || strings.HasPrefix(m, "protect.") }},
}

type que struct {
	clave string
	es    func(motivo string) bool
}

// ClavesNoEditable son las claves de catálogo de lo que los motivos describen, sin
// repetir y en el orden de la frase. Si ninguno se reconoce, «parte de su contenido o
// formato»: decir algo vago es mejor que callarlo. Si alguno sí, el resto no se añade: un
// dibujo trae su `<figure>` y su imagen de respaldo, que no son otra cosa que el dibujo.
func ClavesNoEditable(motivos []string) []string {
	var claves []string
	for _, q := range queNoSabe {
		if slices.ContainsFunc(motivos, q.es) {
			claves = append(claves, q.clave)
		}
	}
	if len(claves) == 0 {
		claves = append(claves, "editor.readOnly.what.other")
	}
	return claves
}

// NoSabeEditar deja el contexto en solo lectura porque el lienzo no entiende el
// documento entero: la franja dirá qué (NoEditable) y ofrecerá EnLaWeb. Los motivos
// técnicos no se enseñan, pero van al registro una vez por carga, que es lo que hace falta
// para saber qué falta por soportar.
func (c *Contexto) NoSabeEditar(app string, motivos []string) {
	c.MotivoSoloLectura = MotivoNoSabeEditar
	c.NoEditable = ClavesNoEditable(motivos)
	slog.Info(app+": solo lectura, el editor nativo no entiende el documento entero", "motivos", motivos)
}

// SinMotivo vuelve al motivo de siempre, el permiso. Va tras cargar un documento que se
// entiende, o cuando quien abre no puede escribir: sin permiso, la salida útil es pedir
// acceso (EngancharPeticionEdicion), no irse a la web, donde tampoco podría editar.
func (c *Contexto) SinMotivo() {
	c.MotivoSoloLectura, c.NoEditable = "", nil
}

// textoNoSabeEditar es el aviso de la franja, con lo que no sabe ya traducido. Sin
// EnLaWeb (un fichero del disco, una conversión) no hay web a la que mandar, así que el
// aviso no la promete: dice qué no sabe editar y nada más.
func (c *Contexto) textoNoSabeEditar() string {
	que := make([]string, len(c.NoEditable))
	for i, k := range c.NoEditable {
		que[i] = c.Traducir(k)
	}
	clave := MotivoNoSabeEditar
	if c.EnLaWeb == nil {
		clave = "editor.readOnly.unsupportedHere"
	}
	return c.Traducir(clave, i18n.Vars{"what": strings.Join(que, ", ")})
}
