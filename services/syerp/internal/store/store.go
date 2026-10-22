// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store implementa la persistencia de SyERP sobre PostgreSQL (pgx).
// Todo acceso pasa por transacciones con `app.tenant` fijado (pg.InTenantTx)
// para que RLS aísle por organización. Consultas SIEMPRE parametrizadas
// (CONVENTIONS §10).
//
// Hay UN CRUD para los treinta y seis modelos, no treinta y seis. El modelo
// decide qué se valida (eso lo hace `domain` antes de llegar aquí) y esta capa
// solo sabe de `records`: por eso añadir un módulo no toca ni una línea de Go.
//
// La única excepción es el libro diario, que tiene tablas y consultas propias
// porque tiene restricciones propias.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

// Store agrupa el acceso a datos de SyERP.
type Store struct {
	pool *pg.Pool
	// Version es la del binario: va en cada registro de VERI*FACTU.
	Version string
}

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// Healthz delega en el pool (para sondas /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// DB expone el pool para lo que necesita Postgres sin pasar por una consulta del
// dominio (hoy, el limitador compartido entre réplicas).
func (s *Store) DB() *pg.Pool { return s.pool }

// tenantDe devuelve el slug de la organización del contexto.
//
// POR QUÉ EL TENANT VIAJA TAMBIÉN EN EL `WHERE` cuando la RLS ya lo impone.
// Medido con 20.000 filas en un modelo: sin él, el planificador NO puede usar
// `records_listado` para ordenar —`tenant` es su primera columna y la política
// lo compara con `current_setting`, que es una expresión estable y no sirve de
// condición de índice ORDENADA—, así que leía las 20.000 filas y hacía un
// top-N: coste 2114 y 9,4 ms para servir 51 registros.
//
// Con la igualdad explícita: Index Scan de 51 filas, coste 8,88. El plan lo
// enseña `plan_integration_test.go`, y la política sigue ahí como `Filter`: esto
// NO sustituye a la RLS, le da al planificador el dato que la RLS no puede
// darle. Si algún día se omite, se pierde velocidad, nunca aislamiento.
func tenantDe(ctx context.Context) (string, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return "", err
	}
	return t.Slug, nil
}

// Errores del dominio de persistencia.
var (
	ErrNoEncontrado = errors.New("no encontrado")
	ErrYaAsentado   = errors.New("el documento ya tiene asiento")
	ErrRefDuplicada = errors.New("referencia duplicada")
)

// Registro es una fila de `records` tal como sale a la API.
type Registro struct {
	ID        string         `json:"id"`
	Modelo    string         `json:"modelo"`
	Ref       string         `json:"ref,omitempty"`
	Etiqueta  string         `json:"etiqueta"`
	Estado    string         `json:"estado,omitempty"`
	Data      map[string]any `json:"data"`
	CreadoEn  time.Time      `json:"creado_en"`
	EditadoEn time.Time      `json:"editado_en"`
}

// Pagina es un tramo de lista con su cursor keyset.
type Pagina struct {
	Registros []Registro `json:"registros"`
	// Etiquetas resuelve los identificadores de las relaciones de ESTA página a
	// su nombre legible, en una sola consulta. Sin esto, pintar una lista de
	// cincuenta pedidos serían cincuenta consultas para poner el nombre del
	// cliente — el N+1 clásico de cualquier ERP.
	Etiquetas map[string]string `json:"etiquetas,omitempty"`
	Siguiente string            `json:"siguiente,omitempty"`
}

// Filtro son los criterios de listado.
type Filtro struct {
	Modelo string
	Estado string
	// Texto busca en el nombre legible (índice trigram).
	Texto string
	// Campo/Valor filtra por un campo declarado: `data @> {campo: valor}`.
	Campo  string
	Valor  string
	Limite int
	Cursor string

	// Orden es la clave del campo por el que ordenar, ya comprobada contra el
	// catálogo por quien llama. Vacío = por fecha de creación, que es el camino
	// rápido (ver `ordenSQL`).
	Orden string
	// OrdenTipo es el tipo que declara el catálogo para ese campo: decide si se
	// compara como número, como fecha o como texto.
	OrdenTipo string
	// Ascendente invierte el sentido. Por defecto, descendente.
	Ascendente bool

	// IDs deja solo esos registros: la exportación de las filas marcadas. Vacío, todos.
	// Quien llama los ha comprobado ya como UUID.
	IDs []string

	// Papelera lista lo BORRADO en vez de lo vivo. No usa `records_listado` —es
	// un índice parcial sobre lo no borrado— así que recorre el modelo; es una
	// vista que se abre de uvas a peras y sobre pocas filas.
	Papelera bool
}

