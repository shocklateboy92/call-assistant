# Call Assistant - Architecture Specification

## 🎯 **Project Overview**

Call Assistant is a multi-protocol video calling system that allows using networked cameras (e.g., Reolink) with any casting target (e.g., Chromecast, F-Cast, Miracast) through various calling protocols (starting with Matrix). The system is designed as a Home Assistant add-on using a monolithic container architecture.

## 🏗️ **Core Architecture**

### **System Design Philosophy**

The architecture follows a graph-based approach where:
- **Video Sources** (cameras) are nodes
- **Video Sinks** (casting targets) are nodes  
- **Calling Protocols** (Matrix, XMPP) are nodes
- **Converters** (go2rtc, FFmpeg) are edges connecting nodes

### **Container Architecture Overview**

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           Call Assistant Add-on                                 │
│                                                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐ │
│  │              │  │              │  │              │  │                      │ │
│  │    go2rtc    │  │      Go      │  │    nginx     │  │    Protocol SDKs     │ │
│  │   (Media)    │  │ Orchestrator │  │ (Web Server) │  │   (Node.js apps)     │ │
│  │              │  │              │  │              │  │                      │ │
│  │ :1984 :8554  │  │    :8080     │  │  :80 :443    │  │ matrix-sdk, xmpp-js  │ │
│  │ :8555        │  │              │  │              │  │                      │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────────────┘ │
│                                          │                                      │
│  ┌─────────────────────────────────────────────────────────────────────────────┐ │
│  │                            supervisord                                      │ │
│  │            Process Manager & Health Monitor                                  │ │
│  └─────────────────────────────────────────────────────────────────────────────┘ │
│                                          │                                      │
│  ┌─────────────────────────────────────────────────────────────────────────────┐ │
│  │                        Shared Filesystem                                    │ │
│  │  /data/config/     - Configuration files                                   │ │
│  │  /data/streams/    - Active stream definitions                             │ │
│  │  /data/calls/      - Call state and history                                │ │
│  │  /data/logs/       - Application logs                                      │ │
│  └─────────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────────┘
```

## 🐳 **Multi-Stage Dockerfile**

```dockerfile
# Multi-stage build for Call Assistant Home Assistant Add-on
ARG BUILD_FROM=ghcr.io/home-assistant/amd64-base:latest
ARG BUILD_ARCH=amd64

#################################################################
# Stage 1: Build Go Orchestrator
#################################################################
FROM golang:1.21-alpine AS go-builder

WORKDIR /workspace

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy Go modules first for better caching
COPY go-orchestrator/go.mod go-orchestrator/go.sum ./
RUN go mod download

# Copy source code
COPY go-orchestrator/ ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${BUILD_ARCH} \
    go build -ldflags="-w -s" -o call-assistant ./cmd/main.go

#################################################################
# Stage 2: Build Web Interface
#################################################################
FROM node:20-alpine AS node-builder

WORKDIR /workspace

# Copy package files first for better caching
COPY web-interface/package*.json ./
RUN npm ci --only=production

# Copy source and build
COPY web-interface/ ./
RUN npm run build

#################################################################
# Stage 3: Prepare Protocol SDKs (Node.js runtime apps)
#################################################################
FROM node:20-alpine AS sdk-builder

WORKDIR /workspace

# Matrix SDK app
COPY protocol-sdks/matrix/package*.json ./matrix/
WORKDIR /workspace/matrix
RUN npm ci --only=production

WORKDIR /workspace
COPY protocol-sdks/matrix/ ./matrix/

# XMPP SDK app  
COPY protocol-sdks/xmpp/package*.json ./xmpp/
WORKDIR /workspace/xmpp
RUN npm ci --only=production

WORKDIR /workspace
COPY protocol-sdks/xmpp/ ./xmpp/

#################################################################
# Stage 4: Final Runtime Image
#################################################################
FROM ${BUILD_FROM}

# Set shell
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Install runtime dependencies
RUN apk add --no-cache \
    supervisor \
    nginx \
    nodejs \
    npm \
    curl \
    wget \
    jq \
    ca-certificates \
    tzdata

# Install go2rtc
ARG GO2RTC_VERSION=1.8.4
RUN case "${BUILD_ARCH}" in \
    amd64) GO2RTC_ARCH="amd64" ;; \
    aarch64) GO2RTC_ARCH="arm64" ;; \
    armhf) GO2RTC_ARCH="arm" ;; \
    armv7) GO2RTC_ARCH="arm" ;; \
    i386) GO2RTC_ARCH="386" ;; \
    *) echo "Unsupported architecture: ${BUILD_ARCH}" && exit 1 ;; \
    esac && \
    wget -O /usr/local/bin/go2rtc \
    "https://github.com/AlexxIT/go2rtc/releases/download/v${GO2RTC_VERSION}/go2rtc_linux_${GO2RTC_ARCH}" && \
    chmod +x /usr/local/bin/go2rtc

# Copy built applications
COPY --from=go-builder /workspace/call-assistant /usr/local/bin/
COPY --from=node-builder /workspace/dist /var/www/html/
COPY --from=sdk-builder /workspace/matrix /opt/call-assistant/matrix/
COPY --from=sdk-builder /workspace/xmpp /opt/call-assistant/xmpp/

