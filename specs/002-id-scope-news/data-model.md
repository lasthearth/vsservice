# Data Model: News Deletion with Scope Authorization

## Entities

### News
- **Id** (bson.ObjectID): Unique identifier for the news item
- **Title** (string): Title of the news item
- **Content** (string): Content/body of the news item
- **CreatedAt** (time.Time): Timestamp when the news was created
- **UpdatedAt** (time.Time): Timestamp when the news was last updated

### User
- **Id** (string/uuid): Unique identifier for the authenticated user
- **Scopes** ([]string): List of authorized scopes for the user (including news:delete)

### Scope
- **Name** (string): The scope name (e.g., "news:delete")
- **Description** (string): Description of what the scope permits

## Relationships

- A **User** can have multiple **Scopes**
- A **News** item can be deleted by a **User** with the appropriate **Scope** ("news:delete")

## State Transitions

For News:
- When a news item is deleted, it transitions from "active" to "deleted" state (actually removed from the database as per requirement FR-008)

## Validation Rules

- News title and content must not be empty strings
- News ID must be a valid BSON ObjectID
- User must possess the "news:delete" scope to perform deletion
- News item must exist before attempting deletion (as per FR-007)