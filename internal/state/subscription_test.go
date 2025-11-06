package state

import (
	"testing"
	"time"
)

func TestNewSubscription(t *testing.T) {
	sub := NewSubscription()

	if sub == nil {
		t.Fatal("NewSubscription returned nil")
	}

	if sub.ch == nil {
		t.Error("subscription channel not initialized")
	}

	if sub.closed {
		t.Error("subscription should not be closed initially")
	}
}

func TestSubscriptionSend(t *testing.T) {
	sub := NewSubscription()

	notification := Notification{
		Affiliate: &Affiliate{ID: "test"},
		Deleted:   false,
	}

	// Send notification
	ok := sub.Send(notification)
	if !ok {
		t.Error("Send returned false")
	}

	// Receive notification
	select {
	case received := <-sub.Ch():
		if received.Affiliate.ID != "test" {
			t.Errorf("expected affiliate ID 'test', got %q", received.Affiliate.ID)
		}
		if received.Deleted {
			t.Error("expected Deleted to be false")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for notification")
	}
}

func TestSubscriptionSendMultiple(t *testing.T) {
	sub := NewSubscription()

	count := 10
	for i := 0; i < count; i++ {
		notification := Notification{
			Affiliate: &Affiliate{ID: string(rune('a' + i))},
			Deleted:   false,
		}
		ok := sub.Send(notification)
		if !ok {
			t.Errorf("Send failed at iteration %d", i)
		}
	}

	// Receive all notifications
	for i := 0; i < count; i++ {
		select {
		case <-sub.Ch():
			// Expected
		case <-time.After(100 * time.Millisecond):
			t.Errorf("timeout waiting for notification %d", i)
		}
	}
}

func TestSubscriptionSendFull(t *testing.T) {
	sub := NewSubscription()

	// Fill the channel (buffer is 32)
	for i := 0; i < 32; i++ {
		notification := Notification{
			Affiliate: &Affiliate{ID: "test"},
			Deleted:   false,
		}
		ok := sub.Send(notification)
		if !ok {
			t.Errorf("Send failed at iteration %d", i)
		}
	}

	// Next send should fail (non-blocking)
	notification := Notification{
		Affiliate: &Affiliate{ID: "overflow"},
		Deleted:   false,
	}
	ok := sub.Send(notification)
	if ok {
		t.Error("Send should return false when channel is full")
	}
}

func TestSubscriptionSendAfterClose(t *testing.T) {
	sub := NewSubscription()

	// Close subscription
	sub.Close()

	// Send should return false
	notification := Notification{
		Affiliate: &Affiliate{ID: "test"},
		Deleted:   false,
	}
	ok := sub.Send(notification)
	if ok {
		t.Error("Send should return false after close")
	}
}

func TestSubscriptionClose(t *testing.T) {
	sub := NewSubscription()

	// Send a notification
	sub.Send(Notification{
		Affiliate: &Affiliate{ID: "test"},
		Deleted:   false,
	})

	// Close subscription
	sub.Close()

	if !sub.closed {
		t.Error("closed flag not set")
	}

	// Channel should be closed but we can still receive buffered messages
	select {
	case notification, ok := <-sub.Ch():
		if !ok {
			t.Error("channel closed before draining buffered messages")
		}
		if notification.Affiliate.ID != "test" {
			t.Error("received wrong notification")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout receiving buffered notification")
	}

	// Now channel should be closed
	select {
	case _, ok := <-sub.Ch():
		if ok {
			t.Error("channel not closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout checking channel close")
	}
}

func TestSubscriptionCloseIdempotent(t *testing.T) {
	sub := NewSubscription()

	// Close multiple times should not panic
	sub.Close()
	sub.Close()
	sub.Close()
}

func TestSubscriptionChReadOnly(t *testing.T) {
	sub := NewSubscription()

	// Ch() should return read-only channel
	ch := sub.Ch()

	// This should compile (read from channel)
	go func() {
		<-ch
	}()

	// This would NOT compile (write to read-only channel):
	// ch <- Notification{}
}
