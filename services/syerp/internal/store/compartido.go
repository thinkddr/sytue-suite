// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

// El ERP compartido por la organización: permisos por módulo, quién lo usa,
// tareas con vencimiento, documentos adjuntos y los avisos ya enviados.

// --- Permisos -----------------------------------------------------------------

// PermisosDe devuelve lo puesto a una persona: su nivel por módulo y, para la
// separación de funciones, la lista de verbos por módulo de quien la tenga.
//
// Las dos en la MISMA transacción: se leen juntas una vez por petición y
// separarlas serían dos viajes a la base para decidir una sola cosa.
func (s *Store) PermisosDe(ctx context.Context, sub string) (map[string]string, map[string][]string, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, nil, err
	}
	niveles := map[string]string{}
	verbos := map[string][]string{}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `SELECT modulo, nivel FROM permisos WHERE tenant = $1 AND sub = $2`, tenant, sub)
		if err != nil {
			return err
		}
		for filas.Next() {
			var modulo, nivel string
			if err := filas.Scan(&modulo, &nivel); err != nil {
				filas.Close()
				return err
			}
			niveles[modulo] = nivel
		}
		filas.Close()
		if err := filas.Err(); err != nil {
			return err
		}
		filas, err = tx.Query(ctx, `SELECT modulo, verbos FROM permisos_verbo WHERE tenant = $1 AND sub = $2`, tenant, sub)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var modulo string
			var lista []string
			if err := filas.Scan(&modulo, &lista); err != nil {
				return err
			}
			if lista == nil {
				// Una lista vacía NO es «no hay lista»: retira todos los gestos, y
				// un nil se confundiría con la ausencia de fila al consultarla.
				lista = []string{}
			}
			verbos[modulo] = lista
		}
		return filas.Err()
	})
	return niveles, verbos, err
}

// Persona es alguien de la organización tal como lo ve la administración del ERP.
type Persona struct {
	Sub     string            `json:"sub"`
	Email   string            `json:"email"`
	Nombre  string            `json:"nombre"`
	Roles   []string          `json:"roles"`
	Modulos map[string]string `json:"modulos"`
	// Verbos son los módulos donde a esta persona se le separaron las funciones
	// a mano. Ausente = los que implique su nivel.
	Verbos  map[string][]string `json:"verbos"`
	VistoEn *time.Time          `json:"visto_en,omitempty"`
}

// Personas junta quien ha entrado en el ERP y a quien se le ha puesto un nivel.
// Tres consultas en UNA transacción: quién entró, qué nivel tiene y a quién le
// separaron las funciones. Cada una en su función porque son tres bucles de
// escaneo distintos y juntos no se leen.
func (s *Store) Personas(ctx context.Context) ([]Persona, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	d := &directorio{porSub: map[string]*Persona{}}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := d.leerUsuarios(ctx, tx, tenant); err != nil {
			return err
		}
		if err := d.leerNiveles(ctx, tx, tenant); err != nil {
			return err
		}
		return d.leerVerbos(ctx, tx, tenant)
	})
	return d.lista(), err
}

// directorio acumula las tres consultas en las mismas personas, en el orden en
// que aparecieron.
type directorio struct {
	porSub map[string]*Persona
	orden  []string
}

func (d *directorio) persona(sub string) *Persona {
	p, ok := d.porSub[sub]
	if !ok {
		p = &Persona{Sub: sub, Roles: []string{}, Modulos: map[string]string{}, Verbos: map[string][]string{}}
		d.porSub[sub] = p
		d.orden = append(d.orden, sub)
	}
	return p
}

func (d *directorio) lista() []Persona {
	out := make([]Persona, 0, len(d.orden))
	for _, sub := range d.orden {
		out = append(out, *d.porSub[sub])
	}
	return out
}

