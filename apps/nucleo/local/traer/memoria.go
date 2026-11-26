// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package traer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// # Lo que ya se trajo
//
// Repetir «Traer» no copia a Drive otra vez lo que ya dejó allí: un fichero del ZIP con
// la MISMA ruta (sin la carpeta de arriba, que cambia con la fecha de cada copia: es
// `drive/files/Obras/planos.pdf`) y la MISMA huella (el SHA-256 de sus bytes) se deja
// una sola vez, y las otras se cuenta como «ya estaba». Vale para todo lo que va a la
// carpeta de lo traído: los ficheros de Drive, lo que no tiene app y los originales de
// las fotos y los vídeos que enlaza Photos.
//
// Se recuerda en `<Datos>/Drive/.traido-de-sytue.json`, junto a las carpetas de lo
// traído y fuera de ellas (Drive no enseña lo que empieza por punto): por cada ruta del
// ZIP, la huella, el tamaño y dónde quedó, relativo a Drive. Así sobrevive a que la
// persona mueva, renombre o borre esas carpetas:
//
//   - lo que sigue donde se dejó, ya estaba (aunque la persona lo haya cambiado: lo
//     suyo no se pisa ni se duplica);
//   - lo que no está donde se dejó se busca en todo Drive por su tamaño y su huella
//     (una vez por vuelta, y solo si falta algo) y, si está en otro sitio —la carpeta
//     movida o renombrada—, se apunta allí y ya estaba;
//   - si no está en ningún sitio, se borró: se copia otra vez.
//
// De una HEIC que entró en Photos como JPEG se recuerda además la huella de ese JPEG,
// para reconocerla sin convertirla otra vez.
//
// Y de lo que entró en un almacén sin un origen propio donde apuntarlo (repetir.go): el
// calendario de aquí al que fue cada `.ics` y la libreta «Sytue», con la huella de cada
// evento y cada contacto como vino de Sytue y como quedó aquí al traerlo; el documento
// de aquí de cada fichero de `docs/documents`, igual; y los correos que se llevaron a
// «Importados». Si la memoria falta o no se lee, se empieza de nuevo: lo peor es copiar
// otra vez, nunca perder ni pisar nada (y lo que se reconoce sin ella —un evento o un
// contacto por su UID, un documento por su origen, un correo por su Message-ID— no se
// repite tampoco).

// ficheroMemoria es el nombre de la memoria, en la raíz de Drive.
const ficheroMemoria = ".traido-de-sytue.json"

// memoria es lo que «Traer» recuerda de otras vueltas.
type memoria struct {
	V int `json:"v"`
	// Ficheros son, por la ruta en el ZIP, lo que se dejó de ella (uno por huella).
	Ficheros map[string][]dejado `json:"ficheros"`
	// Convertidas son, por la huella de una HEIC, la del JPEG que entró en Photos.
	Convertidas map[string]string `json:"convertidas,omitempty"`
	// Calendarios son, por la ruta de su `.ics` en la copia, el calendario de aquí al que
	// se trajo y sus eventos; Libretas, la libreta de aquí de los contactos y los suyos.
	Calendarios map[string]*coleccion `json:"calendarios,omitempty"`
	Libretas    map[string]*coleccion `json:"libretas,omitempty"`
	// Documentos son, por su ruta en la copia, el documento de aquí que la tiene.
	Documentos map[string]*docTraido `json:"documentos,omitempty"`
	// Correos son las claves (huellaCorta del Message-ID o del .eml) de los correos que
	// se llevaron a «Importados»: uno que la persona quitó de allí no vuelve.
	Correos map[string]bool `json:"correos,omitempty"`
}

// coleccion es un calendario o una libreta de aquí con lo que se trajo a él: por el UID
// de cada evento o contacto, cómo vino y cómo quedó.
type coleccion struct {
	ID        string            `json:"id"`
	Elementos map[string]traido `json:"elementos,omitempty"`
}

