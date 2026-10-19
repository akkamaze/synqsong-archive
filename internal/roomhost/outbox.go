package roomhost

import "sync"

// OutboxSize is how many updates one connection may fall behind before it is cut.
const OutboxSize = 64

// Outbox is a connection's queue of updates. When it fills up it gives up for good
// and closes Dropped, so the connection can close and the client can reconnect for
// a fresh snapshot instead of slowing the whole room.
type Outbox struct {
	updates chan Update
	dropped chan struct{}
	once    sync.Once
}

func NewOutbox(size int) *Outbox {
	return &Outbox{updates: make(chan Update, size), dropped: make(chan struct{})}
}

func (o *Outbox) Send(u Update) bool {
	select {
	case <-o.dropped:
		return false

	default:
	}

	select {
	case o.updates <- u:
		return true

	default:
		o.once.Do(func() { close(o.dropped) })

		return false
	}
}

func (o *Outbox) Updates() <-chan Update { return o.updates }

func (o *Outbox) Dropped() <-chan struct{} { return o.dropped }
