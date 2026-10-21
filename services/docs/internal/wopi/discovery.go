// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package wopi

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
)

// El catálogo de descubrimiento: el documento que un host WOPI lee para saber
// qué aplicación abre cada extensión y con qué URL.
//
// Es XML y no JSON porque así lo define el protocolo, y porque el objetivo es
// que CUALQUIER host WOPI —no solo Sytue Drive— pueda usar Sytue Docs como
// editor. Un formato propio aquí ahorraría veinte líneas y cerraría esa puerta.

// Accion es lo que se puede hacer con una extensión.
type Accion string

const (
	// AccionVer abre en solo lectura. Es la que resuelve previsualizar: no crea
	// documento, no duplica y no deja rastro en la unidad de nadie.
	AccionVer Accion = "view"
	// AccionEditar abre para escribir. El host decide si la ofrece mirando
	// `UserCanWrite`; anunciarla aquí no la concede.
	AccionEditar Accion = "edit"
)

// app agrupa extensiones bajo un nombre de aplicación, como manda el formato.
type app struct {
	nombre string
	exts   []string
	// editable: si no, solo se anuncia `view`. Un PDF se ve, no se edita como
	// documento ofimático —para eso está el editor de PDF, que es otra cosa—.
	editable bool
}

// anunciarEdicion enciende la acción `edit` del catálogo.
//
// SIGUE APAGADA, pero ya no por lo mismo. El ciclo de guardado contra el host
// existe (`internal/http/wopi_save.go`: Lock → PutFile → Unlock, con el formato
// original y el permiso revalidado). Lo que falta es la SUPERFICIE: la pantalla
// `/wopi/editor` pinta el documento en un marco aislado de solo lectura, porque
// el lienzo de Sytue Docs está atado a un documento del servicio y a su sesión de
// colaboración, y trabajar sobre un modelo suelto es un cambio en el editor, no
// aquí.
//
// Anunciar `edit` sin esa superficie mandaría a la gente a una pantalla donde no
// se puede teclear, que es peor que no ofrecer la acción. Los hosts caen solos a
// `view` cuando no lo encuentran (el nuestro lo hace en URLFor), así que apagarlo
// no deja a nadie sin abrir su fichero.
//
// El día que el lienzo acepte un modelo suelto, esto pasa a true y su check se
// invierte. No hace falta tocar nada más.
const anunciarEdicion = false

// Catálogo de Sytue Docs. Las extensiones son las que el importador sabe
// convertir de verdad (`domain.Import`): anunciar una que no se sabe abrir da
// una pantalla en blanco, que es peor que no ofrecerla.
var apps = []app{
	{nombre: "Sytue Docs", exts: []string{"docx", "doc", "odt", "rtf", "txt", "md", "html", "htm", "pages"}, editable: true},
	{nombre: "Sytue Sheets", exts: []string{"xlsx", "xls", "ods", "csv", "numbers"}, editable: true},
	{nombre: "Sytue Slides", exts: []string{"pptx", "ppt", "odp", "key"}, editable: true},
	{nombre: "Sytue PDF", exts: []string{"pdf"}, editable: false},
}

// --- serialización del catálogo --------------------------------------------

type xmlDiscovery struct {
	XMLName xml.Name   `xml:"wopi-discovery"`
	NetZone xmlNetZone `xml:"net-zone"`
}

type xmlNetZone struct {
	Name string    `xml:"name,attr"`
	Apps []xmlApp  `xml:"app"`
	Prf  *xmlProof `xml:"proof-key,omitempty"`
}

type xmlApp struct {
	Name    string      `xml:"name,attr"`
	Actions []xmlAction `xml:"action"`
}

type xmlAction struct {
	Name   string `xml:"name,attr"`
	Ext    string `xml:"ext,attr"`
	URLSrc string `xml:"urlsrc,attr"`
}

// xmlProof queda declarado pero no se emite: la firma de peticiones del host
// (proof key) es una capa OPCIONAL del protocolo y anunciarla sin implementar la
// verificación sería mentir en el catálogo. Ver la nota de PENDIENTES.
type xmlProof struct {
	Value string `xml:"value,attr"`
}

// Descubrimiento genera el `discovery.xml` con `base` como raíz pública de Docs
// (por ejemplo https://docs.sytue.eu).
//
// El `urlsrc` lleva el marcador `WOPISrc` que el host sustituye por la dirección
// del fichero. Se emite ADEMÁS `<ui=UI_LLM&>` con la convención de parámetros
// opcionales del protocolo: el host que no los entienda los ignora, y el que sí,
// nos manda el idioma y evita que el editor abra en otro distinto al de la app
// desde la que se pulsó.
func Descubrimiento(base string) ([]byte, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return nil, fmt.Errorf("wopi: falta la URL pública de Sytue Docs")
	}
	if !strings.HasPrefix(base, "https://") {
		return nil, fmt.Errorf("wopi: la URL pública debe ser https")
	}

	out := xmlDiscovery{NetZone: xmlNetZone{Name: "external-https"}}
	for _, a := range apps {
		x := xmlApp{Name: a.nombre}
		exts := append([]string(nil), a.exts...)
		sort.Strings(exts) // orden estable: el catálogo se cachea y se compara
		for _, ext := range exts {
			x.Actions = append(x.Actions, xmlAction{
				Name:   string(AccionVer),
				Ext:    ext,
				URLSrc: fmt.Sprintf("%s/wopi/view?WOPISrc=<WOPISrc>&lang=<ui=UI_LLCC&>", base),
			})
			if a.editable && anunciarEdicion {
				x.Actions = append(x.Actions, xmlAction{
					Name:   string(AccionEditar),
					Ext:    ext,
					URLSrc: fmt.Sprintf("%s/wopi/edit?WOPISrc=<WOPISrc>&lang=<ui=UI_LLCC&>", base),
				})
			}
		}
		out.NetZone.Apps = append(out.NetZone.Apps, x)
	}

	cuerpo, err := xml.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("wopi: serializar el catálogo: %w", err)
	}
	return append([]byte(xml.Header), cuerpo...), nil
}

// Soporta indica si Sytue Docs sabe abrir esa extensión, y con qué acciones.
func Soporta(ext string) (ver, editar bool) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	for _, a := range apps {
		for _, e := range a.exts {
			if e == ext {
				return true, a.editable
			}
		}
	}
	return false, false
}
