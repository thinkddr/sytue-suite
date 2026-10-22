// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

type LineaTPV struct {
	Producto string `json:"producto"`
	Cantidad int64  `json:"cantidad"`
	// Precio esperado: si cambió el catálogo, se pide revisar la cesta.
	Precio  int64  `json:"precio"`
	TipoIVA string `json:"tipo_iva"`
}
type CobroTPV struct {
	Operacion string     `json:"operacion"`
	Sesion    string     `json:"sesion"`
	Cliente   string     `json:"cliente"`
	Medio     string     `json:"medio"`
	Recibido  int64      `json:"recibido"`
	Lineas    []LineaTPV `json:"lineas"`
}
type VentaTPV struct {
	Factura Registro `json:"factura"`
	Cambio  int64    `json:"cambio"`
}

func (c CobroTPV) Validar() error {
	if uuid.Validate(c.Operacion) != nil || uuid.Validate(c.Sesion) != nil || (c.Cliente != "" && uuid.Validate(c.Cliente) != nil) || (c.Medio != "efectivo" && c.Medio != "tarjeta") || c.Recibido < 0 || c.Recibido > 1000000000 || len(c.Lineas) < 1 || len(c.Lineas) > 100 {
		return fmt.Errorf("cobro de TPV inválido: %w", domain.ErrDatos)
	}
	vistos := map[string]bool{}
	for _, l := range c.Lineas {
		if uuid.Validate(l.Producto) != nil || vistos[l.Producto] || l.Cantidad < 1 || l.Cantidad > 1000 || l.Precio < 0 || l.Precio > 1000000 || !slices.Contains([]string{"21", "10", "5", "4", "0"}, l.TipoIVA) {
			return fmt.Errorf("línea de TPV inválida: %w", domain.ErrDatos)
		}
		vistos[l.Producto] = true
	}
	return nil
}

// CobrarTPV registra un cobro ya realizado físicamente. No carga una tarjeta:
// el terminal bancario cobra antes, y aquí se registra su medio e importe.
// Factura, líneas, asiento, cobro, sesión y VERI*FACTU se confirman juntos.
func (s *Store) CobrarTPV(ctx context.Context, cat *catalogo.Catalogo, c CobroTPV, por string) (VentaTPV, error) {
	var out VentaTPV
	if err := c.Validar(); err != nil {
		return out, err
	}
	crudo, err := json.Marshal(c)
	if err != nil {
		return out, err
	}
	hash := sha256.Sum256(crudo)
	huella := hex.EncodeToString(hash[:])
	err = s.escritura(ctx, func(tx pgx.Tx) error {
		// Serializa reintentos incluso antes de que exista su factura.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_setting('app.tenant', true) || ':tpv', 0))`); err != nil {
			return err
		}
		recuperada, existe, err := recuperarCobroTPV(ctx, tx, c, huella)
		if err != nil {
			return err
		}
		if existe {
			out = recuperada
			return nil
		}
		preparado, err := prepararCobroTPV(ctx, tx, cat, c, por)
		if err != nil {
			return err
		}
		factura, err := crearFacturaTPV(ctx, tx, cat, c, preparado, huella, por)
		if err != nil {
			return err
		}
		out, err = s.registrarCobroTPV(ctx, tx, cat, c, preparado, factura, por)
		return err
	})
	return out, traducirError(err)
}

// El reintento se resuelve antes de comprobar módulos o caja: una venta
// confirmada conserva su respuesta aunque se cierre la sesión después.
func recuperarCobroTPV(ctx context.Context, tx pgx.Tx, c CobroTPV, huella string) (VentaTPV, bool, error) {
	var out VentaTPV
	err := escanearRegistro(tx.QueryRow(ctx, `SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at FROM records WHERE tenant = current_setting('app.tenant', true) AND model = 'factura_venta' AND data->>'tpv_operacion' = $1`, c.Operacion), &out.Factura)
	if errors.Is(err, ErrNoEncontrado) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	if cadenaDe(out.Factura.Data["tpv_huella"]) != huella {
		return out, false, fmt.Errorf("la operación ya se usó para otra cesta: %w", domain.ErrDatos)
	}
	total, err := domain.Centimos(out.Factura.Data, "total")
	if err != nil {
		return out, false, err
	}
	if c.Medio == "efectivo" {
		out.Cambio = c.Recibido - total
	}
	return out, true, nil
}

