// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"errors"
	"net/http"
)

// Fase es en cuál de sus estados está una pantalla.
//
// Son los seis que toda pantalla de la suite tiene que saber pintar —carga, éxito,
// vacío, error, sin conexión y permiso denegado— más la sesión caducada, que la web
// distingue del error de red porque lo que hay que hacer es otro (`files.error.session*`
// contra `files.error.network`).
//
// Vive aquí y no en el toolkit porque la cuenta que la produce es de ESTE paquete: lo
// que la decide son los códigos HTTP que devuelve `*api.Error`. Estaba escrita tres
// veces —`drive`, `notes` y `erp`, las dos primeras idénticas hasta la coma— y las tres
// la importan ahora con un alias, que es lo que pide `docs/CONVENTIONS.md` §13.2.
type Fase uint8

// Las fases, con el cero en «cargando»: una pantalla recién construida todavía no ha
// pedido nada, y ese es el estado honesto.
const (
	FaseCargando Fase = iota
	FaseExito
	FaseVacia
	FaseFallo
	FaseSinSesion
	FaseProhibida
	FaseSinConexion
)

// FaseDe traduce el resultado de una carga a la fase que toca pintar.
//
// El orden importa y es el de la web: sin red no se enseña un error de red —eso haría
// creer que el servicio está caído—, un 401 es la sesión y no un fallo, y un 403 es
// permiso y no un reintento. Solo lo que sobrevive a los tres es «no se pudo cargar».
//
// Quien no distinga el 401 —el ERP, porque su web tampoco lo distingue— pinta
// `FaseSinSesion` igual que `FaseFallo`; no se le quita la fase a los demás por eso.
func FaseDe(err error, hayRed bool, elementos int) Fase {
	if !hayRed {
		return FaseSinConexion
	}
	var e *Error
	if errors.As(err, &e) {
		switch e.Status {
		case http.StatusUnauthorized:
			return FaseSinSesion
		case http.StatusForbidden:
			return FaseProhibida
		}
	}
	if err != nil {
		return FaseFallo
	}
	if elementos == 0 {
		return FaseVacia
	}
	return FaseExito
}
