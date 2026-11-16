// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
)

// galeriaDeMentira es una galería en memoria: cada foto es su contenido.
type galeriaDeMentira struct {
	mu        sync.Mutex
	permiso   carrete.Permiso
	fotos     []carrete.Elemento
	contenido map[string]string
	dir       string
	guardadas []string
	// medida es la red de pago; sinConvertir, un sistema que no sabe pasar a JPEG.
	medida, sinConvertir bool
	convertidas          int
}

func nuevaGaleria(t *testing.T) *galeriaDeMentira {
	t.Helper()
	return &galeriaDeMentira{permiso: carrete.Concedido, contenido: map[string]string{}, dir: t.TempDir()}
}

func (g *galeriaDeMentira) poner(id, nombre, contenido string, cuando time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	tipo := carrete.TipoDe(nombre)
	g.fotos = append(g.fotos, carrete.Elemento{
		ID: id, Nombre: nombre, Tipo: tipo, Tomada: cuando, Tamano: int64(len(contenido)), Video: carrete.EsVideo(tipo),
	})
	g.contenido[id] = contenido
}

func (g *galeriaDeMentira) Disponible() bool { return true }

func (g *galeriaDeMentira) Permiso() carrete.Permiso { return g.permiso }

func (g *galeriaDeMentira) PedirPermiso(context.Context) (carrete.Permiso, error) {
	return g.permiso, nil
}

func (g *galeriaDeMentira) Listar(context.Context) ([]carrete.Elemento, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]carrete.Elemento(nil), g.fotos...), nil
}

func (g *galeriaDeMentira) Abrir(_ context.Context, e carrete.Elemento) (string, func(), error) {
	g.mu.Lock()
	c, ok := g.contenido[e.ID]
	g.mu.Unlock()
	if !ok {
		return "", func() {}, os.ErrNotExist
	}
	ruta := filepath.Join(g.dir, e.ID)
	if err := os.WriteFile(ruta, []byte(c), 0o600); err != nil {
		return "", func() {}, err
	}
	return ruta, func() { _ = os.Remove(ruta) }, nil
}

func (g *galeriaDeMentira) Guardar(_ context.Context, _, nombre string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.guardadas = append(g.guardadas, nombre)
	return "propia-" + nombre, nil
}

// Convertir deja «jpeg:<contenido>»: lo que sube de verdad se distingue del original.
func (g *galeriaDeMentira) Convertir(_ context.Context, ruta string) (string, func(), error) {
	g.mu.Lock()
	sin := g.sinConvertir
	g.convertidas++
	g.mu.Unlock()
	if sin {
		return "", func() {}, carrete.ErrNoDisponible
	}
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return "", func() {}, err
	}
	jpeg := ruta + ".jpg"
	if err := os.WriteFile(jpeg, append([]byte("jpeg:"), datos...), 0o600); err != nil {
		return "", func() {}, err
	}
	return jpeg, func() { _ = os.Remove(jpeg) }, nil
}

func (g *galeriaDeMentira) RedMedida() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.medida
}

// destinoDeMentira es «/Fotos» y el índice: lo subido, por nombre, con su contenido y su
// tipo, y lo indexado.
type destinoDeMentira struct {
	mu        sync.Mutex
	ficheros  map[string]string
	tipos     map[string]string
	indexados map[string]int
	leidas    int
	fallar    func(nombre string) error
	// indexar es lo que contesta el índice; nil, que sí a las imágenes que syphotos mide
	// y ErrNoIndexable a lo demás.
	indexar func(id string) error
}

func nuevoDestino() *destinoDeMentira {
	return &destinoDeMentira{ficheros: map[string]string{}, tipos: map[string]string{}, indexados: map[string]int{}}
}

