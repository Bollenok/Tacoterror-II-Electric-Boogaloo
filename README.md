## Requirements

- Go 1.21+ (or your version that matches `go.mod`)
- Ports available (example defaults):
  - gRPC: `:50051`, `:50052`, `:50053`
  - HTTP: `:8080`, `:8081`, `:8082`

---

## How To Run

You’ll run **N servers** (one per node) and **N node processes** (one per node).  
Servers listen for gRPC; nodes broadcast to their peers and use the local HTTP control endpoints.

### 1) Start three servers (each in its own terminal)

```bash
# Terminal A
go run ./Server --id=1 --grpc=:50051 --http=:8080

# Terminal B
go run ./Server --id=2 --grpc=:50052 --http=:8081

# Terminal C
go run ./Server --id=3 --grpc=:50053 --http=:8082
```

You should see logs like:
```
Node 2 running gRPC server on :50052
HTTP status endpoint available at http://localhost:8081/status
HTTP will listen on ":8081"
```

> If a port is busy, pick another (`--http=:0` lets the OS choose a free port and prints it).

### 2) Start three nodes (each in its own terminal)

```bash
# Terminal D
go run ./nodes --id=1 --peers=localhost:50052,localhost:50053 --http=:8080

# Terminal E
go run ./nodes --id=2 --peers=localhost:50051,localhost:50053 --http=:8081

# Terminal F
go run ./nodes --id=3 --peers=localhost:50051,localhost:50052 --http=:8082
```

- `--peers` must list the **gRPC** addresses of the *other* servers.
- `--http` must match this node’s **own** server HTTP port (so the node can mark its state via `/requesting`, `/enter`, `/release`).

---

## Logfiles
- Servers → `Server/server_logs/server_<id>.log`
- Nodes → `nodes/node_logs/node_<id>.log`