// traido es cómo vino de Sytue una cosa (S: la huella de lo de la copia, sin lo que
// cambia sin que cambie nada, como DTSTAMP o REV), cómo quedó aquí al traerla (A: la
// huella de lo guardado) y de qué copia vino (C: su fecha, AAAA-MM-DD, si se sabe).
// Si lo de aquí sigue con la huella A, la persona no lo ha tocado.
type traido struct {
	S string `json:"s"`
	A string `json:"a"`
	C string `json:"c,omitempty"`
}

// docTraido es el documento de aquí que tiene un fichero de la copia: de qué biblioteca
// es y su id; su A es la huella de su contenido.
type docTraido struct {
	Producto string `json:"producto"`
	ID       string `json:"id"`
	traido
}

// vacia dice si no hay nada que recordar.
func (m *memoria) vacia() bool {
	return len(m.Ficheros) == 0 && len(m.Convertidas) == 0 && len(m.Calendarios) == 0 &&
		len(m.Libretas) == 0 && len(m.Documentos) == 0 && len(m.Correos) == 0
}

// dejado es un fichero del ZIP que se dejó en Drive.
type dejado struct {
	Huella string `json:"huella"`
	Bytes  int64  `json:"bytes"`
	// Ruta es dónde quedó, relativa a Drive y con barras normales.
	Ruta string `json:"ruta"`
}

// leerMemoria lee la memoria de la carpeta de Drive; sin ella, o ilegible, una vacía.
func leerMemoria(drive string) *memoria {
	m := &memoria{
		V: 1, Ficheros: map[string][]dejado{}, Convertidas: map[string]string{}, Calendarios: map[string]*coleccion{},
		Libretas: map[string]*coleccion{}, Documentos: map[string]*docTraido{}, Correos: map[string]bool{},
	}
	datos, err := os.ReadFile(filepath.Join(drive, ficheroMemoria)) //nolint:gosec // G304: la nuestra
	if err != nil {
		return m
	}
	var leida memoria
	if json.Unmarshal(datos, &leida) != nil || leida.V != 1 {
		return m
	}
	for ruta, ds := range leida.Ficheros {
		for _, d := range ds {
			if d.Huella != "" && d.Ruta != "" && filepath.IsLocal(filepath.FromSlash(d.Ruta)) {
				m.Ficheros[ruta] = append(m.Ficheros[ruta], d)
			}
		}
	}
	for heic, jpeg := range leida.Convertidas {
		if heic != "" && jpeg != "" {
			m.Convertidas[heic] = jpeg
		}
	}
	sinVacias(m.Calendarios, leida.Calendarios)
	sinVacias(m.Libretas, leida.Libretas)
	for ruta, d := range leida.Documentos {
		if d != nil && d.ID != "" && d.Producto != "" {
			m.Documentos[ruta] = d
		}
	}
	for k, v := range leida.Correos {
		if k != "" && v {
			m.Correos[k] = true
		}
	}
	return m
}

// sinVacias pasa a destino las colecciones leídas que dicen a qué calendario o libreta
// de aquí van.
func sinVacias(destino, leidas map[string]*coleccion) {
	for ruta, c := range leidas {
		if c != nil && c.ID != "" {
			if c.Elementos == nil {
				c.Elementos = map[string]traido{}
			}
			destino[ruta] = c
		}
	}
}

