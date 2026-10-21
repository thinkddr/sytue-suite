// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package scimout

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

// Kind es la clave del Registry con la que se registra este driver. Las plantillas
// cuyo template_key sea "scim" (o que mapeen a él) usan este aprovisionador genérico.
const Kind = "scim"

// ConfigBaseURL es la clave de Instance.Config con la URL base SCIM del SaaS
// (p. ej. https://saas.example.com/scim/v2). La aporta el manifiesto de la plantilla
// —resuelta contra los valores del admin— en el momento del despacho, no el propio
// admin: por eso lleva '_' inicial, prefijo que las claves de campo tienen prohibido y
// que garantiza que nadie la sobrescriba desde la configuración de la instancia.
const ConfigBaseURL = "_scim_base_url"

// Driver es el aprovisionador SCIM 2.0 genérico: implementa la capacidad
// Provisioner del contrato (connector.Provisioner) además de Driver.Test. Cierra
// sobre el directorio de Sytue ID (estado deseado) y el store del mapeo/historial;
// el cliente SCIM lo fabrica en cada operación a partir del Runtime.HTTP inyectado
// (así el Bearer del SaaS nunca lo ve el driver).
type Driver struct {
	dir Directory
	st  MapStore
}

// NewDriver construye el driver con sus dependencias inyectadas al cablear el
// Registry (main.go).
func NewDriver(dir Directory, st MapStore) *Driver {
	return &Driver{dir: dir, st: st}
}

// Kind identifica al driver en el Registry.
func (d *Driver) Kind() string { return Kind }

// Test verifica de extremo a extremo que la instancia puede autenticarse y alcanzar
// el endpoint SCIM del SaaS (GET /Users?count=1), usando EXCLUSIVAMENTE rt.HTTP.
func (d *Driver) Test(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	c, err := d.client(inst, rt)
	if err != nil {
		return err
	}
	// Un listado mínimo confirma auth + conectividad sin escribir nada.
	return c.doJSON(ctx, http.MethodGet, "/Users?count=1", nil, nil)
}

// Sync ejecuta la reconciliación por diff de la instancia (usuarios+grupos) contra
// el SaaS. Persiste una fila de sync_runs con los contadores; un error propaga para
// que el outbox reintente el trabajo con backoff/attempts.
func (d *Driver) Sync(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	c, err := d.client(inst, rt)
	if err != nil {
		return err
	}
	counts, err := Reconcile(ctx, Deps{Dir: d.dir, Map: d.st, SCIM: c}, inst)
	if rt.Log != nil {
		rt.Log.Info("scim sync completado", "instance", inst.ID,
			"created", counts.Created, "updated", counts.Updated,
			"deactivated", counts.Deactivated, "errors", counts.Errors)
	}
	return err
}

// client fabrica el cliente SCIM sobre el Runtime.HTTP de la pasarela y la URL base
// de la config. La URL base es obligatoria.
func (d *Driver) client(inst connector.Instance, rt connector.Runtime) (*Client, error) {
	base := inst.Config[ConfigBaseURL]
	if base == "" {
		return nil, fmt.Errorf("scimout: falta %q en la config de la instancia", ConfigBaseURL)
	}
	if _, err := url.ParseRequestURI(base); err != nil {
		return nil, fmt.Errorf("scimout: %q inválida: %w", ConfigBaseURL, err)
	}
	return NewClient(rt.HTTP, base), nil
}

// Compile-time: el driver satisface Driver y la capacidad opcional Provisioner.
var (
	_ connector.Driver      = (*Driver)(nil)
	_ connector.Provisioner = (*Driver)(nil)
)
