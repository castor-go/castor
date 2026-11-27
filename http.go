package castor

import (
	"context"
	"encoding/json/v2"
	"errors"
	"mime/multipart"
	"net/http"
)

type ctxKeyRequestID int

type ContextKey struct {
	Name string
}

// RequestIDKey is the context key that holds the request ID.
const RequestIDKey ctxKeyRequestID = 0

// RequestIDHeader is the header used to propagate a request ID.
var RequestIDHeader = "X-Request-Id"

func (k *ContextKey) String() string {
	return "context value " + k.Name
}

// GetValue returns ctx's value for key as T, or T's zero value.
func GetValue[T any](ctx context.Context, key any) T {
	if ctx == nil {
		var zero T
		return zero
	}
	value, _ := ctx.Value(key).(T)
	return value
}

// GetLogEntry returns the LogEntry stored in the request context.
func GetLog(req *http.Request) LogEntry {
	return GetValue[LogEntry](req.Context(), LogEntryCtxKey)
}

// Json writes data as a JSON response with status 200 (default).
func Json(res http.ResponseWriter, data any, status ...int) {
	body, err := json.Marshal(data)
	if err != nil {
		http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	res.Header().Set("Content-Type", "application/json; charset=utf-8")
	if len(status) == 0 {
		res.WriteHeader(http.StatusOK)
	} else {
		res.WriteHeader(status[0])
	}
	_, _ = res.Write(body)
}

// ReadJson decodes the request body as JSON into data. Data must be a
// non-nil pointer.
func ReadJson(req *http.Request, data any) error {
	if req == nil || req.Body == nil {
		return errors.New("missing request body")
	}
	return json.UnmarshalRead(req.Body, data)
}

const maxFormMemory = 32 << 20

// ReadFormData returns all form fields sent in the request body. Query
// parameters are not included. For repeated fields, the first value wins.
func ReadFormData(req *http.Request) (map[string]string, error) {
	fields, _, err := ReadFormDataWithFiles(req)
	return fields, err
}

// ReadFormDataWithFiles returns all form fields sent in the request body along
// with uploaded files keyed by form field name.
func ReadFormDataWithFiles(req *http.Request) (map[string]string, map[string][]*multipart.FileHeader, error) {
	if req == nil {
		return nil, nil, errors.New("no request body")
	}

	if err := req.ParseForm(); err != nil {
		return nil, nil, err
	}
	if err := req.ParseMultipartForm(maxFormMemory); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return nil, nil, err
	}

	fields := make(map[string]string, len(req.PostForm))
	for name, values := range req.PostForm {
		if len(values) > 0 {
			fields[name] = values[0]
		}
	}

	if req.MultipartForm == nil {
		return fields, nil, nil
	}
	return fields, req.MultipartForm.File, nil
}

func NotFound(res http.ResponseWriter, message string) {
	res.WriteHeader(http.StatusNotFound)
	res.Write([]byte(message))
}

// GetParam returns a path parameter, or "" when it does not exist.
func GetPathParam(req *http.Request, name string) string {
	return req.PathValue(name)
}

// GetQuery returns a query string parameter, or "" when it does not exist.
func GetQueryParam(req *http.Request, name string) string {
	return req.URL.Query().Get(name)
}

// Redirect writes a redirect to uri. The default status is 302 Found.
func Redirect(res http.ResponseWriter, uri string, status ...int) {
	code := http.StatusFound
	if len(status) > 0 {
		code = status[0]
	}
	res.Header().Set("Location", uri)
	res.WriteHeader(code)
}
