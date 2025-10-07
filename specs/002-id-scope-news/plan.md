# Implementation Plan: Add News Deletion by ID with Scope Authorization

**Branch**: `002-id-scope-news` | **Date**: 2025-10-07 | **Spec**: /Users/ripls/Documents/GitHub/vsservice/specs/002-id-scope-news/spec.md
**Input**: Feature specification from `/specs/002-id-scope-news/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Based on research findings, implement news deletion functionality that allows authorized users with news:delete scope to remove news items by their unique identifier. The implementation will follow existing patterns in the codebase: add DeleteNews RPC to the protobuf, implement the service method in service.go, complete the repository implementation in repository.go, and add scope authorization following the settlement service pattern. The repository's DeleteNews method currently exists but panics as unimplemented, providing a foundation for this feature. The implementation will use JWT authentication via interceptors and implement proper error handling as specified in requirements.

## Technical Context

**Language/Version**: Go 1.24  
**Primary Dependencies**: Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2, JWT authentication with interceptor package  
**Storage**: MongoDB for document storage  
**Testing**: Go native testing framework, integration tests using MongoDB test containers  
**Target Platform**: Linux server container
**Project Type**: Backend microservice (single project with service architecture)  
**Performance Goals**: Delete news items within 2 seconds of request, handle concurrent deletion attempts appropriately  
**Constraints**: Must use existing authentication interceptor pattern, return gRPC error format for failures, maintain consistency with existing news service architecture  
**Scale/Scope**: Handle concurrent deletion requests, maintain 99% uptime during operations

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

1. **Architecture**: Solution must follow microservice architecture with clear service boundaries (PASSED - extending existing news service)
2. **Dependency Injection**: Solution must use Uber fx for dependency management (PASSED - existing architecture pattern)
3. **Data Management**: Follow multi-level data transformation pattern (gRPC protobuf → internal models → MongoDB DTO) (PASSED - existing pattern)
4. **Code Style**: Follow Go idioms and best practices, avoid nested conditionals (KISS principle) (PASSED - will follow existing patterns)
5. **Separation of Concerns**: Business logic in service layer, data access in repository layer (PASSED - implementation will follow existing pattern)
6. **Authentication**: All gRPC methods must be protected by JWT authentication (PASSED - will implement scope authorization following settlement pattern)
7. **Configuration**: Follow environment-based configuration management (PASSED - existing pattern)
8. **Authorization**: Use Scoper interface for method-level authorization (GATE: REQUIREMENT - must implement Scoper to define news:delete scope for DeleteNews method)
9. **Error Handling**: Follow gRPC error format as specified (PASSED - pattern exists in codebase)

**Constitution Compliance Verification Post-Research:**
- Repository layer already has DeleteNews method signature (requires implementation)
- Service layer needs to implement DeleteNews method following existing patterns
- Authorization system already in place via Scoper interface (requires news:delete scope definition)
- Architecture pattern consistent with existing services
- Error handling patterns already established in codebase

**Constitution Check Post-Design:**
- ✓ Business logic separation maintained: service layer handles validation/coordination, repository handles data operations
- ✓ Authorization follows constitution: using Scoper interface for method-level scope validation
- ✓ Multi-level data transformation maintained: gRPC protobuf → internal models → MongoDB DTO pattern unchanged
- ✓ Service boundaries clear: News service manages its own domain logic
- ✓ Authentication pattern preserved: JWT interceptors continue to protect endpoints
- ✓ Dependency injection maintained: Uber fx continues to manage component lifecycle
- ✓ Architecture pattern compliant: Microservice architecture with proper separation of concerns
- ✓ Code style adherence: Following existing patterns and KISS principle
- ✓ Error handling: Following gRPC error format as required by specification
- ✓ API design: Following simplified Google AIP pattern as per constitution

## Project Structure

### Documentation (this feature)

```
specs/[###-feature]/
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
├── pkg/
│   ├── config/                         # Configuration management
│   ├── logger/                         # Logging utilities
│   ├── mongo/                          # MongoDB utilities
│   └── jwt/                           # JWT utilities
└── server/                            # gRPC/HTTP server
    └── interceptor/                    # Authentication and authorization interceptors
```

**Structure Decision**: Extend existing news service following current architecture patterns. Addition of delete endpoint will follow same patterns as other CRUD operations in the news service.

## Complexity Tracking

*Fill ONLY if Constitution Check has violations that must be justified*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [None identified] | [N/A] | [N/A] |
