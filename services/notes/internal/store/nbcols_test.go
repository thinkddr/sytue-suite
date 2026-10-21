package store

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Las dos listas describen las MISMAS columnas y en el mismo orden: `scanNotebook`
// lee posicionalmente, así que una divergencia no da error de compilación sino un
// campo con el valor de otro.
func TestColumnasConYSinAlias(t *testing.T) {
	if nbCols != "nb.id, nb.owner_sub, nb.parent_id, nb.title, nb.emoji, nb.color, nb.layout, "+
		"nb.visibility, nb.indexable, nb.encrypted, nb.crypto, nb.room_id, nb.public_token, "+
		"nb.position, nb.created_at, nb.updated_at, nb.daily_date, nb.archived_at" {
		t.Fatalf("nbCols = %q", nbCols)
	}
	// Y la base NO puede llevar alias: va en `INSERT … RETURNING`, donde no hay
	// tabla que aliasar y Postgres responde «missing FROM-clause entry for table
	// "nb"». Es el fallo que impedía crear y actualizar cuadernos.
	for _, c := range []byte(nbColsBase) {
		if c == '.' {
			t.Fatalf("nbColsBase lleva un alias y se usa en RETURNING: %q", nbColsBase)
		}
	}
	// LO MISMO PARA LAS NOTAS. Al arreglar los cuadernos no se miró el fichero
	// hermano y el rojo volvió con otro nombre de tabla: crear una nota fallaba
	// igual, contra `n` en vez de contra `nb`.
	if !strings.HasPrefix(noteCols, "n.id, ") {
		t.Fatalf("noteCols = %q", noteCols)
	}
	if strings.Contains(noteColsBase, ".") {
		t.Fatalf("noteColsBase lleva un alias y se usa en RETURNING: %q", noteColsBase)
	}
	if strings.Count(nbCols, ",") != strings.Count(nbColsBase, ",") ||
		strings.Count(noteCols, ",") != strings.Count(noteColsBase, ",") {
		t.Fatal("una lista tiene más columnas que su pareja: scanX lee por posición")
	}
}

// EL ALIAS EN `RETURNING` NO ES UNA CUESTIÓN DE GUSTO: lo decide la sentencia.
//
//   - En `INSERT … RETURNING` no hay tabla que aliasar: con `n.id` Postgres
//     responde «missing FROM-clause entry for table "n"». Fue el rojo del
//     2026-09-07, dos veces seguidas (cuadernos y luego notas).
//   - En `UPDATE notes n … FROM notebooks nb … RETURNING`, RETURNING ve LAS DOS
//     tablas, y `id`, `title`, `owner_sub`, `visibility`, `created_at` y
//     `updated_at` existen en ambas: sin alias es «column reference "id" is
//     ambiguous» (42702). Esta prueba exigía la lista sin alias en todos los
//     casos, y con eso EDITAR UNA NOTA fallaba en producción desde ese mismo
//     día; lo destapó el test de integración del anidamiento, que fue el primero
//     en llamar a UpdateNote contra un Postgres de verdad.
//
// Se comprueba sobre el CÓDIGO y no sobre las dos constantes que hoy conocemos:
// una tercera tabla repetiría cualquiera de los dos fallos sin que este test
// dijera nada. Un UPDATE sin FROM puede usar cualquiera de las dos listas; se
// deja en paz.
func TestElAliasEnReturningLoDecideLaSentencia(t *testing.T) {
	re := regexp.MustCompile(`RETURNING ` + "`" + `\+([a-zA-Z]+)`)
	// Un FROM a nivel de sentencia empieza línea; el de una subconsulta en línea
	// —`(SELECT … FROM notebooks …)`— no cuenta, porque no entra en RETURNING.
	fromDeSentencia := regexp.MustCompile(`(?m)^\s*FROM\s+\w+`)
	for _, f := range []string{"notebooks.go", "notes.go"} {
		datos, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("leer %s: %v", f, err)
		}
		src := string(datos)
		for _, m := range re.FindAllStringSubmatchIndex(src, -1) {
			lista := src[m[2]:m[3]]
			// La sentencia es lo que va desde la última apertura de literal hasta
			// este RETURNING.
			inicio := strings.LastIndex(src[:m[0]], "`\n")
			if inicio < 0 {
				inicio = strings.LastIndex(src[:m[0]], "(ctx, `")
			}
			sentencia := src[inicio:m[0]]
			conAlias := !strings.HasSuffix(lista, "Base")
			switch {
			case strings.Contains(sentencia, "INSERT INTO") && conAlias:
				t.Errorf("%s: INSERT … RETURNING usa %q, que lleva alias; ahí no hay tabla que aliasar", f, lista)
			case strings.Contains(sentencia, "UPDATE ") && fromDeSentencia.MatchString(sentencia) && !conAlias:
				t.Errorf("%s: UPDATE … FROM … RETURNING usa %q, sin alias; con el FROM, `id` y compañía son ambiguos", f, lista)
			}
		}
	}
}
