// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"maps"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
)

// La protección de hoja y de libro de la web (`SheetProtection`, `WorkbookProtection` y
// los predicados puros de `sheets/format.ts`), con el mismo criterio de Excel: con la
// hoja protegida, una celda está BLOQUEADA salvo `locked: false`; y con el libro
// protegido no se añaden, borran, renombran, duplican ni reordenan hojas.
//
// La contraseña la cumple el SERVIDOR, como en la web: guarda solo su hash con sal
// (model.HashClave, el esquema de OOXML) y verifica en model.AplicarProteccion quién
// puede quitar o cambiar una protección. Por eso no entra NUNCA en el modelo —que se
// guarda, se sirve a todo lector y queda en el historial—: el modelo lleva solo
// `hasPassword`, que fija el servidor, y lo tecleado viaja aparte con el guardado
// (Sobre, el `proteccion` de POST …/ops). Antes se guardaba en claro dentro de la hoja y
// se cotejaba aquí, y `hasPassword` se perdía al leer: una hoja con contraseña puesta
// desde la web se veía sin ella y se dejaba desproteger, y el servidor rechazaba luego
// ese guardado a quien no era el propietario.

// Proteccion es `SheetProtection`. Su PRESENCIA —aunque sea `{}`— es la hoja protegida,
// y solo se guarda lo que difiere del valor por defecto: `selectCells` solo si es
// false, y los permisos solo si están concedidos.
type Proteccion struct {
	// Los nombres los fija `SheetProtection` de la web, en camelCase.
	//
	// HasPassword lo fija el servidor: la protección tiene contraseña (su hash vive en el
	// servicio y no vuelve al cliente). Se conserva tal cual al leer y al guardar.
	HasPassword bool `json:"hasPassword,omitempty"` //nolint:tagliatelle // la web
	// Ranges son los rangos editables por persona o grupo (`ranges`): sus celdas las
	// puede escribir quien se nombra aunque estén bloqueadas. Son del modelo del
	// servidor, que es quien los cumple (apps/nucleo/office/model/proteccion_rangos.go):
	// perderlos al guardar es cambiar la protección, y a quien no es el propietario se le
	// rechazaba el guardado entero.
	Ranges      []model.RangoEditable `json:"ranges,omitempty"`
	SelectCells *bool                 `json:"selectCells,omitempty"` //nolint:tagliatelle // la web
	FormatCells bool                  `json:"formatCells,omitempty"` //nolint:tagliatelle // la web
	InsertRows  bool                  `json:"insertRows,omitempty"`  //nolint:tagliatelle // la web
	DeleteRows  bool                  `json:"deleteRows,omitempty"`  //nolint:tagliatelle // la web
	InsertCols  bool                  `json:"insertCols,omitempty"`  //nolint:tagliatelle // la web
	DeleteCols  bool                  `json:"deleteCols,omitempty"`  //nolint:tagliatelle // la web
	Sort        bool                  `json:"sort,omitempty"`
	AutoFilter  bool                  `json:"autoFilter,omitempty"` //nolint:tagliatelle // la web
	Objects     bool                  `json:"objects,omitempty"`
	Scenarios   bool                  `json:"scenarios,omitempty"`
}

// ProteccionLibro es `WorkbookProtection`: la estructura del libro, con contraseña
// opcional (`hasPassword`, como en la hoja).
type ProteccionLibro struct {
	HasPassword bool `json:"hasPassword,omitempty"` //nolint:tagliatelle // la web
}

// Permisos son los del cuadro «Proteger hoja», en su orden (`PROTECT_PERMS`): la clave
// del modelo, el rótulo y el campo que la guarda.
var Permisos = []struct {
	Clave, Rotulo string
	Campo         func(*Proteccion) *bool
}{
	{"insertRows", "table.rowAbove", func(p *Proteccion) *bool { return &p.InsertRows }},
	{"deleteRows", "table.deleteRows", func(p *Proteccion) *bool { return &p.DeleteRows }},
	{"insertCols", "data.perm.insertCols", func(p *Proteccion) *bool { return &p.InsertCols }},
	{"deleteCols", "data.perm.deleteCols", func(p *Proteccion) *bool { return &p.DeleteCols }},
	{"sort", "data.perm.sort", func(p *Proteccion) *bool { return &p.Sort }},
	{"autoFilter", "data.perm.autoFilter", func(p *Proteccion) *bool { return &p.AutoFilter }},
	{"objects", "data.perm.objects", func(p *Proteccion) *bool { return &p.Objects }},
	{"scenarios", "data.perm.scenarios", func(p *Proteccion) *bool { return &p.Scenarios }},
}

