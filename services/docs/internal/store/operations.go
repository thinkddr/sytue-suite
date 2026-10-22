// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// EventChannel es el canal lógico de eventos de Docs (events.Channel).
const EventChannel events.Channel = "docs"

// EventDocUpdated es el tipo del evento que AppendOps publica en el outbox por
// cada lote de ops. Su payload lleva document_id, base_seq (seq previa al lote)
// y seq (seq final): el rango (base_seq, seq] identifica exactamente las ops
// del lote para el fan-out cross-réplica (releer y difundir en cada réplica).
const EventDocUpdated = "docs.doc.updated"

// AppendOps secuencia y persiste un lote de operaciones CRDT de forma atómica:
// bloquea el documento, asigna seq monótonos, inserta en doc_operations, sube
// documents.seq y publica un evento "docs.doc.updated" en el outbox — todo en
// la MISMA transacción (atomicidad). Devuelve las operaciones con su seq y la
// nueva seq del documento.
//
// Concurrencia: usa `SELECT ... FOR UPDATE` sobre la fila del documento para
// serializar la asignación de seq; no hay carrera entre escritores.
//
// Con baseSeq != nil la escritura es condicional (compare-and-swap): si alguna
// clave de los `set` del lote se escribió con seq > baseSeq, ErrStaleSeq y no se
// persiste nada. La comprobación va bajo el mismo bloqueo, así que no hay ventana
// entre mirar y escribir.
//
//nolint:gocognit // una transacción lineal: bloquear, autorizar, proteger, insertar, publicar.
func (s *Store) AppendOps(ctx context.Context, docID string, author domain.Subject, ops []domain.Op, baseSeq *int64) ([]domain.SequencedOp, int64, error) {
	if author.Sub == "" {
		return nil, 0, domain.ErrForbidden
	}
	authorSub := author.Sub
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, 0, err
	}
	for i := range ops {
		if err := ops[i].Validate(); err != nil {
			return nil, 0, err
		}
	}

	var (
		out    []domain.SequencedOp
		newSeq int64
	)

	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var curSeq int64
		var doc domain.Document
		out, newSeq = nil, 0 // la transacción puede reintentarse
		err := tx.QueryRow(ctx,
			`SELECT seq, owner_sub, kind FROM documents WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`,
			docID).Scan(&curSeq, &doc.OwnerSub, &doc.Kind)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		role, err := authorizeWriteInTx(ctx, tx, docID, &doc, author)
		if err != nil {
			return err
		}
		// base_seq solo mira los `set` que manda el cliente: los parches no chocan.
		if err := comprobarBase(ctx, tx, docID, baseSeq, curSeq, ops); err != nil {
			return err
		}
		lote, err := materializarParches(ctx, tx, docID, doc.Kind, ops)
		if err != nil {
			return err
		}
		// El HTML del contenido se sanea aquí (domain/saneado_html.go): con los parches ya
		// convertidos en el contenido que dejan, y ANTES de la protección, que compara y
		// guarda lo que de verdad se va a escribir.
		domain.SanearOps(lote)
		if len(lote) == 0 {
			newSeq = curSeq // todo eran parches ya aplicados (reintento) o sin efecto
		}

		// Protección de hojas y libros (domain/proteccion.go), con el documento ya
		// bloqueado: lo que se compara es lo vigente, sin carreras entre escritores.
		prot, lote, err := protegerEnTx(ctx, tx, docID, doc.Kind, role, author, lote)
		if err != nil {
			return err
		}

		out, newSeq, err = insertarOpsEnTx(ctx, tx, t.Slug, docID, authorSub, curSeq, lote)
		if err != nil {
			return err
		}
		if prot != nil {
			if err := guardarProteccion(ctx, tx, t.Slug, docID, prot.Estado, prot.Poner, prot.Quitar); err != nil {
				return err
			}
			if prot.SecuenciaCambia {
				return guardarSecuencia(ctx, tx, docID, prot.Estado.Secuencia)
			}
		}
		return nil
	})
	if err != nil {
		if errorDeDominio(err) {
			return nil, 0, err
		}
		return nil, 0, fmt.Errorf("store: append ops: %w", err)
	}
	return out, newSeq, nil
}

