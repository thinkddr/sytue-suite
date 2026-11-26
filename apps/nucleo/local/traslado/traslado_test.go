// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package traslado

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// datosDeEjemplo deja una carpeta de datos con un poco de todo: documentos, un almacén
// (abierto y cerrado), Drive con su índice de rutas absolutas, un vídeo copiado de Cast,
// un enlace, un fichero vacío y uno grande.
func datosDeEjemplo(t *testing.T) string {
	t.Helper()
	datos := filepath.Join(t.TempDir(), "Sytue Office")
	escribir := func(rel, contenido string) {
		t.Helper()
		r := filepath.Join(datos, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(r), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(r, []byte(contenido), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rec, err := almacen.Abrir(t.Context(), filepath.Join(datos, "Datos", "notes"), "notes", "persona")
	if err != nil {
		t.Fatal(err)
	}
	rec.Cerrar()
	escribir("Documentos/docs/d1.json", `{"formato":1,"documento":{"id":"d1","title":"Acta"}}`)
	escribir("Drive/informe.docx", "PK\x03\x04 un docx")
	escribir("Drive/vacío.txt", "")
	escribir("Drive/Carpeta con espacios/foto.jpg", strings.Repeat("\xff\xd8 foto ", 400_000))
	escribir("Vídeos/viaje.webm", "webm")
	escribir("Datos/cast/videoteca.json", `[{"id":"v1","ruta":`+cita(filepath.Join(datos, "Vídeos", "viaje.webm"))+`,"titulo":"Viaje","creado":"2026-01-01T00:00:00Z","actualizado":"2026-01-01T00:00:00Z","tamano":4}]`)
	escribir("Drive/.sytue/indice.json", `{"recientes":[{"ruta":`+cita(filepath.Join(datos, "Drive", "informe.docx"))+`,"abierto":"2026-01-01T00:00:00Z"}]}`)
	if runtime.GOOS != "windows" {
		if err := os.Symlink("informe.docx", filepath.Join(datos, "Drive", "atajo.docx")); err != nil {
			t.Fatal(err)
		}
	}
	// Una fecha vieja, que tiene que llegar igual.
	vieja := time.Date(2020, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(datos, "Drive", "informe.docx"), vieja, vieja); err != nil {
		t.Fatal(err)
	}
	return datos
}

func cita(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

// arbol es cada fichero (relativo) con su contenido, y cada enlace con su destino.
func arbol(t *testing.T, raiz string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(raiz, func(r string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(raiz, r)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			e, _ := os.Readlink(r)
			out[rel] = "-> " + e
		case d.IsDir():
			out[rel+"/"] = ""
		default:
			raw, err := os.ReadFile(r)
			if err != nil {
				return err
			}
			out[rel] = string(raw)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Se lleva todo, igual; lo apunta solo al final; la vieja no se toca hasta que se pide.
func TestMoverSeLlevaTodoYLoComprueba(t *testing.T) {
	origen := datosDeEjemplo(t)
	antes := arbol(t, origen)
	destino := filepath.Join(t.TempDir(), "nueva", "Sytue Office")
	var apuntada string
	var fases []Fase
	inf, err := Mover(t.Context(), Opciones{
		Origen: origen, Destino: destino,
		Apuntar: func(d string) error {
			apuntada = d
			// Al apuntar ya no queda la marca: la copia está entera.
			if _, err := os.Stat(filepath.Join(d, Marca)); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("la marca sigue al apuntar: %v", err)
			}
			return nil
		},
		Progreso: func(p Progreso) {
			if len(fases) == 0 || fases[len(fases)-1] != p.Fase {
				fases = append(fases, p.Fase)
			}
			if p.Hecho > p.Total {
				t.Errorf("progreso %d de %d", p.Hecho, p.Total)
			}
		},
		Libre: func(string) (int64, error) { return 1 << 40, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if apuntada != destino {
		t.Errorf("apuntada %q; quiero %q", apuntada, destino)
	}
	if want := []Fase{Midiendo, Copiando, Comprobando, Ajustando}; len(fases) != len(want) {
		t.Errorf("fases %v", fases)
	}
	despues := arbol(t, destino)
	for rel, c := range antes {
		got, ok := despues[rel]
		switch {
		case !ok:
			t.Errorf("falta %s", rel)
		case rel == filepath.Join("Datos", "cast", "videoteca.json") || rel == filepath.Join("Drive", ".sytue", "indice.json"):
			// Las rutas absolutas pasan a la carpeta nueva.
			if strings.Contains(got, cita(origen)[1:len(cita(origen))-1]) || !strings.Contains(got, strings.Trim(cita(destino), `"`)) {
				t.Errorf("%s sigue con la ruta vieja:\n%s", rel, got)
			}
		case got != c:
			t.Errorf("%s distinto", rel)
		}
	}
	if len(despues) != len(antes) {
		t.Errorf("hay %d cosas y había %d", len(despues), len(antes))
	}
	fi, err := os.Stat(filepath.Join(destino, "Drive", "informe.docx"))
	if err != nil || !fi.ModTime().Equal(time.Date(2020, 5, 6, 7, 8, 9, 0, time.UTC)) {
		t.Errorf("la fecha no llega: %v %v", fi, err)
	}
	// La vieja, intacta.
	if got := arbol(t, origen); len(got) != len(antes) {
		t.Errorf("la vieja ha cambiado: %d cosas", len(got))
	}
	if inf.Ficheros == 0 || inf.Bytes < 3_000_000 {
		t.Errorf("informe: %+v", inf)
	}
	// El almacén se abre en su sitio nuevo.
	rec, err := almacen.Abrir(t.Context(), filepath.Join(destino, "Datos", "notes"), "notes", "persona")
	if err != nil {
		t.Fatalf("el almacén no se abre en la carpeta nueva: %v", err)
	}
	rec.Cerrar()

	// Y, si se pide, se borra exactamente lo copiado.
	quedan, err := BorrarOrigen(inf)
	if err != nil || quedan != 0 {
		t.Fatalf("borrar: %d, %v", quedan, err)
	}
	if _, err := os.Stat(origen); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("la vieja sigue: %v", err)
	}
}

// BorrarOrigen no quita lo que no se copió.
func TestBorrarOrigenDejaLoQueNoSeCopio(t *testing.T) {
	origen := datosDeEjemplo(t)
	destino := filepath.Join(t.TempDir(), "nueva")
	inf, err := Mover(t.Context(), Opciones{Origen: origen, Destino: destino})
	if err != nil {
		t.Fatal(err)
	}
	nuevo := filepath.Join(origen, "Drive", "puesto después.txt")
	if err := os.WriteFile(nuevo, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	quedan, err := BorrarOrigen(inf)
	if err != nil || quedan == 0 {
		t.Errorf("quedan %d, %v", quedan, err)
	}
	if _, err := os.Stat(nuevo); err != nil {
		t.Errorf("se ha borrado lo que no se copió: %v", err)
	}
	if got := arbol(t, origen); len(got) != 3 { // ./, Drive/ y el fichero
		t.Errorf("queda %v", got)
	}
	if _, err := BorrarOrigen(Informe{}); err == nil {
		t.Error("BorrarOrigen sin traslado")
	}
}

// No se mezcla con lo que haya, ni se copia una carpeta en sí misma.
func TestComprobar(t *testing.T) {
	origen := datosDeEjemplo(t)
	llena := t.TempDir()
	if err := os.WriteFile(filepath.Join(llena, "mío.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for destino, quiero := range map[string]error{
		origen:                               ErrMisma,
		origen + string(filepath.Separator):  ErrMisma,
		filepath.Join(origen, "Drive", "x"):  ErrDentro,
		llena:                                ErrNoVacia,
		filepath.Dir(origen):                 ErrNoVacia,
		t.TempDir():                          nil,
		filepath.Join(t.TempDir(), "a", "b"): nil,
	} {
		if err := Comprobar(origen, destino); !errors.Is(err, quiero) {
			t.Errorf("%s: %v; quiero %v", destino, err, quiero)
		}
	}
	// Mover dice lo mismo sin tocar nada.
	if _, err := Mover(t.Context(), Opciones{Origen: origen, Destino: llena}); !errors.Is(err, ErrNoVacia) {
		t.Errorf("mover a una llena: %v", err)
	}
	if got := arbol(t, llena); len(got) != 2 {
		t.Errorf("la llena ha cambiado: %v", got)
	}
}

// Elegir una carpeta con cosas pone los datos en una «Sytue Office» dentro.
func TestDestino(t *testing.T) {
	vacia := t.TempDir()
	if got := Destino(vacia, "Sytue Office"); got != vacia {
		t.Errorf("vacía: %s", got)
	}
	llena := t.TempDir()
	if err := os.WriteFile(filepath.Join(llena, "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Destino(llena, "Sytue Office"); got != filepath.Join(llena, "Sytue Office") {
		t.Errorf("llena: %s", got)
	}
	if got := Destino(filepath.Join(vacia, "no-existe"), "Sytue Office"); got != filepath.Join(vacia, "no-existe") {
		t.Errorf("no existe: %s", got)
	}
}

// Con una app abierta (su almacén con el cerrojo compartido), no se empieza.
func TestConUnaAppAbiertaNoSeEmpieza(t *testing.T) {
	origen := datosDeEjemplo(t)
	rec, err := almacen.Abrir(t.Context(), filepath.Join(origen, "Datos", "notes"), "notes", "persona")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Cerrar()
	destino := filepath.Join(t.TempDir(), "nueva")
	apuntado := false
	if _, err := Mover(t.Context(), Opciones{Origen: origen, Destino: destino, Apuntar: func(string) error { apuntado = true; return nil }}); !errors.Is(err, ErrEnUso) {
		t.Fatalf("con Notepads abierta: %v", err)
	}
	if _, err := os.Stat(destino); !errors.Is(err, fs.ErrNotExist) || apuntado {
		t.Errorf("ha tocado la nueva (%v) o ha apuntado (%v)", err, apuntado)
	}
}

// Si no cabe, no se empieza.
func TestSinSitio(t *testing.T) {
	origen := datosDeEjemplo(t)
	destino := t.TempDir()
	_, err := Mover(t.Context(), Opciones{Origen: origen, Destino: destino, Libre: func(string) (int64, error) { return 10, nil }})
	if !errors.Is(err, ErrSinSitio) {
		t.Fatalf("sin sitio: %v", err)
	}
	if got := arbol(t, destino); len(got) != 1 {
		t.Errorf("la nueva no está como estaba: %v", got)
	}
}

// Cancelar a medias, una copia mala, un original que cambia y no poder apuntar dejan la
// nueva como estaba (sin crear o vacía), sin apuntar nada y con la vieja entera.
func TestSiFallaAMediasNadaCambia(t *testing.T) {
	for nombre, caso := range map[string]struct {
		// enFase hace algo al entrar en esa fase (ctx: el del traslado).
		enFase  Fase
		hacer   func(t *testing.T, origen, destino string, cancelar func())
		apuntar error
		quiero  error
		existia bool
	}{
		"cancelar copiando":    {enFase: Copiando, hacer: func(_ *testing.T, _, _ string, cancelar func()) { cancelar() }, quiero: context.Canceled},
		"cancelar comprobando": {enFase: Comprobando, hacer: func(_ *testing.T, _, _ string, cancelar func()) { cancelar() }, quiero: context.Canceled, existia: true},
		"copia mala": {enFase: Comprobando, quiero: ErrCopiaMala, hacer: func(t *testing.T, _, destino string, _ func()) {
			// Un bit cambiado en la copia, con el mismo tamaño.
			r := filepath.Join(destino, "Drive", "informe.docx")
			raw, _ := os.ReadFile(r)
			raw[0] ^= 1
			if err := os.WriteFile(r, raw, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		"copia que falta": {enFase: Comprobando, quiero: fs.ErrNotExist, hacer: func(t *testing.T, _, destino string, _ func()) {
			if err := os.Remove(filepath.Join(destino, "Drive", "vacío.txt")); err != nil {
				t.Fatal(err)
			}
		}},
		"el original cambia": {enFase: Comprobando, quiero: ErrCopiaMala, hacer: func(t *testing.T, origen, _ string, _ func()) {
			if err := os.WriteFile(filepath.Join(origen, "Vídeos", "viaje.webm"), []byte("otro vídeo"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		"no se puede apuntar": {apuntar: errors.New("sin permiso"), quiero: errors.New("sin permiso"), existia: true},
	} {
		t.Run(nombre, func(t *testing.T) {
			origen := datosDeEjemplo(t)
			antes := arbol(t, origen)
			base := t.TempDir()
			destino := filepath.Join(base, "nueva")
			if caso.existia {
				if err := os.Mkdir(destino, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancelar := context.WithCancel(t.Context())
			defer cancelar()
			hecho := false
			apuntado := false
			_, err := Mover(ctx, Opciones{
				Origen: origen, Destino: destino,
				Progreso: func(p Progreso) {
					if caso.hacer != nil && p.Fase == caso.enFase && !hecho {
						hecho = true
						if p.Fase == Copiando {
							// Que haya algo copiado ya: se cancela al leer el primero.
							caso.hacer(t, origen, destino, cancelar)
							return
						}
						caso.hacer(t, origen, destino, cancelar)
					}
				},
				Apuntar: func(string) error { apuntado = true; return caso.apuntar },
			})
			if err == nil || (!errors.Is(err, caso.quiero) && err.Error() != caso.quiero.Error()) {
				t.Fatalf("error %v; quiero %v", err, caso.quiero)
			}
			if apuntado && caso.apuntar == nil {
				t.Error("ha apuntado la carpeta nueva")
			}
			if caso.existia {
				if got := arbol(t, destino); len(got) != 1 {
					t.Errorf("la nueva no ha quedado vacía: %v", got)
				}
			} else if _, err := os.Stat(destino); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("la nueva sigue: %v", err)
			}
			if nombre != "el original cambia" {
				if got := arbol(t, origen); len(got) != len(antes) {
					t.Errorf("la vieja ha cambiado")
				}
			}
		})
	}
}

// Un traslado cortado (el equipo se apagó) deja la marca: la carpeta cuenta como vacía y
// se vacía antes de copiar.
func TestUnTrasladoCortadoSeRetoma(t *testing.T) {
	origen := datosDeEjemplo(t)
	destino := t.TempDir()
	for _, f := range []string{Marca, "Drive"} {
		if err := os.WriteFile(filepath.Join(destino, f), []byte("a medias"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Comprobar(origen, destino); err != nil {
		t.Fatalf("una cortada no cuenta como llena: %v", err)
	}
	if _, err := Mover(t.Context(), Opciones{Origen: origen, Destino: destino}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte(arbol(t, destino)[filepath.Join("Drive", "informe.docx")]), []byte("PK\x03\x04 un docx")) {
		t.Error("no ha copiado encima de lo cortado")
	}
	if _, err := os.Stat(filepath.Join(destino, Marca)); !errors.Is(err, fs.ErrNotExist) {
		t.Error("la marca sigue")
	}
}

func TestMedir(t *testing.T) {
	origen := datosDeEjemplo(t)
	n, b, err := Medir(t.Context(), origen)
	if err != nil || n < 8 || b < 3_000_000 {
		t.Errorf("%d ficheros, %d bytes, %v", n, b, err)
	}
}
