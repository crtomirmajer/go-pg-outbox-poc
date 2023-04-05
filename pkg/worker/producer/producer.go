package producer

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/bluelabs-eu/pg-outbox/pkg/message"
	"github.com/bluelabs-eu/pg-outbox/pkg/user"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

const (
	numSimulatedUsers = 100000
)

type Producer struct {
	txOptions pgx.TxOptions
	conn      *pgx.Conn
}

func New(ctx context.Context, connString string) (*Producer, error) {
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		panic(err)
	}
	txOptions := pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	}
	return &Producer{
		txOptions: txOptions,
		conn:      conn,
	}, nil
}

func (p *Producer) Run(ctx context.Context) error {
	var i uint64
	for ctx.Err() != context.Canceled {
		// Simulate serial changes on users
		m, err := p.ExecuteCommand(ctx, i)
		if err != nil {
			return err
		}
		if i%10000 == 0 {
			log.Info().
				Uint64("iteration", i).
				Str("msg", string(m.Payload)).
				Msg("write-finished")
		}
		i += 1
	}
	return nil
}

func (p *Producer) ExecuteCommand(ctx context.Context, iteration uint64) (message.Message, error) {
	// start business logic; it's a POC, so let's just fake some data :)

	userID := fmt.Sprintf("%d", rand.Intn(numSimulatedUsers))

	u := user.User{
		ID:        userID,
		FirstName: "user" + userID,
		Details: map[string]interface{}{
			"key":    time.Now().Unix(),
			"random": rand.Int(),
		},
		BirthDate: time.Now(),
	}

	payload := u.Serialize()
	m := message.Message{
		ID:      fmt.Sprintf("%d", iteration),
		Time:    time.Now(), // used to measure end-to-end latency
		Payload: payload,
	}

	// end business logic

	err := p.Persist(ctx, u, m)
	return m, err
}

func (p *Producer) Persist(ctx context.Context, u user.User, m message.Message) error {
	tx, err := p.conn.BeginTx(ctx, p.txOptions)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // takes no effect if tx is already commited

	// upsert user and send message thought the database, inside the same transaction

	err = p.Upsert(ctx, tx, u)
	if err != nil {
		return err
	}

	err = p.Send(ctx, tx, m)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (p *Producer) Upsert(ctx context.Context, tx pgx.Tx, u user.User) error {
	_, err := tx.Exec(
		ctx,
		`
		insert into users(id, first_name, details, birth_date) 
			values ($1, $2, $3, $4) 
		on conflict (id) do update 
		set first_name = excluded.first_name,
			details = excluded.details,
			birth_date = excluded.birth_date;
		`,
		u.ID, u.FirstName, u.Details, u.BirthDate,
	)
	return err
}

func (p *Producer) Send(ctx context.Context, tx pgx.Tx, m message.Message) error {
	_, err := tx.Exec(
		ctx,
		`select * from pg_logical_emit_message(true, 'outbox', $1)`,
		m.Serialize(), // TODO: move serialization outside transaction, keep transactions as lean as possible
	)
	return err
}

func (p *Producer) Close(ctx context.Context) error {
	return p.conn.Close(ctx)
}