// protegerDe es `sanitizeProtection`: nil si no es un objeto. Una `password` en el
// objeto (la de un cliente anterior) no se lee: no es del modelo.
func protegerDe(crudo any) *Proteccion {
	m, ok := crudo.(map[string]any)
	if !ok {
		return nil
	}
	p := &Proteccion{HasPassword: m["hasPassword"] == true, FormatCells: m["formatCells"] == true, Ranges: rangosSaneados(m["ranges"])}
	if m["selectCells"] == false {
		no := false
		p.SelectCells = &no
	}
	for _, pm := range Permisos {
		*pm.Campo(p) = m[pm.Clave] == true
	}
	return p
}

// rangosSaneados es el `rangosSaneados` de la web: solo los rangos con una referencia que
// se lee («B2» o «B2:D10», `parseRangeRef`) y alguien concedido, con la referencia en
// mayúsculas y sin «$». nil si no queda ninguno: entonces `ranges` no se escribe.
func rangosSaneados(crudo any) []model.RangoEditable {
	lista, _ := crudo.([]any)
	cadenas := func(v any) []string {
		var out []string
		xs, _ := v.([]any)
		for _, x := range xs {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	var out []model.RangoEditable
	for _, x := range lista {
		g, _ := x.(map[string]any)
		ref, ok := g["ref"].(string)
		if !ok {
			continue
		}
		if _, ok := formula.ParseRangeRef(ref); !ok {
			continue
		}
		users, groups := cadenas(g["users"]), cadenas(g["groups"])
		if len(users)+len(groups) == 0 {
			continue
		}
		ref = strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(ref)), "$", "")
		out = append(out, model.RangoEditable{Ref: ref, Users: users, Groups: groups})
	}
	return out
}

// protegerLibroDe es `sanitizeWorkbookProtection`.
func protegerLibroDe(crudo any) *ProteccionLibro {
	m, ok := crudo.(map[string]any)
	if !ok {
		return nil
	}
	return &ProteccionLibro{HasPassword: m["hasPassword"] == true}
}

// Proteccion es la de la hoja activa, o nil.
func (l *Libro) Proteccion() *Proteccion { return l.h().proteccion }

// ProteccionLibro es la de la estructura del libro, o nil.
func (l *Libro) ProteccionLibro() *ProteccionLibro { return l.f.proteccion }

// Proteger protege la hoja activa (`protect`) con la contraseña `clave` ("" sin ella):
// en el modelo solo queda `hasPassword`; la contraseña sale con el próximo guardado
// (`presentarNueva`).
func (l *Libro) Proteger(p *Proteccion, clave string) {
	if p == nil {
		p = &Proteccion{}
	}
	if clave != "" {
		p.HasPassword = true
		l.presentarNueva(model.AmbitoHoja(l.h().nombre), clave)
	}
	l.anotar()
	l.h().proteccion = p
}

// Desproteger es `unprotect`: quita la protección de la hoja activa y presenta `clave`
// con el próximo guardado, que es donde se verifica (model.AplicarProteccion). Aquí
// solo se ahorra el viaje que el servidor va a rechazar seguro: quien no es el
// propietario no quita una protección sin contraseña (ErrProtegido), ni una con
// contraseña sin teclearla. false, y el motivo para el aviso, si no se quita.
func (l *Libro) Desproteger(clave string) (bool, string) {
	p := l.h().proteccion
	if p == nil {
		return true, ""
	}
	if motivo := l.puedeQuitar(p.HasPassword, clave); motivo != "" {
		return false, motivo
	}
	l.sobre.Clave = clave
	l.anotar()
	l.h().proteccion = nil
	return true, ""
}

// ProtegerLibro y DesprotegerLibro son `protectbook` y `unprotectbook`, con la misma
// regla que la hoja.
func (l *Libro) ProtegerLibro(p *ProteccionLibro, clave string) {
	if p == nil {
		p = &ProteccionLibro{}
	}
	if clave != "" {
		p.HasPassword = true
		l.presentarNueva(model.AmbitoLibro, clave)
	}
	l.anotar()
	l.f.proteccion = p
}

