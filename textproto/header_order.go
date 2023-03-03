package textproto

import (
	"strings"
	"sync"
)

// HeaderOrder create header order
type HeaderOrder struct {
	mu    sync.RWMutex
	Order []string
}

func (h *HeaderOrder) Add(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	formatted := h.toLower(key)

	if h.FindIndex(formatted) != -1 {
		return
	}

	h.Order = append(h.Order, formatted)
}

func (h *HeaderOrder) Del(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	formatted := h.toLower(key)
	i := h.FindIndex(formatted)

	if i == -1 {
		return
	}

	h.Order = append(h.Order[:i], h.Order[i+1:]...)
}

func (h *HeaderOrder) FindIndex(key string) int {
	for i, value := range h.Order {
		if value == key {
			return i
		}
	}

	return -1
}

func (h *HeaderOrder) toLower(key string) string {
	return strings.ToLower(key)
}