// idsONulo es la lista como parámetro de la consulta: NULL si está vacía, que es «sin
// filtro». Un `[]string{}` vacío llegaría como un array vacío y no dejaría pasar nada.
func idsONulo(ids []string) any {
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// claveSegura repite aquí la regla de claves del catálogo. Es defensa en
// profundidad: quien llama YA la comprobó contra el modelo, pero esta clave
// acaba dentro de una expresión SQL y un solo camino que se salte la validación
// convierte un nombre de campo en una inyección.
var claveSegura = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// abrirPaginacion interpreta el cursor según la forma de paginar de esta
// consulta: marca de tiempo más identificador cuando va por el índice, y
// desplazamiento cuando el orden lo pidió alguien a mano. Por fuera el cursor es
// una cadena opaca y quien llama no distingue los dos casos.
func abrirPaginacion(cursor string, porFecha bool) (time.Time, string, int, error) {
	if cursor == "" {
		return time.Time{}, "", 0, nil
	}
	if porFecha {
		marca, id, err := abrirCursor(cursor)
		return marca, id, 0, err
	}
	n, err := strconv.Atoi(cursor)
	if err != nil || n < 0 {
		return time.Time{}, "", 0, fmt.Errorf("cursor inválido: %w", domain.ErrDatos)
	}
	return time.Time{}, "", n, nil
}

// ordenSQL construye la expresión de ORDER BY.
//
// LO QUE CUESTA, medido: ordenar por una columna de la tabla —la fecha de
// creación— recorre `records_listado` y se para en la página. Ordenar por un
// campo de dentro del jsonb NO PUEDE usar índice: obliga a leer todas las filas
// del modelo y quedarse con las mejores. Con 20.000 filas eso son ~9 ms frente a
// ~0,5 ms. Es aceptable para una acción deliberada —pulsar una cabecera— y sería
// inaceptable como vista por defecto, que por eso sigue siendo la fecha.
//
// Los números se comparan como números y no como texto, o «9» iría después de
// «10». La guarda con expresión regular evita que un dato viejo —un campo que
// era texto y pasó a ser dinero en el manifiesto— tumbe la consulta con un error
// de conversión: lo que no case sale como nulo y se va al final.
func ordenSQL(campo, tipo string) (string, error) {
	switch campo {
	case "", "creado":
		return "created_at", nil
	case "ref":
		return "ref", nil
	case "estado":
		return "estado", nil
	case "etiqueta":
		return "etiqueta", nil
	}
	if !claveSegura.MatchString(campo) {
		return "", fmt.Errorf("campo de orden inválido: %w", domain.ErrDatos)
	}
	switch tipo {
	case catalogo.TipoDinero, catalogo.TipoEntero, catalogo.TipoDecimal:
		return fmt.Sprintf(
			`(CASE WHEN data->>'%s' ~ '^-?[0-9]+(\.[0-9]+)?$' THEN (data->>'%s')::numeric END)`,
			campo, campo), nil
	default:
		// Fechas y fecha-hora incluidas: van en ISO 8601, y ahí el orden
		// alfabético y el cronológico son el mismo.
		return fmt.Sprintf(`data->>'%s'`, campo), nil
	}
}

const limiteMax = 200

// Listar devuelve una página de registros de un modelo, más nueva primero.
func (s *Store) Listar(ctx context.Context, f Filtro) (Pagina, error) {
	if f.Limite <= 0 || f.Limite > limiteMax {
		f.Limite = 50
	}
	var contenido []byte
	if f.Campo != "" {
		// El campo ya viene comprobado contra el catálogo por el handler, así que
		// esto no es SQL dinámico: es un parámetro jsonb.
		crudo, err := json.Marshal(map[string]string{f.Campo: f.Valor})
		if err != nil {
			return Pagina{}, fmt.Errorf("filtro: %w", err)
		}
		contenido = crudo
	}

	tenant, err := tenantDe(ctx)
	if err != nil {
		return Pagina{}, err
	}

	orden, err := ordenSQL(f.Orden, f.OrdenTipo)
	if err != nil {
		return Pagina{}, err
	}
	sentido := "DESC"
	if f.Ascendente {
		sentido = "ASC"
	}

	// DOS FORMAS DE PAGINAR, y el motivo es el plan, no el capricho.
	//
	// Por defecto (fecha de creación) el cursor es keyset: `records_listado` está
	// ordenado justo así, se entra por el cursor y se sale en la página. Es el
	// camino que sirve casi todas las peticiones y el que no se puede perder.
	//
	// Con un orden pedido a mano no hay índice que valga —el campo vive dentro
	// del jsonb—, así que la consulta lee el modelo entero se pagine como se
	// pagine. Ahí el keyset no ahorraría nada y sí costaría arrastrar el valor de
	// ordenación dentro del cursor, con su tipo; el desplazamiento hace lo mismo
	// por mucho menos. El cursor sigue siendo una cadena opaca: por fuera esto no
	// se nota.
	porFecha := f.Orden == "" || f.Orden == "creado"

	cursorFecha, cursorID, desplazamiento, err := abrirPaginacion(f.Cursor, porFecha)
	if err != nil {
		return Pagina{}, err
	}

	// El SQL se compone con `orden` y `sentido`, que NO vienen del usuario: el
	// primero sale de `ordenSQL` —lista cerrada más una clave validada contra la
	// expresión regular— y el segundo es uno de dos literales. Todo lo demás va
	// parametrizado.
	consulta := fmt.Sprintf(`
			SELECT id::text AS rid, model, ref, etiqueta, estado, data, created_at, updated_at
			  FROM records
			 WHERE tenant = $1
			   AND model = $2
			   AND (deleted_at IS NULL) = %t
			   AND ($3 = '' OR estado = $3)
			   AND ($4 = '' OR plegar(etiqueta) LIKE '%%' || plegar($4) || '%%')
			   AND ($5::jsonb IS NULL OR data @> $5::jsonb)
			   AND ($6::timestamptz IS NULL
			        OR (created_at, id) < ($6::timestamptz, $7::uuid))
			   AND ($10::uuid[] IS NULL OR id = ANY($10::uuid[]))
			 ORDER BY %s %s, id %s
			 LIMIT $8 OFFSET $9`, !f.Papelera, orden, sentido, sentido)

	var pag Pagina
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// `id` como desempate SIEMPRE: sin él, dos filas con el mismo valor de
		// orden pueden salir en distinto orden en dos consultas, y entonces
		// paginar repite unas y se salta otras.
		filas, err := tx.Query(ctx, consulta,
			tenant, f.Modelo, f.Estado, f.Texto, contenido,
			nuloSiCero(cursorFecha), nuloSiVacio(cursorID), f.Limite+1, desplazamiento, idsONulo(f.IDs))
		if err != nil {
			return err
		}
		defer filas.Close()

		for filas.Next() {
			var r Registro
			var crudo []byte
			if err := filas.Scan(&r.ID, &r.Modelo, &r.Ref, &r.Etiqueta, &r.Estado,
				&crudo, &r.CreadoEn, &r.EditadoEn); err != nil {
				return err
			}
			if err := json.Unmarshal(crudo, &r.Data); err != nil {
				return err
			}
			pag.Registros = append(pag.Registros, r)
		}
		return filas.Err()
	})
	if err != nil {
		return Pagina{}, err
	}

	if len(pag.Registros) > f.Limite {
		ultimo := pag.Registros[f.Limite-1]
		pag.Registros = pag.Registros[:f.Limite]
		if porFecha {
			pag.Siguiente = cerrarCursor(ultimo.CreadoEn, ultimo.ID)
		} else {
			pag.Siguiente = strconv.Itoa(desplazamiento + f.Limite)
		}
	}
	return pag, nil
}