func (d *directorio) leerUsuarios(ctx context.Context, tx pgx.Tx, tenant string) error {
	filas, err := tx.Query(ctx, `
		SELECT sub, email, nombre, roles, visto_en FROM usuarios
		 WHERE tenant = $1 ORDER BY lower(nombre), email`, tenant)
	if err != nil {
		return err
	}
	defer filas.Close()
	for filas.Next() {
		var sub, email, nombre string
		var roles []string
		var visto time.Time
		if err := filas.Scan(&sub, &email, &nombre, &roles, &visto); err != nil {
			return err
		}
		p := d.persona(sub)
		p.Email, p.Nombre, p.Roles, p.VistoEn = email, nombre, roles, &visto
	}
	return filas.Err()
}

func (d *directorio) leerNiveles(ctx context.Context, tx pgx.Tx, tenant string) error {
	filas, err := tx.Query(ctx, `SELECT sub, email, nombre, modulo, nivel FROM permisos WHERE tenant = $1`, tenant)
	if err != nil {
		return err
	}
	defer filas.Close()
	for filas.Next() {
		var sub, email, nombre, modulo, nivel string
		if err := filas.Scan(&sub, &email, &nombre, &modulo, &nivel); err != nil {
			return err
		}
		p := d.persona(sub)
		p.Modulos[modulo] = nivel
		if p.Email == "" {
			p.Email, p.Nombre = email, nombre
		}
	}
	return filas.Err()
}

func (d *directorio) leerVerbos(ctx context.Context, tx pgx.Tx, tenant string) error {
	filas, err := tx.Query(ctx, `SELECT sub, modulo, verbos FROM permisos_verbo WHERE tenant = $1`, tenant)
	if err != nil {
		return err
	}
	defer filas.Close()
	for filas.Next() {
		var sub, modulo string
		var lista []string
		if err := filas.Scan(&sub, &modulo, &lista); err != nil {
			return err
		}
		if lista == nil {
			lista = []string{}
		}
		d.persona(sub).Verbos[modulo] = lista
	}
	return filas.Err()
}

// GuardarPermisos sustituye lo puesto a una persona: `modulos` son SOLO los
// niveles que difieren del defecto, y `verbos` SOLO los módulos donde se
// separan las funciones a mano. Lo que no viene se retira: en los dos casos,
// volver al defecto es no tener fila.
func (s *Store) GuardarPermisos(ctx context.Context, sub, email, nombre string, modulos map[string]string, verbos map[string][]string, por string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM permisos WHERE tenant = current_setting('app.tenant', true) AND sub = $1`, sub); err != nil {
			return err
		}
		for modulo, nivel := range modulos {
			if _, err := tx.Exec(ctx, `
				INSERT INTO permisos (tenant, sub, modulo, nivel, email, nombre, actualizado_por)
				VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6)`,
				sub, modulo, nivel, email, nombre, por); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM permisos_verbo WHERE tenant = current_setting('app.tenant', true) AND sub = $1`, sub); err != nil {
			return err
		}
		for modulo, lista := range verbos {
			if lista == nil {
				lista = []string{}
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO permisos_verbo (tenant, sub, modulo, verbos, actualizado_por)
				VALUES (current_setting('app.tenant', true), $1, $2, $3, $4)`,
				sub, modulo, lista, por); err != nil {
				return err
			}
		}
		return nil
	})
}

// VerUsuario apunta que alguien ha entrado, con su correo y sus roles de ahora.
func (s *Store) VerUsuario(ctx context.Context, sub, email, nombre string, roles []string) error {
	if roles == nil {
		roles = []string{}
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO usuarios (tenant, sub, email, nombre, roles)
			VALUES (current_setting('app.tenant', true), $1, $2, $3, $4)
			ON CONFLICT (tenant, sub) DO UPDATE
			   SET email = EXCLUDED.email, nombre = EXCLUDED.nombre, roles = EXCLUDED.roles, visto_en = now()`,
			sub, email, nombre, roles)
		return err
	})
}

// Destinatario es a quién se escribe un aviso.
type Destinatario struct {
	Email, Nombre string
}

