// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/banca"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/obligaciones"
	"git.sytue.eu/sytue/one/apps/nucleo/validation"
)

// EL BANCO COMO TAREAS (entrega 2). Es `store/banca.go` y `http/banca.go` del servidor
// sobre SQLite: cada movimiento es una `linea_extracto` pendiente hasta que tiene su
// factura o su explicación, y justificarlo escribe su asiento (diario «banco», con la
// línea como origen: no se asienta dos veces). El extracto entra como FICHERO Norma 43
// (apps/nucleo/erp/banca): no hay conexión con el banco.

var (
	// ErrYaJustificado: el movimiento ya tiene factura o explicación.
	ErrYaJustificado = errors.New("erp: el movimiento ya está justificado")
	// ErrNoJustificado: deshacer un movimiento que sigue pendiente.
	ErrNoJustificado = errors.New("erp: el movimiento no está justificado")
	// ErrFacturaNoAbierta: casar con una factura ya pagada o anulada.
	ErrFacturaNoAbierta = errors.New("erp: la factura no está pendiente de cobro ni de pago")
	errSentidoFactura   = errors.New("erp: el sentido del movimiento no es el de la factura")
	errMotivo           = errors.New("erp: motivo desconocido")
)

// cuentaBancoPorDefecto es la de tesorería cuando la línea no dice de qué cuenta es.
const cuentaBancoPorDefecto = "572000"

// FacturaAbierta es una factura sin cobrar o sin pagar (la forma de `store.FacturaAbierta`).
type FacturaAbierta struct {
	ID          string `json:"id"`
	Modelo      string `json:"modelo"`
	Ref         string `json:"ref"`
	Estado      string `json:"estado"`
	Tercero     string `json:"tercero"`
	NIF         string `json:"-"`
	Fecha       string `json:"fecha"`
	Vencimiento string `json:"-"`
	Total       int64  `json:"total"`
}

// buscarPor devuelve el primer registro vivo del modelo cuyo campo vale `valor`.
func (a *Almacen) buscarPor(ctx context.Context, q consultor, modelo, campo, valor string) (api.ERPRegistro, bool, error) {
	regs, err := a.listarTx(ctx, q, modelo, false, "")
	if err != nil {
		return api.ERPRegistro{}, false, err
	}
	for i := len(regs) - 1; i >= 0; i-- {
		if s, _ := regs[i].Data[campo].(string); s == valor {
			return regs[i], true, nil
		}
	}
	return api.ERPRegistro{}, false, nil
}

// ImportarN43 guarda las cuentas de un extracto y los movimientos que aún no estaban.
// Por cuenta, devuelve los ids de los nuevos por su índice en `c.Movimientos`.
func (a *Almacen) ImportarN43(ctx context.Context, c banca.Cuenta) (map[int]string, error) {
	nuevos := map[int]string{}
	mc, _ := a.cat.Modelo("cuenta_contable")
	me, _ := a.cat.Modelo("extracto_bancario")
	ml, _ := a.cat.Modelo("linea_extracto")
	if mc == nil || me == nil || ml == nil {
		return nil, ErrNoEncontrado
	}
	huellas := banca.Huellas(c)
	err := a.tx(ctx, func(tx *sql.Tx) error {
		cuenta, err := a.cuentaN43(ctx, tx, mc, c)
		if err != nil {
			return err
		}
		extracto, err := a.extractoN43(ctx, tx, me, c, cuenta.ID)
		if err != nil {
			return err
		}
		// Lo que ya estaba no se toca aunque se haya borrado: un movimiento borrado a
		// propósito no vuelve con la importación siguiente.
		vistos, err := huellasDeLineas(ctx, tx)
		if err != nil {
			return err
		}
		for i, m := range c.Movimientos {
			if vistos[huellas[i]] {
				continue
			}
			data := map[string]any{
				"extracto": extracto.ID, "fecha": m.Fecha.Format(time.DateOnly),
				"descripcion": m.Texto(), "importe": m.Centimos, "externo_id": huellas[i],
			}
			l, err := a.insertarEn(ctx, tx, ml, data, m.Texto(), "pendiente")
			if err != nil {
				return err
			}
			vistos[huellas[i]] = true
			nuevos[i] = l.ID
		}
		return nil
	})
	return nuevos, err
}

