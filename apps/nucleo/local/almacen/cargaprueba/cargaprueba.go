// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package cargaprueba es la plomería común de la prueba de carga del almacén de Sytue
// Office (PLAN-EDICION-LOCAL §5.6, R-MEDIR): generar una carga grande una vez, abrirla
// en frío en un proceso nuevo, medir su memoria y repetir cada medida para dar la
// mediana. Cada carga vive en el paquete de su dominio (notas, fotos, contactos,
// documentos), con su generador y con el repositorio de verdad sobre el puerto real en
// disco. Solo lo importan las pruebas.
//
// Las pruebas de carga no corren en la suite normal: piden la variable SYTUE_CARGA=1, y
// nunca con -short.
//
//	SYTUE_CARGA=1 go test -tags goolm -run 'Carga' -v -timeout 0 ./notes/ ./photos/ ./contacts/ ./local/...
//
// Variables:
//
//   - SYTUE_CARGA_DIR: guarda las cargas generadas en esa carpeta y las reutiliza en la
//     siguiente ejecución (generar 50.000 fotos lleva minutos). Sin ella, cada carga va a
//     una carpeta temporal que se borra al acabar la prueba.
//   - SYTUE_CARGA_VECES: cuántas veces se repite cada medida (5 por defecto); se da la
//     mediana y el intervalo.
//   - SYTUE_CARGA_INFORME: un fichero al que se añaden las filas del informe (Markdown).
//   - SYTUE_CARGA_PERFIL_MEMORIA: el perfil de memoria (pprof) del proceso hijo con todo
//     abierto; para el de CPU, se corre el hijo a mano con SYTUE_CARGA_HIJO=<carpeta> y
//     -test.cpuprofile.
package cargaprueba

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	// Variable activa las pruebas de carga.
	Variable = "SYTUE_CARGA"
	// VariableDir guarda las cargas generadas entre ejecuciones.
	VariableDir = "SYTUE_CARGA_DIR"
	// VariableVeces es cuántas veces se repite cada medida.
	VariableVeces = "SYTUE_CARGA_VECES"
	// VariableInforme es el fichero al que se añaden las filas del informe.
	VariableInforme = "SYTUE_CARGA_INFORME"
	// VariablePerfilMemoria es dónde deja el proceso hijo su perfil de memoria.
	VariablePerfilMemoria = "SYTUE_CARGA_PERFIL_MEMORIA"

	variableHijo = "SYTUE_CARGA_HIJO"
	marcaHijo    = "@@carga "
)

// Activa salta la prueba si no se pidió la carga (SYTUE_CARGA=1) o si corre con -short.
func Activa(tb testing.TB) {
	tb.Helper()
	if testing.Short() {
		tb.Skip("prueba de carga: no corre con -short")
	}
	if os.Getenv(Variable) == "" && os.Getenv(variableHijo) == "" {
		tb.Skip("prueba de carga: se pide con " + Variable + "=1")
	}
}

// Veces es cuántas veces se repite cada medida: SYTUE_CARGA_VECES, o 5.
func Veces() int {
	if n, err := strconv.Atoi(os.Getenv(VariableVeces)); err == nil && n > 0 {
		return n
	}
	return 5
}

