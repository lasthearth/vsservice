# Quickstart: News Deletion with Scope Authorization

## Overview
This guide walks through the implementation of news deletion functionality with scope-based authorization.

## Prerequisites
- Go 1.24 installed
- MongoDB instance running
- Project dependencies installed (`go mod download`)

## Feature Structure
```
internal/
├── news/
│   ├── fx.go                           # News service fx module definition
│   ├── model/                          # News domain models
│   └── internal/
│       ├── dto/                        # Data transfer objects
│       ├── repository/                 # Repository interfaces and implementations 
│       │   └── mongo/                  # MongoDB-specific repository implementation
│       └── service/                    # Service interfaces and implementations
│           ├── interface.go            # Repository interfaces
│           └── service.go              # News service implementation
```

## Implementation Steps
1. Add DeleteNews RPC method to protobuf definition in `/proto/news/v1/news.proto`
2. Generate gRPC code using buf (`buf generate`)
3. Implement DeleteNews method in `internal/news/internal/service/service.go`
4. Complete repository implementation in `internal/news/internal/repository/mongo/repository.go`
5. Add scope validation using Scoper interface
6. Implement error handling following gRPC error format
7. Add tests for successful deletion, unauthorized access, and error scenarios

## Key Components
- **Proto Definition**: Contains the DeleteNews RPC method signature
- **Service Layer**: Contains the business logic for deletion with authorization checks
- **Repository Layer**: Handles the actual deletion from MongoDB
- **Interceptor**: Validates JWT tokens and user scopes before allowing deletion

## Testing
- Unit tests for service and repository layers
- Integration tests to verify complete deletion flow
- Authorization tests to ensure proper scope validation
- Error handling tests for various failure scenarios