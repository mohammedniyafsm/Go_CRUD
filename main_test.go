package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTodoCRUDAndUserScope(t *testing.T) {
	server := newTodoServer()

	created := performRequest(t, server, http.MethodPost, "/users/alice/todos", `{"title":"  Buy milk  "}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", created.Code, http.StatusCreated, created.Body)
	}
	var item todo
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode created todo: %v", err)
	}
	if item.Title != "Buy milk" || item.UserID != "alice" || item.ID == "" {
		t.Fatalf("unexpected created todo: %+v", item)
	}

	listed := performRequest(t, server, http.MethodGet, "/users/alice/todos", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listed.Code, http.StatusOK)
	}
	var items []todo
	if err := json.Unmarshal(listed.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode todo list: %v", err)
	}
	if len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("unexpected todo list: %+v", items)
	}

	otherUser := performRequest(t, server, http.MethodGet, "/users/bob/todos/"+item.ID, "")
	if otherUser.Code != http.StatusNotFound {
		t.Fatalf("other user's read status = %d, want %d", otherUser.Code, http.StatusNotFound)
	}

	updated := performRequest(t, server, http.MethodPut, "/users/alice/todos/"+item.ID, `{"title":"Buy oat milk","completed":true}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d: %s", updated.Code, http.StatusOK, updated.Body)
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode updated todo: %v", err)
	}
	if item.Title != "Buy oat milk" || !item.Completed {
		t.Fatalf("unexpected updated todo: %+v", item)
	}

	deleted := performRequest(t, server, http.MethodDelete, "/users/alice/todos/"+item.ID, "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleted.Code, http.StatusNoContent)
	}

	missing := performRequest(t, server, http.MethodGet, "/users/alice/todos/"+item.ID, "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("read after delete status = %d, want %d", missing.Code, http.StatusNotFound)
	}
}

func TestTodoValidationAndMethods(t *testing.T) {
	server := newTodoServer()

	emptyTitle := performRequest(t, server, http.MethodPost, "/users/alice/todos", `{"title":"  "}`)
	if emptyTitle.Code != http.StatusBadRequest {
		t.Fatalf("empty title status = %d, want %d", emptyTitle.Code, http.StatusBadRequest)
	}

	unsupported := performRequest(t, server, http.MethodDelete, "/users/alice/todos", "")
	if unsupported.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unsupported method status = %d, want %d", unsupported.Code, http.StatusMethodNotAllowed)
	}

	badUpdate := performRequest(t, server, http.MethodPut, "/users/alice/todos/1", `{"title":"Missing completed field"}`)
	if badUpdate.Code != http.StatusBadRequest {
		t.Fatalf("incomplete update status = %d, want %d", badUpdate.Code, http.StatusBadRequest)
	}
}

func performRequest(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
