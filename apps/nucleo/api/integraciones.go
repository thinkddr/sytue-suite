// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Integraciones es el cliente de la pasarela de integraciones (`services/integrations`):
// lo que el lanzador del Hub le pregunta —qué aplicaciones de terceros ha conectado la
// organización y a cuáles ha dado un producto de la suite—.
type Integraciones struct{ c *Client }

// NewIntegraciones crea el cliente sobre la raíz del servicio (ServiceURL(apex,
// "integrations")).
func NewIntegraciones(servicio string, s Session, h *http.Client) *Integraciones {
	return &Integraciones{c: New(servicio+"/api/v1", s, h)}
}

// AppConectada es una tesela de aplicación de terceros, con la MISMA forma que un
// producto del lanzador (`myApp` en `handlers_myapps.go`). `IconURL` es un `data:` con
// el SVG del manifiesto, no una dirección de fuera.
type AppConectada struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Href        string `json:"href"`
	//nolint:tagliatelle // así lo escribe el servicio (camelCase, la forma de SuiteApp del lanzador).
	IconURL string `json:"iconUrl"`
	Color   string `json:"color"`
	Status  string `json:"status"`
}

// MisApps son las conectadas y ACTIVAS de la organización (`GET /my-apps`).
func (i *Integraciones) MisApps(ctx context.Context) ([]AppConectada, error) {
	r, err := JSON[struct {
		Apps []AppConectada `json:"apps"`
	}](ctx, i.c, Request{Path: "/my-apps"})
	return r.Apps, err
}

// Sustituto es adónde abrir un producto que la organización ha dado a una aplicación de
// terceros: «Hojas de cálculo» con Google, por ejemplo.
type Sustituto struct {
	Name string `json:"name"`
	Href string `json:"href"`
}

// Sustituciones es el mapa producto → sustituto (`GET /substitutions/resolved`). Solo
// trae lo sustituido: lo que no está lo sirve Sytue. Se descarta lo que venga sin
// dirección, como hace el BFF de la web.
func (i *Integraciones) Sustituciones(ctx context.Context) (map[string]Sustituto, error) {
	r, err := JSON[struct {
		Products map[string]Sustituto `json:"products"`
	}](ctx, i.c, Request{Path: "/substitutions/resolved"})
	if err != nil {
		return nil, err
	}
	out := make(map[string]Sustituto, len(r.Products))
	for producto, s := range r.Products {
		if s.Href == "" {
			continue
		}
		if s.Name == "" {
			s.Name = producto
		}
		out[producto] = s
	}
	return out, nil
}

// # «Traer mis datos»
//
// El puerto de `packages/api-clients/src/core/importData.ts`: la puerta de cada producto
// al motor de migración de la pasarela, que antes solo tenía la consola de
// administración. La pasarela ignora cualquier identidad que se le mande —origen y
// destino son siempre quien llama—, así que por aquí no viaja ninguna.

// OrigenImportacion es UNA cuenta de Google o Microsoft que la organización ya
// autorizó y de la que se pueden traer datos personales, aunque la haya conectado varias
// veces. La pasarela unifica (`/migrations/self/origins`) y aquí se vuelve a comprobar.
type OrigenImportacion struct {
	// InstanceID es la conexión con la que se lanza: la mejor de las de esa cuenta.
	InstanceID string
	// InstanceIDs son todas las conexiones de la cuenta, la elegida primero.
	InstanceIDs []string
	// Name es el que le puso la organización («el Google de ventas»).
	Name string
	// Source es `google` o `m365`.
	Source string
	// AccountKey es la identidad estable de la cuenta («google:acme.com»); vacía si no
	// se sabe. AccountLabel es cómo se le enseña a una persona («acme.com»).
	AccountKey, AccountLabel string
	// State es `ready` o `unavailable`: una conexión que existe pero no puede leer
	// (caducó, se revocó o nunca se autorizó para datos) se enseña apagada.
	State string
}

// Disponible dice si se puede lanzar.
func (o OrigenImportacion) Disponible() bool { return o.State != "unavailable" }

