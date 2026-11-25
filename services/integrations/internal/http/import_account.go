// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// DE QUÉ CUENTA ES UNA CONEXIÓN DE LA QUE SE TRAEN DATOS.
//
// Una conexión de Google Workspace o de Microsoft 365 hecha con «Conectar con Google» no
// guardaba de qué cuenta era: el dominio solo existía si alguien lo tecleaba en el
// formulario, y con «Iniciar sesión con Microsoft» el inquilino «se detecta solo» y no se
// guardaba en ningún sitio. Sin eso no había forma de saber que dos conexiones eran la
// misma cuenta, y por eso «Trae tus datos» las enseñaba dos veces (import_origins.go).
//
// Se pregunta al proveedor con el token recién concedido, como hace el consentimiento de
// las conversaciones (applyExternalConsentMetadata): ni se decodifica un id_token sin
// verificar ni se adivina una API.

// Direcciones de las que sale la identidad de la cuenta. Las dos están entre los hosts
// oficiales de su plantilla, así que el egress las deja pasar sin excepciones.
const (
	googleUserinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
	m365OrgURL        = "https://graph.microsoft.com/v1.0/organization?$select=id,verifiedDomains"
)

// importAccountFromProvider devuelve la clave y el rótulo de la cuenta que acaba de
// autorizar la conexión.
//
//   - Google: el dominio de Workspace (`hd`), que es la cuenta de la organización; una
//     cuenta personal no tiene dominio propio y se identifica por su dirección.
//   - Microsoft: el dominio inicial del inquilino (`*.onmicrosoft.com`), que es único y
//     no cambia nunca —el de por defecto lo puede cambiar el administrador—; el rótulo
//     es el dominio por defecto, que es el que la gente reconoce.
func importAccountFromProvider(ctx context.Context, rt connector.Runtime, templateKey string) (key, label string, err error) {
	src, ok := selfImportSource(templateKey)
	if !ok {
		return "", "", nil
	}
	if src == domain.SourceGoogle {
		var who struct {
			Email string `json:"email"`
			HD    string `json:"hd"`
		}
		if err := roomsProviderJSON(ctx, rt, http.MethodGet, googleUserinfoURL, nil, &who); err != nil {
			return "", "", err
		}
		account := strings.ToLower(strings.TrimSpace(who.HD))
		if account == "" {
			account = strings.ToLower(strings.TrimSpace(who.Email))
		}
		if account == "" {
			return "", "", errImportAccountUnknown
		}
		return importAccountKey(src, account), account, nil
	}
	if src == domain.SourceM365 {
		var org m365Organizations
		if err := roomsProviderJSON(ctx, rt, http.MethodGet, m365OrgURL, nil, &org); err != nil {
			return "", "", err
		}
		if len(org.Value) == 0 {
			return "", "", errImportAccountUnknown
		}
		var initial, preferred string
		for _, d := range org.Value[0].Domains {
			name := strings.ToLower(strings.TrimSpace(d.Name))
			if d.IsInitial && initial == "" {
				initial = name
			}
			if d.IsDefault && preferred == "" {
				preferred = name
			}
		}
		account := initial
		if account == "" {
			account = strings.ToLower(strings.TrimSpace(org.Value[0].ID))
		}
		if account == "" {
			return "", "", errImportAccountUnknown
		}
		if preferred == "" {
			preferred = account
		}
		return importAccountKey(src, account), preferred, nil
	}
	return "", "", nil
}

// m365Organizations es la respuesta de `GET /organization` de Microsoft Graph.
//
//nolint:tagliatelle // Microsoft Graph nombra así sus campos (camelCase de OData).
type m365Organizations struct {
	Value []struct {
		ID      string `json:"id"`
		Domains []struct {
			Name      string `json:"name"`
			IsDefault bool   `json:"isDefault"`
			IsInitial bool   `json:"isInitial"`
		} `json:"verifiedDomains"`
	} `json:"value"`
}

var errImportAccountUnknown = errors.New("integrations: el proveedor no dice de qué cuenta es la conexión")

