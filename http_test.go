package castor

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestReadJson(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	var got payload
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"Alice","age":30}`))
	if err := ReadJson(req, &got); err != nil {
		t.Fatalf("ReadJson: %v", err)
	}
	if got.Name != "Alice" || got.Age != 30 {
		t.Fatalf("decoded payload = %#v", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":`))
	if err := ReadJson(req, &payload{}); err == nil {
		t.Fatal("expected invalid JSON error")
	}

	if err := ReadJson(nil, &payload{}); err == nil {
		t.Fatal("expected nil request error")
	}
	if err := ReadJson(&http.Request{}, &payload{}); err == nil {
		t.Fatal("expected nil body error")
	}
}

func TestReadFormDataURLEncoded(t *testing.T) {
	values := url.Values{
		"name":  {"Alice"},
		"tag":   {"a", "b"},
		"empty": {""},
	}
	req := httptest.NewRequest(http.MethodPost, "/?query=ignored", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fields, err := ReadFormData(req)
	if err != nil {
		t.Fatalf("ReadFormData: %v", err)
	}
	if fields["name"] != "Alice" || fields["tag"] != "a" {
		t.Fatalf("fields = %#v", fields)
	}
	if _, ok := fields["empty"]; !ok {
		t.Fatal("empty field should be present")
	}
	if _, ok := fields["query"]; ok {
		t.Fatal("query parameter should not be included")
	}

	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	fieldsWithFiles, files, err := ReadFormDataWithFiles(req)
	if err != nil {
		t.Fatalf("ReadFormDataWithFiles: %v", err)
	}
	if fieldsWithFiles["name"] != "Alice" || files != nil {
		t.Fatalf("fields=%#v files=%#v", fieldsWithFiles, files)
	}
}

func TestReadFormDataMultipart(t *testing.T) {
	req := newMultipartRequest(t)

	fields, files, err := ReadFormDataWithFiles(req)
	if err != nil {
		t.Fatalf("ReadFormDataWithFiles: %v", err)
	}
	if fields["name"] != "Alice" || fields["tag"] != "a" {
		t.Fatalf("fields = %#v", fields)
	}
	if _, ok := fields["query"]; ok {
		t.Fatal("query parameter should not be included")
	}
	if len(files["avatar"]) != 1 {
		t.Fatalf("files = %#v", files)
	}
	if files["avatar"][0].Filename != "avatar.txt" {
		t.Fatalf("filename = %q", files["avatar"][0].Filename)
	}

	file, err := files["avatar"][0].Open()
	if err != nil {
		t.Fatalf("open upload: %v", err)
	}
	content, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil || string(content) != "hello file" {
		t.Fatalf("upload content = %q, err = %v", content, err)
	}

	req = newMultipartRequest(t)
	fields, err = ReadFormData(req)
	if err != nil {
		t.Fatalf("ReadFormData multipart: %v", err)
	}
	if fields["name"] != "Alice" {
		t.Fatalf("fields = %#v", fields)
	}
}

func TestReadFormDataErrors(t *testing.T) {
	if _, err := ReadFormData(nil); err == nil {
		t.Fatal("expected nil request error")
	}
	if _, _, err := ReadFormDataWithFiles(nil); err == nil {
		t.Fatal("expected nil request error")
	}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("a=%zz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := ReadFormData(req); err == nil {
		t.Fatal("expected malformed form error")
	}

	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not multipart"))
	req.Header.Set("Content-Type", "multipart/form-data")
	if _, err := ReadFormData(req); err == nil {
		t.Fatal("expected missing multipart boundary error")
	}
}

func newMultipartRequest(t *testing.T) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("name", "Alice"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("tag", "a"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("tag", "b"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("avatar", "avatar.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello file")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/?query=ignored", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	t.Cleanup(func() {
		if req.MultipartForm != nil {
			_ = req.MultipartForm.RemoveAll()
		}
	})
	return req
}
