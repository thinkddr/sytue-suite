// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package guiones reúne las reglas de los scripts de usuario que NO dependen del
// producto (ADR 0069): las cotas del código, las cuotas FIJAS por hora, los estados de
// una versión, quién puede ejecutar cuál y el hash que avala una aprobación. Docs y
// Bases las importan; cada uno pone su manifiesto, sus tablas y su ACL.
//
// Viven aquí para que no diverjan: dos copias de una cuota acaban con dos números.
// Todo es PURO: ni base de datos ni HTTP.
package guiones

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

// Cotas del código y de una ejecución. Ninguna es técnica; son las de un guion de oficina.
const (
	MaxCodigo     = 64 * 1024
	MaxNombre     = 120
	TopeOps       = 10_000
	TopeMs        = 30_000
	OpsPorDefecto = 1_000
	MsPorDefecto  = 5_000
)

// Cuotas por hora móvil, FIJAS (no las cambia ni el producto ni la organización). El
// tiempo cuenta lo reservado (el máximo del manifiesto) hasta que la ejecución se cierra
// con el medido, de modo que abrir y abandonar no es gratis.
const (
	CuotaEjecucionesUsuarioHora = 60
	CuotaEjecucionesOrgHora     = 600
	CuotaMsUsuarioHora          = int64(10 * 60 * 1000)
)

// Estados de una versión: draft -> pending (el autor la propone) -> approved | rejected.
const (
	EstadoBorrador  = "draft"
	EstadoPendiente = "pending"
	EstadoAprobada  = "approved"
	EstadoRechazada = "rejected"
)

// Resultados de una ejecución.
const (
	ResEjecutando = "running"
	ResOK         = "ok"
	ResError      = "error"
	ResDenegado   = "denied"
	ResTiempo     = "timeout"
	ResLimite     = "limit"
)

// ResultadoFinal dice si `s` es un resultado con el que se puede CERRAR una ejecución.
func ResultadoFinal(s string) bool {
	switch s {
	case ResOK, ResError, ResDenegado, ResTiempo, ResLimite:
		return true
	}
	return false
}

// ReCodigoError es la forma del código de error que se audita: corto y nunca texto
// libre (que podría arrastrar contenido del documento o de la base).
var ReCodigoError = regexp.MustCompile(`^[a-z0-9_.-]{0,64}$`)

// ReHash es un SHA-256 en hexadecimal.
var ReHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// HashCodigo es el SHA-256 hexadecimal del código: lo que una aprobación avala.
func HashCodigo(codigo string) string {
	h := sha256.Sum256([]byte(codigo))
	return hex.EncodeToString(h[:])
}

// CodigoValido dice si el código cabe: ni vacío ni mayor que MaxCodigo.
func CodigoValido(codigo string) bool { return codigo != "" && len(codigo) <= MaxCodigo }

// UsoCuota es lo consumido en la última hora.
type UsoCuota struct {
	EjecucionesUsuario int
	EjecucionesOrg     int
	MsUsuario          int64
}

// Motivos por los que no cabe una ejecución más.
const (
	CuotaLibre   = ""
	CuotaUsuario = "usuario"
	CuotaOrg     = "organizacion"
	CuotaTiempo  = "tiempo"
)

// DecidirCuota dice si cabe una ejecución más que reserva `maxMs`: CuotaLibre, o cuál
// de las tres cuotas lo impide.
func DecidirCuota(u UsoCuota, maxMs int) string {
	switch {
	case u.EjecucionesUsuario >= CuotaEjecucionesUsuarioHora:
		return CuotaUsuario
	case u.EjecucionesOrg >= CuotaEjecucionesOrgHora:
		return CuotaOrg
	case u.MsUsuario+int64(maxMs) > CuotaMsUsuarioHora:
		return CuotaTiempo
	}
	return CuotaLibre
}

// PuedeEjecutar decide si `sub` puede ejecutar una versión: el autor siempre la suya
// (para probarla); los demás solo una versión APROBADA de un script compartido.
func PuedeEjecutar(autor, sub string, compartido bool, estado string) bool {
	return sub == autor || (compartido && estado == EstadoAprobada)
}

// Transiciones de estado de una versión: desde cuáles se puede proponer y revisar.
var (
	DesdeProponer = []string{EstadoBorrador, EstadoRechazada}
	DesdeRevisar  = []string{EstadoPendiente}
)