// cuentaN43 es la cuenta contable del banco del extracto; la primera vez se crea, con el
// siguiente código libre de la 572.
func (a *Almacen) cuentaN43(ctx context.Context, tx *sql.Tx, mc *catalogo.Modelo, c banca.Cuenta) (api.ERPRegistro, error) {
	cuenta, ok, err := a.buscarPor(ctx, tx, "cuenta_contable", "externo_id", c.Clave())
	if err != nil || ok {
		return cuenta, err
	}
	todas, err := a.listarTx(ctx, tx, "cuenta_contable", false, "")
	if err != nil {
		return api.ERPRegistro{}, err
	}
	n := 0
	for _, x := range todas {
		if s, _ := x.Data["externo_id"].(string); strings.HasPrefix(s, "n43:") {
			n++
		}
	}
	data := map[string]any{"codigo": fmt.Sprintf("572%03d", n+1), "nombre": c.Nombre(), "tipo": "banco", "externo_id": c.Clave()}
	return a.insertarEn(ctx, tx, mc, data, c.Nombre(), "")
}

// extractoN43 guarda el extracto de la cuenta para su período; si ya estaba, con los
// saldos de ahora.
func (a *Almacen) extractoN43(ctx context.Context, tx *sql.Tx, me *catalogo.Modelo, c banca.Cuenta, cuenta string) (api.ERPRegistro, error) {
	externo := c.Clave() + ":" + c.Desde.Format("20060102") + "-" + c.Hasta.Format("20060102")
	dataExt := map[string]any{
		"cuenta": cuenta, "fecha": c.Hasta.Format(time.DateOnly),
		"saldo_inicial": c.SaldoInicial, "saldo_final": c.SaldoFinal, "externo_id": externo,
	}
	extracto, ok, err := a.buscarPor(ctx, tx, "extracto_bancario", "externo_id", externo)
	if err != nil {
		return api.ERPRegistro{}, err
	}
	if !ok {
		return a.insertarEn(ctx, tx, me, dataExt, "", "confirmado")
	}
	crudo, err := json.Marshal(dataExt)
	if err != nil {
		return api.ERPRegistro{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE registro SET data = ?, editado = ? WHERE id = ?`, string(crudo), a.marcaAhora(), extracto.ID)
	return extracto, err
}

// huellasDeLineas son las huellas (`externo_id`) de todas las líneas de extracto,
// también las borradas. Si la lectura se corta a medias es un error: con el mapa
// incompleto, la importación volvería a meter movimientos que ya estaban.
func huellasDeLineas(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	filas, err := tx.QueryContext(ctx, `SELECT json_extract(data, '$.externo_id') FROM registro WHERE modelo = 'linea_extracto'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	vistos := map[string]bool{}
	for filas.Next() {
		var x sql.NullString
		if err := filas.Scan(&x); err != nil {
			return nil, err
		}
		vistos[x.String] = true
	}
	return vistos, filas.Err()
}

// FacturasAbiertas son las facturas emitidas y recibidas sin cobrar ni pagar, las más
// recientes primero (las 2.000 últimas, como en el servidor).
func (a *Almacen) FacturasAbiertas(ctx context.Context) ([]FacturaAbierta, error) {
	var out []FacturaAbierta
	for _, modelo := range []string{"factura_venta", "factura_compra"} {
		regs, err := a.listarTx(ctx, a.db, modelo, false, "")
		if err != nil {
			return nil, err
		}
		for _, r := range regs {
			if r.Estado != "borrador" && r.Estado != "confirmado" {
				continue
			}
			f := FacturaAbierta{ID: r.ID, Modelo: r.Modelo, Ref: r.Ref, Estado: r.Estado}
			f.Fecha, _ = r.Data["fecha"].(string)
			f.Vencimiento, _ = r.Data["vencimiento"].(string)
			f.Total = cmp.Or(entero(r.Data["total"]), entero(r.Data["base"])+entero(r.Data["iva"]))
			tercero, _ := r.Data["cliente"].(string)
			if tercero == "" {
				tercero, _ = r.Data["proveedor"].(string)
			}
			if tercero != "" {
				if c, err := obtener(ctx, a.db, "contacto", tercero); err == nil {
					f.Tercero = c.Etiqueta
					f.NIF, _ = c.Data["nif"].(string)
				}
			}
			out = append(out, f)
		}
	}
	slices.SortStableFunc(out, func(x, y FacturaAbierta) int { return strings.Compare(y.Fecha, x.Fecha) })
	return out[:min(len(out), 2000)], nil
}

// LineaBanco es un movimiento tal como lo ve la lista de tareas (`store.LineaBanco`).
type LineaBanco struct {
	ID            string `json:"id"`
	Fecha         string `json:"fecha"`
	Descripcion   string `json:"descripcion"`
	Centimos      int64  `json:"importe"`
	FacturaID     string `json:"factura_id,omitempty"`
	FacturaModelo string `json:"factura_modelo,omitempty"`
	FacturaRef    string `json:"factura_ref,omitempty"`
	Motivo        string `json:"motivo,omitempty"`
	Empresa       string `json:"empresa,omitempty"`
	Web           string `json:"web,omitempty"`
	URLFactura    string `json:"url_factura,omitempty"`
	EmailFacturas string `json:"email_facturas,omitempty"`
}

func lineaBancoDe(r api.ERPRegistro) LineaBanco {
	s := func(k string) string { v, _ := r.Data[k].(string); return v }
	return LineaBanco{
		ID: r.ID, Fecha: s("fecha"), Descripcion: s("descripcion"), Centimos: entero(r.Data["importe"]),
		Motivo: s("motivo"), Empresa: s("empresa"), Web: s("web"), URLFactura: s("url_factura"), EmailFacturas: s("email_facturas"),
	}
}

// LineasBanco son los movimientos en un estado: los pendientes del más antiguo al más
// reciente; los justificados, lo último primero. El total cuenta todos.
func (a *Almacen) LineasBanco(ctx context.Context, estado string, limite int) ([]LineaBanco, int, error) {
	regs, err := a.listarTx(ctx, a.db, "linea_extracto", false, estado)
	if err != nil {
		return nil, 0, err
	}
	if estado == "pendiente" {
		slices.Reverse(regs)
		slices.SortStableFunc(regs, func(x, y api.ERPRegistro) int {
			fx, _ := x.Data["fecha"].(string)
			fy, _ := y.Data["fecha"].(string)
			return strings.Compare(fx, fy)
		})
	} else {
		slices.SortStableFunc(regs, func(x, y api.ERPRegistro) int { return y.EditadoEn.Compare(x.EditadoEn) })
	}
	out := []LineaBanco{}
	for _, r := range regs[:min(len(regs), limite)] {
		l := lineaBancoDe(r)
		for _, modelo := range []string{"factura_venta", "factura_compra"} {
			if id, _ := r.Data[modelo].(string); id != "" {
				l.FacturaID, l.FacturaModelo = id, modelo
				if f, err := obtener(ctx, a.db, modelo, id); err == nil {
					l.FacturaRef = f.Ref
				}
			}
		}
		out = append(out, l)
	}
	return out, len(regs), nil
}

// Liquidacion es lo que justifica un movimiento (`store.Liquidacion`).
type Liquidacion struct {
	LineaID                  string
	FacturaID, ModeloFactura string
	Motivo                   string
	Contrapartida            string
	Concepto                 string
}

// Liquidar asienta un movimiento y lo da por justificado: el asiento del banco contra
// la contrapartida, la factura pagada y la línea conciliada, en una transacción.
func (a *Almacen) Liquidar(ctx context.Context, l Liquidacion) (Asiento, error) {
	var out Asiento
	err := a.tx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = a.liquidarEn(ctx, tx, l)
		return err
	})
	return out, err
}