# Copy configuration files
COPY rootfs/ /

# Create directories
RUN mkdir -p \
    /data/config \
    /data/streams \
    /data/calls \
    /data/logs \
    /var/log/supervisor \
    /run/nginx

# Set permissions
RUN chmod +x /usr/local/bin/call-assistant
RUN chown -R root:root /opt/call-assistant

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:80/health || exit 1

# Expose ports
EXPOSE 80 443 1984 8554 8555

# Labels for Home Assistant
LABEL \
    io.hass.name="Call Assistant" \
    io.hass.description="Multi-protocol video calling for cameras" \
    io.hass.arch="${BUILD_ARCH}" \
    io.hass.type="addon" \
    io.hass.version="1.0.0"

# Start supervisord
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
```

## ⚙️ **Supervisord Configuration**

```ini
# /etc/supervisor/conf.d/supervisord.conf
[supervisord]
nodaemon=true
user=root
logfile=/var/log/supervisor/supervisord.log
pidfile=/var/run/supervisord.pid
childlogdir=/var/log/supervisor
loglevel=info

[unix_http_server]
file=/var/run/supervisor.sock
chmod=0700

[supervisorctl]
serverurl=unix:///var/run/supervisor.sock

[rpcinterface:supervisor]
supervisor.rpcinterface_factory = supervisor.rpcinterface:make_main_rpcinterface

#################################################################
# Core Services
#################################################################

[program:go2rtc]
command=/usr/local/bin/go2rtc -config /data/config/go2rtc.yaml
directory=/data
stdout_logfile=/data/logs/go2rtc.log
stderr_logfile=/data/logs/go2rtc.error.log
autorestart=true
startretries=3
priority=100
user=root

[program:call-assistant]
command=/usr/local/bin/call-assistant -config /data/config/call-assistant.yaml
directory=/data
stdout_logfile=/data/logs/call-assistant.log
stderr_logfile=/data/logs/call-assistant.error.log
autorestart=true
startretries=3
priority=200
user=root
depends_on=go2rtc

[program:nginx]
command=/usr/sbin/nginx -g "daemon off;"
stdout_logfile=/data/logs/nginx.log
stderr_logfile=/data/logs/nginx.error.log
autorestart=true
startretries=3
priority=300
user=root

#################################################################
# Protocol SDK Services
#################################################################

[program:matrix-sdk]
command=/usr/bin/node index.js
directory=/opt/call-assistant/matrix
environment=NODE_ENV=production,CONFIG_PATH=/data/config/matrix.json
stdout_logfile=/data/logs/matrix-sdk.log
stderr_logfile=/data/logs/matrix-sdk.error.log
autorestart=true
startretries=3
priority=400
user=root
depends_on=call-assistant

[program:xmpp-sdk]
command=/usr/bin/node index.js
directory=/opt/call-assistant/xmpp
environment=NODE_ENV=production,CONFIG_PATH=/data/config/xmpp.json
stdout_logfile=/data/logs/xmpp-sdk.log
stderr_logfile=/data/logs/xmpp-sdk.error.log
autorestart=true
startretries=3
priority=500
user=root
depends_on=call-assistant

#################################################################
# Health Monitoring
#################################################################

[program:health-monitor]
command=/usr/local/bin/call-assistant -mode health-monitor
directory=/data
stdout_logfile=/data/logs/health-monitor.log
stderr_logfile=/data/logs/health-monitor.error.log
autorestart=true
startretries=3
priority=600
user=root
depends_on=call-assistant

#################################################################
# Event Handlers
#################################################################

[eventlistener:crash-handler]
command=/opt/call-assistant/scripts/crash-handler.sh
events=PROCESS_STATE_FATAL,PROCESS_STATE_EXITED
stdout_logfile=/data/logs/crash-handler.log
stderr_logfile=/data/logs/crash-handler.error.log
```

## 🎛️ **Go Orchestrator Design**

### **Main Entry Point**

```go
// cmd/main.go - Main entry point
package main

import (
    "context"
    "flag"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/call-assistant/internal/config"
    "github.com/call-assistant/internal/orchestrator"
    "github.com/call-assistant/internal/health"
)

func main() {
    var (
        configPath = flag.String("config", "/data/config/call-assistant.yaml", "Configuration file path")
        mode      = flag.String("mode", "orchestrator", "Run mode: orchestrator|health-monitor")
    )
    flag.Parse()

    cfg, err := config.Load(*configPath)
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle shutdown signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    switch *mode {
    case "orchestrator":
        orch := orchestrator.New(cfg)
        go func() {
            if err := orch.Start(ctx); err != nil {
                log.Fatalf("Orchestrator failed: %v", err)
            }
        }()

    case "health-monitor":
        monitor := health.NewMonitor(cfg)
        go func() {
            if err := monitor.Start(ctx); err != nil {
                log.Fatalf("Health monitor failed: %v", err)
            }
        }()

    default:
        log.Fatalf("Unknown mode: %s", *mode)
    }

    <-sigChan
    log.Println("Shutting down gracefully...")
    cancel()
}
```

### **Core Orchestration Logic**

```go
// internal/orchestrator/orchestrator.go - Core orchestration logic
package orchestrator

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/call-assistant/internal/config"
    "github.com/call-assistant/internal/go2rtc"
    "github.com/call-assistant/internal/calls"
    "github.com/call-assistant/internal/devices"
    "github.com/call-assistant/internal/protocols"
)

