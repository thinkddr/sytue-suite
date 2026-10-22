// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package merge mueve los datos de una organización a otra en los diez
// servicios de la suite.
//
// POR QUÉ VIVE EN SYTUE ID Y NO EN CADA SERVICIO. Todas las bases lógicas
// (`id`, `mail`, `drive`, …) viven en la MISMA instancia RDB y las abre el mismo
// usuario: `02b-secretos.sh` materializa un único par de credenciales y solo
// cambia el nombre de la base en el DSN. Así que el DSN de cada servicio se
// deriva del propio de Sytue ID sin secretos nuevos, sin endpoints internos
// nuevos y sin diez despliegues coordinados. La alternativa —un endpoint de
// fusión por servicio— multiplicaba por diez el código y la superficie
// autenticada para mover exactamente las mismas filas con las mismas
// credenciales.
//
// CÓMO SOBREVIVE A RLS. Cada tabla de cada servicio lleva FORCE ROW LEVEL
// SECURITY con aislamiento por `app.tenant`. Una transacción normal NO ve las
// filas del origen, así que un UPDATE afectaría a cero filas y la fusión diría
// «hecho» sin mover nada. Por eso todo ocurre dentro de `pg.InMergeTx`
// (`app.merge='on'`) y por eso el preflight EXIGE que cada tabla con columna
// `tenant` tenga su política de fusión: sin ella el fallo sería mudo.
package merge

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	notesdb "git.sytue.eu/sytue/one/services/notes/db"
)

// Services son las bases lógicas que participan en una fusión, en el orden en
// que se recorren. El orden es estable para que un reintento repita la misma
// secuencia y el progreso guardado signifique siempre lo mismo.
//
// `analytics` va AL FINAL y no en su sitio alfabético: se incorporó después, y
// meterlo en medio habría cambiado el significado del progreso ya guardado de las
// fusiones en curso o fallidas. Añadir al final es lo único que no reescribe el
// pasado.
//
// `notes` va también AL FINAL, y por el mismo motivo: es un producto posterior y
// colocarlo en su sitio alfabético desplazaría el índice de todo lo que va detrás.
//
// Y `syphotos` detrás de todos, por lo mismo. Lo que mueve de esa base son los
// ÍNDICES —qué fotos hay, sus recetas de edición y sus álbumes—, no los ficheros:
// los originales están en Drive y se mueven con Drive, y las derivadas son caché
// regenerable que no hay que mover a ninguna parte.
//
// Y `syerp` el último, por la misma regla de no reescribir el pasado. Es el que
// más pesa de la lista: se lleva el ERP entero de la organización absorbida,
// incluido SU LIBRO DIARIO. Los asientos son inmutables, pero esa inmutabilidad
// prohíbe cambiarles diario, referencia, fecha o moneda —no el tenant—, así que
// pasan enteros sin reescribir contabilidad. Uno que se quedara atrás
// desaparecería del balance de la organización resultante.
var Services = []string{
	"mail", "drive", "docs", "calendar", "contacts", "rooms", "hub", "integrations", "cloud",
	"analytics", "notes", "syphotos", "syerp",
	// Sytue Cast va AL FINAL, no en su sitio «natural»: el progreso de una fusión
	// se guarda por posición en esta lista, y meterlo en medio reescribiría el de
	// las fusiones a medias. Entró cuando existió su primera tabla por tenant
	// (`videos`, migración 0001 de services/cast), que es lo que exige el test.
	"cast",
	// Sytue Web, detrás de Cast por la misma razón: entró con `sites`, su
	// primera tabla por tenant (migración 0001 de services/pages).
	"pages",
	// Sytue Bases, al final por la misma regla: entró con `bases`, su primera tabla por
	// tenant (migración 0001 de services/bases). Sus claves foráneas compuestas
	// (tenant, id) van DIFERIDAS (migración 0004) para que este bucle, que reescribe
	// tabla a tabla, no rompa una hija antes de llegar a su padre.
	"bases",
}

// ServiceResult es el resultado de mover un servicio.
type ServiceResult struct {
	Service string         `json:"service"`
	Rows    map[string]int `json:"rows"`
	Total   int            `json:"total"`
	// Prepared es lo que el gancho de la base decidió antes de mover, en prosa y
	// vacío si no hizo nada. Se registra en el log y en el progreso de la fusión:
	// descartar material de un cliente en silencio es exactamente lo que no se
	// puede hacer, aunque descartarlo sea lo correcto.
	Prepared string `json:"prepared,omitempty"`
}