// DestinatariosDe son quienes llegan a `lectura` en un módulo y tienen correo:
// los que tienen el nivel puesto y los owner y admin que han entrado en el ERP.
// `porDefectoAbierto` añade a todos los que han entrado cuando el módulo está
// abierto por defecto y no tienen un nivel que lo cierre.
func (s *Store) DestinatariosDe(ctx context.Context, modulo string, porDefectoAbierto bool) ([]Destinatario, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	var out []Destinatario
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT DISTINCT ON (lower(email)) email, nombre FROM (
			  -- El correo es el que trajo el token de esa persona al entrar, no
			  -- el que escribió la administración al darle permiso: aquel era
			  -- texto libre y mandaba los avisos a cualquier dirección (NEG-16).
			  SELECT u.email, u.nombre FROM permisos p
			    JOIN usuarios u ON u.tenant = p.tenant AND u.sub = p.sub
			   WHERE p.tenant = $1 AND p.modulo = $2 AND p.nivel <> 'ninguno'
			  UNION ALL
			  SELECT u.email, u.nombre FROM usuarios u
			   WHERE u.tenant = $1
			     AND (u.roles && ARRAY['owner', 'admin']
			          OR ($3 AND NOT EXISTS (
			                SELECT 1 FROM permisos p
			                 WHERE p.tenant = u.tenant AND p.sub = u.sub AND p.modulo = $2 AND p.nivel = 'ninguno')))
			) d
			 WHERE email <> ''
			 ORDER BY lower(email)`, tenant, modulo, porDefectoAbierto)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var d Destinatario
			if err := filas.Scan(&d.Email, &d.Nombre); err != nil {
				return err
			}
			out = append(out, d)
		}
		return filas.Err()
	})
	return out, err
}

// --- Avisos ---------------------------------------------------------------------

// TenantsConPerfil son las organizaciones que han hecho el asistente. Es la ÚNICA
// lectura entre organizaciones del servicio (política de mantenimiento, mig 0009).
func (s *Store) TenantsConPerfil(ctx context.Context) ([]string, error) {
	var out []string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `SELECT tenant FROM organizacion ORDER BY tenant`)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var t string
			if err := filas.Scan(&t); err != nil {
				return err
			}
			out = append(out, t)
		}
		return filas.Err()
	})
	return out, err
}

// ReclamarAviso reserva un aviso: devuelve true la primera vez, y false si otra
// réplica o una pasada anterior ya lo mandó.
func (s *Store) ReclamarAviso(ctx context.Context, clave, periodo, tipo string) (bool, error) {
	var reclamado bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		etiq, err := tx.Exec(ctx, `
			INSERT INTO avisos_enviados (tenant, clave, periodo, tipo)
			VALUES (current_setting('app.tenant', true), $1, $2, $3)
			ON CONFLICT DO NOTHING`, clave, periodo, tipo)
		reclamado = err == nil && etiq.RowsAffected() == 1
		return err
	})
	return reclamado, err
}

// LiberarAviso deshace una reserva cuando el envío falló, para reintentarlo.
func (s *Store) LiberarAviso(ctx context.Context, clave, periodo, tipo string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM avisos_enviados WHERE tenant = current_setting('app.tenant', true) AND clave = $1 AND periodo = $2 AND tipo = $3`, clave, periodo, tipo)
		return err
	})
}

// --- Actividades ---------------------------------------------------------------

// Actividad es una tarea con vencimiento, suelta o sobre un registro.
type Actividad struct {
	ID                string     `json:"id"`
	Modelo            string     `json:"modelo"`
	Registro          string     `json:"registro"`
	RegistroEtiqueta  string     `json:"registro_etiqueta,omitempty"`
	Tipo              string     `json:"tipo"`
	Resumen           string     `json:"resumen"`
	Notas             string     `json:"notas,omitempty"`
	Vence             string     `json:"vence"`
	ResponsableSub    string     `json:"responsable_sub,omitempty"`
	ResponsableNombre string     `json:"responsable_nombre,omitempty"`
	Estado            string     `json:"estado"`
	CreadaEn          time.Time  `json:"creada_en"`
	HechaEn           *time.Time `json:"hecha_en,omitempty"`
}

// TiposDeActividad son los que admite la tabla (mig 0009).
var TiposDeActividad = []string{"subir_documento", "pedir_documento", "llamada", "correo", "reunion", "tarea"}

