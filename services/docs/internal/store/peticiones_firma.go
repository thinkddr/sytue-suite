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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PeticionFirma es una fila de «Pedir firma» (migración 0011, ADR 0051).
type PeticionFirma struct {
	ID                 string     `json:"id"`
	Solicitante        string     `json:"requester"`
	SolicitanteNombre  string     `json:"requester_name,omitempty"`
	Destinatario       string     `json:"recipient"`
	DestinatarioCorreo string     `json:"recipient_email,omitempty"`
	Fichero            string     `json:"file_id"`
	Ruta               string     `json:"path"`
	Mensaje            string     `json:"message,omitempty"`
	Estado             string     `json:"status"`
	Creada             time.Time  `json:"created_at"`
	Resuelta           *time.Time `json:"resolved_at,omitempty"`
	Firmado            string     `json:"signed_file_id,omitempty"`
	FirmadoRuta        string     `json:"signed_path,omitempty"`
	// Externo: quien firma es de OTRA organización (mig. 0012). Lo que ve esa persona
	// sale de [PeticionFirma.ParaQuienFirma]: Ruta es la de SU copia —vacía hasta que
	// la trae— y Nombre el del fichero.
	Externo bool   `json:"external,omitempty"`
	Nombre  string `json:"name,omitempty"`
	// Lo que no sale nunca por la API: el token cifrado del enlace de Drive, el id de
	// ese enlace y la copia del original en la unidad de quien firma.
	Enlace      []byte `json:"-"`
	EnlaceShare string `json:"-"`
	Copia       string `json:"-"`
	CopiaRuta   string `json:"-"`
	// Huella es el SHA-256 del original al pedir (la copia y lo firmado van contra él),
	// y FirmadoHuella el de lo entregado y comprobado (lo que se trae quien pidió).
	Huella        string `json:"-"`
	FirmadoHuella string `json:"-"`
	// Silenciada: se «envió» pero no llega (bloqueo o política, mig. 0013). Quien
	// pidió no la distingue de otra; por eso no sale por la API.
	Silenciada bool `json:"-"`
}

// Estados de una petición de firma.
const (
	FirmaPendiente = "pendiente"
	FirmaFirmada   = "firmada"
	FirmaRechazada = "rechazada"
	FirmaCancelada = "cancelada"
)

var (
	// ErrPeticionFirmaNoEsta: no existe, no es de quien pregunta o ya no está
	// pendiente. Se contesta igual a las tres: distinguirlas diría qué ids existen.
	ErrPeticionFirmaNoEsta = errors.New("store: la petición de firma no está pendiente para esta persona")
	// ErrPeticionFirmaRepetida: ya hay una pendiente de ese fichero a esa persona.
	ErrPeticionFirmaRepetida = errors.New("store: ya hay una petición de firma pendiente de ese fichero a esa persona")
	// ErrDemasiadasPeticionesFirma: quien pide ha llegado al tope de la última hora.
	ErrDemasiadasPeticionesFirma = errors.New("store: demasiadas peticiones de firma en la última hora")
	// ErrDemasiadasParaEsaPersona: la persona de fuera ya tiene demasiadas pendientes,
	// de quien pide o de todas las organizaciones.
	ErrDemasiadasParaEsaPersona = errors.New("store: esa persona tiene demasiadas peticiones de firma pendientes")
	// ErrPeticionFirmaOcupada: otra vuelta de «abrir» o «traer» está en marcha.
	ErrPeticionFirmaOcupada = errors.New("store: la petición de firma está ocupada")
)

// MaxPeticionesFirmaPorHora es cuántas puede crear una persona en una hora. Cada una
// es un aviso y un correo a otra persona; sin tope, pedir y cancelar en bucle sería
// una forma de llenarle el buzón a un compañero.
const MaxPeticionesFirmaPorHora = 30

