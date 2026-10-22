// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/verifactu"
)

// VERI*FACTU en el almacén.
//
// El registro de facturación se genera en la MISMA transacción que saca la
// factura del borrador: o están la factura emitida y su registro, o ninguna de
// las dos cosas. Lo que no puede pasar —cambiar o borrar un registro, bifurcar la
// cadena, retocar una factura enviada— lo impide la base (migración 0010), no
// este código.
//
// Solo se registran las facturas que emite este ERP: las que llegan de otro
// sistema (con `externo_id`) ya las registró quien las emitió (FAQ AEAT, 10).

var (
	// ErrFacturaEmitida: la base no deja cambiar una factura que ya tiene registro.
	ErrFacturaEmitida = errors.New("la factura ya se ha enviado a la Agencia Tributaria: se corrige con una rectificativa")
	// ErrVerifactuPendiente: la AEAT aún no ha contestado al último registro.
	ErrVerifactuPendiente = errors.New("la Agencia Tributaria todavía no ha contestado a esta factura")
	// ErrNadaQueSubsanar: la factura no tiene un registro que corregir.
	ErrNadaQueSubsanar = errors.New("esta factura no tiene nada que corregir en la Agencia Tributaria")
	// ErrNIFConVerifactu: con VERI*FACTU activado el NIF ya no cambia.
	ErrNIFConVerifactu = errors.New("con VERI*FACTU activado el NIF de la organización no se puede cambiar")
)

// Verifactu es el estado de VERI*FACTU en la organización.
type Verifactu struct {
	Activo bool       `json:"activo"`
	Desde  *time.Time `json:"desde,omitempty"`
	// Cuántas facturas están en cada situación, por su último registro.
	Pendientes int `json:"pendientes"`
	ConErrores int `json:"con_errores"`
	Rechazadas int `json:"rechazadas"`
	// ErrorEnvio es por qué no ha salido lo pendiente, si ha fallado el envío.
	ErrorEnvio string `json:"error_envio,omitempty"`
}

// RegistroVerifactu es un registro tal como sale a la API: sin el XML.
type RegistroVerifactu struct {
	ID          string     `json:"id"`
	Tipo        string     `json:"tipo"`
	NIF         string     `json:"nif"`
	NumSerie    string     `json:"num_serie"`
	Fecha       time.Time  `json:"fecha_expedicion"`
	Huella      string     `json:"huella"`
	Estado      string     `json:"estado"`
	CSV         string     `json:"csv,omitempty"`
	Codigo      *int       `json:"codigo_error,omitempty"`
	Descripcion string     `json:"descripcion_error,omitempty"`
	GeneradoEn  time.Time  `json:"generado_en"`
	EnviadoEn   *time.Time `json:"enviado_en,omitempty"`
}

// EstadoVerifactu dice si la organización usa VERI*FACTU y cómo van sus envíos.
func (s *Store) EstadoVerifactu(ctx context.Context) (Verifactu, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return Verifactu{}, err
	}
	var v Verifactu
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT verifactu_desde FROM organizacion WHERE tenant = $1`, tenant).Scan(&v.Desde)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		v.Activo = v.Desde != nil
		return tx.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE estado = 'pendiente'),
			       count(*) FILTER (WHERE estado = 'aceptado_con_errores'),
			       count(*) FILTER (WHERE estado = 'incorrecto'),
			       COALESCE(max(descripcion_error) FILTER (WHERE estado = 'pendiente'), '')
			  FROM (SELECT DISTINCT ON (factura) estado, descripcion_error
			          FROM verifactu_registros
			         WHERE tenant = $1
			         ORDER BY factura, secuencia DESC) u`, tenant).
			Scan(&v.Pendientes, &v.ConErrores, &v.Rechazadas, &v.ErrorEnvio)
	})
	return v, err
}

