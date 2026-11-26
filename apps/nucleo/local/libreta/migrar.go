// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// # Pasar a la base lo del almacén de antes
//
// Hasta esta base, Contacts de Sytue Office guardaba la libreta en el almacén de
// registros (`registros.jsonl` y compañía, `local/almacen`) de la MISMA carpeta. La
// primera vez que se abre la base con un almacén al lado, se lee entero con el
// repositorio de Contacts (que reanuda lo que hubiera quedado a medias) y se pasa a la
// base en UNA transacción: las filas tal cual (ya compuestas con los favoritos y la
// papelera), lo que se ve de cada cosa y el índice de favoritos. Antes de confirmarla se
// comprueba que la base tiene exactamente lo leído: cuántas filas hay de cada clase,
// cuántas libretas, contactos, etiquetas y favoritos, y la huella de todas las filas. Si
// algo no cuadra no se confirma nada.
//
// Solo entonces se apunta la marca (`meta.migrada`, con lo que se pasó), y desde ahí la
// base es la libreta: nadie vuelve a leer ni a escribir el almacén viejo, que se queda
// donde estaba, sin tocar, y NUNCA se borra solo. Un corte a mitad deja la base sin
// marca y sin nada de lo de antes (la transacción no llegó a confirmarse): la próxima
// vez se repite entera. Una base con marca no se vuelve a migrar.
//
// Quien abre se asegura de que nadie escribe en el almacén viejo mientras tanto (Abrir).
// Si dos procesos migran a la vez, el segundo espera al primero (BEGIN IMMEDIATE) y ve la
// marca.

// Informe es lo que dice la marca de la migración.
type Informe struct {
	Fecha time.Time `json:"fecha"`
	// Origen es de dónde vino: «registros» (el almacén de antes) o «ninguno» (no había).
	Origen    string         `json:"origen"`
	Filas     map[string]int `json:"filas,omitempty"`
	Libretas  int            `json:"libretas"`
	Contactos int            `json:"contactos"`
	Etiquetas int            `json:"etiquetas"`
	Favoritos int            `json:"favoritos"`
	// Huella es la de todas las filas, en orden (huellaDeFilas).
	Huella string `json:"huella,omitempty"`
}

// Migracion es la marca de la migración, si la hay (false si aún no se migró).
func (x *Libreta) Migracion(ctx context.Context) (Informe, bool, error) {
	var valor string
	err := x.leer.QueryRowContext(ctx, `SELECT valor FROM meta WHERE clave = ?`, claveMigrada).Scan(&valor)
	if errors.Is(err, sql.ErrNoRows) {
		return Informe{}, false, nil
	}
	if err != nil {
		return Informe{}, false, err
	}
	var inf Informe
	if err := json.Unmarshal([]byte(valor), &inf); err != nil {
		return Informe{}, true, err
	}
	return inf, true, nil
}

// HayQueMigrar dice si en dir hay un almacén de registros de Contacts que aún no ha
// pasado a la libreta: abrirla lo pasará (migrar), y con muchos contactos tarda.
func HayQueMigrar(dir string) bool {
	if !almacen.EsAlmacen(dir) {
		return false
	}
	ctx, fin := context.WithTimeout(context.Background(), 5*time.Second)
	defer fin()
	x, err := AbrirLectura(ctx, dir, "lector")
	if err != nil {
		return true
	}
	_ = x.Cerrar()
	return false
}

// Fases de la migración (Avance.Fase).
const (
	FaseAbrir     = "abrir"     // leyendo y validando el almacén de antes
	FaseCopiar    = "copiar"    // escribiendo en la base: Hechas de Total
	FaseComprobar = "comprobar" // comparando la base con lo leído antes de confirmar
)

// Avance es cómo va la migración. Hechas y Total cuentan lo que se escribe en la base
// (las filas de la libreta y lo que se ve de cada libreta, contacto y etiqueta).
type Avance struct {
	Fase          string
	Hechas, Total int
}

// deAntes es lo leído del almacén de antes.
type deAntes struct {
	filas     []contacts.Row
	vistas    vistas
	favoritos map[string]bool
}

// pasos es cuánto se escribe en la base al pasar lo de antes (Avance.Total).
func (a deAntes) pasos() int {
	return len(a.filas) + len(a.vistas.libros) + len(a.vistas.contactos) + len(a.vistas.grupos) + len(a.favoritos)
}