// Topes a alguien de OTRA organización (ADR 0057): cada petición le pone un aviso y,
// al abrirla, un PDF en la unidad de su organización. Sin ellos, cualquiera que sepa un
// @usuario le llenaría la lista.
const (
	// MaxPendientesPorPersona son las pendientes que una persona de fuera puede tener de
	// una misma persona que pide.
	MaxPendientesPorPersona = 5
	// MaxPendientesDeFuera son las que puede tener de fuera en total, de todas las
	// organizaciones.
	MaxPendientesDeFuera = 20
)

const columnasPeticionFirma = `id::text, solicitante, solicitante_nombre, destinatario, destinatario_correo, fichero, ruta, mensaje,
	estado, creada, resuelta, coalesce(firmado, ''), coalesce(firmado_ruta, ''),
	externo, enlace, coalesce(enlace_share, ''), coalesce(copia, ''), coalesce(copia_ruta, ''),
	coalesce(huella, ''), coalesce(firmado_huella, ''), silenciada`

func escanearPeticionFirma(f pgx.CollectableRow) (PeticionFirma, error) {
	var p PeticionFirma
	return p, f.Scan(&p.ID, &p.Solicitante, &p.SolicitanteNombre, &p.Destinatario, &p.DestinatarioCorreo, &p.Fichero, &p.Ruta,
		&p.Mensaje, &p.Estado, &p.Creada, &p.Resuelta, &p.Firmado, &p.FirmadoRuta,
		&p.Externo, &p.Enlace, &p.EnlaceShare, &p.Copia, &p.CopiaRuta, &p.Huella, &p.FirmadoHuella, &p.Silenciada)
}

// ParaQuienFirma es la fila como la ve su destinatario de FUERA: «el PDF» es su copia
// —la franja y la entrega van con ella— y del original solo sabe el nombre, no dónde
// lo guarda la otra organización ni su id allí.
func (p PeticionFirma) ParaQuienFirma() PeticionFirma {
	if !p.Externo {
		return p
	}
	p.Nombre = p.Ruta[strings.LastIndexByte(p.Ruta, '/')+1:]
	p.Fichero, p.Ruta, p.Firmado, p.FirmadoRuta = p.Copia, p.CopiaRuta, "", ""
	return p
}

// ParaQuienPide es la fila como la ve quien pidió: la copia firmada de alguien de fuera
// vive en la unidad de la otra organización, y su id allí no le dice nada hasta que se
// la trae (entonces es el de SU copia).
func (p PeticionFirma) ParaQuienPide() PeticionFirma {
	if p.Externo && p.FirmadoRuta == "" {
		p.Firmado = ""
	}
	return p
}

// comprobarPeticionFirma son los topes y la repetida, dentro de la transacción. Los
// cerrojos por quien pide y por destinatario hacen que dos peticiones a la vez no los
// pasen las dos.
//
// Devuelve `llena` si la persona de fuera ya tiene su tope de pendientes: esa no se
// rechaza, se crea SILENCIADA. Con un 429 quien pide podría contar, llenando el tope
// desde otras cuentas, cuáles de sus peticiones le han llegado y cuáles no (ADR 0059).
func comprobarPeticionFirma(ctx context.Context, tx pgx.Tx, p PeticionFirma) (llena bool, err error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('pdf-firma:' || $1)),
		pg_advisory_xact_lock(hashtext('pdf-firma-a:' || $2))`, p.Solicitante, p.Destinatario); err != nil {
		return false, err
	}
	var ultimaHora, repetida, dePar int
	var deFuera int64
	if err := tx.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE creada > now() - interval '1 hour'),
			count(*) FILTER (WHERE estado = 'pendiente' AND fichero = $2 AND destinatario = $3),
			count(*) FILTER (WHERE estado = 'pendiente' AND externo AND destinatario = $3)
		FROM pdf_peticiones_firma WHERE solicitante = $1`, p.Solicitante, p.Fichero, p.Destinatario).
		Scan(&ultimaHora, &repetida, &dePar); err != nil {
		return false, err
	}
	// Estos tres cuentan TODO lo de quien pide, llegara o no: los podría contar él
	// mismo, así que no dicen nada que no sepa.
	switch {
	case ultimaHora >= MaxPeticionesFirmaPorHora:
		return false, ErrDemasiadasPeticionesFirma
	case repetida > 0:
		return false, ErrPeticionFirmaRepetida
	case !p.Externo:
		return false, nil
	case dePar >= MaxPendientesPorPersona:
		return false, ErrDemasiadasParaEsaPersona
	}
	if err := tx.QueryRow(ctx, `SELECT docs_firmas_pendientes_para($1)`, p.Destinatario).Scan(&deFuera); err != nil {
		return false, err
	}
	return deFuera >= MaxPendientesDeFuera, nil
}

