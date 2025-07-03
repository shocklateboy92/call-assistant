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

## Advanced Graph Features

### 1. Capability Negotiation
```go
type CapabilityMatcher struct {
    graph *MediaGraph
}

func (cm *CapabilityMatcher) FindCompatiblePath(from, to string) ([]string, error) {
    sourceNode := cm.graph.GetNode(from)
    targetNode := cm.graph.GetNode(to)
    
    // Check direct compatibility
    if sourceNode.IsCompatible(targetNode) {
        return []string{from, to}, nil
    }
    
    // Find converter chain
    return cm.findConverterChain(sourceNode, targetNode)
}

func (cm *CapabilityMatcher) findConverterChain(source, target Node) ([]string, error) {
    // Use A* algorithm to find optimal conversion path
    // considering latency, quality loss, and computational cost
    
    for _, converter := range cm.graph.GetConverters() {
        if converter.CanConvert(source.GetCapabilities(), target.GetRequiredCapabilities()) {
            return []string{source.ID, converter.ID, target.ID}, nil
        }
    }
    
    // Try multi-hop conversion
    return cm.findMultiHopPath(source, target)
}
```

### 2. Quality Adaptation
```go
type QualityController struct {
    flows map[string]*ActiveFlow
    metrics *MetricsCollector
}

func (qc *QualityController) AdaptQuality(flowID string) {
    flow := qc.flows[flowID]
    metrics := qc.metrics.GetFlowMetrics(flowID)
    
    if metrics.PacketLoss > 0.05 { // 5% packet loss
        // Reduce quality
        newProfile := qc.downgradeQuality(flow.CurrentProfile)
        qc.updateFlowQuality(flow, newProfile)
    } else if metrics.Bandwidth > flow.CurrentProfile.RequiredBandwidth*1.5 {
        // Increase quality if bandwidth allows
        newProfile := qc.upgradeQuality(flow.CurrentProfile)
        qc.updateFlowQuality(flow, newProfile)
    }
}
```

### 3. Dynamic Reconfiguration
```go
func (g *MediaGraph) ReconfigureFlow(flowID string, newRequirements FlowRequirements) error {
    flow := g.flows[flowID]
    
    // Calculate new optimal path
    newPath, err := g.findPath(flow.Source, flow.Target, newRequirements)
    if err != nil {
        return err
    }
    
    // Perform hot-swap if possible
    if g.canHotSwap(flow.Path, newPath) {
        return g.hotSwapPath(flow, newPath)
    }
    
    // Otherwise, graceful transition
    return g.gracefulTransition(flow, newPath)
}

func (g *MediaGraph) hotSwapPath(flow *ActiveFlow, newPath []string) error {
    // Create new path in parallel
    newFlow := &ActiveFlow{
        ID: flow.ID + "_transition",
        Path: newPath,
    }
    
    err := g.setupFlow(newFlow)
    if err != nil {
        return err
    }
    
    // Atomic switch
    g.switchFlowPath(flow, newFlow)
    
    // Cleanup old path
    return g.teardownPath(flow.Path)
}
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

### Dynamic API Configuration
```go
// REST API for runtime graph modification
type GraphAPI struct {
    graph *MediaGraph
}

func (api *GraphAPI) AddNode(w http.ResponseWriter, r *http.Request) {
    var nodeConfig NodeConfig
    json.NewDecoder(r.Body).Decode(&nodeConfig)
    
    node, err := api.graph.CreateNode(nodeConfig)
    if err != nil {
        http.Error(w, err.Error(), 400)
        return
    }
    
    api.graph.AddNode(node)
    json.NewEncoder(w).Encode(map[string]string{"id": node.GetID()})
}

func (api *GraphAPI) CreateFlow(w http.ResponseWriter, r *http.Request) {
    var flowRequest FlowRequest
    json.NewDecoder(r.Body).Decode(&flowRequest)
    
    flow, err := api.graph.CreateCall(flowRequest)
    if err != nil {
        http.Error(w, err.Error(), 400)
        return
    }
    
    json.NewEncoder(w).Encode(flow)
}
```

## Path Finding Algorithms

### Optimal Path Selection
```go
type PathFinder struct {
    graph *MediaGraph
}