const columnasActividad = `id::text, modelo, registro, tipo, resumen, notas, vence::text, responsable_sub,
	responsable_nombre, estado, creada_en, hecha_en`

func escanearActividad(fila pgx.Row) (Actividad, error) {
	var a Actividad
	err := fila.Scan(&a.ID, &a.Modelo, &a.Registro, &a.Tipo, &a.Resumen, &a.Notas, &a.Vence,
		&a.ResponsableSub, &a.ResponsableNombre, &a.Estado, &a.CreadaEn, &a.HechaEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNoEncontrado
	}
	return a, err
}

// CrearActividad apunta una tarea.
func (s *Store) CrearActividad(ctx context.Context, a Actividad, por string) (Actividad, error) {
	var out Actividad
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = crearActividadEn(ctx, tx, a, por)
		return err
	})
	return out, err
}

func crearActividadEn(ctx context.Context, tx pgx.Tx, a Actividad, por string) (Actividad, error) {
	return escanearActividad(tx.QueryRow(ctx, `
		INSERT INTO actividades (tenant, modelo, registro, tipo, resumen, notas, vence, responsable_sub,
		                         responsable_nombre, creada_por)
		VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6::date, $7, $8, $9)
		RETURNING `+columnasActividad,
		a.Modelo, a.Registro, a.Tipo, a.Resumen, a.Notas, a.Vence, a.ResponsableSub, a.ResponsableNombre, por))
}

// FiltroActividades acota el listado.
type FiltroActividades struct {
	Estado           string
	Modelo           string
	Registro         string
	ResponsableSub   string
	HastaVencimiento string
	Limite           int
}

// Actividades lista tareas, las que vencen antes primero.
func (s *Store) Actividades(ctx context.Context, f FiltroActividades) ([]Actividad, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	if f.Limite <= 0 || f.Limite > 500 {
		f.Limite = 200
	}
	var out []Actividad
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT `+columnasActividad+` FROM actividades
			 WHERE tenant = $1
			   AND ($2 = '' OR estado = $2)
			   AND ($3 = '' OR modelo = $3)
			   AND ($4 = '' OR registro = $4)
			   AND ($5 = '' OR responsable_sub = $5)
			   AND ($6 = '' OR vence <= NULLIF($6, '')::date)
			 ORDER BY vence, creada_en
			 LIMIT $7`, tenant, f.Estado, f.Modelo, f.Registro, f.ResponsableSub, f.HastaVencimiento, f.Limite)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			a, err := escanearActividad(filas)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return filas.Err()
	})
	return out, err
}

// Actividad lee una tarea.
func (s *Store) Actividad(ctx context.Context, id string) (Actividad, error) {
	var a Actividad
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		a, err = escanearActividad(tx.QueryRow(ctx, `SELECT `+columnasActividad+` FROM actividades WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, id))
		return err
	})
	return a, err
}

// CerrarActividad marca una tarea hecha o cancelada. Solo desde pendiente.
func (s *Store) CerrarActividad(ctx context.Context, id, estado string) (Actividad, error) {
	var a Actividad
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		a, err = escanearActividad(tx.QueryRow(ctx, `
			UPDATE actividades
			   SET estado = $2, hecha_en = CASE WHEN $2 = 'hecha' THEN now() END
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND estado = 'pendiente'
			RETURNING `+columnasActividad, id, estado))
		return err
	})
	return a, err
}

// --- Adjuntos ------------------------------------------------------------------

// Adjunto es un documento colgado de un registro, sin su contenido.
type Adjunto struct {
	ID        string    `json:"id"`
	Modelo    string    `json:"modelo"`
	Registro  string    `json:"registro"`
	Nombre    string    `json:"nombre"`
	Tipo      string    `json:"tipo"`
	Tamano    int       `json:"tamano"`
	SHA256    string    `json:"sha256"`
	CreadoPor string    `json:"-"`
	CreadoEn  time.Time `json:"creado_en"`
}

// MaxAdjunto es el tope de un documento (el mismo CHECK de la tabla).
const MaxAdjunto = 10 << 20

