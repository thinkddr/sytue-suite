// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"math"
	"sort"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// UmbralDeTema, MinimoPorTema y TopeDeTemas definen los montones que propone la
// galería sin que nadie los pida.
//
// 0,85 NO ES EL 0,97 DE LAS REPETIDAS, y la diferencia es la función entera: a
// 0,97 un montón son dos disparos de la misma ráfaga; a 0,85 son las fotos de
// la misma playa en días distintos, que es lo que alguien llamaría «un tema».
// Medimos 0,98 dentro de un tema y 0,72 entre temas: por debajo de 0,80 los
// montones empiezan a mezclar y aparece el de «fotos», que no significa nada.
//
// CUATRO FOTOS COMO MÍNIMO. Dos fotos parecidas son dos fotos parecidas; un
// tema es algo de lo que hay un puñado. Sin ese suelo, la pantalla se llena de
// parejas y esconde los tres montones que sí valen.
//
// DOCE MONTONES COMO MUCHO, porque esto PROPONE. Una lista de cincuenta montones
// es otra galería, y quien la ve la cierra.
const (
	UmbralDeTema  = 0.85
	MinimoPorTema = 4
	TopeDeTemas   = 12
)

// Temas agrupa las fotos de esa persona por parecido, sin mirar el reloj.
//
// Es lo contrario de `Duplicadas`, que agrupa lo que sobra: aquí se agrupa lo
// que va junto. Por eso no hay ventana de tiempo —dos fotos de la misma playa
// separadas por dos veranos son el mismo tema, y en las repetidas serían
// justamente lo que NO se junta.
func (s *Store) Temas(ctx context.Context, duenyo string, limite int) ([]Grupo, error) {
	if limite <= 0 || limite > TopeDeTemas {
		limite = TopeDeTemas
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

		grupos = agruparPorTema(fotos, limite)
		return nil
	})
	return grupos, err
}

// monton es un tema a medio hacer: sus fotos y la SUMA de sus vectores.
//
// Se guarda la suma y no la media porque añadir una foto es entonces sumar, y
// el coseno contra el centro sale de dividir por la norma una sola vez. Con la
// media habría que rehacerla entera en cada comparación.
type monton struct {
	fotos []domain.Activo
	suma  []float64
	norma float64
}

// agruparPorTema junta cada foto con el montón al que más se parece.
//
// UNA SOLA PASADA, cada foto al montón que más se le parece y por encima del
// umbral, o a uno nuevo. No es k-medias y no lo pretende: k-medias pide saber
// cuántos temas hay ANTES de mirar, que es justo lo que aquí no se sabe, y a
// cambio de dos o tres pasadas más da montones que cambian de una vez a otra
// según dónde caigan los centros iniciales. Aquí el resultado es el mismo
// siempre para las mismas fotos, que en una pantalla que propone importa más
// que el montón perfecto.
//
// ponytail: es O(fotos × montones × 1152). Con el tope de dos mil fotos y una
// docena larga de montones son decenas de milisegundos, y esto se abre a
// petición, no al pintar la galería. Si algún día son cientos de miles, el
// sitio de arreglarlo es el mismo que el de `Parecidas`: un índice vectorial.
func agruparPorTema(fotos []conVector, limite int) []Grupo {
	var montones []*monton
	for _, f := range fotos {
		mejor, mejorParecido := -1, float32(UmbralDeTema)
		for i, m := range montones {
			if len(m.suma) != len(f.v) || m.norma == 0 {
				continue // otro tamaño es otro modelo: no se compara
			}
			// `>` y no `>=`: a igualdad se queda el montón más antiguo, que es
			// lo que hace que el resultado no dependa del orden de la lista.
			if p := parecidoConSuma(f.v, m.suma, m.norma); p > mejorParecido {
				mejor, mejorParecido = i, p
			}
		}
		if mejor < 0 {
			suma := make([]float64, len(f.v))
			for i, x := range f.v {
				suma[i] = float64(x)
			}
			montones = append(montones, &monton{fotos: []domain.Activo{f.a}, suma: suma, norma: norma(suma)})
			continue
		}
		m := montones[mejor]
		m.fotos = append(m.fotos, f.a)
		for i, x := range f.v {
			m.suma[i] += float64(x)
		}
		m.norma = norma(m.suma)
	}

	sobreviven := make([]*monton, 0, len(montones))
	for _, m := range montones {
		if len(m.fotos) >= MinimoPorTema {
			sobreviven = append(sobreviven, m)
		}
	}
	// De más fotos a menos: el montón grande es el que se reconoce de un
	// vistazo. `SliceStable` para que a igualdad mande el orden de llegada, que
	// es el de la foto más reciente.
	sort.SliceStable(sobreviven, func(i, j int) bool {
		return len(sobreviven[i].fotos) > len(sobreviven[j].fotos)
	})
	if len(sobreviven) > limite {
		sobreviven = sobreviven[:limite]
	}
	grupos := make([]Grupo, len(sobreviven))
	for i, m := range sobreviven {
		grupos[i] = Grupo{Fotos: m.fotos}
	}
	return grupos
}

// parecidoConSuma es el coseno entre un vector UNITARIO y el centro de un
// montón, sin llegar a construir el centro.
func parecidoConSuma(v []float32, suma []float64, norma float64) float32 {
	var acumulado float64
	for i, x := range v {
		acumulado += float64(x) * suma[i]
	}
	return float32(acumulado / norma)
}

func norma(v []float64) float64 {
	var suma float64
	for _, x := range v {
		suma += x * x
	}
	return math.Sqrt(suma)
}
