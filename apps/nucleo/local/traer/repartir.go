// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package traer

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

// Destino es dónde va lo traído: la carpeta de datos de la edición
// (`espacio.Rutas.Datos`), quién es la persona en sus almacenes (`espacio.ID`) y cómo
// firma los documentos (`espacio.Persona()`).
type Destino struct {
	Datos   string
	Sujeto  string
	Persona biblioteca.Persona
	// Secretos los pide abrir el correo; no se lee ni escribe ninguno al importar.
	Secretos correo.Secretos
	// Ahora fecha la carpeta de lo traído; nil, la hora del sistema.
	Ahora func() time.Time
	// Video mide un vídeo de Photos ya dejado en Drive (su duración, sus lados y un
	// fotograma para el póster), como Photos al enlazar uno; nil, o lo que no sepa, entra
	// con los lados que diga la copia y un póster neutro.
	Video func(ctx context.Context, ruta string) Video
	// HEIC pasa a JPEG, con lo que tenga el sistema, una foto HEIC ya dejada en Drive, y
	// dice dónde ha dejado el JPEG (limpiar lo borra), como Photos al importarla. nil
	// (este equipo no tiene con qué) o si falla: la HEIC se queda en Drive y se dice.
	HEIC func(ctx context.Context, ruta string) (jpeg string, limpiar func(), err error)
}

// Video es lo que se sabe de un vídeo al llevarlo a Photos; lo que no se sabe va a cero.
type Video struct {
	Poster      []byte
	Duracion    time.Duration
	Ancho, Alto int
}

// Parte es cómo ha ido un producto.
type Parte struct {
	Producto string
	// Importados entraron en su almacén; Copiados se dejaron en la carpeta de lo traído;
	// Fallidos no entraron (y también se dejaron allí, tal cual); Repetidos ya estaban de
	// otra vez que se trajo, igual o de una copia más vieja, y no se repiten.
	Importados, Copiados, Fallidos, Repetidos int
	// De lo que ya se trajo otra vez (repetir.go): Actualizados habían cambiado en Sytue
	// y, como aquí no se habían tocado, se han puesto al día (un documento, como versión
	// nueva); Conservados habían cambiado en Sytue y también aquí: lo de aquí no se toca
	// y lo de Sytue queda aparte (un documento, nuevo, «(de Sytue, fecha)»; un evento o un
	// contacto, en su fichero en la carpeta de lo traído); Borrados, la persona los quitó
	// de aquí y no vuelven.
	Actualizados, Conservados, Borrados int
	// YaEnDrive son los ficheros que ya se dejaron en Drive en otra vuelta y siguen allí
	// (donde se dejaron o donde la persona los movió): no se copian otra vez
	// (memoria.go). Los de `drive/files` y lo que se deja tal cual.
	YaEnDrive int
	// SinAbrir son, en Photos, las fotos y los vídeos que no se saben abrir en este
	// equipo (un formato que no lee, una HEIC sin con qué convertirla): se quedan en
	// Drive, en la carpeta de lo traído. SinConversor son, de ellas, las HEIC que no
	// han entrado porque este equipo no tiene con qué convertirlas.
	SinAbrir, SinConversor int
}

// Informe es cómo ha ido todo.
type Informe struct {
	// Carpeta es la de Drive donde se ha dejado lo que no tiene almacén.
	Carpeta string
	Partes  []Parte
}

// Total es lo que entró en algún almacén.
func (i Informe) Total() (importados, copiados, fallidos int) {
	for _, p := range i.Partes {
		importados += p.Importados
		copiados += p.Copiados
		fallidos += p.Fallidos
	}
	return importados, copiados, fallidos
}

// SinAbrir son las fotos y los vídeos que se han quedado en Drive porque Photos no los
// sabe abrir en este equipo, y cuántas de ellas son HEIC sin conversor.
func (i Informe) SinAbrir() (sinAbrir, sinConversor int) {
	for _, p := range i.Partes {
		sinAbrir += p.SinAbrir
		sinConversor += p.SinConversor
	}
	return sinAbrir, sinConversor
}

