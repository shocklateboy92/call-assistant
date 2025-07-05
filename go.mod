module github.com/shocklateboy92/call-assistant

go 1.24

require gopkg.in/yaml.v3 v3.0.1

// Local replacements for generated proto packages
replace (
	github.com/shocklateboy92/call-assistant/src/api/proto/common => ./src/generated/go/github.com/shocklateboy92/call-assistant/src/api/proto/common
	github.com/shocklateboy92/call-assistant/src/api/proto/events => ./src/generated/go/github.com/shocklateboy92/call-assistant/src/api/proto/events
	github.com/shocklateboy92/call-assistant/src/api/proto/module => ./src/generated/go/github.com/shocklateboy92/call-assistant/src/api/proto/module
)