// Plantilla es la carpeta de la carga nombre, generada con generar la primera vez. Con
// SYTUE_CARGA_DIR se guarda ahí y se reutiliza (una a medias, sin su marca de completa,
// se rehace); sin ella, va a una carpeta temporal que se borra al acabar la prueba.
// Dice cuánto tardó en generarse y cuánto ocupa.
func Plantilla(t *testing.T, nombre string, generar func(dir string) error) string {
	t.Helper()
	base := os.Getenv(VariableDir)
	var dir, marca string
	if base == "" {
		dir = filepath.Join(t.TempDir(), nombre)
	} else {
		dir, marca = filepath.Join(base, nombre), filepath.Join(base, nombre+".completa")
		if _, err := os.Stat(marca); err == nil { //nolint:gosec // G703: la carpeta de la carga la elige quien corre la prueba (SYTUE_CARGA_DIR)
			t.Logf("carga %s reutilizada de %s (%s en disco)", nombre, dir, Megas(Ocupa(dir)))
			return dir
		}
		if err := os.RemoveAll(dir); err != nil { //nolint:gosec // G703: la carpeta de la carga la elige quien corre la prueba (SYTUE_CARGA_DIR)
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil { //nolint:gosec // G703: la carpeta de la carga la elige quien corre la prueba (SYTUE_CARGA_DIR)
		t.Fatal(err)
	}
	inicio := time.Now()
	if err := generar(dir); err != nil {
		t.Fatalf("generar la carga %s: %v", nombre, err)
	}
	t.Logf("carga %s generada en %s (%s en disco)", nombre, time.Since(inicio).Round(time.Millisecond), Megas(Ocupa(dir)))
	if marca != "" {
		if err := os.WriteFile(marca, nil, 0o600); err != nil { //nolint:gosec // G703: la carpeta de la carga la elige quien corre la prueba (SYTUE_CARGA_DIR)
			t.Fatal(err)
		}
	}
	return dir
}

// Copia es una copia de la carga dir que la prueba puede cambiar (sin el cerrojo del
// almacén), en una carpeta temporal que se borra al acabar.
func Copia(tb testing.TB, dir string) string {
	tb.Helper()
	destino := filepath.Join(tb.TempDir(), filepath.Base(dir))
	if err := copiar(dir, destino); err != nil {
		tb.Fatal(err)
	}
	return destino
}

// EnMemoria genera con generar en una carpeta en memoria (tmpfs: /dev/shm en Linux) y
// después la copia a dir. El almacén sincroniza cada línea que escribe (fsync), y eso
// es lo que cuesta escribir una carga grande en disco (importar una foto son unas
// cuarenta escrituras sincronizadas: 150 ms en este equipo frente a 7 ms en memoria);
// en dir quedan los mismos ficheros. Sin tmpfs, genera directamente en dir.
func EnMemoria(dir string, generar func(string) error) error {
	tmp, quitar, err := CarpetaEnMemoria()
	if err != nil {
		return generar(dir)
	}
	defer quitar()
	if err := generar(tmp); err != nil {
		return err
	}
	return copiar(tmp, dir)
}

// CarpetaEnMemoria es una carpeta nueva en tmpfs (/dev/shm) y cómo quitarla; error si el
// sistema no tiene tmpfs.
func CarpetaEnMemoria() (string, func(), error) {
	if info, err := os.Stat("/dev/shm"); err != nil || !info.IsDir() {
		return "", nil, errors.New("cargaprueba: sin tmpfs")
	}
	tmp, err := os.MkdirTemp("/dev/shm", "sytue-carga-")
	if err != nil {
		return "", nil, err
	}
	return tmp, func() { _ = os.RemoveAll(tmp) }, nil
}

// CopiarFichero copia el fichero origen en destino, si destino no existe ya.
func CopiarFichero(origen, destino string) error {
	if _, err := os.Stat(destino); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(destino), 0o700); err != nil {
		return err
	}
	return copiarFichero(origen, destino)
}

// CopiarArbol copia el árbol origen en destino, sin el cerrojo del almacén.
func CopiarArbol(origen, destino string) error { return copiar(origen, destino) }

// copiar copia el árbol origen en destino, sin el cerrojo del almacén.
func copiar(origen, destino string) error {
	return filepath.WalkDir(origen, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(origen, ruta)
		if err != nil {
			return err
		}
		fin := filepath.Join(destino, rel)
		if d.IsDir() {
			return os.MkdirAll(fin, 0o700)
		}
		if d.Name() == "cerrojo" {
			return nil
		}
		return copiarFichero(ruta, fin)
	})
}