// YaEstaban son las cosas que ya estaban de otra vuelta y no se han repetido
// (repetidas) y los ficheros que ya se dejaron en Drive en otra y siguen allí (enDrive).
func (i Informe) YaEstaban() (repetidas, enDrive int) {
	for _, p := range i.Partes {
		repetidas += p.Repetidos
		enDrive += p.YaEnDrive
	}
	return repetidas, enDrive
}

// Cambios son, de lo que ya se trajo otra vez, lo que se ha puesto al día, lo que había
// cambiado también aquí y se ha dejado como estaba, y lo que la persona quitó y no vuelve.
func (i Informe) Cambios() (actualizados, conservados, borrados int) {
	for _, p := range i.Partes {
		actualizados += p.Actualizados
		conservados += p.Conservados
		borrados += p.Borrados
	}
	return actualizados, conservados, borrados
}

// repartidor lleva la cuenta mientras se trae.
type repartidor struct {
	ctx context.Context
	d   Destino
	// raizDrive es la carpeta de Drive y carpeta, la de lo traído en esta vuelta, dentro.
	raizDrive, carpeta string
	vistos             map[string]int
	partes             map[string]*Parte
	// copia es la fecha de la copia (AAAA-MM-DD, de su carpeta raíz; vacía si no la
	// lleva) y hoy, la de esta vuelta.
	copia, hoy string
	// mem es lo que se trajo en otras vueltas (memoria.go); enDrive, huellas y
	// reclamados, lo que se sabe de Drive al buscar lo que se movió (una vez por vuelta).
	mem        *memoria
	enDrive    map[int64][]string
	huellas    map[string]string
	reclamados map[string]bool
}

func (r *repartidor) parte(p string) *Parte {
	if r.partes[p] == nil {
		r.partes[p] = &Parte{Producto: p}
	}
	return r.partes[p]
}

// Traer reparte el ZIP de `ruta` en los almacenes de d. Lo que no entra en ninguno se
// copia a `<Datos>/Drive/Traído de Sytue AAAA-MM-DD/`. Quien llama se asegura antes de
// que las apps de [Escriben] están cerradas.
func Traer(ctx context.Context, ruta string, d Destino) (_ Informe, errFinal error) {
	z, c, es, err := abrir(ruta)
	if err != nil {
		return Informe{}, err
	}
	defer z.Close()
	ahora := time.Now
	if d.Ahora != nil {
		ahora = d.Ahora
	}
	drive := filepath.Join(d.Datos, "Drive")
	r := &repartidor{
		ctx: ctx, d: d, vistos: map[string]int{}, partes: map[string]*Parte{}, raizDrive: drive,
		carpeta: filepath.Join(drive, "Traído de Sytue "+ahora().Format("2006-01-02")), mem: leerMemoria(drive),
		copia: fechaDeCopia(c.Raiz), hoy: ahora().Format("2006-01-02"),
	}
	// Lo que se ha dejado, apuntado aunque la vuelta se corte o falle a medias.
	defer func() {
		if !r.mem.vacia() {
			if err := r.mem.guardar(drive); err != nil {
				errFinal = errors.Join(errFinal, fmt.Errorf("traer: no se ha apuntado lo traído: %w", err))
			}
		}
	}()

	porProducto := map[string][]entrada{}
	for _, e := range es {
		porProducto[e.producto()] = append(porProducto[e.producto()], e)
	}
	pasos := []struct {
		producto string
		traer    func([]entrada) ([]entrada, error)
	}{
		{"calendar", r.calendario},
		{"contacts", r.contactos},
		{"notes", r.notas},
		{"docs", r.documentos},
		{"mail", r.correo},
		// Photos antes que Drive: se queda con los originales de sus fotos (que vienen en
		// drive/files) y los deja en Drive ella misma, sin los que ya tiene.
		{"photos", func(es []entrada) ([]entrada, error) {
			resto, drive, err := r.fotos(es, porProducto["drive"])
			porProducto["drive"] = drive
			return resto, err
		}},
		{"drive", r.drive},
	}
	var errs []error
	hechos := map[string]bool{}
	for _, p := range pasos {
		hechos[p.producto] = true
		if len(porProducto[p.producto]) == 0 {
			continue
		}
		resto, err := p.traer(porProducto[p.producto])
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.producto, err))
		}
		for _, e := range resto {
			r.dejarTalCual(e, p.producto, &errs)
		}
		if ctx.Err() != nil {
			return r.informe(), ctx.Err()
		}
	}
	// Lo que no tiene almacén, tal cual.
	var otros []string
	for p := range porProducto {
		if !hechos[p] {
			otros = append(otros, p)
		}
	}
	slices.Sort(otros)
	for _, p := range otros {
		nombre := cmp.Or(p, "sytue")
		for _, e := range porProducto[p] {
			r.dejarTalCual(e, nombre, &errs)
		}
	}
	return r.informe(), errors.Join(errs...)
}

