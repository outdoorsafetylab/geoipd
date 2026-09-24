package middleware

import (
	"encoding/json"
	"net/http"
	"runtime/debug"

	"service/log"
)

type responseDumper struct {
	w http.ResponseWriter
	s int
}

func (d *responseDumper) Header() http.Header {
	return d.w.Header()
}

func (d *responseDumper) Write(data []byte) (int, error) {
	return d.w.Write(data)
}

func (d *responseDumper) WriteHeader(statusCode int) {
	if d.s == 0 {
		d.w.WriteHeader(statusCode)
		d.s = statusCode
	} else {
		log.Warningf("Attempt to write header again: %d", statusCode)
		debug.PrintStack()
	}
}

func Dump(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log the path only: the query carries the IP being looked up.
		log.Debugf("Handling: %s %s", r.Method, r.URL.Path)
		dumper := &responseDumper{w: w}
		handler.ServeHTTP(dumper, r)
		err := dump(r, dumper)
		if err != nil {
			log.Errorf("Failed to dump: %s", err.Error())
		}
	})
}

type request struct {
	Method  string
	Path    string
	Proto   string
	Headers http.Header
}

type response struct {
	Code    int
	Headers http.Header
}

// ipHeaders can carry the caller's IP, or the IP being looked up (Referer can
// hold a ?ip= URL); they are redacted from the dump.
var ipHeaders = []string{
	"X-Forwarded-For", "Forwarded", "X-Real-Ip",
	"Cf-Connecting-Ip", "True-Client-Ip", "Referer",
}

func redact(h http.Header) http.Header {
	out := h.Clone()
	for _, name := range ipHeaders {
		if _, ok := out[http.CanonicalHeaderKey(name)]; ok {
			out.Set(name, "[redacted]")
		}
	}
	return out
}

// dump leaves out the query and both bodies: the query and the response body
// carry the IP being looked up.
func dump(r *http.Request, d *responseDumper) error {
	out := &struct {
		Request  *request
		Response *response
	}{
		Request: &request{
			Method:  r.Method,
			Path:    r.URL.Path,
			Proto:   r.Proto,
			Headers: redact(r.Header),
		},
		Response: &response{
			Code:    d.s,
			Headers: d.Header(),
		},
	}
	if out.Response.Code == 0 {
		out.Response.Code = 200
	}
	data, err := json.Marshal(out)
	if err != nil {
		log.Errorf("Failed to marshal dump: %s", err.Error())
		return err
	}
	log.Debugf("%s", string(data))
	return nil
}
