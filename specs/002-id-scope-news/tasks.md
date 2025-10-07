# Tasks: Add News Deletion by ID with Scope Authorization

**Feature Branch**: `002-id-scope-news` | **Date**: 2025-10-07 | **Spec**: [Link to spec.md]
**Input**: Feature specification from `/specs/002-id-scope-news/spec.md`

## Implementation Strategy

### MVP First (User Story 1 Only)
1. Setup phase → Foundation ready
2. Add User Story 1 (Delete News by ID) → Test independently → Deploy/Demo (MVP!)
3. Add User Story 2 (Unauthorized Deletion Prevention) → Test independently → Deploy/Demo
4. Add User Story 3 (Confirmation and Feedback) → Test independently → Deploy/Demo
5. Each story adds value without breaking previous stories

### Incremental Delivery
1. Setup + Foundational → Foundation ready
2. Add User Story 1 → Test independently → Deploy/Demo (MVP!)
3. Add User Story 2 → Test independently → Deploy/Demo
4. Add User Story 3 → Test independently → Deploy/Demo
5. Each story adds value without breaking previous stories

### Parallel Team Strategy
With multiple developers:
1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1 (T001-T015)
   - Developer B: User Story 2 (T016-T025) - only after US1 is complete
   - Developer C: User Story 3 (T026-T035) - only after US1 is complete

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [ ] T001 [P] Create project structure per implementation plan in internal/news/
- [ ] T002 [P] Review existing news service architecture in internal/news/internal/service/service.go
- [ ] T003 [P] Analyze current repository implementation in internal/news/internal/repository/repository.go
- [ ] T004 [P] Review existing protobuf definition in proto/news/v1/news.proto

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T005 Add DeleteNews RPC method to protobuf definition in proto/news/v1/news.proto
- [ ] T006 Generate gRPC code from updated protobuf definition using buf generate
- [ ] T007 [P] Create DeleteNewsRequest and DeleteNewsResponse message types in proto/news/v1/news.proto
- [ ] T008 [P] Add HTTP annotations for REST access to DeleteNews RPC in proto/news/v1/news.proto
- [ ] T009 Update DTO mappings with goverter for new protobuf messages
- [ ] T010 Create initial scope.go file for news service authorization in internal/news/internal/service/scope.go
- [ ] T011 Register news service as Scoper in fx.go in internal/news/fx.go

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

## Phase 3: User Story 1 - Delete News by ID (Priority: P1) 🎯 MVP

**Goal**: As an authorized user with proper permissions, I want to delete a specific news item by providing its unique identifier, so that outdated or incorrect news can be removed from the system.

**Independent Test**: Can be fully tested by attempting to delete a news item with valid ID and proper authorization, verifying it is no longer accessible, delivering the value of managing news content lifecycle.

### Implementation for User Story 1

- [ ] T012 [P] [US1] Implement DeleteNews method in service.go in internal/news/internal/service/service.go
- [ ] T013 [P] [US1] Complete DeleteNews method implementation in repository.go in internal/news/internal/repository/repository.go
- [ ] T014 [US1] Add validation to check if news item exists before deletion in service layer
- [ ] T015 [US1] Implement proper gRPC error response for "not found" cases in service layer

**Checkpoint**: At this point, User Story 1 should be fully functional with basic deletion capability

## Phase 4: User Story 2 - Unauthorized Deletion Prevention (Priority: P2)

**Goal**: As a security measure, I want to ensure that only users with the appropriate scope can delete news items, preventing unauthorized content modification.

**Independent Test**: Can be fully tested by attempting to delete news with and without proper authorization, verifying that only authorized users can perform the operation, delivering the value of maintaining content security.

### Implementation for User Story 2

- [ ] T016 [P] [US2] Define "news:delete" scope for DeleteNews method in scope.go in internal/news/internal/service/scope.go
- [ ] T017 [P] [US2] Implement scope validation in service layer to check for "news:delete" permission
- [ ] T018 [US2] Add proper error handling for unauthorized access (gRPC PermissionDenied error)
- [ ] T019 [US2] Test scope validation with users having and lacking "news:delete" scope

**Checkpoint**: At this point, User Stories 1 AND 2 should both work independently