// ActivarVerifactu enciende VERI*FACTU desde ahora. Repetirlo no cambia nada: la
// fecha y el número de instalación son los de la primera vez.
func (s *Store) ActivarVerifactu(ctx context.Context) (Verifactu, error) {
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE organizacion
			   SET verifactu_desde = now(),
			       verifactu_instalacion = to_char(now() AT TIME ZONE 'UTC', 'YYYYMMDDHH24MISS')
			 WHERE tenant = current_setting('app.tenant', true) AND verifactu_desde IS NULL`)
		return err
	})
	if err != nil {
		return Verifactu{}, err
	}
	return s.EstadoVerifactu(ctx)
}

// RegistrosVerifactu son los registros de una factura, del primero al último.
func (s *Store) RegistrosVerifactu(ctx context.Context, facturaID string) ([]RegistroVerifactu, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	out := []RegistroVerifactu{}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT id::text, tipo, nif, num_serie, fecha_expedicion, huella, estado, csv, codigo_error,
			       descripcion_error, generado_en, enviado_en
			  FROM verifactu_registros
			 WHERE tenant = $1 AND factura = $2::uuid
			 ORDER BY secuencia`, tenant, facturaID)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var r RegistroVerifactu
			if err := filas.Scan(&r.ID, &r.Tipo, &r.NIF, &r.NumSerie, &r.Fecha, &r.Huella, &r.Estado, &r.CSV,
				&r.Codigo, &r.Descripcion, &r.GeneradoEn, &r.EnviadoEn); err != nil {
				return err
			}
			out = append(out, r)
		}
		return filas.Err()
	})
	return out, err
}

// ExportarVerifactu escribe todos los registros de la organización, en el orden
// de la cadena, a medida que los lee.
func (s *Store) ExportarVerifactu(ctx context.Context, escribir func(xml string) error) error {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return err
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `SELECT xml FROM verifactu_registros WHERE tenant = $1 ORDER BY secuencia`, tenant)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var x string
			if err := filas.Scan(&x); err != nil {
				return err
			}
			if err := escribir(x); err != nil {
				return err
			}
		}
		return filas.Err()
	})
}

// orgVF es lo que hace falta de la organización para generar un registro.
type orgVF struct {
	tenant, nombre, nif, instalacion string
}

// orgVerifactu lee la organización si tiene VERI*FACTU activado.
func orgVerifactu(ctx context.Context, tx pgx.Tx) (orgVF, bool, error) {
	var o orgVF
	var nif string
	err := tx.QueryRow(ctx, `
		SELECT tenant, nombre, nif, verifactu_instalacion FROM organizacion
		 WHERE tenant = current_setting('app.tenant', true) AND verifactu_desde IS NOT NULL`).
		Scan(&o.tenant, &o.nombre, &nif, &o.instalacion)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, false, nil
	}
	if v := domain.VariantesNIF(nif); len(v) > 0 {
		o.nif = v[len(v)-1]
	}
	return o, err == nil, err
}

// pasoVF es un registro ya generado de una factura, en el orden de la cadena.
type pasoVF struct {
	tipo, estado, nif, numSerie string
	fecha                       time.Time
}

