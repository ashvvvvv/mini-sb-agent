package panelserver

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"mini-sb-agent/panelapi"
)

type Agent struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	Secret    string    `json:"secret,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	SeenAt    time.Time `json:"seen_at,omitempty"`
}

type TopologyRecord struct {
	Revision  string            `json:"revision"`
	ETag      string            `json:"etag"`
	Topology  panelapi.Topology `json:"topology"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type Usage struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

type Store struct {
	mu         sync.RWMutex
	agents     map[string]Agent
	topologies map[string]TopologyRecord
	usage      map[string]Usage
	batches    map[string]struct{}
}

func NewStore(_ string) *Store {
	return &Store{
		agents: map[string]Agent{}, topologies: map[string]TopologyRecord{},
		usage: map[string]Usage{}, batches: map[string]struct{}{},
	}
}

func (s *Store) PutAgent(agent Agent) error {
	if agent.ID == "" || agent.Secret == "" {
		return fmt.Errorf("agent id and secret are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if agent.CreatedAt.IsZero() {
		agent.CreatedAt = time.Now().UTC()
	}
	s.agents[agent.ID] = agent
	return nil
}

func (s *Store) AgentBySecret(secret string) (Agent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, agent := range s.agents {
		if agent.Secret == secret {
			return agent, true
		}
	}
	return Agent{}, false
}

func publicAgent(agent Agent) Agent {
	agent.Secret = ""
	return agent
}

func (s *Store) Agents() []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Agent, 0, len(s.agents))
	for _, a := range s.agents {
		out = append(out, publicAgent(a))
	}
	return out
}

func (s *Store) PutTopology(agentID string, topology panelapi.Topology) error {
	if err := topology.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[agentID]; !ok {
		return fmt.Errorf("agent %q not found", agentID)
	}
	revision := randomID()
	digest := sha256.Sum256([]byte(revision))
	s.topologies[agentID] = TopologyRecord{Revision: revision, ETag: `"` + hex.EncodeToString(digest[:]) + `"`, Topology: topology, UpdatedAt: time.Now().UTC()}
	return nil
}

func (s *Store) Topology(agentID string) (TopologyRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.topologies[agentID]
	return v, ok
}
func usageKey(nodeID string, userID int) string { return fmt.Sprintf("%s:%d", nodeID, userID) }
func (s *Store) Usage(nodeID string, userID int) Usage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.usage[usageKey(nodeID, userID)]
}

func (s *Store) ApplyTraffic(agentID string, payload panelapi.TrafficPushRequest) (bool, error) {
	if payload.BatchID == "" {
		return false, fmt.Errorf("batch_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[agentID]; !ok {
		return false, fmt.Errorf("agent not found")
	}
	key := agentID + ":" + payload.BatchID
	if _, duplicate := s.batches[key]; duplicate {
		return false, nil
	}
	for _, item := range payload.Items {
		if item.NodeID == "" || item.UserID <= 0 || item.Upload < 0 || item.Download < 0 {
			return false, fmt.Errorf("invalid traffic item")
		}
		key := usageKey(item.NodeID, item.UserID)
		old := s.usage[key]
		old.Upload += item.Upload
		old.Download += item.Download
		s.usage[key] = old
	}
	s.batches[key] = struct{}{}
	agent := s.agents[agentID]
	agent.SeenAt = time.Now().UTC()
	s.agents[agentID] = agent
	return true, nil
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
