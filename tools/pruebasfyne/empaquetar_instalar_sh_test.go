// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Un xdg-mime de mentira con el mimeapps.list de verdad. Como el de verdad (xdg-utils
// 1.1.3, medido en el frontal), sin nada elegido contesta con cualquier .desktop
// instalado que declare el tipo.
const empaquetarXdgMime = `#!/bin/sh
f="$XDG_CONFIG_HOME/mimeapps.list"
case "$1" in
query)
  v=$(sed -n "s|^$3=||p" "$f" | tr -d ';')
  [ -n "$v" ] || v=$(cd "$PREFIJO/share/applications" 2>/dev/null && grep -l "^MimeType=.*$3;" -- *.desktop | head -1)
  printf '%s\n' "$v" ;;
default) grep -v "^$3=" "$f" >"$f.t"; echo "$3=$2" >>"$f.t"; mv "$f.t" "$f" ;;
esac
`

// --- 8. El instalar.sh del bundle de Linux, ejecutado de verdad.
// Contra un bundle de mentira con la forma que deja empaquetar.sh (los nombres son los
// del .desktop de `fyne package`, el ID de la app): pone el programa, reescribe Exec e
// Icon con rutas completas —`~/.local/bin` no está en el PATH de todos— y --quitar lo
// deja como estaba. Visto en rojo quitando el `sed` de Exec y el `rm` de --quitar.
func TestEmpaquetarInstalarShDeLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("instalar.sh solo se ejecuta en Linux, como en el original")
	}
	tmp := t.TempDir()
	b := empaquetarUnir(tmp, "sytue-prueba")
	empaquetarEjecutable(t, empaquetarUnir(b, "bin/sytue-uno"), "#!/bin/sh\necho hola\n")
	guiontest.Escribir(t, empaquetarUnir(b, "share/applications/eu.sytue.uno.desktop"),
		"[Desktop Entry]\nType=Application\nName=Uno\nExec=sytue-uno %U\nIcon=eu.sytue.uno\nMimeType=x-scheme-handler/sytue-uno;application/pdf;application/vnd.oasis.opendocument.text;application/vnd.apple.pages;\n")
	guiontest.Escribir(t, empaquetarUnir(b, "share/pixmaps/eu.sytue.uno.png"), "")
	r := empaquetar(t, "instalar-sh", "Sytue Prueba")
	if r.Codigo != 0 {
		t.Fatalf("empaquetar.sh instalar-sh falla: %s", r.Todo())
	}
	instalar := empaquetarUnir(b, "instalar.sh")
	empaquetarEjecutable(t, instalar, r.Salida)
	pre := empaquetarUnir(tmp, "prefijo")

	// El PDF lo tenía otra app, el ODT nadie. Sin XDG_CONFIG_HOME propio escribiría en el
	// del que corre la prueba. Lo de antes se apunta ANTES de instalar los nuestros.
	config := empaquetarUnir(tmp, "config")
	mimeapps := empaquetarUnir(config, "mimeapps.list")
	guiontest.Escribir(t, mimeapps, "[Default Applications]\napplication/pdf=otra.desktop\n")
	xdg := empaquetarUnir(tmp, "xdg")
	empaquetarEjecutable(t, empaquetarUnir(xdg, "xdg-mime"), empaquetarXdgMime)
	env := []string{"XDG_CONFIG_HOME=" + config, empaquetarConPATH(xdg), "PREFIJO=" + pre}
	correr := func(args ...string) guiontest.Resultado {
		t.Helper()
		return guiontest.Ejecutar(t, guiontest.Opciones{Env: env}, instalar, args...)
	}

	log := correr()
	if log.Codigo != 0 {
		t.Errorf("instalar.sh falla: %s", log.Todo())
	}
	// Visto en rojo quitando cada `xdg-mime default`, el `awk`, la comprobación de que ya
	// se había hecho y apuntando lo de antes después de instalar los .desktop.
	lista := empaquetarLeer(mimeapps)
	for _, m := range []string{"application/pdf", "application/vnd.oasis.opendocument.text"} {
		if !empaquetarHayLinea(lista, m+"=eu.sytue.uno.desktop") {
			t.Errorf("instalar.sh no deja la app por defecto para %s", m)
		}
	}
	if empaquetarHay(lista, `^x-scheme-handler`) {
		t.Error("instalar.sh pone por defecto el esquema, que no es un documento")
	}
	if empaquetarHay(lista, `^application/vnd.apple`) {
		t.Error("instalar.sh pone por defecto un fichero de Apple, que solo va en «Abrir con»")
	}
	// El usuario elige después otra app para el PDF: una puesta al día no se la quita, y al
	// desinstalar tampoco (más abajo).
	guiontest.Ejecutar(t, guiontest.Opciones{Env: env}, empaquetarUnir(xdg, "xdg-mime"), "default", "elegida.desktop", "application/pdf")
	if r := correr(); r.Codigo != 0 {
		t.Error("instalar.sh falla la segunda vez")
	}
	if !empaquetarHayLinea(empaquetarLeer(mimeapps), "application/pdf=elegida.desktop") {
		t.Error("una puesta al día vuelve a imponer las predeterminadas")
	}
	if _, err := exec.LookPath("wl-copy"); err != nil && !strings.Contains(log.Todo(), "instala wl-clipboard") {
		t.Error("instalar.sh no sugiere wl-clipboard, la dependencia opcional de Wayland")
	}
	if r := guiontest.Ejecutar(t, guiontest.Opciones{}, empaquetarUnir(pre, "bin/sytue-uno")); empaquetarJunto(r) != "hola" {
		t.Error("instalar.sh no deja el programa ejecutable en PREFIJO/bin")
	}
	desktop := empaquetarLeer(empaquetarUnir(pre, "share/applications/eu.sytue.uno.desktop"))
	if !empaquetarHayLinea(desktop, "Exec="+pre+"/bin/sytue-uno %U") {
		t.Error("el .desktop instalado no lleva la ruta completa del programa en Exec")
	}
	if !empaquetarHayLinea(desktop, "Icon="+pre+"/share/pixmaps/eu.sytue.uno.png") {
		t.Error("el .desktop instalado no lleva la ruta completa del icono")
	}

	// Al quitar, la que eligió el usuario se queda y lo que no tenía nadie deja de tenerlo.
	if r := correr("--quitar"); r.Codigo != 0 {
		t.Error("instalar.sh --quitar falla")
	}
	lista = empaquetarLeer(mimeapps)
	if !empaquetarHayLinea(lista, "application/pdf=elegida.desktop") {
		t.Error("instalar.sh --quitar pisa la app que el usuario eligió después")
	}
	if strings.Contains(lista, "eu.sytue.uno") {
		t.Error("instalar.sh --quitar deja la app por defecto de lo que no tenía nadie")
	}
	if quedan := empaquetarFicheros(pre); len(quedan) > 0 {
		t.Errorf("instalar.sh --quitar deja ficheros: %s", strings.Join(quedan, " "))
	}

	// Y si nadie lo tocó, el PDF vuelve a la app que lo tenía.
	guiontest.Escribir(t, mimeapps, "[Default Applications]\napplication/pdf=otra.desktop\n")
	correr()
	correr("--quitar")
	if !empaquetarHayLinea(empaquetarLeer(mimeapps), "application/pdf=otra.desktop") {
		t.Error("instalar.sh --quitar no devuelve el PDF a la app que lo tenía")
	}

	// El original hacía `unset XDG_CONFIG_HOME`; aquí uno vacío propio, para que si el
	// guardián fallara no escribiera en el del que corre la prueba.
	r = guiontest.Ejecutar(t, guiontest.Opciones{Env: []string{"PREFIJO=" + empaquetarUnir(tmp, "con espacio"), "XDG_CONFIG_HOME=" + t.TempDir()}}, instalar)
	if r.Codigo == 0 {
		t.Error("instalar.sh acepta un PREFIJO con espacios, que el menú no sabría abrir")
	}
}

// empaquetarFicheros es `find dir -type f`.
func empaquetarFicheros(dir string) []string {
	var r []string
	_ = filepath.WalkDir(dir, func(ruta string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			r = append(r, ruta)
		}
		return nil
	})
	return r
}
