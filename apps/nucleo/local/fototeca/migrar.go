// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

// # Migrar del almacén de registros
//
// La fototeca vivía en el almacén en claro de la misma carpeta (`registros.jsonl`,
// `objetos/`, `diario/`…: `local/almacen`). La primera vez que se abre la de SQLite con
// ese almacén al lado, se migra, y así:
//
//  1. Solo un proceso migra (el cerrojo `fototeca.migrando`; quien llega después espera
//     y encuentra la migración hecha), y solo si NADIE tiene el almacén abierto (su
//     cerrojo exclusivo: ni Photos de antes, ni el «Traer» del Hub). Si alguien lo tiene,
//     ErrEnUso y no se toca nada.
//  2. Se abre el almacén con el repositorio de la conectada, que lo valida entero como
//     siempre, se terminan las importaciones a medias y se vuelca todo (Volcar): cada foto
//     con su original y sus derivadas, sus recetas, la papelera, lo borrado para siempre
//     (su huella) y los álbumes con sus miembros, también los borrados.
//  3. Todo entra en UNA transacción, y antes de confirmarla se comprueba: los recuentos
//     de lo volcado contra lo escrito, que esté en el disco cada fichero que se cita (el
//     original copiado, las derivadas de la receta actual y de la vacía), y lo que ve la
//     pantalla en las dos (las fotos de la galería, la papelera, las huellas conocidas,
//     los álbumes con su número de fotos y su portada). Si algo no cuadra, no se confirma
//     y el almacén sigue como estaba.
//  4. Confirmada, el diario del almacén se renombra (`registros-antes-de-sqlite.jsonl`)
//     y se conserva con lo demás (`diario/`, `origen.json`): NUNCA se borra solo. Los
//     ficheros de `objetos/` no se copian: son los mismos, y lo que se borre para siempre
//     desde entonces deja de estar también para él.
//
// Un corte antes de confirmar deja todo como estaba y se migra entero otra vez; uno
// entre confirmar y renombrar, lo renombra la próxima apertura.

const (
	// claveMigrada es la fecha de la migración (o «sin almacén»); vacía, no se ha hecho.
	claveMigrada = "migrada"
	// claveMigradaEn es cuándo se migró: barrer no toca lo que es de antes.
	claveMigradaEn = "migrada_en"
	// claveRecuentos son los recuentos de la migración, en JSON.
	claveRecuentos = "migracion"
	// DiarioConservado es como se queda el diario del almacén de registros migrado.
	DiarioConservado = "registros-antes-de-sqlite.jsonl"
	diarioViejo      = "registros.jsonl"
	cerrojoMigrar    = "fototeca.migrando"
)

// ErrEnUso: alguien (otra copia de Photos de antes, el «Traer» del Hub) tiene abierta la
// fototeca de antes; se migrará cuando la cierre.
var ErrEnUso = fmt.Errorf("fototeca: la fototeca de antes está abierta en otra app; ciérrala y vuelve a abrir: %w", almacen.ErrEnUso)

// EsperaMigracion es lo más que se espera a que otro proceso acabe de migrar.
var EsperaMigracion = 10 * time.Minute

// Fases de la migración (Avance.Fase).
const (
	FaseAbrir     = "abrir"     // abriendo y comprobando el almacén de antes
	FaseCopiar    = "copiar"    // pasando las fotos: Hechas de Total
	FaseComprobar = "comprobar" // comparando las dos antes de confirmar
)

// Avance es cómo va la migración.
type Avance struct {
	Fase          string
	Hechas, Total int
}

// Recuentos es lo que pasó a SQLite.
type Recuentos struct {
	Fotos       int `json:"fotos"`
	EnPapelera  int `json:"en_papelera"`
	Purgadas    int `json:"purgadas"`
	Derivadas   int `json:"derivadas"`
	Ediciones   int `json:"ediciones"`
	Albumes     int `json:"albumes"`
	Borrados    int `json:"albumes_borrados"`
	Miembros    int `json:"miembros"`
	SinFicheros int `json:"derivadas_sin_fichero"`
}

// HayQueMigrar dice si en dir hay una fototeca del almacén de registros sin migrar.
func HayQueMigrar(dir string) bool {
	if !almacen.EsAlmacen(dir) {
		return false
	}
	ruta := filepath.Join(dir, Fichero)
	if _, err := os.Stat(ruta); err != nil {
		return true
	}
	f, err := LeerSinAbrir(dir)
	return err != nil || !f.Migrada
}

