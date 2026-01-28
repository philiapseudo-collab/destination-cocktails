package events

import (
	"context"
	"encoding/json"
	"sync"
)

// EventType represents the type of event
type EventType string

const (
	EventNewOrder       EventType = "new_order"
	EventOrderCompleted EventType = "order_completed"
	EventStockUpdated   EventType = "stock_updated"
	EventPriceUpdated   EventType = "price_updated"
)

// Event represents a server-sent event
type Event struct {
	Type EventType   `json:"type"`
	Data interface{} `json:"data"`
}

// EventBus manages SSE subscriptions and broadcasts events
type EventBus struct {
	subscribers map[string]chan Event
	mu          sync.RWMutex
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string]chan Event),
	}
}

// Subscribe adds a new subscriber and returns a channel for receiving events
func (eb *EventBus) Subscribe(ctx context.Context, id string) <-chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Create buffered channel to prevent blocking
	ch := make(chan Event, 10)
	eb.subscribers[id] = ch

	// Clean up when context is done
	go func() {
		<-ctx.Done()
		eb.Unsubscribe(id)
	}()

	return ch
}

// Unsubscribe removes a subscriber
func (eb *EventBus) Unsubscribe(id string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if ch, exists := eb.subscribers[id]; exists {
		close(ch)
		delete(eb.subscribers, id)
	}
}

// Publish sends an event to all subscribers
func (eb *EventBus) Publish(eventType EventType, data interface{}) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	event := Event{
		Type: eventType,
		Data: data,
	}

	// Send to all subscribers (non-blocking)
	for _, ch := range eb.subscribers {
		select {
		case ch <- event:
		default:
			// Skip if channel is full (prevents blocking)
		}
	}
}

// PublishNewOrder publishes a new order event
func (eb *EventBus) PublishNewOrder(order interface{}) {
	eb.Publish(EventNewOrder, order)
}

// PublishOrderCompleted publishes an order completed event
func (eb *EventBus) PublishOrderCompleted(orderID string) {
	eb.Publish(EventOrderCompleted, map[string]string{"order_id": orderID})
}

// PublishStockUpdated publishes a stock updated event
func (eb *EventBus) PublishStockUpdated(productID string, stock int) {
	eb.Publish(EventStockUpdated, map[string]interface{}{
		"product_id": productID,
		"stock":      stock,
	})
}

// PublishPriceUpdated publishes a price updated event
func (eb *EventBus) PublishPriceUpdated(productID string, price float64) {
	eb.Publish(EventPriceUpdated, map[string]interface{}{
		"product_id": productID,
		"price":      price,
	})
}

// FormatSSE formats an event as Server-Sent Event string
func FormatSSE(event Event) (string, error) {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return "", err
	}

	return "event: " + string(event.Type) + "\ndata: " + string(data) + "\n\n", nil
}