// migrar pasa a la base lo del almacén de registros de dir, si aún no se hizo, diciendo
// a avance (si no es nil) cómo va.
func (x *Libreta) migrar(ctx context.Context, dir string, avance func(Avance)) error {
	if ok, err := x.migrada(ctx, x.db); err != nil || ok {
		return err
	}
	if avance == nil {
		avance = func(Avance) {}
	}
	inf := Informe{Origen: "ninguno"}
	var antes deAntes
	if almacen.EsAlmacen(dir) {
		var hay bool
		var err error
		avance(Avance{Fase: FaseAbrir})
		if antes, hay, err = leerAlmacen(ctx, dir, x.scope.Subject); err != nil {
			return fmt.Errorf("libreta: leer el almacén de antes: %w", err)
		}
		if hay {
			inf.Origen = "registros"
		}
	}
	inicio := time.Now()
	hecho := false
	err := x.escribir(ctx, func(tx *sql.Tx) error {
		if ok, err := x.migrada(ctx, tx); err != nil || ok {
			return err // otro proceso acaba de migrar
		}
		var filas int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM fila`).Scan(&filas); err != nil {
			return err
		}
		if filas > 0 {
			// Sin marca y con filas no debería pasar nunca (la migración es una sola
			// transacción): no se mezcla nada.
			return errors.New("libreta: la base tiene contactos sin la marca de la migración")
		}
		if err := ponerTodo(ctx, tx, antes, avance); err != nil {
			return err
		}
		inf.Fecha = x.ahora().UTC()
		if antes.pasos() > 0 {
			avance(Avance{Fase: FaseComprobar})
		}
		if err := comprobar(ctx, tx, antes, &inf); err != nil {
			return err
		}
		valor, err := json.Marshal(inf)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO meta (clave, valor) VALUES (?, ?)`, claveMigrada, string(valor)); err != nil {
			return err
		}
		hecho = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("libreta: pasar los contactos a la base: %w", err)
	}
	if hecho && inf.Origen == "registros" {
		slog.Info("libreta: contactos pasados del almacén de registros a SQLite", "contactos", inf.Contactos,
			"libretas", inf.Libretas, "etiquetas", inf.Etiquetas, "filas", len(antes.filas), "duracion", time.Since(inicio).Round(time.Millisecond))
	}
	return nil
}

// leerAlmacen lee entero el almacén de registros de Contacts de dir con su repositorio.
// hay es falso si el almacén nunca llegó a prepararse (no tiene contactos).
func leerAlmacen(ctx context.Context, dir, sujeto string) (deAntes, bool, error) {
	rec, err := almacen.Abrir(ctx, dir, "contacts", sujeto)
	if err != nil {
		return deAntes{}, false, err
	}
	defer rec.Cerrar() //nolint:contextcheck // cerrar no se cancela; y sin compactar: el almacén se queda como estaba
	repo, err := contacts.New(rec.Config)
	if err != nil {
		return deAntes{}, false, err
	}
	defer repo.Close() //nolint:contextcheck // cerrar no se cancela
	if err := repo.Initialize(ctx); err != nil {
		if errors.Is(err, records.ErrNotReady) {
			return deAntes{}, false, nil
		}
		return deAntes{}, false, err
	}
	foto, err := repo.Snapshot(ctx)
	if err != nil {
		return deAntes{}, false, err
	}
	favoritos, err := repo.IndiceDeFavoritos(ctx)
	if err != nil {
		return deAntes{}, false, err
	}
	v, err := proyectar(foto)
	if err != nil {
		return deAntes{}, false, err
	}
	return deAntes{filas: foto.Rows(), vistas: v, favoritos: favoritos.Values}, true, nil
}

// ponerTodo escribe en tx todo lo de antes: filas, vistas y favoritos, diciendo a avance
// cómo va (como mucho cada 100 ms, y al acabar).
func ponerTodo(ctx context.Context, tx *sql.Tx, antes deAntes, avance func(Avance)) error {
	p := &preparadas{tx: tx, sentencias: map[string]*sql.Stmt{}}
	defer p.cerrar()
	total, hechas, paso := antes.pasos(), 0, time.Time{}
	uno := func() {
		hechas++
		if hechas == total || time.Since(paso) > 100*time.Millisecond {
			paso = time.Now()
			avance(Avance{Fase: FaseCopiar, Hechas: hechas, Total: total})
		}
	}
	if total > 0 {
		avance(Avance{Fase: FaseCopiar, Total: total})
		paso = time.Now()
	}
	for _, r := range antes.filas {
		if err := ponerFila(ctx, p, r); err != nil {
			return err
		}
		uno()
	}
	for _, b := range antes.vistas.libros {
		if err := ponerLibreta(ctx, p, b); err != nil {
			return err
		}
		uno()
	}
	for i := range antes.vistas.contactos {
		if err := ponerContacto(ctx, p, &antes.vistas.contactos[i]); err != nil {
			return err
		}
		uno()
	}
	for i := range antes.vistas.grupos {
		if err := ponerGrupo(ctx, p, &antes.vistas.grupos[i]); err != nil {
			return err
		}
		uno()
	}
	for uid, valor := range antes.favoritos {
		if _, err := p.ExecContext(ctx, `INSERT INTO favorito (uid, valor) VALUES (?, ?)`, uid, valor); err != nil {
			return err
		}
		uno()
	}
	return nil
}

