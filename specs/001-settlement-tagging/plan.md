# Implementation Plan: [FEATURE]

**Branch**: `[###-feature-name]` | **Date**: [DATE] | **Spec**: [link]
**Input**: Feature specification from `/specs/[###-feature-name]/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

The settlement tagging system enables administrators to add tags to settlements (like GitHub labels) with color coding and allows users to view and filter settlements by these tags. The implementation extends the existing settlements service rather than creating a separate tags service, following constitutional principle 14. 

Key design decisions include:
- Tags stored as references (IDs) within settlement documents in MongoDB for efficient filtering
- Unique constraint on tag names enforced via MongoDB unique index
- Soft deletion using an `isActive` field to preserve historical data
- Scope-based authorization with `tags:create` and `tags:delete` permissions
- Partial matching (ANY selected tag) for tag filtering as specified

The API extends the existing settlements protobuf definitions with new methods for tag management and filtering, following established gRPC and REST patterns via grpc-gateway.

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
  
  CRITICAL NOTE FOR EXISTING CODEBASES: Before planning any new feature, analyze the
  existing codebase to understand:
  - Current architectural patterns and conventions
  - Domain boundaries and service responsibilities
  - Data models and repository patterns
  - API design patterns and protobuf definitions
  - Authentication and authorization mechanisms
  - Error handling and logging patterns
  - Dependency injection with Uber fx
  - Testing strategies and patterns
  
  Incorporate these existing patterns in your plan to ensure consistency.
-->

**Language/Version**: Go 1.24, with Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2, MinIO client, JWT authentication, buf for protobuf generation, goverter for struct conversion  
**Primary Dependencies**: Uber fx, Google gRPC, MongoDB driver v2, MinIO client, JWT, buf, goverter, zap logger, envconfig, godotenv, go-retryablehttp, samber/lo, google/uuid  
**Storage**: MongoDB for document storage (with embedded tag references in settlements), MinIO for file/object storage  
**Testing**: Go testing with table-driven tests, gomock for mocking dependencies, integration tests for cross-component verification  
**Target Platform**: Linux server (Docker container)  
**Project Type**: Microservice with gRPC and REST API (via grpc-gateway), extending existing settlements service  
**Performance Goals**: <200ms p95 response time for tag operations, support for up to 20 tags per settlement  
**Constraints**: <200ms p95 for tag filtering operations, maintain 100% accuracy in tag display, handle concurrent tag modifications with appropriate locking/optimistic locking  
**Scale/Scope**: Support small number of users (under 1000), support for settlements with up to 20 tags each, tag system should handle soft deletion for data integrity

## Existing Codebase Analysis

**Current Architecture Patterns**:
- Microservices architecture with each feature as a separate service using fx dependency injection
- gRPC services with grpc-gateway providing automatic REST API conversion
- Domain-driven design approach with services organized by domain
- Data layer using repository pattern with MongoDB as primary storage
- JWT-based authentication with interceptors and scope-based authorization

**Integration Points**:
- The tags feature will integrate into the existing settlements service rather than creating a separate tags service
- Extension of Settlement model to include tags field
- Addition of tag-related methods to the existing settlements service
- Potential modification of settlement listing endpoints to support filtering by tags
- Need to consider how tag permissions integrate with existing scope-based authorization

**Existing Patterns to Follow**:
- Follow the same service structure: protobuf definitions, gRPC service interfaces, service implementation, repository pattern
- Use the same model structure with embedded CreatedAt and UpdatedAt timestamps
- Follow the same DTO conversion patterns using goverter
- Apply the same authentication and authorization interceptors with scope checking
- Use the same error handling and logging patterns with zap
- Follow the same configuration patterns with envconfig and godotenv
- Use the same MongoDB model structure inheriting from mongo.Model

**Potential Conflicts**:
- Modifying the core Settlement model and service may impact existing functionality
- Adding filtering capabilities to settlement listing may affect performance of existing operations
- Need to ensure tag operations maintain consistency with settlement data
- Tag permissions (tags:create, tags:delete) need to integrate with existing scope system

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Principle 2 Check (Architecture and Structure)**: 
- PASS - Implementation extends existing settlements service rather than creating a separate tags service, following the guidance that if functionality is related to an existing domain, it should extend the corresponding service.

**Principle 4 Check (Data Management)**:
- PASS - Follows the pattern of domain models in `/internal/settlements/model/`, DTOs in `/internal/settlements/internal/dto/`, and repository interfaces in `/internal/settlements/internal/service/interface.go`.

**Principle 5 Check (Authentication and Authorization)**:
- PASS - Uses existing JWT authentication with gRPC interceptors, with scope-based authorization for tag operations (tags:create, tags:delete).

**Principle 14 Check (Adding New Code)**:
- PASS - Implementation extends existing settlements service since tagging is related to the existing settlement domain, not creating a new service.

**Principle 15 Check (Changing Existing Code)**:
- PASS - Follows existing architectural patterns in the settlements service when adding tag functionality, studying existing patterns before implementation.

**Security Check (Principle 12)**:
- PASS - Implements proper validation for tag names, uses parameterized queries for MongoDB, and implements rate limiting where needed for tag operations.

**POST-DESIGN RE-EVALUATION**:
- All design decisions align with constitutional principles
- No architectural violations identified
- Implementation approach maintains service isolation through interface-based design
- Follows established patterns for data validation, error handling, and logging
- Tag storage uses efficient MongoDB array queries as appropriate for the use case
- Proper security measures implemented for scope-based authorization
- Soft deletion approach maintains data integrity as required

## Project Structure

### Documentation (this feature)

```
specs/001-settlement-tagging/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

Since we're extending the existing settlements service rather than creating a new service, the changes will be integrated into the existing structure:

```
internal/
└── settlements/
    ├── model/
    │   ├── settlement.go          # Extended to include tags
    │   └── tag.go                 # New tag model with color and isActive fields
    ├── internal/
    │   ├── dto/
    │   │   ├── settlement.go      # Extended DTO with tags
    │   │   └── tag.go             # Tag DTO
    │   ├── service/
    │   │   ├── interface.go       # Extended with tag methods
    │   │   └── implementation.go  # Implementation of tag operations
    │   └── repository/
    │       ├── settlement.go      # Extended with tag-related queries
    │       └── tag.go             # Tag repository with unique name constraint
    ├── pb/                       # Generated protobuf code
    ├── settlements_grpc.pb.go    # Generated gRPC code
    └── settlements.go            # Service registration
proto/
└── settlements/
    ├── settlements.proto         # Extended with tag operations
    └── tag.proto                 # New tag protobuf definitions
gen/
└── proto/
    └── settlements/              # Generated code from protobuf
```

**Structure Decision**: The tagging functionality will be implemented as an extension to the existing settlements service, following the constitution's guidance that new functionality related to an existing domain should extend the corresponding service rather than creating a new one. This ensures data consistency and maintains architectural coherence. The tag model will include a color property and isActive status for soft deletion, with unique name validation across the system.

## Complexity Tracking

*Fill ONLY if Constitution Check has violations that must be justified*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
