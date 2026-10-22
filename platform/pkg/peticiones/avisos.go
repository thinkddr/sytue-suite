// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package peticiones

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// Avisador avisa a quien puede conceder (y, al conceder, a quien lo pidió). Cualquier
// campo puede estar vacío: sin Hub no hay aviso, sin correo configurado no hay correo,
// y pedir sigue funcionando igual.
type Avisador struct {
	Hub    *hubnotify.Client
	Correo correo.Config
	// IDBaseURL es Sytue ID por dentro del clúster: de ahí sale el correo de quien
	// concede, con la sesión de quien pide (`directory:resolve`, misma organización).
	// El servicio no guarda correos de nadie, y uno que mandara el cliente sería un
	// relé hacia cualquier dirección.
	IDBaseURL string
}

// Aviso es uno a UNA persona.
type Aviso struct {
	Tenant string
	// Para es el `sub` de quien lo recibe.
	Para string
	// Source es la app que lo abre; Type, `<producto>.access.requested|granted`.
	Source, Type string
	// Titulo es el respaldo en inglés; cada cliente lo compone con
	// `notifications.kind.<Type>` y Params (`actor`, `name`).
	Titulo string
	URL    string
	// Recurso va en `params.resource`: la franja de quien lo pidió lo reconoce en el
	// flujo del Hub y se desbloquea sin recargar.
	Recurso string
	Params  map[string]string
	// Token es el de quien llama, para buscar el correo de `Para`. Vacío = sin correo.
	Token string
	// Asunto y Cuerpo del correo. Vacíos = sin correo (el de «concedido» no lo lleva).
	Asunto, Cuerpo string
}

// Avisar deja el aviso en el Hub y, si toca, manda el correo. No bloquea: las dos
// cosas van en segundo plano, con su propio plazo.
func (a Avisador) Avisar(ctx context.Context, av Aviso) {
	if av.Para == "" {
		return
	}
	params := map[string]string{"resource": av.Recurso}
	for k, v := range av.Params {
		params[k] = v
	}
	a.Hub.SendAsync(ctx, hubnotify.Notification{
		Tenant: av.Tenant, Subject: av.Para, Source: av.Source, Type: av.Type,
		Level: "info", Title: av.Titulo, URL: av.URL, Params: params,
	})
	if !a.Correo.Activo() || a.IDBaseURL == "" || av.Token == "" || av.Asunto == "" {
		return
	}
	cfg, base := a.Correo, strings.TrimRight(a.IDBaseURL, "/")
	//nolint:gosec // desacoplado de la petición a propósito: el correo sale aunque ella termine antes
	go func() {
		bctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		para, err := correoDe(bctx, base, av.Token, av.Para)
		if err == nil {
			cuerpo := av.Cuerpo
			if av.URL != "" {
				cuerpo += "\n\n" + av.URL + "\n"
			}
			err = correo.Enviar(bctx, cfg, para, av.Asunto, cuerpo)
		}
		if err != nil {
			log.From(bctx).Warn("peticiones: correo no enviado", "tipo", av.Type, "error", err)
		}
	}()
}

var clienteID = &http.Client{Timeout: 10 * time.Second}

// correoDe resuelve un `sub` a su correo.
func correoDe(ctx context.Context, base, token, sub string) (string, error) {
	email, _, err := Persona(ctx, base, token, sub)
	return email, err
}

// Persona resuelve un `sub` a su correo y su nombre con POST /api/v1/directory:resolve
// de Sytue ID (`base`), con la sesión de quien pregunta: la verja es la organización.
func Persona(ctx context.Context, base, token, sub string) (email, nombre string, err error) {
	cuerpo, _ := json.Marshal(map[string]string{"user_id": sub})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/directory:resolve", bytes.NewReader(cuerpo))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := clienteID.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", "", ErrSinPersona
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", errors.New("sytue id: directory:resolve respondió " + resp.Status)
	}
	var out struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Email == "" {
		return "", "", errors.New("sytue id: directory:resolve sin correo")
	}
	return out.Email, out.Name, nil
}

// PorCorreo es el camino de ida de [Persona]: el `sub` de quien usa `email` en la
// organización de quien pregunta (`directory:resolve`). Fuera de ella, o sin cuenta,
// Sytue ID contesta 404 igual, a propósito, y aquí es [ErrSinPersona].
func PorCorreo(ctx context.Context, base, token, email string) (string, error) {
	cuerpo, _ := json.Marshal(map[string]string{"email": email})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/directory:resolve", bytes.NewReader(cuerpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := clienteID.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusBadRequest:
		return "", ErrSinPersona
	default:
		return "", errors.New("sytue id: directory:resolve respondió " + resp.Status)
	}
	var out struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.UserID == "" {
		return "", errors.New("sytue id: directory:resolve sin identificador")
	}
	return out.UserID, nil
}

// ErrSinPersona: nadie de la organización de quien pregunta es esa persona.
var ErrSinPersona = errors.New("peticiones: nadie de la organización es esa persona")

// Los topes de una llamada a PersonasDeOrg: direcciones de cada clase, candidatos en
// total y por dirección, y `sub` por los que preguntar si son miembros. Los mismos en
// Sytue ID, que rechaza más, y en quien parte las tandas.
const (
	MaxDireccionesDirectorio  = 100
	MaxCandidatosDirectorio   = 200
	MaxCandidatosPorDireccion = 50
	MaxSubsDirectorio         = 1000
)

