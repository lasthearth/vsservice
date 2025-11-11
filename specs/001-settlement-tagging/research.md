# Research Summary: Settlement Tagging System

## Decision: Tags Domain Implementation Approach
**Rationale**: Based on the input specifications and constitutional principles, tags will be implemented as an extension to the existing settlements domain rather than as a separate domain. This follows principle 14 which states that if functionality is related to an already existing domain and the domain already exists, we should extend the corresponding service.
**Alternatives considered**: 
- Separate tags service: Would create unnecessary complexity and potential data consistency issues
- Tags as part of a general metadata service: Would be over-engineering for this specific use case

## Decision: Tag Storage and Relationships
**Rationale**: Tags will be stored as references (IDs) within the settlement documents in MongoDB. This approach efficiently supports the filtering requirements while maintaining referential integrity. The tag IDs will be stored in an array field within the settlement document. For soft deletion, tags will have an `isActive` boolean field rather than being completely removed.
**Alternatives considered**:
- Embedded tag objects in settlement: Would duplicate tag information and make updates complex
- Only hard deletion: Would lose historical data and potentially break references

## Decision: Tag Uniqueness Constraint
**Rationale**: The system will enforce unique tag names across the entire system as specified in the functional requirements (FR-013). This will be implemented as a unique index in MongoDB to prevent duplicate tag names from being created.
**Alternatives considered**:
- No uniqueness constraint: Would lead to confusion and inconsistent user experience
- Unique names only within settlements: Would still allow system-wide duplicates

## Decision: Tag Filtering Implementation
**Rationale**: For filtering settlements by tags with partial matching (ANY selected tag), we'll use MongoDB's array field queries. This is efficient and straightforward to implement using the `$in` operator. For settlements that need ALL specified tags, we can use the `$all` operator.
**Alternatives considered**:
- Separate indexing service: Would be overkill for the specified requirements
- In-memory filtering after DB query: Less efficient than using database-level filtering

## Decision: Permission System Integration
**Rationale**: The tag operations will integrate with the existing scope-based authorization system. We'll implement specific scopes for tag operations (tags:create, tags:delete) as specified in FR-010, leveraging the existing interceptor pattern with `interceptor.authorize()`.
**Alternatives considered**:
- Custom permission system: Would create unnecessary complexity
- Role-based permissions: Would be less granular than the existing scope system

## Decision: Soft Delete Implementation
**Rationale**: Following the clarification that tags should be soft-deleted to preserve historical data (FR-012), we'll implement an `isActive` boolean field in the Tag model. This allows for preserving references in settlements while preventing soft-deleted tags from appearing in tag lists or being available for new assignments.
**Alternatives considered**:
- Hard deletion with cleanup: Risk of orphaned references
- Archive table approach: Would add unnecessary complexity for this use case

## Research: MongoDB Array Field Queries for Tag Filtering
For implementing tag filtering with MongoDB:
1. For partial matching (ANY selected tag): Use the `$in` operator
2. For complete matching (ALL selected tags): Use the `$all` operator
3. Both approaches are efficient when using indexes on the tag_ids array field

Example query for partial matching:
```javascript
db.settlements.find({ "tag_ids": { $in: ["tag1", "tag2", "tag3"] } })
```

Example query for complete matching:
```javascript
db.settlements.find({ "tag_ids": { $all: ["tag1", "tag2", "tag3"] } })
```

## Research: Security Implementation for Tag Operations
Based on the requirement for scope-based permissions (tags:create, tags:delete), we'll implement security using the existing JWT interceptor pattern:

1. Tag creation endpoint will require the `tags:create` scope
2. Tag deletion endpoint will require the `tags:delete` scope
3. Both will leverage the existing `interceptor.authorize()` function for validation
4. This follows the constitutional principle for authorization (Principle 5)