// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// SetCardVote marca un valor de la tarjeta de un mensaje en nombre del usuario: la
// opción de una encuesta o el "done" de una tarea. Idempotente. En una encuesta de
// respuesta ÚNICA (Multi=false) sustituye el voto anterior en la misma transacción,
// para que no queden dos votos de la misma persona ni haga falta que el cliente
// borre primero (dos viajes y una ventana en la que el voto se cuenta doble).
//
// Devuelve el canal (para el fan-out) y el recuento ya agregado, de modo que el
// evento WS lleve el resultado y ningún cliente tenga que releer el mensaje.
func (s *Store) SetCardVote(ctx context.Context, messageID uuid.UUID, subject, value string) (uuid.UUID, []domain.CardVote, error) {
	t := tenancy.MustFrom(ctx)
	var channelID uuid.UUID
	var votes []domain.CardVote
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		card, err := cardOfMessageTx(ctx, tx, t.Slug, messageID, &channelID)
		if err != nil {
			return err
		}
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		if !domain.ValidCardValue(card, value) {
			return domain.ErrInvalidInput
		}
		if card.Kind == domain.CardPoll && !card.Multi {
			if _, err := tx.Exec(ctx,
				`DELETE FROM message_card_votes
				 WHERE tenant=$1 AND message_id=$2 AND subject=$3 AND value<>$4`,
				t.Slug, messageID, subject, value); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO message_card_votes (tenant, message_id, subject, value)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (tenant, message_id, subject, value) DO NOTHING`,
			t.Slug, messageID, subject, value); err != nil {
			return err
		}
		votes, err = cardVotesTx(ctx, tx, t.Slug, messageID)
		return err
	})
	return channelID, votes, err
}

// ClearCardVote retira el valor marcado por el usuario (desvotar una opción,
// reabrir una tarea). Devuelve el canal y el recuento agregado resultante.
func (s *Store) ClearCardVote(ctx context.Context, messageID uuid.UUID, subject, value string) (uuid.UUID, []domain.CardVote, error) {
	t := tenancy.MustFrom(ctx)
	var channelID uuid.UUID
	var votes []domain.CardVote
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := cardOfMessageTx(ctx, tx, t.Slug, messageID, &channelID); err != nil {
			return err
		}
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM message_card_votes
			 WHERE tenant=$1 AND message_id=$2 AND subject=$3 AND value=$4`,
			t.Slug, messageID, subject, value); err != nil {
			return err
		}
		votes, err = cardVotesTx(ctx, tx, t.Slug, messageID)
		return err
	})
	return channelID, votes, err
}

// cardJSON serializa la tarjeta para la columna `card`: sin tarjeta devuelve el
// objeto vacío, que es el DEFAULT de la columna, de modo que la lectura no tenga
// que distinguir NULL de «{}».
//
// Vive aquí y no dentro de createMessageTx porque esa transacción ya rozaba el
// techo de complejidad que vigila el linter: cada rama que se le añade se paga en
// una función que ya es larga de leer.
func cardJSON(c *domain.Card) ([]byte, error) {
	if c == nil || !c.HasCard() {
		return []byte("{}"), nil
	}
	return json.Marshal(c)
}

// cardOfMessageTx lee la tarjeta de un mensaje vivo y deja su canal en channelID.
// ErrNotFound si el mensaje no existe; ErrInvalidInput si no lleva tarjeta (votar
// un mensaje normal no es un fallo del servidor, es una petición sin sentido).
func cardOfMessageTx(ctx context.Context, tx pgx.Tx, tenant string, messageID uuid.UUID, channelID *uuid.UUID) (domain.Card, error) {
	var raw []byte
	err := tx.QueryRow(ctx,
		`SELECT channel_id, card FROM messages
		 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
		tenant, messageID).Scan(channelID, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Card{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Card{}, err
	}
	var card domain.Card
	if err := json.Unmarshal(raw, &card); err != nil {
		return domain.Card{}, err
	}
	if !card.HasCard() {
		return domain.Card{}, domain.ErrInvalidInput
	}
	return card, nil
}

// cardVotesTx agrega el estado de la tarjeta de UN mensaje (tras mutarlo).
func cardVotesTx(ctx context.Context, tx pgx.Tx, tenant string, messageID uuid.UUID) ([]domain.CardVote, error) {
	rows, err := tx.Query(ctx,
		`SELECT value, subject FROM message_card_votes
		 WHERE tenant=$1 AND message_id=$2
		 ORDER BY value, created_at`, tenant, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.CardVote
	byValue := map[string]int{}
	for rows.Next() {
		var value, subject string
		if err := rows.Scan(&value, &subject); err != nil {
			return nil, err
		}
		i, ok := byValue[value]
		if !ok {
			out = append(out, domain.CardVote{Value: value})
			i = len(out) - 1
			byValue[value] = i
		}
		out[i].Count++
		out[i].Subs = append(out[i].Subs, subject)
	}
	return out, rows.Err()
}

// attachCardVotes carga el estado de las tarjetas de un LOTE de mensajes en una sola
// consulta (sin N+1) y lo asigna a cada uno, igual que attachReactions. Los mensajes
// sin tarjeta se saltan: no se consulta nada si en la página no hay ninguna.
func attachCardVotes(ctx context.Context, tx pgx.Tx, tenant string, msgs []domain.Message) error {
	var ids []uuid.UUID
	idx := make(map[uuid.UUID]int, len(msgs))
	for i, m := range msgs {
		if m.Card == nil {
			continue
		}
		ids = append(ids, m.ID)
		idx[m.ID] = i
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx,
		`SELECT message_id, value, subject FROM message_card_votes
		 WHERE tenant=$1 AND message_id = ANY($2)
		 ORDER BY value, created_at`, tenant, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	// agg[messageID][value] -> índice en el slice de votos de esa tarjeta.
	agg := make(map[uuid.UUID]map[string]int)
	for rows.Next() {
		var mid uuid.UUID
		var value, subject string
		if err := rows.Scan(&mid, &value, &subject); err != nil {
			return err
		}
		i, ok := idx[mid]
		if !ok {
			continue
		}
		byValue := agg[mid]
		if byValue == nil {
			byValue = make(map[string]int)
			agg[mid] = byValue
		}
		vi, ok := byValue[value]
		if !ok {
			msgs[i].Card.Votes = append(msgs[i].Card.Votes, domain.CardVote{Value: value})
			vi = len(msgs[i].Card.Votes) - 1
			byValue[value] = vi
		}
		msgs[i].Card.Votes[vi].Count++
		msgs[i].Card.Votes[vi].Subs = append(msgs[i].Card.Votes[vi].Subs, subject)
	}
	return rows.Err()
}
