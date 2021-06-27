package zlog

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/rs/xid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/zenazn/goji/web/mutil"
)

// FromRequest gets the logger in the request's context.
// This is a shortcut for log.Ctx(r.Context())
func FromRequest(r *http.Request) *zerolog.Logger {
	return log.Ctx(r.Context())
}

// NewHandler injects log into requests context.
func NewHandler(log zerolog.Logger) func(caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			// Create a copy of the logger (including internal context slice)
			// to prevent data race when using UpdateContext.
			l := log.With().Logger()
			r = r.WithContext(l.WithContext(r.Context()))
			return next.ServeHTTP(w, r)
		})
	}
}

// URLHandler adds the requested URL as a field to the context's logger
// using fieldKey as field key.
func URLHandler(fieldKey string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			log := zerolog.Ctx(r.Context())
			log.UpdateContext(func(c zerolog.Context) zerolog.Context {
				return c.Str(fieldKey, r.URL.String())
			})
			return next.ServeHTTP(w, r)
		})
	}
}

// MethodHandler adds the request method as a field to the context's logger
// using fieldKey as field key.
func MethodHandler(fieldKey string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			log := zerolog.Ctx(r.Context())
			log.UpdateContext(func(c zerolog.Context) zerolog.Context {
				return c.Str(fieldKey, r.Method)
			})
			return next.ServeHTTP(w, r)
		})
	}
}

// RequestHandler adds the request method and URL as a field to the context's logger
// using fieldKey as field key.
func RequestHandler(fieldKey string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			log := zerolog.Ctx(r.Context())
			log.UpdateContext(func(c zerolog.Context) zerolog.Context {
				return c.Str(fieldKey, r.Method+" "+r.URL.String())
			})
			return next.ServeHTTP(w, r)
		})
	}
}

// RemoteAddrHandler adds the request's remote address as a field to the context's logger
// using fieldKey as field key.
func RemoteAddrHandler(fieldKey string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				log := zerolog.Ctx(r.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, host)
				})
			}
			return next.ServeHTTP(w, r)
		})
	}
}

// UserAgentHandler adds the request's user-agent as a field to the context's logger
// using fieldKey as field key.
func UserAgentHandler(fieldKey string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			if ua := r.Header.Get("User-Agent"); ua != "" {
				log := zerolog.Ctx(r.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, ua)
				})
			}
			return next.ServeHTTP(w, r)
		})
	}
}

// RefererHandler adds the request's referer as a field to the context's logger
// using fieldKey as field key.
func RefererHandler(fieldKey string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			if ref := r.Header.Get("Referer"); ref != "" {
				log := zerolog.Ctx(r.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, ref)
				})
			}
			return next.ServeHTTP(w, r)
		})
	}
}

type idKey struct{}

// AccessHandler returns a handler that call f after each request.
func AccessHandler(f func(r *http.Request, status, size int, duration time.Duration)) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			start := time.Now()
			lw := mutil.WrapWriter(w)
			err := next.ServeHTTP(lw, r)
			f(r, lw.Status(), lw.BytesWritten(), time.Since(start))
			return err
		})
	}
}

// IDFromRequest returns the unique id associated to the request if any.
func IDFromRequest(r *http.Request, headerName string) (id xid.ID, err error) {
	if r == nil {
		return
	}
	id, err = xid.FromString(r.Header.Get(headerName))
	return
}

// RequestIDHandler returns a handler setting a unique id to the request which can
// be gathered using IDFromRequest(req). This generated id is added as a field to the
// logger using the passed fieldKey as field name. The id is also added as a response
// header if the headerName is not empty.
//
// The generated id is a URL safe base64 encoded mongo object-id-like unique id.
// Mongo unique id generation algorithm has been selected as a trade-off between
// size and ease of use: UUID is less space efficient and snowflake requires machine
// configuration.
func RequestIDHandler(fieldKey, headerName string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			ctx := r.Context()
			id, err := IDFromRequest(r, headerName)
			if err != nil {
				id = xid.New()
			}
			if fieldKey != "" {
				log := zerolog.Ctx(ctx)
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, id.String())
				})
			}
			if headerName != "" {
				r.Header.Set(headerName, id.String())
			}
			return next.ServeHTTP(w, r)
		})
	}
}

func DumpRequestHandler(fieldKey string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			log := zerolog.Ctx(r.Context())
			log.UpdateContext(func(c zerolog.Context) zerolog.Context {
				res, err := httputil.DumpRequest(r, true)
				var msg string
				if err != nil {
					msg = err.Error()
				} else {
					msg = string(res)
				}
				return c.Str(fieldKey, msg)
			})
			return next.ServeHTTP(w, r)
		})
	}
}

// HeaderHandler adds the request's headerName from Header as a field to the
// context's logger using headerName as field key.
func HeaderHandler(headerName string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			if v := r.Header.Get(headerName); v != "" {
				log := zerolog.Ctx(r.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(headerName, v)
				})
			}
			return next.ServeHTTP(w, r)
		})
	}
}

func DumpResponseHandler(fieldKey string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			nw := NewRespProxyWriter(w)
			err := next.ServeHTTP(nw, r)
			var b bytes.Buffer
			nw.SourceHeader.WriteSubset(&b, nil)
			log := zerolog.Ctx(r.Context())
			log.UpdateContext(func(c zerolog.Context) zerolog.Context {
				return c.Str(fieldKey, ResponseLog{
					Request:    r,
					StatusCode: nw.Code,
					Body:       string(nw.Body),
					Header:     string(b.Bytes())}.DumpResponse())
			})
			return err
		})
	}
}

func DelResponseHeaderHandler(headerName string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			nw := NewRespProxyWriter(w)
			nw.delHeader(headerName)
			return next.ServeHTTP(nw, r)
		})
	}
}

func ResponseHeaderHandler(headerName, valType string) func(next caddyhttp.Handler) caddyhttp.Handler {
	return func(next caddyhttp.Handler) caddyhttp.Handler {
		return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			nw := NewRespProxyWriter(w)
			err := next.ServeHTTP(nw, r)
			if v := nw.SourceHeader.Get(headerName); v != "" {
				log := zerolog.Ctx(r.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					switch valType {
					case "bool":
						if val, err := strconv.ParseBool(v); err == nil {
							return c.Bool(headerName, val)
						}
					case "float":
						if val, err := strconv.ParseFloat(v, 64); err == nil {
							return c.Float64(headerName, val)
						}
					case "int":
						if val, err := strconv.ParseInt(v, 10, 64); err == nil {
							return c.Int64(headerName, val)
						}
					case "uint":
						if val, err := strconv.ParseUint(v, 10, 64); err == nil {
							return c.Uint64(headerName, val)
						}
					case "str":
						return c.Str(headerName, v)
					}
					// if strconv convert failed, saved as str by default
					return c.Str(headerName, v)
				})
			}
			return err
		})
	}
}
