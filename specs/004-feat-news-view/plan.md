# Implementation Plan: News View Counter

**Branch**: `004-feat-news-view` | **Date**: 2025-10-07 | **Spec**: [link to spec.md](spec.md)
**Input**: Feature specification from `/specs/004-feat-news-view/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

The primary requirement is to implement a news view counter that increments each time a user views a news article. The technical approach involves extending the existing news service with an atomic increment operation in MongoDB to update the view count when an article is accessed. The view count will be stored as a field in the news article document and displayed to users when they view an article. Performance goals include incrementing the count within 1 second and displaying the count within 2 seconds of page load. The implementation will follow the existing project architecture using Go 1.24, gRPC, and MongoDB.

## Technical Context

**Language/Version**: Go 1.24  
**Primary Dependencies**: Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2  
**Storage**: MongoDB for document storage  
**Testing**: go test for unit and integration tests  
**Target Platform**: Linux server  
**Project Type**: Microservice - extending existing service architecture  
**Performance Goals**: 99.9% of view count increments completed within 1 second, 2 second page load time including view count display  
**Constraints**: Up to 100 concurrent users, 1,000 news articles with up to 50 views/day per article, no special privacy compliance requirements  
**Scale/Scope**: Small scale - up to 1,000 articles with limited concurrent users

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Compliance with Project Constitution

1. **Technology Stack Compliance**: ✓ Using Go 1.24, Uber fx, gRPC, MongoDB driver v2 as specified in constitution
2. **Architecture Compliance**: ✓ Following microservice architecture with gRPC server and REST API via grpc-gateway
3. **Data Management Compliance**: ✓ Using MongoDB with proper model structure inheriting from mongo.Model
4. **Authentication Compliance**: ✓ JWT-based authentication with gRPC interceptors (if needed for this feature)
5. **Code Style Compliance**: ✓ Following Go idioms, proper error handling, structured logging with zap
6. **API Design Compliance**: ✓ Following Google AIP principles for API design
7. **Service Isolation**: ✓ New feature implemented as extension to existing news service rather than a separate service
8. **Documentation Compliance**: ✓ Following documentation practices as per constitution
9. **Repository Pattern Compliance**: ✓ Following repository pattern with interface in service/ and implementation in internal/repository/
10. **Model Structure Compliance**: ✓ Adding view_count field to News model with proper validation
11. **gRPC Service Compliance**: ✓ Extending existing service with new functionality while maintaining interface contracts

## Project Structure

### Documentation (this feature)

```
specs/004-feat-news-view/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

Based on the project constitution, the news view counter feature will be integrated into the existing news service:

```
internal/
└── news/
    ├── model/
    │   └── news.go          # News article model with view count field
    ├── service/
    │   └── news_service.go  # gRPC service with view count increment logic
    └── internal/
        ├── dto/
        │   └── news_dto.go  # MongoDB DTOs
        ├── repository/
        │   └── mongo/
        │       └── news_repository.go  # Repository with view count operations
        └── handler/
            └── news_handler.go        # gRPC handler methods

proto/
└── news/
    └── news.proto         # gRPC service definition including view count methods

gen/
└── proto/
    └── news/
        └── news_grpc.pb.go # Generated gRPC code

tests/
├── unit/
│   └── news/
│       └── news_service_test.go
├── integration/
│   └── news/
│       └── news_repository_test.go
└── contract/
    └── news/
        └── news_contract_test.go
```

**Structure Decision**: The news view counter feature will be implemented as an extension to the existing news service rather than a new separate service. This follows the constitution's guidance about implementing new functionality within existing domain services when appropriate. The feature requires adding a view count field to news articles and implementing increment logic in the service layer with corresponding gRPC endpoints.

## Complexity Tracking

*Fill ONLY if Constitution Check has violations that must be justified*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
