package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type todo struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type todoServer struct {
	mu     sync.RWMutex
	todos  map[string]map[string]todo
	nextID uint64
}

func newTodoServer() *todoServer {
	return &todoServer{todos: make(map[string]map[string]todo)}
}

func (s *todoServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "users" || parts[1] == "" || parts[2] != "todos" {
		http.NotFound(w, r)
		return
	}

	userID := parts[1]
	if len(parts) == 3 {
		s.handleCollection(w, r, userID)
		return
	}
	if len(parts) == 4 && parts[3] != "" {
		s.handleTodo(w, r, userID, parts[3])
		return
	}
	http.NotFound(w, r)
}

func (s *todoServer) handleCollection(w http.ResponseWriter, r *http.Request, userID string) {
	switch r.Method {
	case http.MethodGet:
		s.listTodos(w, userID)
	case http.MethodPost:
		s.createTodo(w, r, userID)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *todoServer) handleTodo(w http.ResponseWriter, r *http.Request, userID, todoID string) {
	switch r.Method {
	case http.MethodGet:
		s.getTodo(w, userID, todoID)
	case http.MethodPut:
		s.updateTodo(w, r, userID, todoID)
	case http.MethodDelete:
		s.deleteTodo(w, userID, todoID)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *todoServer) listTodos(w http.ResponseWriter, userID string) {
	s.mu.RLock()
	todos := make([]todo, 0, len(s.todos[userID]))
	for _, item := range s.todos[userID] {
		todos = append(todos, item)
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, todos)
}

func (s *todoServer) createTodo(w http.ResponseWriter, r *http.Request, userID string) {
	var input struct {
		Title string `json:"title"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(input.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	now := time.Now().UTC()
	s.mu.Lock()
	s.nextID++
	item := todo{
		ID:        strconv.FormatUint(s.nextID, 10),
		UserID:    userID,
		Title:     strings.TrimSpace(input.Title),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if s.todos[userID] == nil {
		s.todos[userID] = make(map[string]todo)
	}
	s.todos[userID][item.ID] = item
	s.mu.Unlock()

	w.Header().Set("Location", fmt.Sprintf("/users/%s/todos/%s", userID, item.ID))
	writeJSON(w, http.StatusCreated, item)
}

func (s *todoServer) getTodo(w http.ResponseWriter, userID, todoID string) {
	item, ok := s.findTodo(userID, todoID)
	if !ok {
		writeError(w, http.StatusNotFound, "todo not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *todoServer) updateTodo(w http.ResponseWriter, r *http.Request, userID, todoID string) {
	var input struct {
		Title     *string `json:"title"`
		Completed *bool   `json:"completed"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Title == nil || input.Completed == nil || strings.TrimSpace(*input.Title) == "" {
		writeError(w, http.StatusBadRequest, "title and completed are required")
		return
	}

	s.mu.Lock()
	item, ok := s.todos[userID][todoID]
	if !ok {
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, "todo not found")
		return
	}
	item.Title = strings.TrimSpace(*input.Title)
	item.Completed = *input.Completed
	item.UpdatedAt = time.Now().UTC()
	s.todos[userID][todoID] = item
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, item)
}

func (s *todoServer) deleteTodo(w http.ResponseWriter, userID, todoID string) {
	s.mu.Lock()
	_, ok := s.todos[userID][todoID]
	if ok {
		delete(s.todos[userID], todoID)
	}
	s.mu.Unlock()

	if !ok {
		writeError(w, http.StatusNotFound, "todo not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *todoServer) findTodo(userID, todoID string) (todo, bool) {
	s.mu.RLock()
	item, ok := s.todos[userID][todoID]
	s.mu.RUnlock()
	return item, ok
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func main() {
	server := &http.Server{
		Addr:              ":8080",
		Handler:           newTodoServer(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("todo API listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