// liquidarEn es Liquidar dentro de su transacción.
func (a *Almacen) liquidarEn(ctx context.Context, tx *sql.Tx, l Liquidacion) (Asiento, error) {
	linea, err := obtener(ctx, tx, "linea_extracto", l.LineaID)
	if err != nil {
		return Asiento{}, err
	}
	if linea.Estado != "pendiente" {
		return Asiento{}, ErrYaJustificado
	}
	fechaTxt, _ := linea.Data["fecha"].(string)
	importe := entero(linea.Data["importe"])
	fecha, err := time.Parse(time.DateOnly, fechaTxt)
	if err != nil || importe == 0 {
		return Asiento{}, fmt.Errorf("movimiento sin fecha o sin importe: %w", dominio.ErrDatos)
	}
	cuentaBanco := cuentaBancoDe(ctx, tx, linea)

	explica := map[string]any{"motivo": l.Motivo}
	tercero := ""
	if l.FacturaID != "" {
		if tercero, err = terceroDeFactura(ctx, tx, l); err != nil {
			return Asiento{}, err
		}
		explica = map[string]any{l.ModeloFactura: l.FacturaID}
	}
	centimos := max(importe, -importe)
	banco := Apunte{Cuenta: cuentaBanco, Concepto: l.Concepto}
	otra := Apunte{Cuenta: l.Contrapartida, Concepto: l.Concepto, Tercero: tercero}
	if importe > 0 {
		banco.Debe, otra.Haber = centimos, centimos
	} else {
		banco.Haber, otra.Debe = centimos, centimos
	}
	out, err := a.escribirAsiento(ctx, tx, "banco", fecha, l.Concepto, dominio.MonedaPorDefecto, l.LineaID, []Apunte{banco, otra})
	if err != nil {
		return out, err
	}
	if l.FacturaID != "" {
		if err := a.ponerEstado(ctx, tx, l.ModeloFactura, l.FacturaID, "pagado"); err != nil {
			return out, err
		}
	}
	datos := linea.Data
	delete(datos, "motivo")
	for k, v := range explica {
		if v != "" {
			datos[k] = v
		}
	}
	return out, a.guardarDatos(ctx, tx, "linea_extracto", l.LineaID, "conciliado", datos)
}

