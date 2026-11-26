// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain es el modelo de Sytue Bases tal y como lo usa el servicio.
//
// Lo puro (tipos, validación de valores, fórmulas, vistas, CSV, adjuntos, comentarios,
// permisos y tiempo real) vive en `apps/nucleo/bases/dominio`, porque Sytue Office lo
// necesita en el dispositivo (PLAN-EDICION-LOCAL.md §6.12, B-01) y no se copia
// (R-NO-COPIAR). Aquí se reexporta con los mismos nombres, así que el servicio sigue
// escribiendo `domain.X`; lo que solo es del servicio (los scripts, que dependen de
// `platform/pkg/guiones`) se queda en este paquete.
package domain

import "git.sytue.eu/sytue/one/apps/nucleo/bases/dominio"

type (
	Accion            = dominio.Accion
	Actor             = dominio.Actor
	Adjunto           = dominio.Adjunto
	Base              = dominio.Base
	Cambio            = dominio.Cambio
	Campo             = dominio.Campo
	Comentario        = dominio.Comentario
	ConfigVista       = dominio.ConfigVista
	Cursor            = dominio.Cursor
	Entrada           = dominio.Entrada
	EntradaACL        = dominio.EntradaACL
	EntradaCampo      = dominio.EntradaCampo
	Filtro            = dominio.Filtro
	Formula           = dominio.Formula
	JuegoDeVistas     = dominio.JuegoDeVistas
	Orden             = dominio.Orden
	Permiso           = dominio.Permiso
	Presente          = dominio.Presente
	RefFormula        = dominio.RefFormula
	Registro          = dominio.Registro
	RegistroExportado = dominio.RegistroExportado
	Rol               = dominio.Rol
	Tabla             = dominio.Tabla
	TablaCSV          = dominio.TablaCSV
	Tipo              = dominio.Tipo
	TipoVista         = dominio.TipoVista
	VectorDeVista     = dominio.VectorDeVista
	Vista             = dominio.Vista
)

// Los operadores de fecha relativa de los filtros (vistas.go de dominio).
const (
	OpHoy            = dominio.OpHoy
	OpEsteMes        = dominio.OpEsteMes
	OpUltimosDias    = dominio.OpUltimosDias
	OpProximosDia    = dominio.OpProximosDia
	MaxDiasRelativos = dominio.MaxDiasRelativos
)

const (
	AccAuditoria              = dominio.AccAuditoria
	AccBorrarBase             = dominio.AccBorrarBase
	AccComentar               = dominio.AccComentar
	AccConector               = dominio.AccConector
	AccEscribirReg            = dominio.AccEscribirReg
	AccEsquema                = dominio.AccEsquema
	AccFormulario             = dominio.AccFormulario
	AccGestionVistas          = dominio.AccGestionVistas
	AccLeer                   = dominio.AccLeer
	AccPermisos               = dominio.AccPermisos
	AccRevisarScripts         = dominio.AccRevisarScripts
	AccTransferir             = dominio.AccTransferir
	MaxAdjuntosPorCampo       = dominio.MaxAdjuntosPorCampo
	MaxBasesPorOrg            = dominio.MaxBasesPorOrg
	MaxBytesAdjunto           = dominio.MaxBytesAdjunto
	MaxBytesAdjuntosPorBase   = dominio.MaxBytesAdjuntosPorBase
	MaxBytesCSV               = dominio.MaxBytesCSV
	MaxCambiosPorTanda        = dominio.MaxCambiosPorTanda
	MaxCamposPorTabla         = dominio.MaxCamposPorTabla
	MaxClaveOrden             = dominio.MaxClaveOrden
	MaxColumnasCSV            = dominio.MaxColumnasCSV
	MaxComentariosPorRegistro = dominio.MaxComentariosPorRegistro
	MaxCursor                 = dominio.MaxCursor
	MaxEnlaces                = dominio.MaxEnlaces
	MaxFilasCSV               = dominio.MaxFilasCSV
	MaxFiltros                = dominio.MaxFiltros
	MaxFormulasPorTabla       = dominio.MaxFormulasPorTabla
	MaxGruposDelActor         = dominio.MaxGruposDelActor
	MaxLongComentario         = dominio.MaxLongComentario
	MaxLongCorreo             = dominio.MaxLongCorreo
	MaxLongFormula            = dominio.MaxLongFormula
	MaxLongNombre             = dominio.MaxLongNombre
	MaxLongNombreAdjunto      = dominio.MaxLongNombreAdjunto
	MaxLongOpcion             = dominio.MaxLongOpcion
	MaxLongResultado          = dominio.MaxLongResultado
	MaxLongTelefono           = dominio.MaxLongTelefono
	MaxLongTexto              = dominio.MaxLongTexto
	MaxLongURL                = dominio.MaxLongURL
	MaxNodosFormula           = dominio.MaxNodosFormula
	MaxOpciones               = dominio.MaxOpciones
	MaxOrden                  = dominio.MaxOrden
	MaxPagina                 = dominio.MaxPagina
	MaxPermisosPorBase        = dominio.MaxPermisosPorBase
	MaxPresentes              = dominio.MaxPresentes
	MaxRegistrosPorTabla      = dominio.MaxRegistrosPorTabla
	MaxTablasPorBase          = dominio.MaxTablasPorBase
	MaxTamRegistro            = dominio.MaxTamRegistro
	MaxValorAuditado          = dominio.MaxValorAuditado
	MaxValoracion             = dominio.MaxValoracion
	MaxVistasPorTabla         = dominio.MaxVistasPorTabla
	MonedaPorDefecto          = dominio.MonedaPorDefecto
	PaginaPorDefecto          = dominio.PaginaPorDefecto
	PrincipalExterno          = dominio.PrincipalExterno
	PrincipalGrupo            = dominio.PrincipalGrupo
	PrincipalOrganizacion     = dominio.PrincipalOrganizacion
	PrincipalUsuario          = dominio.PrincipalUsuario
	RolComentarista           = dominio.RolComentarista
	RolEditor                 = dominio.RolEditor
	RolLector                 = dominio.RolLector
	RolPropietario            = dominio.RolPropietario
	TipoAdjunto               = dominio.TipoAdjunto
	TipoCasilla               = dominio.TipoCasilla
	TipoCorreo                = dominio.TipoCorreo
	TipoFecha                 = dominio.TipoFecha
	TipoFormula               = dominio.TipoFormula
	TipoMoneda                = dominio.TipoMoneda
	TipoMultiple              = dominio.TipoMultiple
	TipoNumero                = dominio.TipoNumero
	TipoRelacion              = dominio.TipoRelacion
	TipoSeleccion             = dominio.TipoSeleccion
	TipoTelefono              = dominio.TipoTelefono
	TipoTexto                 = dominio.TipoTexto
	TipoURL                   = dominio.TipoURL
	TipoValoracion            = dominio.TipoValoracion
	VersionTiempoReal         = dominio.VersionTiempoReal
	VidaPresencia             = dominio.VidaPresencia
	VistaFormulario           = dominio.VistaFormulario
	VistaGaleria              = dominio.VistaGaleria
	VistaKanban               = dominio.VistaKanban
	VistaRejilla              = dominio.VistaRejilla
)