// ResolverEtiquetas devuelve el nombre legible de los identificadores dados, en
// UNA consulta. Los que no existan (o estén borrados) simplemente no salen.
func (s *Store) ResolverEtiquetas(ctx context.Context, ids []string) (map[string]string, error) {
	// Mapa vacío y no nulo: quien lo recibe indexa sin comprobar, y un nulo aquí
	// solo sirve para provocar un pánico en el primer acceso.
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT id::text AS rid, CASE WHEN etiqueta <> '' THEN etiqueta ELSE ref END
			  FROM records
			 WHERE tenant = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NULL`, tenant, ids)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var id, etiqueta string
			if err := filas.Scan(&id, &etiqueta); err != nil {
				return err
			}
			out[id] = etiqueta
		}
		return filas.Err()
	})
	return out, err
}

// Buscar busca registros a través de todos los modelos por etiqueta o referencia
// (o los más recientes si texto está vacío), respetando el tenant y registros no borrados.
func (s *Store) Buscar(ctx context.Context, texto string, limite int) ([]Registro, error) {
	if limite <= 0 {
		limite = 20
	}
	if limite > 100 {
		limite = 100
	}
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}

	texto = strings.TrimSpace(texto)
	consulta := `
		SELECT id::text AS rid, model, ref, etiqueta, estado, data, created_at, updated_at
		  FROM records
		 WHERE tenant = $1
		   AND deleted_at IS NULL
		   AND ($2 = '' OR plegar(etiqueta) LIKE '%' || plegar($2) || '%' OR plegar(ref) LIKE '%' || plegar($2) || '%')
		 ORDER BY updated_at DESC, id DESC
		 LIMIT $3`

	var registros []Registro
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, consulta, tenant, texto, limite)
		if err != nil {
			return err
		}
		defer filas.Close()

		for filas.Next() {
			var r Registro
			var crudo []byte
			if err := filas.Scan(&r.ID, &r.Modelo, &r.Ref, &r.Etiqueta, &r.Estado,
				&crudo, &r.CreadoEn, &r.EditadoEn); err != nil {
				return err
			}
			if len(crudo) > 0 {
				_ = json.Unmarshal(crudo, &r.Data)
			}
			registros = append(registros, r)
		}
		return filas.Err()
	})
	if err != nil {
		return nil, err
	}
	return registros, nil
}

// Obtener lee un registro por id.
func (s *Store) Obtener(ctx context.Context, modelo, id string) (Registro, error) {
	var r Registro
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return escanearRegistro(tx.QueryRow(ctx, `
			SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at
			  FROM records
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = $2 AND deleted_at IS NULL`, id, modelo), &r)
	})
	return r, err
}

// Crear inserta un registro ya validado. Reserva la referencia si el modelo se
// numera y comprueba que sus relaciones apuntan a filas que existen — todo
// dentro de la MISMA transacción, que es lo único que hace cierta la
// comprobación.
func (s *Store) Crear(ctx context.Context, m *catalogo.Modelo, data map[string]any, etiqueta, ownerSub string) (Registro, error) {
	var r Registro
	if err := rechazarIdentidadTPV(data); err != nil {
		return r, err
	}
	if err := rechazarExternoFactura(m.Clave, data); err != nil {
		return r, err
	}
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		if err := comprobarOrigenCompra(ctx, tx, m.Clave, data); err != nil {
			return err
		}
		if err := comprobarRelaciones(ctx, tx, m, data); err != nil {
			return err
		}
		ref := ""
		if m.Secuencia != "" {
			var err error
			if ref, err = reservarRef(ctx, tx, "doc:"+m.Clave, m.Secuencia, periodoDe(data)); err != nil {
				return err
			}
		}
		crudo, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return escanearRegistro(tx.QueryRow(ctx, `
			INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
			VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6)
			RETURNING id::text, model, ref, etiqueta, estado, data, created_at, updated_at`,
			m.Clave, ref, etiqueta, domain.EstadoInicial(m), crudo, ownerSub), &r)
	})
	return r, traducirError(err)
}

// Actualizar reemplaza los datos de un registro. No toca ni la referencia ni el
// estado: cambiar de estado es otra operación, con sus reglas.
func (s *Store) Actualizar(ctx context.Context, m *catalogo.Modelo, id string, data map[string]any, etiqueta string) (Registro, error) {
	var r Registro
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		if err := conservarIdentidadDocumento(ctx, tx, m.Clave, id, data); err != nil {
			return err
		}
		if err := comprobarRelaciones(ctx, tx, m, data); err != nil {
			return err
		}
		crudo, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return escanearRegistro(tx.QueryRow(ctx, `
			UPDATE records
			   SET data = $3,
			       etiqueta = CASE WHEN $4 <> '' THEN $4 ELSE etiqueta END,
			       updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = $2 AND deleted_at IS NULL
			RETURNING id::text, model, ref, etiqueta, estado, data, created_at, updated_at`,
			id, m.Clave, crudo, etiqueta), &r)
	})
	return r, traducirError(err)
}

// Conserva la identidad interna con el mismo bloqueo que protege la edición.
func conservarIdentidadDocumento(ctx context.Context, tx pgx.Tx, modelo, id string, data map[string]any) error {
	if modelo != "factura_venta" && modelo != "presupuesto" && modelo != "factura_compra" {
		return nil
	}
	var previo map[string]any
	if err := tx.QueryRow(ctx, `SELECT data FROM records WHERE tenant = current_setting('app.tenant', true) AND id=$1::uuid AND model=$2 AND deleted_at IS NULL FOR UPDATE`, id, modelo).Scan(&previo); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoEncontrado
		}
		return err
	}
	if err := conservarIdentidadTPV(previo, data); err != nil {
		return err
	}
	if modelo == "factura_venta" {
		return conservarExternoFactura(previo, data)
	}
	if modelo == "factura_compra" {
		return conservarOrigenCompra(previo, data)
	}
	return nil
}

// CambiarEstado mueve el documento dentro del flujo declarado por su modelo. Una
// factura que se emite o se anula deja antes su registro de VERI*FACTU.
func (s *Store) CambiarEstado(ctx context.Context, m *catalogo.Modelo, id, estado, sub string) (Registro, error) {
	var r Registro
	modelo := m.Clave
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		if err := exigirAprobacionAlEmitir(ctx, tx, m, []string{id}, estado); err != nil {
			return err
		}
		if err := s.emitirVerifactu(ctx, tx, modelo, []string{id}, estado, sub); err != nil {
			return err
		}
		return escanearRegistro(tx.QueryRow(ctx, `
			UPDATE records SET estado = $3, updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = $2 AND deleted_at IS NULL
			RETURNING id::text, model, ref, etiqueta, estado, data, created_at, updated_at`,
			id, modelo, estado), &r)
	})
	return r, traducirError(err)
}

// Borrar marca el registro como borrado. No destruye: un documento contable
// referenciado por un asiento no puede desaparecer del sistema, y distinguir
// cuáles sí y cuáles no es justo el tipo de regla que se olvida.
func (s *Store) Borrar(ctx context.Context, modelo, id string) error {
	return traducirError(s.escritura(ctx, func(tx pgx.Tx) error {
		etiq, err := tx.Exec(ctx, `
			UPDATE records SET deleted_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = $2 AND deleted_at IS NULL`, id, modelo)
		if err != nil {
			return err
		}
		if etiq.RowsAffected() == 0 {
			return ErrNoEncontrado
		}
		return nil
	}))
}

// Restaurar saca un registro de la papelera. No revive lo que nunca estuvo
// borrado: si ya estaba vivo, no hay nada que hacer y se dice.
func (s *Store) Restaurar(ctx context.Context, modelo, id string) (Registro, error) {
	var r Registro
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		return escanearRegistro(tx.QueryRow(ctx, `
			UPDATE records SET deleted_at = NULL, updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = $2 AND deleted_at IS NOT NULL
			RETURNING id::text AS rid, model, ref, etiqueta, estado, data, created_at, updated_at`,
			id, modelo), &r)
	})
	return r, err
}

// LoteEstado actualiza el estado de múltiples registros del mismo modelo en una
// sola transacción, con los registros de VERI*FACTU de las facturas que toquen.
func (s *Store) LoteEstado(ctx context.Context, m *catalogo.Modelo, ids []string, estado, sub string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var afectadas int64
	modelo := m.Clave
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		if err := exigirAprobacionAlEmitir(ctx, tx, m, ids, estado); err != nil {
			return err
		}
		if err := s.emitirVerifactu(ctx, tx, modelo, ids, estado, sub); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE records SET estado = $1, updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = ANY($2::uuid[]) AND model = $3 AND deleted_at IS NULL`,
			estado, ids, modelo)
		if err != nil {
			return err
		}
		afectadas = tag.RowsAffected()
		return nil
	})
	return afectadas, traducirError(err)
}

