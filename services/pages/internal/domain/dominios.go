// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"time"

	"github.com/google/uuid"
)

// EstadoDominio representa el estado de verificación y activación de un dominio propio.
type EstadoDominio string

const (
	DomainStatusPending  EstadoDominio = "pending"
	DomainStatusVerified EstadoDominio = "verified"
	DomainStatusActive   EstadoDominio = "active"
	DomainStatusFailed   EstadoDominio = "failed"
)

// Domain representa un dominio propio asociado a un sitio.
type Domain struct {
	ID            uuid.UUID     `json:"id"`
	Tenant        string        `json:"tenant"`
	SiteID        uuid.UUID     `json:"site_id"`
	Host          string        `json:"host"`
	ParentID      *uuid.UUID    `json:"parent_id,omitempty"`
	Token         string        `json:"token"`
	Status        EstadoDominio `json:"status"`
	Detail        string        `json:"detail"`
	CheckedAt     *time.Time    `json:"checked_at,omitempty"`
	VerifiedAt    *time.Time    `json:"verified_at,omitempty"`
	ActivatedAt   *time.Time    `json:"activated_at,omitempty"`
	RevalidatedAt *time.Time    `json:"revalidated_at,omitempty"`
	CreatedBy     string        `json:"created_by"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// TXT contiene las instrucciones del registro DNS TXT para la verificación.
type TXT struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Target contiene las instrucciones de enrutado CNAME o registro A.
type Target struct {
	CNAME string `json:"cname"`
	A     string `json:"a"`
	// Registro recomendado para este host, calculado por el servicio.
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DominioDTO representa la respuesta de la API con los datos de un dominio y sus instrucciones.
type DominioDTO struct {
	ID          uuid.UUID     `json:"id"`
	Host        string        `json:"host"`
	ParentID    *uuid.UUID    `json:"parent_id,omitempty"`
	Status      EstadoDominio `json:"status"`
	Detail      string        `json:"detail"`
	Primary     bool          `json:"primary"`
	TXT         TXT           `json:"txt"`
	Target      Target        `json:"target"`
	CheckedAt   *time.Time    `json:"checked_at,omitempty"`
	VerifiedAt  *time.Time    `json:"verified_at,omitempty"`
	ActivatedAt *time.Time    `json:"activated_at,omitempty"`
}

// AbuseReason representa el motivo de una denuncia de abuso.
type AbuseReason string

const (
	AbuseReasonPhishing  AbuseReason = "phishing"
	AbuseReasonMalware   AbuseReason = "malware"
	AbuseReasonIllegal   AbuseReason = "illegal"
	AbuseReasonCopyright AbuseReason = "copyright"
	AbuseReasonOther     AbuseReason = "other"
)

// AbuseReportStatus representa el estado de tramitación de una denuncia de abuso.
type AbuseReportStatus string

const (
	AbuseReportStatusOpen      AbuseReportStatus = "open"
	AbuseReportStatusActioned  AbuseReportStatus = "actioned"
	AbuseReportStatusDismissed AbuseReportStatus = "dismissed"
)

// AbuseReport representa una denuncia de contenido o abuso sobre un sitio.
type AbuseReport struct {
	ID            uuid.UUID         `json:"id"`
	SiteID        uuid.UUID         `json:"site_id"`
	Host          string            `json:"host"`
	Path          string            `json:"path"`
	Reason        AbuseReason       `json:"reason"`
	Details       string            `json:"details"`
	ReporterEmail string            `json:"reporter_email"`
	Status        AbuseReportStatus `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
}