// fallarAlComprobar, si no es nil, hace fallar la comprobación: las pruebas cortan así
// la migración a mitad.
var fallarAlComprobar func() error

// comprobar mira que la base tiene exactamente lo de antes y lo apunta en inf.
func comprobar(ctx context.Context, tx *sql.Tx, antes deAntes, inf *Informe) error {
	if fallarAlComprobar != nil {
		if err := fallarAlComprobar(); err != nil {
			return err
		}
	}
	quiero := map[string]int{}
	for _, r := range antes.filas {
		quiero[r.Kind]++
	}
	hay, err := filasPorClase(ctx, tx)
	if err != nil {
		return err
	}
	if len(hay) != len(quiero) {
		return fmt.Errorf("libreta: la base tiene %v filas y el almacén %v", hay, quiero)
	}
	for clase, n := range quiero {
		if hay[clase] != n {
			return fmt.Errorf("libreta: la base tiene %d filas %q y el almacén %d", hay[clase], clase, n)
		}
	}
	for _, c := range []struct {
		tabla string
		n     int
	}{{"libreta", len(antes.vistas.libros)}, {"contacto", len(antes.vistas.contactos)}, {"grupo", len(antes.vistas.grupos)}, {"favorito", len(antes.favoritos)}} {
		var n int
		//nolint:gosec // G202: tabla es una de las cuatro de arriba
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+c.tabla).Scan(&n); err != nil {
			return err
		}
		if n != c.n {
			return fmt.Errorf("libreta: la base tiene %d en %s y el almacén %d", n, c.tabla, c.n)
		}
	}
	p := nuevaParte(ctx, tx)
	filas, err := p.leerFilas(`SELECT clase, id, json FROM fila ORDER BY clase, id`)
	if err != nil {
		return err
	}
	huella := huellaDeFilas(antes.filas)
	if huellaDeFilas(filas) != huella {
		return errors.New("libreta: lo escrito en la base no es lo leído del almacén")
	}
	inf.Filas, inf.Huella = quiero, huella
	inf.Libretas, inf.Contactos, inf.Etiquetas, inf.Favoritos = len(antes.vistas.libros), len(antes.vistas.contactos), len(antes.vistas.grupos), len(antes.favoritos)
	return nil
}

// filasPorClase cuenta las filas de la base de cada clase.
func filasPorClase(ctx context.Context, tx *sql.Tx) (map[string]int, error) {
	rs, err := tx.QueryContext(ctx, `SELECT clase, COUNT(*) FROM fila GROUP BY clase`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rs.Close() }()
	hay := map[string]int{}
	for rs.Next() {
		var clase string
		var n int
		if err := rs.Scan(&clase, &n); err != nil {
			return nil, err
		}
		hay[clase] = n
	}
	return hay, rs.Err()
}

// huellaDeFilas es la huella de unas filas en el orden de contacts.Snapshot.Rows (clase
// e id): la de cada una, con su clase, su id y su JSON, una detrás de otra.
func huellaDeFilas(filas []contacts.Row) string {
	h := sha256.New()
	for _, r := range filas {
		_, _ = fmt.Fprintf(h, "%d:%s%d:%s%d:", len(r.Kind), r.Kind, len(r.ID), r.ID, len(r.JSON))
		_, _ = h.Write(r.JSON)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// preparadas es una transacción que prepara cada sentencia una sola vez: la migración
// escribe decenas de miles de filas con las mismas cuatro.
type preparadas struct {
	tx         *sql.Tx
	sentencias map[string]*sql.Stmt
}

func (p *preparadas) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	st, ok := p.sentencias[query]
	if !ok {
		var err error
		if st, err = p.tx.PrepareContext(ctx, query); err != nil {
			return nil, err
		}
		p.sentencias[query] = st
	}
	return st.ExecContext(ctx, args...)
}

func (p *preparadas) cerrar() {
	for _, st := range p.sentencias {
		_ = st.Close()
	}
}