func historialVerifactu(ctx context.Context, tx pgx.Tx, facturaID string) ([]pasoVF, error) {
	filas, err := tx.Query(ctx, `
		SELECT tipo, estado, nif, num_serie, fecha_expedicion FROM verifactu_registros
		 WHERE tenant = current_setting('app.tenant', true) AND factura = $1::uuid
		 ORDER BY secuencia`, facturaID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []pasoVF
	for filas.Next() {
		var p pasoVF
		if err := filas.Scan(&p.tipo, &p.estado, &p.nif, &p.numSerie, &p.fecha); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, filas.Err()
}

// enLaAEAT dice si la AEAT tiene el alta de la factura: alguna se aceptó.
func enLaAEAT(h []pasoVF) bool {
	for _, p := range h {
		if p.tipo == "alta" && (p.estado == "correcto" || p.estado == "aceptado_con_errores") {
			return true
		}
	}
	return false
}

// ultimaAlta es el último registro de alta: de él salen el NIF, el número y la
// fecha con los que la AEAT conoce la factura.
func ultimaAlta(h []pasoVF) pasoVF {
	for i := len(h) - 1; i >= 0; i-- {
		if h[i].tipo == "alta" {
			return h[i]
		}
	}
	return pasoVF{}
}

// emitirVerifactu genera, dentro de tx y ANTES de cambiar el estado, los registros
// de las facturas de venta de `ids` que pasan a `estado`: el alta de las que se
// emiten ahora y la anulación de las que se anulan después de emitidas. Sin
// VERI*FACTU activado no genera nada, pero la factura que se emite tiene que
// cuadrar igual (cuadreDeFactura).
func (s *Store) emitirVerifactu(ctx context.Context, tx pgx.Tx, modelo string, ids []string, estado, sub string) error {
	if modelo != "factura_venta" || len(ids) == 0 || (estado != "confirmado" && estado != "pagado" && estado != "anulado") {
		return nil
	}
	org, activo, err := orgVerifactu(ctx, tx)
	if err != nil {
		return err
	}
	if !activo {
		return cuadrarAlEmitir(ctx, tx, ids, estado)
	}
	if err := bloquearCadena(ctx, tx, org); err != nil {
		return err
	}
	facturas, err := facturasParaEmitir(ctx, tx, ids)
	if err != nil {
		return err
	}
	for _, f := range facturas {
		h, err := historialVerifactu(ctx, tx, f.ID)
		if err != nil {
			return err
		}
		switch registroQueToca(f, h, estado) {
		case "anulacion":
			err = s.anularVerifactu(ctx, tx, org, f, h, false, sub)
		case "alta":
			err = s.altaVerifactu(ctx, tx, org, f, false, false, sub)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", f.Ref, err)
		}
	}
	return nil
}

// cuadrarAlEmitir comprueba, sin VERI*FACTU, las facturas que se emiten ahora:
// las mismas a las que, con él, les tocaría el alta.
func cuadrarAlEmitir(ctx context.Context, tx pgx.Tx, ids []string, estado string) error {
	facturas, err := facturasParaEmitir(ctx, tx, ids)
	if err != nil {
		return err
	}
	for _, f := range facturas {
		if registroQueToca(f, nil, estado) != "alta" {
			continue
		}
		if _, err := cuadreDeFactura(ctx, tx, f); err != nil {
			return fmt.Errorf("%s: %w", f.Ref, err)
		}
	}
	return nil
}

// registroQueToca decide qué registro deja una factura al pasar a `estado`: el
// alta si se emite ahora (sin registros, desde borrador o anulada, y hecha aquí)
// y la anulación si se anula lo que se emitió. Nada en los demás casos.
func registroQueToca(f Registro, h []pasoVF, estado string) string {
	if estado == "anulado" {
		if len(h) > 0 && h[len(h)-1].tipo == "alta" {
			return "anulacion"
		}
		return ""
	}
	if len(h) == 0 && (f.Estado == "borrador" || f.Estado == "anulado") && strings.TrimSpace(cadenaDe(f.Data["externo_id"])) == "" {
		return "alta"
	}
	return ""
}

// SubsanarVerifactu corrige en la AEAT lo último que se envió de una factura: un
// alta rechazada o aceptada con errores (con los datos de ahora de la factura y
// su cliente) o una anulación rechazada.
func (s *Store) SubsanarVerifactu(ctx context.Context, facturaID, sub string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		org, activo, err := orgVerifactu(ctx, tx)
		if err != nil {
			return err
		}
		if !activo {
			return ErrNadaQueSubsanar
		}
		if err := bloquearCadena(ctx, tx, org); err != nil {
			return err
		}
		facturas, err := facturasParaEmitir(ctx, tx, []string{facturaID})
		if err != nil {
			return err
		}
		if len(facturas) == 0 {
			return ErrNoEncontrado
		}
		h, err := historialVerifactu(ctx, tx, facturaID)
		if err != nil {
			return err
		}
		if len(h) == 0 {
			return ErrNadaQueSubsanar
		}
		switch u := h[len(h)-1]; {
		case u.estado == "pendiente":
			return ErrVerifactuPendiente
		case u.tipo == "alta" && u.estado != "correcto":
			// Aceptada con errores: subsanación. Rechazada sin que la AEAT aceptara
			// nunca otra: no la tiene, así que es un «alta por rechazo».
			return s.altaVerifactu(ctx, tx, org, facturas[0], true, u.estado == "incorrecto" && !enLaAEAT(h), sub)
		case u.tipo == "anulacion" && u.estado == "incorrecto":
			return s.anularVerifactu(ctx, tx, org, facturas[0], h, true, sub)
		}
		return ErrNadaQueSubsanar
	})
}