// errorAlCrear deja pasar los errores de los topes y envuelve el resto.
func errorAlCrear(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return ErrPeticionFirmaRepetida
	case errors.Is(err, ErrDemasiadasPeticionesFirma), errors.Is(err, ErrPeticionFirmaRepetida),
		errors.Is(err, ErrDemasiadasParaEsaPersona):
		return err
	}
	return fmt.Errorf("store: crear la petición de firma: %w", err)
}

// PuedePedirFirma dice, sin crear nada, si CrearPeticionFirma pasaría los topes: para
// no crear en Drive un enlace que haya que quitar enseguida.
func (s *Store) PuedePedirFirma(ctx context.Context, p PeticionFirma) error {
	if err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		_, err := comprobarPeticionFirma(ctx, tx, p)
		return err
	}); err != nil {
		return errorAlCrear(err)
	}
	return nil
}

// CrearPeticionFirma la deja pendiente y devuelve la fila.
func (s *Store) CrearPeticionFirma(ctx context.Context, p PeticionFirma) (PeticionFirma, error) {
	var out PeticionFirma
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		llena, err := comprobarPeticionFirma(ctx, tx, p)
		if err != nil {
			return err
		}
		// El bloqueo se mira OTRA vez aquí, dentro de la transacción: uno que llegue
		// entre la comprobación del handler y este INSERT también la silencia.
		rows, err := tx.Query(ctx, `
			INSERT INTO pdf_peticiones_firma
			    (tenant, solicitante, solicitante_nombre, destinatario, destinatario_correo, fichero, ruta, mensaje,
			     externo, enlace, enlace_share, huella, silenciada)
			VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6, $7, $8, $9, nullif($10, ''), nullif($11, ''),
			        $8 AND ($12 OR docs_firma_bloqueado($3, $1)))
			RETURNING `+columnasPeticionFirma,
			p.Solicitante, p.SolicitanteNombre, p.Destinatario, p.DestinatarioCorreo, p.Fichero, p.Ruta, p.Mensaje,
			p.Externo, p.Enlace, p.EnlaceShare, p.Huella, p.Silenciada || llena)
		if err != nil {
			return err
		}
		out, err = pgx.CollectExactlyOneRow(rows, escanearPeticionFirma)
		return err
	})
	if err != nil {
		return PeticionFirma{}, errorAlCrear(err)
	}
	return out, nil
}

// maxPeticionesFirma es cuántas se listan de cada lado: las más recientes.
const maxPeticionesFirma = 200