type Orchestrator struct {
    config    *config.Config
    go2rtc    *go2rtc.Client
    calls     *calls.Manager
    devices   *devices.Manager
    protocols *protocols.Manager
    server    *http.Server
}

func New(cfg *config.Config) *Orchestrator {
    return &Orchestrator{
        config:    cfg,
        go2rtc:    go2rtc.NewClient("http://localhost:1984"),
        calls:     calls.NewManager(),
        devices:   devices.NewManager(),
        protocols: protocols.NewManager(),
    }
}

func (o *Orchestrator) Start(ctx context.Context) error {
    // Initialize subsystems
    if err := o.initialize(); err != nil {
        return fmt.Errorf("initialization failed: %w", err)
    }

    // Setup HTTP API
    router := o.setupRoutes()
    o.server = &http.Server{
        Addr:    ":8080",
        Handler: router,
    }

    // Start background tasks
    go o.deviceDiscovery(ctx)
    go o.healthCheck(ctx)
    go o.streamCleanup(ctx)

    // Start HTTP server
    return o.server.ListenAndServe()
}

func (o *Orchestrator) setupRoutes() *gin.Engine {
    r := gin.New()
    r.Use(gin.Logger(), gin.Recovery())

    // Health endpoint
    r.GET("/health", o.healthHandler)

    // API v1
    v1 := r.Group("/api/v1")
    {
        // Streams
        v1.GET("/streams", o.listStreams)
        v1.POST("/streams", o.createStream)
        v1.DELETE("/streams/:id", o.deleteStream)

        // Calls
        v1.GET("/calls", o.listCalls)
        v1.POST("/calls", o.startCall)
        v1.DELETE("/calls/:id", o.endCall)

        // Devices
        v1.GET("/devices", o.listDevices)
        v1.POST("/devices/discover", o.discoverDevices)

        // Configuration
        v1.GET("/config", o.getConfig)
        v1.PUT("/config", o.updateConfig)
    }

    return r
}
```

### **Call Management**

```go
// internal/calls/manager.go - Call management
package calls

import (
    "context"
    "fmt"
    "sync"
    "time"
)

type CallState string

const (
    CallStateIdle     CallState = "idle"
    CallStateRinging  CallState = "ringing"
    CallStateActive   CallState = "active"
    CallStateEnding   CallState = "ending"
)

type Call struct {
    ID           string                 `json:"id"`
    Protocol     string                 `json:"protocol"`     // "matrix", "xmpp", "sip"
    State        CallState              `json:"state"`
    Participants []Participant          `json:"participants"`
    Streams      []Stream              `json:"streams"`
    Metadata     map[string]interface{} `json:"metadata"`
    StartedAt    time.Time             `json:"started_at"`
    EndedAt      *time.Time            `json:"ended_at,omitempty"`
}

type Participant struct {
    ID       string `json:"id"`
    Protocol string `json:"protocol"`
    Address  string `json:"address"`  // Matrix user ID, XMPP JID, SIP URI
    Role     string `json:"role"`     // "caller", "callee", "camera"
}

type Stream struct {
    ID        string `json:"id"`
    SourceURL string `json:"source_url"`  // Camera RTSP URL
    TargetURL string `json:"target_url"`  // WebRTC URL for go2rtc
    Quality   string `json:"quality"`     // "low", "medium", "high"
    Active    bool   `json:"active"`
}

type Manager struct {
    calls map[string]*Call
    mutex sync.RWMutex
}

func NewManager() *Manager {
    return &Manager{
        calls: make(map[string]*Call),
    }
}

func (m *Manager) StartCall(req StartCallRequest) (*Call, error) {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    call := &Call{
        ID:           generateCallID(),
        Protocol:     req.Protocol,
        State:        CallStateRinging,
        Participants: req.Participants,
        Streams:      make([]Stream, 0),
        Metadata:     req.Metadata,
        StartedAt:    time.Now(),
    }

    m.calls[call.ID] = call

    // Setup streams for camera participants
    for _, participant := range call.Participants {
        if participant.Role == "camera" {
            stream, err := m.setupCameraStream(participant.Address)
            if err != nil {
                return nil, fmt.Errorf("failed to setup camera stream: %w", err)
            }
            call.Streams = append(call.Streams, *stream)
        }
    }

    call.State = CallStateActive
    return call, nil
}

func (m *Manager) setupCameraStream(cameraURL string) (*Stream, error) {
    // This would integrate with go2rtc to setup the stream
    streamID := generateStreamID()
    
    // Call go2rtc API to add stream
    targetURL := fmt.Sprintf("http://localhost:1984/api/stream/%s/channel/0/webrtc", streamID)
    
    return &Stream{
        ID:        streamID,
        SourceURL: cameraURL,
        TargetURL: targetURL,
        Quality:   "medium",
        Active:    true,
    }, nil
}
```

## 🏠 **Home Assistant Add-on Configuration**

```yaml
# config.yaml - Home Assistant Add-on Configuration Schema
name: "Call Assistant"
description: "Multi-protocol video calling for networked cameras"
version: "1.0.0"
slug: "call_assistant"
url: "https://github.com/your-repo/call-assistant"
init: false
arch:
  - aarch64
  - amd64
  - armhf
  - armv7
  - i386

