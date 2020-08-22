package webtest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type APIRequest struct {
	Method string
	Target string
	Object interface{}
}
type APIResponse struct {
	StatusCode int
	Object     interface{}
}

type APITest struct {
	Handler http.Handler

	Request  APIRequest
	Response APIResponse
}

func (test APITest) makeRequest() *http.Request {
	var request *http.Request
	var requestBody io.Reader
	var requestBuffer bytes.Buffer
	var contentType string

	if test.Request.Object != nil {
		if err := json.NewEncoder(&requestBuffer).Encode(test.Request.Object); err != nil {
			panic(err)
		} else {
			contentType = "application/json"
			requestBody = &requestBuffer
		}
	}

	request = httptest.NewRequest(test.Request.Method, test.Request.Target, requestBody)

	// headers
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	return request
}

func (test APITest) testRequest(request *http.Request) *http.Response {
	var responseWriter = httptest.NewRecorder()

	test.Handler.ServeHTTP(responseWriter, request)

	return responseWriter.Result()
}

func TestAPI(t *testing.T, test APITest) {
	var request = test.makeRequest()
	var response = test.testRequest(request)

	if test.Response.StatusCode != 0 && test.Response.StatusCode != response.StatusCode {
		t.Errorf("%v %v => HTTP %v, expected %v", test.Request.Method, test.Request.Target, response.StatusCode, test.Response.StatusCode)
	}

	if test.Response.Object == nil {

	} else {
		switch contentType := response.Header.Get("Content-Type"); contentType {
		case "application/json":
			if err := json.NewDecoder(response.Body).Decode(test.Response.Object); err != nil {
				panic(err)
			}
		default:
			t.Errorf("%v %v => HTTP %v with unsupported Content-Type:%v", test.Request.Method, test.Request.Target, response.StatusCode, contentType)
		}
	}
}
