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
)

const (
	keepAliveFrequency = 10 * time.Second
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

	for ctx.Err() != context.Canceled {
		if time.Now().After(nextStandbyMessageDeadline) {
			// send status update evey N seconds
			err := pglogrepl.SendStandbyStatusUpdate(
				ctx,
				p.conn,
				pglogrepl.StandbyStatusUpdate{WALWritePosition: clientXLogPos},
			)
			if err != nil {
				return fmt.Errorf("failed to send standby update: %v", err)
			}
			nextStandbyMessageDeadline = time.Now().Add(keepAliveFrequency)
		}

		ctx, cancel := context.WithTimeout(ctx, keepAliveFrequency)
		defer cancel()

		rawMsg, err := p.conn.ReceiveMessage(ctx)
		if pgconn.Timeout(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("something went wrong while listening for message: %v", err)
		}

		var msg *message.Message
		var pos *pglogrepl.LSN
		pos, msg, err = parseMessage(rawMsg)
		if err != nil {
			return err
		}
		if msg != nil {
			fmt.Println("latency: ", time.Since(msg.Time), "ID: ", msg.ID)
		}
		if pos != nil {
			clientXLogPos = *pos
		}
	}

	return nil
}

func (p *Consumer) Close(ctx context.Context) error {
	return p.conn.Close(ctx)
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
		clientXLogPos := xld.WALStart + pglogrepl.LSN(len(xld.WALData))

		switch xld.WALData[0] {
		case 'M': // marks logical-decoding-message
			m := new(LogicalDecodingMessage)
			err := m.Decode(xld.WALData)
			if err != nil {
				return &clientXLogPos, nil, fmt.Errorf("error decoding wal-data: %v", err)
			}
			msg := &message.Message{}
			err = json.Unmarshal(m.Content, msg)
			if err != nil {
				return &clientXLogPos, nil, fmt.Errorf("error unmarshaling logical-decoding-message: %v \n\n %v \n\n %+v \n\n %v", err, string(protoMsg.Data), *m, string(m.Content))
			}
			return &clientXLogPos, msg, nil
		}
		// TODO: this code is hit on BEGIN and COMMIT - ignore for now, for simplicity
		return &clientXLogPos, nil, nil
	}
	return nil, nil, fmt.Errorf("unknown byte ID: %v", protoMsg.Data[0])
}