// fechaDeCopia es la fecha de una copia por su carpeta raíz
// (`sytue-one[-<org>]-AAAA-MM-DD`): AAAA-MM-DD, o vacía si no la lleva.
func fechaDeCopia(raiz string) string {
	if len(raiz) < len("2006-01-02") {
		return ""
	}
	f := raiz[len(raiz)-len("2006-01-02"):]
	if _, err := time.Parse("2006-01-02", f); err != nil {
		return ""
	}
	return f
}

// dejarTalCual deja e en la carpeta de lo traído, en su ruta del ZIP, y lo cuenta en el
// producto: copiado, ya en Drive de otra vuelta o fallido.
func (r *repartidor) dejarTalCual(e entrada, producto string, errs *[]error) {
	_, ya, err := r.dejar(e, e.ruta, "")
	switch {
	case err != nil:
		*errs = append(*errs, err)
		r.parte(producto).Fallidos++
	case ya:
		r.parte(producto).YaEnDrive++
	default:
		r.parte(producto).Copiados++
	}
}

func (r *repartidor) informe() Informe {
	inf := Informe{Carpeta: r.carpeta}
	for _, p := range r.partes {
		inf.Partes = append(inf.Partes, *p)
	}
	slices.SortFunc(inf.Partes, func(a, b Parte) int { return strings.Compare(a.Producto, b.Producto) })
	if _, err := os.Stat(r.carpeta); err != nil {
		inf.Carpeta = "" // no se ha copiado nada
	}
	return inf
}

// copiar deja e en la carpeta de lo traído, en `rel`, sin pisar nada: un nombre que ya
// está sale como «x (2).ext». Devuelve dónde ha quedado, su huella y su tamaño. Se usa
// por [repartidor.dejar], que no copia lo que ya se dejó en otra vuelta.
func (r *repartidor) copiar(e entrada, rel string) (destino, huella string, n int64, err error) {
	destino = r.unico(filepath.Join(r.carpeta, filepath.FromSlash(rel)))
	if err := os.MkdirAll(filepath.Dir(destino), 0o700); err != nil {
		return "", "", 0, err
	}
	if e.f.UncompressedSize64 > MaxFichero {
		return "", "", 0, fmt.Errorf("traer: %s es demasiado grande", e.ruta)
	}
	rc, err := e.f.Open()
	if err != nil {
		return "", "", 0, err
	}
	defer rc.Close()
	f, err := os.OpenFile(destino, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", "", 0, err
	}
	h := sha256.New()
	n, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(rc, int64(e.f.UncompressedSize64)+1)) //nolint:gosec // tope arriba
	if err == nil && uint64(n) > e.f.UncompressedSize64 {                                       //nolint:gosec // G115: io.Copy no da negativos
		err = fmt.Errorf("traer: %s no mide lo que dice", e.ruta)
	}
	if err = errors.Join(err, f.Close()); err != nil {
		_ = os.Remove(destino)
		return "", "", 0, err
	}
	if !e.f.Modified.IsZero() {
		_ = os.Chtimes(destino, e.f.Modified, e.f.Modified)
	}
	return destino, hex.EncodeToString(h.Sum(nil)), n, nil
}