func (pf *PathFinder) FindOptimalPath(source, target Node, constraints PathConstraints) ([]string, error) {
    // Use modified Dijkstra's algorithm considering:
    // - Latency cost
    // - Quality degradation
    // - Computational cost
    // - Resource availability
    
    costs := make(map[string]float64)
    previous := make(map[string]string)
    unvisited := NewPriorityQueue()
    
    costs[source.ID] = 0
    unvisited.Push(source.ID, 0)
    
    for !unvisited.Empty() {
        current := unvisited.Pop()
        
        if current == target.ID {
            return pf.reconstructPath(previous, current), nil
        }
        
        for _, neighbor := range pf.graph.GetNeighbors(current) {
            cost := pf.calculateEdgeCost(current, neighbor, constraints)
            newCost := costs[current] + cost
            
            if newCost < costs[neighbor] {
                costs[neighbor] = newCost
                previous[neighbor] = current
                unvisited.Push(neighbor, newCost)
            }
        }
    }
    
    return nil, errors.New("no path found")
}

func (pf *PathFinder) calculateEdgeCost(from, to string, constraints PathConstraints) float64 {
    fromNode := pf.graph.GetNode(from)
    toNode := pf.graph.GetNode(to)
    
    // Base latency cost
    latencyCost := pf.estimateLatency(fromNode, toNode)
    
    // Quality degradation cost
    qualityCost := pf.estimateQualityLoss(fromNode, toNode)
    
    // Resource utilization cost
    resourceCost := pf.estimateResourceUsage(fromNode, toNode)
    
    // Weight according to constraints
    totalCost := constraints.LatencyWeight*latencyCost +
                constraints.QualityWeight*qualityCost +
                constraints.ResourceWeight*resourceCost
    
    return totalCost
}
```

### Multi-Constraint Optimization
```go
type PathConstraints struct {
    MaxLatency      time.Duration
    MinQuality      QualityProfile
    MaxCPUUsage     float64
    LatencyWeight   float64
    QualityWeight   float64
    ResourceWeight  float64
}

func (pf *PathFinder) FindConstrainedPath(source, target Node, constraints PathConstraints) ([]string, error) {
    // Use multi-objective optimization
    // Find Pareto-optimal solutions for latency vs quality vs resources
    
    candidatePaths := pf.generateCandidatePaths(source, target)
    feasiblePaths := pf.filterFeasiblePaths(candidatePaths, constraints)
    
    if len(feasiblePaths) == 0 {
        return nil, errors.New("no feasible path within constraints")
    }
    
    return pf.selectBestPath(feasiblePaths, constraints), nil
}
```

## Real-Time Graph Updates

### Live Topology Changes
```go
func (g *MediaGraph) HandleNodeFailure(nodeID string) error {
    failedNode := g.nodes[nodeID]
    
    // Find all affected flows
    affectedFlows := g.getFlowsUsingNode(nodeID)
    
    for _, flow := range affectedFlows {
        // Try to find alternative path
        altPath, err := g.findAlternativePath(flow, nodeID)
        if err != nil {
            // Graceful degradation
            g.degradeFlowQuality(flow)
            continue
        }
        
        // Hot-swap to alternative path
        err = g.hotSwapPath(flow, altPath)
        if err != nil {
            g.terminateFlow(flow.ID)
        }
    }
    
    // Mark node as unavailable
    g.nodes[nodeID].SetStatus(NodeStatusUnavailable)
    
    return nil
}
```

### Capacity Management
```go
type ResourceManager struct {
    graph *MediaGraph
    limits map[string]ResourceLimits
}

func (rm *ResourceManager) CanAcceptNewFlow(flow *ActiveFlow) bool {
    for _, nodeID := range flow.Path {
        node := rm.graph.GetNode(nodeID)
        currentUsage := rm.getCurrentUsage(nodeID)
        requiredResources := rm.estimateFlowResources(flow, nodeID)
        
        if !rm.hasCapacity(currentUsage, requiredResources, nodeID) {
            return false
        }
    }
    return true
}

func (rm *ResourceManager) AdmitFlow(flow *ActiveFlow) error {
    if !rm.CanAcceptNewFlow(flow) {
        return errors.New("insufficient resources")
    }
    
    // Reserve resources
    for _, nodeID := range flow.Path {
        rm.reserveResources(nodeID, flow)
    }
    
    return nil
}
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