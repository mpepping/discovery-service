package state

// Notification represents a change to an affiliate
type Notification struct {
	Affiliate *Affiliate
	Deleted   bool
}

// Subscription represents a subscriber to cluster changes
type Subscription struct {
	ch     chan Notification
	closed bool
}

// NewSubscription creates a new subscription with a buffered channel
func NewSubscription() *Subscription {
	return &Subscription{
		ch: make(chan Notification, 32),
	}
}

// Ch returns the notification channel
func (s *Subscription) Ch() <-chan Notification {
	return s.ch
}

// Send sends a notification to the subscriber
// Returns false if the channel is full or closed
func (s *Subscription) Send(notification Notification) bool {
	if s.closed {
		return false
	}

	select {
	case s.ch <- notification:
		return true
	default:
		// Channel is full, skip this notification
		return false
	}
}

// Close closes the subscription
func (s *Subscription) Close() {
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}
