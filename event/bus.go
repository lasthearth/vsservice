package event

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Bus manages event distribution to subscribers
type Bus struct {
	listeners map[Type][]listener
	mu        sync.RWMutex
}

// listener represents a subscriber with a unique ID and channel
type listener struct {
	id uuid.UUID
	ch chan<- Event
}

// NewBus creates a ready-to-use event bus
func NewBus() *Bus {
	return &Bus{
		listeners: make(map[Type][]listener),
	}
}

// Start begins listening to event sources and distributing events
func (b *Bus) Start(ctx context.Context, sources ...<-chan Event) {
	for _, source := range sources {
		go func(s <-chan Event) {
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-s:
					if !ok {
						return
					}
					b.Dispatch(event)
				}
			}
		}(source)
	}
}

// Dispatch sends event to all subscribed listeners
func (b *Bus) Dispatch(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	listeners := b.listeners[event.Type]
	for _, l := range listeners {
		select {
		case l.ch <- event:
		}
	}
}

// Subscribe returns a channel to receive specified event type and a subscription ID
func (b *Bus) Subscribe(eventType Type) (<-chan Event, uuid.UUID) {
	ch := make(chan Event)
	id := uuid.New()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.listeners[eventType] = append(
		b.listeners[eventType],
		listener{
			id: id,
			ch: ch,
		},
	)
	return ch, id
}

// Unsubscribe removes a listener by ID and closes its channel
func (b *Bus) Unsubscribe(eventType Type, id uuid.UUID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	listeners := b.listeners[eventType]
	for i, l := range listeners {
		if l.id == id {
			// Remove the listener
			b.listeners[eventType] = append(
				listeners[:i],
				listeners[i+1:]...,
			)

			close(l.ch)
			break
		}
	}

	// Clean up if no listeners remain
	if len(b.listeners[eventType]) == 0 {
		delete(b.listeners, eventType)
	}
}
