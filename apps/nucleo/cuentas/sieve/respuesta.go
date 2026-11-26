// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sieve

// # La respuesta automática sin tocar las reglas de la persona
//
// En ManageSieve solo hay UN script activo a la vez, y la persona puede tener ya el suyo
// (las reglas que escribió en la web de su proveedor, en Roundcube, en Thunderbird…).
// Sustituirlo por una respuesta automática se las borraría en la práctica. Por eso la app
// escribe SOLO su propio script (el nombre lo pone quien llama: «sytue-office») y:
//
//   - si no hay ningún script activo, sube el propio y lo activa;
//   - si el activo es el propio, lo sustituye (el que incluía, lo sigue incluyendo);
//   - si hay OTRO activo y el servidor sabe incluir scripts (`include`, RFC 6609), el
//     propio hace `include :personal :optional "<el suyo>"` DETRÁS de la respuesta, de
//     modo que sus reglas siguen funcionando igual, y guarda cuál era en un comentario
//     legible del propio script para devolverlo al apagar;
//   - si hay otro activo y no se puede incluir (el servidor no sabe, o el suyo ya
//     responde o rechaza correo —RFC 5230 §4.7 y RFC 5429: no casan con una segunda
//     respuesta—, o incluye scripts globales que no se pueden leer), no se toca nada:
//     [ErrBloqueada], y la pantalla lo explica.
//
// Apagarla deja activo exactamente lo que había antes: el script guardado en el
// comentario o, si no había ninguno, ninguno. El texto de la respuesta se queda en el
// script propio, ya inactivo, para la próxima vez. El estado que enseña la pantalla se
// lee siempre del servidor ([LeerEstado]): no hay copia aparte que se desincronice.

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// ErrSinVacaciones: el servidor no tiene la extensión `vacation`.
var ErrSinVacaciones = errors.New("sieve: el servidor no admite respuesta automática (vacation)")

// ErrBloqueada es no poder activar la respuesta sin cambiar las reglas de la persona.
type ErrBloqueada struct {
	// Script es el de la persona que lo impide.
	Script string
}

func (e *ErrBloqueada) Error() string {
	nombre := e.Script
	if utf8.RuneCountInString(nombre) > 60 {
		nombre = string([]rune(nombre)[:60]) + "…"
	}
	return fmt.Sprintf("sieve: las reglas activas «%s» no se pueden combinar con la respuesta automática sin cambiarlas", nombre)
}

// Estado es lo que hay en el servidor para la respuesta automática.
type Estado struct {
	// Respuesta es lo que dice el script propio (con los días de serie si no hay).
	Respuesta
	// Existe: hay script propio. Activa: es el activo, así que responde.
	Existe, Activa bool
	// Otro es el script activo de la persona cuando no es el propio.
	Otro string
	// Anterior es el script de la persona que el propio incluye y que vuelve a quedar
	// activo al apagar la respuesta ("" si no había ninguno, o si ya no existe).
	Anterior string
	// ConFechas: el servidor admite `date` y `relational`. ConInclude: admite `include`.
	ConFechas, ConInclude bool
	// Bloqueada es el script de la persona que impide activar la respuesta sin tocarlo
	// (ver [ErrBloqueada]); vacío si se puede. También es el propio nombre cuando hay un
	// script con él que no escribió la app.
	Bloqueada string
}

// Topes al mirar los scripts que incluye el de la persona.
const (
	hondoInclude = 3
	scriptsMirar = 10
)

// LeerEstado lee del servidor el estado de la respuesta automática: la lista de scripts,
// el propio (con el nombre `propio`) y, si hay otro activo, si se puede combinar con él.
func LeerEstado(c *Cliente, propio string) (Estado, error) {
	if err := NombreValido(propio); err != nil {
		return Estado{}, err
	}
	caps := c.Capacidades()
	if !caps.Extension("vacation") {
		return Estado{}, ErrSinVacaciones
	}
	st := Estado{ConFechas: caps.Extension("date") && caps.Extension("relational"), ConInclude: caps.Extension("include")}
	scripts, err := c.Scripts()
	if err != nil {
		return Estado{}, err
	}
	existe := func(n string) bool {
		for _, s := range scripts {
			if s.Nombre == n {
				return true
			}
		}
		return false
	}
	for _, s := range scripts {
		if s.Nombre == propio {
			st.Existe, st.Activa = true, s.Activo
		} else if s.Activo {
			st.Otro = s.Nombre
		}
	}
	if st.Existe {
		bloqueado, err := leerPropio(c, propio, existe, &st)
		if err != nil {
			return Estado{}, err
		}
		if bloqueado {
			return st, nil
		}
	}
	if st.Dias <= 0 {
		st.Dias = DiasDeSerie
	}
	if st.Otro != "" {
		ok := st.ConInclude
		if ok {
			if ok, err = combinable(c, st.Otro, existe); err != nil {
				return Estado{}, err
			}
		}
		if !ok {
			st.Bloqueada = st.Otro
		}
	}
	return st, nil
}

