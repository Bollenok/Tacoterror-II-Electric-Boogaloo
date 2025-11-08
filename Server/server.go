package main

import (
	"fmt"
	"net/http"
	"sync"
	proto "tacoterror/proto"
)

// -------------------------- LAMPORT CLOCK ------------------------------------
// LamportClock holds the state of a Lamport clock
type LamportClock struct {
	counter int64
	mutex   sync.Mutex
}

// Increment advances the clock for a local event and returns the new time
func (lc *LamportClock) Increment() int64 {
	lc.mutex.Lock()
	lc.counter++
	t := lc.counter
	lc.mutex.Unlock()
	return t
}

func (lc *LamportClock) CompareAndUpdate(receivedTimestamp int64) int64 {
	lc.mutex.Lock()
	if receivedTimestamp > lc.counter {
		lc.counter = receivedTimestamp
	}
	lc.counter++
	t := lc.counter
	lc.mutex.Unlock()
	return t
}

func (lc *LamportClock) GetTime() int64 {
	lc.mutex.Lock()
	t := lc.counter
	lc.mutex.Unlock()
	return t
}

func main() {
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		response := &proto.StatusResponse{
			Status:  "Server is running",
			Uptime:  12345,
			Version: "1.0.0",
		}
		fmt.Fprintf(w, "Status: %s, Uptime: %d, Version: %s", response.Status, response.Uptime, response.Version)
	})
	fmt.Println("Starting server on :8080")
	http.ListenAndServe(":8080", nil)
}
