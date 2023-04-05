// NOTE: this should be implemented in pglogrepl

package consumer

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/jackc/pglogrepl"
)

type baseMessage struct {
	msgType pglogrepl.MessageType
}

// Type returns message type.
func (m *baseMessage) Type() pglogrepl.MessageType {
	return m.msgType
}

// SetType sets message type.
// This method is added to help writing test code in application.
// The message type is still defined by message data.
func (m *baseMessage) SetType(t pglogrepl.MessageType) {
	m.msgType = t
}

// Decode parse src into message struct. The src must contain the complete message starts after
// the first message type byte.
func (m *baseMessage) Decode(_ []byte) error {
	return fmt.Errorf("message decode not implemented")
}

// If there is no null byte in src, return -1.
func (m *baseMessage) decodeString(src []byte) (string, int) {
	end := bytes.IndexByte(src, byte(0))
	if end == -1 {
		return "", -1
	}
	// Trim the last null byte before converting it to a Golang string, then we can
	// compare the result string with a Golang string literal.
	return string(src[:end]), end + 1
}

func (m *baseMessage) decodeLSN(src []byte) (pglogrepl.LSN, int) {
	return pglogrepl.LSN(binary.BigEndian.Uint64(src)), 8
}

func (m *baseMessage) decodeUint32(src []byte) (uint32, int) {
	return binary.BigEndian.Uint32(src), 4
}

func (m *baseMessage) decodeInt32(src []byte) (int32, int) {
	asUint32, size := m.decodeUint32(src)
	return int32(asUint32), size
}

/*
https://www.postgresql.org/docs/current/protocol-logicalrep-message-formats.html

Message:
	Byte1('M')				|	Identifies the message as a logical decoding message.
	Int32 (TransactionId)	|	Xid of the transaction (only present for streamed transactions). This field is available since protocol version 2.
	Int8					|	Flags; Either 0 for no flags or 1 if the logical decoding message is transactional.
	Int64 (XLogRecPtr)		| 	The LSN of the logical decoding message.
	String					|	The prefix of the logical decoding message.
	Int32					|	Length of the content.
	Byten					|	The content of the logical decoding message.
*/

type LogicalDecodingMessage struct {
	baseMessage
	TransactionID uint32
	Transactional uint8
	LSN           pglogrepl.LSN
	Prefix        string
	ContentLength uint32
	Content       []byte
}

// Decode decodes the message from src.
func (m *LogicalDecodingMessage) Decode(src []byte) error {
	var low, used int
	m.TransactionID, used = m.decodeUint32(src[low:])
	low += used
	m.Transactional = uint8(src[low])
	low++
	m.LSN, used = m.decodeLSN(src[low:])
	low += used
	m.Prefix, used = m.decodeString(src[low:])
	low += used
	m.ContentLength, used = m.decodeUint32(src[low:])
	low += used
	m.Content = src[low:]
	return nil
}