// PrepareFunc es el gancho OPCIONAL que una base puede definir para arreglar,
// dentro de la propia transacción de fusión, lo que un simple `UPDATE tenant` no
// sabe resolver. Recibe (origen, destino) y devuelve una frase para el operador,
// o NULL si no tocó nada.
//
// Existe porque hay conflictos que no son ambiguos: una organización que
// desaparece deja material que NO tiene sentido mover, y moverlo choca con una
// restricción. El caso que lo motivó es la autoridad S/MIME de Sytue Mail, con
// PRIMARY KEY (tenant): si las dos organizaciones tienen la suya, el UPDATE
// aborta la fusión entera, y la respuesta correcta —descartar la del origen, que
// ya no firma nada y cuyo ancla nadie tiene instalado— es criptografía de Mail,
// no del motor de fusión.
//
// El gancho lo declara CADA BASE en una migración suya, junto a las tablas que
// conoce, y este paquete solo pregunta si existe. Así el motor sigue sin saber
// nada de ningún servicio, que es la razón por la que puede vivir en Sytue ID
// (ver el encabezado del paquete): ni endpoints nuevos, ni credenciales nuevas,
// ni un despliegue coordinado para cambiar una regla de un solo servicio.
const PrepareFunc = "platform_merge_prepare"

const (
	// La firma se comprueba con to_regprocedure, que devuelve NULL —sin error— si
	// la función no existe. Una base sin gancho no paga nada.
	prepareExistsSQL = `SELECT to_regprocedure('` + PrepareFunc + `(text,text)') IS NOT NULL`
	prepareCallSQL   = `SELECT ` + PrepareFunc + `($1, $2)`
)

// prepare ejecuta el gancho de la base, si lo hay, dentro de la transacción de
// fusión y ANTES de mover nada: lo suyo es justamente quitar de en medio lo que
// haría fallar al UPDATE.
func prepare(ctx context.Context, tx pgx.Tx, source, target string) (string, error) {
	var existe bool
	if err := tx.QueryRow(ctx, prepareExistsSQL).Scan(&existe); err != nil {
		return "", fmt.Errorf("merge: buscar %s: %w", PrepareFunc, err)
	}
	if !existe {
		return "", nil
	}
	var nota *string
	if err := tx.QueryRow(ctx, prepareCallSQL, source, target).Scan(&nota); err != nil {
		return "", fmt.Errorf("merge: %s: %w", PrepareFunc, err)
	}
	if nota == nil {
		return "", nil
	}
	return *nota, nil
}

