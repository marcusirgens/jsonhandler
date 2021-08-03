package jsonhandler

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
)

func TestNewHandler(t *testing.T) {
	type args struct {
		handler interface{}
	}
	tests := []struct {
		name      string
		args      args
		wantPanic bool
	}{
		{
			name: "Not a handler",
			args: args{
				handler: "a string? really?",
			},
			wantPanic: true,
		},
		{
			name: "Error returner",
			args: args{
				handler: func() error {
					return nil
				},
			},
			wantPanic: true,
		},
		{
			name: "Useless handler",
			args: args{
				handler: func() {},
			},
			wantPanic: true,
		},
		{
			name: "Simple handler",
			args: args{
				handler: func(ctx context.Context, s string) (string, error) {
					return "hello", nil
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if tt.wantPanic && r == nil {
					t.Errorf("expected panic, got nil")
				}
				if !tt.wantPanic && r != nil {
					t.Errorf("expected no panic, got %v", r)
				}
			}()
			NewHandler(tt.args.handler)
		})
	}
}

func Test_handler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name         string
		payload      string
		handler      interface{}
		wantStatus   int
		want         string
		ignoreOutput bool
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
			handler: func(_ context.Context) (string, []ResponseFunc) {
				return "ok", []ResponseFunc{func(r *Response) {
					r.StatusCode = http.StatusCreated
				}}
			},
			wantStatus: http.StatusCreated,
			want:       `"ok"`,
		},
		{
			name:    "Only writes 201",
			payload: `"world"`,
			handler: func(_ context.Context) []ResponseFunc {
				return []ResponseFunc{func(r *Response) {
					r.StatusCode = http.StatusCreated
				}}
			},
			wantStatus:   http.StatusCreated,
			ignoreOutput: true,
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
			h := NewHandler(tt.handler)

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

			if !tt.ignoreOutput {
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
		h := NewHandler(func(ctx context.Context, in payload) (result, error) {
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

type benchmarkPayload struct {
	Name   string   `json:"name"`
	Owners []string `json:"owners"`
	Age    int      `json:"age"`
}

type benchmarkResult struct {
	Dogs bool   `json:"dogs"`
	Cats string `json:"cats"`
}

const benchmarkVals = `{
	"name": "Solfrid",
	"owners": ["Marcus", "Hanna"],
	"age": 4
}`

func BenchmarkHandler(b *testing.B) {
	h := NewHandler(func(_ context.Context, b benchmarkPayload) (benchmarkResult, error) {
		return benchmarkResult{
			Dogs: true,
			Cats: "bad",
		}, nil
	})

	ref := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			req benchmarkPayload
			res benchmarkResult
			out bytes.Buffer
		)
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("Bad request: %v", err), http.StatusBadRequest)
			return
		}
		res.Dogs = true
		res.Cats = "bad"

		if err := json.NewEncoder(&out).Encode(res); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, &out)
	})

	pairs := []struct {
		name    string
		handler http.Handler
	}{
		{name: "Reference", handler: ref},
		{name: "jsonhandler.Handler", handler: h},
	}

	for _, p := range pairs {
		b.Run(p.name, func(b *testing.B) {
			b.ResetTimer()
			b.StopTimer()
			for i := 0; i < b.N; i++ {
				buf := bytes.NewBufferString(benchmarkVals)
				req := httptest.NewRequest(http.MethodPost, "/", buf)
				rec := httptest.NewRecorder()
				b.StartTimer()
				p.handler.ServeHTTP(rec, req)
				b.StopTimer()
				res := rec.Result()
				if res.StatusCode != http.StatusOK {
					b.Fatalf("Invalid status code %d in response", res.StatusCode)
				}
			}
		})
	}

}
