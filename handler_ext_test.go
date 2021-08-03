package jsonhandler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/marcusirgens/jsonhandler"
)

func Test_Handler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name         string
		payload      string
		handler      interface{}
		wantStatus   int
		want         string
		ignoreResult bool
	}{
		{
			name:    "Hello world",
			payload: `"world"`,
			handler: func(_ context.Context, s string) string {
				return fmt.Sprintf("hello %s", s)
			},
			wantStatus: http.StatusOK,
			want:       `"hello world"`,
		},
		{
			name:    "Status code updater",
			payload: `"world"`,
			handler: func(_ context.Context) (string, []jsonhandler.ResponseFunc) {
				return "ok", []jsonhandler.ResponseFunc{func(r *jsonhandler.Response) {
					r.StatusCode = http.StatusCreated
				}}
			},
			wantStatus: http.StatusCreated,
			want:       `"ok"`,
		},
		{
			name:    "Custom error",
			payload: `"world"`,
			handler: func(_ context.Context) error {
				return jsonhandler.Errorf(http.StatusBadGateway, "Bad gateway")
			},
			wantStatus: http.StatusBadGateway,
			want:       `{"error": "Bad gateway"}`,
		},
		{
			name:         "Bad request",
			payload:      `[]`,
			handler:      func(_ context.Context, s string) {},
			wantStatus:   http.StatusBadRequest,
			want:         ``,
			ignoreResult: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					return
				}
				t.Fatalf("expected no panic, got %v", r)
			}()
			h := jsonhandler.NewHandler(tt.handler)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tt.payload))

			h.ServeHTTP(w, r)
			res := w.Result()
			var body bytes.Buffer

			if res.StatusCode != tt.wantStatus {
				t.Errorf("invalid status %d; want %d", res.StatusCode, tt.wantStatus)
			}
			defer res.Body.Close()
			_, _ = io.Copy(&body, res.Body)

			if !tt.ignoreResult {
				if ok, err := equalJSON(tt.want, body.String()); err != nil {
					t.Errorf("failed to compare JSON: %v", err)
				} else if !ok {
					t.Errorf("%s != %s", tt.want, body.String())
				}
			}
		})
	}
	t.Run("Complex handler", func(t *testing.T) {
		type payload struct {
			Name string
			Age  int
		}
		type result struct {
			Greeting string
		}
		h := jsonhandler.NewHandler(func(ctx context.Context, in payload) (result, error) {
			o := result{Greeting: in.Name}
			return o, nil
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"name": "Marcus", "age": 32}`))

		h.ServeHTTP(w, r)
		res := w.Result()

		if res.StatusCode != http.StatusOK {
			t.Errorf("invalid status %d; want %d", res.StatusCode, http.StatusOK)
		}
		var body bytes.Buffer
		defer res.Body.Close()
		_, _ = io.Copy(&body, res.Body)
	})
}

// See https://gist.github.com/turtlemonvh/e4f7404e28387fadb8ad275a99596f67
func equalJSON(s1, s2 string) (bool, error) {
	var o1 interface{}
	var o2 interface{}

	var err error
	err = json.Unmarshal([]byte(s1), &o1)
	if err != nil {
		return false, fmt.Errorf("Error mashalling string 1 :: %s", err.Error())
	}
	err = json.Unmarshal([]byte(s2), &o2)
	if err != nil {
		return false, fmt.Errorf("Error mashalling string 2 :: %s", err.Error())
	}

	return reflect.DeepEqual(o1, o2), nil
}