// PeticionesFirma son las que ha recibido `sub` y las que ha pedido, las más
// recientes primero.
func (s *Store) PeticionesFirma(ctx context.Context, sub string) (recibidas, pedidas []PeticionFirma, err error) {
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		for _, lado := range []struct {
			cond string
			dest *[]PeticionFirma
		}{{"destinatario = $1 AND NOT silenciada", &recibidas}, {"solicitante = $1", &pedidas}} {
			rows, err := tx.Query(ctx, `SELECT `+columnasPeticionFirma+` FROM pdf_peticiones_firma
				WHERE `+lado.cond+` ORDER BY creada DESC LIMIT $2`, sub, maxPeticionesFirma)
			if err != nil {
				return err
			}
			if *lado.dest, err = pgx.CollectRows(rows, escanearPeticionFirma); err != nil {
				return err
			}
		}
		// Y las que le han pedido desde otras organizaciones (mig. 0012).
		rows, err := tx.Query(ctx, `SELECT id::text, solicitante, solicitante_nombre, destinatario, destinatario_correo,
				nombre, copia, copia_ruta, mensaje, estado, creada, resuelta
			FROM docs_firmas_de_fuera($1, current_setting('app.tenant', true))`, sub)
		if err != nil {
			return err
		}
		fuera, err := pgx.CollectRows(rows, func(f pgx.CollectableRow) (PeticionFirma, error) {
			// Del original solo llega el nombre (Ruta sin carpetas): lo que ve quien firma
			// lo compone [PeticionFirma.ParaQuienFirma], como con las demás.
			p := PeticionFirma{Externo: true}
			return p, f.Scan(&p.ID, &p.Solicitante, &p.SolicitanteNombre, &p.Destinatario, &p.DestinatarioCorreo,
				&p.Ruta, &p.Copia, &p.CopiaRuta, &p.Mensaje, &p.Estado, &p.Creada, &p.Resuelta)
		})
		if err != nil {
			return err
		}
		recibidas = append(recibidas, fuera...)
		slices.SortStableFunc(recibidas, func(a, b PeticionFirma) int { return b.Creada.Compare(a.Creada) })
		if len(recibidas) > maxPeticionesFirma {
			recibidas = recibidas[:maxPeticionesFirma]
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("store: listar las peticiones de firma: %w", err)
	}
	return recibidas, pedidas, nil
}