// GuardarAdjunto guarda un documento y da por hechas las tareas «subir documento»
// de ese registro: subir la factura ES la tarea. Devuelve cuántas se cerraron.
func (s *Store) GuardarAdjunto(ctx context.Context, modelo, registro, nombre, tipo string, contenido []byte, por string) (Adjunto, int, error) {
	var a Adjunto
	cerradas := 0
	suma := sha256.Sum256(contenido)
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO adjuntos (tenant, modelo, registro, nombre, tipo, tamano, sha256, contenido, creado_por)
			VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id::text, modelo, registro, nombre, tipo, tamano, sha256, creado_por, creado_en`,
			modelo, registro, nombre, tipo, len(contenido), hex.EncodeToString(suma[:]), contenido, por).
			Scan(&a.ID, &a.Modelo, &a.Registro, &a.Nombre, &a.Tipo, &a.Tamano, &a.SHA256, &a.CreadoPor, &a.CreadoEn); err != nil {
			return err
		}
		etiq, err := tx.Exec(ctx, `
			UPDATE actividades SET estado = 'hecha', hecha_en = now()
			 WHERE tenant = current_setting('app.tenant', true) AND modelo = $1 AND registro = $2 AND tipo = 'subir_documento' AND estado = 'pendiente'`,
			modelo, registro)
		cerradas = int(etiq.RowsAffected())
		return err
	})
	return a, cerradas, err
}

// Adjuntos lista los documentos de un registro, el último primero.
func (s *Store) Adjuntos(ctx context.Context, modelo, registro string) ([]Adjunto, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	out := []Adjunto{}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT id::text, modelo, registro, nombre, tipo, tamano, sha256, creado_por, creado_en
			  FROM adjuntos WHERE tenant = $1 AND modelo = $2 AND registro = $3
			 ORDER BY creado_en DESC`, tenant, modelo, registro)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var a Adjunto
			if err := filas.Scan(&a.ID, &a.Modelo, &a.Registro, &a.Nombre, &a.Tipo, &a.Tamano, &a.SHA256, &a.CreadoPor, &a.CreadoEn); err != nil {
				return err
			}
			out = append(out, a)
		}
		return filas.Err()
	})
	return out, err
}

// AdjuntoConContenido lee un documento entero.
func (s *Store) AdjuntoConContenido(ctx context.Context, id string) (Adjunto, []byte, error) {
	var a Adjunto
	var contenido []byte
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id::text, modelo, registro, nombre, tipo, tamano, sha256, creado_por, creado_en, contenido
			  FROM adjuntos WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, id).
			Scan(&a.ID, &a.Modelo, &a.Registro, &a.Nombre, &a.Tipo, &a.Tamano, &a.SHA256, &a.CreadoPor, &a.CreadoEn, &contenido)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoEncontrado
		}
		return err
	})
	return a, contenido, err
}

// BorrarAdjunto borra un documento. Borra de verdad: es un fichero subido por
// error, no un asiento.
func (s *Store) BorrarAdjunto(ctx context.Context, id string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var modelo, registro string
		if err := tx.QueryRow(ctx, `SELECT modelo, registro FROM adjuntos WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, id).
			Scan(&modelo, &registro); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoEncontrado
			}
			return err
		}
		var proceso []string
		if modelo == "obligacion" {
			proceso = strings.SplitN(registro, ":", 3)
			if len(proceso) == 3 {
				// Mismo cerrojo y mismo orden que AvanzarProceso: completar nunca
				// puede comprobar un archivo mientras esta transacción lo borra.
				var fase int
				err := tx.QueryRow(ctx, `SELECT fase FROM procesos_obligaciones
					WHERE tenant=current_setting('app.tenant',true) AND clave=$1 AND periodo=$2 FOR UPDATE`,
					proceso[0], proceso[1]).Scan(&fase)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			}
		}
		etiq, err := tx.Exec(ctx, `DELETE FROM adjuntos WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, id)
		if err != nil {
			return err
		}
		if etiq.RowsAffected() == 0 {
			return ErrNoEncontrado
		}
		if len(proceso) == 3 {
			_, err = tx.Exec(ctx, `UPDATE procesos_obligaciones SET fase=3, updated_at=now()
				WHERE tenant=current_setting('app.tenant',true) AND clave=$1 AND periodo=$2 AND fase=4
				AND $3=ANY(documentos) AND NOT EXISTS (
					SELECT 1 FROM adjuntos WHERE tenant=current_setting('app.tenant',true)
					AND modelo='obligacion' AND registro=$4)`, proceso[0], proceso[1], proceso[2], registro)
		}
		return err
	})
}