// applyImportAccount recuerda de qué cuenta es una conexión de la que se traen datos y
// frena una SEGUNDA conexión nueva de una cuenta que la organización ya tiene conectada.
//
// Solo se frena una conexión nueva (un borrador que se está autorizando por primera
// vez). Volver a autorizar una que ya existía es repararla, nunca una copia, aunque
// haya otra de la misma cuenta de antes de este control: rechazarlo dejaría la vieja,
// quizá caducada, sin forma de arreglarse. Es la misma regla que las cuentas de correo
// (duplicateMailAccount).
//
// Saber la cuenta mejora la lista pero no es condición para conectar: si el proveedor no
// contesta aquí, el consentimiento sigue adelante sin ella.
//
// egress da el egress de la conexión con el token recién concedido dentro; es una
// función para no fabricarlo en los consentimientos que no son de importación.
func (h *Handlers) applyImportAccount(ctx context.Context, inst *domain.AppInstance, egress func() connector.Runtime, cfg map[string]string) error {
	if _, ok := selfImportSource(inst.TemplateKey); !ok {
		return nil
	}
	cfg[configKeyAuthorizedAt] = time.Now().UTC().Format(time.RFC3339)
	key, label, err := importAccountFromProvider(ctx, egress(), inst.TemplateKey)
	if err != nil || key == "" {
		log.From(ctx).Warn("integrations: no se pudo saber de qué cuenta es la conexión",
			"instance", logsafe.Text(inst.ID), "error", err)
		return nil
	}
	cfg[configKeyImportAccount], cfg[configKeyImportAccountLabel] = key, label
	if inst.Status != domain.StatusDraft {
		return nil
	}
	instances, err := h.store.ListInstances(ctx)
	if err != nil {
		return err
	}
	existing := sameImportAccount(instances, inst.ID, inst.TemplateKey, key)
	if existing == "" {
		return nil
	}
	// El token recién concedido se descarta sin revocarlo: en Google y Microsoft
	// revocarlo revocaría también la concesión de la conexión que ya había.
	if err := h.store.DeleteInstance(ctx, inst.ID); err != nil {
		log.From(ctx).Warn("integrations: borrador de cuenta repetida sin borrar", "instance", logsafe.Text(inst.ID), "error", err)
	}
	return &errAccountExists{instance: existing}
}

// sameImportAccount busca, entre las conexiones de la organización, otra ya hecha (no un
// borrador) de la misma plantilla y la misma cuenta. Una desactivada también cuenta:
// conectar la cuenta otra vez no es la forma de encenderla.
func sameImportAccount(instances []domain.AppInstance, except, templateKey, key string) string {
	if key == "" {
		return ""
	}
	for _, other := range instances {
		if other.ID == except || other.TemplateKey != templateKey || other.Status == domain.StatusDraft {
			continue
		}
		if k, _ := importAccount(other); k == key {
			return other.ID
		}
	}
	return ""
}

// declaredImportAccountTaken es el mismo control al CREAR la conexión, cuando el
// formulario ya dice de qué cuenta es (el dominio de Workspace, el inquilino de Entra):
// se rechaza antes de crear un borrador que acabaría duplicando la cuenta. El código
// `instance_exists` lleva la conexión que ya había, para que la consola la señale.
func declaredImportAccountTaken(instances []domain.AppInstance, templateKey string, cfg json.RawMessage) *httpx.Error {
	if _, ok := selfImportSource(templateKey); !ok {
		return nil
	}
	key, _ := importAccount(domain.AppInstance{TemplateKey: templateKey, Config: cfg})
	existing := sameImportAccount(instances, "", templateKey, key)
	if existing == "" {
		return nil
	}
	name := ""
	for _, inst := range instances {
		if inst.ID == existing {
			name = inst.Name
		}
	}
	return httpx.ErrConflict.WithCode("instance_exists").
		WithMessage("This account is already connected as «{name}». Use that connection, or reconnect it if it stopped working.").
		WithParams(map[string]any{"instance_id": existing, "name": name})
}
