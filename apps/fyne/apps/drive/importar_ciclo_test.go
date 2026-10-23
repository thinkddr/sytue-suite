// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestImportadorDriveNoSeReabreTrasSalir(t *testing.T) {
	entrada, cancelada := make(chan struct{}), make(chan struct{})
	inesperada := make(chan struct{}, 1)
	var pedidas atomic.Int32
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// El inventario de nubes también usa /instances; sólo migrations/self
		// identifica las consultas de progreso del importador que vigila este cambio.
		if r.URL.Path != "/api/v1/migrations/self" {
			_, _ = io.WriteString(w, `{"instances":[]}`)
			return
		}
		if pedidas.Add(1) != 1 {
			select {
			case inesperada <- struct{}{}:
			default:
			}
			_, _ = io.WriteString(w, `{}`)
			return
		}
		close(entrada)
		<-r.Context().Done()
		close(cancelada)
	}))
	defer gw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{}`) }))
	defer srv.Close()
	p := pantallaDePrueba(t, srv)
	p.ConIntegraciones(api.NewIntegraciones(gw.URL, nil, nil))
	p.IniciarImportador(context.Background())
	<-entrada
	p.PararImportador()
	select {
	case <-cancelada:
	case <-time.After(2 * time.Second):
		t.Fatal("la sesión no canceló el importador")
	}
	p.cargarPie() // El pie puede reconstruirse después de otra operación de Drive.
	select {
	case <-inesperada:
		t.Fatal("otra consulta del importador tras salir")
	case <-time.After(150 * time.Millisecond):
	}
	if p.lateral.Importar.Objeto(ui.AnchoUtilMenu, tema.Claro) != nil {
		t.Fatal("contenido del importador tras salir")
	}
}
