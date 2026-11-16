// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// # Aprobaciones con umbral y cuatro ojos
//
// Por encima del umbral de su módulo, asentar un documento contesta 409
// `requiere_aprobacion`: hace falta que OTRA persona lo apruebe (403 `cuatro_ojos` si lo
// intenta quien lo pidió o lo creó). Las formas son las de
// `services/syerp/internal/store/aprobaciones.go`, y las ata
// `TestERPAprobacionesHablaElIdiomaDelServicio`.

// ERPAprobacion es la solicitud de una segunda persona para un documento. Importe y
// divisa quedan congelados al pedirla: los calcula el servicio del propio documento.
type ERPAprobacion struct {
	ID         string `json:"id"`
	Modelo     string `json:"modelo"`
	RegistroID string `json:"registro_id"`
	// Importe va en céntimos enteros.
	Importe int64  `json:"importe"`
	Moneda  string `json:"moneda"`
	// Estado es «pendiente», «aprobada» o «rechazada».
	Estado        string     `json:"estado"`
	SolicitadaPor string     `json:"solicitada_por"`
	SolicitadaEn  time.Time  `json:"solicitada_en"`
	CreadaPor     string     `json:"creada_por,omitempty"`
	ResueltaPor   string     `json:"resuelta_por,omitempty"`
	ResueltaEn    *time.Time `json:"resuelta_en,omitempty"`
	Motivo        string     `json:"motivo,omitempty"`
}

// PuedeResolverla dice si `yo` puede aprobarla o rechazarla: pendiente, y ni pedida ni
// creada por esa persona. El servicio lo impone igual (403 `cuatro_ojos`); esto solo
// evita ofrecer un botón que siempre acabaría en ese error.
func (a ERPAprobacion) PuedeResolverla(yo string) bool {
	return a.Estado == "pendiente" && a.SolicitadaPor != yo && a.CreadaPor != yo
}

// ERPUmbral es el umbral de aprobación de un módulo, en céntimos.
type ERPUmbral struct {
	Modulo  string `json:"modulo"`
	Importe int64  `json:"importe"`
	Moneda  string `json:"moneda"`
}

// ERPModuloRotulado es un módulo con su nombre en el idioma de quien pregunta.
type ERPModuloRotulado struct {
	Clave  string `json:"clave"`
	Rotulo string `json:"rotulo"`
}

// ERPUmbrales es lo que devuelve `GET /admin/umbrales`: los puestos, la divisa en que
// se escribe uno nuevo y los módulos que lo admiten.
type ERPUmbrales struct {
	Umbrales         []ERPUmbral         `json:"umbrales"`
	MonedaPorDefecto string              `json:"moneda_por_defecto"`
	Modulos          []ERPModuloRotulado `json:"modulos"`
}

// Aprobaciones lista las solicitudes (`GET /api/v1/aprobaciones`). `estado` es
// «pendiente», «aprobada» o «rechazada»; vacío, todas.
func (e *ERP) Aprobaciones(ctx context.Context, estado string) ([]ERPAprobacion, error) {
	if e.private != nil {
		// La cola la lleva el servicio sobre registros en claro; los privados no están allí.
		return nil, ErrPrivateERPOperation
	}
	q := url.Values{}
	if estado != "" {
		q.Set("estado", estado)
	}
	r, err := JSON[struct {
		Aprobaciones []ERPAprobacion `json:"aprobaciones"`
	}](ctx, e.c, e.pedir("/aprobaciones", q))
	return r.Aprobaciones, err
}

// PedirAprobacion abre la solicitud de un documento
// (`POST /api/v1/{modelo}/{id}/aprobacion`). El cuerpo va vacío a propósito: el importe
// lo calcula el servicio del documento, que es el mismo que iría al libro.
func (e *ERP) PedirAprobacion(ctx context.Context, modelo, id string) (ERPAprobacion, error) {
	if e.private != nil {
		// La cola la lleva el servicio sobre registros en claro; los privados no están allí.
		return ERPAprobacion{}, ErrPrivateERPOperation
	}
	r := e.pedir("/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/aprobacion", nil)
	r.Method, r.Body = http.MethodPost, []byte("{}")
	r.Header.Set("Content-Type", "application/json")
	return JSON[ERPAprobacion](ctx, e.c, r)
}

// ResolverAprobacion aprueba o rechaza una solicitud
// (`POST /api/v1/aprobaciones/{id}/aprobar|rechazar`). Rechazar exige `motivo`.
func (e *ERP) ResolverAprobacion(ctx context.Context, id string, aprobar bool, motivo string) error {
	if e.private != nil {
		// La cola la lleva el servicio sobre registros en claro; los privados no están allí.
		return ErrPrivateERPOperation
	}
	como := "rechazar"
	if aprobar {
		como = "aprobar"
	}
	return e.enviar(ctx, "/aprobaciones/"+url.PathEscape(id)+"/"+como, map[string]string{"motivo": motivo})
}

// Umbrales devuelve los umbrales por módulo (`GET /api/v1/admin/umbrales`).
func (e *ERP) Umbrales(ctx context.Context) (ERPUmbrales, error) {
	return JSON[ERPUmbrales](ctx, e.c, e.pedir("/admin/umbrales", nil))
}

// GuardarUmbral pone el umbral de un módulo (`PUT /api/v1/admin/umbrales/{modulo}`).
// Un importe negativo lo quita.
func (e *ERP) GuardarUmbral(ctx context.Context, modulo string, centimos int64, moneda string) error {
	datos, err := json.Marshal(struct {
		Importe int64  `json:"importe"`
		Moneda  string `json:"moneda"`
	}{centimos, moneda})
	if err != nil {
		return err
	}
	return e.sinCuerpo(ctx, http.MethodPut, "/admin/umbrales/"+url.PathEscape(modulo), datos)
}
