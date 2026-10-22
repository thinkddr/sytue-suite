// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// EL VECTOR NO ESTÁ EN `colsBase`, y no es un olvido.
//
// Son 1152 números por foto, unos 4,6 KB. Meterlo en la lista de columnas que
// usa la línea de tiempo multiplicaría por cien lo que viaja al pintar una
// página de cien miniaturas, para un dato que esa pantalla no usa. Se lee solo
// donde hace falta, que es aquí.

// Pendiente es una foto que todavía no tiene vector, con lo justo para que el
// lote pueda ir a buscar su miniatura: de qué organización es y qué huella
// nombra su derivada.
type Pendiente struct {
	ID     uuid.UUID
	Tenant string
	Duenyo string
	Huella string
}

// ExcluidaDelLoteIA dice si la organización de la transacción ha pedido quedar
// fuera del lote automático de IA. Sin fila en `org_settings` es false.
func (s *Store) ExcluidaDelLoteIA(ctx context.Context) (bool, error) {
	var excluida bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			SELECT exclude_ai_batch FROM org_settings WHERE tenant = $1`, tenant).Scan(&excluida)
		if errors.Is(err, pgx.ErrNoRows) {
			excluida = false
			return nil
		}
		return err
	})
	return excluida, err
}

// FijarExcluidaDelLoteIA activa o desactiva la exclusión de la organización en
// el lote automático de IA.
func (s *Store) FijarExcluidaDelLoteIA(ctx context.Context, quien string, excluida bool) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO org_settings (tenant, exclude_ai_batch, updated_by, updated_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (tenant) DO UPDATE SET
			    exclude_ai_batch = EXCLUDED.exclude_ai_batch,
			    updated_by       = EXCLUDED.updated_by,
			    updated_at       = now()`, tenant, excluida, quien)
		return err
	})
}

