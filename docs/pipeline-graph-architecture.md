# Pipeline/Graph Architecture

## Core Entities

**Sources**: Video/audio input (RTSP cameras, USB webcams)
**Sinks**: Output devices (Chromecast, Miracast, displays)
**Protocols**: Bidirectional calling (Matrix, XMPP, WebRTC)
**Converters**: Media transformation (go2rtc, FFmpeg)

## Entity Types

```yaml
# Video Sources
rtsp_camera:
  type: "rtsp"
  url: "rtsp://camera/stream"
  capabilities: ["h264", "aac", "1080p"]

# Video Sinks  
chromecast:
  type: "chromecast"
  ip: "192.168.1.50"
  capabilities: ["h264", "vp8", "aac"]

# Calling Protocols
matrix:
  type: "matrix"
  homeserver: "https://matrix.org" 
  capabilities: ["webrtc", "h264", "opus"]

# Converters
go2rtc:
  type: "go2rtc"
  input: ["rtsp", "usb", "hls"]
  output: ["webrtc", "hls", "mp4"]
```

## Pipeline Construction

1. **Find Path**: Source → Converters → Target
2. **Create Entities**: Request modules to create entity instances
3. **Connect**: Establish connections between entities
4. **Start Flow**: Begin media transport

## Example Flow

```
RTSP Camera → go2rtc Converter → Matrix Protocol → Remote User
```

1. Find modules: camera (go2rtc), protocol (matrix)
2. Create entities: `CreateEntity(rtsp_source)`, `CreateEntity(matrix_call)`
3. Connect: `ConnectEntities(source, target)`
4. Start: `StartFlow()` begins media streaming