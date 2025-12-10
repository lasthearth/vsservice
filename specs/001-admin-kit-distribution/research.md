# Research: Admin Kit Distribution

**Feature**: Admin Kit Distribution | **Date**: November 17, 2025

## Overview
Research for implementing admin kit distribution feature that allows administrators to assign game kits to users in the Vintage Story game mode.

## Kit Domain Model Research

### Decision: Kit Domain Structure
**Rationale**: Following the constitution's principle that domain models should encapsulate business logic, the Kit model will include methods for validation, assignment, and state management.

**Domain Model**:
- Kit: Contains name, description, items list, availability status
- KitAssignment: Contains user ID, kit ID, assignment state (pending, delivered, claimed), timestamps
- KitService: Contains business logic for assignment operations

### Alternatives considered:
1. Simple data model with logic in service layer - rejected because constitution specifies business logic should be in domain models
2. Complex hierarchy with inheritance - rejected as over-engineered for this use case

## NATS Integration Approach

### Decision: NATS Request/Response Pattern
**Rationale**: The existing codebase shows NATS being used for request/response patterns (as seen in internal/player/internal/event/app.go). This approach provides reliable delivery and feedback for kit assignment operations.

**Implementation**:
- Use NATS queues for communication between service and Vintage Story game
- Implement request/response pattern for kit delivery confirmation
- Use error handling via headers as per existing pattern

### Alternatives considered:
1. Fire-and-forget publish - rejected due to lack of confirmation for critical operations
2. Direct database sharing - rejected as it violates service isolation principles
3. REST API calls - rejected as NATS was specifically requested by user

## Security and Authorization

### Decision: Scoper Interface Integration
**Rationale**: The constitution and user specification require using Scoper interface for authorization. This provides consistent security patterns across the system.

**Implementation**:
- Implement Scope() method returning required permissions for admin operations
- Use interceptor.authorize() to check permissions
- Follow existing pattern from verification service

### Alternatives considered:
1. Custom role checking - rejected as it would duplicate existing functionality
2. JWT claims only - rejected as it lacks granular permissions

## Assignment State Management

### Decision: State Machine for Assignment Lifecycle
**Rationale**: The user specified different states for assignments. Implementing a state machine ensures consistent transitions and prevents invalid state changes.

**States**:
- Pending: Assignment created but not yet delivered to game
- Delivered: Assignment sent to game system
- Claimed: User has claimed the kit in game

### Alternatives considered:
1. Simple boolean flags - rejected as it doesn't properly represent the lifecycle
2. Timestamp-based states - rejected as it's less clear and harder to validate

## Repository Pattern Implementation

### Decision: Repository Interface with MongoDB Implementation
**Rationale**: Following existing patterns in the codebase where repository interfaces are defined in service packages and implementations in repository packages.

**Interface**:
```
type Repository interface {
    CreateAssignment(ctx context.Context, assignment *Assignment) error
    GetAssignment(ctx context.Context, assignmentID string) (*Assignment, error)
    UpdateAssignment(
        ctx context.Context,
        assignmentID string,
        updateFn func(ctx context.Context, assignment *Assignment) (*Assignment, error),
    ) error
    GetKits(ctx context.Context) ([]*Kit, error)
    GetKitByName(ctx context.Context, name string) (*Kit, error)
}
```

### Alternatives considered:
1. Direct MongoDB access from service - rejected as it violates repository pattern in constitution
2. Generic repository - rejected as it doesn't provide domain-specific methods