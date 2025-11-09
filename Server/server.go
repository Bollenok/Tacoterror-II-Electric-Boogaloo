package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"

	pb "tacoterror/proto" // Adjust this import path if needed

	"google.golang.org/grpc"
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

// ============================================================================
// RICART–AGRAWALA SERVER IMPLEMENTATION
// ============================================================================

// RicartServer implements the gRPC RicartService.
// Each node in the system runs one instance of this server to receive requests
// from other nodes that want to access the critical section (CS).
type RicartServer struct {
	pb.UnimplementedRicartServiceServer
	NodeID int32
	Clock  *LamportClock
}

// RequestAccess handles incoming requests from other nodes that want to
// enter the critical section.
//
// This function updates the local Lamport clock based on the received timestamp
// and then sends back an acknowledgment (Reply).
func (s *RicartServer) RequestAccess(ctx context.Context, req *pb.Request) (*pb.Reply, error) {
	newTime := s.Clock.CompareAndUpdate(req.LogicalTime)

	log.Printf("[Node %d] Received REQUEST from Node %d (logical_time=%d) | Updated clock=%d",
		s.NodeID, req.NodeID, req.LogicalTime, newTime)

	// Always grant access in this simplified implementation not actually fully implemented
	return &pb.Reply{
		Status: true, // Grant access
	}, nil
}

// ============================================================================
// MAIN FUNCTION — SERVER ENTRY POINT
// ============================================================================

func main() {
	// Node configuration — these could later be CLI flags or config values
	nodeID := int32(1)
	grpcPort := ":50051"

	// Initialize Lamport clock and Ricart server
	clock := &LamportClock{}
	server := &RicartServer{
		NodeID: nodeID,
		Clock:  clock,
	}

	// Start gRPC server in a goroutine
	go func() {
		listen, err := net.Listen("tcp", grpcPort)
		if err != nil {
			log.Fatalf("Failed to listen on %s: %v", grpcPort, err)
		}

		grpcServer := grpc.NewServer()
		pb.RegisterRicartServiceServer(grpcServer, server)

		log.Printf("Node %d running gRPC server on %s", nodeID, grpcPort)
		if err := grpcServer.Serve(listen); err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
	}()

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Node %d running. Lamport clock=%d\n", nodeID, clock.GetTime())
	})

	log.Println("HTTP status endpoint available at http://localhost:8080/status")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
