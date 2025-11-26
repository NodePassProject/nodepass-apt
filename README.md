# NodePass Classic

**NodePass** is an open-source, lightweight, enterprise-grade TCP/UDP network tunneling solution featuring an all-in-one architecture with separation of control and data channels, along with flexible and high-performance instance control. It supports zero-configuration deployment, intelligent connection pooling, tiered TLS encryption, and seamless protocol conversion. Designed for DevOps professionals and system administrators to effortlessly handle complex network scenarios.

## Key Features

- **Universal Functionality**
  - Basic TCP/UDP tunneling and protocol conversion across diverse networks.
  - Compatible with port mapping, NAT traversal, and traffic relay.
  - Cross-platform, multi-architecture, single binary or container.

- **Connection Pool**
  - Pre-established connections for zero-latency switching and forwarding.
  - Eliminates handshake delays, boosts performance.
  - Auto-scaling with real-time capacity adjustment.

- **Innovative Architecture**
  - Integrated S/C/M architecture, flexible mode switching.
  - Full decoupling of control/data channels.
  - API-instance management, multi-instance collaboration.

- **Multi-level Security**
  - Three TLS modes: plaintext, self-signed, strict validation.
  - Covers development to enterprise security needs.
  - Hot-reload certificates with zero downtime.

- **Minimal Configuration**
  - No config files required, ready to use via CLI.
  - Optimized for CI/CD and containers.
  - Advanced parameters like timeouts and rate limits.

- **Performance**
  - Intelligent scheduling, auto-tuning, ultra-low resource usage.
  - Stable under high concurrency and heavy load.
  - Load balancing, health checks, self-healing and more.

- **Visualization**
  - Rich cross-platform visual frontends.
  - One-click deployment scripts, easy management.
  - Real-time monitoring, API-instance management, traffic stats.

## Quick Start

**Server Mode**
```bash
nodepass "server://:10101/127.0.0.1:8080?log=debug&tls=1"
```

**Client Mode**
```bash
nodepass "client://server:10101/127.0.0.1:8080?min=128"
```

**Master Mode (API)**
```bash
nodepass "master://:10101/api?log=debug&tls=1"
```

## URL Query Parameters

| Parameter | Description              | Default | server | client | master |
|-----------|--------------------------|---------|:------:|:------:|:------:|
| `log`     | Log level                | `info`  |   O    |   O    |   O    |
| `tls`     | TLS encryption mode      | `0`     |   O    |   X    |   O    |
| `crt`     | Custom certificate path  | N/A     |   O    |   X    |   O    |
| `key`     | Custom key path          | N/A     |   O    |   X    |   O    |
| `dns`     | DNS cache TTL            | `5m`    |   O    |   O    |   X    |
| `min`     | Minimum pool capacity    | `64`    |   X    |   O    |   X    |
| `max`     | Maximum pool capacity    | `1024`  |   O    |   X    |   X    |
| `mode`    | Run mode control         | `0`     |   O    |   O    |   X    |
| `dial`    | Source IP for outbound   | `auto`  |   O    |   O    |   X    |
| `read`    | Data read timeout        | `0`     |   O    |   O    |   X    |
| `rate`    | Bandwidth rate limit     | `0`     |   O    |   O    |   X    |
| `slot`    | Maximum connection limit | `65536` |   O    |   O    |   X    |
| `proxy`   | PROXY protocol support   | `0`     |   O    |   O    |   X    |
| `notcp`   | TCP support control      | `0`     |   O    |   O    |   X    |
| `noudp`   | UDP support control      | `0`     |   O    |   O    |   X    |

## Environment Variables

| Variable               | Description                                   | Default |
|------------------------|-----------------------------------------------|---------|
| `NP_SEMAPHORE_LIMIT`   | Signal channel buffer size                    | 65536   |
| `NP_TCP_DATA_BUF_SIZE` | Buffer size for TCP data transfer             | 16384   |
| `NP_UDP_DATA_BUF_SIZE` | Buffer size for UDP packets                   | 16384   |
| `NP_HANDSHAKE_TIMEOUT` | Timeout for handshake operations              | 5s      |
| `NP_UDP_READ_TIMEOUT`  | Timeout for UDP read operations               | 30s     |
| `NP_TCP_DIAL_TIMEOUT`  | Timeout for establishing TCP connections      | 5s      |
| `NP_UDP_DIAL_TIMEOUT`  | Timeout for establishing UDP connections      | 5s      |
| `NP_POOL_GET_TIMEOUT`  | Timeout for getting connections from pool     | 5s      |
| `NP_MIN_POOL_INTERVAL` | Minimum interval between connection creations | 100ms   |
| `NP_MAX_POOL_INTERVAL` | Maximum interval between connection creations | 1s      |
| `NP_REPORT_INTERVAL`   | Interval for health check reports             | 5s      |
| `NP_SERVICE_COOLDOWN`  | Cooldown period before restart attempts       | 3s      |
| `NP_SHUTDOWN_TIMEOUT`  | Timeout for graceful shutdown                 | 5s      |
| `NP_RELOAD_INTERVAL`   | Interval for cert reload/state backup         | 1h      |

## License

Project **NodePass** is licensed under the [BSD 3-Clause License](LICENSE).
