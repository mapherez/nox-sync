package app

import (
	"encoding/json"
	"sync"
)

type statusBroker struct {
	mu      sync.Mutex
	clients map[string]map[chan []byte]struct{}
}

func newStatusBroker() *statusBroker {
	return &statusBroker{
		clients: map[string]map[chan []byte]struct{}{},
	}
}

func (b *statusBroker) subscribe(vaultID string) (<-chan []byte, func()) {
	ch := make(chan []byte, 4)

	b.mu.Lock()
	if b.clients[vaultID] == nil {
		b.clients[vaultID] = map[chan []byte]struct{}{}
	}
	b.clients[vaultID][ch] = struct{}{}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		delete(b.clients[vaultID], ch)
		if len(b.clients[vaultID]) == 0 {
			delete(b.clients, vaultID)
		}
		close(ch)
		b.mu.Unlock()
	}
}

func (b *statusBroker) broadcast(vaultID string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.clients[vaultID] {
		select {
		case ch <- body:
		default:
		}
	}
}