func (f *Fototeca) migrarSiHaceFalta(ctx context.Context, avance func(Avance)) error {
	if hecha, err := meta(ctx, f.db, claveMigrada); err != nil || hecha != "" {
		if err != nil {
			return err
		}
		return f.conservarDiario()
	}
	if !almacen.EsAlmacen(f.dir) {
		return f.tx(ctx, func(tx *sql.Tx) error { return ponerMeta(ctx, tx, claveMigrada, "sin almacén") })
	}
	soltar, err := f.cerrojoDeMigrar(ctx)
	if err != nil {
		return err
	}
	defer soltar()
	// Quien tenía el cerrojo puede haberla hecho ya.
	if hecha, err := meta(ctx, f.db, claveMigrada); err != nil || hecha != "" {
		if err != nil {
			return err
		}
		return f.conservarDiario()
	}
	if !almacen.EsAlmacen(f.dir) {
		return f.tx(ctx, func(tx *sql.Tx) error { return ponerMeta(ctx, tx, claveMigrada, "sin almacén") })
	}
	if avance == nil {
		avance = func(Avance) {}
	}
	inicio := time.Now()
	r, err := f.migrar(ctx, avance)
	if err != nil {
		return fmt.Errorf("fototeca: no se pudo pasar la fototeca a SQLite (sigue como estaba): %w", err)
	}
	slog.Info("fototeca: migrada a SQLite", "fotos", r.Fotos, "papelera", r.EnPapelera, "purgadas", r.Purgadas, "albumes", r.Albumes,
		"miembros", r.Miembros, "derivadas", r.Derivadas, "ediciones", r.Ediciones, "tarda", time.Since(inicio).Round(time.Millisecond))
	return f.conservarDiario()
}

// cerrojoDeMigrar toma el cerrojo de la migración, esperando a quien la esté haciendo.
func (f *Fototeca) cerrojoDeMigrar(ctx context.Context) (func(), error) {
	fh, err := os.OpenFile(filepath.Join(f.dir, cerrojoMigrar), os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G304: la carpeta de la fototeca
	if err != nil {
		return nil, err
	}
	c, fin := context.WithTimeout(ctx, EsperaMigracion)
	defer fin()
	soltar, err := ajustes.CerrojoFichero(c, fh)
	if err != nil {
		_ = fh.Close()
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrEnUso
		}
		// Donde el sistema no da cerrojos se sigue: el exclusivo del almacén lo dirá.
		slog.Debug("fototeca: sin cerrojo del sistema para migrar", "error", err)
		return func() {}, nil
	}
	return func() { soltar(); _ = fh.Close() }, nil
}

// conservarDiario deja el diario del almacén migrado con su nombre de conservado (si
// sigue con el de antes), sin pisar uno que ya esté.
func (f *Fototeca) conservarDiario() error {
	viejo := filepath.Join(f.dir, diarioViejo)
	if _, err := os.Stat(viejo); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	destino := filepath.Join(f.dir, DiarioConservado)
	for i := 2; ; i++ {
		if _, err := os.Lstat(destino); errors.Is(err, fs.ErrNotExist) {
			break
		}
		destino = filepath.Join(f.dir, "registros-antes-de-sqlite-"+strconv.Itoa(i)+".jsonl")
	}
	if err := os.Rename(viejo, destino); err != nil {
		return fmt.Errorf("fototeca: no se pudo apartar el diario de antes: %w", err)
	}
	return nil
}

// migrar hace la migración entera (ver arriba) y la confirma.
func (f *Fototeca) migrar(ctx context.Context, avance func(Avance)) (Recuentos, error) {
	// Nadie más puede tener el almacén abierto mientras tanto.
	soltar, err := almacen.CerrojoExclusivo(f.dir)
	if err != nil {
		return Recuentos{}, ErrEnUso
	}
	soltar()
	avance(Avance{Fase: FaseAbrir})
	rec, err := almacen.Abrir(ctx, f.dir, Producto, f.scope.Subject)
	if err != nil {
		return Recuentos{}, err
	}
	defer rec.Cerrar()                  //nolint:contextcheck // cerrar el almacén no depende de ctx: se cierra también si ya se canceló
	repo, err := photos.New(rec.Config) //nolint:contextcheck // el motor lleva su contexto en la configuración (rec.Config)
	if err != nil {
		return Recuentos{}, err
	}
	defer repo.Close() //nolint:contextcheck // cerrar no depende de ctx
	repo.AdmitirFormatos(slices.Collect(maps.Keys(f.extra))...)
	repo.LiberarAlPurgar()
	if err := almacen.Preparar(ctx, repo); err != nil {
		return Recuentos{}, err
	}
	if hechas, err := repo.ReanudarImportaciones(ctx); err != nil {
		// Lo que no se pudo terminar sigue en el diario del almacén, que se conserva.
		slog.Warn("fototeca: importaciones a medias que no se pudieron terminar antes de migrar", "terminadas", len(hechas), "error", err)
	}
	var r Recuentos
	err = f.tx(ctx, func(tx *sql.Tx) error {
		var err error
		if r, err = f.volcar(ctx, tx, repo, avance); err != nil {
			return err
		}
		avance(Avance{Fase: FaseComprobar})
		if err := comprobarRecuentos(ctx, tx, r); err != nil {
			return err
		}
		if err := f.comprobarComoSeVe(ctx, tx, repo); err != nil {
			return err
		}
		raw, _ := json.Marshal(r)
		ahora := enMarca(f.ahora())
		for clave, valor := range map[string]string{claveMigrada: ahora, claveMigradaEn: ahora, claveRecuentos: string(raw)} {
			if err := ponerMeta(ctx, tx, clave, valor); err != nil {
				return err
			}
		}
		return nil
	})
	return r, err
}

