package events

import "sync"

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type Bus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewBus() *Bus { return &Bus{subs: map[chan Event]struct{}{}} }

func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}

func (b *Bus) Publish(typ string, data any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- Event{Type: typ, Data: data}:
		default:
		}
	}
}
