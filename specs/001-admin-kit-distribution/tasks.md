# Tasks: Admin Kit Distribution

**Feature**: Admin Kit Distribution | **Branch**: 001-admin-kit-distribution | **Date**: November 17, 2025

## Implementation Strategy

**MVP Scope**: Implement User Story 1 (Admin Assigns Kit to User) with basic kit assignment functionality, minimal UI, and core backend services. This delivers the core value of allowing administrators to assign kits to users.

**Delivery Approach**: Incremental delivery following user story priorities (P1 → P2 → P3). Each user story is independently testable and provides business value when completed.

**Parallel Execution**: Tasks with [P] label can be executed in parallel with other tasks if they operate on different files/modules and have no dependencies on other uncompleted tasks.

## Dependencies

User stories are prioritized in order of business importance. User Story 1 (P1) must be completed before User Story 2 (P2) for proper foundational features. User Story 3 (P3) can be implemented after Story 1 or 2 but may benefit from the foundational work completed during those phases.

**Story Completion Order**: US1 → US2 → US3

## Parallel Execution Examples

- **US1 Parallel Tasks**: 
  - T015 [P] [US1] and T016 [P] [US1] can be developed concurrently after foundational work
  - T017 [P] [US1] and T020 [P] [US1] can be developed after models and repositories exist

## Phase 1: Project Setup

**Goal**: Set up the project structure for the kit service following existing architecture patterns.

- [X] T001 Create internal/kit directory and subdirectories per implementation plan
- [X] T002 Create internal/kit/fx.go with fx module configuration
- [X] T003 Create protobuf definition for kit service API
- [X] T004 Generate gRPC code from protobuf definitions
- [X] T005 Set up NATS connection configuration in config
- [ ] T006 Define base MongoDB collections for kits and assignments

## Phase 2: Foundational Components

**Goal**: Create shared components that will be used by all user stories.

- [X] T007 Create Kit domain model in internal/kit/internal/model/kit.go with validation methods
- [X] T008 Create KitAssignment domain model in internal/kit/internal/model/assignment.go with state transition methods
- [X] T009 Define AssignmentStatus enum in internal/kit/internal/model/status.go
- [X] T010 Create repository interface in internal/kit/internal/service/interface.go
- [X] T011 Create DTOs for MongoDB in internal/kit/internal/dto/mongo/kit.go and assignment.go
- [X] T012 Create DTOs for HTTP in internal/kit/internal/dto/http/kit.go and assignment.go
- [X] T013 Create goverter mappers for DTO conversions
- [X] T014 Implement Scoper interface for authorization in internal/kit/internal/service/scope.go

## Phase 3: User Story 1 - Admin Assigns Kit to User

**Goal**: Enable administrators to assign a specific game kit to a user in the Vintage Story game mode, so that the user can access special items or tools to enhance their gameplay experience.

**Independent Test Criteria**: Can be fully tested by having an admin assign a kit to a user via the API and verifying that the user can access the assigned kit in the Vintage Story game mode.

**Acceptance Scenarios**:
1. Given an administrator is authenticated with proper permissions, When they request to assign a kit to a specific user via the API, Then the system assigns the kit to that user profile
2. Given a user has been assigned a kit by an admin, When the user joins the Vintage Story game mode, Then the assigned kit becomes available for use in the game

- [X] T015 [P] [US1] Create MongoDB repository for kits in internal/kit/internal/repository/kit/mongo.go
- [X] T016 [P] [US1] Create MongoDB repository for assignments in internal/kit/internal/repository/assignment/mongo.go
- [X] T017 [US1] Create gRPC service implementation in internal/kit/internal/service/service.go
- [X] T018 [US1] Implement AssignKitToUser endpoint in the gRPC service
- [X] T019 [US1] Add validation logic to Kit and KitAssignment models for US1 requirements
- [X] T020 [P] [US1] Create NATS event publisher for kit assignments in internal/kit/internal/event/publisher.go
- [X] T021 [US1] Integrate NATS publisher with kit assignment to send KitAssignmentRequestedEvent
- [ ] T022 [US1] Register kit service with gRPC server
- [ ] T023 [US1] Test end-to-end functionality for admin assigning kit to user

## Phase 4: User Story 2 - Admin Selects Kit from Available Pool

**Goal**: Enable administrators to select from a list of available game kits in the Vintage Story game mode when assigning them to users, providing better administrative control and user experience.

**Independent Test Criteria**: Can be tested by providing an admin with a list of available kits and verifying they can select and assign one to a user.

**Acceptance Scenarios**:
1. Given an administrator accesses the kit assignment interface, When they view the list of available kits, Then a comprehensive list of Vintage Story game mode kits is displayed with descriptions

- [X] T024 [US2] Implement GetAvailableKits endpoint in the gRPC service
- [X] T025 [US2] Add filtering to repository to get only active kits
- [X] T026 [US2] Create initial seed data for default kits
- [X] T027 [US2] Update AssignKitToUser to validate kit exists and is active
- [ ] T028 [US2] Test functionality for admin selecting kit from available pool

## Phase 5: User Story 3 - User Receives Kit Notification

**Goal**: Provide notifications to users when an administrator assigns a new kit to them, ensuring users are aware of additional resources available in the Vintage Story game mode.

**Independent Test Criteria**: Can be tested by assigning a kit to a user and verifying they receive appropriate notification.

**Acceptance Scenarios**:
1. Given an administrator assigns a kit to a user, When the assignment is processed, Then the user receives a notification about the new kit availability in the game

- [X] T029 [US3] Implement NATS subscriber to listen for KitAssignmentDeliveredEvent and KitAssignmentClaimedEvent
- [X] T030 [US3] Add notification functionality to send in-game notification when kit is delivered
- [X] T031 [US3] Update assignment status when delivered/claimed events are received from game
- [ ] T032 [US3] Test user notification flow when assigned a kit

## Phase 6: Polish & Cross-Cutting Concerns

**Goal**: Complete the implementation with proper error handling, logging, and edge case management.

- [X] T033 Add comprehensive logging to service operations using zap logger
- [X] T034 Implement error handling for edge cases from specification
- [X] T035 Add validation for all user inputs and API parameters
- [X] T036 Implement rate limiting for kit assignment endpoints
- [X] T037 Add metrics collection for kit assignment operations
- [ ] T038 Write comprehensive unit tests for all components
- [ ] T039 Write integration tests for the full flow
- [X] T040 Update documentation and API comments