// insertarOpsEnTx inserta las ops con seq consecutivos a partir de curSeq, sube
// documents.seq y publica el evento: el cuerpo común de AppendOps y de la
// creación de un documento con contenido inicial (una sola transacción).
func insertarOpsEnTx(ctx context.Context, tx pgx.Tx, tenant, docID, authorSub string, curSeq int64, ops []domain.Op) ([]domain.SequencedOp, int64, error) {
	// Lo que se valida del contenido va AQUÍ, que es por donde pasa todo lo que se
	// guarda: POST /ops, el WebSocket, restaurar una versión, los parches ya
	// materializados y el contenido con el que nace un documento.
	if err := domain.ValidarMedios(ops); err != nil {
		return nil, 0, err
	}
	out := make([]domain.SequencedOp, 0, len(ops))
	newSeq := curSeq
	for _, op := range ops {
		newSeq++
		lamport, err := lamportInt64(op.Lamport)
		if err != nil {
			return nil, 0, err
		}
		raw, err := json.Marshal(op)
		if err != nil {
			return nil, 0, fmt.Errorf("serializar op: %w", err)
		}
		const ins = `
			INSERT INTO doc_operations
			    (document_id, seq, tenant, actor, author_sub, lamport, op)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`
		if _, err := tx.Exec(ctx, ins,
			docID, newSeq, tenant, op.Dot.Actor, authorSub, lamport,
			json.RawMessage(raw), // jsonb (no bytea)
		); err != nil {
			return nil, 0, fmt.Errorf("insertar op: %w", err)
		}
		out = append(out, domain.SequencedOp{
			DocumentID: docID, Seq: newSeq, Actor: op.Dot.Actor,
			AuthorSub: authorSub, Lamport: lamport, Op: op,
		})
	}
	if _, err := tx.Exec(ctx,
		`UPDATE documents SET seq = $2, updated_at = now() WHERE id = $1`,
		docID, newSeq); err != nil {
		return nil, 0, fmt.Errorf("actualizar seq: %w", err)
	}
	// Evento transaccional (mismo tx que la escritura): el Hub/notificaciones
	// y otros consumidores se enteran de la actualización tras el commit.
	ev, err := events.NewEvent(EventChannel, EventDocUpdated, tenant, map[string]any{
		"document_id": docID, "seq": newSeq, "base_seq": curSeq, "author_sub": authorSub,
	})
	if err != nil {
		return nil, 0, err
	}
	return out, newSeq, events.Publish(ctx, tx, ev)
}

// appendOpsTx bloquea el documento recién creado y le añade sus ops semilla, sin
// autorización ni protección: solo lo usa CreateDocument, donde el autor es el
// propietario y el documento aún no tiene nada que proteger.
func appendOpsTx(ctx context.Context, tx pgx.Tx, tenant, docID, authorSub string, ops []domain.Op) ([]domain.SequencedOp, int64, error) {
	var curSeq int64
	err := tx.QueryRow(ctx,
		`SELECT seq FROM documents WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`,
		docID).Scan(&curSeq)
	if isNoRows(err) {
		return nil, 0, domain.ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	domain.SanearOps(ops) // el contenido inicial entra por la misma regla que una edición
	return insertarOpsEnTx(ctx, tx, tenant, docID, authorSub, curSeq, ops)
}

// errorDeDominio: los errores que AppendOps devuelve tal cual, sin envolver.
func errorDeDominio(err error) bool {
	return errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrProtegido) ||
		errors.Is(err, domain.ErrClaveProteccion) || errors.Is(err, domain.ErrProteccionPendiente) ||
		errors.Is(err, domain.ErrSecuenciaPendiente) ||
		errors.Is(err, domain.ErrStaleSeq)
}

// La misma fila bloqueada serializa las escrituras y las mutaciones de ACL.
// Una autorización del servicio anterior a una revocación no concede escritura.
// Devuelve el rol efectivo: la protección distingue al propietario de un editor.
func authorizeWriteInTx(ctx context.Context, tx pgx.Tx, docID string, doc *domain.Document, author domain.Subject) (domain.Role, error) {
	if doc.OwnerSub != "" && doc.OwnerSub == author.Sub {
		return domain.RoleOwner, nil
	}
	perms, err := listPermissionsInTx(ctx, tx, docID)
	if err != nil {
		return "", err
	}
	role, ok := domain.ResolveRole(doc, perms, author)
	if !ok || !role.CanWrite() {
		return "", domain.ErrForbidden
	}
	return role, nil
}

