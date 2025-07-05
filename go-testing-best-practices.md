# Go Testing Best Practices for 2025

## Core Testing Philosophy

**Integration-First Approach**: Integration tests are more valuable for catching real-world issues and testing actual system behavior. They test more real code and are much more likely to catch regressions than unit tests.

## Testing Architecture for gRPC-Based Systems

### 1. **Testcontainers + FauxRPC for gRPC Testing**

Use testcontainers with FauxRPC for comprehensive gRPC testing:
- Tests actual network behavior and middleware
- Dynamically configurable with protobuf descriptors
- Reduces maintenance overhead as APIs evolve

```go
container, err := fauxrpctestcontainers.Run(ctx, "docker.io/sudorandom/fauxrpc:latest")
// ... error handling ...
t.Cleanup(func() { 
    container.Terminate(context.Background()) 
})
```

### 2. **Test Separation Strategy**

Use build tags and testing flags to separate test types:

```go
// Use build tags to separate test types
//go:build integration
// +build integration

// Use -short flag for quick vs comprehensive testing
if testing.Short() {
    t.Skip("skipping integration test")
}
```

**Command Usage**:
```bash
# Run unit tests only
go test -short ./...

# Run integration tests only  
go test -tags=integration ./...

# Run all tests
go test ./...
```

## Table-Driven Integration Tests

Perfect for testing module discovery and gRPC communication patterns:

```go
func TestModuleDiscovery(t *testing.T) {
    tests := []struct {
        name         string
        moduleDir    string
        expectedType string
        wantErr      bool
    }{
        {"go2rtc module", "modules/go2rtc", "converter", false},
        {"matrix module", "modules/matrix", "protocol", false},
        {"invalid module", "modules/invalid", "", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test actual module discovery and registration
            orchestrator := NewOrchestrator()
            module, err := orchestrator.DiscoverModule(tt.moduleDir)
            
            if tt.wantErr {
                require.Error(t, err)
                return
            }
            
            require.NoError(t, err)
            require.Equal(t, tt.expectedType, module.Type)
        })
    }
}
```

## Testify for Enhanced Assertions

**Use `require` instead of `assert`** (stops on failure):

```go
require.NoError(t, err)
require.Equal(t, expected, actual)
require.NotNil(t, result)
```

Key differences:
- `require` stops test execution on failure
- `assert` continues test execution on failure
- 99% of the time you want `require`

## Integration Test Patterns

### End-to-End Pipeline Testing

Test complete flows through your system:

```go
func TestCameraToMatrixPipeline(t *testing.T) {
    // Setup test environment
    orchestrator := setupTestOrchestrator(t)
    
    // Test complete pipeline
    pipeline := &Pipeline{
        Source: "test_camera",
        Target: "test_matrix_room",
        Converters: []string{"go2rtc"},
    }
    
    err := orchestrator.CreatePipeline(pipeline)
    require.NoError(t, err)
    
    // Verify pipeline is active
    status := orchestrator.GetPipelineStatus(pipeline.ID)
    require.Equal(t, "active", status.State)
}
```

### Component Integration Testing

Test orchestrator-module communication:

```go
func TestModuleRegistration(t *testing.T) {
    orchestrator := NewOrchestrator()
    
    // Start module
    module, err := orchestrator.StartModule("go2rtc")
    require.NoError(t, err)
    
    // Test registration
    capabilities, err := module.GetCapabilities()
    require.NoError(t, err)
    require.Contains(t, capabilities.EntityTypes, "converter")
}
```

## Test Environment Setup

### Docker Compose for Dependencies

```yaml
# docker-compose.test.yml
version: '3.8'
services:
  test-matrix:
    image: matrixdotorg/synapse:latest
    ports:
      - "8008:8008"
  test-go2rtc:
    image: alexxit/go2rtc:latest
    ports:
      - "1984:1984"
```

### Setup/Teardown Pattern

```go
func TestMain(m *testing.M) {
    // Start test containers
    setup()
    code := m.Run()
    teardown()
    os.Exit(code)
}

func setup() {
    // Initialize test database
    // Start test containers
    // Setup test data
}

func teardown() {
    // Clean up test containers
    // Reset test state
}
```

## Microservices Testing Strategy

### Focus Areas for Your Architecture

1. **Module Discovery and Registration**
   - Test module.yaml parsing
   - Test dependency resolution
   - Test module startup sequence

2. **gRPC Service Communication**
   - Test service method calls
   - Test error handling
   - Test timeout behavior

3. **Pipeline Construction and Management**
   - Test entity creation
   - Test connection establishment
   - Test flow management

