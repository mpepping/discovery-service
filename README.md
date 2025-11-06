# Discovery Service

An ephemeral cluster discovery service that enables nodes to exchange membership information. Complatible with Talos Linux service discovery and MIT licensed.

## Features

- **gRPC API** - Full-featured gRPC service for cluster discovery
- **Real-time Updates** - Streaming watch API for real-time affiliate changes
- **Privacy-First** - Stores only encrypted data blobs, never has decryption keys
- **Ephemeral Storage** - All data in-memory with configurable TTL (default 30 minutes)
- **Rate Limiting** - Per-IP rate limiting to prevent abuse
- **Prometheus Metrics** - Built-in metrics for monitoring
- **HTTP Landing Page** - Web UI for cluster inspection
- **Auto Garbage Collection** - Automatic cleanup of expired affiliates

## Quick Start

### Building from Source

```bash
# Clone the repository
git clone https://github.com/mpepping/discovery-service
cd discovery-service

# Build the binary
make build

# Run the service
./bin/discovery-service
```

### Using Docker

```bash
# Build the Docker image
make docker-build

# Run the container
docker run -p 3000:3000 -p 3001:3001 -p 2122:2122 discovery-service:latest
```

### Using Pre-built Binaries

Download the latest release from the [releases page](https://github.com/mpepping/discovery-service/releases).

## Configuration

The service is configured via command-line flags:

```bash
./bin/discovery-service \
  -listen-addr :3000 \
  -landing-addr :3001 \
  -metrics-addr :2122 \
  -gc-interval 1m \
  -debug
```

### Available Flags

| Flag                 | Default | Description                                   |
| -------------------- | ------- | --------------------------------------------- |
| `-listen-addr`       | `:3000` | gRPC server listen address                    |
| `-landing-addr`      | `:3001` | HTTP landing page address (empty to disable)  |
| `-metrics-addr`      | `:2122` | Prometheus metrics address (empty to disable) |
| `-redirect-endpoint` | ``      | Optional redirect endpoint for Hello RPC      |
| `-gc-interval`       | `1m`    | Garbage collection interval                   |
| `-debug`             | `false` | Enable debug logging                          |

### Environment Variables

Configuration can be provided via environment variables (overridden by command-line flags):

| Variable            | Default | Description                                   |
| ------------------- | ------- | --------------------------------------------- |
| `LISTEN_ADDR`       | `:3000` | gRPC server listen address                    |
| `LANDING_ADDR`      | `:3001` | HTTP landing page address (empty to disable)  |
| `METRICS_ADDR`      | `:2122` | Prometheus metrics address (empty to disable) |
| `REDIRECT_ENDPOINT` | ``      | Optional redirect endpoint for Hello RPC      |
| `GC_INTERVAL`       | `1m`    | Garbage collection interval                   |
| `DEBUG`             | `false` | Enable debug logging (true/1/yes)             |
| `MODE`              | ``      | Set to `development` for colored log output   |

## API Reference

### gRPC Service

The service implements the `Cluster` service with the following methods:

#### Hello

Initial handshake that returns the client's observed IP address.

```protobuf
rpc Hello(HelloRequest) returns (HelloResponse);
```

#### AffiliateUpdate

Create or update an affiliate record with encrypted data and endpoints.

```protobuf
rpc AffiliateUpdate(AffiliateUpdateRequest) returns (AffiliateUpdateResponse);
```

#### AffiliateDelete

Remove an affiliate from the cluster.

```protobuf
rpc AffiliateDelete(AffiliateDeleteRequest) returns (AffiliateDeleteResponse);
```

#### List

Retrieve all current affiliates for a cluster.

```protobuf
rpc List(ListRequest) returns (ListResponse);
```

#### Watch

Stream real-time affiliate changes (snapshot + deltas).

```protobuf
rpc Watch(WatchRequest) returns (stream WatchResponse);
```

### HTTP Endpoints

- `GET /` - Landing page with service information
- `GET /inspect?clusterID=<id>` - Cluster inspection UI
- `GET /metrics` - Prometheus metrics (port 2122)

## Limits and Constraints

- **Max Affiliates per Cluster:** 100
- **Max Endpoints per Affiliate:** 20
- **Default TTL:** 30 minutes (configurable via request)
- **Rate Limit:** 15 requests/second per IP
- **Burst Allowance:** 60 requests per IP

## Testing with grpcurl

```bash
# Install grpcurl
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# Test Hello endpoint
grpcurl -plaintext \
  -d '{"cluster_id": "test-cluster", "client_version": "1.0.0"}' \
  localhost:3000 \
  discovery.cluster.v1.Cluster/Hello

# Update an affiliate
grpcurl -plaintext \
  -d '{"cluster_id": "test-cluster", "affiliate_id": "node1", "data": "ZW5jcnlwdGVkLWRhdGE="}' \
  localhost:3000 \
  discovery.cluster.v1.Cluster/AffiliateUpdate

# List affiliates
grpcurl -plaintext \
  -d '{"cluster_id": "test-cluster"}' \
  localhost:3000 \
  discovery.cluster.v1.Cluster/List

# Watch for changes
grpcurl -plaintext \
  -d '{"cluster_id": "test-cluster"}' \
  localhost:3000 \
  discovery.cluster.v1.Cluster/Watch
```

## Prometheus Metrics

The service exposes the following metrics on `/metrics`:

- `discovery_clusters_total` - Total number of clusters
- `discovery_affiliates_total` - Total number of affiliates
- `discovery_endpoints_total` - Total number of endpoints
- `discovery_subscriptions_total` - Active watch subscriptions
- `discovery_gc_runs_total` - Total GC runs
- `discovery_gc_collected_clusters_total` - Clusters collected by GC
- `discovery_gc_collected_affiliates_total` - Affiliates collected by GC
- `discovery_hello_requests_total` - Hello requests by client version

## Development

### Prerequisites

- Go 1.23 or later
- Make (optional)
- Docker (optional)

### Project Structure

```
discovery-service/
├── api/                    # API definitions and generated code
│   └── cluster/v1/        # Cluster service protobuf
├── cmd/                    # Command-line applications
│   └── discovery-service/ # Main service entry point
├── internal/               # Private application code
│   ├── landing/           # HTTP landing page
│   ├── limiter/           # Rate limiting
│   └── state/             # State management
├── pkg/                    # Public library code
│   ├── limits/            # Limit constants
│   └── server/            # gRPC server implementation
├── Dockerfile             # Container image
├── Makefile              # Build automation
└── README.md             # This file
```

### Building

```bash
# Build the binary
make build

# Run tests
make test

# Build Docker image
make docker-build

# Run locally
make run

# Clean build artifacts
make clean
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with race detector
go test -race ./...
```

## Deployment

### Docker Compose

```yaml
version: "3.8"
services:
  discovery:
    image: discovery-service:latest
    ports:
      - "3000:3000" # gRPC
      - "3001:3001" # HTTP
      - "2122:2122" # Metrics
    command:
      - "-debug"
      - "-gc-interval=1m"
```

### Kubernetes

See the `deploy/kubernetes/` directory for example manifests.

```bash
kubectl apply -f deploy/kubernetes/
```

## API Compatibility

This implementation maintains wire-protocol compatibility with the original Talos discovery service:

- ✅ Identical gRPC method signatures
- ✅ Same message types and field numbers
- ✅ Compatible behavior (TTL, expiration, rate limiting)
- ✅ Same default ports (3000, 3001, 2122)

## Architecture

### State Management

- **In-Memory Storage:** All data stored in memory with mutex-protected maps
- **Thread-Safe:** Concurrent access handled via RWMutex
- **Garbage Collection:** Periodic cleanup of expired affiliates
- **Subscriptions:** Real-time notifications via buffered channels (32 elements)

### Rate Limiting

- **Per-IP Tracking:** Individual rate limiters for each client IP
- **Token Bucket:** 15 req/s with burst of 60
- **Automatic Cleanup:** Unused limiters cleaned up every minute

### Security Model

1. **No Authentication** - Service should be deployed behind firewall/VPN
2. **Encrypted Data** - All affiliate data is client-encrypted
3. **No Decryption** - Service never has keys to decrypt stored data
4. **Rate Limiting** - Prevents abuse via IP-based throttling
5. **Input Validation** - Strict validation of all inputs

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`make test`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

This project maintains API compatibility with the original [Talos discovery service](https://github.com/siderolabs/discovery-service) while using a different license and implementation.

## Support

- **Issues:** [GitHub Issues](https://github.com/mpepping/discovery-service/issues)
- **Documentation:** [API Research Document](API_RESEARCH.md)

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for release history.
