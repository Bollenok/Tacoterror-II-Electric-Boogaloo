package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"

	pb "tacoterror/proto"

	"google.golang.org/grpc"
)

// ------------------- LAMPORT CLOCK IMPLEMENTATION --------------------
type LamportClock struct {
	counter int64
	mutex   sync.Mutex
}

// Increment increases the clock value by 1 for a local event (e.g., sending a request)
func (lc *LamportClock) Increment() int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	lc.counter++
	return lc.counter
}

// CompareAndUpdate updates the local Lamport clock when a timestamp is received from another node
// Ensures that logical time always moves forward
func (lc *LamportClock) CompareAndUpdate(receivedTimestamp int64) int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	if receivedTimestamp > lc.counter {
		lc.counter = receivedTimestamp
	}
	lc.counter++
	return lc.counter
}

// GetTime returns the current Lamport clock value
func (lc *LamportClock) GetTime() int64 {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	return lc.counter
}

// ------------------ RICART–AGRAWALA SERVER IMPLEMENTATION ---------------------

// RicartServer implements the gRPC RicartService
// Each node runs one instance of this server to receive requests from other nodes that want to access the critical section (CS)
type RicartServer struct {
	pb.UnimplementedRicartServiceServer
	NodeID     int32
	Clock      *LamportClock
	mutex      sync.Mutex
	inCS       bool
	requesting bool
	ourTS      int64
}

// RequestAccess handles incoming requests from other nodes that want to enter the critical section
// This function updates the local Lamport clock based on the received timestamp and then sends back an acknowledgment (Reply)
func (s *RicartServer) RequestAccess(ctx context.Context, req *pb.Request) (*pb.Reply, error) {
	newTime := s.Clock.CompareAndUpdate(req.LogicalTime)

	log.Printf("[Node %d] Received REQUEST from Node %d (logical_time=%d) | Updated clock=%d",
		s.NodeID, req.NodeID, req.LogicalTime, newTime)

	// RA decision: grant unless (inCS) or (requesting with higher priority)
	// Priority: lower (ts,id) wins
	s.mutex.Lock()
	inCS := s.inCS
	reqMe := s.requesting
	ourTS := s.ourTS
	s.mutex.Unlock()

	grant := true
	if inCS {
		grant = false
	} else if reqMe {
		if ourTS < req.LogicalTime || (ourTS == req.LogicalTime && int64(s.NodeID) < req.NodeID) {
			grant = false
		}
	}

	// For logs
	decision := "GRANT"
	if !grant {
		decision = "DEFER"
	}
	log.Printf("REQ from Node %d ts=%d | state: inCS=%v requesting=%v ourTS=%d => %s",
		req.NodeID, req.LogicalTime, inCS, reqMe, ourTS, decision)
	return &pb.Reply{Status: grant}, nil
}

func main() {
	grpcAddr := flag.String("grpc", ":50051", "gRPC listen address, e.g. :50051")
	httpAddr := flag.String("http", ":8080", "HTTP listen address, e.g. :8080")
	idFlag := flag.Int("id", 1, "Node ID")
	flag.Parse()

	nodeID := int32(*idFlag)

	// ----------------- CREATE LOG FILES ----------------------
	log.SetPrefix(fmt.Sprintf("[S%d] ", nodeID))

	// Write to a per-server log file
	os.MkdirAll("Server/server_logs", 0o755)
	f, err := os.OpenFile(fmt.Sprintf("Server/server_logs/server_%d.log", nodeID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		log.SetOutput(io.MultiWriter(os.Stdout, f))
	} else {
		log.Printf("WARN: could not open server log file: %v", err)
	}

	// Initialize Lamport clock and server
	clock := &LamportClock{}

	server := &RicartServer{
		NodeID: nodeID,
		Clock:  clock,
	}

	// Start gRPC server in a goroutine
	go func() {
		listen, err := net.Listen("tcp", *grpcAddr)
		if err != nil {
			log.Fatalf("Failed to listen on %s: %v", *grpcAddr, err)
		}

		grpcServer := grpc.NewServer()
		pb.RegisterRicartServiceServer(grpcServer, server)

		log.Printf("Node %d running gRPC server on %s", nodeID, *grpcAddr)
		if err := grpcServer.Serve(listen); err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
	}()

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Node %d running. Lamport clock=%d\n", nodeID, clock.GetTime())
	})

	// Control endpoints for local node
	http.HandleFunc("/requesting", func(w http.ResponseWriter, r *http.Request) {
		tsStr := r.URL.Query().Get("ts")
		var ts int64
		if tsStr != "" {
			fmt.Sscan(tsStr, &ts)
		}
		server.mutex.Lock()
		server.requesting = true
		server.ourTS = ts
		server.mutex.Unlock()
		fmt.Fprint(w, "ok")
	})
	http.HandleFunc("/enter", func(w http.ResponseWriter, r *http.Request) {
		server.mutex.Lock()
		server.inCS = true
		server.mutex.Unlock()
		fmt.Fprint(w, "ok")
	})
	http.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		server.mutex.Lock()
		server.inCS = false
		server.requesting = false
		server.ourTS = 0
		server.mutex.Unlock()
		fmt.Fprint(w, "ok")
	})

	log.Printf("HTTP status endpoint available at http://localhost%s/status", *httpAddr)
	log.Printf("HTTP will listen on %q", *httpAddr)
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
}
