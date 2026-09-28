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

// usageKey identifies one accounted (agent, node, user) triple. A comparable
// struct key cannot collide the way ":"-joined strings can: agent ids and
// node ids may legally contain colons, so ("edge", "vless:hysteria:21") and
// ("edge:vless", "hysteria:21") must stay distinct accounts.
type usageKey struct {
	agentID string
	nodeKey string
	userID  int
}

// batchKey identifies one agent's idempotency batch. Struct key for the same
// colon-collision reason as usageKey; it also avoids the join allocation.
type batchKey struct {
	agentID string
	batchID string
}

type Store struct {
	mu         sync.RWMutex
	agents     map[string]Agent
	topologies map[string]TopologyRecord
	usage      map[usageKey]Usage
	batches    map[batchKey]struct{}
}

func NewStore(_ string) *Store {
	return &Store{
		agents: map[string]Agent{}, topologies: map[string]TopologyRecord{},
		usage: map[usageKey]Usage{}, batches: map[batchKey]struct{}{},
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
// Usage returns the traffic accounted for one agent's node and user. The
// identity includes the agent and the full node key (normalized protocol plus
// node id), so equal node ids across agents or protocols never share an
// account.
func (s *Store) Usage(agentID, nodeType, nodeID string, userID int) Usage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.usage[usageKey{agentID: agentID, nodeKey: panelapi.NodeKey(nodeType, nodeID), userID: userID}]
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
	bk := batchKey{agentID: agentID, batchID: payload.BatchID}
	if _, duplicate := s.batches[bk]; duplicate {
		return false, nil
	}
	// Validate the whole batch before touching any counter: a rejected batch
	// must leave usage unchanged, and every item must identify, with its full
	// node identity (node_type + node_id), a node assigned to this agent, so
	// traffic can never be booked to another agent's node or to a protocol
	// the agent does not run on that node id.
	record, ok := s.topologies[agentID]
	if !ok {
		return false, fmt.Errorf("agent %q has no assigned topology", agentID)
	}
	owned := make(map[string]bool, len(record.Topology.Nodes))
	for _, node := range record.Topology.Nodes {
		owned[node.Key()] = true
	}
	for _, item := range payload.Items {
		if item.NodeID == "" || item.NodeType == "" || item.UserID <= 0 || item.Upload < 0 || item.Download < 0 {
			return false, fmt.Errorf("invalid traffic item")
		}
		nodeKey := panelapi.NodeKey(item.NodeType, item.NodeID)
		if !owned[nodeKey] {
			return false, fmt.Errorf("node %q of type %q is not assigned to agent %q", item.NodeID, item.NodeType, agentID)
		}
	}
	for _, item := range payload.Items {
		k := usageKey{agentID: agentID, nodeKey: panelapi.NodeKey(item.NodeType, item.NodeID), userID: item.UserID}
		old := s.usage[k]
		old.Upload += item.Upload
		old.Download += item.Download
		s.usage[k] = old
	}
	s.batches[bk] = struct{}{}
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