// bloquearCadena serializa a quien genera registros de la misma organización: el
// anterior de cada registro tiene que ser de verdad el último.
func bloquearCadena(ctx context.Context, tx pgx.Tx, org orgVF) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('syerp-verifactu:' || $1, 0))`, org.tenant)
	return err
}

func facturasParaEmitir(ctx context.Context, tx pgx.Tx, ids []string) ([]Registro, error) {
	filas, err := tx.Query(ctx, `
		SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at
		  FROM records
		 WHERE tenant = current_setting('app.tenant', true) AND id = ANY($1::uuid[]) AND model = 'factura_venta' AND deleted_at IS NULL
		 ORDER BY ref, id
		 FOR UPDATE`, ids)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []Registro
	for filas.Next() {
		var r Registro
		if err := escanearRegistro(filas, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, filas.Err()
}

// anteriorVF es el último registro de la cadena de la instalación.
func anteriorVF(ctx context.Context, tx pgx.Tx, org orgVF) (*verifactu.Anterior, error) {
	var a verifactu.Anterior
	err := tx.QueryRow(ctx, `
		SELECT nif, num_serie, fecha_expedicion, huella FROM verifactu_registros
		 WHERE tenant = $1 AND instalacion = $2
		 ORDER BY secuencia DESC LIMIT 1`, org.tenant, org.instalacion).
		Scan(&a.NIF, &a.NumSerie, &a.Fecha, &a.Huella)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // el primer registro de la cadena no tiene anterior
	}
	return &a, err
}

// ahoraVF es el momento de generación, al segundo: es la precisión con la que va
// en el registro y en la huella, y así lo guardado se vuelve a escribir igual.
func ahoraVF() time.Time { return time.Now().UTC().Truncate(time.Second) }

func (s *Store) altaVerifactu(ctx context.Context, tx pgx.Tx, org orgVF, f Registro, subsanacion, porRechazo bool, sub string) error {
	a, err := altaDe(ctx, tx, org, f)
	if err != nil {
		return err
	}
	a.Subsanacion, a.PorRechazo = subsanacion, porRechazo
	if a.Anterior, err = anteriorVF(ctx, tx, org); err != nil {
		return err
	}
	a.Ref, a.Generado = uuid.NewString(), ahoraVF()
	a.Sistema = verifactu.Sistema{Version: s.Version, NumeroInstalacion: org.instalacion}
	doc, err := a.XML()
	if err != nil {
		return err
	}
	return guardarRegistroVF(ctx, tx, org, f.ID, "alta", a.Factura, a.Ref, a.Huella(), a.Anterior, doc, a.Generado, sub)
}

func (s *Store) anularVerifactu(ctx context.Context, tx pgx.Tx, org orgVF, f Registro, h []pasoVF, porRechazo bool, sub string) error {
	alta := ultimaAlta(h)
	// Un alta que aún no ha contestado la AEAT cuenta como que la tendrá: sale
	// antes que la anulación. Solo si se rechazó todas, la AEAT no la tiene.
	existe := slices.ContainsFunc(h, func(p pasoVF) bool { return p.tipo == "alta" && p.estado != "incorrecto" })
	a := verifactu.Anulacion{
		Ref:               uuid.NewString(),
		Factura:           verifactu.IDFactura{NIF: alta.nif, NumSerie: alta.numSerie, Fecha: alta.fecha},
		SinRegistroPrevio: !existe,
		PorRechazo:        porRechazo && existe,
		Sistema:           verifactu.Sistema{Version: s.Version, NumeroInstalacion: org.instalacion},
		Generado:          ahoraVF(),
	}
	var err error
	if a.Anterior, err = anteriorVF(ctx, tx, org); err != nil {
		return err
	}
	doc, err := a.XML()
	if err != nil {
		return err
	}
	return guardarRegistroVF(ctx, tx, org, f.ID, "anulacion", a.Factura, a.Ref, a.Huella(), a.Anterior, doc, a.Generado, sub)
}

func guardarRegistroVF(ctx context.Context, tx pgx.Tx, org orgVF, facturaID, tipo string, id verifactu.IDFactura,
	ref, huella string, anterior *verifactu.Anterior, doc string, generado time.Time, sub string,
) error {
	previa := ""
	if anterior != nil {
		previa = anterior.Huella
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO verifactu_registros (id, tenant, instalacion, factura, tipo, nif, num_serie, fecha_expedicion,
		                                 huella, huella_anterior, xml, generado_en, generado_por)
		VALUES ($1::uuid, $2, $3, $4::uuid, $5, $6, $7, $8::date, $9, $10, $11, $12, $13)`,
		ref, org.tenant, org.instalacion, facturaID, tipo, id.NIF, id.NumSerie, id.Fecha, huella, previa, doc, generado, sub)
	return err
}