// LoteBorrar envía múltiples registros a la papelera en una sola transacción.
func (s *Store) LoteBorrar(ctx context.Context, modelo string, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var afectadas int64
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE records SET deleted_at = now(), updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = ANY($1::uuid[]) AND model = $2 AND deleted_at IS NULL`,
			ids, modelo)
		if err != nil {
			return err
		}
		afectadas = tag.RowsAffected()
		return nil
	})
	return afectadas, traducirError(err)
}

// LoteRestaurar recupera múltiples registros de la papelera en una sola transacción.
func (s *Store) LoteRestaurar(ctx context.Context, modelo string, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var afectadas int64
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE records SET deleted_at = NULL, updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = ANY($1::uuid[]) AND model = $2 AND deleted_at IS NOT NULL`,
			ids, modelo)
		if err != nil {
			return err
		}
		afectadas = tag.RowsAffected()
		return nil
	})
	return afectadas, err
}

// --- Módulos activos --------------------------------------------------------

// ModulosActivos devuelve las claves de módulo encendidas por la organización.
// Vacío significa «todavía no ha elegido», y el handler decide qué enseñar.
func (s *Store) ModulosActivos(ctx context.Context) ([]string, error) {
	var out []string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `SELECT modulo FROM modulos_activos WHERE tenant = current_setting('app.tenant', true) AND activo ORDER BY modulo`)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var m string
			if err := filas.Scan(&m); err != nil {
				return err
			}
			out = append(out, m)
		}
		return filas.Err()
	})
	return out, err
}