# Port mappings
ports:
  "80/tcp": 8123      # Web interface (redirected by nginx)
  "1984/tcp": 1984    # go2rtc web interface  
  "8554/tcp": 8554    # RTSP server
  "8555/tcp": 8555    # WebRTC connections

# Port descriptions for UI
ports_description:
  "80/tcp": "Web interface"
  "1984/tcp": "go2rtc management interface"
  "8554/tcp": "RTSP server for cameras"
  "8555/tcp": "WebRTC connections"

# Add-on options/configuration
options:
  # Matrix configuration
  matrix:
    enabled: true
    homeserver: "https://matrix.org"
    username: ""
    password: ""
    device_name: "Call Assistant"
    
  # XMPP configuration  
  xmpp:
    enabled: false
    server: ""
    username: ""
    password: ""
    resource: "call-assistant"
    
  # Camera discovery
  cameras:
    auto_discover: true
    onvif_scan: true
    scan_networks:
      - "192.168.1.0/24"
    manual_cameras: []
    
  # Casting targets
  casting:
    chromecast_discovery: true
    airplay_discovery: true
    dlna_discovery: true
    manual_targets: []
    
  # Quality settings
  video:
    default_quality: "medium"
    adaptive_quality: true
    max_bitrate_kbps: 2000
    
  # Security
  security:
    web_username: "admin"
    web_password: ""
    ssl_enabled: false
    ssl_cert_path: ""
    ssl_key_path: ""
    
  # Logging
  logging:
    level: "info"  # debug, info, warn, error
    max_log_files: 10
    max_log_size_mb: 100

# Schema for option validation
schema:
  matrix:
    enabled: "bool"
    homeserver: "url"
    username: "str?"
    password: "password?"
    device_name: "str"
    
  xmpp:
    enabled: "bool"
    server: "str?"
    username: "str?"
    password: "password?"
    resource: "str"
    
  cameras:
    auto_discover: "bool"
    onvif_scan: "bool"
    scan_networks:
      - "str"
    manual_cameras:
      - name: "str"
        url: "str"
        username: "str?"
        password: "password?"
        
  casting:
    chromecast_discovery: "bool"
    airplay_discovery: "bool"
    dlna_discovery: "bool"
    manual_targets:
      - name: "str"
        type: "list(chromecast|airplay|dlna|miracast)"
        address: "str"
        
  video:
    default_quality: "list(low|medium|high|ultra)"
    adaptive_quality: "bool"
    max_bitrate_kbps: "int(100,10000)"
    
  security:
    web_username: "str"
    web_password: "password?"
    ssl_enabled: "bool"
    ssl_cert_path: "str?"
    ssl_key_path: "str?"
    
  logging:
    level: "list(debug|info|warn|error)"
    max_log_files: "int(1,100)"
    max_log_size_mb: "int(1,1000)"

# Host network access for device discovery
host_network: true

# Privileged access for hardware acceleration
privileged:
  - SYS_ADMIN

# Device access for USB cameras
devices:
  - "/dev/video0:/dev/video0:rwm"
  - "/dev/video1:/dev/video1:rwm"

# File system access
map:
  - "config:rw"
  - "ssl:rw"
  - "media:rw"

# Environment variables
environment:
  TZ: "UTC"

# Startup services this add-on depends on  
depends_on:
  - "core_mosquitto"  # Optional MQTT broker

# Add-on startup type
boot: "auto"
startup: "application"

# Resource limits
limits:
  memory: "1G"
  cpu: "2"

# Backup exclusions
backup_exclude:
  - "logs/*"
  - "streams/*"
  - "tmp/*"

# Health check configuration
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:80/health"]
  interval: "30s"
  timeout: "10s"
  retries: 3
  start_period: "60s"
```

## 📝 **Configuration File Template**

```yaml
# /data/config/call-assistant.yaml - Runtime configuration
# Generated from Home Assistant add-on options

server:
  host: "0.0.0.0"
  port: 8080
  cors_origins: 
    - "http://localhost"
    - "https://localhost"

logging:
  level: "{{ .Options.logging.level }}"
  file: "/data/logs/call-assistant.log"
  max_size_mb: {{ .Options.logging.max_log_size_mb }}
  max_files: {{ .Options.logging.max_log_files }}

matrix:
  enabled: {{ .Options.matrix.enabled }}
  {{- if .Options.matrix.enabled }}
  homeserver: "{{ .Options.matrix.homeserver }}"
  username: "{{ .Options.matrix.username }}"
  password: "{{ .Options.matrix.password }}"
  device_name: "{{ .Options.matrix.device_name }}"
  sdk_endpoint: "http://localhost:3001"
  {{- end }}

xmpp:
  enabled: {{ .Options.xmpp.enabled }}
  {{- if .Options.xmpp.enabled }}
  server: "{{ .Options.xmpp.server }}"
  username: "{{ .Options.xmpp.username }}"
  password: "{{ .Options.xmpp.password }}"
  resource: "{{ .Options.xmpp.resource }}"
  sdk_endpoint: "http://localhost:3002"
  {{- end }}

