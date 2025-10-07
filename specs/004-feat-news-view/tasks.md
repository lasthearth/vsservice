# Tasks: News View Counter

**Feature**: News View Counter  
**Branch**: `004-feat-news-view`  
**Generated**: 2025-10-07  
**Input**: Implementation plan from `plan.md`, feature specification from `spec.md`

**Overview**: Implementation of a news view counter that increments each time a user views a news article, with atomic operations for thread safety and persistence in MongoDB.

## Task Execution Strategy

This plan implements an MVP-first approach where User Story 1 (displaying view count) and User Story 2 (incrementing view count) are developed together as they are fundamentally linked. User Story 3 (persistence) is addressed throughout the implementation.

**MVP Scope**: Tasks T001-T011 complete the core functionality allowing users to see and increment view counts (US1 and US2).

**Implementation Phases**:
- Phase 1: Project setup and foundational components
- Phase 2: Core data model and repository layer
- Phase 3: Service layer with increment logic
- Phase 4: gRPC API and integration
- Phase 5: Final testing and polish

---

## Phase 1: Setup Tasks

### Setup and foundational infrastructure needed by all user stories

**T001**: [X] Update protobuf definition for news service to include view_count field  
**File**: `proto/news/news.proto`  
**Story**: [US1][US2]  
**Description**: Add view_count field to GetNewsResponse message in the protobuf definition to support view counter functionality  

**T002**: [X] Update news model to include view_count field  
**File**: `internal/news/model/news.go`  
**Story**: [US1][US2]  
**Description**: Add ViewCount field to the News struct to store the view counter value  

**T003**: [X] Update news DTO to include view_count field  
**File**: `internal/news/internal/dto/news_dto.go`  
**Story**: [US1][US2]  
**Description**: Add ViewCount field to the MongoDB DTO to persist the view counter value  

---

## Phase 2: Foundational Tasks

### Blocking prerequisites that must complete before user stories can be implemented

**T004**: [X] Create MongoDB repository method for incrementing view count  
**File**: `internal/news/internal/repository/mongo/news_repository.go`  
**Story**: [US1][US2]  
**Description**: Implement atomic increment operation using MongoDB's $inc operator to safely update view counts in concurrent scenarios  

**T005**: [X] Create repository method for retrieving news with view count  
**File**: `internal/news/internal/repository/mongo/news_repository.go`  
**Story**: [US1][US2]  
**Description**: Implement method to retrieve news articles with current view count for display  

---

## Phase 3: User Story 1 - News View Counter Display (Priority: P1)

**Goal**: As a user, when I view a news article, I want to see how many times this article has been viewed by other users so that I can understand its popularity and relevance.

**Independent Test**: Can be fully tested by viewing a news article and verifying that the view count is displayed. The feature delivers value by providing users with information about news popularity.

**Acceptance**:
1. Given a user navigates to a news article page, When the page loads, Then the view count for that specific news article is displayed prominently on the page.
2. Given a news article exists with an initial view count of 0, When a user views the article, Then the view count is incremented by 1 and displayed to the user.

**T006**: [X] Update service interface to include view count in GetNews response  
**File**: `internal/news/service/interface.go` (or equivalent service interface)  
**Story**: [US1]  
**Description**: Update the news service interface to include view count in the response when retrieving news articles  

**T007**: [X] Update GetNews service method to return view count  
**File**: `internal/news/service/news_service.go`  
**Story**: [US1]  
**Description**: Ensure the GetNews method returns the current view count as part of the response  

---

## Phase 4: User Story 2 - View Count Increment on News View (Priority: P1)

**Goal**: As a system, when a user views a news article, I need to increment the view counter for that specific news article by exactly 1.

**Independent Test**: Can be fully tested by simulating a news article view and verifying that the view count for that article increases by exactly 1. The feature delivers value by providing accurate popularity metrics.

**Acceptance**:
1. Given a news article with a current view count of N, When a user views the article, Then the view count is incremented to N+1.
2. Given a user views a news article for the first time, When the view is recorded, Then the view count is incremented by 1 regardless of the user's previous viewing history.

