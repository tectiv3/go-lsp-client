package events

import (
	"sync/atomic"
)

// Stats provides events statistics.
type stats struct {
	// EventsFired is the number of listener calls
	EventsFired uint64
	// Subscribers is the total number of subscribers
	Subscribers int64
}

// Snapshot returns statistics' snapshot.
// Use stats returned from Stats.Snapshot() since the original stats can be updated by concurrently running goroutines.
func (es *stats) snapshot() *stats {
	return &stats{
		EventsFired: atomic.LoadUint64(&es.EventsFired),
		Subscribers: atomic.LoadInt64(&es.Subscribers),
	}
}

func (es *stats) incFiredEvents() {
	atomic.AddUint64(&es.EventsFired, 1)
}

func (es *stats) incSubscribers(count int) {
	atomic.AddInt64(&es.Subscribers, int64(count))
}

func (es *stats) decSubscribers() {
	atomic.AddInt64(&es.Subscribers, -1)
}

func (es *stats) resetSubscribers() {
	atomic.StoreInt64(&es.Subscribers, 0)
}
