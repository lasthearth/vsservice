# Quickstart: Settlement Tagging System

## Overview
This guide provides a quick introduction to implementing the settlement tagging functionality in the vsservice project, following the established architecture patterns.

## Prerequisites
- Go 1.24+
- MongoDB (v4.0+)
- MinIO (for file storage)
- buf (for protobuf generation)
- docker-compose (for local development)

## Architecture Pattern
The tagging functionality extends the existing settlements service rather than creating a separate service, following the project's architectural principles.

## Implementation Steps

### 1. Extend Data Models
Update the settlement model to include tag functionality:

```go
// internal/settlements/model/settlement.go
type Settlement struct {
    Id          string    `bson:"_id,omitempty"`
    Name        string    `bson:"name"`
    // ... other existing fields
    
    // New fields for tagging
    TagIds      []string  `bson:"tag_ids,omitempty"`  // IDs of associated tags
    // Note: Full tag objects are computed when reading
}
```

Create a tag model with soft deletion support:

```go
// internal/settlements/model/tag.go
type Tag struct {
    Id          string    `bson:"_id,omitempty"`
    Name        string    `bson:"name"`
    Color       string    `bson:"color,omitempty"`        // For visual distinction in UI
    Description string    `bson:"description,omitempty"`
    CreatedAt   time.Time `bson:"created_at"`
    UpdatedAt   time.Time `bson:"updated_at"`
    IsActive    bool      `bson:"is_active"`              // For soft deletion
}

// Validate ensures the tag meets the required constraints
func (t *Tag) Validate() error {
    if len(t.Name) < 1 || len(t.Name) > 50 {
        return fmt.Errorf("tag name must be between 1 and 50 characters")
    }
    
    // Validate name contains only alphanumeric characters, hyphens, and underscores
    matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, t.Name)
    if !matched {
        return fmt.Errorf("tag name can only contain alphanumeric characters, hyphens, and underscores")
    }
    
    // Validate color format if provided
    if t.Color != "" {
        matched, _ := regexp.MatchString(`^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$`, t.Color)
        if !matched {
            return fmt.Errorf("color must be in hex format (#RRGGBB or #RGB)")
        }
    }
    
    return nil
}
```

### 2. Extend DTOs
Create DTOs for the new models:

```go
// internal/settlements/internal/dto/tag.go
type TagDTO struct {
    Id          string `bson:"_id,omitempty" json:"id"`
    Name        string `bson:"name" json:"name"`
    Color       string `bson:"color,omitempty" json:"color,omitempty"`
    Description string `bson:"description,omitempty" json:"description,omitempty"`
    CreatedAt   int64  `bson:"created_at" json:"created_at"`
    UpdatedAt   int64  `bson:"updated_at" json:"updated_at"`
    IsActive    bool   `bson:"is_active" json:"is_active"`
}
```

### 3. Extend Repository Interfaces
Add tag-related methods to the repository interface:

```go
// internal/settlements/internal/service/interface.go
type Repository interface {
    // ... existing methods ...
    
    // Tag-related methods
    AddTagToSettlement(ctx context.Context, settlementId, tagId string) error
    RemoveTagFromSettlement(ctx context.Context, settlementId, tagId string) error
    CreateTag(ctx context.Context, tag *Tag) (*Tag, error)
    UpdateTag(ctx context.Context, tag *Tag) (*Tag, error)
    GetTagById(ctx context.Context, id string) (*Tag, error)
    GetTagByName(ctx context.Context, name string) (*Tag, error)
    GetAllTags(ctx context.Context, onlyActive bool) ([]*Tag, error)
    GetSettlementsByTagIds(ctx context.Context, tagIds []string, matchAll bool) ([]*Settlement, error)
    SoftDeleteTag(ctx context.Context, id string) error
}
```

### 4. Implement Tag Repository
Implement tag operations with unique name constraint:

```go
// internal/settlements/internal/repository/mongo/tag.go
func (r *Repository) CreateTag(ctx context.Context, tag *Tag) (*Tag, error) {
    // Validate the tag first
    if err := tag.Validate(); err != nil {
        return nil, status.Errorf(codes.InvalidArgument, "invalid tag: %v", err)
    }
    
    // Check if a tag with this name already exists
    existingTag, err := r.GetTagByName(ctx, tag.Name)
    if err == nil && existingTag != nil && existingTag.IsActive {
        return nil, status.Errorf(codes.AlreadyExists, "a tag with name '%s' already exists", tag.Name)
    }
    
    tag.Id = primitive.NewObjectID().Hex()
    tag.CreatedAt = time.Now()
    tag.UpdatedAt = time.Now()
    tag.IsActive = true
    
    _, err = r.collection("tags").InsertOne(ctx, tag)
    if err != nil {
        return nil, err
    }
    
    return tag, nil
}

func (r *Repository) GetSettlementsByTagIds(ctx context.Context, tagIds []string, matchAll bool) ([]*Settlement, error) {
    // Implementation depends on matchAll parameter
    // If matchAll=true, settlement must have ALL specified tags
    // If matchAll=false, settlement must have ANY of the specified tags
    var filter bson.M
    if matchAll {
        filter = bson.M{"tag_ids": bson.M{"$all": tagIds}}
    } else {
        filter = bson.M{"tag_ids": bson.M{"$in": tagIds}}
    }
    
    cursor, err := r.collection("settlements").Find(ctx, filter)
    if err != nil {
        return nil, err
    }
    defer cursor.Close(ctx)
    
    var settlements []*Settlement
    if err = cursor.All(ctx, &settlements); err != nil {
        return nil, err
    }
    
    return settlements, nil
}

func (r *Repository) SoftDeleteTag(ctx context.Context, id string) error {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return status.Errorf(codes.InvalidArgument, "invalid tag ID format")
    }
    
    filter := bson.M{"_id": objectID}
    update := bson.M{"$set": bson.M{"is_active": false, "updated_at": time.Now()}}
    
    result, err := r.collection("tags").UpdateOne(ctx, filter, update)
    if err != nil {
        return err
    }
    
    if result.MatchedCount == 0 {
        return status.Errorf(codes.NotFound, "tag with ID %s not found", id)
    }
    
    return nil
}
```

