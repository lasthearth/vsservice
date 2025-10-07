# Feature Specification: Refactor Settlement Submit Business Logic

**Feature Branch**: `001-submit-settlement`  
**Created**: 2025-10-07  
**Status**: Draft  
**Input**: User description: "Бизнес логика в Submit в Settlement улетела в репозиторий, нужно отрефакторить согласно конституции"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Submit New Settlement Request (Priority: P1)

As a user, I want to submit a new settlement registration request with all required information (name, type, description, diplomacy, coordinates, and attachments) so that my settlement can be reviewed and potentially approved by administrators.

**Why this priority**: This is the core functionality of the settlements feature - without the ability to submit new settlements, the entire system becomes unusable.

**Independent Test**: Can be fully tested by submitting a settlement request with valid data and verifying it appears in the pending verification queue, delivering the value of allowing new settlements to be registered.

**Acceptance Scenarios**:

1. **Given** user is authenticated with valid token, **When** user submits a complete settlement request with all required fields, **Then** the request is successfully created and placed in the verification queue
2. **Given** user is authenticated with valid token, **When** user submits an incomplete settlement request missing required fields, **Then** the system returns a validation error with specific details about missing fields

---

### User Story 2 - Process Settlement Attachments (Priority: P2)

As a user, I want to attach images and descriptions to my settlement request so that administrators can properly verify the legitimacy of the settlement with visual evidence.

**Why this priority**: Attachments are critical for settlement verification. Without proper attachment handling, the verification process cannot function correctly.

**Independent Test**: Can be fully tested by submitting settlement with various attachment types and sizes, verifying they are properly stored and accessible, delivering the value of enabling visual verification.

**Acceptance Scenarios**:

1. **Given** user has selected settlement attachments, **When** user submits settlement request with attachments, **Then** each attachment is properly converted to optimized format and stored with public access

---

### User Story 3 - Validate Settlement Submission Data (Priority: P3)

As a system, I want to validate all settlement submission data against business rules before storing it, ensuring data integrity and compliance with settlement requirements.

**Why this priority**: Proper validation prevents bad data from entering the system and reduces the workload on administrators by automatically rejecting invalid submissions.

**Independent Test**: Can be fully tested by attempting to submit settlements with various invalid data types, verifying the system properly rejects them with clear error messages, delivering the value of maintaining data quality.

**Acceptance Scenarios**:

1. **Given** user enters settlement data, **When** user submits with invalid coordinates or malformed data, **Then** the system returns appropriate validation errors

---

### Edge Cases

- What happens when a user tries to submit multiple settlements simultaneously (should be prevented if user is already a leader of another settlement)?
- How does system handle attachment uploads that fail or exceed size limits?
- What happens when user submits a settlement with same name/location as an existing one?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST validate all settlement submission data according to business rules before processing
- **FR-002**: System MUST ensure user is not already a leader or member of another settlement before allowing new submission
- **FR-003**: Users MUST be able to submit settlement requests with name, type, description, diplomacy, coordinates, and attachments
- **FR-004**: System MUST convert all submitted images to optimized format for efficient storage and delivery
- **FR-005**: System MUST store attachment files in MinIO-compatible object storage with public access for administrators and users
- **FR-006**: System MUST place valid settlement requests in a verification queue for administrator review
- **FR-007**: System MUST reject incomplete or invalid settlement submissions with clear error messages
- **FR-008**: System MUST properly separate business logic from data access concerns
- **FR-009**: System MUST maintain clear separation between business logic and data persistence layers
- **FR-010**: System MUST follow project architecture guidelines regarding validation and business logic placement
- **FR-011**: System MUST authenticate users with valid token containing user ID for settlement submission
- **FR-012**: System MUST automatically retry failed attachment uploads before failing the submission process
- **FR-013**: System MUST fail submission immediately with error message when external services are unavailable

### Key Entities

- **Settlement Request**: Represents a pending settlement awaiting approval with all required details (name, type, description, diplomacy, coordinates, attachments)
- **Settlement Attachment**: Contains image data and description for settlement verification, stored in object storage
- **User**: The authenticated user submitting the settlement request

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can successfully submit settlement requests with all required data in under 30 seconds
- **SC-002**: System maintains 99% uptime during settlement submission process
- **SC-003**: 100% of submitted images are properly converted to optimized format and stored with public access
- **SC-004**: 95% of settlement submission validation occurs with immediate feedback
- **SC-005**: System properly rejects 100% of invalid settlement submissions with clear error messages
- **SC-006**: After refactoring, the system properly separates business logic from data access as required by project architecture
- **SC-007**: System handles concurrent settlement submissions based on available resources without specific limits

## Clarifications

### Session 2025-10-07

- Q: For the settlement submission feature, which authentication level is required for users to submit settlements? → A: just uid from token no scope needed
- Q: For the settlement attachment storage, what are the specific object storage integration requirements for storing settlement attachments? → A: Store attachments in MinIO-compatible object storage with public access
- Q: For error states during settlement submission, how should the system respond when attachment uploads fail? → A: Retry attachment uploads automatically before failing
- Q: What is the expected maximum number of concurrent settlement submissions the system should handle? → A: No specific limit needed, handle based on available resources
- Q: For service dependencies (like object storage), how should the system behave when external services are temporarily unavailable? → A: Fail the submission immediately with error message