// unico es ruta o, si ya existe (en disco o en lo que se ha copiado ahora, sin mirar
// mayúsculas: en Windows y macOS son el mismo fichero), «x (2).ext», «x (3).ext» y así.
func (r *repartidor) unico(ruta string) string {
	ext := filepath.Ext(ruta)
	base := strings.TrimSuffix(ruta, ext)
	for n := 1; ; n++ {
		c := ruta
		if n > 1 {
			c = fmt.Sprintf("%s (%d)%s", base, n, ext)
		}
		clave := strings.ToLower(c)
		if _, err := os.Lstat(c); err != nil && r.vistos[clave] == 0 {
			r.vistos[clave] = 1
			return c
		}
	}
}

// abrirAlmacen abre el almacén en claro de producto, como `local.App.Almacen`.
func (r *repartidor) abrirAlmacen(producto string) (*almacen.Recursos, error) {
	return almacen.Abrir(r.ctx, filepath.Join(r.d.Datos, "Datos", producto), producto, r.d.Sujeto)
}

// ----- Notepads -----

// cuadernoDeLaCopia es un cuaderno de la copia: su HTML nativo y lo que dice su volcado
// de quién es y de dónde cuelga.
type cuadernoDeLaCopia struct {
	e         entrada
	nombre    string
	id, padre string
	notas     int
	cifrado   bool
}

// notas trae a Notepads cada cuaderno de la copia con `notes.TraerCuaderno`. La fuente es
// el HTML nativo de cada uno (`notes/notebooks/<título>.html`), que lleva dentro el
// volcado íntegro —notas con sus fechas, tareas, etiquetas y adjuntos— sea cual sea el
// formato que se eligió para las tablas (`notes/notebooks` y `notes/notes`, que en CSV o
// XML no se pueden volver a leer sin pérdida). Un cuaderno entra después del que lo
// contiene, colgado de él, y lo que ya se trajo otra vez no se repite (por el origen de
// cada cuaderno y cada nota). Un cuaderno cifrado no se puede leer aquí: no entra.
//
// Lo que no entra entero se deja tal cual en la carpeta de lo traído: el HTML de ese
// cuaderno y, si algo se quedó fuera, también las tablas.
func (r *repartidor) notas(es []entrada) ([]entrada, error) {
	var cuadernos []cuadernoDeLaCopia
	copiar := map[string]bool{} // las rutas que se dejan tal cual
	for _, e := range es {
		if !strings.HasPrefix(e.ruta, "notes/notebooks/") || !strings.EqualFold(path.Ext(e.ruta), ".html") {
			if !esTablaDeNotas(e.ruta) {
				copiar[e.ruta] = true
			}
			continue
		}
		if c, ok := leerCabecera(e); ok {
			cuadernos = append(cuadernos, c)
		} else {
			copiar[e.ruta] = true
		}
	}
	var err error
	if len(cuadernos) > 0 {
		if err = r.traerCuadernos(cuadernos, r.parte("notes"), copiar); err != nil {
			for _, c := range cuadernos {
				copiar[c.e.ruta] = true
			}
		}
	}
	perdidas := len(cuadernos) == 0
	for _, c := range cuadernos {
		perdidas = perdidas || copiar[c.e.ruta]
	}
	var resto []entrada
	for _, e := range es {
		if copiar[e.ruta] || perdidas && esTablaDeNotas(e.ruta) {
			resto = append(resto, e)
		}
	}
	return resto, err
}

// esTablaDeNotas dice si ruta es `notes/notebooks.<formato>` o `notes/notes.<formato>`.
func esTablaDeNotas(ruta string) bool {
	base := strings.TrimSuffix(ruta, path.Ext(ruta))
	return base == "notes/notebooks" || base == "notes/notes"
}

// leerCabecera saca del HTML de un cuaderno su volcado y, de él, quién es, de quién
// cuelga, cuántas notas trae y si está cifrado.
func leerCabecera(e entrada) (cuadernoDeLaCopia, bool) {
	datos, err := e.leer()
	if err != nil {
		return cuadernoDeLaCopia{}, false
	}
	volcado, ok := model.VolcadoDeHTML(datos)
	if !ok {
		return cuadernoDeLaCopia{}, false
	}
	var cab struct {
		Notebook struct {
			ID        string  `json:"id"`
			ParentID  *string `json:"parent_id"`
			Encrypted bool    `json:"encrypted"`
		} `json:"notebook"`
		Notas []json.RawMessage `json:"notas"`
	}
	if json.Unmarshal(volcado, &cab) != nil {
		return cuadernoDeLaCopia{}, false
	}
	c := cuadernoDeLaCopia{e: e, nombre: path.Base(e.ruta), id: cab.Notebook.ID, notas: len(cab.Notas), cifrado: cab.Notebook.Encrypted}
	if cab.Notebook.ParentID != nil {
		c.padre = *cab.Notebook.ParentID
	}
	return c, true
}