// ResolverPeticionFirma pasa una PENDIENTE a `estado`. Solo el destinatario la firma
// o la rechaza, y solo el solicitante la cancela: `quien` es el `sub` de quien llama
// y la sentencia lo exige en la columna que toca, así que no hay un paso de
// «comprobar y luego escribir» que se pueda adelantar.
//
// `enlace` es lo que queda en la fila: el token cifrado del enlace a la copia firmada
// cuando quien firma es de fuera, y nil en cualquier otro caso (el del original ya no
// hace falta: se borra). `firmadoHuella` es el SHA-256 de lo entregado.
func (s *Store) ResolverPeticionFirma(ctx context.Context, id, quien, estado, firmado, firmadoRuta string, enlace []byte, firmadoHuella string) (PeticionFirma, error) {
	// A quien recibe, lo silenciado no le llega: no lo firma ni lo rechaza. Quien
	// pidió sí la cancela, como cualquier otra.
	columna, silencio := "destinatario", " AND NOT silenciada"
	if estado == FirmaCancelada {
		columna, silencio = "solicitante", ""
	}
	var out PeticionFirma
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE pdf_peticiones_firma
			SET estado = $3, resuelta = now(), firmado = nullif($4, ''), firmado_ruta = nullif($5, ''), enlace = $6,
			    firmado_huella = nullif($7, ''), ocupada = NULL
			WHERE id = $1::uuid AND `+columna+` = $2 AND estado = 'pendiente'`+silencio+`
			RETURNING `+columnasPeticionFirma, id, quien, estado, firmado, firmadoRuta, enlace, firmadoHuella)
		if err != nil {
			return err
		}
		out, err = pgx.CollectExactlyOneRow(rows, escanearPeticionFirma)
		return err
	})
	if isNoRows(err) {
		return PeticionFirma{}, ErrPeticionFirmaNoEsta
	}
	if err != nil {
		return PeticionFirma{}, fmt.Errorf("store: resolver la petición de firma: %w", err)
	}
	return out, nil
}

// PeticionFirmaPendiente es la pendiente `id` si `destinatario` es a quien se pidió.
func (s *Store) PeticionFirmaPendiente(ctx context.Context, id, destinatario string) (PeticionFirma, error) {
	var out PeticionFirma
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columnasPeticionFirma+` FROM pdf_peticiones_firma
			WHERE id = $1::uuid AND destinatario = $2 AND estado = 'pendiente' AND NOT silenciada`, id, destinatario)
		if err != nil {
			return err
		}
		out, err = pgx.CollectExactlyOneRow(rows, escanearPeticionFirma)
		return err
	})
	if isNoRows(err) {
		return PeticionFirma{}, ErrPeticionFirmaNoEsta
	}
	return out, err
}

// AnfitrionPeticionFirma es la organización donde vive la petición `id` si `sub` es su
// destinatario de FUERA; "" en cualquier otro caso.
func (s *Store) AnfitrionPeticionFirma(ctx context.Context, id, sub string) (string, error) {
	var t *string
	if err := s.pool.QueryRow(ctx, `SELECT docs_firma_anfitrion($1::uuid, $2)`, id, sub).Scan(&t); err != nil {
		return "", fmt.Errorf("store: organización de la petición de firma: %w", err)
	}
	if t == nil {
		return "", nil
	}
	return *t, nil
}

// OcuparPeticionFirma reserva la petición de fuera `id` para una vuelta de «abrir»
// (quien firma, `recibida`, pendiente) o de «traer» (quien pidió, firmada y con el
// enlace del firmado). Dos a la vez dejarían dos copias: la segunda recibe
// [ErrPeticionFirmaOcupada]. La reserva caduca sola a los dos minutos.
func (s *Store) OcuparPeticionFirma(ctx context.Context, id, sub string, recibida bool) (PeticionFirma, error) {
	cond := `destinatario = $2 AND estado = 'pendiente' AND NOT silenciada`
	if !recibida {
		cond = `solicitante = $2 AND estado = 'firmada' AND enlace IS NOT NULL`
	}
	var out PeticionFirma
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE pdf_peticiones_firma SET ocupada = now()
			WHERE id = $1::uuid AND externo AND `+cond+`
			  AND (ocupada IS NULL OR ocupada < now() - interval '2 minutes')
			RETURNING `+columnasPeticionFirma, id, sub)
		if err != nil {
			return err
		}
		out, err = pgx.CollectExactlyOneRow(rows, escanearPeticionFirma)
		if !isNoRows(err) {
			return err
		}
		var hay bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pdf_peticiones_firma
			WHERE id = $1::uuid AND externo AND `+cond+`)`, id, sub).Scan(&hay); err != nil {
			return err
		}
		if hay {
			return ErrPeticionFirmaOcupada
		}
		return ErrPeticionFirmaNoEsta
	})
	return out, err
}

// SoltarPeticionFirma quita la reserva de [Store.OcuparPeticionFirma] si la vuelta falla.
func (s *Store) SoltarPeticionFirma(ctx context.Context, id, sub string) error {
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE pdf_peticiones_firma SET ocupada = NULL
			WHERE id = $1::uuid AND (destinatario = $2 OR solicitante = $2)`, id, sub)
		return err
	})
}

// GuardarCopiaFirma apunta la copia que se trajo quien firma (otra vez, si borró la
// anterior) y suelta la reserva.
func (s *Store) GuardarCopiaFirma(ctx context.Context, id, destinatario, copia, copiaRuta string) (PeticionFirma, error) {
	var out PeticionFirma
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE pdf_peticiones_firma SET copia = $3, copia_ruta = $4, ocupada = NULL
			WHERE id = $1::uuid AND destinatario = $2 AND estado = 'pendiente' AND externo AND NOT silenciada
			RETURNING `+columnasPeticionFirma, id, destinatario, copia, copiaRuta)
		if err != nil {
			return err
		}
		out, err = pgx.CollectExactlyOneRow(rows, escanearPeticionFirma)
		return err
	})
	if isNoRows(err) {
		return PeticionFirma{}, ErrPeticionFirmaNoEsta
	}
	return out, err
}

// GuardarFirmadoTraido apunta la copia firmada que se trajo quien pidió, en SU unidad,
// borra el token del enlace y suelta la reserva. Solo una vez.
func (s *Store) GuardarFirmadoTraido(ctx context.Context, id, solicitante, firmado, firmadoRuta string) (PeticionFirma, error) {
	var out PeticionFirma
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE pdf_peticiones_firma SET firmado = $3, firmado_ruta = $4, enlace = NULL, ocupada = NULL
			WHERE id = $1::uuid AND solicitante = $2 AND estado = 'firmada' AND externo AND enlace IS NOT NULL
			RETURNING `+columnasPeticionFirma, id, solicitante, firmado, firmadoRuta)
		if err != nil {
			return err
		}
		out, err = pgx.CollectExactlyOneRow(rows, escanearPeticionFirma)
		return err
	})
	if isNoRows(err) {
		return PeticionFirma{}, ErrPeticionFirmaNoEsta
	}
	return out, err
}