// DSNForDatabase deriva el DSN de la base lógica de `service` a partir del DSN de
// Sytue ID cambiando ÚNICAMENTE el nombre de la base. Devuelve error si el DSN de
// partida no es una URL con base: construir a mano una cadena de conexión a
// partir de trozos es como se acaba apuntando a la base equivocada.
//
// El nombre de la base se deduce del de la PROPIA base de ID, no se supone: cada
// despliegue nombra las suyas de una forma y las nombra todas igual. Scaleway las
// llama como el servicio (`id`, `mail`: deploy/production/02b-secretos.sh) y el
// piloto les antepone `sytue_` (`sytue_id`, `sytue_mail`:
// deploy/helm/bin/deploy.sh). Suponer el primero dejó la fusión de
// organizaciones y «Borrar mi cuenta» sin conectar en el piloto; leer el prefijo
// de la base de ID sirve en los dos sin una segunda opción que pueda discrepar.
func DSNForDatabase(idDSN, service string) (string, error) {
	if !identSafe(service) {
		return "", fmt.Errorf("merge: nombre de servicio no seguro: %q", service)
	}
	u, err := url.Parse(idDSN)
	if err != nil {
		return "", fmt.Errorf("merge: DSN ilegible: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("merge: el DSN no es postgres://: %q", u.Scheme)
	}
	idBase := strings.Trim(u.Path, "/")
	if idBase == "" {
		return "", errors.New("merge: el DSN no indica base de datos")
	}
	prefijo, ok := strings.CutSuffix(idBase, "id")
	if !ok {
		return "", fmt.Errorf("merge: la base de Sytue ID (%q) no termina en «id»: no se sabe cómo se llaman las de los demás servicios", idBase)
	}
	database := prefijo + service
	if !identSafe(database) {
		return "", fmt.Errorf("merge: nombre de base no seguro: %q", database)
	}
	u.Path = "/" + database
	return u.String(), nil
}

// identSafe replica la validación de pg para nombres de base: solo minúsculas,
// dígitos y guion bajo. Los nombres vienen de Services, no del usuario, pero la
// comprobación evita que un cambio futuro abra una inyección por el DSN.
func identSafe(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c == '_', c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return s[0] < '0' || s[0] > '9'
}

// tenantTables devuelve las tablas con tenant y las que no podrían moverse. La
// consulta vive en platform/pkg/pg para que el guardián de cada servicio
// (pgtest.ExigirPoliticaDeFusion) compruebe EXACTAMENTE lo mismo que este preflight.
var tenantTables = pg.ConsultaTablasConTenant

// MoveService reescribe el tenant de todas las tablas de UNA base lógica, en una
// sola transacción: o se mueve el servicio entero o no se mueve nada de él. Es
// idempotente porque el criterio es `tenant = origen`: repetirlo tras un éxito
// mueve cero filas.
//
// Las colisiones de unicidad NO se resuelven en silencio. Si el destino ya tiene
// una fila que chocaría (misma dirección de correo, mismo nombre de grupo…), la
// transacción falla y la fusión queda en `failed` con la tabla y la restricción
// en el error. Inventar una desambiguación —renombrar, descartar, sufijar— sería
// perder datos de un cliente sin que nadie lo decidiera.
//
// Lo que SÍ puede resolverse es lo que no es ambiguo, y para eso está el gancho
// PrepareFunc: cada base declara en una migración suya qué hacer con el material
// que no tiene sentido mover, lo dice en prosa (ServiceResult.Prepared) y ocurre
// en esta misma transacción. La decisión la escribe quien conoce esos datos, no
// este bucle.
func MoveService(ctx context.Context, pool *pg.Pool, source, target string, revokers ...callaccess.Revoker) (ServiceResult, error) {
	res := ServiceResult{Rows: map[string]int{}}
	if source == target {
		return res, nil
	}
	err := pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		// Adopt the fixed binary contract before hooks or writes. It is harmless
		// in other service databases and never inferred from their name or data.
		if err := notesdb.MarkWriteTransaction(ctx, tx); err != nil {
			return fmt.Errorf("merge: notes write contract: %w", err)
		}
		tables, sinPolitica, err := tenantTables(ctx, tx)
		if err != nil {
			return err
		}
		if len(sinPolitica) > 0 {
			return fmt.Errorf("merge: sin política de fusión en %s: la reescritura sería muda",
				strings.Join(sinPolitica, ", "))
		}
		if res.Prepared, err = prepare(ctx, tx, source, target); err != nil {
			return err
		}
		for _, table := range tables {
			// El nombre de tabla viene del catálogo del propio esquema y se cita con
			// pgx.Identifier; los valores de tenant van SIEMPRE parametrizados.
			sql := fmt.Sprintf("UPDATE %s SET tenant = $1 WHERE tenant = $2",
				pgx.Identifier{table}.Sanitize())
			tag, err := tx.Exec(ctx, sql, target, source)
			if err != nil {
				return fmt.Errorf("merge: %s: %w", table, err)
			}
			if n := int(tag.RowsAffected()); n > 0 {
				res.Rows[table] = n
				res.Total += n
			}
		}
		return callaccess.Flush(ctx, tx, "", firstCallRevoker(revokers))
	})
	return res, err
}

// Leftover es una fila que TODAVÍA pertenece al origen después de mover.
type Leftover struct {
	Service string `json:"service"`
	Table   string `json:"table"`
	Rows    int    `json:"rows"`
}