// comprobarBase devuelve ErrStaleSeq si alguna clave de los `set` de ops tiene
// una escritura posterior a baseSeq. Recorre solo las ops desde baseSeq (índice
// (document_id, seq)) y para en la primera que coincide. Sin base, o con la base
// al día (curSeq), no hay nada que mirar.
func comprobarBase(ctx context.Context, tx pgx.Tx, docID string, baseSeq *int64, curSeq int64, ops []domain.Op) error {
	if baseSeq == nil || *baseSeq >= curSeq {
		return nil
	}
	var claves []string
	for _, op := range ops {
		if op.Kind == domain.OpSet {
			claves = append(claves, op.Key)
		}
	}
	if len(claves) == 0 {
		return nil
	}
	var obsoleta bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM doc_operations
			WHERE document_id = $1 AND seq > $2
			  AND op->>'kind' = 'set' AND op->>'key' = ANY($3))`,
		docID, *baseSeq, claves).Scan(&obsoleta)
	if err != nil {
		return fmt.Errorf("comprobar base_seq: %w", err)
	}
	if obsoleta {
		return domain.ErrStaleSeq
	}
	return nil
}

// OpsSince devuelve las operaciones del documento con seq > sinceSeq, en orden,
// para que un cliente que reconecta haga replay desde su último seq conocido.
func (s *Store) OpsSince(ctx context.Context, docID string, sinceSeq int64, limit int) ([]domain.SequencedOp, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	out := make([]domain.SequencedOp, 0, limit)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		const q = `
			SELECT seq, actor, author_sub, lamport, op
			FROM doc_operations
			WHERE document_id = $1 AND seq > $2
			ORDER BY seq ASC
			LIMIT $3`
		rows, err := tx.Query(ctx, q, docID, sinceSeq, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var so domain.SequencedOp
			var raw []byte
			if err := rows.Scan(&so.Seq, &so.Actor, &so.AuthorSub, &so.Lamport, &raw); err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &so.Op); err != nil {
				return fmt.Errorf("deserializar op seq %d: %w", so.Seq, err)
			}
			so.DocumentID = docID
			out = append(out, so)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: ops desde seq: %w", err)
	}
	return out, nil
}

// lamportInt64 valida que el reloj de Lamport del CRDT (uint64) quepa en el
// BIGINT de PostgreSQL; un valor mayor solo puede venir de un cliente hostil y
// se rechaza en vez de truncarse en negativo (gosec G115).
func lamportInt64(v uint64) (int64, error) {
	if v > math.MaxInt64 {
		return 0, fmt.Errorf("store: lamport %d fuera de rango", v)
	}
	return int64(v), nil
}

// materializarParches sustituye cada `patch` del lote por el `set` del registro de
// contenido que resulta de aplicarlo al valor vigente (parche.go). Va con el
// documento bloqueado, así que "vigente" es lo último escrito: los parches se
// aplican en el orden en que entran. Se descartan los que ya constan en el log
// (mismo op_id: reintento o replay) y los que no cambian nada.
func materializarParches(ctx context.Context, tx pgx.Tx, docID string, kind domain.Kind, ops []domain.Op) ([]domain.Op, error) {
	hay := false
	for i := range ops {
		hay = hay || ops[i].Kind == domain.OpPatch
	}
	if !hay {
		return ops, nil
	}
	vigente := map[string]*registroVigente{} // por clave: el registro tras los parches del lote
	vistos := map[string]struct{}{}
	out := make([]domain.Op, 0, len(ops))
	for _, op := range ops {
		if op.Kind != domain.OpPatch {
			out = append(out, op)
			continue
		}
		if op.Key != domain.ContentKey(kind) {
			return nil, fmt.Errorf("%w: patch de %s en un documento de otro tipo", domain.ErrInvalidOp, op.Key)
		}
		if _, dup := vistos[op.OpID]; dup {
			continue
		}
		vistos[op.OpID] = struct{}{}
		var ya bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM doc_operations WHERE document_id = $1 AND op->>'op_id' = $2)`,
			docID, op.OpID).Scan(&ya); err != nil {
			return nil, fmt.Errorf("comprobar op_id: %w", err)
		}
		if ya {
			continue
		}
		reg, ok := vigente[op.Key]
		if !ok {
			var err error
			if reg, err = leerVigente(ctx, tx, docID, op.Key); err != nil {
				return nil, err
			}
			vigente[op.Key] = reg
		}
		nuevo, err := domain.AplicarCambios(reg.contenido, op.Cambios)
		if err != nil {
			return nil, err
		}
		if nuevo == reg.contenido {
			continue
		}
		valor, err := json.Marshal(nuevo)
		if err != nil {
			return nil, fmt.Errorf("serializar contenido: %w", err)
		}
		reg.lamport++ // gana al registro que sustituye (LWW) en cualquier réplica
		reg.contenido = nuevo
		// El Dot sale del op_id: único por parche, y reintentar da el mismo.
		out = append(out, domain.Op{
			Kind: domain.OpSet, Key: op.Key, Value: valor, Lamport: reg.lamport,
			Dot:  domain.Dot{Actor: "patch:" + op.OpID, Counter: 1},
			OpID: op.OpID, Cambios: op.Cambios, Base: op.Base,
		})
	}
	return out, nil
}

type registroVigente struct {
	contenido string
	lamport   uint64
}

// leerVigente devuelve el valor ganador (LWW) del registro `clave`: mayor Lamport
// y, a igualdad, mayor Dot, que es el orden de State.Apply.
func leerVigente(ctx context.Context, tx pgx.Tx, docID, clave string) (*registroVigente, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `
		SELECT op FROM doc_operations
		WHERE document_id = $1 AND op->>'kind' = 'set' AND op->>'key' = $2
		ORDER BY lamport DESC, (op->'dot'->>'c')::numeric DESC, op->'dot'->>'a' DESC
		LIMIT 1`, docID, clave).Scan(&raw)
	if isNoRows(err) {
		return nil, fmt.Errorf("%w: no hay contenido sobre el que aplicar el parche", domain.ErrInvalidOp)
	}
	if err != nil {
		return nil, fmt.Errorf("leer registro vigente: %w", err)
	}
	var op domain.Op
	if err := json.Unmarshal(raw, &op); err != nil {
		return nil, fmt.Errorf("deserializar registro vigente: %w", err)
	}
	var contenido string
	if err := json.Unmarshal(op.Value, &contenido); err != nil {
		return nil, fmt.Errorf("%w: el registro vigente no es un modelo", domain.ErrInvalidOp)
	}
	return &registroVigente{contenido: contenido, lamport: op.Lamport}, nil
}
