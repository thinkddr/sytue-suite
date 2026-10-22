// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import "fyne.io/fyne/v2"

// PorPersona es lo que la web guarda en `localStorage`: preferencias pequeñas de ESTA app
// —qué secciones del carril están plegadas, si la barra de formato se ve, cuándo caduca
// el estado personalizado— que no tienen que viajar al servidor.
//
// `localStorage` es de un ORIGEN, y cada producto de la suite es el suyo; aquí cada
// producto es su propia app y `fyne.Preferences` es de su `ID`, así que el ámbito es el
// mismo. Lo que se añade es la PERSONA: en un equipo donde entran dos cuentas, lo que
// una pliega no se lo encuentra plegado la otra. Sin nadie dentro no se guarda nada.
//
// No es para secretos (van al llavero) ni para lo que ven los demás (va al servidor).
type PorPersona struct {
	prefs  fyne.Preferences
	sujeto func() string
}

// PorPersona son las preferencias de esta app para quien haya entrado en cada momento.
func (s *Suite) PorPersona() PorPersona {
	return PorPersona{prefs: s.app.Preferences(), sujeto: func() string { return s.Estado().Sujeto }}
}

// NuevoPorPersona es PorPersona sobre unas preferencias cualesquiera; lo usan los
// guardianes.
func NuevoPorPersona(p fyne.Preferences, sujeto func() string) PorPersona {
	return PorPersona{prefs: p, sujeto: sujeto}
}

func (p PorPersona) clave(k string) (string, bool) {
	if p.prefs == nil || p.sujeto == nil {
		return "", false
	}
	s := p.sujeto()
	return s + "\x00" + k, s != ""
}

// Leer es el valor guardado, o "" si no lo hay.
func (p PorPersona) Leer(k string) string {
	if c, ok := p.clave(k); ok {
		return p.prefs.String(c)
	}
	return ""
}

// Poner lo guarda; "" lo borra.
func (p PorPersona) Poner(k, v string) {
	c, ok := p.clave(k)
	switch {
	case !ok:
	case v == "":
		p.prefs.RemoveValue(c)
	default:
		p.prefs.SetString(c, v)
	}
}
