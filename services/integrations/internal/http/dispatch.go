// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/graph"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// resolveCapability devuelve el driver que sirve una capacidad de la instancia, junto
// con la instancia lista para entregársela: con la config que ese driver necesita del
// manifiesto (la raíz SCIM, por ejemplo) ya resuelta.
//
// Antes cada handler hacía `registry.Get(inst.TemplateKey)`, es decir, buscaba el driver
// por la CLAVE DE LA PLANTILLA. Eso obligaba a que cada instancia tuviera un único
// driver, cuando el manifiesto lleva desde el principio dos capacidades independientes
// (`provisioning` y `data`) cuyos valores SON claves del registro. Consecuencia: el
// aprovisionamiento no funcionaba en ninguna plantilla del catálogo, y las tres que
// resolvían por alias lo hacían al driver de datos, que no sabe aprovisionar.
func (h *Handlers) resolveCapability(ctx context.Context, templateKey string, cfg map[string]string, capab manifest.Capability) (connector.Driver, map[string]string, error) {
	tpl, err := h.store.GetTemplate(ctx, templateKey)
	if err != nil {
		return nil, nil, err
	}
	m, err := manifest.Parse(tpl.Manifest)
	if err != nil {
		return nil, nil, err
	}
	kind := m.DriverFor(capab)
	if kind == "" {
		return nil, nil, fmt.Errorf("%w: la plantilla %q no ofrece %s", manifest.ErrNoCapability, templateKey, capab)
	}
	drv, ok := h.registry.Get(kind)
	if !ok {
		// El test de las semillas impide que esto llegue a producción: toda capacidad
		// declarada resuelve a un driver registrado, o el catálogo no carga.
		return nil, nil, fmt.Errorf("%w: no hay driver %q para %s", domain.ErrInvalidInput, kind, capab)
	}

	extra, err := m.CapabilityConfig(capab, cfg)
	if err != nil {
		return nil, nil, err
	}
	if len(extra) == 0 {
		return drv, cfg, nil
	}
	merged := make(map[string]string, len(cfg)+len(extra))
	for k, v := range cfg {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return drv, merged, nil
}

// --- La PROPIA suite como origen ---------------------------------------------

const (
	// suiteTemplateKey es la clave de la plantilla de la propia suite: los MISMOS
	// adaptadores que hablan con Microsoft 365 apuntados a los endpoints /compat de
	// Sytue Drive (que sirve el dialecto de Graph). Es lo que distingue un destino
	// interno de la suite de un tercero.
	suiteTemplateKey = "sytue"
	// suiteAudience es el client de la suite al que va dirigido el token canjeado: su
	// verificador solo acepta tokens con SU audiencia. La capacidad de datos de esta
	// plantilla la sirve Sytue Drive.
	suiteAudience = "sytue-drive"
	// suiteGraphPath es el prefijo bajo el que Sytue Drive habla el dialecto de Graph.
	suiteGraphPath = "/compat/graph"
	// suiteTokenPath es el token endpoint de Sytue ID (el del canje RFC 8693).
	suiteTokenPath = "/oauth2/token" //nolint:gosec // G101: es una ruta, no una credencial.
	// suiteClientSecret es la clave del vault con la que el driver graph recibe el
	// secreto de cliente; debe coincidir con `secretClientSecret` de
	// internal/driver/graph (mismo criterio que manifest.ConfigSCIMBaseURL).
	suiteClientSecret = "client_secret"
)

// loadInstance carga la instancia con sus secretos y, si es de la plantilla de la PROPIA
// suite, le INYECTA la configuración que activa ese camino en el driver.
//
// Es el punto ÚNICO de despacho: los cuatro handlers que fabrican un Runtime —probar la
// conexión, consulta tabular, embed y búsqueda federada— cargan por aquí, de modo que la
// inyección no puede quedarse en tres de cuatro.
//
// Por qué la pone el SERVICIO y no un administrador: los destinos salen de la
// configuración del servicio (ID_URL / DRIVE_URL) y las credenciales, del client
// DELEGABLE de la organización —el que instala el operador de plataforma, nunca la
// consola—. Eso es justo lo que permite tratar el destino como interno de la suite
// (policy.RuntimeParams.Internal): un valor tecleado jamás llega aquí, y la inyección
// PISA lo que hubiera en la config de la instancia.
//
// Del administrador solo sale la persona cuyo Drive se lee (campo `subject` de la
// plantilla): /compat/graph es /me, así que el token va acotado a ELLA. Poder elegirla es
// la misma capacidad que ya concede la delegación a la migración de contenido.
func (h *Handlers) loadInstance(ctx context.Context, id string) (connector.Instance, error) {
	inst, err := h.store.LoadInstanceWithSecrets(ctx, id)
	if err != nil || inst.TemplateKey != suiteTemplateKey {
		return inst, err
	}
	// El MISMO client delegable con el que la migración escribe en la cuenta de cada
	// persona (mig 0006, sellado con la KEK). No se duplica el mecanismo: se reusa.
	del, err := h.store.DelegatedClient(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return connector.Instance{}, httpx.NewError(http.StatusServiceUnavailable, "no_delegation",
				"Esta organización todavía no puede leer datos de la propia suite: falta que el operador de Sytue aprovisione su permiso de delegación.")
		}
		return connector.Instance{}, fmt.Errorf("leer el client delegable de la organización: %w", err)
	}
	inst.Config[graph.ConfigTokenURL] = strings.TrimRight(h.idBaseURL, "/") + suiteTokenPath
	inst.Config[graph.ConfigGraphBase] = strings.TrimRight(h.driveBaseURL, "/") + suiteGraphPath
	inst.Config[graph.ConfigClientID] = del.ClientID
	inst.Config[graph.ConfigAudience] = suiteAudience
	inst.Secrets[suiteClientSecret] = del.Secret
	return inst, nil
}

