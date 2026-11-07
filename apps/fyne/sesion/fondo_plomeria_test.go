// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// TestLosCmdCarganTrasLaEscena: iOS lanza la app sin escena para refrescar su widget y
// corre el `main` entero (fondo.go). Lo que ese `main` hace al arrancar —sin contar lo
// que espera a un gesto o a un AlCambiar, que la suite retiene hasta la escena— no puede
// cargar la pantalla, entrar ni lanzar rutinas: todo eso va dentro de CuandoHayaEscena.
//
// Se sigue el flujo del `main` de cada `cmd/<app>`: sus sentencias, las funciones
// locales que llama, las funciones del paquete `main` y los literales que se invocan en
// el acto. No se entra en los literales que se guardan (un botón, AlAbrirURL, AlVolver)
// ni en los de AlCambiar.
//
// Es una lista POSITIVA: de los valores de otros paquetes —la app, la ventana, la barra,
// la pantalla de cada producto— antes de la escena solo se llama a lo de
// permitidoAntesDeLaEscena (construir, configurar, registrar receptores, montar y
// arrancar el bucle). Cualquier otro método, como `p.MostrarPortada`, `c.Reloj` o
// `c.Cargar`, se marca: una carga con otro nombre no se cuela. Las funciones de paquete
// (los constructores) y los métodos del propio `main`, cuyo cuerpo se recorre, sí.
//
// VISTO EN ROJO: con `p.MostrarPortada(...)` de cmd/certipdf fuera de CuandoHayaEscena.
func TestLosCmdCarganTrasLaEscena(t *testing.T) {
	cmds, err := filepath.Glob(filepath.Join("..", "cmd", "*"))
	if err != nil {
		t.Fatal(err)
	}
	vistos := 0
	for _, dir := range cmds {
		fallos, conSuite := cargasAntesDeLaEscena(t, dir)
		if !conSuite {
			continue // el demo no abre sesión
		}
		vistos++
		for _, f := range fallos {
			t.Errorf("%s: %s, fuera de CuandoHayaEscena: corre también con la app lanzada solo para el widget", dir, f)
		}
	}
	if vistos != 15 {
		t.Fatalf("se han mirado %d cmd con sesión, y la suite tiene quince", vistos)
	}
}

// cargasAntesDeLaEscena devuelve lo que el `main` de dir hace al arrancar sin esperar a
// la escena, y si abre la sesión de la suite.
func cargasAntesDeLaEscena(t *testing.T, dir string) (fallos []string, conSuite bool) {
	t.Helper()
	fset := token.NewFileSet()
	ficheros, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	funciones := map[string]*ast.FuncDecl{}
	paquetes := map[string]bool{}
	var principal *ast.FuncDecl
	for _, nombre := range ficheros {
		if strings.HasSuffix(nombre, "_test.go") {
			continue
		}
		fuente, err := os.ReadFile(nombre)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, nombre, fuente, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range f.Imports {
			paquetes[nombreImportado(im)] = true
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
				if fd.Recv == nil && fd.Name.Name == "main" {
					principal = fd
				} else {
					funciones[fd.Name.Name] = fd
				}
			}
		}
	}
	if principal == nil {
		return nil, false
	}
	r := &recorrido{fset: fset, funciones: funciones, paquetes: paquetes, locales: map[string]*ast.FuncLit{}, hechos: map[string]bool{}}
	r.suite = suiteDe(principal)
	if r.suite == "" {
		return nil, false
	}
	r.mirar(principal.Body, false)
	sort.Strings(r.fallos)
	return r.fallos, true
}

// suiteDe es la variable a la que `main` asigna sesion.Abrir.
func suiteDe(principal *ast.FuncDecl) string {
	nombre := ""
	ast.Inspect(principal.Body, func(n ast.Node) bool {
		a, ok := n.(*ast.AssignStmt)
		if !ok || len(a.Lhs) != 1 || len(a.Rhs) != 1 {
			return true
		}
		if c, ok := a.Rhs[0].(*ast.CallExpr); ok && esSelector(c.Fun, "sesion", "Abrir") {
			if id, ok := a.Lhs[0].(*ast.Ident); ok {
				nombre = id.Name
			}
		}
		return true
	})
	return nombre
}

func esSelector(e ast.Expr, x, sel string) bool {
	s, ok := e.(*ast.SelectorExpr)
	if !ok || s.Sel.Name != sel {
		return false
	}
	id, ok := s.X.(*ast.Ident)
	return ok && (x == "" || id.Name == x)
}

// nombreImportado es el nombre con que un fichero usa un import: el suyo si lo renombra;
// si no, el último elemento de la ruta que no es una versión (`fyne.io/fyne/v2` es `fyne`).
func nombreImportado(im *ast.ImportSpec) string {
	if im.Name != nil {
		return im.Name.Name
	}
	ruta, _ := strconv.Unquote(im.Path.Value)
	nombre := path.Base(ruta)
	if len(nombre) > 1 && nombre[0] == 'v' && strings.Trim(nombre[1:], "0123456789") == "" {
		nombre = path.Base(path.Dir(ruta))
	}
	return nombre
}