// volcado es el paso de las filas de repo a tx.
type volcado struct {
	f      *Fototeca
	ctx    context.Context //nolint:containedctx // vive lo que la transacción que lo usa
	tx     *sql.Tx
	avance func(Avance)
	paso   time.Time
	vacia  string
	// revisiones es la última revisión de cada foto vista (un conflicto deja dos).
	revisiones map[string]time.Time
	// fotos son las no purgadas.
	fotos map[string]bool
}

// volcar pasa a tx todo lo que guarda repo.
func (f *Fototeca) volcar(ctx context.Context, tx *sql.Tx, repo *photos.Repository, avance func(Avance)) (Recuentos, error) {
	v := &volcado{f: f, ctx: ctx, tx: tx, avance: avance, paso: time.Now(), vacia: recetaVacia(), revisiones: map[string]time.Time{}, fotos: map[string]bool{}}
	var r Recuentos
	if err := repo.Volcar(ctx, v.foto, v.album); err != nil {
		return r, err
	}
	// Los recuentos, de lo que quedó (sin las revisiones viejas de un conflicto).
	for consulta, n := range map[string]*int{
		`SELECT COUNT(*) FROM fotos WHERE borrada IS NULL`:                         &r.Fotos,
		`SELECT COUNT(*) FROM fotos WHERE borrada IS NOT NULL AND purgada IS NULL`: &r.EnPapelera,
		`SELECT COUNT(*) FROM fotos WHERE purgada IS NOT NULL`:                     &r.Purgadas,
		`SELECT COUNT(*) FROM derivadas`:                                           &r.Derivadas,
		`SELECT COUNT(*) FROM ediciones`:                                           &r.Ediciones,
		`SELECT COUNT(*) FROM albumes WHERE borrado IS NULL`:                       &r.Albumes,
		`SELECT COUNT(*) FROM albumes WHERE borrado IS NOT NULL`:                   &r.Borrados,
		`SELECT COUNT(*) FROM album_fotos`:                                         &r.Miembros,
	} {
		if err := tx.QueryRowContext(ctx, consulta).Scan(n); err != nil {
			return r, err
		}
	}
	return r, nil
}

// foto pasa una foto con su receta y sus derivadas.
func (v *volcado) foto(d photos.FotoVolcada, hechas, total int) error {
	if time.Since(v.paso) > 100*time.Millisecond || hechas == total {
		v.paso = time.Now()
		v.avance(Avance{Fase: FaseCopiar, Hechas: hechas, Total: total})
	}
	id := d.Fila.ID
	if antes, ya := v.revisiones[id]; ya {
		if !d.Fila.UpdatedAt.After(antes) {
			return nil
		}
		if err := quitarFotoMigrada(v.ctx, v.tx, id); err != nil {
			return err
		}
	}
	v.revisiones[id] = d.Fila.UpdatedAt
	x := foto{fila: d.Fila, huella: d.Huella, retenido: d.Retenido, ruta: d.Ruta}
	if d.Ruta != "" {
		m := d.Modificado
		x.modificado = &m
	}
	if d.Retenido && d.Fila.PurgedAt == nil {
		if err := v.f.existe(d.Huella); err != nil {
			return fmt.Errorf("el original de %s: %w", d.Fila.Name, err)
		}
	}
	if err := insertarFoto(v.ctx, v.tx, x); err != nil {
		return err
	}
	if d.Fila.PurgedAt == nil {
		v.fotos[id] = true
	}
	actual := v.vacia
	if d.Receta != nil {
		actual = d.HuellaReceta
		raw, err := json.Marshal(*d.Receta)
		if err != nil {
			return err
		}
		if _, err := v.tx.ExecContext(v.ctx, `INSERT INTO ediciones (foto, receta, huella, editada) VALUES (?, ?, ?, ?)`,
			id, string(raw), d.HuellaReceta, enMarca(d.Fila.UpdatedAt)); err != nil {
			return err
		}
	}
	for _, der := range d.Derivadas {
		if err := v.f.existe(der.SHA256); err != nil {
			if der.Receta == actual || der.Receta == v.vacia {
				return fmt.Errorf("una derivada de %s: %w", d.Fila.Name, err)
			}
			continue // la de una receta de antes: no se lee nunca más
		}
		if _, err := v.tx.ExecContext(v.ctx, `INSERT OR IGNORE INTO derivadas (foto, receta, variante, formato, sha, bytes) VALUES (?, ?, ?, ?, ?, ?)`,
			id, der.Receta, string(der.Variante), der.Formato, der.SHA256, der.Bytes); err != nil {
			return err
		}
	}
	return nil
}