type cobroTPVPreparado struct {
	Sesion  Registro
	Lineas  []domain.LineaFactura
	Cliente string
	Tipo    string
	Base    int64
	IVA     int64
	Total   int64
	Ahora   time.Time
}

// Los productos y la sesión se leen bajo sus bloqueos originales, dentro de
// la transacción de cobro, y siempre después de resolver la idempotencia.
func prepararCobroTPV(ctx context.Context, tx pgx.Tx, cat *catalogo.Catalogo, c CobroTPV, por string) (cobroTPVPreparado, error) {
	var preparado cobroTPVPreparado
	if err := exigirModulosVenta(ctx, tx, "tpv", "productos", "contactos", "facturacion"); err != nil {
		return preparado, err
	}
	if err := exigirTerritorioTPV(ctx, tx); err != nil {
		return preparado, err
	}
	if err := escanearRegistro(tx.QueryRow(ctx, `SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at FROM records WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = 'sesion_tpv' AND deleted_at IS NULL FOR UPDATE`, c.Sesion), &preparado.Sesion); err != nil {
		return preparado, err
	}
	if preparado.Sesion.Estado != "abierto" {
		return preparado, fmt.Errorf("la caja está cerrada: %w", domain.ErrDatos)
	}
	lineas, err := lineasTPV(ctx, tx, c.Lineas)
	if err != nil {
		return preparado, err
	}
	preparado.Lineas = lineas
	for _, g := range domain.Desglose(lineas) {
		preparado.Base += g.Base
		preparado.IVA += g.Cuota
	}
	preparado.Total = preparado.Base + preparado.IVA
	if err := validarImporteCobroTPV(c, preparado.Total); err != nil {
		return preparado, err
	}
	preparado.Cliente, preparado.Tipo, err = clienteCobroTPV(ctx, tx, cat, c.Cliente, preparado.Total, por)
	if err != nil {
		return preparado, err
	}
	zona, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		return preparado, err
	}
	preparado.Ahora = time.Now().In(zona)
	return preparado, nil
}

func exigirTerritorioTPV(ctx context.Context, tx pgx.Tx) error {
	var pais, cp string
	if err := tx.QueryRow(ctx, `SELECT pais, cp FROM organizacion WHERE tenant = current_setting('app.tenant', true)`).Scan(&pais, &cp); err != nil {
		return err
	}
	if pais != "ES" || (len(cp) >= 2 && slices.Contains([]string{"35", "38", "51", "52"}, cp[:2])) {
		return fmt.Errorf("este TPV utiliza IVA de España peninsular y Baleares: %w", domain.ErrDatos)
	}
	return nil
}

func validarImporteCobroTPV(c CobroTPV, total int64) error {
	if total <= 0 || total > 1000000000 || (c.Medio == "efectivo" && c.Recibido < total) {
		return fmt.Errorf("el importe recibido no cubre la venta: %w", domain.ErrDatos)
	}
	// Ley 7/2012, artículo 7: el límite incluye exactamente 1.000 €.
	// No se presume la excepción de particulares no residentes: el TPV no
	// recoge todavía la documentación necesaria para justificararla.
	if c.Medio == "efectivo" && total >= 100000 {
		return fmt.Errorf("una venta de 1.000 € o más no se puede cobrar en efectivo; utiliza tarjeta: %w", domain.ErrDatos)
	}
	return nil
}

func clienteCobroTPV(ctx context.Context, tx pgx.Tx, cat *catalogo.Catalogo, cliente string, total int64, por string) (string, string, error) {
	if cliente != "" {
		var nif string
		if err := tx.QueryRow(ctx, `SELECT COALESCE(data->>'nif','') FROM records WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = 'contacto' AND deleted_at IS NULL`, cliente).Scan(&nif); err != nil {
			return "", "", err
		}
		if !validation.NIF(nif) {
			return "", "", fmt.Errorf("el cliente necesita un NIF válido: %w", domain.ErrDatos)
		}
		return cliente, "F1", nil
	}
	// Tope general, sin presumir una actividad minorista de la organización.
	if total > 40000 {
		return "", "", fmt.Errorf("por encima de 400 € identifica al cliente para emitir factura completa: %w", domain.ErrDatos)
	}
	err := tx.QueryRow(ctx, `SELECT id::text FROM records WHERE tenant = current_setting('app.tenant', true) AND model = 'contacto' AND data->>'externo_id' = 'tpv:publico' AND deleted_at IS NULL LIMIT 1`).Scan(&cliente)
	if err == nil {
		return cliente, "F2", nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}
	m, _ := cat.Modelo("contacto")
	r, err := insertarNuevo(ctx, tx, Nuevo{Modelo: m, Data: map[string]any{"nombre": "TPV", "tipo": "organizacion", "externo_id": "tpv:publico"}, Etiqueta: "TPV"}, nil, por)
	if err != nil {
		return "", "", err
	}
	return r.ID, "F2", nil
}

