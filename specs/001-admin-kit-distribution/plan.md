# Implementation Plan: Admin Kit Distribution

**Branch**: `001-admin-kit-distribution` | **Date**: November 17, 2025 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-admin-kit-distribution/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Implementation of admin kit distribution feature allowing administrators to assign game kits to users in the Vintage Story game mode, with NATS integration for game communication. The feature includes API endpoints for kit assignment, state management for assignments, and proper security using the Scoper interface for authorization.

## Technical Context

**Language/Version**: Go 1.24
**Primary Dependencies**: Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2, MinIO client, JWT authentication, buf for protobuf generation, goverter for struct conversion, NATS for messaging
**Storage**: MongoDB for document storage, MinIO for file/object storage
**Testing**: go testing, mock repositories for unit tests, integration tests for service interactions
**Target Platform**: Linux server environment
**Project Type**: Microservice with gRPC/REST API
**Performance Goals**: Support up to 1,000 registered users with max 10 concurrent admin assignments per hour
**Constraints**: <30 seconds for kit assignment completion, 95% availability for kit access
**Scale/Scope**: Up to 1,000 registered users with max 10 concurrent admin assignments per hour

## Existing Codebase Analysis

**Current Architecture Patterns**:
- Microservices with separate domains (player, news, settlement, etc.)
- gRPC services with protobuf definitions
- Uber fx for dependency injection
- Repository pattern with interfaces between services and data storage
- NATS for messaging between services
- MongoDB for data persistence
- JWT-based authentication with interceptors
- Scoper interface for authorization

**Integration Points**:
- New service will be created in internal/kit directory
- gRPC service will be registered in main application
- MongoDB collection for storing kits and assignments
- NATS integration for communicating with Vintage Story game
- Authentication/authorization via existing interceptors

**Existing Patterns to Follow**:
- Repository interface pattern: `Create(ctx context.Context, req *Type) error`, `Get(ctx context.Context, id string) (*Type, error)`, `Update(ctx context.Context, id string, updateFn func(ctx context.Context, req *Type) (*Type, error)) error`
- Uber fx dependency injection patterns
- Structured logging with zap
- gRPC error handling patterns
- Model validation within domain models
- Repository implementation with MongoDB DTO conversion

**Potential Conflicts**:
- Domain isolation: need to ensure kit service doesn't directly access other service data
- User identification: integration with existing user system
- Database naming consistency

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Gates passed**: All constitutional requirements met:
- Uses Go 1.24 as required
- Uses Uber fx for dependency injection
- Uses MongoDB for data storage
- Implements proper authentication/authorization with JWT and Scoper
- Follows repository pattern for data access
- Uses NATS for messaging as specified in user input

## Project Structure

### Documentation (this feature)

```
specs/001-admin-kit-distribution/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```
internal/
└── kit/                 # New kit service
    ├── internal/
    │   ├── app/         # fx app configuration
    │   ├── dto/
    │   │   ├── http/    # HTTP API DTOs
    │   │   └── mongo/   # MongoDB DTOs
    │   ├── model/       # Domain models with business logic
    │   ├── repository/  # Repository interfaces and implementations
    │   └── service/     # gRPC service implementation
    └── fx.go            # fx module for dependency injection
```

**Structure Decision**: New service in internal/kit directory following existing patterns. Service will implement domain models with business logic, repository interfaces with implementations, and gRPC service endpoints.

## Complexity Tracking

*Fill ONLY if Constitution Check has violations that must be justified*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |