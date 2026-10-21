// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del cifrado de PDF. Van de ida y vuelta —se protege un documento y se
// vuelve a abrir— porque es la única forma honesta de comprobar criptografía sin
// meter ficheros binarios en el repo: si el escritor y el lector se equivocaran
// IGUAL, el texto seguiría saliendo, así que además se comprueba que los bytes
// del fichero ya no contienen el texto en claro.
package domain

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// contratoDePrueba es el documento de dos páginas que usan estos checks.
func contratoDePrueba(t *testing.T) *PDFDoc {
	t.Helper()
	return docDePrueba(t, "Contrato confidencial", "Segunda pagina del contrato")
}

func TestProtegerYAbrirConContrasenaDeUsuario(t *testing.T) {
	protegido, err := ProtegerPDF(contratoDePrueba(t), OpcionesProteccion{
		ContrasenaUsuario: "secreta",
		Permitir:          PermisosTodos(),
	})
	if err != nil {
		t.Fatalf("ProtegerPDF: %v", err)
	}

	// El texto NO puede estar legible en el fichero: si lo estuviera, el cifrado
	// no se habría aplicado y el resto del test pasaría igual.
	if bytes.Contains(protegido, []byte("Contrato confidencial")) {
		t.Fatal("el texto sigue en claro dentro del PDF «protegido»")
	}
	if _, err := LeerPDF(protegido); !errors.Is(err, ErrPDFClaveIncorrecta) {
		t.Errorf("sin contraseña debería pedirla, dio: %v", err)
	}

	d, err := LeerPDFConClave(protegido, "secreta")
	if err != nil {
		t.Fatalf("LeerPDFConClave: %v", err)
	}
	if d.NumPaginas() != 2 {
		t.Fatalf("NumPaginas = %d, quería 2", d.NumPaginas())
	}
	texto, err := d.TextoPlano(0)
	if err != nil {
		t.Fatalf("TextoPlano: %v", err)
	}
	if !strings.Contains(texto, "Contrato confidencial") {
		t.Errorf("el texto descifrado = %q", texto)
	}
	if !d.EstaCifrado() {
		t.Error("EstaCifrado debería ser true")
	}
}

func TestProtegerConContrasenaVaciaSeAbreSinPedirNada(t *testing.T) {
	// Es el caso mayoritario en una empresa: cifrado solo para declarar permisos.
	// Un visor que pidiera contraseña aquí sería inutilizable.
	protegido, err := ProtegerPDF(contratoDePrueba(t), OpcionesProteccion{
		ContrasenaDueno: "solo-el-dueno",
		Permitir:        Permisos{Imprimir: true, Accesibilidad: true},
	})
	if err != nil {
		t.Fatalf("ProtegerPDF: %v", err)
	}
	d, err := LeerPDF(protegido)
	if err != nil {
		t.Fatalf("un PDF con contraseña de usuario vacía tiene que abrirse solo: %v", err)
	}
	texto, _ := d.TextoPlano(0)
	if !strings.Contains(texto, "Contrato confidencial") {
		t.Errorf("texto = %q", texto)
	}

	p := d.PermisosDelDocumento()
	if !p.Imprimir {
		t.Error("se permitió imprimir y llegó prohibido")
	}
	if p.Copiar {
		t.Error("NO se permitió copiar y llegó permitido")
	}
	if !p.SoloConClaveDeDueno {
		t.Error("se abrió con la clave de usuario: los permisos deben aplicar")
	}
}

func TestAbrirConContrasenaDeDuenoLevantaLosPermisos(t *testing.T) {
	protegido, err := ProtegerPDF(contratoDePrueba(t), OpcionesProteccion{
		ContrasenaUsuario: "lectura",
		ContrasenaDueno:   "mando",
		Permitir:          Permisos{}, // todo prohibido
	})
	if err != nil {
		t.Fatalf("ProtegerPDF: %v", err)
	}
	d, err := LeerPDFConClave(protegido, "mando")
	if err != nil {
		t.Fatalf("la contraseña de dueño tiene que abrir el documento: %v", err)
	}
	if d.PermisosDelDocumento().SoloConClaveDeDueno {
		t.Error("con la clave de dueño no aplican los permisos")
	}
}

func TestContrasenaIncorrectaNoAbre(t *testing.T) {
	protegido, err := ProtegerPDF(contratoDePrueba(t), OpcionesProteccion{
		ContrasenaUsuario: "correcta", Permitir: PermisosTodos(),
	})
	if err != nil {
		t.Fatalf("ProtegerPDF: %v", err)
	}
	if _, err := LeerPDFConClave(protegido, "incorrecta"); !errors.Is(err, ErrPDFClaveIncorrecta) {
		t.Errorf("con la contraseña mal debería dar ErrPDFClaveIncorrecta, dio: %v", err)
	}
}

// TestDesprotegerDevuelveUnPDFLimpio cubre el fallo que aparece en cuanto el
// lector aprende a abrir documentos cifrados: el ensamblador copia los flujos
// SIN tocar, así que sin descifrarlos al copiar el resultado abre y sale en
// blanco, y no salta ningún error en todo el camino.
func TestDesprotegerDevuelveUnPDFLimpio(t *testing.T) {
	protegido, err := ProtegerPDF(contratoDePrueba(t), OpcionesProteccion{
		ContrasenaUsuario: "x", Permitir: PermisosTodos(),
	})
	if err != nil {
		t.Fatalf("ProtegerPDF: %v", err)
	}
	d, err := LeerPDFConClave(protegido, "x")
	if err != nil {
		t.Fatalf("LeerPDFConClave: %v", err)
	}
	limpio, err := DesprotegerPDF(d)
	if err != nil {
		t.Fatalf("DesprotegerPDF: %v", err)
	}
	d2, err := LeerPDF(limpio)
	if err != nil {
		t.Fatalf("el PDF desprotegido no abre: %v", err)
	}
	if d2.EstaCifrado() {
		t.Error("sigue cifrado")
	}
	texto, err := d2.TextoPlano(0)
	if err != nil {
		t.Fatalf("TextoPlano: %v", err)
	}
	if !strings.Contains(texto, "Contrato confidencial") {
		t.Errorf("el contenido se perdió al desproteger: %q", texto)
	}
}

// TestOperacionDePaginasSobreDocumentoCifrado es el mismo agujero por otra
// puerta: combinar o rotar un documento protegido tiene que dar un fichero que
// se vea.
func TestOperacionDePaginasSobreDocumentoCifrado(t *testing.T) {
	protegido, err := ProtegerPDF(contratoDePrueba(t), OpcionesProteccion{Permitir: PermisosTodos()})
	if err != nil {
		t.Fatalf("ProtegerPDF: %v", err)
	}
	d, err := LeerPDF(protegido)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	salida, err := ExtraerPaginasPDF(d, []int{1})
	if err != nil {
		t.Fatalf("ExtraerPaginasPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF de la extracción: %v", err)
	}
	texto, err := d2.TextoPlano(0)
	if err != nil {
		t.Fatalf("TextoPlano: %v", err)
	}
	if !strings.Contains(texto, "Segunda pagina") {
		t.Errorf("la página extraída de un PDF cifrado salió ilegible: %q", texto)
	}
}

func TestRenderDeDocumentoCifrado(t *testing.T) {
	protegido, err := ProtegerPDF(contratoDePrueba(t), OpcionesProteccion{Permitir: PermisosTodos()})
	if err != nil {
		t.Fatalf("ProtegerPDF: %v", err)
	}
	d, err := LeerPDF(protegido)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("RenderizarPaginaSVG: %v", err)
	}
	if !strings.Contains(svg, "Contrato confidencial") {
		t.Errorf("el render de un PDF cifrado salió vacío: %s", primerasLineas(svg))
	}
}

func TestPermisosDeUnDocumentoSinCifrar(t *testing.T) {
	p := contratoDePrueba(t).PermisosDelDocumento()
	if !p.Imprimir || !p.Copiar || !p.Modificar {
		t.Error("un PDF sin proteger no restringe nada")
	}
}

// TestPDePermisosDejaLosBitsReservados comprueba el detalle que rompe /P sin
// avisar: es un entero CON SIGNO y los bits altos van a uno.
func TestPDePermisosDejaLosBitsReservados(t *testing.T) {
	todo := pDePermisos(PermisosTodos())
	if todo >= 0 {
		t.Errorf("/P con todos los permisos debería ser negativo, es %d", todo)
	}
	if todo&(1<<2) == 0 {
		t.Error("el bit 3 (imprimir) tendría que estar activo")
	}
	sinCopiar := pDePermisos(Permisos{Imprimir: true})
	if sinCopiar&(1<<4) != 0 {
		t.Error("el bit 5 (copiar) tendría que estar apagado")
	}
	// Y la vuelta: lo que se escribe se tiene que poder leer igual.
	leidos := permisosDeP(pDePermisos(Permisos{Imprimir: true, Anotar: true}))
	if !leidos.Imprimir || !leidos.Anotar || leidos.Copiar || leidos.Modificar {
		t.Errorf("ida y vuelta de permisos = %+v", leidos)
	}
}

func TestHuellaDelDocumento(t *testing.T) {
	// El escritor de la casa no emite /Info, así que la huella sale limpia; lo que
	// se comprueba es que no invente datos ni reviente sin metadatos.
	h := contratoDePrueba(t).Huella()
	if h.Autor != "" || h.ConJS {
		t.Errorf("huella inesperada: %+v", h)
	}
}

func TestCompararPDFDetectaLoQueCambia(t *testing.T) {
	a := contratoDePrueba(t)
	b := docDePrueba(t, "Contrato confidencial", "Segunda pagina revisada")

	difs, err := CompararPDF(a, b)
	if err != nil {
		t.Fatalf("CompararPDF: %v", err)
	}
	if len(difs) != 1 {
		t.Fatalf("esperaba 1 página con cambios, hay %d: %+v", len(difs), difs)
	}
	if difs[0].Pagina != 2 {
		t.Errorf("el cambio está en la página 2, dijo %d", difs[0].Pagina)
	}
	if len(difs[0].Quitado) != 1 || !strings.Contains(difs[0].Quitado[0], "del contrato") {
		t.Errorf("quitado = %v", difs[0].Quitado)
	}
	if len(difs[0].Anadido) != 1 || !strings.Contains(difs[0].Anadido[0], "revisada") {
		t.Errorf("añadido = %v", difs[0].Anadido)
	}
}

// TestCompararIgnoraElOrden comprueba que mover un párrafo no se cuenta como
// borrado más alta: eso es el ruido que hace inservible un comparador.
func TestCompararIgnoraElOrden(t *testing.T) {
	quitado, anadido := diferenciaDeLineas(
		[]string{"uno", "dos", "tres"},
		[]string{"tres", "uno", "dos"},
	)
	if len(quitado) != 0 || len(anadido) != 0 {
		t.Errorf("reordenar no es cambiar: quitado=%v anadido=%v", quitado, anadido)
	}
}

func TestCompararDocumentosIdenticosNoDaNada(t *testing.T) {
	difs, err := CompararPDF(contratoDePrueba(t), contratoDePrueba(t))
	if err != nil {
		t.Fatalf("CompararPDF: %v", err)
	}
	if len(difs) != 0 {
		t.Errorf("dos documentos iguales no tienen diferencias: %+v", difs)
	}
}

func TestHashDeContenidoDistingueDocumentos(t *testing.T) {
	a := hashDeContenido(contratoDePrueba(t))
	if a != hashDeContenido(contratoDePrueba(t)) {
		t.Error("el mismo documento tiene que dar el mismo hash")
	}
	otro := docDePrueba(t, "otro documento")
	if a == hashDeContenido(otro) {
		t.Error("documentos distintos con el mismo hash")
	}
}

// pdfConAdjuntoCifrado es como los de AuthEvent /EFOpen del corpus
// (auth-event-ef-open de pdf.js, nontrivial-crypt-filter de qpdf): el documento
// va en claro y solo el adjunto, cifrado con la contraseña «clave».
func pdfConAdjuntoCifrado(t *testing.T) []byte {
	t.Helper()
	c, err := nuevoCifrador(OpcionesProteccion{ContrasenaUsuario: "clave", Permitir: PermisosTodos()})
	if err != nil {
		t.Fatal(err)
	}
	enc := c.diccionario()
	enc["StmF"], enc["StrF"], enc["EFF"] = pdfName("Identity"), pdfName("Identity"), pdfName("StdCF")
	enc["CF"].(pdfDict)["StdCF"].(pdfDict)["AuthEvent"] = pdfName("EFOpen")
	adjunto := cifrarAES(c.clave, []byte("secreto"))
	contenido := "BT /F1 12 Tf 10 50 Td (hola) Tj ET"
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles << /Names [(a.txt) 5 0 R] >> >> >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Contents 4 0 R ` +
			`/Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> >> >> >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(contenido), contenido),
		`<< /Type /Filespec /F (a.txt) /EF << /F 6 0 R >> >>`,
		fmt.Sprintf("<< /Type /EmbeddedFile /Length %d >>\nstream\n%s\nendstream", len(adjunto), adjunto),
		string(serializarObjetoPDF(enc)),
	}
	return pdfMinimo(t, fmt.Sprintf(`<< /Size 8 /Root 1 0 R /Encrypt 7 0 R /ID [<%x> <%x>] >>`, c.id, c.id), objs)
}

// Si solo van cifrados los adjuntos, el documento se abre sin contraseña —lo
// que hace Poppler— y la contraseña se pide para el adjunto o para guardar.
func TestAbrirConSoloLosAdjuntosCifrados(t *testing.T) {
	datos := pdfConAdjuntoCifrado(t)
	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if txt, err := d.TextoPlano(0); err != nil || !strings.Contains(txt, "hola") {
		t.Errorf("texto = %q, %v; quería «hola»", txt, err)
	}
	if _, err := d.ExtraerAdjunto("a.txt"); !errors.Is(err, ErrPDFClaveIncorrecta) {
		t.Errorf("adjunto sin contraseña: err = %v, quería ErrPDFClaveIncorrecta", err)
	}
	if _, err := RotarPaginasPDF(d, map[int]int{0: 90}); !errors.Is(err, ErrPDFClaveIncorrecta) {
		t.Errorf("guardar sin la contraseña del adjunto: err = %v, quería ErrPDFClaveIncorrecta", err)
	}

	d, err = LeerPDFConClave(datos, "clave")
	if err != nil {
		t.Fatalf("LeerPDFConClave: %v", err)
	}
	if adj, err := d.ExtraerAdjunto("a.txt"); err != nil || string(adj) != "secreto" {
		t.Errorf("adjunto con contraseña = %q, %v; quería «secreto»", adj, err)
	}
}