// album pasa un álbum con sus miembros (los de fotos no purgadas).
func (v *volcado) album(a photos.AlbumVolcado) error {
	var hay string
	err := v.tx.QueryRowContext(v.ctx, `SELECT editado FROM albumes WHERE id = ?`, a.Fila.ID).Scan(&hay)
	switch {
	case err == nil:
		if antes, _ := deMarca(hay); !a.Fila.UpdatedAt.After(antes) {
			return nil
		}
		for _, q := range []string{`DELETE FROM album_fotos WHERE album = ?`, `DELETE FROM albumes WHERE id = ?`} {
			if _, err := v.tx.ExecContext(v.ctx, q, a.Fila.ID); err != nil {
				return err
			}
		}
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	var portada any
	if a.Fila.CoverAssetID != nil {
		portada = *a.Fila.CoverAssetID
	}
	creado, editado := a.Fila.CreatedAt, a.Fila.UpdatedAt
	if editado.IsZero() {
		editado = creado
	}
	if _, err := v.tx.ExecContext(v.ctx, `INSERT INTO albumes (id, titulo, portada, origen, creado, editado, borrado) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.Fila.ID, a.Fila.Title, portada, a.Fila.Origin, enMarca(creado), enMarca(editado), marcaNula(a.Fila.DeletedAt)); err != nil {
		return err
	}
	for _, m := range a.Miembros {
		if !v.fotos[m.AssetID] {
			continue
		}
		anadida := m.AddedAt
		if anadida.IsZero() {
			anadida = editado
		}
		if _, err := v.tx.ExecContext(v.ctx, `INSERT OR IGNORE INTO album_fotos (album, foto, posicion, anadida) VALUES (?, ?, ?, ?)`,
			a.Fila.ID, m.AssetID, m.Position, enMarca(anadida)); err != nil {
			return err
		}
	}
	return nil
}

// quitarFotoMigrada quita de tx una foto ya volcada (una revisión vieja de un conflicto).
func quitarFotoMigrada(ctx context.Context, tx *sql.Tx, id string) error {
	for _, q := range []string{`DELETE FROM derivadas WHERE foto = ?`, `DELETE FROM ediciones WHERE foto = ?`, `DELETE FROM fotos WHERE id = ?`} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}

// existe dice si está en el disco el fichero de esa huella.
func (f *Fototeca) existe(sha string) error {
	if !esHuella(sha) {
		return fmt.Errorf("%w: huella %q", ErrSource, sha)
	}
	if info, err := os.Stat(f.ruta(sha)); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: falta el fichero %s", ErrSource, sha)
	}
	return nil
}

// comprobarRecuentos compara lo escrito con lo que cuenta tx de cada tabla, otra vez y
// por otro camino (las fotos con sus derivadas y los miembros con su foto).
func comprobarRecuentos(ctx context.Context, tx *sql.Tx, r Recuentos) error {
	var fotos, sinDerivadas, huerfanos int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM fotos`).Scan(&fotos); err != nil {
		return err
	}
	if fotos != r.Fotos+r.EnPapelera+r.Purgadas {
		return fmt.Errorf("%w: %d fotos escritas y %d contadas", ErrSource, fotos, r.Fotos+r.EnPapelera+r.Purgadas)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM fotos f WHERE f.purgada IS NULL AND
		(SELECT COUNT(*) FROM derivadas d WHERE d.foto = f.id AND d.receta = COALESCE((SELECT huella FROM ediciones e WHERE e.foto = f.id), ?)) <> ?`,
		recetaVacia(), len(model.Variantes())).Scan(&sinDerivadas); err != nil {
		return err
	}
	if sinDerivadas > 0 {
		return fmt.Errorf("%w: %d fotos sin todas sus derivadas", ErrSource, sinDerivadas)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM album_fotos m LEFT JOIN fotos f ON f.id = m.foto
		LEFT JOIN albumes a ON a.id = m.album WHERE f.id IS NULL OR f.purgada IS NOT NULL OR a.id IS NULL`).Scan(&huerfanos); err != nil {
		return err
	}
	if huerfanos > 0 {
		return fmt.Errorf("%w: %d miembros de álbum sin su foto o su álbum", ErrSource, huerfanos)
	}
	return nil
}

// comprobarComoSeVe compara lo que ve la pantalla en las dos: las fotos de la galería
// (en orden), la papelera, las huellas conocidas y los álbumes con su número de fotos y
// su portada.
func (f *Fototeca) comprobarComoSeVe(ctx context.Context, tx *sql.Tx, repo *photos.Repository) error {
	if err := f.compararGaleria(ctx, tx, repo); err != nil {
		return err
	}
	if err := compararPapeleraYHuellas(ctx, tx, repo); err != nil {
		return err
	}
	return compararAlbumes(ctx, tx, repo)
}

func (f *Fototeca) compararGaleria(ctx context.Context, tx *sql.Tx, repo *photos.Repository) error {
	var antes []api.Photo
	for cursor := ""; ; {
		pagina, siguiente, err := repo.Timeline(ctx, cursor, 200, false, "")
		if err != nil {
			return err
		}
		antes = append(antes, pagina...)
		if siguiente == "" {
			break
		}
		cursor = siguiente
	}
	ahora, err := f.consultarFotos(ctx, tx, `SELECT `+columnasFoto+` FROM fotos f WHERE f.borrada IS NULL ORDER BY f.fecha DESC, f.id DESC`)
	if err != nil {
		return err
	}
	if len(antes) != len(ahora) {
		return fmt.Errorf("%w: la galería tenía %d fotos y tiene %d", ErrSource, len(antes), len(ahora))
	}
	for i, a := range antes {
		b := ahora[i].vista()
		if a.ID != b.ID || !a.TakenAt.Equal(b.TakenAt) || a.Favorite != b.Favorite || a.Name != b.Name || a.SizeBytes != b.SizeBytes || a.Externo != b.Externo {
			return fmt.Errorf("%w: la foto %d de la galería no es la misma (%s, %s)", ErrSource, i+1, a.ID, b.ID)
		}
	}
	return nil
}

func compararPapeleraYHuellas(ctx context.Context, tx *sql.Tx, repo *photos.Repository) error {
	papelera, err := repo.Papelera(ctx)
	if err != nil {
		return err
	}
	var enPapelera, huellas int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM fotos WHERE borrada IS NOT NULL AND purgada IS NULL`).Scan(&enPapelera); err != nil {
		return err
	}
	if len(papelera) != enPapelera {
		return fmt.Errorf("%w: la papelera tenía %d fotos y tiene %d", ErrSource, len(papelera), enPapelera)
	}
	conocidas, err := repo.HuellasConocidas(ctx)
	if err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(DISTINCT huella) FROM fotos WHERE huella <> ''`).Scan(&huellas); err != nil {
		return err
	}
	if len(conocidas) != huellas {
		return fmt.Errorf("%w: se conocían %d originales y se conocen %d", ErrSource, len(conocidas), huellas)
	}
	return nil
}

func compararAlbumes(ctx context.Context, tx *sql.Tx, repo *photos.Repository) error {
	antes, err := repo.Albums(ctx)
	if err != nil {
		return err
	}
	ahora, err := albumesDe(ctx, tx)
	if err != nil {
		return err
	}
	if len(antes) != len(ahora) {
		return fmt.Errorf("%w: había %d álbumes y hay %d", ErrSource, len(antes), len(ahora))
	}
	porID := map[string]api.Album{}
	for _, a := range ahora {
		porID[a.ID] = a
	}
	for _, a := range antes {
		b, ok := porID[a.ID]
		if !ok || a.Title != b.Title || a.Count != b.Count || a.CoverAssetID != b.CoverAssetID {
			return fmt.Errorf("%w: el álbum %q no es el mismo (%d fotos y %d)", ErrSource, a.Title, a.Count, b.Count)
		}
	}
	return nil
}