func copiarFichero(origen, destino string) error {
	o, err := os.Open(origen)
	if err != nil {
		return err
	}
	defer o.Close()
	f, err := os.OpenFile(destino, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, o); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Ocupa es lo que ocupan los ficheros de dir (su tamaño, no los bloques).
func Ocupa(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error { //nolint:gosec // G703: la carpeta de la carga la elige quien corre la prueba (SYTUE_CARGA_DIR)
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// Megas escribe n bytes en MiB.
func Megas(n int64) string { return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20)) }

// ----- Proceso hijo: la apertura en frío -----

// Medidas son las de una apertura en un proceso nuevo: los tiempos con nombre, en
// milisegundos, en el orden en que se tomaron, y la memoria del proceso al acabar.
type Medidas struct {
	Nombres []string           `json:"nombres"`
	Tiempos map[string]float64 `json:"tiempos"`
	// HeapInuse es la memoria en uso del montón de Go tras un GC, con todo lo abierto
	// aún vivo.
	HeapInuse uint64 `json:"heap_inuse"`
	// Sys es lo que el runtime de Go ha pedido al sistema.
	Sys uint64 `json:"sys"`
	// RSS y PicoRSS son la memoria residente del proceso al acabar y su máximo (VmRSS y
	// VmHWM de /proc/self/status; 0 fuera de Linux). Incluyen el binario de pruebas.
	RSS     uint64 `json:"rss"`
	PicoRSS uint64 `json:"pico_rss"`
}

// Reloj toma los tiempos en el proceso hijo.
type Reloj struct{ m *Medidas }

// Medir cronometra f con nombre.
func (r *Reloj) Medir(nombre string, f func() error) error {
	inicio := time.Now()
	err := f()
	r.m.Nombres = append(r.m.Nombres, nombre)
	r.m.Tiempos[nombre] = float64(time.Since(inicio).Microseconds()) / 1000
	return err
}

// Hija dice si la prueba corre como proceso hijo de [EnFrio] y, si lo es, con qué
// carpeta.
func Hija() (string, bool) {
	dir := os.Getenv(variableHijo)
	return dir, dir != ""
}

// Responder es lo que hace la prueba en el proceso hijo: abrir con el reloj, medir la
// memoria con todo lo abierto aún vivo y contestar al padre. cerrar suelta lo abierto
// después de medir.
func Responder(t *testing.T, abrir func(r *Reloj) (cerrar func(), err error)) {
	t.Helper()
	m := Medidas{Tiempos: map[string]float64{}}
	cerrar, err := abrir(&Reloj{m: &m})
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m.HeapInuse, m.Sys = ms.HeapInuse, ms.Sys
	m.RSS, m.PicoRSS = memoriaDelProceso()
	// Con SYTUE_CARGA_PERFIL_MEMORIA, el perfil de memoria con todo abierto (el de
	// -test.memprofile se toma al acabar, ya cerrado): dónde se va la memoria.
	if ruta := os.Getenv(VariablePerfilMemoria); ruta != "" {
		if f, err := os.Create(ruta); err == nil { //nolint:gosec // G703: la carpeta de la carga la elige quien corre la prueba (SYTUE_CARGA_PERFIL_MEMORIA)
			_ = pprof.WriteHeapProfile(f)
			_ = f.Close()
		}
	}
	if cerrar != nil {
		cerrar()
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(marcaHijo + string(raw))
}

// memoriaDelProceso lee VmRSS y VmHWM de /proc/self/status, en bytes.
func memoriaDelProceso() (rss, pico uint64) {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, 0
	}
	for linea := range strings.SplitSeq(string(raw), "\n") {
		campos := strings.Fields(linea)
		if len(campos) < 2 {
			continue
		}
		kb, err := strconv.ParseUint(campos[1], 10, 64)
		if err != nil {
			continue
		}
		switch campos[0] {
		case "VmRSS:":
			rss = kb << 10
		case "VmHWM:":
			pico = kb << 10
		}
	}
	return rss, pico
}

// EnFrio abre la carga dir tantas veces como pide veces, cada una en un proceso nuevo: el mismo binario
// de pruebas, que corre solo la prueba t y, al verse hija ([Hija]), abre y contesta
// ([Responder]). Así cada apertura empieza sin nada en memoria (la caché de disco del
// sistema sí está caliente: la carga se acaba de escribir o de leer).
func EnFrio(t *testing.T, dir string, veces int) []Medidas {
	t.Helper()
	partes := strings.Split(t.Name(), "/")
	for i, p := range partes {
		partes[i] = "^" + regexp.QuoteMeta(p) + "$"
	}
	out := make([]Medidas, 0, veces)
	for range veces {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run", strings.Join(partes, "/"), "-test.count=1", "-test.timeout=0") //nolint:gosec // G204: es el propio binario de pruebas
		cmd.Env = append(os.Environ(), variableHijo+"="+dir)
		var salida bytes.Buffer
		cmd.Stdout, cmd.Stderr = &salida, &salida
		if err := cmd.Run(); err != nil {
			t.Fatalf("apertura en frío: %v\n%s", err, salida.String())
		}
		m, ok := leerRespuesta(salida.Bytes())
		if !ok {
			t.Fatalf("apertura en frío: el hijo no contestó\n%s", salida.String())
		}
		out = append(out, m)
	}
	return out
}

func leerRespuesta(salida []byte) (Medidas, bool) {
	s := bufio.NewScanner(bytes.NewReader(salida))
	s.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for s.Scan() {
		if resto, ok := strings.CutPrefix(s.Text(), marcaHijo); ok {
			var m Medidas
			if json.Unmarshal([]byte(resto), &m) == nil {
				return m, true
			}
		}
	}
	return Medidas{}, false
}

// ----- Repetir y resumir -----

// Serie son las repeticiones de una medida, en milisegundos.
type Serie []float64

// Mediana de la serie.
func (s Serie) Mediana() float64 {
	if len(s) == 0 {
		return 0
	}
	o := slices.Clone(s)
	slices.Sort(o)
	if len(o)%2 == 1 {
		return o[len(o)/2]
	}
	return (o[len(o)/2-1] + o[len(o)/2]) / 2
}

// String es «mediana (mínimo–máximo)».
func (s Serie) String() string {
	if len(s) == 0 {
		return "—"
	}
	return fmt.Sprintf("%s (%s–%s)", ms(s.Mediana()), ms(slices.Min(s)), ms(slices.Max(s)))
}

func ms(v float64) string {
	switch {
	case v >= 1000:
		return fmt.Sprintf("%.2f s", v/1000)
	case v >= 10:
		return fmt.Sprintf("%.0f ms", v)
	default:
		return fmt.Sprintf("%.1f ms", v)
	}
}

// Tiempo es la serie del tiempo nombre en las medidas de [EnFrio].
func Tiempo(ms []Medidas, nombre string) Serie {
	s := make(Serie, 0, len(ms))
	for _, m := range ms {
		if v, ok := m.Tiempos[nombre]; ok {
			s = append(s, v)
		}
	}
	return s
}

// Suma es la serie de la suma de los tiempos nombres en cada medida de [EnFrio].
func Suma(ms []Medidas, nombres ...string) Serie {
	s := make(Serie, 0, len(ms))
	for _, m := range ms {
		total := 0.0
		for _, n := range nombres {
			total += m.Tiempos[n]
		}
		s = append(s, total)
	}
	return s
}

// Memoria es la serie, en MiB, de lo que elige de cada medida de [EnFrio].
func Memoria(ms []Medidas, de func(Medidas) uint64) Serie {
	s := make(Serie, 0, len(ms))
	for _, m := range ms {
		s = append(s, float64(de(m))/(1<<20))
	}
	return s
}

// MiB escribe una serie de memoria.
func MiB(s Serie) string {
	if len(s) == 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f MiB (%.0f–%.0f)", s.Mediana(), slices.Min(s), slices.Max(s))
}

// Repetir cronometra f, tantas veces como pide veces, en este proceso; prep, si no es nil, va antes de
// cada vez y fuera del reloj.
func Repetir(tb testing.TB, veces int, prep func(i int), f func(i int) error) Serie {
	tb.Helper()
	s := make(Serie, 0, veces)
	for i := range veces {
		if prep != nil {
			prep(i)
		}
		inicio := time.Now()
		if err := f(i); err != nil {
			tb.Fatal(err)
		}
		s = append(s, float64(time.Since(inicio).Microseconds())/1000)
	}
	return s
}

// ----- Informe -----

var informeMu sync.Mutex

// Fila es una línea del informe: la carga, qué se mide, el valor, el umbral del plan
// (vacío si no tiene) y si lo cumple.
type Fila struct {
	Carga, Medida, Valor, Umbral string
	Cumple                       bool
}

// Informar escribe las filas en el registro de la prueba y, con SYTUE_CARGA_INFORME,
// las añade a ese fichero como tabla Markdown, con la carga del equipo.
func Informar(t *testing.T, filas ...Fila) {
	t.Helper()
	carga := CargaDelEquipo()
	var b strings.Builder
	for _, f := range filas {
		veredicto := ""
		if f.Umbral != "" {
			veredicto = "cumple"
			if !f.Cumple {
				veredicto = "NO CUMPLE"
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", f.Carga, f.Medida, f.Valor, f.Umbral, veredicto, carga)
	}
	t.Logf("\n| Carga | Medida | Mediana (mín–máx) | Umbral | | Carga del equipo |\n|---|---|---|---|---|---|\n%s", b.String())
	ruta := os.Getenv(VariableInforme)
	if ruta == "" {
		return
	}
	informeMu.Lock()
	defer informeMu.Unlock()
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // G703: la carpeta de la carga la elige quien corre la prueba (SYTUE_CARGA_INFORME)
	if err != nil {
		t.Log(err)
		return
	}
	defer f.Close()
	_, _ = f.WriteString(b.String())
}

// CargaDelEquipo es la carga media del sistema y sus núcleos («3,1 / 1,0 / 0,5 en 4
// CPU»), para leer las medidas sabiendo con qué se compartía la máquina.
func CargaDelEquipo() string {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return fmt.Sprintf("%d CPU", runtime.NumCPU())
	}
	campos := strings.Fields(string(raw))
	if len(campos) < 3 {
		return fmt.Sprintf("%d CPU", runtime.NumCPU())
	}
	return fmt.Sprintf("%s / %s / %s en %d CPU", campos[0], campos[1], campos[2], runtime.NumCPU())
}