**T008**: [X] Update service interface to include view increment functionality  
**File**: `internal/news/service/interface.go` (or equivalent service interface)  
**Story**: [US2]  
**Description**: Define method in the service interface to increment view count when an article is viewed  

**T009**: [X] Update GetNews service method to increment view count atomically  
**File**: `internal/news/service/news_service.go`  
**Story**: [US2]  
**Description**: Modify GetNews to first increment the view count (via repository) before returning the article data to ensure thread-safe counting  

**T010**: [X] Generate gRPC code from updated protobuf  
**File**: `gen/proto/news/news_grpc.pb.go`  
**Story**: [US1][US2]  
**Description**: Run buf generate to regenerate the gRPC code based on the updated protobuf definition  

**T011**: [X] Update gRPC handler to return view count in response  
**File**: `internal/news/internal/handler/news_handler.go`  
**Story**: [US1][US2]  
**Description**: Update the gRPC handler to ensure the view count is properly returned in GetNews responses  

---

## Phase 5: User Story 3 - View Count Persistence (Priority: P2)

**Goal**: As a system, I need to ensure that news view counts are reliably stored and maintained even when the system restarts or experiences failures.

**Independent Test**: Can be tested by performing system restarts or failure scenarios and verifying that view counts remain accurate and persistent.

**Acceptance**:
1. Given a news article has accumulated view counts, When the system restarts, Then the view counts are preserved and remain accurate.

**T012**: [X] Add error handling and logging for view count increment failures  
**File**: `internal/news/service/news_service.go`  
**Story**: [US3]  
**Description**: Implement proper error handling and logging for cases where view count increment fails, ensuring the core functionality remains available  

**T013**: [X] Update data validation to ensure view count integrity  
**File**: `internal/news/model/news.go`  
**Story**: [US3]  
**Description**: Add validation to ensure view count cannot be negative and maintains integrity  

---

## Phase 6: Polish & Cross-Cutting Concerns

**T014**: [P] Write unit tests for news service with view count functionality  
**File**: `internal/news/service/news_service_test.go`  
**Story**: [US1][US2][US3]  
**Description**: Create comprehensive unit tests for the news service, specifically testing the view count increment and retrieval functionality  

**T015**: [P] Write integration tests for view count operations  
**File**: `tests/integration/news/news_repository_test.go`  
**Story**: [US1][US2][US3]  
**Description**: Create integration tests to verify that the view count increment works correctly with the database, including concurrent access scenarios  

**T016**: [P] Update documentation for view counter feature  
**File**: `README.md` (or docs)  
**Story**: [US1][US2][US3]  
**Description**: Update documentation to include information about the new view counter functionality and how it works  

---

## Dependencies

**User Story Completion Order**:
- US1 (Display) depends on: T001, T002, T003, T004, T005, T006, T007
- US2 (Increment) depends on: T001, T002, T003, T004, T005, T008, T009, T010, T011
- US3 (Persistence) depends on: T004, T012, T013

**Since US1 and US2 are fundamentally linked, they should be completed together. US3 is addressed throughout the implementation.**

---

## Parallel Execution Examples

**For User Story 1 (Display)**:
- Tasks T006 and T007 can be developed in parallel since they're in different files
- T002 and T003 can be done in parallel (model and DTO updates)

**For User Story 2 (Increment)**:
- T008 and T011 can be developed in parallel (service interface and handler)
- T009 and T010 can be done together but are technically separate concerns

**Overall**:
- T001, T002, T003 can be done in parallel (proto, model, DTO updates)
- T004, T005 can be done in parallel (repository methods)

---

## Implementation Strategy

**MVP (Minimum Viable Product)**: Tasks T001-T011 deliver the complete view counter functionality allowing users to both view and increment view counts. This implements the core requirements of User Stories 1 and 2.

**Incremental Delivery**: 
- After Phase 2: Foundation is established with data model and repository
- After Phase 3 & 4: Complete functionality with display and increment working
- After Phase 5: Persistence with proper error handling
- After Phase 6: Full test coverage and documentation

This approach ensures that the core functionality is delivered quickly while maintaining code quality and test coverage.