// Full example of how to create a counting handler, similar to the handler in
// net/http/example_handle_test.go.

package jsonhandler_test

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/marcusirgens/jsonhandler"
)

type countInput struct {
	Increment int `json:"increment,omitempty"`
}

type countOutput struct {
	Count int `json:"count"`
}

type countHandler struct {
	mu sync.Mutex
	n  int
}

func (h *countHandler) ServeJSON(ctx context.Context, in countInput) countOutput {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.n = h.n + in.Increment

	return countOutput{
		Count: h.n,
	}
}

func ExampleNewHandler() {
	// create a new handler
	handler := new(countHandler)

	// create a jsonhandler from the ServeJSON function on the handler struct.
	http.Handle("/count", jsonhandler.NewHandler(handler.ServeJSON))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