// enOrden pone cada cuaderno después del que lo contiene, si viene en la copia.
func enOrden(cs []cuadernoDeLaCopia) []cuadernoDeLaCopia {
	enCopia := map[string]bool{}
	for _, c := range cs {
		enCopia[c.id] = true
	}
	hecho := make([]bool, len(cs))
	puesto := map[string]bool{}
	out := make([]cuadernoDeLaCopia, 0, len(cs))
	for len(out) < len(cs) {
		antes := len(out)
		for i, c := range cs {
			if !hecho[i] && (c.padre == "" || !enCopia[c.padre] || puesto[c.padre]) {
				hecho[i], puesto[c.id] = true, true
				out = append(out, c)
			}
		}
		if len(out) == antes { // un ciclo: lo que queda, a la raíz
			for i, c := range cs {
				if !hecho[i] {
					c.padre = ""
					hecho[i], puesto[c.id] = true, true
					out = append(out, c)
				}
			}
		}
	}
	return out
}

// traerCuadernos abre Notepads y trae los cuadernos en orden. Marca en copiar el HTML de
// los que no han entrado enteros.
func (r *repartidor) traerCuadernos(cs []cuadernoDeLaCopia, p *Parte, copiar map[string]bool) error {
	rec, err := r.abrirAlmacen("notes")
	if err != nil {
		return err
	}
	defer rec.Cerrar()
	// NewLocal: el almacén de Notepads de Sytue Office puede tener notas de imagen con
	// la imagen dentro y enlaces a ficheros (N-06, N-07), que New no lee.
	n, err := notes.NewLocal(rec.Config)
	if err != nil {
		return err
	}
	defer n.Close()
	if err := almacen.Preparar(r.ctx, n); err != nil {
		return err
	}
	traidos, err := n.LoTraido(r.ctx)
	if err != nil {
		return err
	}
	for _, c := range enOrden(cs) {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		fallo := func() {
			p.Fallidos += c.notas
			copiar[c.e.ruta] = true
		}
		if c.cifrado {
			fallo() // la clave no está aquí
			continue
		}
		datos, err := c.e.leer()
		if err != nil {
			fallo()
			continue
		}
		volcado, _ := model.VolcadoDeHTML(datos)
		padre, _ := traidos.Cuaderno(c.padre)
		res, err := n.TraerCuaderno(r.ctx, traidos, volcado, c.nombre, padre)
		if err != nil {
			fallo()
			continue
		}
		p.Importados += res.Importadas
		p.Repetidos += res.Repetidas
		p.Fallidos += res.Descartadas
		if res.Descartadas > 0 {
			copiar[c.e.ruta] = true
		}
	}
	return nil
}

// ----- Drive -----

// drive deja los ficheros de `drive/files/` en la carpeta de lo traído, con su árbol,
// en `Drive/`: Drive los enseña como cualquier otro fichero suyo. Lo que ya se dejó en
// otra vuelta y sigue en Drive no se copia otra vez (memoria.go).
func (r *repartidor) drive(es []entrada) ([]entrada, error) {
	var resto []entrada
	p := r.parte("drive")
	var errs []error
	for _, e := range es {
		rel, ok := strings.CutPrefix(e.ruta, "drive/files/")
		if !ok {
			resto = append(resto, e)
			continue
		}
		_, ya, err := r.dejar(e, path.Join("Drive", rel), "")
		switch {
		case err != nil:
			p.Fallidos++
			errs = append(errs, err)
		case ya:
			p.YaEnDrive++
		default:
			p.Importados++
		}
		if r.ctx.Err() != nil {
			return resto, r.ctx.Err()
		}
	}
	return resto, errors.Join(errs...)
}