// leerPropio lee en st lo que dice el script propio. bloqueado: no lo escribió la app (es
// de la persona y se llama como el propio), así que no se toca y no hay más que mirar.
func leerPropio(c *Cliente, propio string, existe func(string) bool, st *Estado) (bloqueado bool, err error) {
	texto, err := c.Leer(propio)
	if err != nil {
		return false, err
	}
	if !EsPropio(texto) {
		// Un script de la persona que se llama como el de la app: no se toca nunca.
		st.Existe, st.Bloqueada = false, propio
		if st.Activa {
			st.Activa, st.Otro = false, propio
		}
		st.Dias = DiasDeSerie
		return true, nil
	}
	r, anterior, _ := LeerRespuesta(texto)
	st.Respuesta = r
	if st.Activa && anterior != "" && existe(anterior) {
		st.Anterior = anterior
	}
	return false, nil
}

// combinable dice si el script de la persona puede ir detrás de la respuesta automática
// sin cambiar lo que hace: ni él ni lo que incluye responden ni rechazan correo, ni
// incluyen scripts globales (que no se pueden leer para comprobarlo). Lo que no se puede
// leer o entender cuenta como que no. err solo es la conexión.
func combinable(c *Cliente, nombre string, existe func(string) bool) (bool, error) {
	vistos := map[string]bool{}
	var mirar func(n string, hondo int) (bool, error)
	mirar = func(n string, hondo int) (bool, error) {
		if vistos[n] {
			return true, nil
		}
		if hondo > hondoInclude || len(vistos) >= scriptsMirar {
			return false, nil
		}
		vistos[n] = true
		if !existe(n) {
			return true, nil // un include de algo que no está no hace nada
		}
		texto, err := c.Leer(n)
		if err != nil {
			var es *ErrorServidor
			if errors.As(err, &es) && es.Respuesta == "NO" {
				return false, nil
			}
			return false, err
		}
		a, err := analizar(texto)
		if err != nil || a.choca || a.global {
			return false, nil //nolint:nilerr // un script que no se entiende cuenta como que no combina; el error es solo el de la conexión
		}
		for _, inc := range a.incluidos {
			if ok, err := mirar(inc, hondo+1); !ok || err != nil {
				return ok, err
			}
		}
		return true, nil
	}
	return mirar(nombre, 0)
}

// Guardar escribe la respuesta en el script propio y lo activa (activar) o lo apaga,
// con las reglas del principio del fichero: nunca sustituye ni cambia un script de la
// persona. Antes de subirlo lo valida el servidor con CHECKSCRIPT, si lo tiene.
func Guardar(c *Cliente, propio string, r Respuesta, activar bool) error {
	st, err := LeerEstado(c, propio)
	if err != nil {
		return err
	}
	if st.Bloqueada == propio {
		return &ErrBloqueada{Script: propio}
	}
	r = Normalizar(r)
	if (!r.Desde.IsZero() || !r.Hasta.IsZero()) && !st.ConFechas {
		return ErrSinFechas
	}
	if activar {
		return encender(c, propio, r, st)
	}
	return apagar(c, propio, r, st)
}

// encender sube la respuesta al script propio, delante del que estuviera activo, y lo
// deja activo.
func encender(c *Cliente, propio string, r Respuesta, st Estado) error {
	anterior := st.Anterior
	if !st.Activa && st.Otro != "" {
		if st.Bloqueada != "" {
			return &ErrBloqueada{Script: st.Bloqueada}
		}
		anterior = st.Otro
	}
	texto, err := Componer(r, anterior, st.ConFechas)
	if err != nil {
		return err
	}
	if err := comprobar(c, texto); err != nil {
		return err
	}
	if err := c.Subir(propio, texto); err != nil {
		return err
	}
	if st.Activa {
		return nil // sustituir el activo ya vale en el acto
	}
	return c.Activar(propio)
}

// apagar devuelve el script que estaba activo y guarda la respuesta, sin activarla, para
// la próxima vez.
func apagar(c *Cliente, propio string, r Respuesta, st Estado) error {
	texto, err := Componer(r, "", st.ConFechas)
	if err != nil {
		return err
	}
	if st.Activa {
		// Primero se devuelve lo que había: apagar es lo que se pidió, y guardar el texto
		// para la próxima vez es secundario.
		if err := c.Activar(st.Anterior); err != nil {
			return err
		}
	}
	if !st.Existe && r.Mensaje == "" && r.Asunto == "" {
		return nil // nada que recordar: no se deja un script vacío en el servidor
	}
	if err := comprobar(c, texto); err != nil {
		return err
	}
	return c.Subir(propio, texto)
}

// comprobar valida el script en el servidor si tiene CHECKSCRIPT; si no, ya lo valida
// PUTSCRIPT antes de guardarlo.
func comprobar(c *Cliente, texto string) error {
	if err := c.Comprobar(texto); err != nil && !errors.Is(err, ErrNoAdmitido) {
		return err
	}
	return nil
}