// guardar escribe la memoria en un temporal y lo pone en su sitio de una vez.
func (m *memoria) guardar(drive string) error {
	if err := os.MkdirAll(drive, 0o700); err != nil {
		return err
	}
	datos, err := json.Marshal(m)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(drive, ficheroMemoria+".*")
	if err != nil {
		return err
	}
	_, err = f.Write(datos)
	if err = errors.Join(err, f.Sync(), f.Close()); err == nil {
		err = os.Rename(f.Name(), filepath.Join(drive, ficheroMemoria))
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}

// recordar apunta que la ruta del ZIP con esa huella quedó en d.
func (m *memoria) recordar(ruta string, d dejado) {
	ds := m.Ficheros[ruta]
	if i := slices.IndexFunc(ds, func(x dejado) bool { return x.Huella == d.Huella }); i >= 0 {
		ds[i] = d
		return
	}
	m.Ficheros[ruta] = append(ds, d)
}

// dejar deja e en la carpeta de lo traído, en rel, una sola vez: si ya se dejó en otra
// vuelta (la misma ruta del ZIP con la misma huella) y sigue en Drive, donde se dejó o
// donde la persona la haya movido, no se copia y dice dónde está (ya). huella puede ir
// vacía: se calcula si hace falta.
func (r *repartidor) dejar(e entrada, rel, huella string) (donde string, ya bool, err error) {
	if previos := r.mem.Ficheros[e.ruta]; len(previos) > 0 {
		if huella == "" {
			if huella, err = huellaDeEntrada(e); err != nil {
				return "", false, err
			}
		}
		if i := slices.IndexFunc(previos, func(d dejado) bool { return d.Huella == huella }); i >= 0 {
			if donde, ok := r.localizar(e.ruta, previos[i]); ok {
				return donde, true, nil
			}
		}
	}
	destino, err := r.copiarYRecordar(e, rel)
	return destino, false, err
}

// copiarYRecordar copia e en la carpeta de lo traído, en rel (sin pisar nada), y apunta
// dónde ha quedado.
func (r *repartidor) copiarYRecordar(e entrada, rel string) (string, error) {
	destino, suma, n, err := r.copiar(e, rel)
	if err != nil {
		return "", err
	}
	if relativa, err := filepath.Rel(r.raizDrive, destino); err == nil && filepath.IsLocal(relativa) {
		r.mem.recordar(e.ruta, dejado{Huella: suma, Bytes: n, Ruta: filepath.ToSlash(relativa)})
		r.reclamar(destino)
	}
	return destino, nil
}

// original es [repartidor.dejar] para el original de una foto o un vídeo, que Photos
// enlaza y tiene que ser EXACTAMENTE el de la copia: si el que ya estaba en Drive de otra
// vuelta lo ha cambiado la persona (lo suyo no se toca), se deja otra copia al lado.
func (r *repartidor) original(e entrada, rel, huella string) (string, error) {
	donde, ya, err := r.dejar(e, rel, huella)
	if err != nil || !ya {
		return donde, err
	}
	if h, err := huellaDeFichero(donde); err == nil && h == huella {
		return donde, nil
	}
	return r.copiarYRecordar(e, rel)
}

// localizar busca lo dejado d de la ruta del ZIP: donde se dejó o, si ya no está, en
// todo Drive por su tamaño y su huella. Si lo encuentra en otro sitio, lo apunta.
func (r *repartidor) localizar(ruta string, d dejado) (string, bool) {
	abs := filepath.Join(r.raizDrive, filepath.FromSlash(d.Ruta))
	if fi, err := os.Lstat(abs); err == nil && fi.Mode().IsRegular() {
		return abs, true
	}
	otra, ok := r.buscarEnDrive(d.Bytes, d.Huella)
	if !ok {
		return "", false
	}
	if relativa, err := filepath.Rel(r.raizDrive, otra); err == nil {
		d.Ruta = filepath.ToSlash(relativa)
		r.mem.recordar(ruta, d)
		r.reclamar(otra)
	}
	return otra, true
}

// reclamar apunta que el fichero de ruta ya es lo dejado de una entrada: buscar lo
// movido de otra no lo coge (dos ficheros del ZIP iguales en dos carpetas son dos).
func (r *repartidor) reclamar(ruta string) {
	if r.reclamados != nil {
		r.reclamados[filepath.Clean(ruta)] = true
	}
}

// buscarEnDrive busca en Drive un fichero de ese tamaño y esa huella que no sea lo
// dejado de otra entrada. Recorre Drive una vez por vuelta (sin lo oculto ni los
// enlaces), y la huella de cada candidato se calcula una vez.
func (r *repartidor) buscarEnDrive(bytes int64, huella string) (string, bool) {
	if r.enDrive == nil {
		r.recorrerDrive()
	}
	for _, c := range r.enDrive[bytes] {
		if r.reclamados[c] {
			continue
		}
		h, ok := r.huellas[c]
		if !ok {
			h, _ = huellaDeFichero(c)
			r.huellas[c] = h
		}
		if h == huella {
			return c, true
		}
	}
	return "", false
}

// recorrerDrive apunta los ficheros de Drive por su tamaño (sin lo oculto ni los
// enlaces) y, como reclamados, los que se dejaron en otras vueltas y siguen ahí.
func (r *repartidor) recorrerDrive() {
	r.enDrive, r.huellas, r.reclamados = map[int64][]string{}, map[string]string{}, map[string]bool{}
	for _, ds := range r.mem.Ficheros {
		for _, d := range ds {
			abs := filepath.Join(r.raizDrive, filepath.FromSlash(d.Ruta))
			if fi, err := os.Lstat(abs); err == nil && fi.Mode().IsRegular() {
				r.reclamados[abs] = true
			}
		}
	}
	_ = filepath.WalkDir(r.raizDrive, func(ruta string, de fs.DirEntry, err error) error {
		if r.ctx.Err() != nil {
			return r.ctx.Err()
		}
		if err != nil {
			if de != nil && de.IsDir() && ruta != r.raizDrive {
				return fs.SkipDir
			}
			return nil
		}
		if ruta != r.raizDrive && strings.HasPrefix(de.Name(), ".") {
			if de.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !de.Type().IsRegular() {
			return nil
		}
		if fi, err := de.Info(); err == nil {
			r.enDrive[fi.Size()] = append(r.enDrive[fi.Size()], filepath.Clean(ruta))
		}
		return nil
	})
}

// huellaCorta es la de unos bytes en la memoria de lo traído: la mitad del SHA-256, en
// hexadecimal (sobra para saber si algo cambió, y la memoria de miles de eventos o
// correos se queda en poco).
func huellaCorta(datos []byte) string {
	suma := sha256.Sum256(datos)
	return hex.EncodeToString(suma[:16])
}

// huellaSin es la huella corta de un iCalendar o una vCard sin las propiedades sin
// (DTSTAMP, PRODID, REV…), que cambian cada vez que se escribe aunque no cambie nada:
// con las líneas desplegadas (RFC 5545 §3.1) y los saltos de línea iguales.
func huellaSin(texto string, sin ...string) string {
	texto = strings.ReplaceAll(texto, "\r\n", "\n")
	texto = strings.ReplaceAll(strings.ReplaceAll(texto, "\n ", ""), "\n\t", "")
	h := sha256.New()
	for linea := range strings.SplitSeq(texto, "\n") {
		if strings.TrimSpace(linea) == "" || slices.Contains(sin, propiedad(linea)) {
			continue
		}
		_, _ = io.WriteString(h, linea)
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// propiedad es el nombre de la propiedad de una línea de un iCalendar o una vCard, en
// mayúsculas y sin su grupo (`item1.EMAIL;TYPE=work:…` es EMAIL).
func propiedad(linea string) string {
	nombre := linea
	if i := strings.IndexAny(nombre, ":;"); i >= 0 {
		nombre = nombre[:i]
	}
	if i := strings.LastIndexByte(nombre, '.'); i >= 0 {
		nombre = nombre[i+1:]
	}
	return strings.ToUpper(strings.TrimSpace(nombre))
}

// huellaDeFichero es el SHA-256 de un fichero, en hexadecimal.
func huellaDeFichero(ruta string) (string, error) {
	f, err := os.Open(ruta) //nolint:gosec // G304: un fichero de Drive de la persona
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
