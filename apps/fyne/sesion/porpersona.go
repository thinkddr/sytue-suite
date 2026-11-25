// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"crypto/sha256"
	"encoding/hex"

	"fyne.io/fyne/v2"
)

// PorPersona es lo que la web guarda en `localStorage`: preferencias pequeñas de ESTA app
// —qué secciones del carril están plegadas, si la barra de formato se ve, cuándo caduca
// el estado personalizado— que no tienen que viajar al servidor.
//
// `localStorage` es de un ORIGEN, y cada producto de la suite es el suyo; aquí cada
// producto es su propia app y `fyne.Preferences` es de su `ID`, así que el ámbito es el
// mismo. Lo que se añade es la CUENTA: el emisor (la URL de Sytue ID), la persona y su
// organización, lo mismo que [IdentidadDeCuenta]. En un equipo donde entran dos cuentas,
// lo que una pliega no se lo encuentra plegado la otra; y la misma persona en dos
// organizaciones —o el mismo `sub` en otra instalación de Sytue ID— tampoco mezcla lo de
// una con lo de la otra. Sin nadie dentro no se guarda nada.
//
// # La migración desde el ámbito de antes
//
// Hasta el 2026-11-25 la clave era solo el `sub` (`<sub>\x00<clave>`), compartida por
// todas las organizaciones de la persona. Lo guardado así no se pierde ni se reparte: la
// primera cuenta de ese `sub` que LEE o ESCRIBE la clave se lo queda —se copia a su ámbito
// y se borra el de antes— y las demás organizaciones empiezan sin nada, como una cuenta
// nueva. Es por clave y no de golpe porque `fyne.Preferences` no sabe enumerar lo que
// guarda; cada clave pasa una sola vez y después solo queda la nueva.
//
// No es para secretos (van al llavero) ni para lo que ven los demás (va al servidor).
type PorPersona struct {
	prefs   fyne.Preferences
	alcance func() Alcance
}

// Alcance es de quién es lo guardado: el emisor (la URL de Sytue ID), el `sub` de la
// persona y su organización. Sin sujeto no hay nadie dentro.
type Alcance struct {
	Emisor, Sujeto, Organizacion string
}

// huella es el prefijo de las claves de este ámbito: una suma, para que la URL del
// emisor y los dos identificadores no vayan en cada clave, ni se confundan dos ámbitos
// cuyos campos, juntos, se escriben igual. "" sin sujeto.
func (a Alcance) huella() string {
	if a.Sujeto == "" {
		return ""
	}
	suma := sha256.Sum256([]byte("sytue-por-persona\x00" + a.Emisor + "\x00" + a.Sujeto + "\x00" + a.Organizacion))
	return hex.EncodeToString(suma[:16])
}

// PorPersona son las preferencias de esta app para la cuenta que haya entrado en cada
// momento.
func (s *Suite) PorPersona() PorPersona {
	return PorPersona{prefs: s.app.Preferences(), alcance: func() Alcance {
		// Sujeto y organización de una sola lectura: nunca mezcla dos sesiones adoptadas.
		id := s.Sesion.SnapshotIdentity()
		return Alcance{Emisor: s.URL("id"), Sujeto: id.Subject, Organizacion: id.Organization}
	}}
}

// NuevoPorPersona es PorPersona sobre unas preferencias cualesquiera; lo usan los
// guardianes.
func NuevoPorPersona(p fyne.Preferences, alcance func() Alcance) PorPersona {
	return PorPersona{prefs: p, alcance: alcance}
}

// Persona identifica la cuenta de quien ha entrado (la huella de su ámbito), o "" sin
// nadie dentro: lo que necesita quien guarda aquí algo y además lo tiene en memoria, para
// saber cuándo cambia de dueño. Cambia con la persona y también con su organización.
func (p PorPersona) Persona() string {
	if p.prefs == nil || p.alcance == nil {
		return ""
	}
	return p.alcance().huella()
}

// claves son la clave de `k` en el ámbito de ahora y la que tenía antes, con solo el
// sujeto. ok es falso sin nadie dentro.
func (p PorPersona) claves(k string) (nueva, vieja string, ok bool) {
	if p.prefs == nil || p.alcance == nil {
		return "", "", false
	}
	a := p.alcance()
	h := a.huella()
	if h == "" {
		return "", "", false
	}
	return "pp2\x00" + h + "\x00" + k, a.Sujeto + "\x00" + k, true
}

// Leer es el valor guardado, o "" si no lo hay. Lo que el ámbito de antes tenía en esa
// clave pasa a este (ver «La migración»).
func (p PorPersona) Leer(k string) string {
	nueva, vieja, ok := p.claves(k)
	if !ok {
		return ""
	}
	if v := p.prefs.String(nueva); v != "" {
		return v
	}
	v := p.prefs.String(vieja)
	if v != "" {
		p.prefs.SetString(nueva, v)
		p.prefs.RemoveValue(vieja)
	}
	return v
}

// Poner lo guarda; "" lo borra. Lo que el ámbito de antes tuviera en esa clave se borra
// también: quien escribe se queda con ella, y no la hereda luego otra organización.
func (p PorPersona) Poner(k, v string) {
	nueva, vieja, ok := p.claves(k)
	if !ok {
		return
	}
	if p.prefs.String(vieja) != "" {
		p.prefs.RemoveValue(vieja)
	}
	if v == "" {
		p.prefs.RemoveValue(nueva)
		return
	}
	p.prefs.SetString(nueva, v)
}
