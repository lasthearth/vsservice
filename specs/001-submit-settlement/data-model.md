# Data Model: Refactor Settlement Submit Business Logic

## Entities

### SettlementVerification
Represents a pending settlement awaiting approval with all required details (name, type, description, diplomacy, coordinates, attachments)

- **Id**: string - Unique identifier for the settlement verification request
- **Name**: string - Name of the settlement
- **Type**: SettlementType - Type of settlement (Camp, Village, City, Province, Guild, GuildLvl2, Orden)
- **Leader**: Member - The user who is the leader of this settlement
- **Coordinates**: Vector2 - X,Y coordinates of the settlement location
- **Attachments**: []Attachment - Collection of attachments for verification
- **Diplomacy**: string - Diplomatic relations information
- **Description**: string - Description of the settlement
- **Status**: SettlementStatus - Current status (Pending, Approved, Rejected, UpdateRejected)
- **RejectionReason**: string - Reason for rejection if status is Rejected
- **UpdatedAt**: time.Time - Timestamp of last update
- **CreatedAt**: time.Time - Timestamp of creation

**Business Methods:**
- **LvlUp()**: Progresses the settlement type to the next level in the hierarchy (camp → village → city → province, guild → guildLvl2)

**Validation Rules:**
- Name, Type, Leader, Coordinates, and Description are required
- Attachments must not be empty for new submissions
- User must not already be a leader or member of another settlement

### Member
Represents a user who is part of a settlement

- **UserId**: string - Unique identifier for the user

### Vector2
Represents 2D coordinates

- **X**: int - X coordinate
- **Y**: int - Y coordinate

### Attachment
Contains image data and description for settlement verification

- **Url**: string - Public URL to access the attachment
- **Desc**: string - Description of the attachment
- **MimeType**: string - MIME type of the attachment

### SettlementOpts
Data transfer object containing all information needed to submit or update a settlement request

- **Name**: string - Name of the settlement
- **Type**: SettlementType - Type of settlement
- **Leader**: Member - The settlement leader
- **Description**: string - Description of the settlement
- **Diplomacy**: string - Diplomatic relations information
- **Coordinates**: Vector2 - Location coordinates
- **Attachments**: []Attachment - Collection of attachments

## Relationships

- SettlementVerification has one Leader (Member)
- SettlementVerification has many Attachments
- SettlementVerification has one set of Coordinates (Vector2)

## State Transitions

### SettlementStatus Transitions
- **Pending**: Initial state when request is submitted
- **Approved**: Set when administrators approve the request
- **Rejected**: Set when administrators reject the request
- **UpdateRejected**: Set when a higher-level settlement request is rejected

### LvlUp() Business Logic
- Camp → Village (when approved)
- Village → City (when approved)
- City → Province (when approved) 
- Guild → GuildLvl2 (when approved)
- GuildLvl2, Province, and Orden do not level up further

## Business Rules

### Submission Rules
1. A user cannot submit multiple settlements simultaneously - must not be leader or member of existing settlement
2. Settlement requests must include required attachments with descriptions
3. Settlement name and location must be unique (not implemented in current scope)
4. Only one pending request per user is allowed

### Validation Requirements
1. All required fields must be present in the submission
2. User must be authenticated and have valid token
3. Attachments must be successfully processed and stored
4. User must not violate membership rules (not already in settlement)