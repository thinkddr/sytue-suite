// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/services/cast/internal/cuota"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
)

const idDeDrive = "7d1c6a3e-0b8e-4f39-9d3a-4a3e8a1f2b10"

// driveFalso es un Sytue Drive de mentira: un fichero, su contenido y la cuota.
type driveFalso struct {
	nombre     string
	tipo       string
	tamano     int64 // el que dicen los metadatos
	contenido  []byte
	codigo     int   // de los metadatos y del contenido; 0 = 200
	libres     int64 // available_bytes de la cuota
	cortarEn   int   // >0: el contenido se corta tras estos bytes
	contenidos int   // cuántas veces se pidió el contenido
	mu         sync.Mutex
}

func (d *driveFalso) servir(t *testing.T) *cuota.Drive {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/quota":
			_, _ = fmt.Fprintf(w, `{"limit_bytes":%d,"used_bytes":0,"available_bytes":%d}`, d.libres, d.libres)
		case "/api/v1/files/" + idDeDrive:
			if d.codigo != 0 {
				w.WriteHeader(d.codigo)
				return
			}
			_, _ = fmt.Fprintf(w, `{"id":%q,"name":%q,"size_bytes":%d,"content_type":%q}`, idDeDrive, d.nombre, d.tamano, d.tipo)
		case "/api/v1/files/" + idDeDrive + "/content":
			d.mu.Lock()
			d.contenidos++
			d.mu.Unlock()
			if d.codigo != 0 {
				w.WriteHeader(d.codigo)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(d.contenido)))
			if d.cortarEn > 0 {
				_, _ = w.Write(d.contenido[:d.cortarEn])
				return // el servidor corta la conexión: faltan bytes
			}
			_, _ = w.Write(d.contenido)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return cuota.New(srv.URL, "")
}

// objetosEnMemoria es el almacén de Cast en un mapa. Put lee en streaming, como S3.
type objetosEnMemoria struct {
	mu    sync.Mutex
	datos map[string][]byte
}

func (o *objetosEnMemoria) Put(_ context.Context, tenant, key string, r io.Reader, size int64, _ string) error {
	var b bytes.Buffer
	n, err := io.Copy(&b, r)
	if err != nil {
		return err
	}
	if n != size {
		return fmt.Errorf("put: %d bytes de %d", n, size)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.datos == nil {
		o.datos = map[string][]byte{}
	}
	o.datos[tenant+"/"+key] = b.Bytes()
	return nil
}

func (o *objetosEnMemoria) Get(context.Context, string, string, string) (Fichero, error) {
	return Fichero{}, domain.ErrNoExiste
}

func (o *objetosEnMemoria) DeletePrefix(_ context.Context, tenant, prefix string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	for k := range o.datos {
		if strings.HasPrefix(k, tenant+"/"+prefix) {
			delete(o.datos, k)
		}
	}
	return nil
}

func (o *objetosEnMemoria) cuantos() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.datos)
}

var ana = Quien{Sub: "ana", Tenant: "acme", Nombre: "Ana", Token: "tok"}

// servicioSinBase monta el servicio SIN store: si un caso llegara a crear el
// vídeo, reventaría. Es la prueba de que los rechazos no crean nada.
func servicioSinBase(t *testing.T, d *driveFalso) (*Servicio, *objetosEnMemoria) {
	t.Helper()
	cliente := d.servir(t)
	obj := &objetosEnMemoria{}
	return New(Deps{Objetos: obj, Cuota: cliente, Drive: cliente, Firmante: media.New("clave")}), obj
}

func TestSubirDeDriveRechazaSinCrearNada(t *testing.T) {
	video := func() *driveFalso {
		return &driveFalso{nombre: "Clase.mp4", tipo: "video/mp4", tamano: 11, contenido: []byte("hola, vídeo"), libres: 1 << 30}
	}
	casos := []struct {
		nombre string
		ajuste func(*driveFalso)
		quiero error
	}{
		{"el fichero no existe", func(d *driveFalso) { d.codigo = http.StatusNotFound }, domain.ErrNoExiste},
		{"sin permiso sobre el fichero", func(d *driveFalso) { d.codigo = http.StatusForbidden }, domain.ErrProhibido},
		{"no es un vídeo", func(d *driveFalso) { d.tipo = "application/pdf"; d.nombre = "a.pdf" }, ErrNoEsVideo},
		{"pasa de 8 GiB", func(d *driveFalso) { d.tamano = domain.MaxOriginalBytes + 1 }, domain.ErrDemasiadoGrande},
		{"vacío", func(d *driveFalso) { d.tamano = 0 }, domain.ErrEntradaInvalida},
		{"no cabe en la cuota", func(d *driveFalso) { d.libres = 10 }, domain.ErrSinCuota},
		{"Drive no contesta", func(d *driveFalso) { d.codigo = http.StatusBadGateway }, ErrDriveNoContesta},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			d := video()
			c.ajuste(d)
			svc, obj := servicioSinBase(t, d)

			_, err := svc.PrevisualizarDeDrive(context.Background(), ana, idDeDrive)
			if !errors.Is(err, c.quiero) {
				t.Fatalf("previsualizar: err = %v; quiero %v", err, c.quiero)
			}
			_, err = svc.SubirDeDrive(context.Background(), ana, DeDrive{FileID: idDeDrive})
			if !errors.Is(err, c.quiero) {
				t.Fatalf("subir: err = %v; quiero %v", err, c.quiero)
			}
			svc.EsperarCopias()
			if obj.cuantos() != 0 {
				t.Fatalf("se guardó algo en el almacén")
			}
		})
	}
}