// OlvidarEnlaceFirma apunta que el enlace del original ya se revocó en Drive.
func (s *Store) OlvidarEnlaceFirma(ctx context.Context, id, solicitante string) error {
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE pdf_peticiones_firma SET enlace_share = NULL
			WHERE id = $1::uuid AND solicitante = $2`, id, solicitante)
		return err
	})
}

// # Bloqueos y política de las peticiones de fuera (mig. 0013, ADR 0059)

var (
	// ErrDemasiadosBloqueos: la persona ya tiene 500 bloqueos.
	ErrDemasiadosBloqueos = errors.New("store: demasiados bloqueos")
	// ErrBloqueoSinSentido: bloquearse a sí misma, o sin nadie a quien bloquear.
	ErrBloqueoSinSentido = errors.New("store: bloqueo sin sentido")
)

// BloqueoFirma es alguien a quien `quien` no quiere recibir peticiones.
type BloqueoFirma struct {
	Bloqueado string    `json:"sub"`
	Nombre    string    `json:"name,omitempty"`
	Creado    time.Time `json:"created_at"`
}

// BloquearFirma apunta que `quien` no recibe peticiones de `bloqueado` y silencia las
// pendientes, en todas las organizaciones. `tenant` es desde qué organización lo hace.
func (s *Store) BloquearFirma(ctx context.Context, quien, bloqueado, nombre, tenant string) error {
	var r int
	if err := s.pool.QueryRow(ctx, `SELECT docs_firma_bloquear($1, $2, $3, $4)`, quien, bloqueado, nombre, tenant).Scan(&r); err != nil {
		return fmt.Errorf("store: bloquear: %w", err)
	}
	switch r {
	case 0:
		return nil
	case 1:
		return ErrDemasiadosBloqueos
	}
	return ErrBloqueoSinSentido
}

// PeticionDeFueraRecibida es la petición `id` que alguien de fuera pidió a
// `destinatario`, esté como esté —para bloquear también tras rechazarla o firmarla—.
// Lo silenciado no: ya no le llega.
func (s *Store) PeticionDeFueraRecibida(ctx context.Context, id, destinatario string) (PeticionFirma, error) {
	var out PeticionFirma
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columnasPeticionFirma+` FROM pdf_peticiones_firma
			WHERE id = $1::uuid AND destinatario = $2 AND externo AND NOT silenciada`, id, destinatario)
		if err != nil {
			return err
		}
		out, err = pgx.CollectExactlyOneRow(rows, escanearPeticionFirma)
		return err
	})
	if isNoRows(err) {
		return PeticionFirma{}, ErrPeticionFirmaNoEsta
	}
	return out, err
}

// DesbloquearFirma quita el bloqueo. Lo que llegó silenciado sigue sin llegar.
func (s *Store) DesbloquearFirma(ctx context.Context, quien, bloqueado string) error {
	if _, err := s.pool.Exec(ctx, `SELECT docs_firma_desbloquear($1, $2)`, quien, bloqueado); err != nil {
		return fmt.Errorf("store: desbloquear: %w", err)
	}
	return nil
}

// BloqueosFirma son los de `quien`, los más recientes antes.
func (s *Store) BloqueosFirma(ctx context.Context, quien string) ([]BloqueoFirma, error) {
	rows, err := s.pool.Query(ctx, `SELECT bloqueado, nombre, creado FROM docs_firma_bloqueos($1)`, quien)
	if err != nil {
		return nil, fmt.Errorf("store: bloqueos: %w", err)
	}
	return pgx.CollectRows(rows, func(f pgx.CollectableRow) (BloqueoFirma, error) {
		var b BloqueoFirma
		return b, f.Scan(&b.Bloqueado, &b.Nombre, &b.Creado)
	})
}

// FirmaBloqueada dice si `destinatario` ha bloqueado a `solicitante`.
func (s *Store) FirmaBloqueada(ctx context.Context, destinatario, solicitante string) (bool, error) {
	var si bool
	if err := s.pool.QueryRow(ctx, `SELECT docs_firma_bloqueado($1, $2)`, destinatario, solicitante).Scan(&si); err != nil {
		return false, fmt.Errorf("store: ¿bloqueado?: %w", err)
	}
	return si, nil
}

// FirmaAdmitida dice si alguna de `orgs` admite peticiones de fuera que vienen de
// `origen`, sin decir cuál.
func (s *Store) FirmaAdmitida(ctx context.Context, orgs []string, origen string) (bool, error) {
	var si bool
	if err := s.pool.QueryRow(ctx, `SELECT docs_firma_admite($1, $2)`, orgs, origen).Scan(&si); err != nil {
		return false, fmt.Errorf("store: ¿admite?: %w", err)
	}
	return si, nil
}

// Modos de la política de firmas de fuera.
const (
	PoliticaAbierta   = "abierta"
	PoliticaCerrada   = "cerrada"
	PoliticaConfianza = "confianza"
)

// MaxConfianza son las organizaciones de confianza que admite una política.
const MaxConfianza = 100

// PoliticaFirma es la de la organización de la sesión.
type PoliticaFirma struct {
	Modo           string     `json:"mode"`
	Confianza      []string   `json:"trusted"`
	Actualizada    *time.Time `json:"updated_at,omitempty"`
	ActualizadaPor string     `json:"updated_by,omitempty"`
}

// PoliticaDeFirma es la de la organización del contexto; sin fila, abierta.
func (s *Store) PoliticaDeFirma(ctx context.Context) (PoliticaFirma, error) {
	out := PoliticaFirma{Modo: PoliticaAbierta, Confianza: []string{}}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		var act time.Time
		err := tx.QueryRow(ctx, `SELECT modo, confianza, actualizada, actualizada_por FROM pdf_firma_politica
			WHERE tenant = current_setting('app.tenant', true)`).Scan(&out.Modo, &out.Confianza, &act, &out.ActualizadaPor)
		if isNoRows(err) {
			return nil
		}
		out.Actualizada = &act
		return err
	})
	if err != nil {
		return PoliticaFirma{}, fmt.Errorf("store: política de firmas: %w", err)
	}
	return out, nil
}

// GuardarPoliticaDeFirma deja la de la organización del contexto.
func (s *Store) GuardarPoliticaDeFirma(ctx context.Context, p PoliticaFirma, quien string) (PoliticaFirma, error) {
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO pdf_firma_politica (tenant, modo, confianza, actualizada, actualizada_por)
			VALUES (current_setting('app.tenant', true), $1, $2, now(), $3)
			ON CONFLICT (tenant) DO UPDATE SET modo = EXCLUDED.modo, confianza = EXCLUDED.confianza,
			    actualizada = now(), actualizada_por = EXCLUDED.actualizada_por`, p.Modo, p.Confianza, quien)
		return err
	})
	if err != nil {
		return PoliticaFirma{}, fmt.Errorf("store: guardar la política de firmas: %w", err)
	}
	return s.PoliticaDeFirma(ctx)
}
