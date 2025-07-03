# Call Assistant: Pipeline/Graph-Based Architecture

## Overview

The Call Assistant uses a **dynamic graph-based architecture** where media flows through interconnected nodes. Each node represents a distinct capability (video source, protocol handler, converter, or sink), and edges represent data flow between them.

## Core Graph Entities

### 1. Video Sources (Input Nodes)

**Purpose**: Capture or provide video/audio streams

```yaml
sources:
  reolink_front_door:
    type: "rtsp"
    url: "rtsp://admin:password@192.168.1.100/Preview_01_main"
    capabilities: ["h264", "aac", "1080p"]

  usb_webcam:
    type: "usb"
    device: "/dev/video0"
    capabilities: ["mjpeg", "pcm", "720p"]

  hls_stream:
    type: "hls"
    url: "https://example.com/live/stream.m3u8"
    capabilities: ["h264", "aac", "variable_bitrate"]
```

### 2. Video Sinks (Output Nodes)

**Purpose**: Display or consume video/audio streams

```yaml
sinks:
  living_room_chromecast:
    type: "chromecast"
    ip: "192.168.1.50"
    capabilities: ["h264", "vp8", "aac"]

  bedroom_miracast:
    type: "miracast"
    discovery: "mdns"
    capabilities: ["h264", "pcm"]

  airplay_apple_tv:
    type: "airplay"
    ip: "192.168.1.75"
    capabilities: ["h264", "hevc", "aac"]
```

### 3. Calling Protocols (Bidirectional Nodes)

**Purpose**: Handle signaling and media exchange with remote participants

```yaml
protocols:
  matrix_home:
    type: "matrix"
    homeserver: "https://matrix.example.com"
    user_id: "@callassistant:example.com"
    capabilities: ["webrtc", "h264", "vp8", "opus"]

  xmpp_work:
    type: "xmpp"
    server: "xmpp.company.com"
    jid: "callassistant@company.com"
    capabilities: ["jingle", "h264", "opus"]

  direct_webrtc:
    type: "webrtc"
    stun_servers: ["stun:stun.l.google.com:19302"]
    capabilities: ["h264", "vp8", "vp9", "opus"]
```

### 4. Converters (Processing Nodes)

**Purpose**: Transform media between formats, protocols, or qualities

```yaml
converters:
  go2rtc_main:
    type: "go2rtc"
    instance: "http://localhost:1984"
    capabilities:
      input: ["rtsp", "rtmp", "usb", "hls"]
      output: ["webrtc", "hls", "mjpeg", "mp4"]

  ffmpeg_transcoder:
    type: "ffmpeg"
    capabilities:
      codecs: ["h264", "h265", "vp8", "vp9", "av1"]
      audio: ["aac", "opus", "mp3", "pcm"]
      formats: ["mp4", "webm", "hls", "dash"]

  quality_adapter:
    type: "adaptive"
    profiles: ["1080p", "720p", "480p", "360p"]
    bitrates: ["4000k", "2000k", "1000k", "500k"]
```

## Graph Construction and Management

### Dynamic Graph Building

```go
type MediaGraph struct {
    nodes map[string]Node
    edges map[string][]Edge
    flows map[string]*ActiveFlow
}

type Node interface {
    GetCapabilities() []Capability
    GetNodeType() NodeType
    IsCompatible(other Node) bool
    Start() error
    Stop() error
}

type Edge struct {
    From     string
    To       string
    MediaType string // "video", "audio", "data"
    Protocol  string // "rtsp", "webrtc", "http"
    Quality   QualityProfile
}
```

### Example Graph Construction

