package panelserver

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"mini-sb-agent/panelapi"
)

type Server struct {
	adminToken string
	store      *Store
	mux        *http.ServeMux
}

func New(adminToken string, store *Store) *Server {
	if store == nil {
		store = NewStore("")
	}
	s := &Server{adminToken: adminToken, store: store, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /", s.dashboard)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("GET /api/v2/agent/topology", s.agentTopology)
	s.mux.HandleFunc("POST /api/v2/agent/traffic", s.agentTraffic)
	s.mux.HandleFunc("GET /api/v1/admin/agents", s.adminAgents)
	s.mux.HandleFunc("POST /api/v1/admin/agents", s.adminPutAgent)
	s.mux.HandleFunc("PUT /api/v1/admin/agents/{id}/topology", s.adminPutTopology)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }
func bearer(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}
func equal(a, b string) bool {
	return a != "" && b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func (s *Server) requireAgent(w http.ResponseWriter, r *http.Request) (Agent, bool) {
	a, ok := s.store.AgentBySecret(bearer(r))
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return Agent{}, false
	}
	return a, true
}
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !equal(bearer(r), s.adminToken) {
		writeError(w, http.StatusUnauthorized, "invalid admin token")
		return false
	}
	return true
}

func (s *Server) agentTopology(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.requireAgent(w, r)
	if !ok {
		return
	}
	record, ok := s.store.Topology(agent.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "no topology assigned")
		return
	}
	w.Header().Set("ETag", record.ETag)
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("If-None-Match") == record.ETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, panelapi.TopologyResponse{SchemaVersion: 1, Revision: record.Revision, Topology: record.Topology})
}
func (s *Server) agentTraffic(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.requireAgent(w, r)
	if !ok {
		return
	}
	var payload panelapi.TrafficPushRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if key := r.Header.Get("Idempotency-Key"); key != "" && payload.BatchID != "" && key != payload.BatchID {
		writeError(w, http.StatusBadRequest, "idempotency key does not match batch_id")
		return
	}
	applied, err := s.store.ApplyTraffic(agent.ID, payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"applied": applied})
}
func (s *Server) adminAgents(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": s.store.Agents()})
}
func (s *Server) adminPutAgent(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var a Agent
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&a); err != nil {
		writeError(w, 400, "invalid JSON payload")
		return
	}
	if err := s.store.PutAgent(a); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	a.Secret = ""
	writeJSON(w, http.StatusCreated, a)
}
func (s *Server) adminPutTopology(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var body struct {
		Topology panelapi.Topology `json:"topology"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeError(w, 400, "invalid JSON payload")
		return
	}
	if err := s.store.PutTopology(r.PathValue("id"), body.Topology); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	record, _ := s.store.Topology(r.PathValue("id"))
	writeJSON(w, http.StatusOK, record)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