4. **Error Handling and Recovery**
   - Test module failure scenarios
   - Test automatic restart
   - Test graceful shutdown

5. **Configuration Management**
   - Test config service integration
   - Test runtime configuration updates
   - Test config validation

### Test Pyramid Adjustment

For microservices architectures:
- **More integration tests** than traditional pyramid
- **Unit tests** for pure business logic
- **Integration tests** for cross-module interactions  
- **End-to-end tests** for critical user flows

## Modern Go Testing Tools (2025)

### Primary Stack

- **Go's built-in testing**: Foundation for all tests
- **Testify**: Enhanced assertions and mocking
- **Testcontainers**: Infrastructure dependencies
- **FauxRPC**: gRPC service mocking

### Alternative Tools

- **Ginkgo/Gomega**: BDD-style tests
- **httpexpect**: HTTP API testing
- **GoConvey**: Web UI test reports

## Performance and Observability Testing

### Benchmark Integration Tests

```go
func BenchmarkPipelineSetup(b *testing.B) {
    orchestrator := NewOrchestrator()
    
    for i := 0; i < b.N; i++ {
        pipeline := &Pipeline{
            Source: "benchmark_source",
            Target: "benchmark_target",
        }
        
        err := orchestrator.CreatePipeline(pipeline)
        if err != nil {
            b.Fatal(err)
        }
        
        orchestrator.DestroyPipeline(pipeline.ID)
    }
}
```

### Fuzz Testing for gRPC Messages

```go
func FuzzModuleRegistration(f *testing.F) {
    f.Add([]byte(`{"name": "test-module", "version": "1.0.0"}`))
    
    f.Fuzz(func(t *testing.T, data []byte) {
        orchestrator := NewOrchestrator()
        
        // Test module registration with fuzzy data
        _, err := orchestrator.ParseModuleManifest(data)
        // Should not panic, may return error
        _ = err
    })
}
```

## CI/CD Integration

### GitHub Actions Pattern

```yaml
name: Test
on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run Unit Tests
        run: go test -short ./...

  integration-tests:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: postgres
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run Integration Tests
        run: go test -tags=integration ./...
```

## Testing Patterns for Your Call Assistant Architecture

### Module Lifecycle Testing

```go
func TestModuleLifecycle(t *testing.T) {
    orchestrator := NewOrchestrator()
    
    // Test discovery
    modules, err := orchestrator.DiscoverModules()
    require.NoError(t, err)
    require.NotEmpty(t, modules)
    
    // Test startup
    for _, module := range modules {
        err := orchestrator.StartModule(module.ID)
        require.NoError(t, err)
        
        // Test health check
        health, err := orchestrator.CheckModuleHealth(module.ID)
        require.NoError(t, err)
        require.Equal(t, "healthy", health.Status)
    }
    
    // Test shutdown
    err = orchestrator.ShutdownAllModules()
    require.NoError(t, err)
}
```

### Pipeline Flow Testing

```go
func TestPipelineFlow(t *testing.T) {
    tests := []struct {
        name     string
        source   string
        target   string
        expected string
    }{
        {"RTSP to WebRTC", "rtsp_camera", "webrtc_client", "active"},
        {"USB to Chromecast", "usb_camera", "chromecast_tv", "active"},
        {"Matrix to SIP", "matrix_room", "sip_phone", "active"},
    }
    
    orchestrator := setupTestOrchestrator(t)
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            flow, err := orchestrator.CreateFlow(tt.source, tt.target)
            require.NoError(t, err)
            
            // Wait for flow to stabilize
            time.Sleep(2 * time.Second)
            
            status := orchestrator.GetFlowStatus(flow.ID)
            require.Equal(t, tt.expected, status.State)
        })
    }
}
```

## Best Practices Summary

1. **Start with Integration Tests**: They catch more real-world issues
2. **Use Table-Driven Tests**: Organize test cases efficiently
3. **Leverage Testcontainers**: Test with real dependencies
4. **Separate Test Types**: Use build tags and flags
5. **Focus on Critical Paths**: Test end-to-end user flows
6. **Test Error Scenarios**: Don't just test happy paths
7. **Use Testify Wisely**: Prefer `require` over `assert`
8. **Document Test Intent**: Clear test names and comments
9. **Automate in CI/CD**: Fast feedback loops
10. **Monitor Test Performance**: Keep integration tests fast enough

This approach emphasizes real-world testing scenarios while maintaining the modularity and independence your architecture requires. The integration-first philosophy will help catch issues that unit tests might miss, especially in a complex multi-language, multi-protocol system.