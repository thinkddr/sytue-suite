// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

// Solo los campos que el CLI renderiza en tabla; el modo -o json usa el cuerpo
// crudo, así que no hace falta espejar el esquema completo.

type project struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Framework     string `json:"framework"`
	GitProvider   string `json:"git_provider"`
	RepoURL       string `json:"repo_url"`
	BuildStrategy string `json:"build_strategy"`
	ProductionRef string `json:"production_ref"`
	InstanceSize  string `json:"instance_size"`
	InstanceCount int    `json:"instance_count"`
	// Ausentes en la respuesta = los defaults del producto (techo automático y
	// 75 % de CPU); projectRows los muestra tal cual.
	AutoscaleMaxCount  int    `json:"autoscale_max_count"`
	AutoscaleCPUTarget int    `json:"autoscale_cpu_target"`
	AppPort            int    `json:"app_port"`
	HealthPath         string `json:"health_path"`
	ReleaseCommand     string `json:"release_command"`
	Paused             bool   `json:"paused"`
	WebhookSecret      string `json:"webhook_secret"`
	WebhookURL         string `json:"webhook_url"`
	CreatedAt          string `json:"created_at"`
}

type envVar struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	IsSecret bool   `json:"is_secret"`
	Scope    string `json:"scope"`
}

type domain struct {
	ID        string `json:"id"`
	Hostname  string `json:"hostname"`
	Status    string `json:"status"`
	IsPrimary bool   `json:"is_primary"`
}

type managedService struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type deployment struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	BuildID   string `json:"build_id"`
	Target    string `json:"target"`
	Status    string `json:"status"`
	CommitSHA string `json:"commit_sha"`
	GitRef    string `json:"git_ref"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	ReadyAt   string `json:"ready_at"`
	Error     string `json:"error"`
}

type invoice struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	PeriodFrom  string  `json:"period_from"`
	PeriodTo    string  `json:"period_to"`
	SubtotalEUR float64 `json:"subtotal_eur"`
	TotalEUR    float64 `json:"total_eur"`
}
