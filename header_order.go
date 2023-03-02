package http

import (
	"strings"
	"sync"
)

// HeaderOrder create header order
type HeaderOrder struct {
	mu    sync.RWMutex
	order []string
}

func (h *HeaderOrder) Add(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	formatted := h.toLower(key)

	if h.FindIndex(formatted) != -1 {
		return
	}

	h.order = append(h.order, formatted)
}

func (h *HeaderOrder) Del(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	formatted := h.toLower(key)
	i := h.FindIndex(formatted)

	if i == -1 {
		return
	}

	h.order = append(h.order[:i], h.order[i+1:]...)
}

func (h *HeaderOrder) FindIndex(key string) int {
	for i, value := range h.order {
		if value == key {
			return i
		}
	}

	return -1
}

func (h *HeaderOrder) toLower(key string) string {
	return strings.ToLower(key)
}
