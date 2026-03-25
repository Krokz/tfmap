package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Krokz/tfmap/internal/model"
)

func newTestProject() *model.Project {
	return &model.Project{
		Path: "/tmp/my-infra",
		Resources: []model.Resource{
			{Type: "aws_instance", Name: "web", Attributes: map[string]interface{}{"ami": "ami-123"}},
			{Type: "aws_s3_bucket", Name: "data", Attributes: map[string]interface{}{}},
		},
		Variables: []model.Variable{
			{Name: "region", Type: "string", Default: "us-east-1"},
		},
		Outputs: []model.Output{
			{Name: "ip", Value: "10.0.0.1"},
		},
	}
}

func callHandleProject(s *Server) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/project", nil)
	s.handleProject(w, r)
	return w
}

func TestAPIProject(t *testing.T) {
	s := New(newTestProject(), nil)
	w := callHandleProject(s)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got model.Project
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.Path != "/tmp/my-infra" {
		t.Errorf("Path = %q, want /tmp/my-infra", got.Path)
	}
}

func TestAPIProjectContent(t *testing.T) {
	s := New(newTestProject(), nil)
	w := callHandleProject(s)

	var got model.Project
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got.Resources) != 2 {
		t.Errorf("Resources count = %d, want 2", len(got.Resources))
	}
	if len(got.Variables) != 1 {
		t.Errorf("Variables count = %d, want 1", len(got.Variables))
	}
	if len(got.Outputs) != 1 {
		t.Errorf("Outputs count = %d, want 1", len(got.Outputs))
	}
}

func TestUpdateProject(t *testing.T) {
	s := New(newTestProject(), nil)

	updated := &model.Project{
		Path: "/tmp/updated-infra",
		Resources: []model.Resource{
			{Type: "aws_lambda_function", Name: "handler", Attributes: map[string]interface{}{}},
		},
	}
	s.UpdateProject(updated)

	w := callHandleProject(s)
	var got model.Project
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.Path != "/tmp/updated-infra" {
		t.Errorf("Path = %q, want /tmp/updated-infra", got.Path)
	}
	if len(got.Resources) != 1 {
		t.Errorf("Resources count = %d, want 1", len(got.Resources))
	}
	if got.Resources[0].Type != "aws_lambda_function" {
		t.Errorf("Resources[0].Type = %q, want aws_lambda_function", got.Resources[0].Type)
	}
}

func TestAPIProjectCORS(t *testing.T) {
	s := New(newTestProject(), nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/project", nil)
	r.Header.Set("Origin", "http://127.0.0.1:3000")
	s.handleProject(w, r)

	cors := w.Header().Get("Access-Control-Allow-Origin")
	if cors != "http://127.0.0.1:3000" {
		t.Errorf("Access-Control-Allow-Origin = %q, want http://127.0.0.1:3000", cors)
	}
}

func TestAPIProjectCORSRejectsExternal(t *testing.T) {
	s := New(newTestProject(), nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/project", nil)
	r.Header.Set("Origin", "http://evil.com")
	s.handleProject(w, r)

	cors := w.Header().Get("Access-Control-Allow-Origin")
	if cors != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for external origin", cors)
	}
}