// permitidoAntesDeLaEscena son los métodos de valores de otros paquetes a los que un
// `main` puede llamar con la app lanzada solo para el widget: ninguno lee de la red ni
// carga una pantalla. Añadir uno aquí pide mirar qué hace.
func permitidoAntesDeLaEscena(metodo string) bool {
	switch metodo {
	case
		// la app, la ventana y su lienzo
		"NewWindow", "SetContent", "Resize", "SetPadded", "SetMainMenu", "SetOnClosed",
		"SetOnDropped", "SetOnTypedRune", "Settings", "SetTheme", "ThemeVariant", "Canvas",
		"Metadata", "Driver", "Device", "IsMobile", "Orientation",
		// el bucle de la app: lo que espera a la escena ya está apuntado
		"ShowAndRun", "Run", "Quit",
		// textos y estado ya leído
		"T", "Locale", "Dentro", "Organizacion",
		// la barra de la suite (ui.BarraSuite) y sus opciones
		"Montar", "Objeto", "Atajos", "Avisos", "Desplazable", "ClaveTitulo",
		"CederAtajoSi", "CallarAvisosSi",
		// enganches de cada producto, sin red: guardan lo que se les da
		"Portada", "PonerAbrir", "ImportarAlAbrir", "Conexion", "CambiarSesion":
		return true
	}
	// Con y Con…: las opciones de la vista del Hub y de la barra (ConDrive, ConCuenta…),
	// que solo guardan.
	return metodo == "Con" || len(metodo) > 3 && strings.HasPrefix(metodo, "Con") && unicode.IsUpper(rune(metodo[3]))
}

type recorrido struct {
	fset      *token.FileSet
	suite     string
	paquetes  map[string]bool
	funciones map[string]*ast.FuncDecl
	locales   map[string]*ast.FuncLit
	hechos    map[string]bool
	fallos    []string
}

func (r *recorrido) fallo(n ast.Node, que string) {
	p := r.fset.Position(n.Pos())
	r.fallos = append(r.fallos, filepath.Base(p.Filename)+":"+strconv.Itoa(p.Line)+" "+que)
}

// cuerpo recorre una vez cada función local o del paquete en cada contexto.
func (r *recorrido) cuerpo(clave string, n ast.Node, tras bool) {
	if n == nil {
		return
	}
	k := clave + map[bool]string{true: "|tras", false: "|antes"}[tras]
	if r.hechos[k] {
		return
	}
	r.hechos[k] = true
	r.mirar(n, tras)
}

// mirar recorre n sabiendo si corre ya con escena (`tras`).
func (r *recorrido) mirar(n ast.Node, tras bool) {
	ast.Inspect(n, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for i, rhs := range x.Rhs {
				if lit, ok := rhs.(*ast.FuncLit); ok && i < len(x.Lhs) {
					if id, ok := x.Lhs[i].(*ast.Ident); ok {
						r.locales[id.Name] = lit
						continue
					}
				}
				r.mirar(rhs, tras)
			}
			return false
		case *ast.FuncLit:
			return false // se guarda para luego: un gesto, una URL, AlCambiar…
		case *ast.GoStmt:
			if !tras {
				r.fallo(x, "lanza una rutina")
				return false
			}
			r.llamada(x.Call, tras)
			return false
		case *ast.CallExpr:
			r.llamada(x, tras)
			return false
		}
		return true
	})
}

func (r *recorrido) llamada(c *ast.CallExpr, tras bool) {
	if esSelector(c.Fun, r.suite, "CuandoHayaEscena") {
		for _, a := range c.Args {
			r.argumento(a, true)
		}
		return
	}
	switch f := c.Fun.(type) {
	case *ast.FuncLit:
		r.mirar(f.Body, tras)
	case *ast.Ident:
		if lit, ok := r.locales[f.Name]; ok {
			r.cuerpo("local:"+f.Name, lit.Body, tras)
		} else if fd, ok := r.funciones[f.Name]; ok {
			r.cuerpo("func:"+f.Name, fd.Body, tras)
		}
	case *ast.SelectorExpr:
		id, _ := f.X.(*ast.Ident)
		esSuite := id != nil && id.Name == r.suite
		esPaquete := id != nil && r.paquetes[id.Name]
		fd, propio := r.funciones[f.Sel.Name]
		propio = propio && fd.Recv != nil
		switch {
		case !tras && !esSuite && f.Sel.Name == "Arrancar":
			r.fallo(c, "arranca la pantalla ("+nombreDe(f)+")")
		case !tras && esSuite && f.Sel.Name == "Entrar":
			r.fallo(c, "abre el inicio de sesión")
		case !tras && !esSuite && !esPaquete && !propio && !permitidoAntesDeLaEscena(f.Sel.Name):
			r.fallo(c, "llama a "+nombreDe(f)+", que no está en permitidoAntesDeLaEscena")
		}
		if propio {
			r.cuerpo("metodo:"+f.Sel.Name, fd.Body, tras)
		}
		r.mirar(f.X, tras)
	}
	for _, a := range c.Args {
		r.mirar(a, tras)
	}
}

// argumento recorre lo que se pasa a CuandoHayaEscena: un literal, una función local o
// un método del paquete.
func (r *recorrido) argumento(a ast.Expr, tras bool) {
	switch x := a.(type) {
	case *ast.FuncLit:
		r.mirar(x.Body, tras)
	case *ast.Ident:
		if lit, ok := r.locales[x.Name]; ok {
			r.cuerpo("local:"+x.Name, lit.Body, tras)
		}
	}
}

func nombreDe(s *ast.SelectorExpr) string {
	if id, ok := s.X.(*ast.Ident); ok {
		return id.Name + "." + s.Sel.Name
	}
	return s.Sel.Name
}
