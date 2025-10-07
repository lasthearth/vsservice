---
description: "Task list for refactoring settlement submit business logic"
---

# Tasks: Refactor Settlement Submit Business Logic

**Input**: Design documents from `/specs/001-submit-settlement/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/
**Tests**: Tests are OPTIONAL - not explicitly requested in feature specification.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`
- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions
- **Single project**: `src/`, `tests/` at repository root
- **Web app**: `backend/src/`, `frontend/src/`
- **Mobile**: `api/src/`, `ios/src/` or `android/src/`
- Paths shown below assume single project - adjust based on plan.md structure

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [X] T001 [P] Create project structure per implementation plan in internal/settlement/
- [X] T002 [P] Review existing settlement service architecture in internal/settlement/internal/service/service.go
- [X] T003 [P] Analyze current repository implementation in internal/settlement/internal/repository/mongo/verification.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

Examples of foundational tasks (adjust based on your project):

- [X] T004 Identify business logic in repository Submit method that needs to be moved to service layer
- [X] T005 [P] Update repository interface to remove business logic responsibilities
- [X] T006 [P] Plan service layer methods to handle business validation and rules
- [X] T007 [P] Review error handling patterns to maintain consistency during refactoring
- [X] T008 [P] Document data flow and transformation layers to ensure they remain intact

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Refactor Submit Business Logic (Priority: P1) 🎯 MVP

**Goal**: Move business logic from repository Submit method to service layer while maintaining all functionality

**Independent Test**: Can be fully tested by submitting a settlement request with valid data and verifying it appears in the pending verification queue, delivering the value of allowing new settlements to be registered while maintaining proper separation of concerns.

### Implementation for User Story 1

- [X] T009 [P] [US1] Move user membership validation logic from repository to service layer in internal/settlement/internal/service/service.go
- [X] T010 [P] [US1] Move settlement type progression logic from repository to service layer in internal/settlement/internal/service/service.go
- [X] T011 [US1] Update service Submit method to perform user validation before calling repository (depends on T009, T010)
- [X] T012 [US1] Update repository Submit method to focus purely on data operations in internal/settlement/internal/repository/mongo/verification.go
- [X] T013 [US1] Update repository interface to reflect pure data operations in internal/settlement/internal/service/interface.go
- [X] T014 [US1] Add error handling for business validation failures in service layer
- [X] T015 [US1] Update logging to reflect new architecture in both service and repository layers
- [X] T016 [US1] Test refactored Submit functionality to ensure all business rules still work correctly

**Checkpoint**: At this point, User Story 1 should be fully functional with business logic properly separated from data access

---

## Phase 4: User Story 2 - Process Settlement Attachments (Priority: P2)

**Goal**: Ensure attachment processing remains intact and properly integrated with refactored business logic

**Independent Test**: Can be fully tested by submitting settlement with various attachment types and sizes, verifying they are properly stored and accessible, delivering the value of enabling visual verification.

### Implementation for User Story 2

- [X] T017 [P] [US2] Verify attachment processing logic remains in service layer in internal/settlement/internal/service/service.go
- [X] T018 [US2] Update attachment processing to work with refactored business validation
- [X] T019 [US2] Ensure attachment validation occurs before repository operations
- [X] T020 [US2] Test attachment handling with the refactored business logic

**Checkpoint**: At this point, User Stories 1 AND 2 should both work independently

---

## Phase 5: User Story 3 - Validate Settlement Submission Data (Priority: P3)

**Goal**: Ensure all validation logic is properly placed in the service layer according to architecture principles

**Independent Test**: Can be fully tested by attempting to submit settlements with various invalid data types, verifying the system properly rejects them with clear error messages, delivering the value of maintaining data quality.

### Implementation for User Story 3

- [X] T021 [P] [US3] Identify all validation logic that should remain in service layer
- [X] T022 [US3] Ensure validation occurs before repository operations
- [X] T023 [US3] Test validation edge cases with refactored architecture
- [X] T024 [US3] Verify error propagation works correctly from service to gRPC layer

**Checkpoint**: All user stories should now be independently functional

---

## Phase N: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [X] T025 [P] Update documentation in specs/001-submit-settlement/ to reflect refactored architecture
- [X] T026 [P] Code cleanup and refactoring after core changes are tested
- [X] T027 [P] Verify all error handling paths work correctly
- [X] T028 [P] Run quickstart.md validation
- [X] T029 [P] Update any README files to reflect new architecture

---

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
- **User Story 2 (P2)**: Depends on User Story 1 completion - attachment processing must work with refactored business logic
- **User Story 3 (P3)**: Depends on User Story 1 completion - validation must work with refactored architecture

### Within Each User Story

- Models before services (if needed)
- Services before endpoints
- Core implementation before integration
- Story complete before moving to next priority
- T009-T016 must be completed for US1 to function independently

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel
- All Foundational tasks marked [P] can run in parallel (within Phase 2)
- Once Foundational phase completes:
  - T009 and T010 can run in parallel in US1
  - T017 and T018 can run in parallel in US2
  - T021 and T022 can run in parallel in US3

### Parallel Example: User Story 1

```bash
# Launch all setup tasks together:
Task: "Review existing settlement service architecture in internal/settlement/internal/service/service.go"
Task: "Analyze current repository implementation in internal/settlement/internal/repository/mongo/verification.go"

# Launch all US1 tasks that can run in parallel:
Task: "Move user membership validation logic from repository to service layer in internal/settlement/internal/service/service.go"
Task: "Move settlement type progression logic from repository to service layer in internal/settlement/internal/service/service.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1
4. **STOP and TEST**: Verify business logic is properly separated and all functionality works
5. Deploy/demo if ready

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
   - Developer A: User Story 1 (T009-T016)
   - Developer B: User Story 2 (T017-T020) - only after US1 is complete
   - Developer C: User Story 3 (T021-T024) - only after US1 is complete

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Dependencies: US2 and US3 depend on US1 completion
- Avoid: vague tasks, same file conflicts, cross-story dependencies that break independence