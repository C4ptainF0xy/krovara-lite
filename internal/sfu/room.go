package sfu

import (
	"log/slog"
	"sync"
)

type Hub struct {
	log *slog.Logger

	mu    sync.Mutex
	rooms map[string]*Room
}

func NewHub(log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{log: log, rooms: map[string]*Room{}}
}

func (h *Hub) GetOrCreate(id string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.rooms[id]; ok {
		return r
	}
	r := newRoom(h, id)
	h.rooms[id] = r
	return r
}

func (h *Hub) drop(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms, id)
}

func (h *Hub) Rooms() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.rooms))
	for id := range h.rooms {
		out = append(out, id)
	}
	return out
}

type Room struct {
	id  string
	hub *Hub

	mu    sync.RWMutex
	peers map[string]*Peer
}

func newRoom(h *Hub, id string) *Room {
	return &Room{id: id, hub: h, peers: map[string]*Peer{}}
}

func (r *Room) ID() string { return r.id }

func (r *Room) Join(p *Peer) []*Peer {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing := make([]*Peer, 0, len(r.peers))
	for _, other := range r.peers {
		existing = append(existing, other)
	}
	r.peers[p.id] = p
	return existing
}

func (r *Room) Leave(peerID string) {
	r.mu.Lock()
	delete(r.peers, peerID)
	empty := len(r.peers) == 0
	r.mu.Unlock()
	if empty {
		r.hub.drop(r.id)
	}
}

func (r *Room) Peers() []*Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Peer, 0, len(r.peers))
	for _, p := range r.peers {
		out = append(out, p)
	}
	return out
}

func (r *Room) Peer(peerID string) *Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.peers[peerID]
}

func (r *Room) Others(peerID string) []*Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Peer, 0, len(r.peers))
	for id, p := range r.peers {
		if id == peerID {
			continue
		}
		out = append(out, p)
	}
	return out
}
