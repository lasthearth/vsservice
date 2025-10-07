# Implementation Plan: Refactor Settlement Submit Business Logic

**Branch**: `001-submit-settlement` | **Date**: 2025-10-07 | **Spec**: /Users/ripls/Documents/GitHub/vsservice/specs/001-submit-settlement/spec.md
**Input**: Feature specification from `/specs/001-submit-settlement/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Based on research findings, refactor the business logic in the Submit method of the Settlement service according to the project constitution. The repository layer's Submit method currently contains business logic that should be in the service layer, including user membership validation, settlement type progression logic (LvlUp), and complex conditional flows. The service layer already handles attachment processing, authentication, and validation, but needs to incorporate the business rules currently in the repository. Domain model methods like LvlUp() will remain as they properly encapsulate business logic in the model. This refactoring maintains the multi-level data transformation pattern while ensuring service layer handles business logic and repository focuses purely on data persistence.

## Technical Context

**Language/Version**: Go 1.24  
**Primary Dependencies**: Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2, MinIO client, JWT authentication, buf for protobuf generation, goverter for struct conversion  
**Storage**: MongoDB for document storage, MinIO-compatible object storage for attachments  
**Testing**: Go native testing framework, integration tests using MongoDB and MinIO test containers  
**Target Platform**: Linux server container  
**Project Type**: Backend microservice (single project with service architecture)  
**Performance Goals**: Under 30 seconds for settlement submission process, 99% uptime during submission, 100% attachment conversion success  
**Constraints**: Must maintain clear separation between business logic (service layer) and data access (repository layer), must follow project constitution regarding validation in domain models  
**Scale/Scope**: Handle concurrent submissions based on available resources without specific limits, immediate failure on external service unavailability

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

1. **Architecture**: Solution must follow microservice architecture with clear service boundaries (PASSED - refactoring within existing settlement service)
2. **Dependency Injection**: Solution must use Uber fx for dependency management (PASSED - existing architecture pattern)
3. **Data Management**: Follow multi-level data transformation pattern (gRPC protobuf → internal models → MongoDB DTO) (PASSED - existing pattern)
4. **Code Style**: Business logic validation must be encapsulated in domain models (RESOLVED - LvlUp method properly in domain, other logic moving to service layer)
5. **Separation of Concerns**: Business logic must remain in service layer, not repository layer (GATE: REQUIRES ACTION - moving business logic from repository to service as main purpose of this feature)
6. **Authentication**: All gRPC methods must be protected by JWT authentication (PASSED - existing interceptor pattern)
7. **Configuration**: Follow environment-based configuration management (PASSED - existing pattern)

**Constitution Compliance Verification Post-Research:**
- Repository layer currently contains business logic (violates constitution principle #5)
- Service layer should handle business logic, repository should focus on data persistence
- Domain model business logic (like LvlUp) is acceptable per constitution
- After refactoring: service layer will contain business validation and rules, repository will handle pure data operations

**Constitution Check Post-Design:**
- ✓ Business logic separation will be achieved: service layer handles validation/rules, repository focuses on data operations
- ✓ Domain model encapsulation preserved: LvlUp method remains in SettlementVerification model as appropriate
- ✓ Multi-level data transformation maintained: gRPC protobuf → internal models → MongoDB DTO pattern unchanged
- ✓ Service boundaries clear: Settlement service manages its own domain logic
- ✓ Authentication pattern preserved: JWT interceptors continue to protect endpoints
- ✓ Dependency injection maintained: Uber fx continues to manage component lifecycle
- ✓ Architecture pattern compliant: Microservice architecture with proper separation of concerns

## Project Structure

### Documentation (this feature)

```
specs/001-submit-settlement/
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
├── settlement/
│   ├── fx.go                           # Settlement service fx module definition
│   ├── model/                         # Domain models (Settlement, Attachment, etc.)
│   └── internal/
│       ├── dto/                       # Data transfer objects
│       ├── repository/                # Repository interfaces and implementations 
│       │   └── mongo/                 # MongoDB-specific repository implementation
│       └── service/                   # Service interfaces and implementations
│           ├── interface.go           # Repository interfaces
│           └── service.go             # Settlement service implementation
├── pkg/
│   ├── config/                        # Configuration management
│   ├── logger/                        # Logging utilities
│   ├── mongo/                         # MongoDB utilities
│   ├── storage/                       # Object storage (MinIO) utilities
│   └── jwt/                          # JWT utilities
└── server/                           # gRPC/HTTP server
    └── interceptor/                   # Authentication and authorization interceptors
```

**Structure Decision**: Refactor existing settlement service following current architecture patterns. Business logic will be moved from repository layer to service layer while maintaining all existing interfaces and data transformation layers.

## Complexity Tracking

*Fill ONLY if Constitution Check has violations that must be justified*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [None identified] | [N/A] | [N/A] |