// cuentaBancoDe es el código de la cuenta contable del banco de una línea de extracto
// (la de la cuenta de su extracto) o, si no se sabe, la de por defecto.
func cuentaBancoDe(ctx context.Context, tx *sql.Tx, linea api.ERPRegistro) string {
	cuentaBanco := ""
	if ext, _ := linea.Data["extracto"].(string); ext != "" {
		if e, err := obtener(ctx, tx, "extracto_bancario", ext); err == nil {
			if c, _ := e.Data["cuenta"].(string); c != "" {
				if cc, err := obtener(ctx, tx, "cuenta_contable", c); err == nil {
					cuentaBanco, _ = cc.Data["codigo"].(string)
				}
			}
		}
	}
	return cmp.Or(cuentaBanco, cuentaBancoPorDefecto)
}

// terceroDeFactura comprueba que la factura que justifica el movimiento sigue abierta y
// dice su tercero: el cliente o el proveedor.
func terceroDeFactura(ctx context.Context, tx *sql.Tx, l Liquidacion) (string, error) {
	f, err := obtener(ctx, tx, l.ModeloFactura, l.FacturaID)
	if err != nil {
		return "", err
	}
	if f.Estado != "confirmado" {
		return "", ErrFacturaNoAbierta
	}
	campo := "cliente"
	if l.ModeloFactura == "factura_compra" {
		campo = "proveedor"
	}
	tercero, _ := f.Data[campo].(string)
	return tercero, nil
}