## Phase 5: User Story 3 - Confirmation and Feedback (Priority: P3)

**Goal**: As a user performing deletion, I want clear feedback about the success or failure of my deletion request, so I know the current state of the system.

**Independent Test**: Can be fully tested by performing deletion operations and verifying appropriate responses are returned, delivering the value of clear user experience.

### Implementation for User Story 3

- [ ] T020 [P] [US3] Implement success response for successful deletion operations
- [ ] T021 [P] [US3] Add proper error handling for all failure scenarios (gRPC error format)
- [ ] T022 [US3] Implement validation for invalid/non-existent news IDs with clear error messages
- [ ] T023 [US3] Test concurrent deletion handling (first request succeeds, subsequent return "not found")

**Checkpoint**: All user stories should now be independently functional

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T024 [P] Update documentation in specs/002-id-scope-news/ to reflect implemented architecture
- [ ] T025 [P] Add protobuf documentation in Russian language for news service
- [ ] T026 [P] Code cleanup and refactoring after core changes are tested
- [ ] T027 [P] Verify all error handling paths work correctly
- [ ] T028 [P] Run quickstart.md validation
- [ ] T029 [P] Update any README files to reflect new architecture
- [ ] T030 [P] Add proper logging for deletion operations with zap logger

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3+)**: All depend on Foundational phase completion
  - User stories can then proceed in parallel (if staffed)
  - Or sequentially in priority order (P1 → P2 → P3)
- **Polish (Final Phase)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) - No dependencies on other stories
- **User Story 2 (P2)**: Depends on User Story 1 completion - scope validation must work with deletion functionality
- **User Story 3 (P3)**: Depends on User Story 1 completion - feedback mechanisms must work with deletion operations

### Within Each User Story

- Models before services (if needed)
- Services before endpoints
- Core implementation before integration
- Story complete before moving to next priority
- Tasks within each story should be completed in sequence unless marked [P]

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel
- All Foundational tasks marked [P] can run in parallel (within Phase 2)
- Once Foundational phase completes:
  - T012 and T013 can run in parallel in US1
  - T016 and T017 can run in parallel in US2
  - T020 and T021 can run in parallel in US3

### Parallel Example: User Story 1

```bash
# Launch all setup tasks together:
Task: "Review existing news service architecture in internal/news/internal/service/service.go"
Task: "Analyze current repository implementation in internal/news/internal/repository/repository.go"

# Launch all US1 tasks that can run in parallel:
Task: "Implement DeleteNews method in service.go in internal/news/internal/service/service.go"
Task: "Complete DeleteNews method implementation in repository.go in internal/news/internal/repository/repository.go"
```

## Validation & Testing

### User Story 1 Validation
- [ ] T031 Given user has valid authentication token with news:delete scope, when user requests to delete a news item with valid ID that exists, then the news item is successfully deleted and no longer accessible
- [ ] T032 Given user has valid authentication token with news:delete scope, when user requests to delete a news item with invalid or non-existent ID, then the system returns an appropriate error message

### User Story 2 Validation
- [ ] T033 Given user has valid authentication but lacks news:delete scope, when user requests to delete a news item, then the system returns an unauthorized access error
- [ ] T034 Given user has no authentication token, when user requests to delete a news item, then the system returns an authentication required error

### User Story 3 Validation
- [ ] T035 Given user has proper authorization and attempts deletion, when deletion is successful, then the system returns a success confirmation
- [ ] T036 Given user has proper authorization and deletion fails, when deletion attempt is made, then the system returns a clear error message

## Implementation Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Dependencies: US2 and US3 depend on US1 completion
- Avoid: vague tasks, same file conflicts, cross-story dependencies that break independence

**Total Tasks Generated**: 36
**Tasks Per User Story**: 
- User Story 1: 4 implementation tasks + 2 validation tasks = 6 tasks
- User Story 2: 4 implementation tasks + 2 validation tasks = 6 tasks
- User Story 3: 4 implementation tasks + 2 validation tasks = 6 tasks
- Setup/Foundation: 11 tasks
- Polish/Cross-cutting: 6 tasks
- Validation: 6 tasks