// regimenesIVA traducen el régimen de una factura sin IVA a su clave de la AEAT.
var regimenesIVA = map[string]verifactu.Detalle{
	"iva_exenta":      {Exencion: "E1"},
	"iva_exportacion": {Exencion: "E2"},
	"iva_entrega_ue":  {Exencion: "E5"},
	"iva_no_sujeta":   {Calificacion: "N2"},
	"iva_inversion":   {Calificacion: "S2"},
}

// altaDe arma el alta con los datos de la factura, su cliente y, si es
// rectificativa, la factura que rectifica.
func altaDe(ctx context.Context, tx pgx.Tx, org orgVF, f Registro) (verifactu.Alta, error) {
	a := verifactu.Alta{
		Emisor:      verifactu.Parte{Nombre: org.nombre, NIF: org.nif},
		TipoFactura: "F1",
		Descripcion: strings.TrimSpace(cadenaDe(f.Data["notas"])),
	}
	if a.Descripcion == "" {
		a.Descripcion = "Venta de bienes o prestación de servicios"
	}
	fecha, err := fechaDe(f.Data["fecha"])
	if err != nil {
		return a, err
	}
	a.Factura = verifactu.IDFactura{NIF: org.nif, NumSerie: f.Ref, Fecha: fecha}

	base, err := domain.Centimos(f.Data, "base")
	if err != nil {
		return a, err
	}
	iva, err := domain.Centimos(f.Data, "iva")
	if err != nil {
		return a, err
	}
	a.CuotaTotal, a.ImporteTotal = iva, base+iva
	grupos, err := cuadreDeFactura(ctx, tx, f)
	if err != nil {
		return a, err
	}
	if a.Desglose, err = desgloseDe(grupos, base, iva, cadenaDe(f.Data["regimen_iva"])); err != nil {
		return a, err
	}

	// Una simplificada no lleva destinatario: su cliente es, a lo sumo, el genérico
	// de las ventas en mostrador o en línea.
	simplificada := cadenaDe(f.Data["tipo_factura"]) == "F2"
	if simplificada {
		a.TipoFactura = "F2"
	} else if a.Destinatario, err = destinatarioDe(ctx, tx, cadenaDe(f.Data["cliente"])); err != nil {
		return a, err
	}
	if id := cadenaDe(f.Data["rectifica"]); id != "" {
		tipo, rectificada, err := rectificadaDe(ctx, tx, org.nif, id, simplificada)
		if err != nil {
			return a, err
		}
		a.TipoFactura, a.Rectificadas = tipo, []verifactu.IDFactura{rectificada}
	}
	return a, nil
}

