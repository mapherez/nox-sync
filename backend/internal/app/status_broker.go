package app

import (
	"encoding/json"
	"sync"
)

type statusBroker struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newStatusBroker() *statusBroker {
	return &statusBroker{
		clients: map[chan []byte]struct{}{},
	}
}

func (b *statusBroker) subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 4)

	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		delete(b.clients, ch)
		close(ch)
		b.mu.Unlock()
	}
}

func (b *statusBroker) broadcast(payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.clients {
		select {
		case ch <- body:
		default:
		}
	}
}
