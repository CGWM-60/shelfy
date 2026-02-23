package events

import (
	"sync"
	"time"
)

type Event struct {
	Type          string      `json:"type"`
	CorrelationID string      `json:"correlationId"`
	Payload       interface{} `json:"payload"`
	At            time.Time   `json:"at"`
}

type Bus struct {
	mu   sync.RWMutex
	next int
	subs map[int]chan Event
}

func NewBus() *Bus {
	return &Bus{subs: map[int]chan Event{}}
}

func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (b *Bus) Subscribe(buffer int) (int, <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	ch := make(chan Event, buffer)
	b.subs[id] = ch
	return id, ch
}

func (b *Bus) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch, ok := b.subs[id]
	if ok {
		delete(b.subs, id)
		close(ch)
	}
}
