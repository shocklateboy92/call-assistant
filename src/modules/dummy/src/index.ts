#!/usr/bin/env node

import { createServer } from 'nice-grpc';
import { 
  ModuleServiceImplementation,
  ModuleServiceDefinition,
  HealthCheckRequest,
  HealthCheckResponse,
  ConfigureRequest,
  ConfigureResponse,
  ShutdownRequest,
  ShutdownResponse,
} from 'call-assistant-protos/module';
import {
  ModuleState,
  HealthStatus,
} from 'call-assistant-protos/common';
import type { CallContext } from 'nice-grpc-common';

class DummyModule implements ModuleServiceImplementation {
  private moduleId: string = 'dummy';
  private config: { [key: string]: string } = {};


  async healthCheck(
    request: HealthCheckRequest,
    context: CallContext
  ): Promise<HealthCheckResponse> {
    console.log('[Dummy Module] HealthCheck called');
    
    return {
      status: {
        state: ModuleState.MODULE_STATE_READY,
        health: HealthStatus.HEALTH_STATUS_HEALTHY,
        error_message: '',
        last_heartbeat: new Date(),
      },
    };
  }

  async configure(
    request: ConfigureRequest,
    context: CallContext
  ): Promise<ConfigureResponse> {
    console.log('[Dummy Module] Configure called with config:', request.config);
    
    // Update configuration
    this.config = { ...this.config, ...request.config };
    
    return {
      success: true,
      error_message: '',
      status: {
        state: ModuleState.MODULE_STATE_READY,
        health: HealthStatus.HEALTH_STATUS_HEALTHY,
        error_message: '',
        last_heartbeat: new Date(),
      },
    };
  }

  async shutdown(
    request: ShutdownRequest,
    context: CallContext
  ): Promise<ShutdownResponse> {
    console.log('[Dummy Module] Shutdown called with:', request);
    
    const response: ShutdownResponse = {
      success: true,
      error_message: '',
    };

    // Gracefully shutdown after sending response
    setTimeout(() => {
      console.log('[Dummy Module] Shutting down gracefully');
      process.exit(0);
    }, 1000);

    return response;
  }

}

// Main execution
async function main() {
  const port = parseInt(process.env.GRPC_PORT || '50051');
  console.log(`[Dummy Module] Starting on port ${port}`);

  const server = createServer();
  server.add(ModuleServiceDefinition, new DummyModule());

  await server.listen(`0.0.0.0:${port}`);
  console.log(`[Dummy Module] Server started on port ${port}`);
  console.log('[Dummy Module] Ready to receive requests');

  // Keep the process alive and handle shutdown signals
  process.on('SIGINT', () => {
    console.log('[Dummy Module] Received SIGINT, shutting down gracefully');
    server.shutdown();
    process.exit(0);
  });

  process.on('SIGTERM', () => {
    console.log('[Dummy Module] Received SIGTERM, shutting down gracefully');
    server.shutdown();
    process.exit(0);
  });
}

if (require.main === module) {
  main().catch((error) => {
    console.error('[Dummy Module] Failed to start:', error);
    process.exit(1);
  });
}