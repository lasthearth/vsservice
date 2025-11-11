# Data Model: Settlement Tagging System

## Tag Entity

**Definition**: Represents a label that can be applied to settlements for categorization and filtering

**Fields**:
- `id` (string): Unique identifier for the tag (MongoDB ObjectID as string)
- `name` (string): Display name of the tag (alphanumeric, hyphens, underscores, max 50 chars, unique across system)
- `color` (string): Color code for visual representation (optional, hex format)
- `description` (string): Brief description of what the tag represents (optional, max 200 chars)
- `createdAt` (timestamp): When the tag was created
- `updatedAt` (timestamp): When the tag was last modified
- `isActive` (bool): Whether the tag is currently in use (for soft deletion)

**Constraints**:
- `name` field has a unique index across the entire system
- `isActive` field defaults to true, used for soft deletion
- Maximum tag name length of 50 characters
- Color format validation (hex format: #RRGGBB or #RGB)

## Settlement Entity Extension

**Existing Fields** (from original settlement model):
- `id` (string): Unique identifier for the settlement
- `name` (string): Name of the settlement
- [other existing fields]

**New Fields** (for tagging support):
- `tagIds` (array of strings): List of tag IDs associated with this settlement
- `tags` (virtual): Computed field containing the full tag objects (not stored in DB, populated on read)

## TaggedSettlement Relationship

**Implicit Relationship**: Through the `tagIds` array field in the Settlement document

**Constraints**:
- A settlement can have up to 20 tags (per feature spec requirement)
- Each tag ID in the array must reference a valid and active Tag document
- No duplicate tag IDs in the same settlement

## Data Validation Rules

**Tag Creation/Update**:
- Name must be 1-50 characters
- Name contains only alphanumeric characters, hyphens, and underscores
- Name must be unique across all tags (enforced by unique index)
- Color (if provided) follows hex format (#RRGGBB or #RGB)
- Description (if provided) is 1-200 characters

**Settlement Tagging**:
- A settlement cannot have more than 20 tags
- Each tag ID must exist in the Tag collection and be active
- No duplicate tag IDs per settlement
- If a tag is soft-deleted (isActive=false), it remains in settlements but is not shown in tag lists

## Indexes

**Tag Collection**:
- Primary: `_id`
- Secondary: `name` (unique index to ensure tag name uniqueness)
- Secondary: `isActive` (index to efficiently filter active tags)

**Settlement Collection**:
- Primary: `_id`
- Secondary: `tagIds` (index to support efficient filtering by tags)

## State Transitions

**Tag States**:
- Active: Tag can be assigned to settlements
- Inactive: Tag cannot be assigned but still exists for historical data (when isActive = false)

**State Transition Rules**:
- Active → Inactive: When soft deletion occurs
- Inactive → Active: When tag is restored (if restoration feature is implemented)

## Business Rules

1. Tag names must be unique system-wide
2. Users with appropriate scopes (tags:create, tags:delete) can manage tags
3. Settlements retain references to soft-deleted tags for historical integrity
4. A settlement can have up to 20 tags maximum
5. When filtering by tags, partial matching (ANY selected tag) is used