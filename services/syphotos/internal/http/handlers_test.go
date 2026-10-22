// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// LAS DIMENSIONES SALEN YA ORIENTADAS. En la base se guardan crudas —es lo que
// dice el fichero— pero quien pinta necesita el tamaño que se VE. Si cada cliente
// aplicara la tabla EXIF por su cuenta, la web y el escritorio acabarían
// aplicándola distinto, que es el fallo clásico de las galerías.
func TestAJSON_orientaLasDimensiones(t *testing.T) {
	// Orientación 6: un cuarto de vuelta, así que 4000×3000 se ve 3000×4000.
	a := domain.Activo{ID: uuid.New(), Ancho: 4000, Alto: 3000, Orientacion: 6}
	j := aJSON(a)
	if j.Ancho != 3000 || j.Alto != 4000 {
		t.Errorf("salió %d×%d, se esperaba 3000×4000", j.Ancho, j.Alto)
	}

	sinGirar := domain.Activo{ID: uuid.New(), Ancho: 4000, Alto: 3000, Orientacion: 1}
	if j := aJSON(sinGirar); j.Ancho != 4000 || j.Alto != 3000 {
		t.Errorf("sin rotación salió %d×%d", j.Ancho, j.Alto)
	}
}

// LA INTERFAZ TIENE QUE PODER DECIR DE DÓNDE SALE LA FECHA. Colocar una descarga
// de 2019 entre las fotos de hoy sin avisar es peor que no ordenarla.
func TestAJSON_diceSiLaFechaEsDeCaptura(t *testing.T) {
	captura := time.Date(2019, 5, 4, 10, 0, 0, 0, time.UTC)
	subida := time.Date(2026, 9, 8, 8, 0, 0, 0, time.UTC)

	con := aJSON(domain.Activo{TomadaEn: &captura, SubidaEn: subida})
	if !con.DeCaptura || !con.Cuando.Equal(captura) {
		t.Errorf("con EXIF: cuando=%v deCaptura=%v", con.Cuando, con.DeCaptura)
	}
	sin := aJSON(domain.Activo{SubidaEn: subida})
	if sin.DeCaptura || !sin.Cuando.Equal(subida) {
		t.Errorf("sin EXIF: cuando=%v deCaptura=%v", sin.Cuando, sin.DeCaptura)
	}
}