cameras:
  auto_discover: {{ .Options.cameras.auto_discover }}
  onvif_scan: {{ .Options.cameras.onvif_scan }}
  scan_networks:
    {{- range .Options.cameras.scan_networks }}
    - "{{ . }}"
    {{- end }}
  manual_cameras:
    {{- range .Options.cameras.manual_cameras }}
    - name: "{{ .name }}"
      url: "{{ .url }}"
      username: "{{ .username }}"
      password: "{{ .password }}"
    {{- end }}

casting:
  chromecast_discovery: {{ .Options.casting.chromecast_discovery }}
  airplay_discovery: {{ .Options.casting.airplay_discovery }}
  dlna_discovery: {{ .Options.casting.dlna_discovery }}
  manual_targets:
    {{- range .Options.casting.manual_targets }}
    - name: "{{ .name }}"
      type: "{{ .type }}"
      address: "{{ .address }}"
    {{- end }}

video:
  default_quality: "{{ .Options.video.default_quality }}"
  adaptive_quality: {{ .Options.video.adaptive_quality }}
  max_bitrate_kbps: {{ .Options.video.max_bitrate_kbps }}

go2rtc:
  endpoint: "http://localhost:1984"
  config_file: "/data/config/go2rtc.yaml"

storage:
  config_dir: "/data/config"
  streams_dir: "/data/streams"  
  calls_dir: "/data/calls"
  logs_dir: "/data/logs"
```

## 🖥️ **Web Interface Design**

### **Main Dashboard HTML**

```html
<!-- /var/www/html/index.html - Main Dashboard -->
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Call Assistant</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css" rel="stylesheet">
    <link href="https://cdn.jsdelivr.net/npm/bootstrap-icons@1.11.0/font/bootstrap-icons.css" rel="stylesheet">
    <link href="/css/style.css" rel="stylesheet">