// egressRuntime fabrica el Runtime de una instancia: el ÚNICO cliente de egress, con la
// cadena completa de políticas. Los cuatro puntos de despacho lo construían con el mismo
// bloque copiado; vive aquí para que `Internal` —lo que decide si el destino puede ser
// una IP privada— se decida en UN sitio, y no se olvide (o se ponga de más) en uno.
func (h *Handlers) egressRuntime(inst connector.Instance, logger *slog.Logger) connector.Runtime {
	return policy.BuildRuntime(policy.RuntimeParams{
		OrgID:      inst.OrgID,
		InstanceID: inst.ID,
		// El Bearer convencional sale del vault y lo inyecta la pasarela; el driver no lo ve.
		Bearer:    inst.Secrets[store.SecretKeyBearer],
		Limiter:   h.limiter,
		Audit:     h.audit,
		Policy:    h.engine,
		APIFamily: inst.TemplateKey,
		// SOLO la plantilla de la propia suite: su destino sale de la configuración del
		// servicio (ver loadInstance), nunca de un valor que teclee un administrador.
		Internal:     inst.TemplateKey == suiteTemplateKey,
		Log:          logger,
		Timeout:      h.egressTimeout,
		MaxRedirects: h.egressMaxRedirect,
	})
}

// capabilityError traduce el fallo de resolución a una respuesta que el administrador
// entiende: que una aplicación no haga algo NO es una avería del servicio.
func capabilityError(err error, what string) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, manifest.ErrNoCapability):
		return httpx.ErrBadRequest.WithMessage(fmt.Sprintf("This app does not support %s from Sytue.", what))
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrBadRequest.WithMessage("The template for this connection is no longer in the catalogue.")
	default:
		return toAPIError(err)
	}
}

// gateCapability decide si una instancia puede EJERCER una capacidad hacia el tercero.
// Es la verja que faltaba: hasta ahora ni `QueryData` ni `PostSync` miraban el estado ni
// las capacidades habilitadas, así que DESACTIVAR un conector era cosmético —seguía
// sirviendo consultas de datos y seguía aprovisionando personas en el SaaS— y los
// interruptores de capacidad que el administrador ve en la consola no gateaban nada:
// `level_enabled` solo aparecía en un Scan de la fila.
//
// Se aplica a lo que SALE hacia el tercero, no a `TestInstance`: probar la conexión
// antes de activarla es justo para lo que existe, y probar una capacidad que has
// apagado es una comprobación legítima de diagnóstico.
// Consulta la instancia de dominio (`connector.Instance` es la vista del driver y no
// lleva ni estado ni capacidades, y meterlos ahí filtraría política a los drivers). Es
// un SELECT local por clave primaria frente a una llamada de red a un tercero: no se
// nota.
func (h *Handlers) gateCapability(ctx context.Context, id string, capab manifest.Capability) error {
	inst, err := h.store.GetInstance(ctx, id)
	if err != nil {
		return toAPIError(err)
	}
	if inst.Status != domain.StatusActive {
		return httpx.NewError(http.StatusConflict, "instance_inactive",
			"Esta conexión no está activa. Actívala para que vuelva a hablar con la aplicación.")
	}
	if !capabilityEnabled(inst.LevelEnabled, capab) {
		return httpx.NewError(http.StatusForbidden, "capability_disabled",
			"Esta capacidad está desactivada para esta conexión.")
	}
	return nil
}

// capabilityEnabled resuelve la lista de capacidades habilitadas.
//
// Una lista VACÍA se trata como «todas», y es deliberado pero incómodo: la columna nace
// con DEFAULT '{}' y todas las instancias anteriores a que esto gateara nada la tienen
// así, de modo que interpretarla como «ninguna» dejaría mudos a todos los conectores ya
// creados. El precio es que un administrador que desmarque TODAS las capacidades acaba
// con una lista vacía y las recupera todas.
//
// ponytail: el arreglo de verdad es rellenar la lista al crear la instancia con las
// capacidades que declara su plantilla y hacer un backfill de las filas viejas; a
// partir de ahí, vacío significa «ninguna» sin ambigüedad. Mientras tanto la consola
// no ofrece desmarcarlas todas.
func capabilityEnabled(enabled []string, capab manifest.Capability) bool {
	if len(enabled) == 0 {
		return true
	}
	return slices.Contains(enabled, string(capab))
}