// --- Varios registros de una vez ------------------------------------------------

// Nuevo es un registro por crear dentro de CrearVarios. `Enlaces` son campos de
// relación que apuntan a otro Nuevo anterior por su posición.
type Nuevo struct {
	Modelo   *catalogo.Modelo
	Data     map[string]any
	Enlaces  map[string]int
	Etiqueta string
	desdeTPV bool
}

// CrearVarios crea registros relacionados en UNA transacción: o entran todos, o
// ninguno. Es lo que hace que «apunta un gasto de Anthropic» no deje un proveedor
// suelto si la factura no pasa la validación.
func (s *Store) CrearVarios(ctx context.Context, nuevos []Nuevo, actividades []ActividadDe, ownerSub string) ([]Registro, []Actividad, error) {
	creados := make([]Registro, 0, len(nuevos))
	var hechas []Actividad
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		for i, n := range nuevos {
			r, err := insertarNuevo(ctx, tx, n, creados[:i], ownerSub)
			if err != nil {
				return err
			}
			creados = append(creados, r)
		}
		for _, a := range actividades {
			act := a.Actividad
			if a.Indice >= 0 && a.Indice < len(creados) {
				act.Modelo, act.Registro = creados[a.Indice].Modelo, creados[a.Indice].ID
			}
			hecha, err := crearActividadEn(ctx, tx, act, ownerSub)
			if err != nil {
				return err
			}
			hechas = append(hechas, hecha)
		}
		return nil
	})
	return creados, hechas, traducirError(err)
}

// insertarNuevo crea un registro de CrearVarios con sus enlaces ya resueltos a
// los creados antes que él.
func insertarNuevo(ctx context.Context, tx pgx.Tx, n Nuevo, antes []Registro, ownerSub string) (Registro, error) {
	var r Registro
	if !n.desdeTPV {
		if err := rechazarIdentidadTPV(n.Data); err != nil {
			return r, err
		}
	}
	if err := rechazarExternoFactura(n.Modelo.Clave, n.Data); err != nil {
		return r, err
	}
	data := make(map[string]any, len(n.Data)+len(n.Enlaces))
	for k, v := range n.Data {
		data[k] = v
	}
	for campo, j := range n.Enlaces {
		if j < 0 || j >= len(antes) {
			return r, fmt.Errorf("%s: enlace a un registro posterior: %w", campo, domain.ErrDatos)
		}
		data[campo] = antes[j].ID
	}
	if err := comprobarOrigenCompra(ctx, tx, n.Modelo.Clave, data); err != nil {
		return r, err
	}
	if err := comprobarRelaciones(ctx, tx, n.Modelo, data); err != nil {
		return r, err
	}
	ref := ""
	if n.Modelo.Secuencia != "" {
		var err error
		if ref, err = reservarRef(ctx, tx, "doc:"+n.Modelo.Clave, n.Modelo.Secuencia, periodoDe(data)); err != nil {
			return r, err
		}
	}
	crudo, err := json.Marshal(data)
	if err != nil {
		return r, err
	}
	err = escanearRegistro(tx.QueryRow(ctx, `
		INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
		VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6)
		RETURNING id::text, model, ref, etiqueta, estado, data, created_at, updated_at`,
		n.Modelo.Clave, ref, n.Etiqueta, domain.EstadoInicial(n.Modelo), crudo, ownerSub), &r)
	return r, err
}

// ActividadDe es una tarea que se cuelga de uno de los registros de CrearVarios
// (por su posición; -1 = suelta).
type ActividadDe struct {
	Indice int
	Actividad
}
