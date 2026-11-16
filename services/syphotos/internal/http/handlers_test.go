// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"strings"
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

// LA PAPELERA DICE CUÁNDO SE VA CADA FOTO. La fecha la pone el servicio, que es
// quien purga: si la sumara cada cliente, la web y el escritorio podrían prometer
// días distintos para la misma foto.
func TestPapeleraJSON_diceCuandoSeBorraCadaFoto(t *testing.T) {
	borrada := time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)
	fotos := []domain.Activo{{ID: uuid.New(), Ancho: 10, Alto: 10, BorradaEn: &borrada}}

	r := papeleraJSON(fotos, 30*24*time.Hour)
	if r.RetentionDays != 30 {
		t.Errorf("retentionDays = %d, se esperaba 30", r.RetentionDays)
	}
	if len(r.Items) != 1 {
		t.Fatalf("salieron %d fotos", len(r.Items))
	}
	f := r.Items[0]
	if f.BorradaEn == nil || !f.BorradaEn.Equal(borrada) {
		t.Errorf("deletedAt = %v, se esperaba %v", f.BorradaEn, borrada)
	}
	if want := borrada.Add(30 * 24 * time.Hour); f.SeBorraEn == nil || !f.SeBorraEn.Equal(want) {
		t.Errorf("purgeAt = %v, se esperaba %v", f.SeBorraEn, want)
	}

	// Y en el JSON, junto a los campos de siempre (la tesela los sigue necesitando).
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, clave := range []string{`"deletedAt":"2026-11-01T09:30:00Z"`, `"purgeAt":"2026-12-01T09:30:00Z"`, `"retentionDays":30`, `"width":10`} {
		if !strings.Contains(string(b), clave) {
			t.Errorf("falta %s en %s", clave, b)
		}
	}
}

// SIN RETENCIÓN NO SE PROMETE FECHA: con `TRASH_RETENTION=0` nada caduca, y decir
// «se borrará en 0 días» sería mentir. Una retención que no es de días enteros se
// redondea hacia arriba: 36 h no son «1 día».
func TestPapeleraJSON_sinRetencionNoPrometeFecha(t *testing.T) {
	borrada := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	r := papeleraJSON([]domain.Activo{{BorradaEn: &borrada}}, 0)
	if r.RetentionDays != 0 || r.Items[0].SeBorraEn != nil || r.Items[0].BorradaEn == nil {
		t.Errorf("sin retención: %+v", r)
	}
	if r := papeleraJSON(nil, 36*time.Hour); r.RetentionDays != 2 || r.Items == nil {
		t.Errorf("36 h dieron %d días (o items nulo: %v)", r.RetentionDays, r.Items == nil)
	}
}
