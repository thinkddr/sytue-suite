// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
)

// TestLaCopiaConLaAppCerrada: la tarea registrada no hace nada con la copia apagada; con
// ella encendida, sube con la sesión y los servicios que le da el sistema, con el MISMO
// registro (y la misma cuenta) que la app abierta, así que lo que una subió la otra no lo
// repite.
func TestLaCopiaConLaAppCerrada(t *testing.T) {
	g, d := nuevaGaleria(t), &driveFalso{}
	g.poner("1", "IMG_0001.JPG", "uno", tomaCopia)
	d.resto, d.subidos, d.t = &servicioFalso{}, map[string]string{}, t
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close)
	ruta := filepath.Join(t.TempDir(), "copia.json")
	antes, antesRuta := galeriaFondo, rutaFondo
	galeriaFondo, rutaFondo = g, func() (string, error) { return ruta, nil }
	t.Cleanup(func() { galeriaFondo, rutaFondo = antes, antesRuta })
	f := sesion.EnFondo{URL: func(string) string { return srv.URL }, Cuenta: "ana"}

	if r := copiaEnFondo(context.Background(), f); r != sesion.SinCambios || len(d.subidos) != 0 {
		t.Fatalf("apagada: %v, subidas %v", r, d.subidos)
	}
	if err := CopiaDe(g, ruta).CambiarAjustes(func(a *AjustesCopia) { a.Activa = true }); err != nil {
		t.Fatal(err)
	}
	if r := copiaEnFondo(context.Background(), f); r != sesion.Nuevos || len(d.subidos) != 1 || d.indexados != 1 {
		t.Fatalf("encendida: %v, subidas %v, indexadas %d", r, d.subidos, d.indexados)
	}
	if r := copiaEnFondo(context.Background(), f); r != sesion.SinCambios || len(d.subidos) != 1 {
		t.Fatalf("la segunda vez ha vuelto a subir: %v, %v", r, d.subidos)
	}
	if CopiaDe(g, ruta).Ultima("ana").IsZero() {
		t.Error("la app abierta no ve lo que hizo la copia con la app cerrada")
	}
}

// TestResultadoDeCopia: lo que se arregla esperando se aplaza; lo que no, no se reintenta.
func TestResultadoDeCopia(t *testing.T) {
	for _, c := range []struct {
		p    ProgresoCopia
		err  error
		want sesion.Resultado
	}{
		{ProgresoCopia{Subidas: 2}, nil, sesion.Nuevos},
		{ProgresoCopia{}, nil, sesion.SinCambios},
		{ProgresoCopia{}, ErrCopiaEnMarcha, sesion.SinCambios},
		{ProgresoCopia{}, carrete.ErrSinPermiso, sesion.SinCambios},
		{ProgresoCopia{}, ErrCopiaEsperaWifi, sesion.Aplazado},
		{ProgresoCopia{Subidas: 1}, context.DeadlineExceeded, sesion.Aplazado},
		{ProgresoCopia{}, errors.Join(ErrCopiaSinRed, errors.New("x")), sesion.Fallo},
	} {
		if got := resultadoDeCopia(c.p, c.err); got != c.want {
			t.Errorf("resultadoDeCopia(%+v, %v) = %v, quiero %v", c.p, c.err, got, c.want)
		}
	}
}