func crearFacturaTPV(ctx context.Context, tx pgx.Tx, cat *catalogo.Catalogo, c CobroTPV, preparado cobroTPVPreparado, huella, por string) (Registro, error) {
	m, _ := cat.Modelo("factura_venta")
	factura, err := insertarNuevo(ctx, tx, Nuevo{desdeTPV: true, Modelo: m, Data: map[string]any{"cliente": preparado.Cliente, "fecha": preparado.Ahora.Format("2006-01-02"), "base": preparado.Base, "iva": preparado.IVA, "total": preparado.Total, "tipo_factura": preparado.Tipo, "regimen_iva": "iva_general", "tpv_operacion": c.Operacion, "sesion": c.Sesion, "medio_pago": c.Medio, "tpv_huella": huella}}, nil, por)
	if err != nil {
		return Registro{}, err
	}
	lm, _ := cat.Modelo("linea_factura")
	for _, l := range preparado.Lineas {
		if _, err := insertarNuevo(ctx, tx, Nuevo{Modelo: lm, Etiqueta: l.Descripcion, Data: map[string]any{"factura": factura.ID, "descripcion": l.Descripcion, "cantidad": l.Cantidad, "precio_con_iva": l.Precio, "tipo_iva": strconv.FormatInt(l.Tipo, 10)}}, nil, por); err != nil {
			return Registro{}, err
		}
	}
	return factura, nil
}

