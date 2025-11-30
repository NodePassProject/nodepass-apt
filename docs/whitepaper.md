# NodePass Technical Whitepaper

**Version 1.0**
**Date: November 2025**

---

## Abstract

NodePass is a high-performance, secure TCP/UDP tunnel proxy system implemented in Go. It provides bidirectional network traffic tunneling with support for TLS 1.3 encryption, connection pooling, rate limiting, and a comprehensive RESTful API for centralized management. This whitepaper presents the technical architecture, design principles, and implementation details of the NodePass system.

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [System Architecture](#2-system-architecture)
3. [Core Components](#3-core-components)
4. [Communication Protocol](#4-communication-protocol)
5. [Security Model](#5-security-model)
6. [Connection Pool Management](#6-connection-pool-management)
7. [Data Flow Mechanisms](#7-data-flow-mechanisms)
8. [Master Control API](#8-master-control-api)
9. [Configuration Parameters](#9-configuration-parameters)
10. [Performance Optimizations](#10-performance-optimizations)
11. [Conclusion](#11-conclusion)

---

## 1. Introduction

### 1.1 Background

Modern distributed systems require secure, efficient, and reliable mechanisms for network traffic tunneling across heterogeneous environments. NodePass addresses these requirements by providing a lightweight yet powerful tunnel proxy solution that supports both TCP and UDP protocols.

### 1.2 Design Goals

NodePass is designed with the following primary objectives:

- **High Performance**: Minimize latency and maximize throughput through efficient buffer management and connection pooling
- **Security**: Provide robust encryption using TLS 1.3 with support for custom certificates
- **Flexibility**: Support multiple operational modes including forward proxy, reverse proxy, and single-node deployment
- **Scalability**: Enable centralized management of multiple tunnel instances through the Master control plane
- **Reliability**: Implement automatic reconnection, graceful shutdown, and error recovery mechanisms

### 1.3 Terminology

| Term | Definition |
|------|------------|
| **Server** | The tunnel endpoint that listens for incoming connections and forwards traffic |
| **Client** | The tunnel endpoint that initiates connections to the server |
| **Master** | The centralized control plane for managing multiple server/client instances |
| **Tunnel** | The secure channel established between server and client |
| **Target** | The destination endpoint where traffic is ultimately forwarded |

---

## 2. System Architecture

### 2.1 High-Level Overview

NodePass implements a three-tier architecture consisting of:

```
┌─────────────────────────────────────────────────────────────────┐
│                        MASTER LAYER                             │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  RESTful API  │  Instance Manager  │  SSE Event Stream  │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                        TUNNEL LAYER                             │
│  ┌─────────────┐                           ┌─────────────┐      │
│  │   SERVER    │◄────── TLS Tunnel ───────►│   CLIENT    │      │
│  │             │                           │             │      │
│  │ - Listener  │                           │ - Connector │      │
│  │ - Pool Mgr  │                           │ - Pool Mgr  │      │
│  │ - Handshake │                           │ - Handshake │      │
│  └─────────────┘                           └─────────────┘      │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      TRANSPORT LAYER                            │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  Connection Pool  │  Rate Limiter  │  Buffer Management   │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 Component Interaction

The system operates through the following interaction model:

1. **Initialization Phase**: Components parse URL-based configurations and initialize internal state
2. **Handshake Phase**: Server and client establish authenticated tunnel connections
3. **Data Transfer Phase**: Bidirectional traffic flows through the established tunnel
4. **Management Phase**: Master provides real-time monitoring and control capabilities

---

## 3. Core Components

### 3.1 Component Overview

```
┌────────────────────────────────────────────────────────────────────────┐
│                           COMMON MODULE                                │
│  ┌───────────────┬───────────────┬───────────────┬───────────────┐     │
│  │ URL Parser    │ DNS Cache     │ Buffer Pools  │ Rate Limiter  │     │
│  ├───────────────┼───────────────┼───────────────┼───────────────┤     │
│  │ TLS Config    │ Signal Codec  │ Slot Manager  │ Target Router │     │
│  └───────────────┴───────────────┴───────────────┴───────────────┘     │
└────────────────────────────────────────────────────────────────────────┘
                    ▲                   ▲                   ▲
                    │ inherits          │ inherits          │ inherits
        ┌───────────┴───────┐   ┌───────┴───────┐   ┌───────┴────────┐
        │      SERVER       │   │     CLIENT    │   │     MASTER     │
        ├───────────────────┤   ├───────────────┤   ├────────────────┤
        │ • Tunnel Listener │   │ • Tunnel Conn │   │ • HTTP Server  │
        │ • Client IP Auth  │   │ • Server Name │   │ • Instance Mgr │
        │ • Handshake Init  │   │ • Handshake   │   │ • SSE Events   │
        │ • Pool Manager    │   │ • Pool Manager│   │ • State Persist│
        └───────────────────┘   └───────────────┘   └────────────────┘
```

### 3.2 Server Operational Modes

```
MODE 0 (Auto)                    MODE 1 (Reverse)                 MODE 2 (Forward)
─────────────                    ────────────────                 ────────────────

┌─────────┐                      ┌─────────┐                      ┌─────────┐
│ Server  │◄── Auto-detect       │ Server  │◄── Target Listener   │ Server  │◄── Tunnel Only
└────┬────┘                      └────┬────┘                      └────┬────┘
     │                                │                                │
     ▼                                ▼                                ▼
┌─────────┐  Yes   ┌─────────┐   ┌─────────┐                      ┌─────────┐
│ Target  │───────►│ Mode 1  │   │ Target  │◄── Accepts           │ Tunnel  │◄── Receives
│Listener?│        │(Reverse)│   │ Listen  │    Connections       │ Listen  │    Connections
└────┬────┘        └─────────┘   └────┬────┘                      └────┬────┘
     │ No                             │                                │
     ▼                                ▼                                ▼
┌─────────┐                      ┌─────────┐                      ┌─────────┐
│ Mode 2  │                      │ Forward │───► Tunnel ───►      │ Forward │───► Target
│(Forward)│                      │ Traffic │    Client            │ Traffic │    Address
└─────────┘                      └─────────┘                      └─────────┘
```

### 3.3 Client Operational Modes

```
MODE 0 (Auto)                    MODE 1 (Single)                  MODE 2 (Dual)
─────────────                    ───────────────                  ────────────────

┌─────────┐                      ┌─────────┐                      ┌─────────┐
│ Client  │◄── Auto-detect       │ Client  │◄── Local Listener    │ Client  │◄── Server Handshake
└────┬────┘                      └────┬────┘                      └────┬────┘
     │                                │                                │
     ▼                                ▼                                ▼
┌─────────┐  Yes   ┌─────────┐   ┌─────────┐                      ┌─────────┐
│ Local   │───────►│ Mode 1  │   │ Listen  │◄── Accept Local      │ Connect │───► Server
│Listener?│        │(Single) │   │ Local   │    Connections       │ Server  │    Tunnel
└────┬────┘        └─────────┘   └────┬────┘                      └────┬────┘
     │ No                             │                                │
     ▼                                ▼                                ▼
┌─────────┐                      ┌─────────┐                      ┌─────────┐
│ Mode 2  │                      │ Forward │───► Target           │ Exchange│◄──► Bidirectional
│ (Dual)  │                      │ Direct  │    (No Server)       │ Config  │    Data Transfer
└─────────┘                      └─────────┘                      └─────────┘
```

### 3.4 Master Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                            MASTER CONTROL PLANE                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                        HTTP SERVER (TLS)                         │   │
│  │  ┌────────────┬────────────┬────────────┬────────────────────┐   │   │
│  │  │ /instances │ /events    │ /info      │ /docs              │   │   │
│  │  │ CRUD API   │ SSE Stream │ System Info│ Swagger UI         │   │   │
│  │  └────────────┴────────────┴────────────┴────────────────────┘   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                              │                                          │
│                              ▼                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                      INSTANCE MANAGER                            │   │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐              │   │
│  │  │Server-1 │  │Server-2 │  │Client-1 │  │Client-N │   ...        │   │
│  │  │ Status  │  │ Status  │  │ Status  │  │ Status  │              │   │
│  │  │ Metrics │  │ Metrics │  │ Metrics │  │ Metrics │              │   │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘              │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                              │                                          │
│                              ▼                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                      STATE PERSISTENCE                           │   │
│  │                     (GOB Serialization)                          │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Communication Protocol

### 4.1 URL-Based Configuration

NodePass employs a URL-based configuration scheme:

```
<scheme>://<password>@<tunnel_host>:<tunnel_port>/<target_addr>?<parameters>
```

**Schemes:**

| Scheme | Description |
|--------|-------------|
| `server` | Server mode tunnel endpoint |
| `client` | Client mode tunnel endpoint |
| `master` | Master control plane |

**Example Configurations:**

```
server://mypassword@0.0.0.0:8080/192.168.1.100:80?tls=1&max=1024
client://mypassword@server.example.com:8080/localhost:8000?min=64&mode=2
master://0.0.0.0:9000/api?tls=2&crt=cert.pem&key=key.pem
```

### 4.2 Handshake Protocol

The tunnel handshake protocol establishes authenticated connections:

```
┌──────────┐                                              ┌──────────┐
│  CLIENT  │                                              │  SERVER  │
└────┬─────┘                                              └────┬─────┘
     │                                                         │
     │     TCP Connect                                         │
     │────────────────────────────────────────────────────────►│
     │                                                         │
     │     Tunnel Key (base64 + XOR obfuscation)               │
     │────────────────────────────────────────────────────────►│
     │                                                         │
     │                                       Validate Key      │
     │                                    ┌────────────────┐   │
     │                                    │ password match?│   │
     │                                    └───────┬────────┘   │
     │                                            │            │
     │     Tunnel Config: np://max/flow#tls       ▼            │
     │◄────────────────────────────────────────────────────────│
     │                                                         │
     │     Parse Config & Update Settings                      │
     ├─────────────────┐                                       │
     │                 │ • Max pool capacity                   │
     │◄────────────────┘ • Data flow direction                 │
     │                   • TLS mode                            │
     │                                                         │
     │     Initialize Connection Pool                          │
     │◄───────────────────────────────────────────────────────►│
     │                                                         │
     │     Session Established ✓                               │
     │═════════════════════════════════════════════════════════│
     │                                                         │
```

### 4.3 Signal Protocol

NodePass uses an internal signal protocol for control messages:

| Signal | Format | Purpose |
|--------|--------|---------|
| Flush | `np:#f` | Reset connection pool |
| Ping | `np:#i` | Keepalive request |
| Pong | `np:#o` | Keepalive response |
| TCP | `np://<id>#t` | TCP connection signal |
| UDP | `np://<id>#u` | UDP packet signal |

---

## 5. Security Model

### 5.1 TLS Configuration

NodePass supports three TLS modes:

```
┌─────────────────────────────────────────────────────────────────┐
│                          TLS MODE SELECTION                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   MODE 0        │  │   MODE 1        │  │   MODE 2        │  │
│  │   (tls=0)       │  │   (tls=1)       │  │   (tls=2)       │  │
│  ├─────────────────┤  ├─────────────────┤  ├─────────────────┤  │
│  │                 │  │                 │  │                 │  │
│  │  ┌───────────┐  │  │  ┌───────────┐  │  │  ┌───────────┐  │  │
│  │  │ Plaintext │  │  │  │ TLS 1.3   │  │  │  │ TLS 1.3   │  │  │
│  │  │   TCP     │  │  │  │ Self-Sign │  │  │  │ Custom CA │  │  │
│  │  └───────────┘  │  │  └───────────┘  │  │  └───────────┘  │  │
│  │                 │  │                 │  │                 │  │
│  │  • No encrypt   │  │  • ECDSA P-256  │  │  • File-based   │  │
│  │  • Fast         │  │  • Auto-gen     │  │  • Hot-reload   │  │
│  │  • Internal use │  │  • 1-year valid │  │  • Verify chain │  │
│  │                 │  │  • In-memory    │  │  • Production   │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
│         ▲                    ▲                    ▲             │
│         │                    │                    │             │
│    Development          Testing/Dev          Production         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

                    Certificate Hot-Reload Mechanism
                    ────────────────────────────────
                              
┌──────────┐    Check      ┌──────────┐    Reload     ┌──────────┐
│ Request  │──────────────►│ Expired? │──────────────►│ Load New │
│ Arrives  │   Interval    │ (1 hour) │    Yes        │ From Disk│
└──────────┘               └────┬─────┘               └────┬─────┘
                                │ No                       │
                                ▼                          ▼
                           ┌──────────┐               ┌──────────┐
                           │Use Cached│               │ Update   │
                           │   Cert   │               │  Cache   │
                           └──────────┘               └──────────┘
```

### 5.2 Authentication

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    TUNNEL KEY AUTHENTICATION                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Key Derivation:                                                        │
│  ┌─────────────────┐         ┌─────────────────┐                        │
│  │ URL Password    │    OR   │ Port-based Hash │                        │
│  │ user:password@  │         │ FNV-32a(port)   │                        │
│  └────────┬────────┘         └────────┬────────┘                        │
│           │                           │                                 │
│           └───────────┬───────────────┘                                 │
│                       ▼                                                 │
│              ┌─────────────────┐                                        │
│              │   Tunnel Key    │                                        │
│              └────────┬────────┘                                        │
│                       │                                                 │
│  Transmission:        ▼                                                 │
│              ┌─────────────────┐     ┌─────────────────┐                │
│              │   XOR with Key  │────►│ Base64 Encode   │                │
│              └─────────────────┘     └────────┬────────┘                │
│                                               │                         │
│                                               ▼                         │
│                                      ┌─────────────────┐                │
│                                      │ Send to Server  │                │
│                                      └─────────────────┘                │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                    MASTER API KEY AUTHENTICATION                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────┐      ┌─────────────┐      ┌─────────────┐              │
│  │   Request   │─────►│ X-API-Key   │─────►│  Validate   │              │
│  │   Arrives   │      │   Header?   │      │   Match?    │              │
│  └─────────────┘      └──────┬──────┘      └──────┬──────┘              │
│                              │ No                 │                     │
│                              ▼                    │                     │
│                       ┌─────────────┐             │                     │
│                       │    401      │             │                     │
│                       │Unauthorized │             │ Yes                 │
│                       └─────────────┘             ▼                     │
│                                            ┌─────────────┐              │
│  API Key: 32-char hex                      │   Process   │              │
│  Storage: GOB state file                   │   Request   │              │
│  Scope: Protected endpoints                └─────────────┘              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 5.3 Certificate Management

The `pkg/cert` package provides certificate generation:

```go
func NewTLSConfig(name string) (*tls.Config, error) {
    // Generate ECDSA P-256 private key
    private, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    
    // Create self-signed certificate (1-year validity)
    template := x509.Certificate{
        SerialNumber: serialNumber,
        NotBefore:    time.Now(),
        NotAfter:     time.Now().AddDate(1, 0, 0),
        KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
        ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
    }
    
    // Return configured TLS config
    return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}
```

---

## 6. Connection Pool Management

### 6.1 Pool Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                       CONNECTION POOL STRUCTURE                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                        sync.Map: conns                          │    │
│  │  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐     │    │
│  │  │ID: a1b2│  │ID: c3d4│  │ID: e5f6│  │ID: g7h8│  │  ...   │     │    │
│  │  │ *Conn  │  │ *Conn  │  │ *Conn  │  │ *Conn  │  │        │     │    │
│  │  └────────┘  └────────┘  └────────┘  └────────┘  └────────┘     │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▲                                          │
│                              │ Store/Load                               │
│                              │                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    chan string: idChan                          │    │
│  │  ┌──────┬──────┬──────┬──────┬──────┬──────┬──────┬──────┐      │    │
│  │  │ a1b2 │ c3d4 │ e5f6 │ g7h8 │      │      │      │ ...  │      │    │
│  │  └──────┴──────┴──────┴──────┴──────┴──────┴──────┴──────┘      │    │
│  │            Available Connection IDs (FIFO Queue)                │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
│  ┌───────────────────┐  ┌───────────────────┐  ┌───────────────────┐    │
│  │ capacity: int32   │  │ interval: int64   │  │ errCount: int32   │    │
│  │ (atomic)          │  │ (atomic)          │  │ (atomic)          │    │
│  │                   │  │                   │  │                   │    │
│  │ Current pool size │  │ Creation interval │  │ Error counter     │    │
│  │ min ≤ cap ≤ max   │  │ 100ms → 1s        │  │ Auto-recovery     │    │
│  └───────────────────┘  └───────────────────┘  └───────────────────┘    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 6.2 Client Pool Management

**Adaptive Capacity & Interval Algorithms:**

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      ADAPTIVE CAPACITY SCALING                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Utilization = Active Connections / Current Capacity                    │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                                                                 │    │
│  │   0%          20%                    80%              100%      │    │
│  │   ├───────────┼─────────────────────┼────────────────┤          │    │
│  │   │  SCALE    │      MAINTAIN       │    SCALE       │          │    │
│  │   │   DOWN    │       STABLE        │     UP         │          │    │
│  │   │  ÷2       │                     │    ×2          │          │    │
│  │   │           │                     │                │          │    │
│  │   ▼           │                     │                ▼          │    │
│  │  min_cap      │                     │             max_cap       │    │
│  │  (floor)      │                     │             (ceiling)     │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────────────┐
│                      ADAPTIVE INTERVAL ADJUSTMENT                      │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│                   ┌──────────────────────┐                             │
│                   │  Check Pool Status   │                             │
│                   └──────────┬───────────┘                             │
│                              │                                         │
│            ┌─────────────────┼─────────────────┐                       │
│            │                 │                 │                       │
│            ▼                 ▼                 ▼                       │
│     ┌────────────┐    ┌────────────┐    ┌────────────┐                 │
│     │  Need More │    │   Stable   │    │  Excess    │                 │
│     │Connections │    │            │    │Connections │                 │
│     └─────┬──────┘    └─────┬──────┘    └─────┬──────┘                 │
│           │                 │                 │                        │
│           ▼                 ▼                 ▼                        │
│     ┌────────────┐    ┌────────────┐    ┌────────────┐                 │
│     │  DECREASE  │    │   KEEP     │    │  INCREASE  │                 │
│     │  Interval  │    │  Current   │    │  Interval  │                 │
│     │  -100ms    │    │            │    │  +100ms    │                 │
│     └────────────┘    └────────────┘    └────────────┘                 │
│                                                                        │
│     min: 100ms ◄─────────────────────────────────► max: 1000ms         │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

### 6.3 Server Pool Management

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    SERVER CONNECTION ACCEPTANCE FLOW                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────┐                                                        │
│  │  Incoming   │                                                        │
│  │ Connection  │                                                        │
│  └──────┬──────┘                                                        │
│         │                                                               │
│         ▼                                                               │
│  ┌─────────────┐    No     ┌─────────────┐                              │
│  │ Pool Full?  │──────────►│  Continue   │                              │
│  │ (≥ maxCap)  │           │             │                              │
│  └──────┬──────┘           └──────┬──────┘                              │
│         │ Yes                     │                                     │
│         ▼                         ▼                                     │
│  ┌─────────────┐           ┌─────────────┐    Mismatch   ┌──────────┐   │
│  │   Reject    │           │ Validate IP │──────────────►│  Reject  │   │
│  │ Connection  │           │ (if set)    │               │          │   │
│  └─────────────┘           └──────┬──────┘               └──────────┘   │
│                                   │ Match                               │
│                                   ▼                                     │
│                            ┌─────────────┐                              │
│                            │ TLS Config? │                              │
│                            └──────┬──────┘                              │
│                      Yes ┌────────┴────────┐ No                         │
│                          ▼                 ▼                            │
│                   ┌─────────────┐   ┌─────────────┐                     │
│                   │ TLS Server  │   │  Raw TCP    │                     │
│                   │  Handshake  │   │ Connection  │                     │
│                   └──────┬──────┘   └──────┬──────┘                     │
│                          │                 │                            │
│                          └────────┬────────┘                            │
│                                   ▼                                     │
│                            ┌─────────────┐                              │
│                            │ Generate ID │                              │
│                            │ (4-byte hex)│                              │
│                            └──────┬──────┘                              │
│                                   │                                     │
│                    ┌──────────────┼──────────────┐                      │
│                    ▼              ▼              ▼                      │
│             ┌───────────┐  ┌───────────┐  ┌───────────┐                 │
│             │ Send ID   │  │ Store in  │  │ Push ID   │                 │
│             │ to Client │  │  conns    │  │ to idChan │                 │
│             └───────────┘  └───────────┘  └───────────┘                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 6.4 Connection Lifecycle

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Creation   │────►│    Active    │────►│   Cleanup    │
└──────────────┘     └──────────────┘     └──────────────┘
       │                    │                    │
       │                    │                    │
       ▼                    ▼                    ▼
  • TCP dial           • Data transfer      • Close conn
  • TLS handshake      • Keepalive          • Remove from pool
  • ID assignment      • Error handling     • Decrement counter
```

---

## 7. Data Flow Mechanisms

### 7.1 TCP Data Transfer

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    BIDIRECTIONAL TCP EXCHANGE                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│       Connection A                              Connection B            │
│  ┌─────────────────┐                      ┌─────────────────┐           │
│  │                 │                      │                 │           │
│  │   ┌─────────┐   │   Goroutine 1 (TX)   │   ┌─────────┐   │           │
│  │   │  Read   │───┼──────────────────────┼──►│  Write  │   │           │
│  │   │         │   │    copy + rate limit │   │         │   │           │
│  │   └─────────┘   │                      │   └─────────┘   │           │
│  │                 │                      │                 │           │
│  │   ┌─────────┐   │   Goroutine 2 (RX)   │   ┌─────────┐   │           │
│  │   │  Write  │◄──┼──────────────────────┼───│  Read   │   │           │
│  │   │         │   │    copy + rate limit │   │         │   │           │
│  │   └─────────┘   │                      │   └─────────┘   │           │
│  │                 │                      │                 │           │
│  └─────────────────┘                      └─────────────────┘           │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                        Buffer Pool                              │    │
│  │   ┌──────────┐    ┌──────────┐    sync.Pool ensures efficient   │    │
│  │   │  aBuf    │    │  bBuf    │    buffer reuse across all       │    │
│  │   │ 16KB TCP │    │ 16KB TCP │    concurrent connections        │    │
│  │   └──────────┘    └──────────┘                                  │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
│  Return: (rx bytes received, tx bytes transmitted)                      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 7.2 UDP Data Transfer

**Session Management:**

- Sessions tracked by source address
- Configurable read timeout (default: 30s)
- Automatic session cleanup on timeout

**Packet Format:**

```
┌────────────────────────────────────────┐
│ Signal: np://<session_id>#u            │
├────────────────────────────────────────┤
│ Payload: <udp_data>                    │
└────────────────────────────────────────┘
```

### 7.3 Rate Limiting

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      TOKEN BUCKET RATE LIMITER                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                      Token Buckets                              │    │
│  │                                                                 │    │
│  │   READ BUCKET                        WRITE BUCKET               │    │
│  │  ┌──────────────┐                  ┌──────────────┐             │    │
│  │  │ ████████░░░░ │                  │ ██████░░░░░░ │             │    │
│  │  │  readTokens  │                  │ writeTokens  │             │    │
│  │  │              │                  │              │             │    │
│  │  │ Capacity:    │                  │ Capacity:    │             │    │
│  │  │ readRate/sec │                  │ writeRate/sec│             │    │
│  │  └──────────────┘                  └──────────────┘             │    │
│  │        ▲                                  ▲                     │    │
│  │        │ Refill                           │ Refill              │    │
│  │        │ (time-based)                     │ (time-based)        │    │
│  └────────┼──────────────────────────────────┼─────────────────────┘    │
│           │                                  │                          │
│  ┌────────┴──────────────────────────────────┴────────┐                 │
│  │                  REFILL MECHANISM                  │                 │
│  │                                                    │                 │
│  │  tokens += rate × (now - lastUpdate) / second      │                 │
│  │  tokens = min(tokens, rate)  // cap at max         │                 │
│  │                                                    │                 │
│  └────────────────────────────────────────────────────┘                 │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                    WAIT FOR TOKENS FLOW                          │   │
│  │                                                                  │   │
│  │  Request N bytes ──► Enough tokens? ──► Yes ──► Consume & Return │   │
│  │                            │                                     │   │
│  │                            │ No                                  │   │
│  │                            ▼                                     │   │
│  │                      Wait (sync.Cond)                            │   │
│  │                            │                                     │   │
│  │                            │ Broadcast                           │   │
│  │                            ▼                                     │   │
│  │                      Retry check ─────────────────────────────── │   │
│  │                                                                  │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 8. Master Control API

### 8.1 API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/instances` | List all instances |
| `POST` | `/api/v1/instances` | Create new instance |
| `GET` | `/api/v1/instances/{id}` | Get instance details |
| `PUT` | `/api/v1/instances/{id}` | Update instance |
| `DELETE` | `/api/v1/instances/{id}` | Delete instance |
| `PATCH` | `/api/v1/instances/{id}` | Control instance (start/stop/restart) |
| `GET` | `/api/v1/events` | SSE event stream |
| `GET` | `/api/v1/info` | System information |
| `GET` | `/api/v1/tcping` | TCP connectivity test |

### 8.2 Instance Data Model

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          INSTANCE STRUCTURE                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌───────────────────────────┐   ┌───────────────────────────┐          │
│  │      IDENTIFICATION       │   │       CONFIGURATION       │          │
│  │  • ID: string             │   │  • URL: string            │          │
│  │  • Alias: string          │   │  • Config: string         │          │
│  │  • Type: server/client    │   │  • Restart: bool          │          │
│  │  • Status: running/       │   │  • Mode: int32            │          │
│  │           stopped/error   │   │                           │          │
│  └───────────────────────────┘   └───────────────────────────┘          │
│                                                                         │
│  ┌───────────────────────────┐   ┌───────────────────────────┐          │
│  │       CONNECTIONS         │   │        TRAFFIC            │          │
│  │  • Ping: int32 (ms)       │   │  • TCPRX: uint64 (bytes)  │          │
│  │  • Pool: int32            │   │  • TCPTX: uint64 (bytes)  │          │
│  │  • TCPS: int32 (sessions) │   │  • UDPRX: uint64 (bytes)  │          │
│  │  • UDPS: int32 (sessions) │   │  • UDPTX: uint64 (bytes)  │          │
│  └───────────────────────────┘   └───────────────────────────┘          │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────┐          │
│  │                         METADATA                          │          │
│  │  Meta.Peer: { SID, Type, Alias }                          │          │
│  │  Meta.Tags: map[string]string (custom key-value pairs)    │          │
│  └───────────────────────────────────────────────────────────┘          │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 8.3 Server-Sent Events

**Event Types:**

| Type | Description |
|------|-------------|
| `initial` | Complete instance list on connection |
| `create` | New instance created |
| `update` | Instance state changed |
| `delete` | Instance removed |
| `shutdown` | Master shutting down |
| `log` | Instance log message |

**Event Format:**

```
event: <type>
data: {"type":"<type>","time":"<timestamp>","instance":{...}}
retry: 3000

```

### 8.4 State Persistence

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      GOB STATE PERSISTENCE                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│                           SAVE STATE FLOW                               │
│  ┌───────────────┐    ┌───────────────┐    ┌───────────────┐            │
│  │   Acquire     │    │   Collect     │    │    Encode     │            │
│  │    Mutex      │───►│  Instances    │───►│    to GOB     │            │
│  │  (stateMu)    │    │  (sync.Map)   │    │    Format     │            │
│  └───────────────┘    └───────────────┘    └───────┬───────┘            │
│                                                    │                    │
│                                                    ▼                    │
│                                            ┌───────────────┐            │
│                                            │  Write File   │            │
│                                            │ nodepass.gob  │            │
│                                            └───────────────┘            │
│                                                                         │
│                           LOAD STATE FLOW                               │
│  ┌───────────────┐    ┌───────────────┐    ┌───────────────┐            │
│  │   Read File   │    │    Decode     │    │   Restore     │            │
│  │ nodepass.gob  │───►│   from GOB    │───►│   to Map      │            │
│  └───────────────┘    └───────────────┘    └───────────────┘            │
│                                                                         │
│  File Location: <executable_dir>/gob/nodepass.gob                       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 9. Configuration Parameters

### 9.1 URL Query Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `tls` | `0` | TLS mode (0=none, 1=RAM, 2=custom) |
| `crt` | - | Certificate file path |
| `key` | - | Private key file path |
| `log` | `info` | Log level (none/debug/info/warn/error/event) |
| `dns` | `5m` | DNS cache TTL |
| `min` | `64` | Minimum pool capacity |
| `max` | `1024` | Maximum pool capacity |
| `mode` | `0` | Run mode (0=auto, 1/2=explicit) |
| `dial` | `auto` | Local IP for dialing |
| `read` | `0` | Read timeout |
| `rate` | `0` | Rate limit (Mbps) |
| `slot` | `65536` | Max concurrent connections |
| `proxy` | `0` | PROXY protocol version |
| `notcp` | `0` | Disable TCP forwarding |
| `noudp` | `0` | Disable UDP forwarding |

### 9.2 Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NP_SEMAPHORE_LIMIT` | `65536` | Signal channel buffer size |
| `NP_TCP_DATA_BUF_SIZE` | `16384` | TCP buffer size |
| `NP_UDP_DATA_BUF_SIZE` | `16384` | UDP buffer size |
| `NP_HANDSHAKE_TIMEOUT` | `5s` | Handshake timeout |
| `NP_TCP_DIAL_TIMEOUT` | `5s` | TCP dial timeout |
| `NP_UDP_DIAL_TIMEOUT` | `5s` | UDP dial timeout |
| `NP_UDP_READ_TIMEOUT` | `30s` | UDP read timeout |
| `NP_POOL_GET_TIMEOUT` | `5s` | Pool connection timeout |
| `NP_MIN_POOL_INTERVAL` | `100ms` | Min pool creation interval |
| `NP_MAX_POOL_INTERVAL` | `1s` | Max pool creation interval |
| `NP_REPORT_INTERVAL` | `5s` | Statistics report interval |
| `NP_SERVICE_COOLDOWN` | `3s` | Restart cooldown period |
| `NP_SHUTDOWN_TIMEOUT` | `5s` | Graceful shutdown timeout |
| `NP_RELOAD_INTERVAL` | `1h` | Certificate reload interval |

---

## 10. Performance Optimizations

### 10.1 Buffer Pool Management

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        BUFFER POOL (sync.Pool)                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                       TCP Buffer Pool                           │    │
│  │   ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐        │    │
│  │   │ 16 KB  │ │ 16 KB  │ │ 16 KB  │ │ 16 KB  │ │  ...   │        │    │
│  │   └────────┘ └────────┘ └────────┘ └────────┘ └────────┘        │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                       UDP Buffer Pool                           │    │
│  │   ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐        │    │
│  │   │ 16 KB  │ │ 16 KB  │ │ 16 KB  │ │ 16 KB  │ │  ...   │        │    │
│  │   └────────┘ └────────┘ └────────┘ └────────┘ └────────┘        │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
│                            LIFECYCLE                                    │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                                                                  │   │
│  │   getBuffer()              Data Transfer              putBuffer()│   │
│  │  ┌──────────┐             ┌──────────┐              ┌──────────┐ │   │
│  │  │  Pool    │────────────►│   Use    │─────────────►│  Pool    │ │   │
│  │  │   Get    │   Buffer    │  Buffer  │   Return     │   Put    │ │   │
│  │  └────┬─────┘             └──────────┘              └────┬─────┘ │   │
│  │       │                                                  │       │   │
│  │       │ Empty?                              Capacity OK? │       │   │
│  │       ▼                                                  ▼       │   │
│  │  ┌──────────┐                                      ┌──────────┐  │   │
│  │  │ Allocate │◄── New 16KB buffer                   │  Reuse   │  │   │
│  │  │   New    │                                      │ (No GC)  │  │   │
│  │  └──────────┘                                      └──────────┘  │   │
│  │                                                                  │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Benefits: • Zero-allocation hot path  • Reduced GC pressure            │
│            • Consistent buffer sizes   • Thread-safe operations         │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 10.2 DNS Caching

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          DNS CACHE SYSTEM                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    sync.Map: dnsCacheEntries                    │    │
│  │                                                                 │    │
│  │   Key: "host:port"          Value: dnsCacheEntry                │    │
│  │  ┌────────────────────┬─────────────────────────────────────┐   │    │
│  │  │ "api.example.com:  │ tcpAddr: 192.168.1.100:443          │   │    │
│  │  │  443"              │ udpAddr: 192.168.1.100:443          │   │    │
│  │  │                    │ expiredAt: 2025-11-30 12:05:00      │   │    │
│  │  ├────────────────────┼─────────────────────────────────────┤   │    │
│  │  │ "db.internal:5432" │ tcpAddr: 10.0.0.50:5432             │   │    │
│  │  │                    │ udpAddr: 10.0.0.50:5432             │   │    │
│  │  │                    │ expiredAt: 2025-11-30 12:10:00      │   │    │
│  │  └────────────────────┴─────────────────────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
│                          RESOLUTION FLOW                                │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                                                                 │    │
│  │  resolve("host:port")                                           │    │
│  │         │                                                       │    │
│  │         ▼                                                       │    │
│  │  ┌─────────────┐  Hit   ┌─────────────┐  Valid  ┌───────────┐   │    │
│  │  │ Cache       │───────►│   Check     │────────►│  Return   │   │    │
│  │  │ Lookup      │        │  Expiry     │         │  Cached   │   │    │
│  │  └─────────────┘        └──────┬──────┘         └───────────┘   │    │
│  │         │ Miss                 │ Expired                        │    │
│  │         │                      │                                │    │
│  │         ▼                      ▼                                │    │
│  │  ┌─────────────┐        ┌─────────────┐                         │    │
│  │  │  System     │        │   Delete    │                         │    │
│  │  │ DNS Resolve │◄───────│   Entry     │                         │    │
│  │  └──────┬──────┘        └─────────────┘                         │    │
│  │         │                                                       │    │
│  │         ▼                                                       │    │
│  │  ┌─────────────┐                                                │    │
│  │  │ Store with  │  TTL: 5 minutes (default)                      │    │
│  │  │ new expiry  │  Configurable via ?dns= parameter              │    │
│  │  └─────────────┘                                                │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 10.3 Connection Slot Management

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    ATOMIC SLOT TRACKING SYSTEM                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                      SLOT ALLOCATION                            │    │
│  │                                                                 │    │
│  │   Total Slots: slotLimit (default: 65536)                       │    │
│  │  ┌─────────────────────────────────────────────────────────┐    │    │
│  │  │░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  │    │    │
│  │  │◄────── tcpSlot ──────►│◄────── udpSlot ──────►│ free    │    │    │
│  │  └─────────────────────────────────────────────────────────┘    │    │
│  │                                                                 │    │
│  │   TCP Connections        UDP Sessions           Available       │    │
│  │   (atomic int32)         (atomic int32)         Capacity        │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
│                         ACQUIRE / RELEASE FLOW                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                                                                 │    │
│  │   tryAcquireSlot(isUDP)                    releaseSlot(isUDP)   │    │
│  │  ┌───────────────────┐                   ┌───────────────────┐  │    │
│  │  │ slotLimit == 0?   │──Yes──► Allow     │ slotLimit == 0?   │  │    │
│  │  └────────┬──────────┘                   └────────┬──────────┘  │    │
│  │           │ No                                    │ No          │    │
│  │           ▼                                       ▼             │    │
│  │  ┌───────────────────┐                   ┌───────────────────┐  │    │
│  │  │ tcp + udp ≥ limit?│──Yes──► Reject    │    Decrement      │  │    │
│  │  └────────┬──────────┘                   │  (atomic -1)      │  │    │
│  │           │ No                           └───────────────────┘  │    │
│  │           ▼                                                     │    │
│  │  ┌───────────────────┐                                          │    │
│  │  │   Increment       │                                          │    │
│  │  │  (atomic +1)      │                                          │    │
│  │  │  isUDP? udpSlot   │                                          │    │
│  │  │  else   tcpSlot   │                                          │    │
│  │  └───────────────────┘                                          │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
│  Benefits: • Lock-free atomic operations  • Fair TCP/UDP distribution   │
│            • Prevents resource exhaustion • Configurable limits         │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 10.4 Load Balancing

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    ROUND-ROBIN WITH FAILOVER                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Target Addresses: [Target-A, Target-B, Target-C, Target-D]             │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    ROUND-ROBIN SELECTION                        │    │
│  │                                                                 │    │
│  │   Request 1    Request 2    Request 3    Request 4              │    │
│  │       │            │            │            │                  │    │
│  │       ▼            ▼            ▼            ▼                  │    │
│  │   ┌───────┐    ┌───────┐    ┌───────┐    ┌───────┐              │    │
│  │   │   A   │    │   B   │    │   C   │    │   D   │              │    │
│  │   └───────┘    └───────┘    └───────┘    └───────┘              │    │
│  │       │                                      │                  │    │
│  │       │         ◄──── Atomic Index ────►     │                  │    │
│  │       ▼                (wraps around)        ▼                  │    │
│  │   Request 5                              Request 6              │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    FAILOVER MECHANISM                           │    │
│  │                                                                 │    │
│  │   Start Index: 2 (Target-C)                                     │    │
│  │                                                                 │    │
│  │   ┌───────┐    ┌───────┐    ┌───────┐    ┌───────┐              │    │
│  │   │   A   │    │   B   │    │   C   │    │   D   │              │    │
│  │   │ idx=0 │    │ idx=1 │    │ idx=2 │    │ idx=3 │              │    │
│  │   └───────┘    └───────┘    └───┬───┘    └───────┘              │    │
│  │       ▲            ▲            │            ▲                  │    │
│  │       │            │            ▼            │                  │    │
│  │       │            │       Try C ──► FAIL    │                  │    │
│  │       │            │            │            │                  │    │
│  │       │            │            ▼            │                  │    │
│  │       │            │       Try D ──► FAIL ───┘                  │    │
│  │       │            │                 │                          │    │
│  │       │            │                 ▼                          │    │
│  │       │            └──────── Try A ──► FAIL                     │    │
│  │       │                           │                             │    │
│  │       │                           ▼                             │    │
│  │       └───────────────────  Try B ──► SUCCESS ✓                 │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
│  Features: • Atomic index increment    • Automatic failover             │
│            • All targets tried         • Preserves round-robin order    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 11. Conclusion

NodePass provides a robust, high-performance solution for TCP/UDP tunnel proxying with comprehensive management capabilities. Its design emphasizes:

- **Security**: TLS 1.3 encryption with flexible certificate management
- **Performance**: Efficient buffer pooling, connection pooling, and rate limiting
- **Operability**: RESTful API, SSE event streaming, and persistent state management
- **Flexibility**: Multiple operational modes supporting diverse deployment scenarios

The URL-based configuration model simplifies deployment while the Master control plane enables centralized management of complex tunnel topologies.

---

## Appendix A: Package Structure

```
nodepass/
├── cmd/nodepass/
│   ├── main.go          # Entry point
│   └── core.go          # Core initialization
├── internal/
│   ├── common.go        # Shared functionality
│   ├── server.go        # Server mode implementation
│   ├── client.go        # Client mode implementation
│   └── master.go        # Master control plane
├── pkg/
│   ├── cert/cert.go     # TLS certificate generation
│   ├── conn/conn.go     # Connection utilities
│   ├── logs/logs.go     # Logging system
│   └── pool/pool.go     # Connection pool management
└── docs/
    └── whitepaper.md    # This whitepaper
```

## Appendix B: Signal Flow Diagram

```
┌─────────┐                                    ┌─────────┐
│ CLIENT  │                                    │ SERVER  │
└────┬────┘                                    └────┬────┘
     │                                              │
     │  1. TCP Connect                              │
     │─────────────────────────────────────────────►│
     │                                              │
     │  2. Send Tunnel Key (base64/XOR)             │
     │─────────────────────────────────────────────►│
     │                                              │
     │  3. Tunnel Config (np://max/flow#tls)        │
     │◄─────────────────────────────────────────────│
     │                                              │
     │  4. Pool Connections (TLS handshake)         │
     │◄────────────────────────────────────────────►│
     │                                              │
     │  5. TCP Signal (np://id#t)                   │
     │◄─────────────────────────────────────────────│
     │                                              │
     │  6. Data Exchange                            │
     │◄────────────────────────────────────────────►│
     │                                              │
     │  7. Ping/Pong (np:#i / np:#o)                │
     │◄────────────────────────────────────────────►│
     │                                              │
```

---

**Document Revision History**

| Version | Date | Description |
|---------|------|-------------|
| 1.0 | November 2025 | Initial release |

---

*© 2025 NodePass Project. This document is provided for technical reference purposes.*