// DirectorioInternoCierre es cuándo deja de existir la puerta sin sesión de Sytue ID
// (410): hubnotify.CierreReconstruccion, la misma fecha que la ruta histórica del Hub.
var DirectorioInternoCierre = hubnotify.CierreReconstruccion

// ConsultaDirectorio es lo que se le pregunta al directorio interno de Sytue ID.
type ConsultaDirectorio struct {
	// Emails se resuelven a la persona de LA organización que las usa hoy.
	Emails []string `json:"emails,omitempty"`
	// Candidatos, por dirección, son los `sub` que ya tienen permiso sobre algo: la
	// dirección se resuelve contra el directorio GLOBAL (quien colabora desde otra
	// organización) y solo se contesta si sale uno de ellos. Nada que quien pregunta
	// no supiera ya, salvo que esa dirección es de esa persona.
	Candidatos map[string][]string `json:"candidatos,omitempty"`
	// Subs: de estos `sub`, cuáles son HOY miembros activos de la organización (quien se
	// fue conserva sus filas de permiso, pero ya no puede abrir nada de ella).
	Subs []string `json:"subs,omitempty"`
}

// Resueltas es la respuesta: dirección → `sub`.
type Resueltas struct {
	// Miembros son las de Emails: gente de la organización.
	Miembros map[string]string `json:"users"`
	// ConPermiso son las de Candidatos: uno de los candidatos dados, que no es de la
	// organización.
	ConPermiso map[string]string `json:"con_permiso"`
	// Activos son los de Subs que hoy son miembros activos.
	Activos []string `json:"activos"`
}

// PersonasDeOrg pregunta al directorio de Sytue ID SIN sesión: POST
// /internal/directory/resolve con su secreto propio (`sytue-id-directorio`). Solo para
// operaciones de mantenimiento, como reconstruir las menciones de los avisos
// históricos: en una petición normal se usa PorCorreo, con la sesión de quien llama.
func PersonasDeOrg(ctx context.Context, base, secreto, org string, c ConsultaDirectorio) (Resueltas, error) {
	if secreto == "" {
		return Resueltas{}, errors.New("sytue id: falta el secreto del directorio interno")
	}
	cuerpo, _ := json.Marshal(struct {
		Org string `json:"org"`
		ConsultaDirectorio
	}{org, c})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/internal/directory/resolve", bytes.NewReader(cuerpo))
	if err != nil {
		return Resueltas{}, err
	}
	req.Header.Set("Authorization", "Bearer "+secreto)
	req.Header.Set("Content-Type", "application/json")
	resp, err := clienteID.Do(req)
	if err != nil {
		return Resueltas{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Resueltas{}, errors.New("sytue id: /internal/directory/resolve respondió " + resp.Status)
	}
	var out Resueltas
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Resueltas{}, err
	}
	return out, nil
}

// OrganizacionesDe son las organizaciones activas de la persona `sub` y si tiene
// cuenta personal, SIN sesión: POST /internal/people/orgs de Sytue ID con su secreto
// propio. Solo para decidir en el servidor —la política de firmas de fuera, ADR
// 0059—; nunca se le enseña a nadie.
func OrganizacionesDe(ctx context.Context, base, secreto, sub string) (orgs []string, personal bool, err error) {
	if secreto == "" {
		return nil, false, errors.New("sytue id: falta el secreto de las organizaciones de una persona")
	}
	cuerpo, _ := json.Marshal(map[string]string{"user_id": sub})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/internal/people/orgs", bytes.NewReader(cuerpo))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+secreto)
	req.Header.Set("Content-Type", "application/json")
	resp, err := clienteID.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false, errors.New("sytue id: /internal/people/orgs respondió " + resp.Status)
	}
	var out struct {
		Orgs     []string `json:"orgs"`
		Personal bool     `json:"personal"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, false, err
	}
	return out.Orgs, out.Personal, nil
}

// DeFuera encuentra a alguien de CUALQUIER organización por su @usuario o su dirección
// entera, con POST /api/v1/people:resolve de Sytue ID (ADR 0040) y la sesión de quien
// pregunta, que es la que paga el tope por persona de Sytue ID. Devuelve su `sub` y
// cómo se le enseña (nombre, o @usuario); NUNCA su correo ni su organización, que
// Sytue ID no da a propósito. Sin nadie así, [ErrSinPersona].
func DeFuera(ctx context.Context, base, token, query string) (sub, nombre string, err error) {
	cuerpo, _ := json.Marshal(map[string]string{"query": query})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/people:resolve", bytes.NewReader(cuerpo))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := clienteID.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusBadRequest:
		return "", "", ErrSinPersona
	default:
		return "", "", errors.New("sytue id: people:resolve respondió " + resp.Status)
	}
	var out struct {
		UserID   string `json:"user_id"`
		Name     string `json:"name"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.UserID == "" {
		return "", "", errors.New("sytue id: people:resolve sin identificador")
	}
	nombre = strings.TrimSpace(out.Name)
	if nombre == "" && out.Username != "" {
		nombre = "@" + out.Username
	}
	return out.UserID, nombre, nil
}