// desgloseDe es el desglose de la factura: el de sus líneas, agrupado por tipo, si
// las tiene; si no, el de la cabecera, que es de un solo tipo.
func desgloseDe(grupos []domain.GrupoIVA, base, iva int64, regimen string) ([]verifactu.Detalle, error) {
	if len(grupos) > 0 {
		return desgloseDeGrupos(grupos, regimen)
	}
	switch d, ok := regimenesIVA[regimen]; {
	case iva != 0 && regimen != "" && regimen != "iva_general":
		return nil, fmt.Errorf("%w: una factura con IVA va en el régimen general", verifactu.ErrDatos)
	case iva != 0:
		tipo, ok := verifactu.TipoIVA(base, iva)
		if !ok {
			return nil, fmt.Errorf("%w: el IVA no corresponde a ningún tipo vigente (21 %%, 10 %%, 4 %%…) sobre la base", verifactu.ErrDatos)
		}
		return []verifactu.Detalle{{Calificacion: "S1", Tipo: tipo, Base: base, Cuota: iva}}, nil
	case !ok:
		return nil, fmt.Errorf("%w: una factura sin IVA tiene que decir por qué: elige su régimen de IVA", verifactu.ErrDatos)
	default:
		d.Base = base
		return []verifactu.Detalle{d}, nil
	}
}

// rectificadaDe lee la factura que se rectifica y dice el tipo de la
// rectificativa: R1, o R5 si rectifica una simplificada (y entonces tampoco lleva
// destinatario).
func rectificadaDe(ctx context.Context, tx pgx.Tx, nif, id string, simplificada bool) (string, verifactu.IDFactura, error) {
	var ref, estado string
	var data map[string]any
	err := tx.QueryRow(ctx, `
		SELECT ref, estado, data FROM records WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = 'factura_venta'`, id).
		Scan(&ref, &estado, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", verifactu.IDFactura{}, ErrNoEncontrado
	}
	if err != nil {
		return "", verifactu.IDFactura{}, err
	}
	if estado == "borrador" {
		return "", verifactu.IDFactura{}, fmt.Errorf("%w: la factura que se rectifica (%s) todavía es un borrador", verifactu.ErrDatos, ref)
	}
	original, err := fechaDe(data["fecha"])
	if err != nil {
		return "", verifactu.IDFactura{}, err
	}
	if (cadenaDe(data["tipo_factura"]) == "F2") != simplificada {
		return "", verifactu.IDFactura{}, fmt.Errorf("%w: una rectificativa es del mismo tipo (completa o simplificada) que la factura que rectifica (%s)",
			verifactu.ErrDatos, ref)
	}
	tipo := "R1"
	if simplificada {
		tipo = "R5"
	}
	return tipo, verifactu.IDFactura{NIF: nif, NumSerie: ref, Fecha: original}, nil
}

// destinatarioDe es el cliente como lo pide la AEAT: NIF si es de España; si no,
// su NIF-IVA (Unión Europea) o su identificación fiscal con el país.
func destinatarioDe(ctx context.Context, tx pgx.Tx, contactoID string) (*verifactu.Parte, error) {
	if contactoID == "" {
		return nil, nil //nolint:nilnil // sin cliente no hay destinatario; Validar dice que falta
	}
	var etiqueta string
	var data map[string]any
	err := tx.QueryRow(ctx, `SELECT etiqueta, data FROM records WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = 'contacto'`, contactoID).
		Scan(&etiqueta, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // el contacto ya no existe: lo mismo que sin cliente
	}
	if err != nil {
		return nil, err
	}
	p := &verifactu.Parte{Nombre: strings.TrimSpace(cadenaDe(data["nombre"]))}
	if p.Nombre == "" {
		p.Nombre = etiqueta
	}
	variantes := domain.VariantesNIF(cadenaDe(data["nif"]))
	if len(variantes) == 0 {
		return p, nil
	}
	pais := strings.ToUpper(strings.TrimSpace(cadenaDe(data["pais"])))
	// Sin país, un NIF-IVA con prefijo de otro Estado de la Unión dice de dónde es.
	if len(pais) != 2 && len(variantes) == 2 && !strings.HasPrefix(variantes[0], "ES") {
		pais = strings.Replace(variantes[0][:2], "EL", "GR", 1)
	}
	switch {
	case len(pais) != 2 || pais == "ES":
		p.NIF = variantes[len(variantes)-1]
	case domain.MiembroUE(pais):
		p.CodigoPais, p.TipoID, p.ID = pais, "02", variantes[0]
		if len(variantes) == 1 {
			p.ID = domain.PrefijoIVA(pais) + variantes[0]
		}
	default:
		p.CodigoPais, p.TipoID, p.ID = pais, "04", variantes[0]
	}
	return p, nil
}

