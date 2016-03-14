package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

type EventHandlerFunc func(w *EventSource, req *http.Request)

type EventSource struct {
	w http.ResponseWriter
	f http.Flusher
}

type Event struct {
	Comment string
	Name    string
	Data    string
}

func HandleEventSource(handler EventHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		src := EventSource{w: w}
		if f, ok := w.(http.Flusher); ok {
			src.f = f
		}

		w.Header().Set("Content-Type", "text/event-stream")
		handler(&src, req)
	}
}

func (s *EventSource) SendJSON(data interface{}) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return s.SendEvent(Event{
		Data: string(dataBytes),
	})
}

func (s *EventSource) SendEvent(e Event) error {
	if e.Comment != "" {
		if err := writePrefixedLines(":", e.Comment, s.w); err != nil {
			return err
		}
	}
	if e.Name != "" {
		if err := writePrefixedLines("event:", e.Name, s.w); err != nil {
			return err
		}
	}
	if e.Data != "" {
		if err := writePrefixedLines("data:", e.Data, s.w); err != nil {
			return err
		}
	}
	_, err := s.w.Write([]byte{'\n'})

	log.Print("finished writing event")

	if s.f != nil {
		log.Print("flushing")
		s.f.Flush()
	}

	return err
}

/*
 writePrefixedLines writes each line of data to w with a prefix and a space.
 E.g.:

 	{"one": 1,
 	 "two": 2}

 with prefix=data gives

 	data: {"one": 1,
 	data:  "two": 2}

*/
func writePrefixedLines(prefix, data string, w io.Writer) error {
	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		_, err := fmt.Fprintln(w, prefix, scanner.Text())
		if err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}
