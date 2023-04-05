package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bluelabs-eu/pg-outbox/pkg/message"
	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/rs/zerolog/log"
)

const (
	keepAliveFrequency = 5 * time.Second
	outputPlugin       = "pgoutput"
	publicationName    = "outbox_publication"
	slotName           = "outbox_slot"
)

type Consumer struct {
	conn *pgconn.PgConn
}

func New(ctx context.Context, connString string) (*Consumer, error) {
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		panic(err)
	}
	pgConn := conn.PgConn()
	return &Consumer{
		conn: pgConn,
	}, nil
}

func (p *Consumer) Close(ctx context.Context) error {
	return p.conn.Close(ctx)
}

func (p *Consumer) Run(ctx context.Context) error {

	pluginArguments := []string{
		"proto_version '1'",
		"messages 'true'",   // allow logical-decoding-messages
		"binary 'true'",     // use binary format
		"streaming 'false'", // prevent uncommited data from appearing in the stream
		fmt.Sprintf("publication_names '%s'", publicationName),
	}

	sysident, err := pglogrepl.IdentifySystem(ctx, p.conn)
	if err != nil {
		return fmt.Errorf("identify-system failed: %v", err)
	}
	clientXLogPos := sysident.XLogPos

	log.Info().Any("lsn", clientXLogPos).Msg("start-position")

	err = pglogrepl.StartReplication(
		ctx,
		p.conn,
		slotName,
		clientXLogPos,
		pglogrepl.StartReplicationOptions{
			Mode:       pglogrepl.LogicalReplication,
			PluginArgs: pluginArguments,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to establish start replication: %v", err)
	}

	nextStandbyMessageDeadline := time.Now().Add(keepAliveFrequency)

	idx := 0
	for ctx.Err() != context.Canceled {

		if time.Now().After(nextStandbyMessageDeadline) {
			// send status update evey N seconds
			err := pglogrepl.SendStandbyStatusUpdate(
				ctx,
				p.conn,
				pglogrepl.StandbyStatusUpdate{
					WALWritePosition: clientXLogPos,
					WALFlushPosition: clientXLogPos,
					WALApplyPosition: clientXLogPos,
				},
			)

			log.Info().
				Any("reported-lsn", clientXLogPos).
				Msg("status-updated")

			if err != nil {
				return fmt.Errorf("failed to send standby update: %v", err)
			}
			nextStandbyMessageDeadline = time.Now().Add(keepAliveFrequency)
		}

		ctxTemp, cancel := context.WithDeadline(context.Background(), nextStandbyMessageDeadline)
		rawMsg, err := p.conn.ReceiveMessage(ctxTemp)
		cancel()
		if err != nil {
			if pgconn.Timeout(err) {
				continue
			}
			return fmt.Errorf("something went wrong while listening for message: %v", err)
		}

		var msg *message.Message
		var pos *pglogrepl.LSN
		pos, msg, err = parseMessage(rawMsg)
		if err != nil {
			return err
		}
		if pos != nil {
			// available on COMMIT message
			clientXLogPos = *pos
		}
		if msg != nil {
			if idx%10000 == 0 {
				log.Info().
					Any("last-committed-lsn", clientXLogPos).
					Dur("latency", time.Since(msg.Time)).
					Str("ID", msg.ID).
					Str("payload", string(msg.Payload)).
					Msg("message-received")
			}
			idx++
		}
	}

	return nil
}

func parseMessage(rawMsg pgproto3.BackendMessage) (*pglogrepl.LSN, *message.Message, error) {

	protoMsg, ok := rawMsg.(*pgproto3.CopyData)
	if !ok {
		return nil, nil, fmt.Errorf("received unexpected message: %v", rawMsg)
	}

	switch protoMsg.Data[0] {
	case pglogrepl.PrimaryKeepaliveMessageByteID:
		// TODO: use for keep-alive signaling
		return nil, nil, nil
	case pglogrepl.XLogDataByteID:
		xld, err := pglogrepl.ParseXLogData(protoMsg.Data[1:])
		if err != nil {
			return nil, nil, fmt.Errorf("parseXLogData failed: %v", err)
		}
		return parseData(xld)
	}
	return nil, nil, fmt.Errorf("unknown byte ID: %v", protoMsg.Data[0])
}

func parseData(xld pglogrepl.XLogData) (*pglogrepl.LSN, *message.Message, error) {
	// function to parse XLogData
	size := len(xld.WALData)
	if size == 0 {
		return nil, nil, fmt.Errorf("wal-data is missing")
	}
	clientXLogPos := xld.WALStart + pglogrepl.LSN(size)

	// only consider BEGIN, MESSAGE and COMMIT records
	switch xld.WALData[0] {
	case 'M': // marks logical-decoding-message
		m := new(LogicalDecodingMessage)
		err := m.Decode(xld.WALData)
		if err != nil {
			return nil, nil, fmt.Errorf("error decoding %T: %v", m, err)
		}
		msg := &message.Message{}
		err = json.Unmarshal(m.Content, msg)
		if err != nil {
			return nil, nil, fmt.Errorf("error unmarshaling logical-decoding-message: %v :: %v", err, string(m.Content))
		}
		return nil, msg, nil
	case 'C': // marks commit
		m := new(pglogrepl.CommitMessage)
		err := m.Decode(xld.WALData)
		if err != nil {
			return nil, nil, fmt.Errorf("error decoding %T: %v", m, err)
		}
		// return LSN of COMMIT
		return &clientXLogPos, &message.Message{}, nil
	case 'B': // marks begin
		return nil, nil, nil
	}

	// NOTE: this code shouldn't be hit given how the PUBLICATION is configured
	log.Warn().
		Any("lsn", clientXLogPos).
		Str("type", string(xld.WALData[0])).
		Msg("unexpected-message-received")

	// NOTE: we could as-well return error here, for the sake of simplicity, just ignore
	return nil, nil, nil
}
