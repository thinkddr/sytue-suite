package domain

import (
	"errors"
	"strings"
	"testing"
)

// El token del enlace público es una CREDENCIAL: abre el cuaderno sin sesión. Un
// volcado se guarda, se manda y se sube a copias de seguridad, así que si viaja
// ahí dentro sigue abriendo la puerta desde cualquiera de esos sitios.
func TestNativoNoFiltraElTokenPublico(t *testing.T) {
	out := NotebookANativo(Notebook{
		Title:       "C",
		Visibility:  VisPublic,
		PublicToken: "s3cr3t0-del-enlace",
	}, nil)
	if strings.Contains(out, "s3cr3t0-del-enlace") {
		t.Fatal("el token del enlace público salió en el volcado: es una credencial")
	}
}

// El criptograma SÍ va: es la razón de ser de este formato. Sin él, una copia de
// seguridad de un cuaderno cifrado no existe.
func TestNativoConservaElCriptograma(t *testing.T) {
	out := NotebookANativo(Notebook{Title: "C", Encrypted: true},
		[]Note{{Title: "s", ContentCipher: "encv1:AAAA"}})
	if !strings.Contains(out, "encv1:AAAA") {
		t.Fatalf("sin el criptograma no se puede restaurar la nota:\n%s", out)
	}
}

// EL VIAJE DE IDA Y VUELTA es lo que hace que este formato sirva de algo: lo que
// se exporta tiene que volver igual. Si esto se rompe, no da error —el fichero
// se genera y se acepta— y la pérdida solo se ve al abrir el cuaderno restaurado.
func TestNativoIdaYVuelta(t *testing.T) {
	orig := Notebook{Title: "Proyecto", Emoji: "📓", Color: "#ff0000", Encrypted: true}
	notas := []Note{
		{Title: "b", Content: "<p>dos</p>", Col: 0, Position: 2, Width: 300},
		{
			Title: "a", Content: "<p>uno</p>", Col: 0, Position: 1, Width: 260,
			States: []NoteState{{TagName: "P", Name: "Alta"}},
			Links:  []NoteLink{{TargetKind: "drive_file", TargetID: "f-1", Label: "Doc"}},
		},
	}
	nb2, notas2, err := LeerNativo([]byte(NotebookANativo(orig, notas)))
	if err != nil {
		t.Fatalf("LeerNativo: %v", err)
	}
	if nb2.Title != orig.Title || nb2.Emoji != orig.Emoji || nb2.Color != orig.Color {
		t.Errorf("el cuaderno no volvió igual: %+v", nb2)
	}
	if !nb2.Encrypted {
		t.Error("se perdió que el cuaderno está cifrado: al restaurarlo aceptaría texto en claro")
	}
	if len(notas2) != 2 {
		t.Fatalf("volvieron %d notas de 2", len(notas2))
	}
	// El orden de pantalla se conserva: una restauración que recoloca las notas no
	// es una restauración.
	if notas2[0].Title != "a" || notas2[1].Title != "b" {
		t.Errorf("el orden cambió: %q, %q", notas2[0].Title, notas2[1].Title)
	}
	if notas2[0].Width != 260 || len(notas2[0].States) != 1 || len(notas2[0].Links) != 1 {
		t.Errorf("se perdió el ancho, los estados o los enlaces: %+v", notas2[0])
	}
}

// Lo que NO debe volver: la identidad la pone quien importa, no el fichero. Si
// no, cualquiera podría fabricar un `.onex` que dijera pertenecer a otra persona
// o a otra organización.
func TestNativoNoRestauraIdentidadNiToken(t *testing.T) {
	crudo := `{"formato":"sytue.notepads","version":1,
	  "notebook":{"title":"C","owner_sub":"otra-persona","public_token":"s3cr3t0"},
	  "notas":[{"title":"n","owner_sub":"otra-persona"}]}`
	nb, notas, err := LeerNativo([]byte(crudo))
	if err != nil {
		t.Fatalf("LeerNativo: %v", err)
	}
	if nb.OwnerSub != "" || nb.PublicToken != "" {
		t.Errorf("el fichero impuso dueño o token: %+v", nb)
	}
	if len(notas) != 1 || notas[0].OwnerSub != "" {
		t.Errorf("una nota impuso su dueño: %+v", notas)
	}
}

func TestLeerNativoRechazaLoQueNoEs(t *testing.T) {
	casos := []struct {
		nombre string
		datos  string
		quiero error
	}{
		{"vacío", "", ErrFormatoNoReconocido},
		{"json que no es un volcado", `{"foo":1}`, ErrFormatoNoReconocido},
		// El caso REAL: `.onex` es también la extensión del índice de un cuaderno
		// de OneNote, que es binario. Va a llegar, y tiene que decirlo claro.
		{"un .onex de OneNote (binario)", "\x00\x01\xa1\x2f\xffbinario", ErrFormatoNoReconocido},
		{"otro producto", `{"formato":"otra.cosa","version":1}`, ErrFormatoNoReconocido},
		{"versión futura", `{"formato":"sytue.notepads","version":99}`, ErrVersionNoSoportada},
		{"versión cero", `{"formato":"sytue.notepads","version":0}`, ErrVersionNoSoportada},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, _, err := LeerNativo([]byte(c.datos)); !errors.Is(err, c.quiero) {
				t.Errorf("err = %v; quiero %v", err, c.quiero)
			}
		})
	}
}
