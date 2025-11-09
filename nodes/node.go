package main

import (
	"log"
	"sync"

	proto "tacoterror/proto" // Adjust this import path if needed
)

// ============================================================================
// LAMPORT CLOCK IMPLEMENTATION
// ============================================================================

// LamportClock represents a Lamport logical clock used to order events
// across distributed nodes without relying on synchronized physical clocks.
type LamportClock struct {
	counter int64
	mutex   sync.Mutex
}

// Increment increases the clock value by 1 for a local event (e.g., sending a request).
func (lc *LamportClock) Increment() int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	lc.counter++
	return lc.counter
}

// CompareAndUpdate updates the local Lamport clock when a timestamp is received
// from another node. Ensures that logical time always moves forward.
func (lc *LamportClock) CompareAndUpdate(receivedTimestamp int64) int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	if receivedTimestamp > lc.counter {
		lc.counter = receivedTimestamp
	}
	lc.counter++
	return lc.counter
}

// GetTime returns the current Lamport clock value.
func (lc *LamportClock) GetTime() int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	return lc.counter
}

// i do something me not know what :D

type Node struct {
	ID           int
	Mutex        sync.Mutex
	RequestQueue []proto.Request
	Clock        *LamportClock
	Peers        []string
	inCS         bool
	ReplyCount   int
}

func (n *Node) RequestCS() {
	n.Mutex.Lock()
	n.ReplyCount = 0
	defer n.Mutex.Unlock()

	n.Clock.Increment()
	log.Printf("[Node %d] Requesting critical section (time=%d)", n.ID, n.Clock.GetTime())

	req := &proto.Request{
		NodeID:      int64(n.ID),
		LogicalTime: n.Clock.GetTime(),
	}

}
func (n *Node) RecieveReply() {}

func main() {

}