func cadenaDe(v any) string {
	s, _ := v.(string)
	return s
}

func fechaDe(v any) (time.Time, error) {
	s := cadenaDe(v)
	if len(s) < 10 {
		return time.Time{}, fmt.Errorf("%w: la factura no tiene fecha", verifactu.ErrDatos)
	}
	t, err := time.Parse(time.DateOnly, s[:10])
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: la fecha de la factura no es válida", verifactu.ErrDatos)
	}
	return t, nil
}

// --- Envío --------------------------------------------------------------------

// LoteVerifactu es un envío: los registros pendientes de una organización.
type LoteVerifactu struct {
	Obligado verifactu.Parte
	IDs      []string
	XML      []string
}

// TenantsConVerifactuPendiente son las organizaciones con registros por enviar.
func (s *Store) TenantsConVerifactuPendiente(ctx context.Context) ([]string, error) {
	var out []string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `SELECT DISTINCT tenant FROM verifactu_registros WHERE estado = 'pendiente' ORDER BY tenant`)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var t string
			if err := filas.Scan(&t); err != nil {
				return err
			}
			out = append(out, t)
		}
		return filas.Err()
	})
	return out, err
}

// reservaEnvio es cuánto se aparta el turno de envío mientras se espera la
// respuesta: más que el plazo del cliente HTTP, para que otra réplica no mande lo
// mismo a la vez.
const reservaEnvio = 2 * time.Minute

