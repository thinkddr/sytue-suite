// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
	"time"
)

// La ORIENTACIÓN es el fallo clásico de toda galería: la cámara no rota los
// píxeles, escribe una etiqueta, y quien la aplica en la miniatura pero no en el
// visor —o al revés— enseña la foto tumbada.
//
// ESTE TEST NO BASTA, y conviene decirlo aquí: comprueba que la tabla dice lo
// que dice, y la 5 y la 7 estuvieron cambiadas con este test en verde, porque
// repetía la misma suposición. Lo que las distingue de verdad es DÓNDE ACABA
// cada píxel, y eso se prueba en
// `services/syphotos/internal/service/orientar_test.go`, sobre una imagen real.
func TestOrientacion_tablaDelEstandar(t *testing.T) {
	casos := []struct {
		o           Orientacion
		grados      int
		espejo      bool
		intercambia bool
	}{
		{1, 0, false, false},
		{2, 0, true, false},
		{3, 180, false, false},
		{4, 180, true, false},
		{5, 270, true, true},
		{6, 90, false, true},
		{7, 90, true, true},
		{8, 270, false, true},
	}
	for _, c := range casos {
		got := c.o.Transformacion()
		if got.Grados != c.grados || got.EspejoH != c.espejo {
			t.Errorf("orientación %d: %+v, se esperaba {Grados:%d EspejoH:%v}",
				c.o, got, c.grados, c.espejo)
		}
		if got.IntercambiaLados() != c.intercambia {
			t.Errorf("orientación %d: IntercambiaLados=%v, se esperaba %v",
				c.o, got.IntercambiaLados(), c.intercambia)
		}
	}
}

// ANTE LA DUDA, NO TOCAR: hay cámaras y editores que escriben basura en esa
// etiqueta, y perder la foto por eso es peor que enseñarla como está guardada.
func TestOrientacion_valorRaroNoRotaYSeSabe(t *testing.T) {
	for _, o := range []Orientacion{0, 9, 42, 255} {
		if got := o.Transformacion(); got.Grados != 0 || got.EspejoH {
			t.Errorf("orientación %d devolvió %+v; se esperaba sin transformar", o, got)
		}
		if o.Valida() {
			t.Errorf("orientación %d se dio por válida", o)
		}
	}
	if !Orientacion(6).Valida() {
		t.Error("la orientación 6 tiene que ser válida")
	}
}

// Reservar el lienzo con el ancho equivocado deja la línea de tiempo llena de
// huecos que saltan al cargar cada foto.
func TestActivo_dimensionesVistas(t *testing.T) {
	vertical := Activo{Ancho: 4000, Alto: 3000, Orientacion: 6}
	if a, h := vertical.Vistas(); a != 3000 || h != 4000 {
		t.Errorf("con orientación 6 se esperaba 3000×4000, salió %d×%d", a, h)
	}
	tal := Activo{Ancho: 4000, Alto: 3000, Orientacion: 1}
	if a, h := tal.Vistas(); a != 4000 || h != 3000 {
		t.Errorf("con orientación 1 se esperaba 4000×3000, salió %d×%d", a, h)
	}
}

// Sin fecha de captura la línea de tiempo cae a la de subida, y la interfaz
// tiene que poder DECIRLO: colocar una descarga de 2019 entre las fotos de hoy
// sin avisar es peor que no ordenarla.
func TestActivo_fechaDeGaleria(t *testing.T) {
	captura := time.Date(2019, 7, 1, 10, 0, 0, 0, time.UTC)
	subida := time.Date(2026, 9, 8, 8, 0, 0, 0, time.UTC)

	con := Activo{TomadaEn: &captura, SubidaEn: subida}
	if cuando, deCaptura := con.FechaDeGaleria(); !cuando.Equal(captura) || !deCaptura {
		t.Errorf("con EXIF: %v/%v, se esperaba la de captura", cuando, deCaptura)
	}
	sin := Activo{SubidaEn: subida}
	if cuando, deCaptura := sin.FechaDeGaleria(); !cuando.Equal(subida) || deCaptura {
		t.Errorf("sin EXIF: %v/%v, se esperaba la de subida y deCaptura=false", cuando, deCaptura)
	}
	// Una fecha cero es «no hay», no «el año 1»: sin esto la foto se iría al
	// principio de la línea de tiempo, delante de todo.
	cero := time.Time{}
	nula := Activo{TomadaEn: &cero, SubidaEn: subida}
	if cuando, deCaptura := nula.FechaDeGaleria(); !cuando.Equal(subida) || deCaptura {
		t.Errorf("con fecha cero: %v/%v, se esperaba caer a la de subida", cuando, deCaptura)
	}
}

// El truncado es un fallo silencioso: un 260 leído del EXIF cabía en un uint8
// como 4, que ES válida, y habría rotado la foto sin que nadie lo pidiera.
func TestOrientacionDe_fueraDeRangoNoSeVuelveValida(t *testing.T) {
	for _, v := range []int{-1, 0, 9, 260, 65535} {
		if o := OrientacionDe(v); o.Valida() {
			t.Fatalf("OrientacionDe(%d) = %d, y se da por válida", v, o)
		}
	}
	for v := 1; v <= 8; v++ {
		if o := OrientacionDe(v); !o.Valida() || int(o) != v {
			t.Fatalf("OrientacionDe(%d) = %d", v, o)
		}
	}
}
