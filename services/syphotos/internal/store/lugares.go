// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"math"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// RadioDeCerca es hasta dónde llega «por aquí», en kilómetros.
//
// DOS KILÓMETROS ES EL MISMO SITIO: una playa, un parque, un barrio. Con diez ya
// es la misma ciudad, y quien pulsa sobre las coordenadas de una foto concreta
// no está preguntando «qué tengo de Valencia», está preguntando «qué más hice
// aquí». Ampliarlo no da más respuesta: da otra pregunta.
const RadioDeCerca = 2.0

// TopeDeCerca acota la respuesta. Un sitio donde alguien ha hecho más de cien
// fotos no necesita la ciento una para contar lo que pasó allí.
const TopeDeCerca = 100

// CercaDe devuelve las fotos de esa persona hechas cerca de la dada, de la más
// próxima a la más lejana, sin incluirla a ella.
//
// El filtro grueso lo hace Postgres con una CAJA de latitud y longitud, y la
// distancia de verdad se calcula aquí. La caja sobra por las esquinas —es un
// cuadrado alrededor de un círculo— y por eso se descarta después: sin ella
// habría que traerse la biblioteca entera para medir; sin el cálculo de
// después, «cerca» sería un rectángulo, que a estas latitudes es un 40 % más
// ancho que alto.
//
// Una foto SIN coordenadas no es un error: es la mayoría de las fotos de una
// medida es una foto candidata con su distancia a la de referencia.
type medida struct {
	a  domain.Activo
	km float64
}

// masCercanas ordena por distancia y recorta al límite.
//
// FUERA DE LA TRANSACCIÓN PARA PODER PROBARLA, igual que `agruparParecidas`: lo
// que decide qué sale en pantalla —el orden y el corte— no debería exigir
// levantar Postgres para comprobarlo.
//
// `SliceStable` y no `Slice`: dos fotos a la MISMA distancia —lo normal en una
// ráfaga, que comparte posición— cambiarían de orden entre dos llamadas
// idénticas, y la galería parecería barajarse sola al recargar.
func masCercanas(cerca []medida, limite int) []domain.Activo {
	sort.SliceStable(cerca, func(i, j int) bool { return cerca[i].km < cerca[j].km })
	if len(cerca) > limite {
		cerca = cerca[:limite]
	}
	out := make([]domain.Activo, len(cerca))
	for i, m := range cerca {
		out[i] = m.a
	}
	return out
}

// cámara sin GPS, y la respuesta es una lista vacía.
func (s *Store) CercaDe(ctx context.Context, id uuid.UUID, duenyo string, limite int) ([]domain.Activo, error) {
	if limite <= 0 || limite > TopeDeCerca {
		limite = 48
	}
	var out []domain.Activo
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		var lat, lon *float64
		if err := tx.QueryRow(ctx, `
			SELECT latitude, longitude FROM assets
			WHERE tenant = $1 AND id = $2 AND owner_sub = $3 AND deleted_at IS NULL`,
			tenant, id, duenyo).Scan(&lat, &lon); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if lat == nil || lon == nil {
			return nil
		}

		dLat, dLon := caja(*lat, RadioDeCerca)
		rows, err := tx.Query(ctx, `
			SELECT `+cols+`
			  FROM assets a
			 WHERE a.tenant = $1 AND a.owner_sub = $2 AND a.deleted_at IS NULL
			   AND a.id <> $3 AND a.latitude IS NOT NULL
			   AND a.latitude BETWEEN $4 AND $5
			   AND a.longitude BETWEEN $6 AND $7
			 ORDER BY coalesce(a.taken_at, a.uploaded_at) DESC
			 LIMIT $8`,
			tenant, duenyo, id, *lat-dLat, *lat+dLat, *lon-dLon, *lon+dLon, TopeDeCerca*4)
		if err != nil {
			return err
		}
		defer rows.Close()

		var cerca []medida
		for rows.Next() {
			var a domain.Activo
			var orient int16
			if err := rows.Scan(&a.ID, &a.Duenyo, &a.DriveID, &a.Nombre, &a.Tipo, &a.Bytes,
				&a.Ancho, &a.Alto, &orient, &a.TomadaEn, &a.SubidaEn, &a.Favorita, &a.Camara,
				&a.Objetivo, &a.Lat, &a.Lon, &a.Descripcion, &a.Etiquetas); err != nil {
				return err
			}
			if a.Lat == nil || a.Lon == nil {
				continue
			}
			km := distanciaKm(*lat, *lon, *a.Lat, *a.Lon)
			if km > RadioDeCerca {
				continue // esquina de la caja: dentro del cuadrado, fuera del círculo
			}
			a.Orientacion = domain.OrientacionDe(int(orient))
			a.Tenant = tenant
			cerca = append(cerca, medida{a, km})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		out = masCercanas(cerca, limite)
		return nil
	})
	return out, err
}

// caja devuelve cuántos grados de latitud y de longitud hay que sumar y restar
// para que el cuadrado contenga el círculo de ese radio.
//
// Un grado de latitud son siempre ~111,32 km; uno de longitud, esos mismos
// kilómetros MULTIPLICADOS POR EL COSENO de la latitud, porque los meridianos
// se juntan al subir. En Madrid un grado de longitud son 82 km y en Oslo, 51:
// usar el mismo delta para los dos ejes dejaría fuera media respuesta en el
// norte.
func caja(lat, km float64) (dLat, dLon float64) {
	const kmPorGrado = 111.32
	dLat = km / kmPorGrado
	// Cerca de los polos el coseno tiende a cero y el delta se dispara; se
	// acota para no acabar pidiendo el mundo entero por un error de redondeo.
	cos := math.Cos(lat * math.Pi / 180)
	if cos < 0.01 {
		cos = 0.01
	}
	return dLat, km / (kmPorGrado * cos)
}

// distanciaKm es la fórmula del haversine sobre una Tierra esférica.
//
// El error frente a un elipsoide es de un 0,3 %: seis metros en dos kilómetros,
// cuando el GPS de un móvil acierta a once. Aquí no hace falta más.
func distanciaKm(lat1, lon1, lat2, lon2 float64) float64 {
	const radioTierra = 6371.0
	rad := func(g float64) float64 { return g * math.Pi / 180 }
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return radioTierra * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