func (l *Libro) DesprotegerLibro(clave string) (bool, string) {
	p := l.f.proteccion
	if p == nil {
		return true, ""
	}
	if motivo := l.puedeQuitar(p.HasPassword, clave); motivo != "" {
		return false, motivo
	}
	l.sobre.Clave = clave
	l.anotar()
	l.f.proteccion = nil
	return true, ""
}

// puedeQuitar es la comprobación previa de `unprotect`: "" si el guardado puede llevarlo
// al servidor, o la clave del título del aviso. El propietario siempre puede.
func (l *Libro) puedeQuitar(conClave bool, clave string) string {
	switch {
	case !l.ajeno:
		return ""
	case !conClave:
		return "sheet.msg.protectedSheet"
	case clave == "":
		return "sheet.msg.wrongPassword"
	}
	return ""
}

// presentarNueva es `presentarNueva`: la contraseña nueva de un ámbito sale en el
// próximo guardado.
func (l *Libro) presentarNueva(ambito, clave string) {
	if l.sobre.Nuevas == nil {
		l.sobre.Nuevas = map[string]string{}
	}
	l.sobre.Nuevas[ambito] = clave
}

// FijarAjeno dice que quien edita NO es el propietario del documento
// (`fijarPropietario(false)` de la web).
func (l *Libro) FijarAjeno(ajeno bool) { l.ajeno = ajeno }

// Sobre es lo tecleado que aún no ha recibido el servidor (`sobreProteccion`), para el
// próximo guardado: model.ConProteccion lo adjunta al contexto y api.Docs lo manda
// como `proteccion`, fuera de las operaciones.
func (l *Libro) Sobre() model.Proteccion {
	return model.Proteccion{Clave: l.sobre.Clave, Nuevas: maps.Clone(l.sobre.Nuevas)}
}

// OlvidarSobre es `olvidarProteccion`: el servidor ya tiene `enviado` (o lo rechazó) y
// no se guarda ni un instante más. Lo tecleado DESPUÉS de mandarlo se queda para el
// guardado siguiente.
func (l *Libro) OlvidarSobre(enviado model.Proteccion) {
	if l.sobre.Clave == enviado.Clave {
		l.sobre.Clave = ""
	}
	for k, v := range enviado.Nuevas {
		if l.sobre.Nuevas[k] == v {
			delete(l.sobre.Nuevas, k)
		}
	}
}

// FijarYo dice quién edita (su `sub`), como `fijarYo` de la web: las celdas de sus
// rangos editables se le enseñan desbloqueadas. Sin nadie, ningún rango concede nada.
func (l *Libro) FijarYo(sub string) { l.yo = sub }

// misRangos es la parte de `estiloParaMi` que no depende de la celda: los rangos
// editables de la hoja activa concedidos a quien edita, con la regla del servidor.
//
// ponytail: como en la web, solo por su `sub`. Los grupos están en la sesión del
// servidor y el cliente no los ve: una celda concedida solo a un grupo se enseña
// bloqueada aunque el servicio aceptaría escribirla.
func (l *Libro) misRangos() model.RangosEditables {
	p := l.Proteccion()
	if p == nil {
		return nil
	}
	return model.RangosConcedidos(p.Ranges, model.Subject{Sub: l.yo})
}

// desbloqueada es `canEditCell` con la hoja protegida sobre `estiloParaMi`: `locked:
// false`, o una celda de un rango editable de quien edita.
func (l *Libro) desbloqueada(c, f int) bool { return l.desbloqueadaCon(l.misRangos(), c, f) }

// desbloqueadaCon es desbloqueada con los rangos ya leídos, para quien mira muchas celdas.
func (l *Libro) desbloqueadaCon(mios model.RangosEditables, c, f int) bool {
	b := l.Estilo(c, f).Bloqueada
	return (b != nil && !*b) || mios.Contiene(c+1, f+1)
}

// FormulaOculta es `isFormulaHidden` con una fórmula dentro: hoja protegida, celda
// `hidden` y un «=…». Su fórmula no se enseña en la barra ni sale al copiar.
func (l *Libro) FormulaOculta(c, f int) bool {
	return l.Proteccion() != nil && l.Estilo(c, f).Oculta && esFormula(l.HTML(c, f))
}