var (
	ErrConflicto    = dominio.ErrConflicto
	ErrInvalido     = dominio.ErrInvalido
	ErrLimite       = dominio.ErrLimite
	ErrNoExiste     = dominio.ErrNoExiste
	ErrReferenciado = dominio.ErrReferenciado
	ErrSinPermiso   = dominio.ErrSinPermiso
)

// Las funciones, como variables: se llaman igual (`domain.F(x)`).
var (
	Acotar                = dominio.Acotar
	AnalizarFormula       = dominio.AnalizarFormula
	CabeceraExportada     = dominio.CabeceraExportada
	CambioParaElFlujo     = dominio.CambioParaElFlujo
	CeldaCSV              = dominio.CeldaCSV
	CeldaExportada        = dominio.CeldaExportada
	ConRetiradas          = dominio.ConRetiradas
	ConexionValida        = dominio.ConexionValida
	DiasRelativos         = dominio.DiasRelativos
	Efectivos             = dominio.Efectivos
	EtiquetaDe            = dominio.EtiquetaDe
	Escribe               = dominio.Escribe
	EvaluarFormulas       = dominio.EvaluarFormulas
	FilaExportada         = dominio.FilaExportada
	FiltrarYOrdenar       = dominio.FiltrarYOrdenar
	InferirTipo           = dominio.InferirTipo
	LeerCSV               = dominio.LeerCSV
	LeerCursor            = dominio.LeerCursor
	LimpiarComentario     = dominio.LimpiarComentario
	LimpiarNombre         = dominio.LimpiarNombre
	NombreDeAdjunto       = dominio.NombreDeAdjunto
	NombresDeAdjuntos     = dominio.NombresDeAdjuntos
	Permite               = dominio.Permite
	PuedeBorrarComentario = dominio.PuedeBorrarComentario
	ResolverRol           = dominio.ResolverRol
	TamanoDatos           = dominio.TamanoDatos
	TipoAlServir          = dominio.TipoAlServir
	TipoDeContenido       = dominio.TipoDeContenido
	ValidarConfig         = dominio.ValidarConfig
	ValidarDatos          = dominio.ValidarDatos
	ValidarEnlaces        = dominio.ValidarEnlaces
	ValidarEntradaCampo   = dominio.ValidarEntradaCampo
	ValidarOpciones       = dominio.ValidarOpciones
	ValidarParcial        = dominio.ValidarParcial
	ValidarValor          = dominio.ValidarValor
	ValoresDeEnlaces      = dominio.ValoresDeEnlaces
	VectoresDeVista       = dominio.VectoresDeVista
)