```go
func (g *MediaGraph) CreateCall(request CallRequest) (*ActiveFlow, error) {
    // 1. Find source node
    source := g.nodes[request.SourceID]

    // 2. Find target protocol/sink
    target := g.nodes[request.TargetID]

    // 3. Calculate optimal path
    path, err := g.findPath(source, target)
    if err != nil {
        return nil, err
    }

    // 4. Setup converters along the path
    flow := &ActiveFlow{
        ID: generateFlowID(),
        Path: path,
        Status: "initializing",
    }

    // 5. Configure each node in the path
    for i, nodeID := range path {
        node := g.nodes[nodeID]
        if i > 0 {
            // Setup connection from previous node
            prevNode := g.nodes[path[i-1]]
            err := g.connectNodes(prevNode, node, flow)
            if err != nil {
                return nil, err
            }
        }
    }

    return flow, nil
}
```

## Real-World Pipeline Examples

### Example 1: Camera to Matrix Call

```
RTSP Camera → go2rtc Converter → WebRTC Protocol → Matrix Participant

[H.264/AAC] → [Protocol Bridge] → [WebRTC Stream] → [Remote User]
```

**Configuration:**

```yaml
flows:
  camera_to_matrix:
    source: "reolink_front_door"
    target: "matrix_home"
    room: "!abc123:example.com"
    converters:
      - id: "go2rtc_main"
        config:
          input_stream: "rtsp://192.168.1.100/Preview_01_main"
          output_format: "webrtc"
          quality: "720p"
```

### Example 2: Multi-Protocol Bridge

```
USB Camera → go2rtc → Quality Adapter → [Matrix Protocol, XMPP Protocol, Chromecast Sink]
                                      ↓             ↓                ↓
                                 Matrix Room  XMPP Conference  Living Room TV
```

**Dynamic Configuration:**

```go
func (ca *CallAssistant) BridgeToMultipleTargets(sourceID string, targets []string) error {
    source := ca.graph.GetNode(sourceID)

    // Create fan-out through quality adapter
    adapter := &QualityAdapter{
        InputFormat: source.GetOutputFormat(),
        OutputProfiles: make(map[string]QualityProfile),
    }

    for _, targetID := range targets {
        target := ca.graph.GetNode(targetID)

        // Calculate optimal quality for this target
        profile := ca.calculateOptimalQuality(source, target)
        adapter.OutputProfiles[targetID] = profile

        // Create edge from adapter to target
        edge := Edge{
            From: adapter.ID,
            To: targetID,
            Quality: profile,
        }
        ca.graph.AddEdge(edge)
    }

    return adapter.Start()
}
```

### Example 3: Protocol Translation

```
Matrix User → WebRTC Protocol → Media Bridge → SIP Protocol → Legacy Phone System
           ↓                   ↓             ↓
     Matrix Signaling → Protocol Translator → SIP Signaling
```

## Configuration Examples

### Static Configuration (YAML)

```yaml
call_assistant:
  graph:
    nodes:
      cameras:
        - id: "front_door"
          type: "rtsp"
          url: "rtsp://admin:pass@192.168.1.100/main"

      protocols:
        - id: "matrix_main"
          type: "matrix"
          homeserver: "https://matrix.org"

      sinks:
        - id: "living_room_tv"
          type: "chromecast"
          ip: "192.168.1.50"

      converters:
        - id: "go2rtc"
          type: "go2rtc"
          endpoint: "http://localhost:1984"

    flows:
      - id: "doorbell_to_tv"
        source: "front_door"
        target: "living_room_tv"
        converters: ["go2rtc"]
        quality: "720p"

      - id: "doorbell_to_matrix"
        source: "front_door"
        target: "matrix_main"
        converters: ["go2rtc"]
        auto_quality: true
```

## Benefits of Graph Architecture

### 1. **Flexibility**

- Add new protocols without changing existing code
- Support new device types through plugins
- Dynamic path recalculation

### 2. **Scalability**

- Horizontal scaling of converter nodes
- Load balancing across multiple instances
- Resource optimization

### 3. **Reliability**

- Automatic failover to backup paths
- Health monitoring of all nodes
- Graceful degradation

### 4. **Observability**

- Complete flow tracing
- Per-node metrics collection
- Visual graph representation

This graph-based architecture provides the foundation for a flexible, scalable call assistant that can adapt to changing requirements and network conditions while maintaining optimal performance.
