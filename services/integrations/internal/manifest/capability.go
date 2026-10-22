// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package manifest

import (
	"fmt"
	"strings"
)

// Capability es una de las cosas que una conexión puede hacer. Cada una la sirve un
// driver DISTINTO, y una misma instancia puede tener varias: Jira aprovisiona por SCIM
// (driver "scim") y trae datos por JQL (driver "jira") con credenciales diferentes.
type Capability string

const (
	// CapProvision — altas y bajas de personas en el SaaS (SCIM 2.0 saliente).
	CapProvision Capability = "provisioning"
	// CapData — leer datos tabulares del SaaS (a Sytue Sheets) y buscarlos desde el Hub.
	CapData Capability = "data"
)

// ErrNoCapability indica que la plantilla no declara esa capacidad. Es información, no
// avería: significa «esta aplicación no hace eso», y el llamante responde 400 con un
// mensaje que lo dice en llano.
var ErrNoCapability = fmt.Errorf("%w: la aplicación no declara esa capacidad", ErrInvalid)

// DriverFor devuelve el Kind del driver que sirve la capacidad, o "" si la plantilla no
// la declara.
//
// Este método es el arreglo de un defecto de fondo: los puntos de despacho resolvían el
// driver por `template_key`, así que una instancia solo podía tener UN driver. El
// manifiesto ya decía la verdad —`provisioning: "scim"` y `data: "jira"` SON claves del
// registro— pero nadie la leía: el aprovisionamiento no funcionaba en ninguna plantilla
// del catálogo y las que sí resolvían lo hacían al driver equivocado.
func (m *Manifest) DriverFor(capab Capability) string {
	switch capab {
	case CapProvision:
		if m.provisioningKind() == ProvisioningSCIM {
			return m.Provisioning
		}
		return ""
	case CapData:
		return strings.TrimSpace(m.Data)
	default:
		return ""
	}
}

// Capabilities lista las capacidades que la plantilla declara, en orden estable.
func (m *Manifest) Capabilities() []Capability {
	var out []Capability
	for _, c := range []Capability{CapProvision, CapData} {
		if m.DriverFor(c) != "" {
			out = append(out, c)
		}
	}
	return out
}

// CapabilityConfig devuelve la configuración que el driver de esa capacidad necesita y
// que NO viene del admin, sino del propio manifiesto: hoy, la raíz SCIM con sus
// plantillas {campo} ya resueltas contra la config de la instancia.
//
// Se resuelve en el momento del despacho, no al activar: así una instancia creada antes
// de que existiera este campo funciona sin migrarla, y si la plantilla corrige la URL
// del SaaS, la corrección aplica sin tocar las instancias.
func (m *Manifest) CapabilityConfig(capab Capability, values map[string]string) (map[string]string, error) {
	if capab != CapProvision || m.DriverFor(CapProvision) == "" {
		//nolint:nilnil // (nil, nil) ES el contrato: esta capacidad no necesita config extra del manifiesto.
		return nil, nil
	}
	base, err := render(m.SCIMBaseURL, values)
	if err != nil {
		return nil, err
	}
	return map[string]string{ConfigSCIMBaseURL: base}, nil
}

// ConfigSCIMBaseURL es la clave con la que el driver SCIM recibe su raíz (debe coincidir
// con scimout.ConfigBaseURL). Lleva '_' inicial a propósito: `fieldKeyRe` prohíbe ese
// prefijo en los campos de admin, así que nunca puede colisionar con un valor que
// rellene un administrador.
const ConfigSCIMBaseURL = "_scim_base_url"
