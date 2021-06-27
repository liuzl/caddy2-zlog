package zlog

import (
	"fmt"
	"net/http"
)

type ResponseLog struct {
	Request    *http.Request
	StatusCode int    `json:"staus_code"`
	Body       string `json:"body"`
	Header     string `json:"header"`
}

func (rl ResponseLog) DumpResponse() string {
	res := ""
	res += fmt.Sprintf("HTTP/%d.%d %d %s\r\n", rl.Request.ProtoMajor, rl.Request.ProtoMinor,
		rl.StatusCode, http.StatusText(rl.StatusCode))
	res += rl.Header
	res += "\r\n"
	res += rl.Body
	return res
}

// headerOperation represents an operation on the header
type headerOperation func(http.Header)

type ResponseProxyWriter struct {
	writer       http.ResponseWriter
	Body         []byte
	Code         int
	ops          []headerOperation
	SourceHeader http.Header
	wroteHeader  bool
}

func (w *ResponseProxyWriter) Header() http.Header {
	return w.writer.Header()
}

func (w *ResponseProxyWriter) Write(bytes []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	w.Body = append(w.Body, bytes[0:len(bytes)]...)
	return w.writer.Write(bytes)
}

func cloneHeader(h http.Header) http.Header {
	h2 := make(http.Header, len(h))
	for k, vv := range h {
		vv2 := make([]string, len(vv))
		copy(vv2, vv)
		h2[k] = vv2
	}
	return h2
}

func (w *ResponseProxyWriter) WriteHeader(i int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true

	h := w.writer.Header()
	w.SourceHeader = cloneHeader(h)

	// perform our revisions
	for _, op := range w.ops {
		op(h)
	}

	w.Code = i
	w.writer.WriteHeader(i)
}

func (w *ResponseProxyWriter) delHeader(key string) {
	// remove the existing one if any
	w.writer.Header().Del(key)

	// register a future deletion
	w.ops = append(w.ops, func(h http.Header) {
		h.Del(key)
	})
}

func NewRespProxyWriter(w http.ResponseWriter) *ResponseProxyWriter {
	return &ResponseProxyWriter{
		writer: w,
		Body:   []byte{},
	}
}