func TestSubirDeDriveSinCuotaDiceCuantoQueda(t *testing.T) {
	svc, _ := servicioSinBase(t, &driveFalso{nombre: "a.mp4", tipo: "video/mp4", tamano: 100, libres: 42})
	_, err := svc.SubirDeDrive(context.Background(), ana, DeDrive{FileID: idDeDrive})
	var sin *domain.SinCuota
	if !errors.As(err, &sin) || sin.Disponibles != 42 {
		t.Fatalf("err = %v; quiero SinCuota con 42 libres", err)
	}
}

func TestSubirDeDriveRechazaLoQueNoHaceFaltaPreguntar(t *testing.T) {
	d := &driveFalso{nombre: "a.mp4", tipo: "video/mp4", tamano: 1, contenido: []byte("x"), libres: 1 << 30}
	svc, _ := servicioSinBase(t, d)
	casos := []struct {
		nombre string
		quien  Quien
		a      DeDrive
		quiero error
	}{
		{"sin sesión", Nadie, DeDrive{FileID: idDeDrive}, domain.ErrProhibido},
		{"id que no es un id", ana, DeDrive{FileID: "../quota"}, domain.ErrEntradaInvalida},
		{"visibilidad inventada", ana, DeDrive{FileID: idDeDrive, Visibility: "todos"}, domain.ErrEntradaInvalida},
		{"título demasiado largo", ana, DeDrive{FileID: idDeDrive, Title: strings.Repeat("a", domain.MaxTitulo+1)}, domain.ErrEntradaInvalida},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, err := svc.SubirDeDrive(context.Background(), c.quien, c.a); !errors.Is(err, c.quiero) {
				t.Fatalf("err = %v; quiero %v", err, c.quiero)
			}
		})
	}
	if d.contenidos != 0 {
		t.Fatalf("se pidió el contenido %d veces", d.contenidos)
	}
}

func TestSubirDeDriveSinDriveConfigurado(t *testing.T) {
	svc := New(Deps{Objetos: &objetosEnMemoria{}, Firmante: media.New("clave")})
	if _, err := svc.SubirDeDrive(context.Background(), ana, DeDrive{FileID: idDeDrive}); !errors.Is(err, ErrSinDrive) {
		t.Fatalf("err = %v; quiero ErrSinDrive", err)
	}
}

func TestPrevisualizarDeDrive(t *testing.T) {
	svc, _ := servicioSinBase(t, &driveFalso{nombre: "Clase 3 · repaso.mkv", tipo: "video/x-matroska", tamano: 2048, libres: 1 << 30})
	f, err := svc.PrevisualizarDeDrive(context.Background(), ana, idDeDrive)
	if err != nil {
		t.Fatal(err)
	}
	if f.ID != idDeDrive || f.Nombre != "Clase 3 · repaso.mkv" || f.Bytes != 2048 || f.Titulo != "Clase 3 · repaso" {
		t.Fatalf("previsualización = %+v", f)
	}
}

func TestTituloDeFichero(t *testing.T) {
	casos := map[string]string{
		"Clase 3.mp4":          "Clase 3",
		"  dos   espacios.mov": "dos espacios",
		"sin extension":        "sin extension",
		".mp4":                 ".mp4",
		"a.b.c.webm":           "a.b.c",
	}
	for in, quiero := range casos {
		if got := TituloDeFichero(in); got != quiero {
			t.Errorf("TituloDeFichero(%q) = %q; quiero %q", in, got, quiero)
		}
	}
	largo := TituloDeFichero(strings.Repeat("ñ", domain.MaxTitulo+50) + ".mp4")
	if _, err := domain.Titulo(largo); err != nil {
		t.Fatalf("el título por defecto no es válido: %v", err)
	}
}

func TestExactoNotaElCorte(t *testing.T) {
	e := &exacto{r: strings.NewReader("abc"), faltan: 5}
	if _, err := io.ReadAll(e); !errors.Is(err, io.ErrUnexpectedEOF) || e.err == nil {
		t.Fatalf("err = %v, e.err = %v; quiero io.ErrUnexpectedEOF", err, e.err)
	}
	// Con más bytes de los dichos, lee los dichos y para.
	e = &exacto{r: strings.NewReader("abcdef"), faltan: 3}
	b, err := io.ReadAll(e)
	if err != nil || string(b) != "abc" {
		t.Fatalf("leído %q, err %v", b, err)
	}
}