// TrabajoImportacion es una importación en curso o terminada.
type TrabajoImportacion struct {
	ID         string `json:"id"`
	InstanceID string `json:"instance_id"`
	Domain     string `json:"domain"`
	Source     string `json:"source"`
	Status     string `json:"status"` // queued, running, done, error o canceled
	LastError  string `json:"last_error"`
	// ErrorCode es `connection_expired` o `needs_admin` cuando el fallo tiene arreglo en
	// manos de alguien; vacío si no hay más arreglo que reintentar.
	ErrorCode string `json:"error_code"`
	// La cuenta de la que viene: dos trabajos de la misma cuenta son UNA importación.
	AccountKey   string `json:"account_key"`
	AccountLabel string `json:"account_label"`
	// Counts son ACUMULADOS de todas las pasadas del trabajo.
	Counts struct {
		Migrated int `json:"migrated"`
		Skipped  int `json:"skipped"`
		Errors   int `json:"errors"`
	} `json:"counts"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// EnCurso dice si sigue trabajando: mientras lo haga se vuelve a preguntar.
func (m TrabajoImportacion) EnCurso() bool { return m.Status == "queued" || m.Status == "running" }

// plantillasDeImportacion son las plantillas de las que la pasarela sabe traer contenido
// personal: EXACTAMENTE las de `sourceForTemplate`. Solo se usan con una pasarela
// anterior al listado de orígenes.
//
// Antes se reconocía por prefijo («google…», «microsoft…») y ese era el duplicado: la
// misma cuenta conectada para Drive, Meet, Chat, Teams o el correo de Outlook salía como
// otro «Traerlo de…», y todos menos uno fallaban al pulsarlos.
var plantillasDeImportacion = map[string]string{"google-workspace": "google", "microsoft365": "m365"}

// OrigenesImportacion son las cuentas de Google o Microsoft de la organización de las
// que quien llama puede traerse sus datos, una vez por cuenta. Quien llama trata un error
// como «no hay»: en una organización que no ha conectado ninguna, enseñar un fallo solo
// asustaría.
func (i *Integraciones) OrigenesImportacion(ctx context.Context) ([]OrigenImportacion, error) {
	r, err := JSON[struct {
		Origins []struct {
			InstanceID   string   `json:"instance_id"`
			InstanceIDs  []string `json:"instance_ids"`
			Name         string   `json:"name"`
			Source       string   `json:"source"`
			AccountKey   string   `json:"account_key"`
			AccountLabel string   `json:"account_label"`
			State        string   `json:"state"`
		} `json:"origins"`
	}](ctx, i.c, Request{Path: "/migrations/self/origins"})
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return i.origenesDeInstancias(ctx)
	}
	if err != nil {
		return nil, err
	}
	out := make([]OrigenImportacion, 0, len(r.Origins))
	for _, o := range r.Origins {
		if (o.Source != "google" && o.Source != "m365") || o.InstanceID == "" {
			continue
		}
		ids := o.InstanceIDs
		if !slices.Contains(ids, o.InstanceID) {
			ids = append([]string{o.InstanceID}, ids...)
		}
		out = append(out, OrigenImportacion{
			InstanceID: o.InstanceID, InstanceIDs: ids, Name: o.Name, Source: o.Source,
			AccountKey: strings.ToLower(strings.TrimSpace(o.AccountKey)), AccountLabel: strings.TrimSpace(o.AccountLabel),
			State: o.State,
		})
	}
	return UnificarOrigenes(out), nil
}

// origenesDeInstancias deduce los orígenes de `/instances` para una pasarela anterior al
// listado: plantillas exactas, ni borradores ni desactivadas, y la cuenta del dominio o
// el inquilino que diga la configuración.
func (i *Integraciones) origenesDeInstancias(ctx context.Context) ([]OrigenImportacion, error) {
	r, err := JSON[struct {
		Instances []struct {
			ID          string         `json:"id"`
			Name        string         `json:"name"`
			TemplateKey string         `json:"template_key"`
			Status      string         `json:"status"`
			HasSecrets  *bool          `json:"has_secrets"`
			Config      map[string]any `json:"config"`
		} `json:"instances"`
	}](ctx, i.c, Request{Path: "/instances"})
	if err != nil {
		return nil, err
	}
	texto := func(v any) string {
		s, _ := v.(string)
		return strings.TrimSpace(s)
	}
	var out []OrigenImportacion
	for _, x := range r.Instances {
		s := plantillasDeImportacion[x.TemplateKey]
		if s == "" || (x.Status != "active" && x.Status != "error") {
			continue
		}
		clave, rotulo := strings.ToLower(texto(x.Config["_import_account"])), texto(x.Config["_import_account_label"])
		if clave == "" {
			campo := "tenant"
			if s == "google" {
				campo = "domain"
			}
			if d := strings.ToLower(texto(x.Config[campo])); d != "" {
				clave, rotulo = s+":"+d, d
			}
		}
		estado := "ready"
		if x.Status != "active" || (x.HasSecrets != nil && !*x.HasSecrets) {
			estado = "unavailable"
		}
		out = append(out, OrigenImportacion{
			InstanceID: x.ID, InstanceIDs: []string{x.ID}, Name: x.Name, Source: s,
			AccountKey: clave, AccountLabel: rotulo, State: estado,
		})
	}
	return UnificarOrigenes(out), nil
}

// UnificarOrigenes deja UN origen por (proveedor, cuenta): el `unifyOrigins` de
// `packages/api-clients/src/core/importData.ts`. La pasarela ya lo hace; esto es la red
// por si una vieja, o una con un fallo, devolviera la misma cuenta dos veces. Se queda el
// primero salvo que uno posterior pueda leer y él no, y se juntan las conexiones. Una
// conexión de cuenta desconocida se suma a la ÚNICA cuenta conocida de su proveedor.
func UnificarOrigenes(os []OrigenImportacion) []OrigenImportacion {
	conocidas := map[string]map[string]bool{}
	for _, o := range os {
		if o.AccountKey == "" {
			continue
		}
		if conocidas[o.Source] == nil {
			conocidas[o.Source] = map[string]bool{}
		}
		conocidas[o.Source][o.AccountKey] = true
	}
	var orden []string
	por := map[string]OrigenImportacion{}
	for _, o := range os {
		cuenta := o.AccountKey
		if cuenta == "" && len(conocidas[o.Source]) == 1 {
			for unica := range conocidas[o.Source] {
				cuenta = unica
			}
		}
		clave := o.Source + "|" + cuenta
		antes, visto := por[clave]
		if !visto {
			o.AccountKey, o.InstanceIDs = cuenta, slices.Clone(o.InstanceIDs)
			por[clave] = o
			orden = append(orden, clave)
			continue
		}
		gana := antes
		if !antes.Disponible() && o.Disponible() {
			gana = o
		}
		ids := slices.Clone(gana.InstanceIDs)
		for _, id := range slices.Concat(antes.InstanceIDs, o.InstanceIDs) {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		gana.AccountKey, gana.InstanceIDs = cuenta, ids
		gana.AccountLabel = cmp.Or(gana.AccountLabel, antes.AccountLabel, o.AccountLabel)
		por[clave] = gana
	}
	out := make([]OrigenImportacion, 0, len(orden))
	for _, clave := range orden {
		out = append(out, por[clave])
	}
	return out
}

// UltimasImportaciones deja la importación VIGENTE de cada cuenta y contenido: la más
// reciente. Es el `latestImports` de la web. Una cuenta importada dos veces dejaba dos
// trabajos y el resumen los SUMABA; los contadores de un trabajo ya son acumulados de
// todas sus pasadas, así que el último basta. Sin cuenta conocida se agrupa por conexión.
func UltimasImportaciones(ts []TrabajoImportacion) []TrabajoImportacion {
	orden := slices.Clone(ts)
	slices.SortStableFunc(orden, func(a, b TrabajoImportacion) int { return b.CreatedAt.Compare(a.CreatedAt) })
	vistas := map[string]bool{}
	out := orden[:0]
	for _, t := range orden {
		cuenta := t.AccountKey
		if cuenta == "" {
			cuenta = "instance:" + cmp.Or(t.InstanceID, t.ID)
		}
		if clave := cuenta + "|" + t.Domain; !vistas[clave] {
			vistas[clave] = true
			out = append(out, t)
		}
	}
	return out
}

// MisImportaciones son las de quien llama con el contenido de `dominios`, reducidas a la
// vigente de cada cuenta y contenido. Se filtra aquí y no en la pasarela porque el mismo
// trabajo le importa a dos sitios: una migración de contactos la enseñan Contacts y
// Mail, cada uno con sus palabras.
func (i *Integraciones) MisImportaciones(ctx context.Context, dominios []string) ([]TrabajoImportacion, error) {
	r, err := JSON[struct {
		Migrations []TrabajoImportacion `json:"migrations"`
	}](ctx, i.c, Request{Path: "/migrations/self"})
	if err != nil {
		return nil, err
	}
	var out []TrabajoImportacion
	for _, m := range r.Migrations {
		if slices.Contains(dominios, m.Domain) {
			out = append(out, m)
		}
	}
	return UltimasImportaciones(out), nil
}

// Importar lanza la importación de `dominios` desde un origen y devuelve los trabajos.
// La pasarela es idempotente: si esa cuenta y contenido ya tenían uno, devuelve ese —en
// marcha, o reanudado donde se quedó— en vez de copiarlo todo de nuevo. Un rechazo es
// un *Error con su código (`import_connection_unavailable`…).
func (i *Integraciones) Importar(ctx context.Context, o OrigenImportacion, dominios []string) ([]TrabajoImportacion, error) {
	r, err := PostJSON[struct {
		Migrations []TrabajoImportacion `json:"migrations"`
	}](ctx, i.c, "/migrations/self", map[string]any{
		"instance_id": o.InstanceID, "source": o.Source, "domains": dominios,
	})
	return r.Migrations, err
}
