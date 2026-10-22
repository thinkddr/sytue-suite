// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/logofit"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// La lista de colores y la de formas viven cada una en dos sitios que nadie compila
// juntos: el dominio, que valida lo que guarda el administrador, y la hoja de tokens,
// que es quien pinta. Un valor aceptado aquí y sin bloque allí se guardaría bien y no
// cambiaría nada en pantalla; uno pintable y no aceptado no se podría elegir.
func TestLaAparienciaDeOrganizacionExisteEnLaHoja(t *testing.T) {
	for _, eje := range []struct {
		atributo, hoja, deLaCasa string
		dominio                  []string
	}{
		{"data-color", coloresCSS, "petroleo", domain.OrgColors},
		{"data-forma", appearanceCSS, domain.OrgFormaDefault, domain.OrgFormas},
	} {
		bloques := regexp.MustCompile(`(?m)^\[`+eje.atributo+`='([a-z]+)'\] \{`).FindAllStringSubmatch(eje.hoja, -1)
		enLaHoja := []string{eje.deLaCasa} // el de la casa es la hoja sin atributo
		for _, m := range bloques {
			enLaHoja = append(enLaHoja, m[1])
		}
		if !slices.Equal(enLaHoja, eje.dominio) {
			t.Errorf("%s: la hoja pinta %v y el dominio acepta %v", eje.atributo, enLaHoja, eje.dominio)
		}
	}
}

// Los mensajes de error de la subida los lee un administrador que no tiene por qué
// saber de imágenes. «error al decodificar» no le sirve de nada: cada fallo tiene
// que decir qué pasa Y qué hacer con su fichero.
func TestCadaFalloDelReencuadreDiceQueHacer(t *testing.T) {
	casos := []struct {
		err   error
		pista string // lo accionable que NO puede faltar
	}{
		{logofit.ErrTipo, "PNG"},
		{logofit.ErrEnorme, "px"},
		{logofit.ErrIlegible, "Export"},
		{logofit.ErrVacio, "colour"},
		{logofit.ErrAlargado, "Crop"},
	}
	for _, c := range casos {
		msg := mensajeLogo(c.err)
		if msg == "" || msg == "The image could not be processed." {
			t.Errorf("%v cae en el mensaje genérico", c.err)
			continue
		}
		if !strings.Contains(msg, c.pista) {
			t.Errorf("%v → %q: no dice qué hacer (falta %q)", c.err, msg, c.pista)
		}
	}
	// Un error desconocido no puede quedarse sin mensaje.
	if mensajeLogo(errors.New("otro")) == "" {
		t.Error("un error inesperado tiene que dar algún mensaje")
	}
}

// El caso más frecuente de todos —un logo corporativo en SVG— tiene que salir del
// rechazo sabiendo exactamente qué hacer, porque le va a pasar a mucha gente.
func TestElRechazoDeSVGExplicaLaSalida(t *testing.T) {
	msg := mensajeLogo(logofit.ErrTipo)
	for _, quiero := range []string{"SVG", "PNG"} {
		if !strings.Contains(msg, quiero) {
			t.Errorf("el mensaje de tipo no admitido no menciona %q: %q", quiero, msg)
		}
	}
}