// ReabrirMovimiento deshace la justificación: anula su asiento, devuelve la factura a
// confirmada y la línea vuelve a ser una tarea.
func (a *Almacen) ReabrirMovimiento(ctx context.Context, id string) error {
	return a.tx(ctx, func(tx *sql.Tx) error {
		linea, err := obtener(ctx, tx, "linea_extracto", id)
		if err != nil {
			return err
		}
		if linea.Estado != "conciliado" {
			return ErrNoJustificado
		}
		fechas, err := fechasAsentadas(ctx, tx, id)
		if err != nil {
			return err
		}
		for _, f := range fechas {
			d, _ := time.Parse(time.DateOnly, f)
			if err := exigirPeriodoAbierto(ctx, tx, d); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE asiento SET estado = 'anulado', concepto = concepto || ' · reabierto' WHERE origen = ? AND estado = 'asentado'`, id); err != nil {
			return err
		}
		for _, modelo := range []string{"factura_venta", "factura_compra"} {
			if f, _ := linea.Data[modelo].(string); f != "" {
				if r, err := obtener(ctx, tx, modelo, f); err == nil && r.Estado == "pagado" {
					if err := a.ponerEstado(ctx, tx, modelo, f, "confirmado"); err != nil {
						return err
					}
				}
			}
		}
		datos := linea.Data
		delete(datos, "factura_venta")
		delete(datos, "factura_compra")
		delete(datos, "motivo")
		return a.guardarDatos(ctx, tx, "linea_extracto", id, "pendiente", datos)
	})
}

// fechasAsentadas son las fechas de los asientos vigentes que salieron de origen. Si la
// lectura se corta a medias es un error: con la lista incompleta se anularía un asiento
// de un período cerrado sin comprobarlo.
func fechasAsentadas(ctx context.Context, tx *sql.Tx, origen string) ([]string, error) {
	filas, err := tx.QueryContext(ctx, `SELECT fecha FROM asiento WHERE origen = ? AND estado = 'asentado'`, origen)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	var fechas []string
	for filas.Next() {
		var f string
		if err := filas.Scan(&f); err != nil {
			return nil, err
		}
		fechas = append(fechas, f)
	}
	return fechas, filas.Err()
}

// ponerEstado cambia el estado SIN las comprobaciones de emitir (es lo que hace el banco
// al dar una factura por pagada), con su entrada en el historial.
func (a *Almacen) ponerEstado(ctx context.Context, tx *sql.Tx, modelo, id, estado string) error {
	previo, err := obtener(ctx, tx, modelo, id)
	if err != nil || previo.Estado == estado {
		return err
	}
	ahora := a.marcaAhora()
	if _, err := tx.ExecContext(ctx, `UPDATE registro SET estado = ?, editado = ? WHERE id = ?`, estado, ahora, id); err != nil {
		return err
	}
	return anotar(ctx, tx, modelo, id, "cambio", map[string]api.ERPCambio{"estado": {Antes: previo.Estado, Despues: estado}}, "", ahora)
}

// guardarDatos reescribe los datos y el estado de un registro (lo que el servidor hace
// con `data || …`).
func (a *Almacen) guardarDatos(ctx context.Context, tx *sql.Tx, modelo, id, estado string, datos map[string]any) error {
	crudo, err := json.Marshal(datos)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE registro SET estado = ?, data = ?, editado = ? WHERE id = ? AND modelo = ?`,
		estado, string(crudo), a.marcaAhora(), id, modelo)
	return err
}

// --- Rutas ------------------------------------------------------------------------

func (s *Servidor) rutasBanco(h func(string, http.HandlerFunc)) {
	v := "/api/v1"
	h("GET "+v+"/banco", s.banco)
	h("POST "+v+"/banco/{id}/conciliar", s.conciliarMovimiento)
	h("POST "+v+"/banco/{id}/explicar", s.explicarMovimiento)
	h("POST "+v+"/banco/{id}/reabrir", s.reabrirMovimiento)
}

// proponer compara un movimiento con las facturas abiertas de su sentido.
func proponer(m banca.Movimiento, abiertas []FacturaAbierta) (banca.Propuesta, map[string]FacturaAbierta) {
	modelo := "factura_venta"
	if m.Centimos < 0 {
		modelo = "factura_compra"
	}
	porID := map[string]FacturaAbierta{}
	var pendientes []banca.Pendiente
	for _, f := range abiertas {
		if f.Modelo != modelo || f.Total <= 0 {
			continue
		}
		fecha, _ := time.Parse(time.DateOnly, f.Fecha)
		venc, _ := time.Parse(time.DateOnly, f.Vencimiento)
		porID[f.ID] = f
		pendientes = append(pendientes, banca.Pendiente{
			ID: f.ID, Ref: f.Ref, Tercero: f.Tercero, NIF: f.NIF,
			Fecha: fecha, Vencimiento: venc, Centimos: f.Total,
		})
	}
	return banca.Emparejar(m, pendientes), porID
}

// facturaSegura es la factura de un movimiento solo si no hay duda Y ya está
// confirmada: casar con un borrador lo confirmaría, y eso lo decide una persona.
func facturaSegura(m banca.Movimiento, abiertas []FacturaAbierta) (FacturaAbierta, bool) {
	p, porID := proponer(m, abiertas)
	if p.Automatica == nil {
		return FacturaAbierta{}, false
	}
	f := porID[p.Automatica.ID]
	return f, f.Estado == "confirmado"
}

func (s *Servidor) esPersonaFisica(ctx context.Context) bool {
	o, err := s.alm.Organizacion(ctx)
	if err != nil {
		return false
	}
	pais, ok := s.obl.Pais(o.Pais)
	if !ok {
		return false
	}
	f, ok := pais.Forma(o.Forma)
	return ok && f.Tipo == obligaciones.TipoPersonaFisica
}

// conciliar casa un movimiento con una factura; si está en borrador, primero se asienta.
func (s *Servidor) conciliar(ctx context.Context, lineaID string, f FacturaAbierta) (Asiento, error) {
	m, ok := s.cat.Modelo(f.Modelo)
	if !ok || m.Contable == nil {
		return Asiento{}, ErrNoEncontrado
	}
	if f.Estado == "borrador" {
		if _, err := s.alm.Asentar(ctx, m, f.ID); err != nil {
			return Asiento{}, err
		}
	}
	concepto := "Cobro " + f.Ref
	if !m.Contable.Cobro {
		concepto = "Pago " + f.Ref
	}
	return s.alm.Liquidar(ctx, Liquidacion{
		LineaID: lineaID, FacturaID: f.ID, ModeloFactura: f.Modelo,
		Contrapartida: m.Contable.CuentaContrapartida, Concepto: concepto,
	})
}

// explicar justifica un movimiento sin factura con su motivo.
func (s *Servidor) explicar(ctx context.Context, lineaID, motivo string, ingreso, personaFisica bool, texto, idioma string) (Asiento, error) {
	cuenta, ok := banca.CuentaDeMotivo(motivo, ingreso, personaFisica)
	if !ok {
		return Asiento{}, errMotivo
	}
	return s.alm.Liquidar(ctx, Liquidacion{
		LineaID: lineaID, Motivo: motivo, Contrapartida: cuenta,
		Concepto: validation.TruncateRunes(s.cat.Rotulo("valor."+motivo, idioma)+" · "+texto, 200),
	})
}

// importarN43 mete un extracto y hace a la vez lo que se puede sin preguntar: casa
// cada movimiento con su factura cuando no hay duda y justifica los que se explican
// solos. Lo demás queda como tarea.
func (s *Servidor) importarN43(w http.ResponseWriter, r *http.Request, cuerpo []byte) {
	cuentas, err := banca.LeerN43(bytes.NewReader(cuerpo))
	if err != nil {
		fallo(w, http.StatusUnprocessableEntity, "unprocessable", err.Error(), nil)
		return
	}
	ctx := r.Context()
	personaFisica := s.esPersonaFisica(ctx)
	abiertas, err := s.alm.FacturasAbiertas(ctx)
	if err != nil {
		falloDe(w, err)
		return
	}
	l := idioma(r)
	res := api.ERPResultadoExtracto{Cuentas: []string{}}
	for _, c := range cuentas {
		nuevos, err := s.alm.ImportarN43(ctx, c)
		if err != nil {
			falloDe(w, err)
			return
		}
		res.Cuentas = append(res.Cuentas, c.Nombre())
		res.Repetidos += len(c.Movimientos) - len(nuevos)
		for i, m := range c.Movimientos {
			id, nuevo := nuevos[i]
			if !nuevo {
				continue
			}
			res.Nuevos++
			if f, ok := facturaSegura(m, abiertas); ok {
				if _, err := s.conciliar(ctx, id, f); err == nil {
					res.Casados++
					abiertas = slices.DeleteFunc(abiertas, func(x FacturaAbierta) bool { return x.ID == f.ID })
					continue
				}
			}
			if motivo := banca.MotivoEvidente(m); motivo != "" {
				if _, err := s.explicar(ctx, id, motivo, m.Centimos > 0, personaFisica, m.Texto(), l); err == nil {
					res.Explicados++
					continue
				}
			}
			res.Pendientes++
		}
	}
	responder(w, http.StatusOK, res)
}

func (s *Servidor) banco(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pendientes, total, err := s.alm.LineasBanco(ctx, "pendiente", 200)
	if err != nil {
		falloDe(w, err)
		return
	}
	hechas, _, err := s.alm.LineasBanco(ctx, "conciliado", 20)
	if err != nil {
		falloDe(w, err)
		return
	}
	abiertas, err := s.alm.FacturasAbiertas(ctx)
	if err != nil {
		falloDe(w, err)
		return
	}
	type candidata struct {
		FacturaAbierta
		Segura bool `json:"segura"`
	}
	type tarea struct {
		LineaBanco
		Sugerencia string      `json:"sugerencia,omitempty"`
		Candidatas []candidata `json:"candidatas"`
	}
	tareas := make([]tarea, 0, len(pendientes))
	for _, l := range pendientes {
		fecha, _ := time.Parse(time.DateOnly, l.Fecha)
		m := banca.Movimiento{Fecha: fecha, Centimos: l.Centimos, Descripcion: l.Descripcion}
		p, porID := proponer(m, abiertas)
		t := tarea{LineaBanco: l, Sugerencia: banca.MotivoProbable(m), Candidatas: []candidata{}}
		for _, c := range p.Candidatas {
			t.Candidatas = append(t.Candidatas, candidata{porID[c.ID], p.Automatica != nil && p.Automatica.ID == c.ID})
		}
		tareas = append(tareas, t)
	}
	l := idioma(r)
	motivos := make([]api.ERPMotivoBanco, 0, len(banca.Motivos))
	for _, m := range banca.Motivos {
		motivos = append(motivos, api.ERPMotivoBanco{Clave: m, Rotulo: s.cat.Rotulo("valor."+m, l)})
	}
	responder(w, http.StatusOK, map[string]any{"pendientes": tareas, "total_pendientes": total, "hechas": hechas, "motivos": motivos})
}

func (s *Servidor) conciliarMovimiento(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Factura string `json:"factura"`
	}
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	if err := validation.New().Required("factura", c.Factura).Err(); err != nil {
		falloValidacion(w, err)
		return
	}
	ctx := r.Context()
	linea, err := s.alm.Obtener(ctx, "linea_extracto", r.PathValue("id"))
	if err != nil {
		falloDe(w, err)
		return
	}
	var factura api.ERPRegistro
	for _, modelo := range []string{"factura_venta", "factura_compra"} {
		if factura, err = s.alm.Obtener(ctx, modelo, c.Factura); err == nil {
			break
		}
	}
	if err != nil {
		falloDe(w, err)
		return
	}
	if (factura.Modelo == "factura_venta") != (entero(linea.Data["importe"]) > 0) {
		falloDe(w, errSentidoFactura)
		return
	}
	if factura.Estado != "borrador" && factura.Estado != "confirmado" {
		falloDe(w, ErrFacturaNoAbierta)
		return
	}
	asiento, err := s.conciliar(ctx, linea.ID, FacturaAbierta{ID: factura.ID, Modelo: factura.Modelo, Ref: factura.Ref, Estado: factura.Estado})
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, asiento)
}

func (s *Servidor) explicarMovimiento(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Motivo string `json:"motivo"`
	}
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	ctx := r.Context()
	linea, err := s.alm.Obtener(ctx, "linea_extracto", r.PathValue("id"))
	if err != nil {
		falloDe(w, err)
		return
	}
	texto, _ := linea.Data["descripcion"].(string)
	asiento, err := s.explicar(ctx, linea.ID, c.Motivo, entero(linea.Data["importe"]) > 0, s.esPersonaFisica(ctx), texto, idioma(r))
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, asiento)
}

func (s *Servidor) reabrirMovimiento(w http.ResponseWriter, r *http.Request) {
	if err := s.alm.ReabrirMovimiento(r.Context(), r.PathValue("id")); err != nil {
		falloDe(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