// Editable es `canEditCellContent`: sin protección, siempre; con ella, solo lo
// desbloqueado cuya fórmula no esté oculta —editarla la revelaría—.
func (l *Libro) Editable(c, f int) bool {
	return l.Proteccion() == nil || (l.desbloqueada(c, f) && !l.FormulaOculta(c, f))
}

// Seleccionable es `canSelectAt`: sin «Seleccionar celdas», solo las desbloqueadas.
func (l *Libro) Seleccionable(c, f int) bool {
	p := l.Proteccion()
	return p == nil || p.SelectCells == nil || l.desbloqueada(c, f)
}

// rangoBloqueado es `rangeLocked`: con la hoja protegida, alguna celda bloqueada.
func (l *Libro) rangoBloqueado(c1, f1, c2, f2 int) bool {
	if l.Proteccion() == nil {
		return false
	}
	mios := l.misRangos()
	for f := f1; f <= f2; f++ {
		for c := c1; c <= c2; c++ {
			if !l.desbloqueadaCon(mios, c, f) {
				return true
			}
		}
	}
	return false
}

// Comandos vetados con la hoja protegida (`PROTECT_CONTENT_CMDS`, `PROTECT_FORMAT_CMDS`
// y `PROTECT_PERMISSION_CMDS`). Los de la cinta genérica van con su equivalente de la
// hoja: el subrayado del cromo es aquí un `style`.
var (
	cmdsContenido = map[editor.Comando]bool{
		CmdPegar: true, CmdDuplicados: true, CmdTextoCols: true, CmdCombinar: true, CmdSubtotal: true,
		CmdDinamica: true, CmdDinamicaAct: true, CmdDinamicaQuitarla: true,
		CmdObjetivo: true, CmdSolver: true, CmdTablaDatos: true, CmdEscAplicar: true, CmdEscInforme: true,
		CmdRellenar: true, CmdTablaAnadir: true, CmdBloque: true, CmdLimpiarTexto: true, CmdAutosuma: true,
	}
	cmdsFormato = map[editor.Comando]bool{
		CmdFormatoNum: true, CmdDecimales: true, CmdEstilo: true, CmdBordes: true, CmdQuitarEstil: true, CmdAlinear: true,
		CmdOcultar:     true, // «Formato de filas/columnas» en Excel: el permiso más cercano es el de formato
		CmdAnchoCol:    true, // ídem: el ancho y el alto, como `PROTECT_FORMAT_CMDS` de la web
		CmdAltoFila:    true,
		editor.Negrita: true, editor.Cursiva: true, editor.Subrayado: true, editor.Tachado: true, editor.SinFormato: true,
		editor.AlinearIzq: true, editor.AlinearCen: true, editor.AlinearDer: true,
	}
	cmdsPermiso = map[editor.Comando]func(*Proteccion) bool{
		CmdInsFila:       func(p *Proteccion) bool { return p.InsertRows },
		CmdBorrarFilas:   func(p *Proteccion) bool { return p.DeleteRows },
		CmdInsCol:        func(p *Proteccion) bool { return p.InsertCols },
		CmdBorrarCols:    func(p *Proteccion) bool { return p.DeleteCols },
		CmdOrdenar:       func(p *Proteccion) bool { return p.Sort },
		CmdAutofiltro:    func(p *Proteccion) bool { return p.AutoFilter },
		CmdFiltrar:       func(p *Proteccion) bool { return p.AutoFilter },
		CmdCondFiltro:    func(p *Proteccion) bool { return p.AutoFilter },
		CmdOrdenarFiltro: func(p *Proteccion) bool { return p.Sort },
		CmdCondAnadir:    func(p *Proteccion) bool { return p.Objects },
		CmdCondQuitar:    func(p *Proteccion) bool { return p.Objects },
		CmdCondMover:     func(p *Proteccion) bool { return p.Objects },
		CmdValAnadir:     func(p *Proteccion) bool { return p.Objects },
		CmdValQuitar:     func(p *Proteccion) bool { return p.Objects },
		CmdGrafico:       func(p *Proteccion) bool { return p.Objects },
		CmdMiniAnadir:    func(p *Proteccion) bool { return p.Objects },
		CmdMiniQuitar:    func(p *Proteccion) bool { return p.Objects },
		CmdSegAnadir:     func(p *Proteccion) bool { return p.Objects },
	}
)