func (s *Store) registrarCobroTPV(ctx context.Context, tx pgx.Tx, cat *catalogo.Catalogo, c CobroTPV, preparado cobroTPVPreparado, factura Registro, por string) (VentaTPV, error) {
	var out VentaTPV
	m, _ := cat.Modelo("factura_venta")
	var asiento Asiento
	if err := s.asentarEn(ctx, tx, m, factura.ID, por, &asiento); err != nil {
		return out, err
	}
	cuenta, diario := "572000", "bancos"
	if c.Medio == "efectivo" {
		cuenta, diario = "570000", "caja"
		out.Cambio = c.Recibido - preparado.Total
	}
	if _, err := escribirAsiento(ctx, tx, diario, preparado.Ahora, "TPV "+factura.Ref, "", por, []Apunte{{Cuenta: cuenta, Debe: preparado.Total}, {Cuenta: "430000", Haber: preparado.Total, Tercero: preparado.Cliente}}); err != nil {
		return out, err
	}
	if _, err := tx.Exec(ctx, `UPDATE records SET estado = 'pagado', updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, factura.ID); err != nil {
		return out, err
	}
	if err := acumularSesionTPV(ctx, tx, preparado.Sesion, c.Medio, preparado.Total); err != nil {
		return out, err
	}
	factura.Estado = "pagado"
	out.Factura = factura
	return out, nil
}

func acumularSesionTPV(ctx context.Context, tx pgx.Tx, sesion Registro, medio string, total int64) error {
	for _, campo := range []string{medio, "total"} {
		previo, err := domain.Centimos(sesion.Data, campo)
		if err != nil {
			return err
		}
		sesion.Data[campo] = previo + total
	}
	data, err := json.Marshal(sesion.Data)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE records SET data = $2, updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, sesion.ID, data)
	return err
}

func lineasTPV(ctx context.Context, tx pgx.Tx, entradas []LineaTPV) ([]domain.LineaFactura, error) {
	lineas := make([]domain.LineaFactura, 0, len(entradas))
	for _, l := range entradas {
		var producto Registro
		if err := escanearRegistro(tx.QueryRow(ctx, `SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at FROM records WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = 'producto' AND deleted_at IS NULL FOR SHARE`, l.Producto), &producto); err != nil {
			return nil, err
		}
		precio, err := domain.Centimos(producto.Data, "precio_con_iva")
		if err != nil || producto.Data["precio_con_iva"] == nil || precio != l.Precio || cadenaDe(producto.Data["tipo_iva"]) != l.TipoIVA || producto.Estado == "pausada" {
			return nil, fmt.Errorf("revisa el precio y el IVA de %s: %w", producto.Etiqueta, domain.ErrDatos)
		}
		tipo, err := strconv.ParseInt(l.TipoIVA, 10, 64)
		if err != nil {
			return nil, err
		}
		if tipo == 0 {
			return nil, fmt.Errorf("el TPV solo vende productos sujetos a IVA; las exenciones se facturan desde facturación: %w", domain.ErrDatos)
		}
		lineas = append(lineas, domain.LineaFactura{Descripcion: producto.Etiqueta, Cantidad: strconv.FormatInt(l.Cantidad, 10), Precio: precio, Tipo: tipo, Importe: precio * l.Cantidad})
	}
	return lineas, nil
}

// PresupuestoTPV prepara una oferta con detalle fiscal real, sin cobrar ni
// emitir una factura. Reutiliza los precios y validaciones de la cesta.
func (s *Store) PresupuestoTPV(ctx context.Context, cat *catalogo.Catalogo, c CobroTPV, por string) (Registro, error) {
	var out Registro
	if c.Cliente == "" {
		return out, fmt.Errorf("elige el cliente del presupuesto: %w", domain.ErrDatos)
	}
	// Un presupuesto no requiere caja abierta ni importe recibido.
	c.Sesion = c.Operacion
	c.Medio = "tarjeta"
	c.Recibido = 0
	if err := c.Validar(); err != nil {
		return out, err
	}
	crudo, err := json.Marshal(c)
	if err != nil {
		return out, err
	}
	hash := sha256.Sum256(crudo)
	huella := hex.EncodeToString(hash[:])
	err = s.escritura(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_setting('app.tenant',true)||':presupuesto-tpv',0))`); err != nil {
			return err
		}
		err := escanearRegistro(tx.QueryRow(ctx, `SELECT id::text,model,ref,etiqueta,estado,data,created_at,updated_at FROM records WHERE tenant=current_setting('app.tenant',true) AND model='presupuesto' AND data->>'tpv_operacion'=$1`, c.Operacion), &out)
		if err == nil {
			if cadenaDe(out.Data["tpv_huella"]) != huella {
				return fmt.Errorf("la operación ya se usó para otro presupuesto: %w", domain.ErrDatos)
			}
			return nil
		}
		if !errors.Is(err, ErrNoEncontrado) {
			return err
		}
		if err := exigirModulosVenta(ctx, tx, "tpv", "ventas", "productos", "contactos"); err != nil {
			return err
		}
		lineas, err := lineasTPV(ctx, tx, c.Lineas)
		if err != nil {
			return err
		}
		var base, iva int64
		for _, g := range domain.Desglose(lineas) {
			base += g.Base
			iva += g.Cuota
		}
		if base+iva <= 0 || base+iva > 1000000000 {
			return fmt.Errorf("el importe del presupuesto no es válido: %w", domain.ErrDatos)
		}
		m, _ := cat.Modelo("presupuesto")
		out, err = insertarNuevo(ctx, tx, Nuevo{desdeTPV: true, Modelo: m, Data: map[string]any{"cliente": c.Cliente, "fecha": time.Now().Format("2006-01-02"), "base": base, "iva": iva, "total": base + iva, "tpv_operacion": c.Operacion, "tpv_huella": huella}}, nil, por)
		if err != nil {
			return err
		}
		lm, _ := cat.Modelo("linea_presupuesto")
		for i, l := range lineas {
			if _, err := insertarNuevo(ctx, tx, Nuevo{Modelo: lm, Etiqueta: l.Descripcion, Data: map[string]any{"presupuesto": out.ID, "producto": c.Lineas[i].Producto, "nombre": l.Descripcion, "cantidad": l.Cantidad, "precio_con_iva": l.Precio, "tipo_iva": strconv.FormatInt(l.Tipo, 10)}}, nil, por); err != nil {
				return err
			}
		}
		return nil
	})
	return out, traducirError(err)
}

// La activación se comprueba donde se escribe: cambiarla en otra sesión no
// puede dejar crear documentos nuevos en un módulo apagado.
func exigirModulosVenta(ctx context.Context, tx pgx.Tx, modulos ...string) error {
	var activos int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT modulo FROM modulos_activos WHERE tenant = current_setting('app.tenant', true) AND modulo = ANY($1::text[]) AND activo ORDER BY modulo FOR SHARE) activos`, modulos).Scan(&activos); err != nil {
		return err
	}
	if activos != len(modulos) {
		return ErrModulosDesactivados
	}
	return nil
}
