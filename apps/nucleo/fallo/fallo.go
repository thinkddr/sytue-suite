// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package fallo pone nombre FIJO a por qué falló una operación del núcleo de
// cliente, para que el navegador lo enseñe en «Más información» sin copiar nada
// del error: ni direcciones, ni nombres, ni textos de un servicio.
//
// Sin esto, el núcleo de Mail contestaba `crypto_rejected` a secas y una cuenta
// que no abría el correo (`prepare/error.Error`) no decía qué paso había fallado.
//
// El motivo son los pasos marcados con [En], de fuera adentro, y el nombre del
// primer error conocido ([Conocido]) de la cadena: `abrir.preparar.capacidades.no_listo`.
package fallo

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
)

// Un nombre de paso o de error es una palabra fija del código.
var nombreValido = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

const (
	maxTrozos = 8
	maxLargo  = 160
)

type paso struct {
	nombre string
	err    error
}

func (p *paso) Error() string { return p.nombre + ": " + p.err.Error() }
func (p *paso) Unwrap() error { return p.err }

// En marca err con el paso en que iba la operación. nombre es una palabra fija del
// código (`[a-z0-9_]{1,32}`); si no lo es, err se devuelve tal cual. Sin error, nil.
func En(nombre string, err error) error {
	if err == nil || !nombreValido.MatchString(nombre) {
		return err
	}
	return &paso{nombre: nombre, err: err}
}

type conocido struct {
	err    error
	nombre string
}

var (
	mu        sync.RWMutex
	conocidos = []conocido{
		{context.Canceled, "cancelado"},
		{context.DeadlineExceeded, "plazo"},
	}
)

// Conocido da nombre fijo a un error centinela (`var ErrX = errors.New(...)`). Lo
// llama quien sirve el módulo, una vez, al arrancar.
func Conocido(err error, nombre string) {
	if err == nil || !nombreValido.MatchString(nombre) {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, c := range conocidos {
		if c.err == err { //nolint:errorlint // se busca el centinela mismo, no su cadena
			return
		}
	}
	conocidos = append(conocidos, conocido{err: err, nombre: nombre})
}

// Motivo dice por qué falló err con palabras fijas: sus pasos y su error conocido.
// Vacío si no hay ni lo uno ni lo otro.
func Motivo(err error) string {
	mu.RLock()
	defer mu.RUnlock()
	trozos := recorrer(err, nil, 0)
	if len(trozos) > maxTrozos {
		trozos = trozos[:maxTrozos]
	}
	motivo := strings.Join(trozos, ".")
	for len(motivo) > maxLargo && len(trozos) > 1 {
		trozos = trozos[:len(trozos)-1]
		motivo = strings.Join(trozos, ".")
	}
	return motivo
}

// recorrer baja por la cadena de err: apunta los pasos y se para en el primer
// error conocido. De un errors.Join se queda con la primera rama que diga algo.
func recorrer(err error, trozos []string, profundidad int) []string {
	for err != nil && profundidad < 64 {
		profundidad++
		for _, c := range conocidos {
			if err == c.err { //nolint:errorlint // el centinela mismo, sin recorrer su cadena otra vez
				return append(trozos, c.nombre)
			}
		}
		if p, ok := err.(*paso); ok { //nolint:errorlint // se recorre la cadena a mano, paso a paso
			trozos = append(trozos, p.nombre)
			err = p.err
			continue
		}
		if varios, ok := err.(interface{ Unwrap() []error }); ok { //nolint:errorlint // ídem
			for _, rama := range varios.Unwrap() {
				if mas := recorrer(rama, nil, profundidad); len(mas) > 0 {
					return append(trozos, mas...)
				}
			}
			return trozos
		}
		err = errors.Unwrap(err)
	}
	return trozos
}
