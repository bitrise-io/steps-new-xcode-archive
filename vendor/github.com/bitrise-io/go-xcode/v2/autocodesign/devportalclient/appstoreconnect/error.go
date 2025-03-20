package appstoreconnect

import (
	"fmt"
	"net/http"
	"strings"
)

// ErrorResponseError ...
type ErrorResponseError struct {
	Code   string      `json:"code,omitempty"`
	Status string      `json:"status,omitempty"`
	ID     string      `json:"id,omitempty"`
	Title  string      `json:"title,omitempty"`
	Detail string      `json:"detail,omitempty"`
	Source interface{} `json:"source,omitempty"`
}

// ErrorResponse ...
type ErrorResponse struct {
	Response *http.Response
	Errors   []ErrorResponseError `json:"errors,omitempty"`
}

// Error ...
func (r ErrorResponse) Error() string {
	var m string
	if r.Response.Request != nil {
		m = fmt.Sprintf("%s %s: %d\n", r.Response.Request.Method, r.Response.Request.URL, r.Response.StatusCode)
	}

	var s string
	for _, err := range r.Errors {
		m += s + fmt.Sprintf("- %s: %s: %s", err.Code, err.Title, err.Detail)
		s = "\n"
	}

	return m
}

// IsCursorInvalid ...
func (r ErrorResponse) IsCursorInvalid() bool {
	// {"errors"=>[{"id"=>"[ ... ]", "status"=>"400", "code"=>"PARAMETER_ERROR.INVALID", "title"=>"A parameter has an invalid value", "detail"=>"'eyJvZmZzZXQiOiIyMCJ9' is not a valid cursor for this request", "source"=>{"parameter"=>"cursor"}}]}
	for _, err := range r.Errors {
		if err.Code == "PARAMETER_ERROR.INVALID" && strings.Contains(err.Detail, "is not a valid cursor for this request") {
			return true
		}
	}
	return false
}

// DeviceRegistrationError ...
type DeviceRegistrationError struct {
	Reason string
}

// Error ...
func (e DeviceRegistrationError) Error() string {
	return e.Reason
}
