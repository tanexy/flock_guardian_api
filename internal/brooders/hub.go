package brooders

import "sync"

type Hub struct {
	mu   sync.Mutex
	subs map[uint][]chan SensorUpdate
}

func NewHub() *Hub {
	return &Hub{subs: make(map[uint][]chan SensorUpdate)}
}

func (h *Hub) Subscribe(brooderID uint) chan SensorUpdate {
	ch := make(chan SensorUpdate, 4)
	h.mu.Lock()
	h.subs[brooderID] = append(h.subs[brooderID], ch)
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(brooderID uint, ch chan SensorUpdate) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.subs[brooderID]
	for i, c := range list {
		if c == ch {
			h.subs[brooderID] = append(list[:i], list[i+1:]...)
			close(ch)
			return
		}
	}
}

func (h *Hub) Publish(brooderID uint, data SensorUpdate) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs[brooderID] {
		select {
		case ch <- data:
		default:
		}
	}
}