// TenantExcluidoDelLoteIA comprueba en una transacción de mantenimiento si una
// organización se ha excluido del lote mientras el lote está en marcha.
func (s *Store) TenantExcluidoDelLoteIA(ctx context.Context, tenant string) (bool, error) {
	var excluida bool
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT exclude_ai_batch FROM org_settings WHERE tenant = $1`, tenant).Scan(&excluida)
		if errors.Is(err, pgx.ErrNoRows) {
			excluida = false
			return nil
		}
		return err
	})
	return excluida, err
}

// PendientesDeVector lista lo que falta por vectorizar, de TODAS las
// organizaciones.
//
// Va en transacción de MANTENIMIENTO por lo mismo que la purga de la papelera:
// el lote no es una petición de nadie, así que no hay `app.tenant` que fijar y
// la política de aislamiento no le dejaría ver una sola fila. Sin eso, el lote
// diría «nada que hacer» para siempre sin que nada fallara.
//
// `modelo` filtra por el que se va a usar: al cambiar de modelo, lo vectorizado
// con el anterior vuelve a estar pendiente, porque dos vectores de modelos
// distintos viven en espacios distintos y compararlos da parecidos inventados.
func (s *Store) PendientesDeVector(ctx context.Context, modelo string, limite int) ([]Pendiente, error) {
	if limite <= 0 || limite > 20000 {
		limite = 5000
	}
	var out []Pendiente
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.tenant, a.owner_sub, coalesce(e.fingerprint, '')
			  FROM assets a
			  LEFT JOIN edits e
			    ON e.tenant = a.tenant AND e.asset_id = a.id AND e.owner_sub = a.owner_sub
			 WHERE a.deleted_at IS NULL
			   AND (a.embedding IS NULL OR a.embedding_model <> $1)
			   AND NOT EXISTS (
			       SELECT 1 FROM org_settings os
			        WHERE os.tenant = a.tenant AND os.exclude_ai_batch
			   )
			 ORDER BY a.uploaded_at DESC
			 LIMIT $2`, modelo, limite)
		if err != nil {
			return err
		}
		defer rows.Close()
		out = out[:0]
		for rows.Next() {
			var p Pendiente
			if err := rows.Scan(&p.ID, &p.Tenant, &p.Duenyo, &p.Huella); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// GuardarVector fija el vector de una foto y con qué modelo se sacó.
//
// NORMALIZA A LONGITUD 1 al guardar. Con los vectores normalizados, parecerse
// es el producto escalar y no hace falta dividir por las normas en cada
// comparación: la búsqueda pasa de tres bucles a uno. Y como se hace una sola
// vez por foto, no se paga en cada consulta.
//
// Un vector de ceros se rechaza: no tiene dirección, así que no se parece a
// nada ni deja de parecerse, y guardarlo sería guardar un dato que envenena
// cualquier orden.
func (s *Store) GuardarVector(ctx context.Context, id uuid.UUID, modelo string, v []float32) error {
	if modelo == "" {
		return fmt.Errorf("%w: un vector sin modelo no se puede comparar con nada", domain.ErrInvalidInput)
	}
	unitario, err := normalizar(v)
	if err != nil {
		return err
	}
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE assets a SET embedding = $2, embedding_model = $3, updated_at = now()
			WHERE a.id = $1 AND a.deleted_at IS NULL
			  AND NOT EXISTS (
			      SELECT 1 FROM org_settings os
			       WHERE os.tenant = a.tenant AND os.exclude_ai_batch
			  )`, id, unitario, modelo)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// normalizar devuelve el vector con longitud 1.
func normalizar(v []float32) ([]float32, error) {
	if len(v) == 0 {
		return nil, fmt.Errorf("%w: vector vacío", domain.ErrInvalidInput)
	}
	var suma float64
	for _, x := range v {
		f := float64(x)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("%w: el vector trae un número que no lo es", domain.ErrInvalidInput)
		}
		suma += f * f
	}
	norma := math.Sqrt(suma)
	if norma == 0 {
		return nil, fmt.Errorf("%w: un vector de ceros no apunta a ninguna parte", domain.ErrInvalidInput)
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / norma)
	}
	return out, nil
}

// TopeDeComparacion acota cuántas fotos entran en una búsqueda por parecido.
//
// Sin índice vectorial, parecerse se calcula contra todas: 1152 números por
// foto son 4,6 KB, así que dos mil fotos son nueve megas que viajan y se
// recorren en cada consulta. A esta escala es coste de milisegundos; a partir
// de ahí, el sitio de arreglarlo es pgvector con su índice aproximado, y por eso
// el vector se guarda ya normalizado: la migración no obliga a recalcular nada.
const TopeDeComparacion = 2000

// Parecidas devuelve las fotos de esa persona más parecidas a la dada, de más a
// menos, sin incluirla a ella.
//
// SOLO COMPARA LO DEL MISMO MODELO. Mezclar espacios distintos no da un
// resultado peor: da un orden sin sentido con pinta de resultado.
func (s *Store) Parecidas(ctx context.Context, id uuid.UUID, duenyo string, limite int) ([]domain.Activo, error) {
	if limite <= 0 || limite > 100 {
		limite = 24
	}
	var out []domain.Activo
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		var refe []float32
		var modelo string
		if err := tx.QueryRow(ctx, `
			SELECT embedding, embedding_model FROM assets
			WHERE tenant = $1 AND id = $2 AND owner_sub = $3 AND deleted_at IS NULL`,
			tenant, id, duenyo).Scan(&refe, &modelo); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		// Sin vector no hay a qué parecerse. No es un error: es una foto que el
		// lote todavía no ha mirado, y la interfaz lo dice con una lista vacía.
		if len(refe) == 0 {
			return nil
		}

		rows, err := tx.Query(ctx, `
			SELECT `+cols+`, a.embedding
			  FROM assets a
			 WHERE a.tenant = $1 AND a.owner_sub = $2 AND a.deleted_at IS NULL
			   AND a.id <> $3 AND a.embedding_model = $4 AND a.embedding IS NOT NULL
			 ORDER BY coalesce(a.taken_at, a.uploaded_at) DESC
			 LIMIT $5`, tenant, duenyo, id, modelo, TopeDeComparacion)
		if err != nil {
			return err
		}
		defer rows.Close()

		var todas []puntuada
		for rows.Next() {
			var a domain.Activo
			var orient int16
			var v []float32
			if err := rows.Scan(&a.ID, &a.Duenyo, &a.DriveID, &a.Nombre, &a.Tipo, &a.Bytes,
				&a.Ancho, &a.Alto, &orient, &a.TomadaEn, &a.SubidaEn, &a.Favorita, &a.Camara,
				&a.Objetivo, &a.Lat, &a.Lon, &a.Descripcion, &a.Etiquetas, &v); err != nil {
				return err
			}
			if len(v) != len(refe) {
				continue // otro tamaño es otro modelo: no se compara
			}
			a.Orientacion = domain.OrientacionDe(int(orient))
			a.Tenant = tenant
			todas = append(todas, puntuada{a, parecido(refe, v)})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		out = mejorPuntuadas(todas, limite)
		return nil
	})
	return out, err
}

// parecido es el producto escalar, que con vectores normalizados ES el coseno.
func parecido(a, b []float32) float32 {
	var suma float32
	for i := range a {
		suma += a[i] * b[i]
	}
	return suma
}

// TopeDeSemillas y UmbralDeAmpliacion definen «más como estas».
//
// DOCE SEMILLAS BASTAN. El centroide de doce vectores ya describe un tema; a
// partir de ahí cada foto que se añade lo mueve menos y solo alarga la lectura.
// Y quien marca ochenta fotos no está pidiendo un tema, está pidiendo su
// galería otra vez.
//
// EL UMBRAL ES LO QUE HACE HONESTA A ESTA PANTALLA. Sin él siempre devolvería
// veinticuatro fotos —las menos malas— aunque ninguna se parezca a nada, y una
// lista siempre llena es una lista en la que no se puede confiar. Con SigLIP 2
// medimos 0,98 dentro de un mismo tema y 0,72 entre temas distintos; 0,80 cae
// entre los dos y del lado prudente, porque el centroide de varias fotos baja
// los parecidos de todas (apunta al promedio, no a ninguna). Preferimos que
// falte una foto a que sobren cinco: quien ve ruido deja de mirar la sección.
const (
	TopeDeSemillas     = 12
	UmbralDeAmpliacion = 0.80
)

// ParecidasAVarias devuelve las fotos que se parecen al CONJUNTO de las dadas,
// sin incluirlas a ellas.
//
// Es lo que convierte describir unas pocas en encontrar el resto: la búsqueda
// literal solo saca lo que alguien escribió, y desde esos aciertos esto llega a
// las que nadie describió nunca. Por eso vale igual para ampliar una búsqueda
// que para el «más como estas» de la selección: en los dos casos la pregunta es
// la misma y las fotos de partida ya se saben.
//
// EL CENTROIDE ES LA MEDIA DE LOS VECTORES, no una consulta por cada semilla
// con las listas pegadas después. Pegar listas devuelve lo más parecido a CADA
// foto, que con semillas variadas es un revoltijo de temas; la media apunta a
// puntuarCandidatas lee las candidatas y se queda con las que pasan el umbral.
//
// Extraída por lo mismo que `sumarSemillas`: el bucle con su scan de dieciocho
// columnas y sus dos filtros era la otra mitad de la complejidad de
// `ParecidasAVarias`, y no tiene nada que ver con decidir qué se consulta.
//
// Un vector de otra longitud se SALTA en vez de fallar: es una foto vectorizada
// con otro modelo, y compararla daría un número sin significado.
func puntuarCandidatas(cand pgx.Rows, refe []float32, tenant string) ([]puntuada, error) {
	var todas []puntuada
	for cand.Next() {
		var a domain.Activo
		var orient int16
		var v []float32
		if err := cand.Scan(&a.ID, &a.Duenyo, &a.DriveID, &a.Nombre, &a.Tipo, &a.Bytes,
			&a.Ancho, &a.Alto, &orient, &a.TomadaEn, &a.SubidaEn, &a.Favorita, &a.Camara,
			&a.Objetivo, &a.Lat, &a.Lon, &a.Descripcion, &a.Etiquetas, &v); err != nil {
			return nil, err
		}
		if len(v) != len(refe) {
			continue
		}
		p := parecido(refe, v)
		if p < UmbralDeAmpliacion {
			continue
		}
		a.Orientacion = domain.OrientacionDe(int(orient))
		a.Tenant = tenant
		todas = append(todas, puntuada{a, p})
	}
	return todas, cand.Err()
}

// puntuada es una foto candidata con su parecido al centroide.
type puntuada struct {
	a domain.Activo
	p float32
}

// mejorPuntuadas ordena de más a menos parecido y recorta al límite.
//
// FUERA DE LA TRANSACCIÓN PARA PODER PROBARLA, como `agruparParecidas` y
// `masCercanas`: lo que decide el orden de la pantalla no debería exigir
// Postgres para comprobarlo.
//
// `SliceStable` y no `Slice`: dos fotos con el MISMO parecido —lo normal en una
// ráfaga— cambiarían de orden entre dos llamadas idénticas, y la galería
// parecería barajarse sola al recargar.
func mejorPuntuadas(todas []puntuada, limite int) []domain.Activo {
	sort.SliceStable(todas, func(i, j int) bool { return todas[i].p > todas[j].p })
	if len(todas) > limite {
		todas = todas[:limite]
	}
	out := make([]domain.Activo, len(todas))
	for i, t := range todas {
		out[i] = t.a
	}
	return out
}

// sumarSemillas acumula los vectores de las semillas en un centroide sin
// normalizar, y devuelve con qué modelo se calculó.
//
// FUERA DE LA TRANSACCIÓN, como el resto de la lógica de esta familia: aquí
// estaba la mitad de la complejidad de `ParecidasAVarias` —un bucle con cuatro
// ramas dentro de una closure dentro de un método—, y mezclada con el manejo de
// la conexión.
//
// MANDA EL PRIMER MODELO y los demás se ignoran: mezclar vectores de modelos
// distintos no da un resultado peor, da un orden sin sentido con pinta de
// resultado.
//
// Modelo vacío al salir = ninguna semilla tenía vector todavía. No es un error:
// son fotos que el lote aún no ha mirado.
func sumarSemillas(rows pgx.Rows) (modelo string, centro []float64, err error) {
	for rows.Next() {
		var v []float32
		var m string
		if err := rows.Scan(&v, &m); err != nil {
			return "", nil, err
		}
		if modelo == "" {
			modelo, centro = m, make([]float64, len(v))
		}
		if m != modelo || len(v) != len(centro) {
			continue
		}
		for i, x := range v {
			centro[i] += float64(x)
		}
	}
	return modelo, centro, rows.Err()
}

// lo que tienen EN COMÚN, que es lo que se está pidiendo.
func (s *Store) ParecidasAVarias(ctx context.Context, duenyo string, semillas []uuid.UUID, limite int) ([]domain.Activo, error) {
	if len(semillas) == 0 {
		return nil, fmt.Errorf("%w: sin fotos de partida no hay a qué parecerse", domain.ErrInvalidInput)
	}
	if len(semillas) > TopeDeSemillas {
		semillas = semillas[:TopeDeSemillas]
	}
	if limite <= 0 || limite > 100 {
		limite = 24
	}
	var out []domain.Activo
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		// El filtro por dueño va aquí y no en la comprobación de después: una
		// foto de otra persona no es que no sirva de semilla, es que no existe
		// para quien pregunta, y colarla en el centroide diría algo de su
		// contenido sin haberla enseñado nunca.
		rows, err := tx.Query(ctx, `
			SELECT embedding, embedding_model FROM assets
			 WHERE tenant = $1 AND owner_sub = $2 AND id = ANY($3)
			   AND deleted_at IS NULL AND embedding IS NOT NULL`, tenant, duenyo, semillas)
		if err != nil {
			return err
		}
		modelo, centro, err := sumarSemillas(rows)
		rows.Close()
		if err != nil {
			return err
		}
		// Ninguna semilla tiene vector todavía. No es un error: son fotos que el
		// lote aún no ha mirado, y la interfaz lo dice con una lista vacía.
		if modelo == "" {
			return nil
		}
		refe, err := normalizarF64(centro)
		if err != nil {
			return nil // vectores opuestos que se anulan: no apuntan a nada
		}

		cand, err := tx.Query(ctx, `
			SELECT `+cols+`, a.embedding
			  FROM assets a
			 WHERE a.tenant = $1 AND a.owner_sub = $2 AND a.deleted_at IS NULL
			   AND a.id <> ALL($3) AND a.embedding_model = $4 AND a.embedding IS NOT NULL
			 ORDER BY coalesce(a.taken_at, a.uploaded_at) DESC
			 LIMIT $5`, tenant, duenyo, semillas, modelo, TopeDeComparacion)
		if err != nil {
			return err
		}
		defer cand.Close()

		todas, err := puntuarCandidatas(cand, refe, tenant)
		if err != nil {
			return err
		}
		sort.SliceStable(todas, func(i, j int) bool { return todas[i].p > todas[j].p })
		if len(todas) > limite {
			todas = todas[:limite]
		}
		out = make([]domain.Activo, len(todas))
		for i, t := range todas {
			out[i] = t.a
		}
		return nil
	})
	return out, err
}

// normalizarF64 es `normalizar` para el acumulador del centroide, que suma en
// doble precisión porque doce sumas de 1152 términos en float32 arrastran error
// justo donde se compara con un umbral.
func normalizarF64(v []float64) ([]float32, error) {
	en32 := make([]float32, len(v))
	for i, x := range v {
		en32[i] = float32(x)
	}
	return normalizar(en32)
}

// UmbralDeDuplicada y VentanaDeRafaga definen qué cuenta como «repetida».
//
// 0,97 de coseno no es «parecidas»: es la misma escena. Con SigLIP 2, dos fotos
// distintas de la misma playa se quedan en 0,85-0,92; pasar de 0,97 son dos
// disparos de la misma ráfaga o dos intentos del mismo encuadre. Bajarlo
// empieza a proponer borrar fotos que no sobran, que es el error caro: quien
// pierde una foto por hacer caso a esta pantalla no vuelve a usarla.
//
// LA VENTANA DE TIEMPO NO ES UNA OPTIMIZACIÓN, ES PARTE DE LA DEFINICIÓN. Lo
// que sobra son las ráfagas y los reintentos del mismo momento; dos fotos casi
// idénticas separadas por dos años son la misma vista en dos viajes, y eso no
// se borra. Además evita comparar todas contra todas: con la lista ordenada por
// fecha, cada foto solo mira a sus vecinas.
const (
	UmbralDeDuplicada = 0.97
	VentanaDeRafaga   = 10 * time.Minute
)

// Grupo son fotos que se repiten entre sí, de la más reciente a la más antigua.
type Grupo struct {
	Fotos []domain.Activo
}

// Duplicadas agrupa las fotos que sobran: ráfagas y reintentos del mismo
// momento. Solo mira las que tienen vector del mismo modelo.
//
// conVector es una foto con su vector, tal como sale de la consulta.
type conVector struct {
	a domain.Activo
	v []float32
}

// agruparParecidas junta en un grupo las fotos que se parecen y están seguidas
// en el tiempo. `fotos` tiene que venir ORDENADA por fecha descendente.
//
// SEPARADA DE LA CONSULTA PARA PODER PROBARLA. Es la lógica que decide qué es
// un duplicado —el umbral de parecido y la ventana de ráfaga— y estaba dentro
// de la transacción, así que comprobarla exigía levantar Postgres. Aquí se
// prueba con dos vectores escritos a mano.
//
// La lista viene ordenada por fecha, así que las candidatas de cada foto son
// las que vienen justo detrás mientras quepan en la ventana. En cuanto una se
// sale, las siguientes también: se corta y a la próxima.
func agruparParecidas(fotos []conVector, limite int) []Grupo {
	var grupos []Grupo
	usada := make([]bool, len(fotos))
	// Se recorre con la variable del rango y no indexando `fotos[i]`: dice lo
	// mismo, se lee mejor y el analizador no tiene que demostrar que el índice
	// cabe —dentro de la closure original podía; extraída la función, no—.
	for i, foto := range fotos {
		if usada[i] {
			continue
		}
		cuando, _ := foto.a.FechaDeGaleria()
		grupo := []domain.Activo{foto.a}
		for j := i + 1; j < len(fotos); j++ {
			otra := fotos[j]
			cuandoOtra, _ := otra.a.FechaDeGaleria()
			if cuando.Sub(cuandoOtra) > VentanaDeRafaga {
				break
			}
			if usada[j] || len(otra.v) != len(foto.v) {
				continue
			}
			if parecido(foto.v, otra.v) >= UmbralDeDuplicada {
				usada[j] = true
				grupo = append(grupo, otra.a)
			}
		}
		if len(grupo) > 1 {
			usada[i] = true
			grupos = append(grupos, Grupo{Fotos: grupo})
			if len(grupos) >= limite {
				return grupos
			}
		}
	}
	return grupos
}

// Devuelve grupos de DOS O MÁS. Un grupo de una no es un duplicado, es una foto.
func (s *Store) Duplicadas(ctx context.Context, duenyo string, limite int) ([]Grupo, error) {
	if limite <= 0 || limite > 200 {
		limite = 50
	}
	var grupos []Grupo
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+cols+`, a.embedding
			  FROM assets a
			 WHERE a.tenant = $1 AND a.owner_sub = $2 AND a.deleted_at IS NULL
			   AND a.embedding IS NOT NULL
			 ORDER BY coalesce(a.taken_at, a.uploaded_at) DESC, a.id DESC
			 LIMIT $3`, tenant, duenyo, TopeDeComparacion)
		if err != nil {
			return err
		}
		defer rows.Close()

		var fotos []conVector
		for rows.Next() {
			var a domain.Activo
			var orient int16
			var v []float32
			if err := rows.Scan(&a.ID, &a.Duenyo, &a.DriveID, &a.Nombre, &a.Tipo, &a.Bytes,
				&a.Ancho, &a.Alto, &orient, &a.TomadaEn, &a.SubidaEn, &a.Favorita, &a.Camara,
				&a.Objetivo, &a.Lat, &a.Lon, &a.Descripcion, &a.Etiquetas, &v); err != nil {
				return err
			}
			a.Orientacion = domain.OrientacionDe(int(orient))
			a.Tenant = tenant
			fotos = append(fotos, conVector{a, v})
		}
		if err := rows.Err(); err != nil {
			return err
		}

		grupos = agruparParecidas(fotos, limite)
		return nil
	})
	return grupos, err
}
