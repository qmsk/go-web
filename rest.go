package web

import (
	"encoding/json"
	"fmt"
	"log"
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

func RequestError(err error) Error {
	return Error{400, err}
}

func readRequest(request *http.Request, object interface{}) error {
	if err := json.NewDecoder(request.Body).Decode(object); err != nil {
		return Error{StatusUnprocessableEntity, err}
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

// Resource that supports sub-Resources
type IndexResource interface {
	Index(name string) (Resource, error)
}

// apiResource that supports GET
type GetResource interface {
	// Optional
	// Perform any independent post-processing + JSON encoding in the request handler goroutine.
	// Must be goroutine-safe!
	GetREST() (Resource, error)
}

// apiResource that supports POST
type PostResource interface {
	PostREST() (Resource, error)
}

type API struct {
	root Resource
}

func MakeAPI(root Resource) API {
	return API{
		root: root,
	}
}

func (api API) index(path string) (Resource, error) {
	// lookup from root
	var resource = api.root

	for _, name := range strings.Split(path, "/") {
		if indexResource, ok := resource.(IndexResource); !ok {
			return resource, Error{http.StatusNotFound, nil}
		} else if nextResource, err := indexResource.Index(name); err != nil {
			return resource, err
		} else if nextResource == nil {
			return nil, Error{http.StatusNotFound, nil}
		} else {
			resource = nextResource
		}
	}

	return resource, nil
}

func (api API) handle(w http.ResponseWriter, r *http.Request) error {
	resource, err := api.index(r.URL.Path)

	if err != nil {
		return err
	}

	switch r.Method {
	case "GET":
		// resolve GET resource
		if getResource, ok := resource.(GetResource); !ok {
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
			return Error{http.StatusMethodNotAllowed, nil}
		} else if err := readRequest(r, resource); err != nil {
			return err
		} else if ret, err := postResource.PostREST(); err != nil {
			return err
		} else if ret == nil {
			return Error{http.StatusNoContent, nil}
		} else {
			resource = ret
		}

	default:
		return Error{http.StatusNotImplemented, nil}
	}

	if err := writeResponse(w, resource); err != nil {
		return err
	} else {
		log.Printf("qmsk.web: %v %v: %#v", r.Method, r.URL.Path, resource)
	}

	return nil
}

func (api API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := api.handle(w, r); err == nil {

	} else if httpError, ok := err.(Error); !ok {
		http.Error(w, err.Error(), 500)
	} else if httpError.Err != nil {
		http.Error(w, httpError.Err.Error(), httpError.Status)
	} else {
		http.Error(w, "", httpError.Status)
	}
}