// ActivarModulo enciende o apaga un módulo para la organización.
func (s *Store) ActivarModulo(ctx context.Context, modulo string, activo bool) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO modulos_activos (tenant, modulo, activo)
			VALUES (current_setting('app.tenant', true), $1, $2)
			ON CONFLICT (tenant, modulo)
			DO UPDATE SET activo = EXCLUDED.activo, updated_at = now()`, modulo, activo)
		return err
	})
}

// --- Ayudas internas --------------------------------------------------------

func escanearRegistro(fila pgx.Row, r *Registro) error {
	var crudo []byte
	err := fila.Scan(&r.ID, &r.Modelo, &r.Ref, &r.Etiqueta, &r.Estado, &crudo, &r.CreadoEn, &r.EditadoEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoEncontrado
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(crudo, &r.Data)
}

// comprobarRelaciones exige que cada relación apunte a una fila viva del modelo
// que declara el catálogo. Una sola consulta para todas: un formulario con seis
// relaciones no puede costar seis viajes.
func comprobarRelaciones(ctx context.Context, tx pgx.Tx, m *catalogo.Modelo, data map[string]any) error {
	rel := domain.Relaciones(m, data)
	if len(rel) == 0 {
		return nil
	}
	esperado := map[string]string{} // id -> modelo que debe tener
	ids := make([]string, 0, len(rel))
	for _, f := range m.Campos {
		if id, ok := rel[f.Clave]; ok {
			esperado[id] = f.Modelo
			ids = append(ids, id)
		}
	}
	filas, err := tx.Query(ctx, `
		SELECT id::text, model FROM records
		 WHERE tenant = current_setting('app.tenant', true) AND id = ANY($1::uuid[]) AND deleted_at IS NULL`, ids)
	if err != nil {
		return err
	}
	defer filas.Close()
	encontrado := map[string]string{}
	for filas.Next() {
		var id, modelo string
		if err := filas.Scan(&id, &modelo); err != nil {
			return err
		}
		encontrado[id] = modelo
	}
	if err := filas.Err(); err != nil {
		return err
	}
	for campo, id := range rel {
		modelo, existe := encontrado[id]
		if !existe {
			return fmt.Errorf("%s: el registro referenciado no existe: %w", campo, domain.ErrDatos)
		}
		if modelo != esperado[id] {
			return fmt.Errorf("%s: se esperaba un registro de %q y es de %q: %w", campo, esperado[id], modelo, domain.ErrDatos)
		}
	}
	return nil
}

// reservarRef toma el siguiente número de la serie DENTRO de la transacción que
// crea el documento: dos peticiones a la vez no pueden llevarse el mismo, porque
// la segunda espera al bloqueo de fila de la primera.
func reservarRef(ctx context.Context, tx pgx.Tx, clave, prefijo, periodo string) (string, error) {
	var n int64
	err := tx.QueryRow(ctx, `
		INSERT INTO sequences (tenant, clave, periodo, ultimo)
		VALUES (current_setting('app.tenant', true), $1, $2, 1)
		ON CONFLICT (tenant, clave, periodo)
		DO UPDATE SET ultimo = sequences.ultimo + 1
		RETURNING ultimo`, clave, periodo).Scan(&n)
	if err != nil {
		return "", fmt.Errorf("reservar número de %s: %w", prefijo, err)
	}
	return fmt.Sprintf("%s-%s-%05d", prefijo, periodo, n), nil
}

// periodoDe es el año con el que se numera el documento: el de SU fecha, no el
// de hoy. Una factura del 31 de diciembre registrada en enero pertenece al
// ejercicio que dice la factura.
func periodoDe(data map[string]any) string {
	for _, clave := range []string{"fecha", "fecha_inicio"} {
		if s, ok := data[clave].(string); ok && len(s) >= 4 {
			if _, err := time.Parse(time.DateOnly, s[:10]); err == nil {
				return s[:4]
			}
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return fmt.Sprintf("%04d", t.Year())
			}
		}
	}
	return fmt.Sprintf("%04d", time.Now().UTC().Year())
}

func nuloSiCero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func nuloSiVacio(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// El cursor es opaco por fuera y trivial por dentro: la marca de tiempo y el id
// de la última fila servida. No se firma porque no autoriza nada — RLS decide
// qué se ve, y adelantar el cursor solo permite saltarse páginas propias.
func cerrarCursor(t time.Time, id string) string {
	return t.UTC().Format(time.RFC3339Nano) + "|" + id
}

func abrirCursor(c string) (time.Time, string, error) {
	marca, id, ok := strings.Cut(c, "|")
	if !ok {
		return time.Time{}, "", fmt.Errorf("cursor inválido")
	}
	t, err := time.Parse(time.RFC3339Nano, marca)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("cursor inválido")
	}
	return t, id, nil
}

func traducirError(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case !errors.As(err, &pgErr):
		return err
	case pgErr.Code == "23505":
		return ErrRefDuplicada
	case pgErr.Code == "SY002":
		// El trigger de la 0010: la factura ya tiene registro de VERI*FACTU.
		return ErrFacturaEmitida
	}
	return err
}

// ResumenFacturacion agrupa métricas de ventas y facturación.
type ResumenFacturacion struct {
	TotalCentimos          int64  `json:"total_centimos"`
	TotalRegistros         int    `json:"total_registros"`
	PendienteCobroCentimos int64  `json:"pendiente_cobro_centimos"`
	FacturasPendientes     int    `json:"facturas_pendientes"`
	ProximoVencimiento     string `json:"proximo_vencimiento,omitempty"`
}

// ResumenContabilidad agrupa el estado de cuadre del libro diario.
type ResumenContabilidad struct {
	TotalAsientos int64 `json:"total_asientos"`
	TotalApuntes  int64 `json:"total_apuntes"`
	TotalDebe     int64 `json:"total_debe"`
	TotalHaber    int64 `json:"total_haber"`
	Cuadra        bool  `json:"cuadra"`
}

// ResumenOperativo contiene los indicadores clave del tenant para la pantalla de inicio.
type ResumenOperativo struct {
	Facturacion  ResumenFacturacion  `json:"facturacion"`
	Contabilidad ResumenContabilidad `json:"contabilidad"`
	Contactos    int                 `json:"contactos"`
}

// Resumen calcula las métricas operativas del tenant en una sola pasada.
func (s *Store) Resumen(ctx context.Context) (ResumenOperativo, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return ResumenOperativo{}, err
	}
	var res ResumenOperativo
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// 1. Facturas de cliente
		const qFacturas = `
			SELECT
				COALESCE(SUM(CASE WHEN estado <> 'anulado' THEN COALESCE((data->>'total')::bigint, 0) ELSE 0 END), 0),
				COUNT(CASE WHEN estado <> 'anulado' THEN 1 END),
				COALESCE(SUM(CASE WHEN estado IN ('confirmado', 'emitida', 'pendiente') THEN COALESCE((data->>'total')::bigint, 0) ELSE 0 END), 0),
				COUNT(CASE WHEN estado IN ('confirmado', 'emitida', 'pendiente') THEN 1 END),
				COALESCE(MIN(CASE WHEN estado IN ('confirmado', 'emitida', 'pendiente') AND (data->>'vencimiento') IS NOT NULL AND (data->>'vencimiento') <> '' THEN (data->>'vencimiento') END), '')
			FROM records
			WHERE tenant = $1 AND model IN ('factura_venta', 'facturas_cliente') AND deleted_at IS NULL;
		`
		var proxVenc string
		if err := tx.QueryRow(ctx, qFacturas, tenant).Scan(
			&res.Facturacion.TotalCentimos,
			&res.Facturacion.TotalRegistros,
			&res.Facturacion.PendienteCobroCentimos,
			&res.Facturacion.FacturasPendientes,
			&proxVenc,
		); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("resumen facturas: %w", err)
		}
		res.Facturacion.ProximoVencimiento = proxVenc

		// 2. Contabilidad (moves & move_lines)
		const qMoves = `
			SELECT
				COUNT(DISTINCT m.id),
				COUNT(l.id),
				COALESCE(SUM(l.debe), 0),
				COALESCE(SUM(l.haber), 0)
			FROM moves m
			LEFT JOIN move_lines l ON l.tenant = m.tenant AND l.move_id = m.id
			WHERE m.tenant = $1 AND m.estado = 'asentado';
		`
		if err := tx.QueryRow(ctx, qMoves, tenant).Scan(
			&res.Contabilidad.TotalAsientos,
			&res.Contabilidad.TotalApuntes,
			&res.Contabilidad.TotalDebe,
			&res.Contabilidad.TotalHaber,
		); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("resumen contabilidad: %w", err)
		}
		res.Contabilidad.Cuadra = (res.Contabilidad.TotalDebe == res.Contabilidad.TotalHaber)

		// 3. Contactos
		const qContactos = `
			SELECT COUNT(*)
			FROM records
			WHERE tenant = $1 AND model IN ('contacto', 'contactos') AND deleted_at IS NULL;
		`
		if err := tx.QueryRow(ctx, qContactos, tenant).Scan(&res.Contactos); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("resumen contactos: %w", err)
		}

		return nil
	})
	return res, err
}

// EstadisticasModelo agrupa agregados y conteos por estado de un modelo.
type EstadisticasModelo struct {
	Modelo     string         `json:"modelo"`
	Total      int            `json:"total"`
	PorEstado  map[string]int `json:"por_estado"`
	SumaDinero int64          `json:"suma_dinero,omitempty"`
}

// Estadisticas calcula el desglose de registros por estado y la suma monetaria si aplica.
func (s *Store) Estadisticas(ctx context.Context, m *catalogo.Modelo) (EstadisticasModelo, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return EstadisticasModelo{}, err
	}
	res := EstadisticasModelo{
		Modelo:    m.Clave,
		PorEstado: make(map[string]int),
	}

	var campoDinero string
	for _, col := range m.Lista {
		for _, c := range m.Campos {
			if c.Clave == col && c.Tipo == catalogo.TipoDinero {
				campoDinero = col
				break
			}
		}
		if campoDinero != "" {
			break
		}
	}

	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		const qEstados = `
			SELECT estado, COUNT(*)
			FROM records
			WHERE tenant = $1 AND model = $2 AND deleted_at IS NULL
			GROUP BY estado;
		`
		rows, err := tx.Query(ctx, qEstados, tenant, m.Clave)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var est string
			var cnt int
			if err := rows.Scan(&est, &cnt); err != nil {
				return err
			}
			res.PorEstado[est] = cnt
			res.Total += cnt
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if campoDinero != "" {
			qSuma := fmt.Sprintf(`
				SELECT COALESCE(SUM((data->>'%s')::bigint), 0)
				FROM records
				WHERE tenant = $1 AND model = $2 AND deleted_at IS NULL;
			`, campoDinero)
			_ = tx.QueryRow(ctx, qSuma, tenant, m.Clave).Scan(&res.SumaDinero)
		}
		return nil
	})
	return res, err
}

// Duplicar clona un registro existente como borrador sin su referencia previa.
func (s *Store) Duplicar(ctx context.Context, m *catalogo.Modelo, id, ownerSub string) (Registro, error) {
	orig, err := s.Obtener(ctx, m.Clave, id)
	if err != nil {
		return Registro{}, err
	}

	copiaData := make(map[string]any, len(orig.Data))
	for k, v := range orig.Data {
		copiaData[k] = v
	}
	delete(copiaData, "ref")
	delete(copiaData, "externo_id")
	for _, clave := range identidadTPV {
		delete(copiaData, clave)
	}
	delete(copiaData, "sesion")
	delete(copiaData, "medio_pago")

	etiqueta := orig.Etiqueta
	if etiqueta != "" && !strings.HasSuffix(etiqueta, " (copia)") {
		etiqueta += " (copia)"
	}

	return s.Crear(ctx, m, copiaData, etiqueta, ownerSub)
}
