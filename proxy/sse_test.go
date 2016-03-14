package proxy

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendEvent(t *testing.T) {
	rr := httptest.NewRecorder()
	src := EventSource{w: rr}

	err := src.SendEvent(Event{
		Comment: "comment",
		Name:    "name",
		Data: `{
"key": "value"
}`,
	})

	require.NoError(t, err)
	assert.Equal(t, `: comment
event: name
data: {
data: "key": "value"
data: }

`, rr.Body.String())
}

func TestSendMultipleEvents(t *testing.T) {
	rr := httptest.NewRecorder()
	src := EventSource{w: rr}

	err := src.SendEvent(Event{
		Name: "name1",
		Data: "data1",
	})
	require.NoError(t, err)
	err = src.SendEvent(Event{
		Name: "name2",
		Data: "data2",
	})
	require.NoError(t, err)

	assert.Equal(t, `event: name1
data: data1

event: name2
data: data2

`, rr.Body.String())
}