</head>
<body>
    <nav class="navbar navbar-expand-lg navbar-dark bg-primary">
        <div class="container-fluid">
            <a class="navbar-brand" href="#">
                <i class="bi bi-camera-video-fill"></i> Call Assistant
            </a>
            <div class="navbar-nav ms-auto">
                <div class="nav-item">
                    <span class="navbar-text" id="connection-status">
                        <i class="bi bi-circle-fill text-success"></i> Connected
                    </span>
                </div>
            </div>
        </div>
    </nav>

    <div class="container-fluid mt-4">
        <div class="row">
            <!-- Sidebar -->
            <div class="col-md-3 col-lg-2">
                <div class="list-group">
                    <a href="#dashboard" class="list-group-item list-group-item-action active" data-tab="dashboard">
                        <i class="bi bi-house-fill"></i> Dashboard
                    </a>
                    <a href="#cameras" class="list-group-item list-group-item-action" data-tab="cameras">
                        <i class="bi bi-camera-fill"></i> Cameras
                    </a>
                    <a href="#calls" class="list-group-item list-group-item-action" data-tab="calls">
                        <i class="bi bi-telephone-fill"></i> Calls
                    </a>
                    <a href="#casting" class="list-group-item list-group-item-action" data-tab="casting">
                        <i class="bi bi-cast"></i> Casting
                    </a>
                    <a href="#protocols" class="list-group-item list-group-item-action" data-tab="protocols">
                        <i class="bi bi-globe"></i> Protocols
                    </a>
                    <a href="#settings" class="list-group-item list-group-item-action" data-tab="settings">
                        <i class="bi bi-gear-fill"></i> Settings
                    </a>
                    <a href="#logs" class="list-group-item list-group-item-action" data-tab="logs">
                        <i class="bi bi-file-text-fill"></i> Logs
                    </a>
                </div>
            </div>

            <!-- Main Content -->
            <div class="col-md-9 col-lg-10">
                <!-- Dashboard Tab -->
                <div id="dashboard-content" class="tab-content active">
                    <div class="row mb-4">
                        <div class="col-md-3">
                            <div class="card bg-primary text-white">
                                <div class="card-body">
                                    <div class="d-flex justify-content-between">
                                        <div>
                                            <h5 class="card-title">Active Calls</h5>
                                            <h2 id="active-calls-count">0</h2>
                                        </div>
                                        <div class="align-self-center">
                                            <i class="bi bi-telephone-fill fs-1"></i>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="col-md-3">
                            <div class="card bg-success text-white">
                                <div class="card-body">
                                    <div class="d-flex justify-content-between">
                                        <div>
                                            <h5 class="card-title">Cameras</h5>
                                            <h2 id="cameras-count">0</h2>
                                        </div>
                                        <div class="align-self-center">
                                            <i class="bi bi-camera-fill fs-1"></i>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="col-md-3">
                            <div class="card bg-info text-white">
                                <div class="card-body">
                                    <div class="d-flex justify-content-between">
                                        <div>
                                            <h5 class="card-title">Streams</h5>
                                            <h2 id="streams-count">0</h2>
                                        </div>
                                        <div class="align-self-center">
                                            <i class="bi bi-broadcast fs-1"></i>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="col-md-3">
                            <div class="card bg-warning text-white">
                                <div class="card-body">
                                    <div class="d-flex justify-content-between">
                                        <div>
                                            <h5 class="card-title">Cast Targets</h5>
                                            <h2 id="targets-count">0</h2>
                                        </div>
                                        <div class="align-self-center">
                                            <i class="bi bi-cast fs-1"></i>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>

                    <!-- Quick Actions -->
                    <div class="row mb-4">
                        <div class="col-12">
                            <div class="card">
                                <div class="card-header">
                                    <h5 class="mb-0">Quick Actions</h5>
                                </div>
                                <div class="card-body">
                                    <button class="btn btn-primary me-2" onclick="startQuickCall()">
                                        <i class="bi bi-camera-video"></i> Start Camera Call
                                    </button>
                                    <button class="btn btn-success me-2" onclick="discoverDevices()">
                                        <i class="bi bi-search"></i> Discover Devices
                                    </button>
                                    <button class="btn btn-info me-2" onclick="viewGo2RTC()">
                                        <i class="bi bi-gear"></i> go2rtc Interface
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>

                    <!-- Recent Activity -->
                    <div class="row">
                        <div class="col-12">
                            <div class="card">
                                <div class="card-header">
                                    <h5 class="mb-0">Recent Activity</h5>
                                </div>
                                <div class="card-body">
                                    <div id="recent-activity" class="list-group list-group-flush">
                                        <!-- Activity items will be populated via JavaScript -->
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Cameras Tab -->
                <div id="cameras-content" class="tab-content">
                    <div class="d-flex justify-content-between align-items-center mb-4">
                        <h3>Camera Management</h3>
                        <div>
                            <button class="btn btn-success" onclick="discoverCameras()">
                                <i class="bi bi-search"></i> Discover
                            </button>
                            <button class="btn btn-primary" onclick="addCamera()">
                                <i class="bi bi-plus"></i> Add Camera
                            </button>
                        </div>
                    </div>
                    
                    <div class="row" id="cameras-grid">
                        <!-- Camera cards will be populated via JavaScript -->
                    </div>
                </div>

                <!-- Calls Tab -->
                <div id="calls-content" class="tab-content">
                    <div class="d-flex justify-content-between align-items-center mb-4">
                        <h3>Active Calls</h3>
                        <button class="btn btn-primary" onclick="startNewCall()">
                            <i class="bi bi-plus"></i> Start New Call
                        </button>
                    </div>
                    
                    <div id="calls-list">
                        <!-- Call items will be populated via JavaScript -->
                    </div>
                </div>

                <!-- Settings Tab -->
                <div id="settings-content" class="tab-content">
                    <h3>Settings</h3>
                    <form id="settings-form">
                        <div class="row">
                            <div class="col-md-6">
                                <div class="card">
                                    <div class="card-header">
                                        <h6 class="mb-0">Matrix Configuration</h6>
                                    </div>
                                    <div class="card-body">
                                        <div class="mb-3">
                                            <label class="form-label">Homeserver URL</label>
                                            <input type="url" class="form-control" id="matrix-homeserver">
                                        </div>
                                        <div class="mb-3">
                                            <label class="form-label">Username</label>
                                            <input type="text" class="form-control" id="matrix-username">
                                        </div>
                                        <div class="mb-3">
                                            <label class="form-label">Password</label>
                                            <input type="password" class="form-control" id="matrix-password">
                                        </div>
                                    </div>
                                </div>
                            </div>
                            
                            <div class="col-md-6">
                                <div class="card">
                                    <div class="card-header">
                                        <h6 class="mb-0">Video Quality</h6>
                                    </div>
                                    <div class="card-body">
                                        <div class="mb-3">
                                            <label class="form-label">Default Quality</label>
                                            <select class="form-select" id="default-quality">
                                                <option value="low">Low (480p)</option>
                                                <option value="medium">Medium (720p)</option>
                                                <option value="high">High (1080p)</option>
                                                <option value="ultra">Ultra (4K)</option>
                                            </select>
                                        </div>
                                        <div class="mb-3">
                                            <div class="form-check">
                                                <input class="form-check-input" type="checkbox" id="adaptive-quality">
                                                <label class="form-check-label">
                                                    Adaptive Quality
                                                </label>
                                            </div>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                        
                        <div class="mt-4">
                            <button type="submit" class="btn btn-primary">Save Settings</button>
                        </div>
                    </form>
                </div>
            </div>
        </div>
    </div>

    <!-- Modals -->
    <div class="modal fade" id="cameraModal" tabindex="-1">
        <div class="modal-dialog">
            <div class="modal-content">
                <div class="modal-header">
                    <h5 class="modal-title">Add Camera</h5>
                    <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
                </div>
                <div class="modal-body">
                    <form id="camera-form">
                        <div class="mb-3">
                            <label class="form-label">Camera Name</label>
                            <input type="text" class="form-control" id="camera-name" required>
                        </div>
                        <div class="mb-3">
                            <label class="form-label">RTSP URL</label>
                            <input type="url" class="form-control" id="camera-url" required>
                        </div>
                        <div class="mb-3">
                            <label class="form-label">Username</label>
                            <input type="text" class="form-control" id="camera-username">
                        </div>
                        <div class="mb-3">
                            <label class="form-label">Password</label>
                            <input type="password" class="form-control" id="camera-password">
                        </div>
                    </form>
                </div>
                <div class="modal-footer">
                    <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
                    <button type="button" class="btn btn-primary" onclick="saveCamera()">Save</button>
                </div>
            </div>
        </div>
    </div>

    <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/js/bootstrap.bundle.min.js"></script>
    <script src="/js/app.js"></script>
