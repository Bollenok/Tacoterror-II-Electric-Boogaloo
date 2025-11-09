package main

import (
	"context"
	"log"
	"sync"
	"time"

	proto "tacoterror/proto" // Adjust this import path if needed

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ============================================================================
// LAMPORT CLOCK IMPLEMENTATION
// ============================================================================

// LamportClock represents a Lamport logical clock used to order events
// across distributed nodes without relying on synchronized physical clocks.
//
// Each node in the distributed system maintains its own logical clock.
// This clock ensures that even though nodes do not share a physical clock,
// they can still agree on a consistent order of events (e.g., requests).
type LamportClock struct {
	counter int64      // logical time counter
	mutex   sync.Mutex // ensures thread-safe access to the counter
}

// Increment advances the Lamport clock by one tick.
// This method should be called for each local event (e.g., sending a message).
func (lc *LamportClock) Increment() int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	lc.counter++
	return lc.counter
}

// CompareAndUpdate synchronizes the Lamport clock with a timestamp received
// from another node. It sets the local clock to max(local, received) + 1.
// This ensures that logical time always moves forward.
func (lc *LamportClock) CompareAndUpdate(receivedTimestamp int64) int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	if receivedTimestamp > lc.counter {
		lc.counter = receivedTimestamp
	}
	lc.counter++
	return lc.counter
}

// GetTime returns the current logical time value of the Lamport clock.
// This is a read-only operation protected by a mutex.
func (lc *LamportClock) GetTime() int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	return lc.counter
}

// ============================================================================
// NODE IMPLEMENTATION
// ============================================================================

// Node represents a single participant in the distributed mutual exclusion system.
// Each node runs its own instance of this structure, maintaining its Lamport clock,
// list of peers, and state information.
type Node struct {
	ID           int             // unique identifier for this node
	Mutex        sync.Mutex      // protects access to critical state variables
	RequestQueue []proto.Request // stores deferred requests
	Clock        *LamportClock   // Lamport clock instance
	Peers        []string        // list of peer node addresses (host:port)
	inCS         bool            // whether the node is currently in critical section
	ReplyCount   int             // number of replies received for the current request
}

// RequestCS initiates a request to enter the Critical Section (CS).
// The node increments its Lamport clock, constructs a Request message,
// and broadcasts it to all peer nodes using gRPC.
func (n *Node) RequestCS() {
	n.Mutex.Lock()
	defer n.Mutex.Unlock()

	n.Clock.Increment()
	log.Printf("[Node %d] Requesting critical section (time=%d)", n.ID, n.Clock.GetTime())

	req := &proto.Request{
		NodeID:      int64(n.ID),
		LogicalTime: n.Clock.GetTime(),
	}

	// Broadcast the request to all peers concurrently
	for _, addr := range n.Peers {
		go func(target string) {
			// Connect to peer via gRPC
			conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Printf("[Node %d] Failed to connect to %s: %v", n.ID, target, err)
				return
			}
			defer conn.Close()

			client := proto.NewRicartServiceClient(conn)
			_, err = client.RequestAccess(context.Background(), req)
			if err != nil {
				log.Printf("[Node %d] Error sending REQUEST to %s: %v", n.ID, target, err)
			} else {
				log.Printf("[Node %d] Sent REQUEST to %s", n.ID, target)
			}
		}(addr)
	}
}

// EnterCS simulates the node entering the critical section.
// It logs entry and exit events and sleeps for a few seconds to represent
// work being done in the CS.
func (n *Node) EnterCS() {
	n.inCS = true
	log.Printf("[Node %d] ENTERED critical section", n.ID)

	// Simulate doing some work inside the CS
	time.Sleep(3 * time.Second)

	log.Printf("[Node %d] EXITING critical section", n.ID)
	n.inCS = false

	// After exiting, process any deferred requests
	n.ReleaseDeferred()
}

// ReleaseDeferred handles all deferred requests that were postponed
// while this node was inside the critical section.
// In this simplified version, it just clears the deferred queue.
func (n *Node) ReleaseDeferred() {
	n.Mutex.Lock()
	defer n.Mutex.Unlock()

	log.Printf("[Node %d] Releasing deferred requests", n.ID)
	n.RequestQueue = nil
}

// ReceiveReply should be called when this node receives a reply from a peer.
// It increments the ReplyCount, and when all replies have been received,
// the node can safely enter the critical section.
func (n *Node) ReceiveReply() {
	n.Mutex.Lock()
	defer n.Mutex.Unlock()

	n.ReplyCount++
	log.Printf("[Node %d] Received reply (%d/%d)", n.ID, n.ReplyCount, len(n.Peers))

	// If all peers have replied, enter the critical section
	if n.ReplyCount == len(n.Peers) {
		n.EnterCS()
	}
}

// ============================================================================
// MAIN FUNCTION
// ============================================================================

func main() {
	// Create a new node instance with ID = 1 and two peers.
	node := &Node{
		ID:    1,
		Clock: &LamportClock{},
		Peers: []string{"localhost:50052", "localhost:50053"}, // Example peer addresses
	}

	// Main event loop: periodically request access to the CS
	for {
		node.RequestCS()
		time.Sleep(10 * time.Second)
		node.EnterCS()
		time.Sleep(10 * time.Second)
	}
}