// vetado dice si la hoja protegida no deja mandar ese comando.
func (l *Libro) vetado(cmd editor.Comando) bool {
	p := l.Proteccion()
	if p == nil {
		return false
	}
	if permiso, ok := cmdsPermiso[cmd]; ok {
		return !permiso(p)
	}
	return cmdsContenido[cmd] || (cmdsFormato[cmd] && !p.FormatCells)
}

// Los comandos de la protección.
const (
	CmdProteger         editor.Comando = "protect"       // JSON de una SheetProtection
	CmdDesproteger      editor.Comando = "unprotect"     // la contraseña tecleada
	CmdProtegerLibro    editor.Comando = "protectbook"   // JSON {password?}
	CmdDesprotegerLibro editor.Comando = "unprotectbook" // la contraseña tecleada
)

// mandarProteccion ejecuta los cuatro comandos de la protección. false si no es uno.
func (c *Lienzo) mandarProteccion(cmd editor.Comando, valor string) bool {
	var crudo any
	switch cmd {
	case CmdProteger, CmdProtegerLibro:
		if json.Unmarshal([]byte(valor), &crudo) != nil {
			return true
		}
	case CmdDesproteger, CmdDesprotegerLibro:
	default:
		return false
	}
	l := c.Libro
	// La contraseña del comando NO entra en el modelo: va al sobre (Libro.Sobre).
	clave := ""
	if m, ok := crudo.(map[string]any); ok {
		clave, _ = m["password"].(string)
	}
	switch cmd {
	case CmdProteger:
		l.Proteger(protegerDe(crudo), clave)
		c.Rejilla.Cancelar() // cierra cualquier edición en curso
	case CmdProtegerLibro:
		l.ProtegerLibro(protegerLibroDe(crudo), clave)
	case CmdDesproteger:
		if ok, motivo := l.Desproteger(valor); !ok {
			c.avisar2(ui.AvisoAdvertencia, motivo, detalleNoQuita(motivo, "sheet.msg.unprotectSheetFailed"))
			return true
		}
	default:
		if ok, motivo := l.DesprotegerLibro(valor); !ok {
			if motivo == "sheet.msg.protectedSheet" {
				motivo = "sheet.msg.protectedBook"
			}
			c.avisar2(ui.AvisoAdvertencia, motivo, detalleNoQuita(motivo, "sheet.msg.unprotectBookFailed"))
			return true
		}
	}
	c.trasCambio()
	return true
}

// detalleNoQuita es el detalle del aviso de una protección que no se quita: sin
// contraseña, el de la web (`protectionDenied`); con ella, el de la contraseña.
func detalleNoQuita(motivo, conClave string) string {
	if motivo == "sheet.msg.wrongPassword" {
		return conClave
	}
	return "sheet.msg.protectionDenied"
}

// avisar2 enseña un aviso de dos claves del catálogo.
func (c *Lienzo) avisar2(tono ui.TonoAviso, titulo, detalle string) {
	if c.AlAviso != nil {
		c.AlAviso(tono, c.t(titulo, nil), c.t(detalle, nil))
	}
}

// EsperaAvisoProtegida es el antirrebote del aviso «hoja protegida»: teclear encima de
// una celda bloqueada no llena la pila de avisos.
const EsperaAvisoProtegida = 1500 * time.Millisecond

// avisarProtegida es `notifyProtected`.
func (c *Lienzo) avisarProtegida(celda Celda) {
	if time.Since(c.avisoProtegida) < EsperaAvisoProtegida || c.AlAviso == nil {
		return
	}
	c.avisoProtegida = time.Now()
	c.AlAviso(ui.AvisoInfo, c.t("sheet.msg.protectedSheet", nil),
		c.t("sheet.msg.protectedCell", i18n.Vars{"ref": Ref(celda.C, celda.F)}))
}

// estructura es `guardStructure`: con el libro protegido, no, y se dice.
func (c *Lienzo) estructura() bool {
	if c.Libro.ProteccionLibro() == nil {
		return true
	}
	c.avisar2(ui.AvisoAdvertencia, "sheet.msg.protectedBook", "sheet.msg.unprotectToEdit")
	return false
}
