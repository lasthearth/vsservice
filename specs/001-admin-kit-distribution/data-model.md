# Data Model: Admin Kit Distribution

**Feature**: Admin Kit Distribution | **Date**: November 17, 2025

## Domain Models

### Kit
- `id: string` (auto-generated UUID)
- `name: string` (unique kit identifier)
- `description: string` (what the kit contains)
- `items: []string` (list of item IDs or names in the kit)
- `createdAt: time.Time`
- `updatedAt: time.Time`
- `isActive: bool` (whether the kit can be assigned)

**Methods**:
- `Validate() error` - validates kit data integrity
- `AddItem(itemID string)` - adds an item to the kit
- `RemoveItem(itemID string)` - removes an item from the kit

### KitAssignment
- `id: string` (auto-generated UUID)
- `userID: string` (ID of the user receiving the kit)
- `kitID: string` (ID of the assigned kit)
- `status: AssignmentStatus` (enum: pending, delivered, claimed)
- `assignedAt: time.Time` (when the assignment was created)
- `deliveredAt: *time.Time` (when delivered to the game, null if not delivered)
- `claimedAt: *time.Time` (when claimed by user, null if not claimed)
- `assignedBy: string` (ID of admin who assigned the kit)

**Methods**:
- `Validate() error` - validates assignment data
- `TransitionTo(status AssignmentStatus) error` - transitions assignment to a new state
- `IsDelivered() bool` - checks if assignment has been delivered
- `IsClaimed() bool` - checks if assignment has been claimed

### AssignmentStatus (enum)
- `PENDING` - Assignment created but not yet delivered to game
- `DELIVERED` - Assignment sent to game system
- `CLAIMED` - User has claimed the kit in game

## Validation Rules

### Kit Validation
- Name must be 1-50 characters (alphanumeric and spaces)
- Description must be 1-200 characters
- Must have at least 1 item and no more than 20 items
- Name must be unique

### KitAssignment Validation
- userID, kitID, and assignedBy must not be empty
- Status must be a valid AssignmentStatus
- assignedAt must not be in the future
- Status transitions must follow valid sequence: PENDING → DELIVERED → CLAIMED

## State Transitions

### KitAssignment State Transitions
- From `PENDING` to `DELIVERED` when kit is sent to Vintage Story game
- From `DELIVERED` to `CLAIMED` when user picks up kit in game
- No backwards transitions allowed
- No skipping states (must go PENDING → DELIVERED → CLAIMED)

## Relationships

### User → KitAssignment (One-to-Many)
- A user can have multiple kit assignments over time
- Each assignment is linked to one specific user

### Kit → KitAssignment (One-to-Many)
- A kit can be assigned to multiple users
- Each assignment is linked to one specific kit

## MongoDB Collections

### kits
- `_id`: ObjectId (maps to kit.id)
- `name`: string (unique)
- `description`: string
- `items`: array of strings
- `created_at`: date
- `updated_at`: date
- `is_active`: boolean

### kit_assignments
- `_id`: ObjectId (maps to assignment.id)
- `user_id`: string
- `kit_id`: string
- `status`: string (enum)
- `assigned_at`: date
- `delivered_at`: date (nullable)
- `claimed_at`: date (nullable)
- `assigned_by`: string