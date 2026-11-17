// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain preserves Cast service types while the domain is also usable
// on authorized devices.
package domain

import "git.sytue.eu/sytue/one/apps/nucleo/cast/model"

type (
	SinCuota   = model.SinCuota
	Kind       = model.Kind
	Status     = model.Status
	Visibility = model.Visibility
	LiveSource = model.LiveSource
	Video      = model.Video
	Persona    = model.Persona
	Cambios    = model.Cambios
	JobKind    = model.JobKind
	JobStatus  = model.JobStatus
	Job        = model.Job
)

const (
	KindUpload         = model.KindUpload
	KindLive           = model.KindLive
	StatusUploading    = model.StatusUploading
	StatusProcessing   = model.StatusProcessing
	StatusReady        = model.StatusReady
	StatusFailed       = model.StatusFailed
	StatusScheduled    = model.StatusScheduled
	StatusLive         = model.StatusLive
	StatusEnded        = model.StatusEnded
	VisibilityPublic   = model.VisibilityPublic
	VisibilityUnlisted = model.VisibilityUnlisted
	VisibilityOrg      = model.VisibilityOrg
	VisibilityPeople   = model.VisibilityPeople
	LiveSourceRTMP     = model.LiveSourceRTMP
	LiveSourceWebRTC   = model.LiveSourceWebRTC
	MaxTitulo          = model.MaxTitulo
	MaxDescripcion     = model.MaxDescripcion
	MaxPersonas        = model.MaxPersonas
	MaxOriginalBytes   = model.MaxOriginalBytes
	MaxPosterBytes     = model.MaxPosterBytes
	JobUpload          = model.JobUpload
	JobLiveVOD         = model.JobLiveVOD
	JobPoster          = model.JobPoster
	JobQualities       = model.JobQualities
	JobPending         = model.JobPending
	JobRunning         = model.JobRunning
	JobDone            = model.JobDone
	JobFailed          = model.JobFailed
	MaxIntentos        = model.MaxIntentos
	Huerfano           = model.Huerfano
)

var (
	ErrNoExiste        = model.ErrNoExiste
	ErrProhibido       = model.ErrProhibido
	ErrEntradaInvalida = model.ErrEntradaInvalida
	ErrConflicto       = model.ErrConflicto
	ErrDemasiadoGrande = model.ErrDemasiadoGrande
	ErrSinCuota        = model.ErrSinCuota
	PuedePasar         = model.PuedePasar
	EstadoInicial      = model.EstadoInicial
	Titulo             = model.Titulo
	Descripcion        = model.Descripcion
)

// TipoDe is shared by the server, retained source and authorized devices.
var TipoDe = model.TipoDe
