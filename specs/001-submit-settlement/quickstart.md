# Quickstart: Refactor Settlement Submit Business Logic

## Overview
This guide provides a quick overview of the refactored settlement submission feature that properly separates business logic from data access concerns according to the project constitution.

## Architecture Changes

### Before Refactoring
- Business logic was mixed in repository layer
- Service layer handled only attachment processing and basic validation
- Violated constitution principle of separating business logic from data access

### After Refactoring
- All business logic (user membership checks, settlement progression, validation) is in service layer
- Repository layer handles only data persistence operations
- Domain models contain appropriate business logic (like LvlUp method)
- Clear separation of concerns according to project constitution

## Key Components

### Service Layer (`internal/settlement/internal/service/service.go`)
- Contains all business validation and rules for settlement submissions
- Performs user membership validation before allowing submissions
- Handles settlement type progression logic
- Interacts with repository for data persistence

### Repository Layer (`internal/settlement/internal/repository/mongo/`)
- Focuses on pure data operations (CRUD, queries)
- No business logic or validation rules
- Maintains existing data transformation patterns
- Handles database-specific operations and transactions

### Domain Models (`internal/settlement/model/`)
- Contain business logic that belongs to the entity itself (e.g., LvlUp method)
- Data structures representing business concepts
- Validation rules that are inherent to the entity

## Implementation Steps

### 1. Move Business Logic
- Extract user membership validation from repository to service
- Move settlement progression logic to service layer
- Update service to perform validation before calling repository

### 2. Update Repository Interface
- Simplify repository methods to focus on data operations
- Remove business logic concerns from repository interface
- Maintain existing data transformation patterns

### 3. Preserve Existing Patterns
- Keep gRPC service interface unchanged for API compatibility
- Maintain existing fx module definitions
- Preserve authentication and authorization flows

## Testing Considerations

### Service Layer Tests
- Test business validation logic independently
- Mock repository layer for unit tests
- Verify proper validation of user membership, submission rules

### Repository Layer Tests
- Focus on data persistence operations
- Verify CRUD operations work correctly
- Test database query accuracy

## Integration Points

### Authentication
- Authentication still handled via gRPC interceptors
- User ID extracted from token context in service layer
- No changes to authentication flow

### Object Storage
- Attachment processing remains in service layer
- MinIO integration unchanged
- Public URL generation preserved

## Error Handling

### Business Validation Errors
- Moved from repository to service layer
- Proper error responses maintained
- Validation failure messages preserved

### Data Persistence Errors
- Remain in repository layer
- Error propagation unchanged
- Same error types and handling patterns

## Performance Impact
- No significant performance changes expected
- Same database queries and operations preserved
- Potentially improved testability and maintenance

## Deployment

The refactoring doesn't change any external APIs or interfaces, so deployment should be straightforward:
- No API contract changes required
- No database schema changes needed
- Existing configuration and environment settings maintained