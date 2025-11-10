package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	proto "tacoterror/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ---------- LAMPORT CLOCK --------------------------
type LamportClock struct {
	counter int64      // Logical time counter
	mutex   sync.Mutex // Ensures thread-safe access to the counter
}

// Increment advances the Lamport clock by one tick
func (lc *LamportClock) Increment() int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	lc.counter++
	return lc.counter
}

// CompareAndUpdate synchronizes the Lamport clock with a timestamp received from another node
func (lc *LamportClock) CompareAndUpdate(receivedTimestamp int64) int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	if receivedTimestamp > lc.counter {
		lc.counter = receivedTimestamp
	}
	lc.counter++
	return lc.counter
}

// GetTime returns the current logical time value of the Lamport clock
// This is a read-only operation protected by a mutex
func (lc *LamportClock) GetTime() int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	return lc.counter
}

// ---------- NODE IMPLEMENTATION -------------

// Node represents a single participant in the distributed mutual exclusion system
// Each node runs its own instance of this structure, maintaining its Lamport clock, list of peers, and state information
type Node struct {
	ID           int             // unique identifier for the node
	Mutex        sync.Mutex      // protects access to critical state variables
	RequestQueue []proto.Request // stores deferred requests
	Clock        *LamportClock   // Lamport clock instance
	Peers        []string        // list of peer node addresses (host:port)
	inCS         bool            // whether the node is currently in critical section
	ReplyCount   int             // number of replies received for the current request
	HTTPAddr     string          // e.g., ":8080"
}

// RequestCS initiates a request to enter the Critical Section (CS)
// The node increments its Lamport clock, constructs a Request message, and broadcasts it to all peer nodes using gRPC
func (n *Node) RequestCS() {
	n.Mutex.Lock()
	defer n.Mutex.Unlock()

	n.Clock.Increment()
	log.Printf("[Node %d] Requesting critical section (time=%d)", n.ID, n.Clock.GetTime())

	req := &proto.Request{
		NodeID:      int64(n.ID),
		LogicalTime: n.Clock.GetTime(),
	}

	// Notify the local server that we're requesting
	go func(ts int64) {
		go http.Get(fmt.Sprintf("http://localhost%s/requesting?ts=%d", n.HTTPAddr, n.Clock.GetTime()))
	}(n.Clock.GetTime())

	// Reset reply count
	n.ReplyCount = 0

	// Broadcast the request to all peers concurrently and retry until granted
	for _, addr := range n.Peers {
		go func(target string) {
			for {
				log.Printf("SEND Request -> %s ts=%d", target, req.LogicalTime) // For logs

				// Connect to peer via gRPC
				conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					log.Printf("[Node %d] Connection error to %s: %v", n.ID, target, err)
					time.Sleep(500 * time.Millisecond)
					continue
				}
				client := proto.NewRicartServiceClient(conn)
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				reply, err := client.RequestAccess(ctx, req)
				conn.Close()
				if err != nil {
					log.Printf("[Node %d] RPC error to %s: %v", n.ID, target, err)
					time.Sleep(500 * time.Millisecond)
					continue
				}
				if reply.GetStatus() {
					log.Printf("REPLY <- %s GRANT", target)
					n.ReceiveReply()
					return
				}

				// Deferred, backoff and retry
				time.Sleep(300 * time.Millisecond)
			}
		}(addr)
	}
}

// EnterCS simulates the node entering the critical section
// It logs entry and exit events and sleeps for a few seconds to represent work being done in the CS
func (n *Node) EnterCS() {
	n.inCS = true
	log.Printf("[Node %d] ENTERED critical section", n.ID)

	// notify local server
	go http.Get(fmt.Sprintf("http://localhost%s/enter", n.HTTPAddr))

	// Simulate doing some work inside the CS
	time.Sleep(3 * time.Second)

	log.Printf("[Node %d] EXITING critical section", n.ID)
	n.inCS = false

	// After exiting, process any deferred requests
	n.ReleaseDeferred()
}

// ReleaseDeferred handles all deferred requests that were postponed while this node was inside the critical section
// In this simplified version, it just clears the deferred queue
func (n *Node) ReleaseDeferred() {
	n.Mutex.Lock()
	defer n.Mutex.Unlock()

	log.Printf("[Node %d] Releasing deferred requests", n.ID)

	// notify local server that we've released
	go http.Get(fmt.Sprintf("http://localhost%s/release", n.HTTPAddr))
	n.RequestQueue = nil
}

// ReceiveReply should be called when this node receives a reply from a peer
// Increments the ReplyCount, and when all replies have been received, the node can safely enter the critical section
func (n *Node) ReceiveReply() {
	n.Mutex.Lock()

	n.ReplyCount++
	log.Printf("[Node %d] Received reply (%d/%d)", n.ID, n.ReplyCount, len(n.Peers))

	n.Mutex.Unlock()

	// If all peers have replied, enter the critical section
	if n.ReplyCount == len(n.Peers) {
		n.EnterCS()
	}
}

// ------------------ MAIN --------------------------------
func main() {
	idFlag := flag.Int("id", 1, "Node ID")
	peersFlag := flag.String("peers", "", "Comma-separated peer addresses")
	httpCtl := flag.String("http", ":8080", "Local server HTTP address, e.g. :8080")
	flag.Parse()

	var peers []string
	if *peersFlag != "" {
		for _, p := range strings.Split(*peersFlag, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				peers = append(peers, p)
			}
		}
	}

	n := &Node{
		ID:       *idFlag,
		Clock:    &LamportClock{},
		Peers:    peers,
		HTTPAddr: *httpCtl,
	}

	// ----------------- CREATE LOG FILES ----------------------
	log.SetPrefix(fmt.Sprintf("[N%d] ", n.ID))

	os.MkdirAll("nodes/node_logs", 0o755)
	f, err := os.OpenFile(fmt.Sprintf("nodes/node_logs/node_%d.log", n.ID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		log.SetOutput(io.MultiWriter(os.Stdout, f))
	} else {
		log.Printf("WARN: could not open node log file: %v", err)
	}

	n.RequestCS()
	// Keeps one goroutine alive -> otherwise will deadlock before running a second node
	go func() {
		for range time.NewTicker(time.Hour).C {
		}
	}()
	select {}
}