</body>
</html>
```

### **Main Application JavaScript**

```javascript
// /var/www/html/js/app.js - Main application logic
class CallAssistantApp {
    constructor() {
        this.apiBase = '/api/v1';
        this.ws = null;
        this.currentTab = 'dashboard';
        this.init();
    }

    async init() {
        this.setupEventHandlers();
        this.connectWebSocket();
        await this.loadDashboard();
        this.startPeriodicUpdates();
    }

    setupEventHandlers() {
        // Tab navigation
        document.querySelectorAll('[data-tab]').forEach(tab => {
            tab.addEventListener('click', (e) => {
                e.preventDefault();
                this.switchTab(e.target.dataset.tab);
            });
        });

        // Settings form
        document.getElementById('settings-form').addEventListener('submit', (e) => {
            e.preventDefault();
            this.saveSettings();
        });
    }

    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;
        
        this.ws = new WebSocket(wsUrl);
        
        this.ws.onopen = () => {
            console.log('WebSocket connected');
            this.updateConnectionStatus(true);
        };
        
        this.ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            this.handleWebSocketMessage(data);
        };
        
        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            this.updateConnectionStatus(false);
            // Reconnect after 5 seconds
            setTimeout(() => this.connectWebSocket(), 5000);
        };
    }

    handleWebSocketMessage(data) {
        switch (data.type) {
            case 'camera_discovered':
                this.addCameraToGrid(data.camera);
                break;
            case 'call_started':
                this.updateCallsList();
                this.updateDashboardCounts();
                break;
            case 'call_ended':
                this.updateCallsList();
                this.updateDashboardCounts();
                break;
            case 'stream_status':
                this.updateStreamStatus(data.stream);
                break;
        }
    }

    async loadDashboard() {
        try {
            const [calls, cameras, streams, targets] = await Promise.all([
                this.fetchAPI('/calls'),
                this.fetchAPI('/cameras'),
                this.fetchAPI('/streams'),
                this.fetchAPI('/casting/targets')
            ]);

            this.updateDashboardCounts(calls.length, cameras.length, streams.length, targets.length);
            this.updateRecentActivity();
        } catch (error) {
            console.error('Failed to load dashboard:', error);
        }
    }

    updateDashboardCounts(calls = 0, cameras = 0, streams = 0, targets = 0) {
        document.getElementById('active-calls-count').textContent = calls;
        document.getElementById('cameras-count').textContent = cameras;
        document.getElementById('streams-count').textContent = streams;
        document.getElementById('targets-count').textContent = targets;
    }

    async startQuickCall() {
        try {
            // Show camera selection modal
            const cameras = await this.fetchAPI('/cameras');
            if (cameras.length === 0) {
                alert('No cameras available. Please add cameras first.');
                return;
            }

            // For demo, use first camera
            const camera = cameras[0];
            const callRequest = {
                protocol: 'matrix', // Default to Matrix
                participants: [
                    { role: 'camera', address: camera.url }
                ],
                metadata: { camera_name: camera.name }
            };

            const call = await this.fetchAPI('/calls', 'POST', callRequest);
            alert(`Call started: ${call.id}`);
            this.updateCallsList();
        } catch (error) {
            console.error('Failed to start call:', error);
            alert('Failed to start call');
        }
    }

    async discoverDevices() {
        try {
            await this.fetchAPI('/devices/discover', 'POST');
            alert('Device discovery started. New devices will appear automatically.');
        } catch (error) {
            console.error('Failed to start discovery:', error);
        }
    }

    switchTab(tabName) {
        // Hide all tabs
        document.querySelectorAll('.tab-content').forEach(tab => {
            tab.classList.remove('active');
        });

        // Show selected tab
        document.getElementById(`${tabName}-content`).classList.add('active');

        // Update navigation
        document.querySelectorAll('[data-tab]').forEach(tab => {
            tab.classList.remove('active');
        });
        document.querySelector(`[data-tab="${tabName}"]`).classList.add('active');

        this.currentTab = tabName;

        // Load tab-specific data
        switch (tabName) {
            case 'cameras':
                this.loadCameras();
                break;
            case 'calls':
                this.loadCalls();
                break;
            case 'settings':
                this.loadSettings();
                break;
        }
    }

    async fetchAPI(endpoint, method = 'GET', data = null) {
        const options = {
            method,
            headers: {
                'Content-Type': 'application/json',
            },
        };

        if (data) {
            options.body = JSON.stringify(data);
        }

        const response = await fetch(`${this.apiBase}${endpoint}`, options);
        
        if (!response.ok) {
            throw new Error(`API request failed: ${response.statusText}`);
        }

        return response.json();
    }

    updateConnectionStatus(connected) {
        const statusElement = document.getElementById('connection-status');
        if (connected) {
            statusElement.innerHTML = '<i class="bi bi-circle-fill text-success"></i> Connected';
        } else {
            statusElement.innerHTML = '<i class="bi bi-circle-fill text-danger"></i> Disconnected';
        }
    }

    startPeriodicUpdates() {
        // Update dashboard every 30 seconds
        setInterval(() => {
            if (this.currentTab === 'dashboard') {
                this.loadDashboard();
            }
        }, 30000);
    }
}

