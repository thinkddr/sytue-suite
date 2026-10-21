// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package peticiones es «Pedir acceso de edición» de la franja de solo lectura
// (ReadOnlyBar en la web, ui.FranjaSoloLectura en Fyne), para cualquier producto.
//
// Quien solo puede leer algo —un documento, un calendario, un buzón compartido, un
// cuaderno, un módulo del ERP, un fichero compartido— lo pide; quien puede concederlo
// recibe un aviso del Hub y un correo, lo concede con un clic desde el sitio al que
// lleva el aviso, y la franja de quien lo pidió se quita sola.
//
// Aquí está lo que es igual en todos: la tabla de peticiones pendientes (cada servicio
// la crea en su migración con estas columnas, su RLS y su `<tabla>_platform_merge`), y
// el aviso por el Hub y por correo. Lo que NO es igual —quién puede conceder y qué
// significa conceder— lo pone cada servicio, que es el dueño de su permiso.
//
// El contrato HTTP es el mismo en todos, para que la franja de la web y la de Fyne se
// escriban una vez:
//
//	POST   …/edit-requests               pedir (idempotente)  → {"requested": true}
//	GET    …/edit-requests               las pendientes (quien concede) → {"requests": [...]}
//	POST   …/edit-requests/{sub}/grant   conceder
//	DELETE …/edit-requests/{sub}         rechazar
//
// Y al conceder, un aviso de tipo `<producto>.access.granted` con `params.resource`
// al que lo pidió: es lo que desbloquea su franja en vivo por el flujo del Hub en las
// apps que no tienen canal propio.
package peticiones

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
)

// Peticion es la de UNA persona sobre UN recurso.
type Peticion struct {
	Recurso string    `json:"resource"`
	Sujeto  string    `json:"sub"`
	Nombre  string    `json:"name,omitempty"`
	Correo  string    `json:"email,omitempty"`
	Creada  time.Time `json:"created_at"`
}

// Quien es cómo se enseña: el nombre, si no el correo, si no el `sub`.
func (p Peticion) Quien() string {
	for _, s := range []string{p.Nombre, p.Correo} {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return p.Sujeto
}

// De es la petición de quien llama sobre `recurso`, con el nombre y el correo de SU
// token: quien concede tiene que saber a quién, y el servicio no puede preguntárselo
// a Sytue ID sin la sesión de nadie. El permiso se concede contra el `sub`.
func De(c authn.Claims, recurso string) Peticion {
	return Peticion{Recurso: recurso, Sujeto: c.Subject, Nombre: strings.TrimSpace(c.Name), Correo: strings.TrimSpace(c.Email)}
}

// Tabla es la tabla de peticiones de un servicio. Columnas: <Recurso>, tenant,
// sujeto, nombre, correo, creada, con PRIMARY KEY (<Recurso>, sujeto) —o (tenant,
// <Recurso>, sujeto) con PorTenant, cuando el recurso no es un uuid sino una clave que
// se repite entre organizaciones, como un módulo del ERP—. Nombre y Recurso son
// constantes del código, nunca entrada.
type Tabla struct {
	Nombre    string
	Recurso   string
	PorTenant bool
}

func (t Tabla) sql(q string) string {
	unica := "{r}, sujeto"
	if t.PorTenant {
		unica = "tenant, {r}, sujeto"
	}
	q = strings.ReplaceAll(q, "{unica}", unica)
	return strings.NewReplacer("{t}", pgx.Identifier{t.Nombre}.Sanitize(), "{r}", pgx.Identifier{t.Recurso}.Sanitize()).Replace(q)
}

// Pedir deja (o renueva) la petición. true si es NUEVA: solo entonces se avisa, para
// que pulsar dos veces no mande dos avisos. Va dentro de la transacción de tenant de
// quien llama.
func (t Tabla) Pedir(ctx context.Context, tx pgx.Tx, tenant string, p Peticion) (bool, error) {
	nueva := false
	// `xmax = 0` distingue el INSERT del UPDATE del ON CONFLICT en la misma sentencia.
	err := tx.QueryRow(ctx, t.sql(`
		INSERT INTO {t} ({r}, tenant, sujeto, nombre, correo) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT ({unica}) DO UPDATE SET nombre = EXCLUDED.nombre, correo = EXCLUDED.correo
		RETURNING xmax = 0`), p.Recurso, tenant, p.Sujeto, p.Nombre, p.Correo).Scan(&nueva)
	if err != nil {
		return false, fmt.Errorf("peticiones: pedir: %w", err)
	}
	return nueva, nil
}

// deEstaOrg es la organización de la transacción de tenant de quien llama, EN el SQL.
// Un recurso puede repetirse entre organizaciones (un módulo del ERP), y la política
// RLS de la tabla no filtra nada si el rol de la conexión se la salta: sin esto,
// listar enseñaba las peticiones de todas y quitar borraba las de todas.
const deEstaOrg = `tenant = current_setting('app.tenant', true)`

// Listar son las pendientes de un recurso, las más antiguas primero.
func (t Tabla) Listar(ctx context.Context, tx pgx.Tx, recurso string) ([]Peticion, error) {
	rows, err := tx.Query(ctx, t.sql(`
		SELECT {r}::text, sujeto, nombre, correo, creada FROM {t}
		 WHERE `+deEstaOrg+` AND {r} = $1 ORDER BY creada`), recurso)
	if err != nil {
		return nil, fmt.Errorf("peticiones: listar: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(f pgx.CollectableRow) (Peticion, error) {
		var p Peticion
		return p, f.Scan(&p.Recurso, &p.Sujeto, &p.Nombre, &p.Correo, &p.Creada)
	})
	if err != nil {
		return nil, fmt.Errorf("peticiones: listar: %w", err)
	}
	return out, nil
}

// Quitar borra la petición (al conceder o al rechazar). false si no había: no se
// concede a quien no lo pidió.
func (t Tabla) Quitar(ctx context.Context, tx pgx.Tx, recurso, sujeto string) (bool, error) {
	tag, err := tx.Exec(ctx, t.sql(`DELETE FROM {t} WHERE `+deEstaOrg+` AND {r} = $1 AND sujeto = $2`), recurso, sujeto)
	if err != nil {
		return false, fmt.Errorf("peticiones: quitar: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// Hay dice si esta persona lo tiene pedido: la franja lo enseña como enviado al volver.
func (t Tabla) Hay(ctx context.Context, tx pgx.Tx, recurso, sujeto string) (bool, error) {
	hay := false
	err := tx.QueryRow(ctx, t.sql(`SELECT EXISTS (SELECT 1 FROM {t} WHERE `+deEstaOrg+` AND {r} = $1 AND sujeto = $2)`), recurso, sujeto).Scan(&hay)
	if err != nil {
		return false, fmt.Errorf("peticiones: consultar: %w", err)
	}
	return hay, nil
}