func suma(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func (d *destinoDeMentira) Contenido(context.Context) (map[string]bool, map[string]bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.leidas++
	sumas, nombres := map[string]bool{}, map[string]bool{}
	for n, c := range d.ficheros {
		sumas[suma(c)], nombres[n] = true, true
	}
	return sumas, nombres, nil
}

func (d *destinoDeMentira) Subir(_ context.Context, ruta, nombre, tipo string) (Subido, error) {
	if d.fallar != nil {
		if err := d.fallar(nombre); err != nil {
			return Subido{}, err
		}
	}
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return Subido{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ya := d.ficheros[nombre]; ya {
		// Lo que haría un Drive sin `keep_both`: sustituir. La regla 2 dice que no pasa.
		return Subido{}, errors.New("se ha pisado " + nombre)
	}
	d.ficheros[nombre], d.tipos[nombre] = string(datos), tipo
	return Subido{ID: nombre, Nombre: nombre}, nil
}

func (d *destinoDeMentira) Indexar(_ context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.indexar != nil {
		if err := d.indexar(id); err != nil {
			return err
		}
	} else if !SyphotosMide(d.tipos[id]) {
		return ErrNoIndexable
	}
	d.indexados[id]++
	return nil
}

var tomaCopia = time.Date(2024, 5, 1, 12, 0, 0, 0, time.Local)

// TestCopiaSubeCadaFotoUnaVez: lo que ya está en «/Fotos» —aunque con otro nombre— no
// sube; lo demás sube con la fecha delante; una segunda ronda no sube nada, y una foto
// nueva sube sola.
func TestCopiaSubeCadaFotoUnaVez(t *testing.T) {
	g, d := nuevaGaleria(t), nuevoDestino()
	d.ficheros["playa.jpg"] = "foto de la playa" // subida a mano desde la web
	g.poner("1", "IMG_0001.JPG", "foto de la playa", tomaCopia)
	g.poner("2", "IMG_0002.JPG", "foto del monte", tomaCopia.Add(time.Hour))
	c := NuevaCopia(g, filepath.Join(t.TempDir(), "copia.json"))
	ctx := context.Background()

	var avisos []ProgresoCopia
	p, err := c.Ronda(ctx, "ana", d, func(p ProgresoCopia) { avisos = append(avisos, p) })
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 2 || p.Hechas != 2 || p.Subidas != 1 || p.Fallidas != 0 {
		t.Fatalf("progreso %+v", p)
	}
	if len(avisos) != 3 || avisos[0].Total != 2 || avisos[0].Hechas != 0 {
		t.Errorf("avisos %+v", avisos)
	}
	if d.ficheros["2024-05-01 13.00.00 IMG_0002.JPG"] != "foto del monte" || len(d.ficheros) != 2 {
		t.Fatalf("destino %v", d.ficheros)
	}
	if c.Ultima("ana").IsZero() {
		t.Error("una ronda sin fallos no ha apuntado la última copia")
	}

	if p, err := c.Ronda(ctx, "ana", d, nil); err != nil || p.Total != 0 || p.Subidas != 0 {
		t.Fatalf("segunda ronda: %+v %v", p, err)
	}
	g.poner("3", "IMG_0003.JPG", "foto del río", tomaCopia.Add(2*time.Hour))
	if p, err := c.Ronda(ctx, "ana", d, nil); err != nil || p.Total != 1 || p.Subidas != 1 {
		t.Fatalf("tercera ronda: %+v %v", p, err)
	}
	if d.leidas != 1 {
		t.Errorf("«/Fotos» se ha leído %d veces en el mismo día; quiero una", d.leidas)
	}
}

// TestCopiaNoPisaNombres: dos fotos distintas con el mismo nombre y la misma hora (o una
// que ya está en «/Fotos» con ese nombre y otro contenido) no se sustituyen: la segunda
// sube con « (2)».
func TestCopiaNoPisaNombres(t *testing.T) {
	g, d := nuevaGaleria(t), nuevoDestino()
	d.ficheros["2024-05-01 12.00.00 IMG_0001.JPG"] = "otra foto de otro teléfono"
	g.poner("a", "IMG_0001.JPG", "la mía", tomaCopia)
	g.poner("b", "IMG_0001.JPG", "otra mía", tomaCopia)
	c := NuevaCopia(g, "")
	if p, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || p.Subidas != 2 {
		t.Fatalf("%+v %v", p, err)
	}
	for _, n := range []string{"2024-05-01 12.00.00 IMG_0001 (2).JPG", "2024-05-01 12.00.00 IMG_0001 (3).JPG"} {
		if _, ok := d.ficheros[n]; !ok {
			t.Errorf("falta %q en %v", n, d.ficheros)
		}
	}
	if d.ficheros["2024-05-01 12.00.00 IMG_0001.JPG"] != "otra foto de otro teléfono" {
		t.Error("se ha pisado la foto que ya estaba")
	}
}

// TestCopiaSeParaSinRedYSigueDespues: tres fallos seguidos paran la ronda; la siguiente
// empieza por las que faltan y no repite las hechas.
func TestCopiaSeParaSinRedYSigueDespues(t *testing.T) {
	g, d := nuevaGaleria(t), nuevoDestino()
	for i, n := range []string{"1", "2", "3", "4", "5"} {
		g.poner(n, "IMG_"+n+".jpg", "contenido "+n, tomaCopia.Add(time.Duration(i)*time.Minute))
	}
	sinRed := errors.New("sin red")
	subidas := 0
	d.fallar = func(string) error {
		subidas++
		if subidas > 1 {
			return sinRed
		}
		return nil
	}
	c := NuevaCopia(g, "")
	p, err := c.Ronda(context.Background(), "ana", d, nil)
	if !errors.Is(err, ErrCopiaSinRed) || !errors.Is(err, sinRed) || p.Subidas != 1 || p.Fallidas != 3 {
		t.Fatalf("%+v %v", p, err)
	}
	if !c.Ultima("ana").IsZero() {
		t.Error("una ronda cortada ha apuntado la última copia")
	}
	d.fallar = nil
	p, err = c.Ronda(context.Background(), "ana", d, nil)
	if err != nil || p.Total != 4 || p.Subidas != 4 || len(d.ficheros) != 5 {
		t.Fatalf("segunda ronda %+v %v; destino %v", p, err, d.ficheros)
	}
}

// TestCopiaRecuerdaEntreArranques: el registro está en disco; otra Copia sobre el mismo
// fichero no sube nada de nuevo ni vuelve a leer «/Fotos» el mismo día, y lo de otra
// cuenta es aparte.
func TestCopiaRecuerdaEntreArranques(t *testing.T) {
	g, d := nuevaGaleria(t), nuevoDestino()
	g.poner("1", "IMG_1.jpg", "uno", tomaCopia)
	ruta := filepath.Join(t.TempDir(), "copia.json")
	if _, err := NuevaCopia(g, ruta).Ronda(context.Background(), "ana", d, nil); err != nil {
		t.Fatal(err)
	}
	c := NuevaCopia(g, ruta)
	if p, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || p.Total != 0 || d.leidas != 1 {
		t.Fatalf("tras arrancar otra vez: %+v %v, leídas %d", p, err, d.leidas)
	}
	// Otra cuenta en el mismo equipo empieza de cero, y su «/Fotos» es otro.
	otro := nuevoDestino()
	if p, err := c.Ronda(context.Background(), "berta", otro, nil); err != nil || p.Subidas != 1 {
		t.Fatalf("otra cuenta: %+v %v", p, err)
	}
	datos, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	for _, quien := range []string{"ana", "berta"} {
		if strings.Contains(string(datos), quien) {
			t.Errorf("el registro guarda la cuenta %q en claro", quien)
		}
	}
	c.Olvidar("ana")
	if !c.Ultima("ana").IsZero() {
		t.Error("Olvidar no ha olvidado")
	}
}

// TestCopiaNoSubeLoQueGuardoSytue y sin permiso no lee nada.
func TestCopiaNoSubeLoQueGuardoSytue(t *testing.T) {
	g, d := nuevaGaleria(t), nuevoDestino()
	g.poner("propia-1", "bajada.jpg", "exportada de la nube", tomaCopia)
	c := NuevaCopia(g, "")
	c.Marcar("ana", "propia-1")
	if p, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || p.Total != 0 || len(d.ficheros) != 0 {
		t.Fatalf("%+v %v %v", p, err, d.ficheros)
	}
	g.permiso = carrete.Denegado
	if _, err := c.Ronda(context.Background(), "ana", d, nil); !errors.Is(err, carrete.ErrSinPermiso) {
		t.Fatalf("sin permiso: %v", err)
	}
}

// TestCopiaConvierteLoQueLaGaleriaNoEnsena: una foto HEIC sube como JPEG con la fecha
// delante y se indexa; donde el sistema no sabe convertir, sube el original, que es la
// copia, sin pedir a la galería lo que no sabe enseñar. Las dos cuentan como hechas.
func TestCopiaConvierteLoQueLaGaleriaNoEnsena(t *testing.T) {
	g, d := nuevaGaleria(t), nuevoDestino()
	g.poner("1", "IMG_0001.HEIC", "heic uno", tomaCopia)
	c := NuevaCopia(g, "")
	if p, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || p.Subidas != 1 {
		t.Fatalf("%+v %v", p, err)
	}
	const jpeg = "2024-05-01 12.00.00 IMG_0001.jpg"
	if d.ficheros[jpeg] != "jpeg:heic uno" || d.tipos[jpeg] != "image/jpeg" || d.indexados[jpeg] != 1 {
		t.Fatalf("destino %v, tipos %v, indexados %v", d.ficheros, d.tipos, d.indexados)
	}

	g.sinConvertir = true
	g.poner("2", "IMG_0002.HEIC", "heic dos", tomaCopia.Add(time.Minute))
	if p, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || p.Subidas != 1 || p.Fallidas != 0 {
		t.Fatalf("sin convertir: %+v %v", p, err)
	}
	const original = "2024-05-01 12.01.00 IMG_0002.HEIC"
	if d.ficheros[original] != "heic dos" || d.indexados[original] != 0 {
		t.Fatalf("sin convertir: destino %v, indexados %v", d.ficheros, d.indexados)
	}
	// Un JPEG no se convierte: sube tal cual.
	antes := g.convertidas
	g.poner("3", "IMG_0003.JPG", "jpeg tres", tomaCopia.Add(2*time.Minute))
	if _, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || g.convertidas != antes {
		t.Fatalf("un JPEG se ha convertido (%d → %d): %v", antes, g.convertidas, err)
	}
}

// TestCopiaDeVideos: los vídeos suben tal cual y no se piden a la galería; dejarlos fuera
// los salta sin darlos por hechos, y volver a incluirlos los sube.
func TestCopiaDeVideos(t *testing.T) {
	g, d := nuevaGaleria(t), nuevoDestino()
	g.poner("v1", "VID_0001.MOV", "vídeo", tomaCopia)
	g.poner("1", "IMG_0001.JPG", "foto", tomaCopia.Add(time.Minute))
	c := NuevaCopia(g, "")
	if err := c.CambiarAjustes(func(a *AjustesCopia) { a.SinVideos = true }); err != nil {
		t.Fatal(err)
	}
	if p, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || p.Total != 1 || p.Subidas != 1 {
		t.Fatalf("sin vídeos: %+v %v", p, err)
	}
	_ = c.CambiarAjustes(func(a *AjustesCopia) { a.SinVideos = false })
	if p, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || p.Total != 1 || p.Subidas != 1 {
		t.Fatalf("con vídeos: %+v %v", p, err)
	}
	const video = "2024-05-01 12.00.00 VID_0001.MOV"
	if d.ficheros[video] != "vídeo" || d.tipos[video] != "video/quicktime" || d.indexados[video] != 0 || g.convertidas != 0 {
		t.Fatalf("el vídeo: destino %v, tipos %v, indexados %v, convertidas %d", d.ficheros, d.tipos, d.indexados, g.convertidas)
	}
}

// TestCopiaEsperaALaWifi: con datos móviles no sube nada (ni lee «/Fotos») hasta que se
// permiten; y si la Wi-Fi se va a media ronda, la ronda se para ahí.
func TestCopiaEsperaALaWifi(t *testing.T) {
	g, d := nuevaGaleria(t), nuevoDestino()
	for i, n := range []string{"1", "2", "3"} {
		g.poner(n, "IMG_"+n+".jpg", "contenido "+n, tomaCopia.Add(time.Duration(i)*time.Minute))
	}
	g.medida = true
	c := NuevaCopia(g, "")
	if _, err := c.Ronda(context.Background(), "ana", d, nil); !errors.Is(err, ErrCopiaEsperaWifi) || d.leidas != 0 || len(d.ficheros) != 0 {
		t.Fatalf("con datos: %v, leídas %d, destino %v", err, d.leidas, d.ficheros)
	}
	g.medida = false
	d.fallar = func(string) error {
		g.mu.Lock()
		g.medida = true // se va la Wi-Fi tras la primera
		g.mu.Unlock()
		return nil
	}
	p, err := c.Ronda(context.Background(), "ana", d, nil)
	if !errors.Is(err, ErrCopiaEsperaWifi) || p.Subidas != 1 {
		t.Fatalf("sin Wi-Fi a media ronda: %+v %v", p, err)
	}
	d.fallar = nil
	_ = c.CambiarAjustes(func(a *AjustesCopia) { a.ConDatos = true })
	if p, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || p.Subidas != 2 {
		t.Fatalf("con datos permitidos: %+v %v", p, err)
	}
}

// TestCopiaReintentaElIndiceSinVolverASubir: si la galería no apunta una foto ya subida
// (un fallo de red), la foto cuenta como copiada y la siguiente ronda solo la indexa; lo
// que la galería no sabe enseñar no se reintenta.
func TestCopiaReintentaElIndiceSinVolverASubir(t *testing.T) {
	g, d := nuevaGaleria(t), nuevoDestino()
	g.poner("1", "IMG_1.jpg", "uno", tomaCopia)
	sinRed := errors.New("sin red")
	d.indexar = func(string) error { return sinRed }
	c := NuevaCopia(g, "")
	if p, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || p.Subidas != 1 || p.Fallidas != 0 {
		t.Fatalf("%+v %v", p, err)
	}
	d.indexar = nil
	if p, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || p.Total != 0 || len(d.ficheros) != 1 {
		t.Fatalf("segunda ronda %+v %v, destino %v", p, err, d.ficheros)
	}
	if d.indexados["2024-05-01 12.00.00 IMG_1.jpg"] != 1 {
		t.Fatalf("no se ha reintentado el índice: %v", d.indexados)
	}
	pedidas := 0
	d.indexar = func(string) error { pedidas++; return ErrNoIndexable }
	if _, err := c.Ronda(context.Background(), "ana", d, nil); err != nil || pedidas != 0 {
		t.Fatalf("lo ya indexado se ha vuelto a pedir %d veces: %v", pedidas, err)
	}
}

// TestCopiaAjustesEnElRegistro: los ajustes del dispositivo van en el registro en disco,
// que es lo que lee la copia con la app cerrada; el cero es lo recomendado; y CopiaDe da
// la MISMA copia para un mismo fichero en el proceso.
func TestCopiaAjustesEnElRegistro(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "copia.json")
	g := nuevaGaleria(t)
	c := NuevaCopia(g, ruta)
	if aj := c.Ajustes(); aj.Activa || aj.ConDatos || aj.SinVideos {
		t.Fatalf("lo recomendado es apagada, solo Wi-Fi y con vídeos: %+v", aj)
	}
	if err := c.CambiarAjustes(func(a *AjustesCopia) { a.Activa, a.ConDatos = true, true }); err != nil {
		t.Fatal(err)
	}
	if aj := NuevaCopia(g, ruta).Ajustes(); !aj.Activa || !aj.ConDatos || aj.SinVideos {
		t.Fatalf("leídos de disco: %+v", aj)
	}
	if una, otra := CopiaDe(g, ruta), CopiaDe(g, ruta); una != otra {
		t.Fatal("dos Copia sobre el mismo registro en el mismo proceso")
	}
}