// Initialize app when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    window.app = new CallAssistantApp();
});
```

## 📁 **Project Structure**

```
call-assistant/
├── Dockerfile                          # Multi-stage build
├── config.yaml                         # HA add-on configuration
├── CHANGELOG.md
├── README.md
├── 
├── rootfs/                             # Files copied to container
│   ├── etc/
│   │   ├── supervisor/
│   │   │   └── conf.d/
│   │   │       └── supervisord.conf    # Process management
│   │   └── nginx/
│   │       └── nginx.conf              # Web server config
│   ├── opt/
│   │   └── call-assistant/
│   │       └── scripts/
│   │           └── crash-handler.sh    # Error recovery
│   └── usr/
│       └── local/
│           └── bin/
│               └── entrypoint.sh       # Startup script
│
├── go-orchestrator/                    # Go service
│   ├── cmd/
│   │   └── main.go                     # Entry point
│   ├── internal/
│   │   ├── config/                     # Configuration management
│   │   ├── orchestrator/               # Core orchestration
│   │   ├── calls/                      # Call management
│   │   ├── devices/                    # Device discovery
│   │   ├── protocols/                  # Protocol abstraction
│   │   ├── go2rtc/                     # go2rtc client
│   │   └── health/                     # Health monitoring
│   ├── go.mod
│   └── go.sum
│
├── protocol-sdks/                      # Node.js services
│   ├── matrix/                         # Matrix protocol handler
│   │   ├── package.json
│   │   ├── index.js
│   │   └── lib/
│   └── xmpp/                          # XMPP protocol handler
│       ├── package.json
│       ├── index.js
│       └── lib/
│
├── web-interface/                      # Frontend
│   ├── package.json
│   ├── src/
│   │   ├── index.html
│   │   ├── css/
│   │   │   └── style.css
│   │   └── js/
│   │       └── app.js
│   └── dist/                          # Built files
│
└── docs/                              # Documentation
    ├── API.md
    ├── DEVELOPMENT.md
    └── USER_GUIDE.md
```

## 🚀 **Key Benefits of This Architecture**

### **1. Single Container Deployment**
- ✅ **Home Assistant compatible**: Meets HA add-on requirements
- ✅ **Simple installation**: One-click install from add-on store
- ✅ **Resource efficient**: Shared memory and processes

### **2. Process Isolation**
- ✅ **Fault tolerance**: supervisord manages process crashes
- ✅ **Independent scaling**: Each process can be optimized separately
- ✅ **Clean separation**: Each component has defined responsibilities

### **3. Development Friendly**
- ✅ **Multi-language support**: Go for performance, Node.js for protocols
- ✅ **Hot reload**: Development mode with file watching
- ✅ **Comprehensive logging**: Centralized log management

### **4. Production Ready**
- ✅ **Health monitoring**: Built-in health checks and recovery
- ✅ **Configuration management**: Template-based config generation
- ✅ **Security**: Process isolation and secure defaults

### **5. Extensible Design**
- ✅ **Plugin architecture**: Easy to add new protocols
- ✅ **API-driven**: RESTful APIs for integration
- ✅ **WebSocket updates**: Real-time UI updates

## 🎯 **Implementation Phases**

### **Phase 1: MVP Core**
1. **Go orchestrator** with basic RTSP → WebRTC via go2rtc
2. **Simple Matrix integration** for signaling
3. **Basic Chromecast casting**
4. **go2rtc integration** for protocol bridging

### **Phase 2: Protocol Expansion**
1. **XMPP/Jingle support**
2. **Additional casting protocols** (F-Cast, Miracast)
3. **ONVIF camera discovery**
4. **Quality adaptation**

### **Phase 3: Advanced Features**
1. **Multi-party calling** (SFU/MCU)
2. **End-to-end encryption**
3. **Home Assistant integration**
4. **Mobile apps** (React Native/Flutter)

### **Phase 4: Polish & Optimization**
1. **Performance optimization**
2. **Advanced UI features**
3. **Comprehensive testing**
4. **Documentation completion**

## 📊 **Technical Decisions Summary**

| Component | Technology | Justification |
|-----------|------------|---------------|
| **Core Orchestrator** | Go | Excellent I/O performance, gRPC ecosystem, concurrent processing |
| **Media Processing** | go2rtc | Proven RTSP→WebRTC conversion, active maintenance |
| **Protocol SDKs** | Node.js | Rich ecosystem for Matrix/XMPP, rapid development |
| **Web Interface** | Vanilla JS + Bootstrap | Simple, fast, no build complexity |
| **Container** | Alpine Linux | Small size, security, package availability |
| **Process Management** | supervisord | Reliable process supervision, crash recovery |
| **Configuration** | YAML | Human-readable, HA standard |

This architecture provides a solid foundation that's both Home Assistant compatible and professionally architected for future growth, balancing simplicity with extensibility.