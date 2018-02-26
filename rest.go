package web

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/schema"
	"net/http"
	"strings"
)

// Go 1.6 compat
const (
	StatusUnprocessableEntity = 422 // RFC 4918, 11.2
)

type Error struct {
	Status int
	Err    error
}

func (err Error) Error() string {
	if err.Err == nil {
		return fmt.Sprintf("HTTP %d", err.Status)
	} else {
		return fmt.Sprintf("%v", err.Err)
	}
}

func Errorf(status int, f string, args ...interface{}) Error {
	return Error{status, fmt.Errorf(f, args...)}
}
func RequestError(err error) Error {
	return Error{StatusUnprocessableEntity, err}
}
func RequestErrorf(f string, args ...interface{}) Error {
	return Errorf(StatusUnprocessableEntity, f, args...)
}

func readRequest(request *http.Request, object interface{}) error {
	contentType := request.Header.Get("Content-Type")

	switch contentType {
	case "application/json":
		if err := json.NewDecoder(request.Body).Decode(object); err != nil {
			return RequestError(err)
		} else {
			return nil
		}

	default:
		return Errorf(http.StatusUnsupportedMediaType, "Unknown Content-Type: %v", contentType)
	}
}

func readQuery(request *http.Request, resource QueryResource) error {
	var decoder = schema.NewDecoder()

	if err := decoder.Decode(resource.QueryREST(), request.URL.Query()); err != nil {
		return RequestError(err)
	} else {
		return nil
	}
}

func writeResponse(responseWriter http.ResponseWriter, object interface{}) error {
	responseWriter.Header().Set("Content-Type", "application/json")

	return json.NewEncoder(responseWriter).Encode(object)
}

// Encodable resource
type Resource interface{}

// Resource collection with sub-Resources
type IndexResource interface {
	// TODO: List() ([]Resource, error)
	Index(name string) (Resource, error)
}

// Resoruce that decodes ?... query vars ussing github.com/gorilla/schema
type QueryResource interface {
	QueryREST() interface{}
}

// Resource that supports GET
type GetResource interface {
	// Return marshalable response resource
	// Perform any independent post-processing + JSON encoding in the request handler goroutine.
	// Must be goroutine-safe!
	GetREST() (Resource, error)
}

// Resource that supports POST
type PostResource interface {
	// Return unmarshalable request resource
	PostREST() (Resource, error)
}

type GetPostResource interface {
	GetResource
	PostResource
}

// Resources that are notified after POST
type MutableResource interface {
	ApplyREST() error
}

type API struct {
	root Resource
}

func MakeAPI(root Resource) API {
	return API{
		root: root,
	}
}

func (api API) index(path string) (Resource, []MutableResource, error) {
	// lookup from root
	var resource = api.root
	var mutables []MutableResource

	if mutableResource, ok := resource.(MutableResource); ok {
		mutables = append(mutables, mutableResource)
	}

	for _, name := range strings.Split(path, "/") {
		if indexResource, ok := resource.(IndexResource); !ok {
			return resource, nil, Error{http.StatusNotFound, nil}
		} else if nextResource, err := indexResource.Index(name); err != nil {
			return resource, nil, err
		} else if nextResource == nil {
			return nil, nil, Error{http.StatusNotFound, nil}
		} else {
			resource = nextResource
		}

		if mutableResource, ok := resource.(MutableResource); ok {
			mutables = append(mutables, mutableResource)
		}
	}

	// reverse
	for i, j := 0, len(mutables)-1; i < j; i, j = i+1, j-1 {
		mutables[i], mutables[j] = mutables[j], mutables[i]
	}

	return resource, mutables, nil
}

func (api API) apply(resource MutableResource, parents []MutableResource) error {
	if resource != nil {
		if err := resource.ApplyREST(); err != nil {
			return err
		}
	}
	for _, resource := range parents {
		if err := resource.ApplyREST(); err != nil {
			return err
		}
	}

	return nil
}

func (api API) handle(w http.ResponseWriter, r *http.Request) error {
	resource, mutableResources, err := api.index(r.URL.Path)

	if err != nil {
		return err
	}

	if queryResource, ok := resource.(QueryResource); !ok {

	} else if err := readQuery(r, queryResource); err != nil {
		return err
	} else {

	}

	switch r.Method {
	case "GET":
		// resolve GET resource
		if getResource, ok := resource.(GetResource); !ok {
			log.Warnf("Not a GetResource: %T", resource)
			return Error{http.StatusMethodNotAllowed, nil}
		} else if ret, err := getResource.GetREST(); err != nil {
			return err
		} else if ret == nil {
			return Error{http.StatusNotFound, nil}
		} else {
			resource = ret
		}

	case "POST":
		if postResource, ok := resource.(PostResource); !ok {
			log.Warnf("Not a PostResource: %T", resource)
			return Error{http.StatusMethodNotAllowed, nil}
		} else if ret, err := postResource.PostREST(); err != nil {
			return err
		} else if ret == nil {
			return Error{http.StatusNotFound, nil}
		} else if err := readRequest(r, ret); err != nil {
			return err
		} else {
			resource = ret
		}

		// apply
		mutableResource, _ := resource.(MutableResource)

		if err := api.apply(mutableResource, mutableResources); err != nil {
			return err
		}

	default:
		return Error{http.StatusNotImplemented, nil}
	}

	if err := writeResponse(w, resource); err != nil {
		return err
	} else {
		log.Infof("%v %v: %T", r.Method, r.URL.Path, resource)
	}

	return nil
}

func (api API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := api.handle(w, r); err == nil {

	} else if httpError, ok := err.(Error); !ok {
		log.Infof("%v %v: HTTP %v: %v", r.Method, r.URL.Path, 500, err.Error())

		http.Error(w, err.Error(), 500)
	} else if httpError.Err != nil {
		log.Infof("%v %v: HTTP %v: %v", r.Method, r.URL.Path, httpError.Status, httpError.Err.Error())

		http.Error(w, httpError.Err.Error(), httpError.Status)
	} else {
		log.Infof("%v %v: HTTP %v", r.Method, r.URL.Path, httpError.Status)

		http.Error(w, "", httpError.Status)
	}
}
