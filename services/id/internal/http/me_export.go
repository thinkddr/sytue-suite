// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
)

// Filas de la entrega de la cuenta. Tipos propios y no los del dominio a
// propósito: domain.User lleva el hash de la contraseña y una credencial WebAuthn
// su clave pública, y aquí solo sale lo que la persona reconoce como suyo.
type (
	perfilExportado struct {
		ID            string    `json:"id"`
		Email         string    `json:"email"`
		EmailVerified bool      `json:"email_verified"`
		Name          string    `json:"name"`
		GivenName     string    `json:"given_name"`
		FamilyName    string    `json:"family_name"`
		Title         string    `json:"title"`
		Department    string    `json:"department"`
		Location      string    `json:"location"`
		Locale        string    `json:"locale"`
		MFAEnabled    bool      `json:"mfa_enabled"`
		CreatedAt     time.Time `json:"created_at"`
	}
	organizacionExportada struct {
		Name   string `json:"name"`
		Slug   string `json:"slug"`
		Role   string `json:"role"`
		Status string `json:"status"`
		Kind   string `json:"kind"`
	}
	sesionExportada struct {
		UserAgent string    `json:"user_agent"`
		AMR       []string  `json:"amr"`
		AuthTime  time.Time `json:"auth_time"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	dispositivoExportado struct {
		Name      string    `json:"name"`
		UserAgent string    `json:"user_agent"`
		Trusted   bool      `json:"trusted"`
		LastIP    string    `json:"last_ip"`
		FirstSeen time.Time `json:"first_seen"`
		LastSeen  time.Time `json:"last_seen"`
	}
	passkeyExportada struct {
		Name       string     `json:"name"`
		Transports []string   `json:"transports"`
		CreatedAt  time.Time  `json:"created_at"`
		LastUsedAt *time.Time `json:"last_used_at"`
	}
)

// handleMeExport entrega la cuenta para «Exportar tus datos» (GET
// /api/v1/me/export): perfil, organizaciones, sesiones abiertas, dispositivos,
// llaves de acceso y contraseñas de aplicación (su nombre y sus fechas, nunca el
// secreto, que ni siquiera se guarda).
func (s *Server) handleMeExport(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	p, err := portabilidad.Nuevo(w, r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}
	if err := s.exportarCuenta(r.Context(), c.Subject, p); err != nil {
		_ = p.Fallo(err)
		return
	}
	_ = p.Cerrar()
}

func (s *Server) exportarCuenta(ctx context.Context, sub string, p *portabilidad.Paquete) error {
	u, err := s.store.GetUserByID(ctx, sub)
	if err != nil {
		return err
	}
	perfil := []perfilExportado{{
		ID: u.ID, Email: u.Email, EmailVerified: u.EmailVerified, Name: u.Name, GivenName: u.GivenName,
		FamilyName: u.FamilyName, Title: u.Title, Department: u.Department, Location: u.Location,
		Locale: u.Locale, MFAEnabled: u.MFAEnabled, CreatedAt: u.CreatedAt,
	}}
	if err := portabilidad.Tabla(p, "profile", "profile", portabilidad.Todas(perfil)); err != nil {
		return err
	}

	mems, err := s.store.ListOrgMembershipsForUser(ctx, sub)
	if err != nil {
		return err
	}
	orgs := make([]organizacionExportada, 0, len(mems))
	for _, m := range mems {
		orgs = append(orgs, organizacionExportada{
			Name: m.Name, Slug: m.Slug, Role: string(m.Role), Status: string(m.Status), Kind: string(m.Kind),
		})
	}
	if err := portabilidad.Tabla(p, "organizations", "organization", portabilidad.Todas(orgs)); err != nil {
		return err
	}

	sesiones, err := s.store.ListSessionsForUser(ctx, sub, "")
	if err != nil {
		return err
	}
	ses := make([]sesionExportada, 0, len(sesiones))
	for _, x := range sesiones {
		ses = append(ses, sesionExportada{UserAgent: x.UserAgent, AMR: x.AMR, AuthTime: x.AuthTime, ExpiresAt: x.ExpiresAt})
	}
	if err := portabilidad.Tabla(p, "sessions", "session", portabilidad.Todas(ses)); err != nil {
		return err
	}

	return s.exportarCredenciales(ctx, sub, p)
}

func (s *Server) exportarCredenciales(ctx context.Context, sub string, p *portabilidad.Paquete) error {
	devs, err := s.store.ListDevicesForUser(ctx, sub)
	if err != nil {
		return err
	}
	dispositivos := make([]dispositivoExportado, 0, len(devs))
	for _, d := range devs {
		dispositivos = append(dispositivos, dispositivoExportado{
			Name: d.Name, UserAgent: d.UserAgent, Trusted: d.Trusted, LastIP: d.LastIP, FirstSeen: d.FirstSeen, LastSeen: d.LastSeen,
		})
	}
	if err := portabilidad.Tabla(p, "devices", "device", portabilidad.Todas(dispositivos)); err != nil {
		return err
	}

	creds, err := s.store.ListCredentialsForUser(ctx, sub)
	if err != nil {
		return err
	}
	llaves := make([]passkeyExportada, 0, len(creds))
	for _, k := range creds {
		llaves = append(llaves, passkeyExportada{Name: k.Name, Transports: k.Transports, CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt})
	}
	if err := portabilidad.Tabla(p, "passkeys", "passkey", portabilidad.Todas(llaves)); err != nil {
		return err
	}

	claves, err := s.store.ListAppPasswords(ctx, sub)
	if err != nil {
		return err
	}
	return portabilidad.Tabla(p, "app_passwords", "app_password", portabilidad.Todas(claves))
}
