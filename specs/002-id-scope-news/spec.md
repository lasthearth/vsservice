# Feature Specification: Add News Deletion by ID with Scope Authorization

**Feature Branch**: `002-id-scope-news`  
**Created**: 2025-10-07  
**Status**: Draft  
**Input**: User description: "Добавить удаление новости, по ее id. Удаление возможно только при наличии scope: news:delete"

## Clarifications

### Session 2025-10-07

- Q: What is the specific authentication method used for validating user tokens? → A: get from context, use interceptor pkg, example can be found in settlements
- Q: What specific error response format should be used when deletion fails? → A: gRPC error format with status codes and message
- Q: What is the expected maximum time for deleting a single news item under normal conditions? → A: 2 seconds - as currently specified
- Q: For concurrent deletion scenario, what should be the system behavior? → A: Allow all requests to proceed but only first one actually deletes
- Q: What happens to associated data when a news item is deleted? → A: Only delete the news item, leave associated data intact

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Delete News by ID (Priority: P1)

As an authorized user with proper permissions, I want to delete a specific news item by providing its unique identifier, so that outdated or incorrect news can be removed from the system.

**Why this priority**: This is the core functionality that enables content management by authorized users. Without this capability, the news system cannot be properly maintained.

**Independent Test**: Can be fully tested by attempting to delete a news item with valid ID and proper authorization, verifying it is no longer accessible, delivering the value of managing news content lifecycle.

**Acceptance Scenarios**:

1. **Given** user has valid authentication token with news:delete scope, **When** user requests to delete a news item with valid ID that exists, **Then** the news item is successfully deleted and no longer accessible
2. **Given** user has valid authentication token with news:delete scope, **When** user requests to delete a news item with invalid or non-existent ID, **Then** the system returns an appropriate error message

---

### User Story 2 - Unauthorized Deletion Prevention (Priority: P2)

As a security measure, I want to ensure that only users with the appropriate scope can delete news items, preventing unauthorized content modification.

**Why this priority**: This is crucial for maintaining content integrity and preventing unauthorized users from removing news items.

**Independent Test**: Can be fully tested by attempting to delete news with and without proper authorization, verifying that only authorized users can perform the operation, delivering the value of maintaining content security.

**Acceptance Scenarios**:

1. **Given** user has valid authentication but lacks news:delete scope, **When** user requests to delete a news item, **Then** the system returns an unauthorized access error
2. **Given** user has no authentication token, **When** user requests to delete a news item, **Then** the system returns an authentication required error

---

### User Story 3 - Confirmation and Feedback (Priority: P3)

As a user performing deletion, I want clear feedback about the success or failure of my deletion request, so I know the current state of the system.

**Why this priority**: Proper feedback prevents confusion and helps users understand the outcome of their actions.

**Independent Test**: Can be fully tested by performing deletion operations and verifying appropriate responses are returned, delivering the value of clear user experience.

**Acceptance Scenarios**:

1. **Given** user has proper authorization and attempts deletion, **When** deletion is successful, **Then** the system returns a success confirmation
2. **Given** user has proper authorization and deletion fails, **When** deletion attempt is made, **Then** the system returns a clear error message

---

### Edge Cases

- What happens when multiple users try to delete the same news item simultaneously?
- How does system handle requests to delete news items that are already deleted?
- What happens if system experiences failure during the deletion process?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide an endpoint to delete a news item by its unique identifier
- **FR-002**: System MUST verify user has news:delete scope before allowing deletion
- **FR-003**: System MUST authenticate user token using interceptor package following settlement service pattern
- **FR-004**: System MUST return gRPC error format with status codes and message for failures
- **FR-005**: System MUST return appropriate error response when news item does not exist
- **FR-006**: System MUST return unauthorized error when user lacks required scope
- **FR-007**: System MUST check if requested news exists before attempting deletion
- **FR-008**: System MUST permanently remove news item from data storage upon successful deletion, leaving associated data intact
- **FR-009**: System MUST prevent deletion attempts by unauthenticated users
- **FR-010**: System MUST handle concurrent deletion attempts by allowing all requests to proceed but only first one actually deletes, subsequent attempts return not found error

### Key Entities

- **News**: Content item with unique identifier, title, content, and metadata that can be deleted
- **User**: Authenticated entity with specific scopes that determine permissions
- **Scope**: Authorization token permission that determines access rights (specifically news:delete)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Authorized users can successfully delete news items within 2 seconds of request
- **SC-002**: System properly rejects 100% of deletion requests from users without news:delete scope
- **SC-003**: System returns appropriate error for 100% of requests to delete non-existent news items
- **SC-004**: Users receive clear confirmation for 95% of successful deletion operations
- **SC-005**: System maintains 99% uptime during news deletion operations
- **SC-006**: Unauthorized access attempts are properly blocked 100% of the time