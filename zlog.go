// Copyright 2021 ZLIU.ORG
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package zlog

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/liuzl/filestore"
	"github.com/rs/zerolog"
)

func init() {
	caddy.RegisterModule(Middleware{})
	httpcaddyfile.RegisterHandlerDirective("zlog", parseCaddyfile)
}

var once sync.Once
var c Chain

// Middleware implements an HTTP handler that logs the
// whole response by zerolog.
type Middleware struct {
	LogDir  string `json:"log_dir,omitempty"`
	SplitBy string `json:"split_by,omitempty"`
	HashDir string `json:"hash_dir,omitempty"`
}

// CaddyModule returns the Caddy module information.
func (Middleware) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.zlog",
		New: func() caddy.Module { return new(Middleware) },
	}
}

// Provision implements caddy.Provisioner.
func (m *Middleware) Provision(ctx caddy.Context) error {
	if m.LogDir == "" {
		m.LogDir = filepath.Join(filepath.Dir(os.Args[0]), "zerolog")
	}
	if m.SplitBy == "" {
		m.SplitBy = "day"
	}
	if m.HashDir == "on" {
		m.HashDir = filepath.Join(filepath.Dir(os.Args[0]), "hashdata")
	}
	return nil
}

// Validate implements caddy.Validator.
func (m *Middleware) Validate() error {
	if m.SplitBy != "day" && m.SplitBy != "hour" {
		return fmt.Errorf("zlog split_by must be day or hour")
	}
	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (m Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	once.Do(func() {
		hostname, _ := os.Hostname()
		var out io.Writer
		f, err := filestore.NewFileStorePro(m.LogDir, m.SplitBy)
		if err != nil {
			out = os.Stdout
			fmt.Fprintf(os.Stderr, "err: %+v, will zerolog to stdout\n", err)
		} else {
			out = f
		}

		log := zerolog.New(out).With().
			Timestamp().
			Str("service", filepath.Base(os.Args[0])).
			Str("host", hostname).
			Logger()

		c = NewChain()

		// Install the logger handler with default output on the console
		c = c.Append(NewHandler(log))

		c = c.Append(AccessHandler(func(r *http.Request,
			status, size int, duration time.Duration) {
			FromRequest(r).Debug().
				Str("method", r.Method).
				Str("url", r.URL.String()).
				Int("status", status).
				Int("size", size).
				Dur("duration", duration).
				Msg("")
		}))

		// Install some provided extra handler to set some request's context fields.
		// Thanks to those handler, all our logs will come with some pre-populated fields.
		c = c.Append(RemoteAddrHandler("server"))
		c = c.Append(HeaderHandler("X-Forwarded-For"))
		c = c.Append(HeaderHandler("User-Agent"))
		c = c.Append(HeaderHandler("Referer"))
		c = c.Append(RequestIDHandler("req_id", "Request-Id"))
		// keep in order
		c = c.Append(DelResponseHeaderHandler("Cost"))
		c = c.Append(ResponseHeaderHandler("Cost", "float"))
		c = c.Append(DumpResponseHandler("response"))
		c = c.Append(DumpRequestHandler("request"))

		// init the hash file store
		if m.HashDir != "" {
			var err error
			c.hashStore, err = filestore.NewFileStorePro(m.HashDir, m.SplitBy)
			if err != nil {
				c.hashStore = nil
				fmt.Fprintf(os.Stderr, "err: %+v, open %s error\n", err, m.HashDir)
			}
		}

	})
	return c.Then(next).ServeHTTP(w, r)
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (m *Middleware) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for nesting := d.Nesting(); d.NextBlock(nesting); {
			switch d.Val() {
			case "log_dir":
				if d.NextArg() {
					m.LogDir = d.Val()
				}
				// ...
			case "split_by":
				if d.NextArg() {
					m.SplitBy = d.Val()
				}
				// ...
			case "hash_dir":
				if d.NextArg() {
					m.HashDir = d.Val()
				}
			}
		}
	}
	return nil
}

// parseCaddyfile unmarshals tokens from h into a new Middleware.
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m Middleware
	err := m.UnmarshalCaddyfile(h.Dispenser)
	return m, err
}

// Interface guards
var (
	_ caddy.Provisioner           = (*Middleware)(nil)
	_ caddy.Validator             = (*Middleware)(nil)
	_ caddyhttp.MiddlewareHandler = (*Middleware)(nil)
	_ caddyfile.Unmarshaler       = (*Middleware)(nil)
)