// Verify cuenta lo que sigue siendo del origen tras mover los servicios. Devuelve
// la lista vacía cuando no quedó nada, que es el caso normal.
//
// POR QUÉ NO BASTA CON QUE MoveService NO DIERA ERROR. Ese bucle recorre las
// tablas que el catálogo listaba AL EMPEZAR su transacción, y declara el servicio
// movido si ningún UPDATE falló. Eso no es lo mismo que «no queda nada del
// origen»: una escritura que entrara entre el UPDATE de una tabla y el COMMIT del
// servicio —el origen sigue vivo y sus usuarios siguen trabajando hasta que se
// mueve la identidad— vuelve a dejar filas suyas detrás. Ninguna alarma salta,
// porque nada falló.
//
// La consulta va dentro de InMergeTx por la misma razón que la reescritura: sin
// `app.merge` la RLS filtra las filas del origen y el recuento sale cero. Un
// verificador que se salte eso no comprueba nada, y además CONFIRMA la fusión —es
// el peor resultado posible, porque convierte «no lo he mirado» en «está limpio».
func Verify(ctx context.Context, idDSN, source string) ([]Leftover, error) {
	var restos []Leftover
	for _, svc := range Services {
		dsn, err := DSNForDatabase(idDSN, svc)
		if err != nil {
			return nil, err
		}
		pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 1})
		if err != nil {
			return nil, fmt.Errorf("merge: verificar %s: %w", svc, err)
		}
		delSvc, err := LeftoversIn(ctx, pool, svc, source)
		pool.Close()
		if err != nil {
			return nil, fmt.Errorf("merge: verificar %s: %w", svc, err)
		}
		restos = append(restos, delSvc...)
	}
	return restos, nil
}

// LeftoversIn cuenta lo que sigue siendo del origen en UNA base. Está separada de
// Verify porque es la parte que se puede probar: un test levanta un esquema real
// y comprueba que una fila rezagada se ve, mientras que Verify solo repite esto
// diez veces sobre DSN que en un test no existen.
func LeftoversIn(ctx context.Context, pool *pg.Pool, service, source string) ([]Leftover, error) {
	var restos []Leftover
	err := pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		tables, _, err := tenantTables(ctx, tx)
		if err != nil {
			return err
		}
		for _, table := range tables {
			// Nombre del catálogo, citado; el tenant siempre parametrizado.
			sql := fmt.Sprintf("SELECT count(*) FROM %s WHERE tenant = $1",
				pgx.Identifier{table}.Sanitize())
			var n int
			if err := tx.QueryRow(ctx, sql, source).Scan(&n); err != nil {
				return fmt.Errorf("merge: contar %s: %w", table, err)
			}
			if n > 0 {
				restos = append(restos, Leftover{Service: service, Table: table, Rows: n})
			}
		}
		return nil
	})
	return restos, err
}

// LeftoversError describe los restos en una frase para el operador y para el
// campo de fallo de `org_merges`, que es lo que se ve en la consola.
func LeftoversError(restos []Leftover) error {
	partes := make([]string, 0, len(restos))
	total := 0
	for _, r := range restos {
		partes = append(partes, fmt.Sprintf("%s.%s (%d)", r.Service, r.Table, r.Rows))
		total += r.Rows
	}
	return fmt.Errorf("quedan %d filas del origen tras mover: %s", total, strings.Join(partes, ", "))
}

// Preflight comprueba, ANTES de mover nada, que las diez bases son alcanzables y
// que ninguna tabla se quedaría fuera por falta de política. Una fusión a medias
// es cara de deshacer; descubrir en el servicio noveno que falta una política lo
// sería mucho más.
func Preflight(ctx context.Context, idDSN string) error {
	var problemas []string
	for _, svc := range Services {
		dsn, err := DSNForDatabase(idDSN, svc)
		if err != nil {
			return err
		}
		pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 1})
		if err != nil {
			problemas = append(problemas, fmt.Sprintf("%s: sin conexión (%v)", svc, err))
			continue
		}
		_, sinPolitica, err := tenantTables(ctx, pool)
		pool.Close()
		switch {
		case err != nil:
			problemas = append(problemas, fmt.Sprintf("%s: %v", svc, err))
		case len(sinPolitica) > 0:
			sort.Strings(sinPolitica)
			problemas = append(problemas, fmt.Sprintf("%s: sin política de fusión en %s",
				svc, strings.Join(sinPolitica, ", ")))
		}
	}
	if len(problemas) > 0 {
		return fmt.Errorf("preflight de fusión: %s", strings.Join(problemas, "; "))
	}
	return nil
}
