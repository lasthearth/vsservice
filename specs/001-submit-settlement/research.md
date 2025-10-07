# Research: Refactor Settlement Submit Business Logic

## Current Architecture Analysis

### Service Layer (internal/settlement/internal/service/service.go)
The Submit method currently:
- Processes attachments (converts to webp format, uploads to MinIO)
- Extracts user ID from authentication context
- Validates attachments are provided
- Creates model.Attachment objects from request data
- Converts request data to SettlementOpts structure
- Calls repository Submit method with opts

### Repository Layer (internal/settlement/internal/repository/mongo/verification.go)
The Submit method currently:
- Checks if a settlement request already exists for the user
- Performs business logic for checking if user is already a member or leader
- Has a LvlUp() function that handles settlement type progression (camp → village → city → province, etc.)
- Handles both new submissions and updates to existing requests
- Implements the core business rule logic for settlement submissions

## Issues Identified

### 1. Business Logic in Repository
The repository's Submit method contains business logic that should be in the service layer:
- Checking if user is already a member/leader (IsMemberOrLeader call)
- Settlement type progression logic (LvlUp method)
- Status management logic
- Complex conditional flows

### 2. Violation of Architecture Principles
- Business validation and logic are in the repository, violating the principle of separating data access from business logic
- Domain models (like SettlementVerification) contain some business logic (LvlUp) but other logic is in repository
- Repository should only handle data persistence concerns

## Refactoring Plan

### 1. Move Business Logic to Service Layer
- Move user membership checking logic to service layer
- Move settlement type progression logic to service layer
- Preserve data persistence operations in repository layer

### 2. Enhance Domain Models
- Ensure domain models contain appropriate business logic as per constitution
- The LvlUp method in SettlementVerification should remain as it's properly encapsulated in the domain model

### 3. Update Repository Interface
- Simplify repository interface to focus on data operations only
- Remove business logic operations from repository interface

### 4. Preserve Data Transformation Layer
- Maintain the transformation layers (gRPC protobuf → internal models → MongoDB DTO)
- Keep mapper interfaces and implementations as they are

## Implementation Strategy

### Phase 1: Extract Business Logic
1. Identify all business logic in repository Submit method
2. Move validation and business rule checks to service layer
3. Update service layer to call appropriate validation before repository operations

### Phase 2: Update Repository Contract
1. Modify repository interface to focus purely on persistence operations
2. Create simpler, more focused repository methods
3. Ensure repository methods have clear data-only contracts

### Phase 3: Maintain Inter-Service Communication
1. Keep existing gRPC service interface unchanged (maintain API compatibility)
2. Preserve existing fx module definitions
3. Maintain existing authentication and authorization flows

## Risks and Considerations

### 1. Transaction Safety
- Currently, repository operations may involve transactions for complex operations
- Need to ensure transaction boundaries are preserved during refactoring

### 2. Error Handling
- Ensure error propagation remains consistent
- Maintain existing error types and messages where possible

### 3. Testing Impact
- Existing tests may need updates to reflect new logic location
- Unit tests should be easier to implement with proper separation

## Decision

The refactoring will follow the constitution principle that business logic should be in the service layer while data access remains in the repository layer. Domain model methods like LvlUp() can remain as they represent business logic encapsulated directly in the model, which is acceptable per the constitution.

## Rationale

1. **Separation of Concerns**: Service layer handles business logic, repository handles data persistence
2. **Testability**: Business logic in service layer can be more easily unit tested with mock repositories
3. **Maintainability**: Clear separation makes it easier to understand and modify business rules independently of data access concerns
4. **Constitution Compliance**: Follows project principle that business logic validation should be in service layer

## Architecture Alignment

This refactoring aligns with the project constitution:
- Maintains clear separation between business logic and data access concerns
- Keeps validation in appropriate layers (domain models and service layer)
- Preserves multi-level data transformation pattern
- Maintains existing authentication and dependency injection patterns