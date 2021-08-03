package jsonhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
)

// HandlerErr is the error type created by Error and Errorf.
type HandlerErr struct {
	code    int
	message string
	err     error
}

type returnType int

const (
	returnInvalid returnType = iota
	returnPayload
	returnOpts
	returnErr
)

type ctxKey int

const (
	ctxKeyRequest ctxKey = iota + 1
)

type handler struct {
	fn           interface{}
	hv           reflect.Value
	ht           reflect.Type
	payloadType  reflect.Type
	returnType   reflect.Type
	takesPayload bool
	errN         int
	optsN        int
	outN         int
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	ctx = context.WithValue(ctx, ctxKeyRequest, r)

	// construct arguments
	var args []reflect.Value
	args = append(args, reflect.ValueOf(ctx))

	if h.takesPayload {
		pl := reflect.New(h.payloadType)

		if err := json.NewDecoder(r.Body).Decode(pl.Interface()); err != nil {
			defer r.Body.Close()
			h.handleError(w, Errorf(http.StatusBadRequest, "Bad request: %w", err))
			return
		}
		_ = r.Body.Close()
		args = append(args, pl.Elem())
	}

	// call the actual handler!
	responses := h.hv.Call(args)

	// collect responses
	var (
		out  interface{}
		err  error
		opts []ResponseFunc
	)
	if h.outN < 0 && h.errN < 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	if h.outN >= 0 {
		out = responses[h.outN].Interface()
	}

	if h.errN >= 0 {
		if v, ok := responses[h.errN].Interface().(error); ok {
			err = v
		}
	}
	if h.optsN >= 0 {
		opts = responses[h.optsN].Interface().([]ResponseFunc)
	}

	if err != nil {
		h.handleError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, out, opts)
}

// errResp is the output format of the errors returned by handler.ServeHTTP
type errResp struct {
	Message string `json:"error"`
}

// handleError correctly serializes and writes errors to the http.ResponseWriter.
func (h handler) handleError(w http.ResponseWriter, err error) {
	var (
		jErr HandlerErr
		msg  string
		code = http.StatusInternalServerError
	)
	if errors.As(err, &jErr) {
		msg = jErr.message
		code = jErr.code
	} else {
		msg = err.Error()
	}
	h.writeJSON(w, code, errResp{Message: msg}, nil)
}

// writeJSON writes the JSON representation of the output from handler.fn.
func (h handler) writeJSON(w http.ResponseWriter, code int, out interface{}, opts []ResponseFunc) {
	res := Response{
		hd:         w.Header(),
		StatusCode: code,
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")

	if err := enc.Encode(out); err != nil {
		http.Error(w, "Internal error: Encoding response failed", http.StatusInternalServerError)
	}

	for _, o := range opts {
		o(&res)
	}

	w.Header().Set("content-type", "application/json; charset=utf-8")
	for _, c := range res.cookies {
		http.SetCookie(w, c)
	}
	w.WriteHeader(res.StatusCode)

	_, _ = io.Copy(w, &buf)
}

// NewHandler creates a new handler. fn must be one of the following types:
//
//    func(context.Context)
//    func(context.Context) error
//    func(context.Context) (type, error)
//    func(context.Context) (type, []ResponseFunc, error)
//    func(context.Context, type)
//    func(context.Context, type) error
//    func(context.Context, type) (type, error)
//    func(context.Context, type) (type, []ResponseFunc, error)
//
// fn is evaluated during NewHandler, and will panic if the handler does not
// match any of the provided signatures. "type" may be any value that can be
// converted to json using json.Marshal. If marshaling fails, the handler
// returns 400 Bad Request. If marshaling the output fails, the handler returns
// 500 Internal Server Error.
//
// To retrieve the original request, call Request() with the context received.
//
// The result signature may or may not include a result, a slice of functions
// to apply to the response (See ResponseFunc) and/or an error. If an error is
// returned, it is printed in the following format and the status code is set
// to 500:
//
//    {"error": "message"}
//
// To customize the status code, an error may be created using Errorf().
func NewHandler(fn interface{}) http.Handler {
	h := &handler{fn: fn}

	if err := parseHandler(h); err != nil {
		panic(err)
	}

	return h
}

func parseHandler(h *handler) interface{} {
	if h.fn == nil {
		return fmt.Errorf("nil handlers are invalid")
	}
	handlerType := reflect.TypeOf(h.fn)
	h.hv = reflect.ValueOf(h.fn)
	h.ht = handlerType

	if h.ht.Kind() != reflect.Func {
		return errors.New("handler must be a function")
	}

	if err := parseArgs(h, handlerType); err != nil {
		return err
	}
	if err := parseReturns(h, handlerType); err != nil {
		return err
	}

	return nil
}

func parseArgs(h *handler, ht reflect.Type) error {
	inputs := ht.NumIn()
	if inputs == 0 {
		return errors.New("handler function must take a context.Context as its first argument")
	}

	if err := validateContext(ht.In(0)); err != nil {
		return err
	}
	if err := parsePayload(h, ht); err != nil {
		return err
	}

	return nil
}

func parseReturns(h *handler, ht reflect.Type) error {
	// reset these before proceeding (-1 indicates not present)
	h.errN, h.outN, h.optsN = -1, -1, -1

	n := ht.NumOut()
	if n == 0 {
		// the handler signature is func(...)
		return nil
	}
	if n > 3 {
		return fmt.Errorf("too many return values")
	}

	for i := 0; i < n; i++ {
		switch getReturnType(ht.Out(i)) {
		case returnErr:
			h.errN = i
		case returnPayload:
			h.outN = i
		case returnOpts:
			h.optsN = i
		case returnInvalid:
			return fmt.Errorf("invalid return type in position %d", i)
		}
	}

	// if there are return values, one of them is an error, and it is not the last one,
	if n > 1 && h.errN >= 0 && h.errN < n-1 {
		return fmt.Errorf("error must be the last return value")
	}
	// if there are return values, one of them is the payload, and it is not the first one,
	if n > 0 && h.outN >= 0 && h.outN != 0 {
		return fmt.Errorf("the payload must be the first return value")
	}
	return nil
}

func getReturnType(ret reflect.Type) returnType {
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	optsType := reflect.TypeOf((*ResponseFunc)(nil)).Elem()

	if ret.Implements(errorType) {
		return returnErr
	}

	if ret.Kind() == reflect.Slice && ret.Elem().Kind() == reflect.Func {
		if ret.Elem().ConvertibleTo(optsType) {
			return returnOpts
		}
	}

	return returnPayload
}

func validateContext(arg reflect.Type) error {
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	if !arg.Implements(contextType) {
		return fmt.Errorf("first argument of handler does not implement context.Context")
	}

	return nil
}

func parsePayload(h *handler, ht reflect.Type) error {
	if ht.NumIn() != 2 {
		return nil
	}
	h.takesPayload = true
	h.payloadType = ht.In(1)
	return nil
}

// Request returns the original http.Request that NewHandler's http.Handler
// stores in the request context. The body will have been read before fn is
// invoked, and is only accessible if the JSON handler function does not
// expect any payload.
func Request(ctx context.Context) *http.Request {
	return ctx.Value(ctxKeyRequest).(*http.Request)
}