### 5. Extend gRPC Service
Update the settlements protobuf with the new tag operations as defined in the API contract, then generate the code:
```bash
buf generate
```

### 6. Update Service Implementation
Implement the tag methods in the settlements service with proper authentication checks:

```go
// internal/settlements/internal/service/implementation.go
func (s *Service) CreateTag(ctx context.Context, req *CreateTagRequest) (*CreateTagResponse, error) {
    // Check if user has the required scope
    if !s.auth.HasScope(ctx, "tags:create") {
        return nil, status.Error(codes.PermissionDenied, "missing tags:create scope")
    }
    
    // Validate the request
    if req.Tag == nil {
        return nil, status.Error(codes.InvalidArgument, "tag is required")
    }
    
    // Create the tag via repository
    tag, err := s.repo.CreateTag(ctx, &Tag{
        Name:        req.Tag.Name,
        Color:       req.Tag.Color,
        Description: req.Tag.Description,
    })
    if err != nil {
        return nil, err
    }
    
    return &CreateTagResponse{
        Tag: s.converter.ToProto(tag),
    }, nil
}

func (s *Service) AddTagToSettlement(ctx context.Context, req *AddTagToSettlementRequest) (*AddTagToSettlementResponse, error) {
    // Check if user has the required scope
    if !s.auth.HasScope(ctx, "tags:create") {
        return nil, status.Error(codes.PermissionDenied, "missing tags:create scope")
    }
    
    // 1. Verify settlement exists
    settlement, err := s.repo.GetSettlementById(ctx, req.SettlementId)
    if err != nil {
        return nil, status.Errorf(codes.NotFound, "settlement with ID %s not found", req.SettlementId)
    }
    
    // 2. Verify tag exists and is active
    tag, err := s.repo.GetTagById(ctx, req.TagId)
    if err != nil {
        return nil, status.Errorf(codes.NotFound, "tag with ID %s not found", req.TagId)
    }
    if !tag.IsActive {
        return nil, status.Errorf(codes.FailedPrecondition, "tag with ID %s is not active", req.TagId)
    }
    
    // 3. Verify settlement doesn't exceed tag limit (max 20)
    if len(settlement.TagIds) >= 20 {
        return nil, status.Errorf(codes.FailedPrecondition, "settlement already has maximum number of tags (20)")
    }
    
    // 4. Check if tag is already added to this settlement
    for _, existingTagId := range settlement.TagIds {
        if existingTagId == req.TagId {
            return nil, status.Errorf(codes.InvalidArgument, "tag with ID %s is already assigned to this settlement", req.TagId)
        }
    }
    
    // 5. Add tag ID to the settlement
    err = s.repo.AddTagToSettlement(ctx, req.SettlementId, req.TagId)
    if err != nil {
        return nil, err
    }
    
    // 6. Return updated settlement
    updatedSettlement, err := s.repo.GetSettlementById(ctx, req.SettlementId)
    if err != nil {
        return nil, err
    }
    
    return &AddTagToSettlementResponse{
        Settlement: s.converter.ToProto(updatedSettlement),
    }, nil
}

func (s *Service) ListSettlementsByTags(ctx context.Context, req *ListSettlementsByTagsRequest) (*ListSettlementsByTagsResponse, error) {
    // This operation is available to all authenticated users
    settlements, err := s.repo.GetSettlementsByTagIds(ctx, req.TagIds, req.MatchAll)
    if err != nil {
        return nil, err
    }
    
    protoSettlements := make([]*pb.Settlement, len(settlements))
    for i, settlement := range settlements {
        protoSettlements[i] = s.converter.ToProto(settlement)
    }
    
    return &ListSettlementsByTagsResponse{
        Settlements: protoSettlements,
    }, nil
}
```

### 7. Update Service Registration
Register the updated service with fx:

```go
// internal/settlements/settlements.go
func Module() fx.Option {
    return fx.Options(
        fx.Provide(
            NewService,
            NewRepository,
            NewConverter,
        ),
        fx.Invoke(RegisterService),
    )
}
```

### 8. Add Authentication/Authorization
Ensure the new tag endpoints use the appropriate scope checking:

```go
// The service implementation includes scope checking using s.auth.HasScope(ctx, "scope_name")
// This leverages the existing JWT interceptor pattern with scope-based authorization
```

## Running Locally

```bash
# Start dependencies
docker-compose up -d

# Generate protobuf code
buf generate

# Run the service
go run main.go
```

## Testing

Create tests following the existing patterns:

```go
// For repository tests
func TestCreateTag(t *testing.T) {
    // Test tag creation with unique name validation
}

func TestSoftDeleteTag(t *testing.T) {
    // Test that soft deletion works correctly
}

// For service tests
func TestService_AddTagToSettlement(t *testing.T) {
    // Test the service logic for adding tags to settlements
}

func TestService_ListSettlementsByTags(t *testing.T) {
    // Test filtering settlements by tags with both matchAll and partial matching
}
```

## Database Indexes

Ensure the following indexes are created for optimal performance:

```javascript
// On the tags collection:
db.tags.createIndex({ "name": 1 }, { unique: true })
db.tags.createIndex({ "is_active": 1 })

// On the settlements collection:
db.settlements.createIndex({ "tag_ids": 1 })
```