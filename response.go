package jsonhandler

import "net/http"

// Response carries data that will be written to the http.ResponseWriter when
// the handler created by NewHandler finishes executing. Using ResponseFunc,
// users may alter the status code, headers and cookies.
type Response struct {
	hd         http.Header
	StatusCode int
	cookies    []*http.Cookie
}

func (r *Response) Header() http.Header {
	return r.hd
}

func (r *Response) SetCookie(cookie *http.Cookie) {
	r.cookies = append(r.cookies, cookie)
}

// ResponseFunc may alter the response further after the http.Handler created
// by NewHandler returns.
type ResponseFunc func(r *Response)

