package proxy

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"encoding/json"
)

var byteOrder = binary.LittleEndian
var ErrMsgTooLarge = errors.New("message too large")

const (
	// 1 MB
	MaxOutgoingMsgLen = 1024 * 1024
)

func ReadMessage(r io.Reader) ([]byte, error) {
	var msgLen uint32
	if err := binary.Read(r, byteOrder, &msgLen); err != nil {
		return nil, err
	}

	if msgLen < 1 {
		log.Print("read message length of 0")
		return nil, nil
	}

	msgData := make([]byte, msgLen)
	if _, err := io.ReadFull(r, msgData); err != nil {
		return nil, err
	}
	return msgData, nil
}

func SendMessage(w io.Writer, msg interface{}) error {
	msgData, err := json.MarshalIndent(msg, "", "  ");
	if err != nil {
		return err
	}

	msgLen := uint32(len(msgData))
	if msgLen > MaxOutgoingMsgLen {
		return ErrMsgTooLarge
	}

	log.Printf("sending '%s'", string(msgData))

	if err := binary.Write(w, byteOrder, msgLen); err != nil {
		return err
	}

	if _, err := w.Write(msgData); err != nil {
		return err
	}
	return nil
}
