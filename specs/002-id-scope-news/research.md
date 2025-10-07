# Research: News Deletion with Scope Authorization

## Technology Stack Research

### gRPC Implementation
- Go 1.24 with gRPC for service communication
- Repository layer already has DeleteNews method signature that needs implementation
- Service layer requires implementing DeleteNews method following existing patterns
- Error handling follows gRPC error format with status codes and messages

### Authentication & Authorization
- Use JWT authentication via interceptors following settlement service pattern
- Scoper interface exists for method-level authorization
- Need to implement news:delete scope for DeleteNews method

### Data Management
- Follow multi-level data transformation pattern: gRPC protobuf → internal models → MongoDB DTO
- News entity consists of: ID, Title, Content, CreatedAt, UpdatedAt
- MongoDB document should inherit from `mongo.Model` with fields `Id` (bson.ObjectID), `CreatedAt`, `UpdatedAt`

### Performance Requirements
- Delete news items within 2 seconds of request (as specified in requirements)
- Handle concurrent deletion attempts appropriately

## API Design
- Follow simplified Google AIP (API Improvement Proposals) pattern as per project constitution
- Add DeleteNews RPC method to existing protobuf definition
- Implement both gRPC and REST API endpoints (through http annotations)
- Use appropriate error response format (gRPC error format with status codes)

## Implementation Approach
- The implementation will follow existing architecture patterns for consistency
- Repository layer already has DeleteNews method signature (requires implementation)
- Service layer needs to implement DeleteNews method following existing patterns
- Authorization system already in place via Scoper interface (requires news:delete scope definition)