// ReclamarLoteVerifactu toma lo pendiente de la organización si ya le toca enviar
// —ha pasado el tiempo que marcó la AEAT o hay un envío lleno— y reserva el turno.
// Un lote vacío es que no toca.
func (s *Store) ReclamarLoteVerifactu(ctx context.Context) (LoteVerifactu, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return LoteVerifactu{}, err
	}
	var l LoteVerifactu
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// El reloj es el de la base, el mismo para todas las réplicas.
		var toca bool
		err := tx.QueryRow(ctx, `
			SELECT nombre, verifactu_proximo_envio <= now() FROM organizacion WHERE tenant = $1 FOR UPDATE`, tenant).
			Scan(&l.Obligado.Nombre, &toca)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		filas, err := tx.Query(ctx, `
			SELECT id::text, nif, num_serie, xml, intentos FROM verifactu_registros
			 WHERE tenant = $1 AND estado = 'pendiente'
			 ORDER BY secuencia LIMIT $2`, tenant, verifactu.MaxRegistros)
		if err != nil {
			return err
		}
		defer filas.Close()
		vistas := map[string]bool{}
		reintento := false
		for filas.Next() {
			var id, nif, serie, doc string
			var intentos int
			if err := filas.Scan(&id, &nif, &serie, &doc, &intentos); err != nil {
				return err
			}
			reintento = reintento || intentos > 0
			// El alta y la anulación de una misma factura van en envíos distintos:
			// la segunda necesita que la AEAT ya tenga la primera.
			if vistas[serie] {
				break
			}
			vistas[serie] = true
			l.Obligado.NIF = nif
			l.IDs, l.XML = append(l.IDs, id), append(l.XML, doc)
		}
		filas.Close()
		if err := filas.Err(); err != nil {
			return err
		}
		// Un envío lleno no espera al plazo de la AEAT (art. 16.2), pero solo la
		// primera vez: si ya se intentó y falló, espera como todos. Sin esto, con
		// mil pendientes y la AEAT caída se reenviaba el mismo lote cada 30 s, sin
		// fin (NEG-21).
		if len(l.IDs) == 0 || (!toca && (len(l.IDs) < verifactu.MaxRegistros || reintento)) {
			l = LoteVerifactu{}
			return nil
		}
		if err := aplazarEnvio(ctx, tx, tenant, reservaEnvio); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE verifactu_registros SET intentos = intentos + 1 WHERE tenant = current_setting('app.tenant', true) AND id = ANY($1::uuid[])`, l.IDs)
		return err
	})
	return l, err
}

// estadosAEAT pasan el estado de la respuesta al de la tabla.
var estadosAEAT = map[string]string{
	"Correcto":           "correcto",
	"AceptadoConErrores": "aceptado_con_errores",
	"Incorrecto":         "incorrecto",
}

// GuardarRespuestaVerifactu anota lo que contestó la AEAT a un lote y cuándo se
// puede volver a enviar. Si el envío falló, lo pendiente sigue pendiente, con el
// motivo, y se espera más cuantas más veces haya fallado.
func (s *Store) GuardarRespuestaVerifactu(ctx context.Context, l LoteVerifactu, r verifactu.Respuesta, errEnvio error) error {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return err
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if errEnvio != nil {
			var intentos int
			if err := tx.QueryRow(ctx, `
				UPDATE verifactu_registros SET descripcion_error = $2
				 WHERE tenant = current_setting('app.tenant', true) AND id = ANY($1::uuid[]) AND estado = 'pendiente'
				RETURNING intentos`, l.IDs, truncarTexto(errEnvio.Error(), 1500)).Scan(&intentos); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			return aplazarEnvio(ctx, tx, tenant, min(time.Hour, verifactu.EsperaInicial<<min(intentos, 6)))
		}
		delLote := map[string]bool{}
		for _, id := range l.IDs {
			delLote[id] = true
		}
		hoy := time.Now().In(madridVF).Format(time.DateOnly)
		for _, linea := range r.Lineas {
			estadoAEAT, codigo, descripcion := linea.EstadoFinal()
			estado, ok := estadosAEAT[estadoAEAT]
			if !ok || !delLote[linea.Ref] {
				continue
			}
			var facturaID, serie string
			err := tx.QueryRow(ctx, `
				UPDATE verifactu_registros
				   SET estado = $2, csv = $3, codigo_error = NULLIF($4, 0), descripcion_error = $5, enviado_en = now()
				 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND estado = 'pendiente'
				RETURNING factura::text, num_serie`,
				linea.Ref, estado, r.CSV, codigo, truncarTexto(descripcion, 1500)).Scan(&facturaID, &serie)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if estado == "correcto" {
				continue
			}
			resumen := "Corrige la factura " + serie + ": la Agencia Tributaria la ha rechazado"
			if estado == "aceptado_con_errores" {
				resumen = "Revisa la factura " + serie + ": la Agencia Tributaria la ha aceptado con errores"
			}
			if _, err := crearActividadEn(ctx, tx, Actividad{
				Modelo: "factura_venta", Registro: facturaID, Tipo: "tarea",
				Resumen: resumen, Notas: descripcion, Vence: hoy,
			}, "verifactu"); err != nil {
				return err
			}
		}
		return aplazarEnvio(ctx, tx, tenant, r.EsperaDe())
	})
}

// aplazarEnvio fija cuándo puede volver a enviar la organización.
func aplazarEnvio(ctx context.Context, tx pgx.Tx, tenant string, espera time.Duration) error {
	_, err := tx.Exec(ctx, `
		UPDATE organizacion SET verifactu_proximo_envio = now() + make_interval(secs => $2)
		 WHERE tenant = $1`, tenant, espera.Seconds())
	return err
}

var madridVF = func() *time.Location {
	l, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		return time.UTC
	}
	return l
}()

func truncarTexto(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
