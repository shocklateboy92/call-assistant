module github.com/shocklateboy92/call-assistant

go 1.24

require gopkg.in/yaml.v3 v3.0.1

require (
	github.com/shocklateboy92/call-assistant/src/api/proto/common v0.0.0
	github.com/shocklateboy92/call-assistant/src/api/proto/events v0.0.0
	github.com/shocklateboy92/call-assistant/src/api/proto/orchestrator v0.0.0
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250324211829-b45e905df463 // indirect
	google.golang.org/grpc v1.73.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

// Local replacements for generated proto packages
replace (
	github.com/shocklateboy92/call-assistant/src/api/proto/common => ./src/generated/go/github.com/shocklateboy92/call-assistant/src/api/proto/common
	github.com/shocklateboy92/call-assistant/src/api/proto/events => ./src/generated/go/github.com/shocklateboy92/call-assistant/src/api/proto/events
	github.com/shocklateboy92/call-assistant/src/api/proto/module => ./src/generated/go/github.com/shocklateboy92/call-assistant/src/api/proto/module
	github.com/shocklateboy92/call-assistant/src/api/proto/orchestrator => ./src/generated/go/github.com/shocklateboy92/call-assistant/src/api/proto/orchestrator
)
