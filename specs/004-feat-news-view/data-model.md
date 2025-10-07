# Data Model: News View Counter

## Entity: News Article

### Fields
- `id` (string/objectId): Unique identifier for the news article
- `title` (string): Title of the news article
- `content` (string): Content/body of the news article
- `author` (string): Author of the news article
- `created_at` (timestamp): When the article was created
- `updated_at` (timestamp): When the article was last updated
- `view_count` (integer): Number of times the article has been viewed (new field for this feature)

### Relationships
- None directly related to the view counter feature

### Validation Rules
- `view_count` must be >= 0
- `view_count` must be an integer value
- `view_count` can only be incremented (not directly set by users)

## State Transitions

The view count state transitions are simple:
- Initial state: `view_count` = 0 (when article is created)
- After first view: `view_count` = 1
- After subsequent views: `view_count` = `current_value` + 1

## Constraints

- The `view_count` can only be modified through atomic increment operations
- Only the system (not users) can modify the `view_count` field
- The increment operation must be thread-safe to handle concurrent views