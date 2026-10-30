// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import "testing"

// TestEnWindowsDriveNoDiceQueNoHaySinConexion: con el botón de conectar, en Windows Drive
// es la carpeta de archivos a petición y el texto tiene que decirlo; sin botón o en otro
// sistema, sigue el del WebDAV.
func TestEnWindowsDriveNoDiceQueNoHaySinConexion(t *testing.T) {
	for _, c := range []struct {
		goos  string
		boton bool
		clave string
	}{
		{"windows", true, "devices.drive.windowsExpectBody"},
		{"windows", false, "devices.drive.expectBody"},
		{"darwin", true, "devices.native.driveBody"},
		{"linux", true, "devices.native.driveBody"},
	} {
		if got := claveEsperarDrive(c.goos, c.boton); got != c.clave {
			t.Errorf("%s con botón %v: %q, quiero %q", c.goos, c.boton, got, c.clave)
		}
	}
}

// TestConElBotonLosPasosDeDriveVanPlegados: con «Conectar Drive en este equipo», mapear
// una unidad de red a mano es el plan B y va en «Valores manuales», plegado; a la vista,
// debajo del botón, parecía otra forma de hacer lo mismo (visto en Windows). Sin botón
// —la web, o sin dirección— los pasos siguen a la vista.
func TestConElBotonLosPasosDeDriveVanPlegados(t *testing.T) {
	for _, c := range []struct {
		boton bool
		filas int
	}{{true, 0}, {false, 2}} {
		d := NuevosClientesExternos(castellano(t))
		d.Estado, d.Coordenadas, d.Correo = Listo, coordenadasDePrueba, "ada@acme.eu"
		d.Plataforma, d.Paso = UnidadDrive, 1
		if c.boton {
			d.AlMontar = func(string) {}
		}
		r := pintaClientesExternos(t, d, 1280, "dispositivos-drive")
		filas := 0
		for _, o := range r.objetos {
			if _, ok := o.(*FilaCopia); ok {
				filas++
			}
		}
		if filas != c.filas {
			t.Errorf("con botón %v hay %d filas de copiar a la vista; quiero %d", c.boton, filas, c.filas)
		}
	}
}

// TestElPerfilDeAppleSeInstalaEnEsteMac: en el Mac, «Descargar el perfil de Sytue» lo
// instala aquí mismo (con su contraseña de aplicación dentro) en vez de abrir la
// descarga; en la pestaña del iPhone y sin el enganche pide un enlace autenticado
// de un uso. Se vio en ROJO quitando la comparación con `EsteApple`: el iPhone también lo
// tomaba.
func TestElPerfilDeAppleSeInstalaEnEsteMac(t *testing.T) {
	for _, c := range []struct {
		plataforma        Plataforma
		enganche, instala bool
	}{{Mac, true, true}, {Mac, false, false}, {IPhone, true, false}} {
		d := NuevosClientesExternos(castellano(t))
		d.Estado, d.Coordenadas, d.Correo = Listo, coordenadasDePrueba, "ada@acme.eu"
		d.Plataforma, d.Paso = c.plataforma, 1
		instalado, abierto := false, ""
		if c.enganche {
			d.AlConfigurarApple, d.EsteApple = func() { instalado = true }, Mac
		}
		d.AlEnlacePerfilApple = func(Plataforma) { abierto = "https://id.example/perfil/autenticado" }
		r := pintaClientesExternos(t, d, 1280, "dispositivos-apple")
		var boton *Accion
		for _, o := range r.objetos {
			if a, ok := o.(*Accion); ok && a.Texto == d.T.T("devices.apple.download", nil) {
				boton = a
			}
		}
		if boton == nil {
			t.Fatalf("%v: no está el botón del perfil", c.plataforma)
		}
		boton.AlPulsar()
		if instalado != c.instala || (abierto != "") == c.instala {
			t.Errorf("%v con enganche %v: instala %v y abre %q", c.plataforma, c.enganche, instalado, abierto)
		}
	}
}
