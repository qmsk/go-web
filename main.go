package web

import (
	"fmt"
	"net/http"
	"path"
)

type Options struct {
	Listen             string `long:"http-listen" value-name:"[HOST]:PORT" default:":8284"`
	Static             string `long:"http-static" value-name:"PATH"`
	StaticCacheControl string `long:"http-static-cache-control" value-name:"HEADER-VALUE" default:"no-cache"`
}

type Route struct {
	Pattern string
	Handler http.Handler
}

type CacheFilter struct {
	Handler      http.Handler
	CacheControl string
}

func (cacheFilter CacheFilter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", cacheFilter.CacheControl)

	cacheFilter.Handler.ServeHTTP(w, r)
}

func RoutePrefix(prefix string, handler http.Handler) Route {
	return Route{
		Pattern: prefix,
		Handler: http.StripPrefix(prefix, handler),
	}
}

// Return a route that services the tree relative to --http-static=
func (options Options) Route(prefix string, handler http.Handler) Route {
	return Route{
		Pattern: prefix,
		Handler: http.StripPrefix(prefix, handler),
	}
}

// Return a route that services the tree relative to --http-static=
func (options Options) RouteStatic(prefix string) Route {
	var route = Route{Pattern: prefix}
	var handler http.Handler

	if options.Static != "" {
		log.Infof("Serve %v from %v", prefix, options.Static)

		handler = http.FileServer(http.Dir(options.Static))

		if options.StaticCacheControl != "" {
			handler = CacheFilter{handler, options.StaticCacheControl}
		}

		route.Handler = http.StripPrefix(prefix, handler)
	}

	return route
}

// Return a route that serves a named static file, relative to --http-static=
func (options Options) RouteFile(url string, file string) Route {
	file = path.Join(options.Static, file)

	return Route{
		Pattern: url,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != url {
				w.WriteHeader(404)
			} else {
				http.ServeFile(w, r, file)
			}
		}),
	}
}

func (options Options) RouteAPI(prefix string, api API) Route {
	return Route{
		Pattern: prefix,
		Handler: http.StripPrefix(prefix, api),
	}
}

func (options Options) RouteEvents(url string, events Events) Route {
	return Route{
		Pattern: url,
		Handler: events,
	}
}

func (options Options) Server(routes ...Route) error {
	var serveMux = http.NewServeMux()

	for _, route := range routes {
		if route.Handler == nil {
			continue
		}

		serveMux.Handle(route.Pattern, route.Handler)
	}

	if options.Listen != "" {
		var server = http.Server{
			Addr:    options.Listen,
			Handler: serveMux,
		}

		log.Infof("Listen on %v...", options.Listen)

		if err := server.ListenAndServe(); err != nil {
			return fmt.Errorf("ListenAndServe %v: %v", options.Listen, err)
		}
	}

	return nil
}
