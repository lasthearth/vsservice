# Quickstart: News View Counter Feature

## Overview
This guide explains how to implement and test the news view counter feature in the existing news service.

## Prerequisites
- Go 1.24 installed
- MongoDB running and accessible
- Project dependencies installed (`go mod download`)

## Implementation Steps

### 1. Update the News Model
Update the news model to include the view_count field:

```go
// In internal/news/model/news.go
type News struct {
    Id        bson.ObjectID `bson:"_id,omitempty"`
    Title     string        `bson:"title"`
    Content   string        `bson:"content"`
    Author    string        `bson:"author"`
    CreatedAt time.Time     `bson:"created_at"`
    UpdatedAt time.Time     `bson:"updated_at"`
    ViewCount int64         `bson:"view_count"`  // New field
}
```

### 2. Update the MongoDB DTO
Update the MongoDB DTO to match the model:

```go
// In internal/news/internal/dto/news_dto.go
type NewsDTO struct {
    Id        bson.ObjectID `bson:"_id,omitempty"`
    Title     string        `bson:"title"`
    Content   string        `bson:"content"`
    Author    string        `bson:"author"`
    CreatedAt time.Time     `bson:"created_at"`
    UpdatedAt time.Time     `bson:"updated_at"`
    ViewCount int64         `bson:"view_count"`  // New field
}
```

### 3. Update the Repository
Add a method to increment the view count atomically:

```go
// In internal/news/internal/repository/mongo/news_repository.go
func (r *NewsRepository) IncrementViewCount(ctx context.Context, id string) error {
    objectID, err := bson.NewObjectIDFromHex(id)
    if err != nil {
        return err
    }

    filter := bson.M{"_id": objectID}
    update := bson.M{"$inc": bson.M{"view_count": 1}}

    _, err = r.collection.UpdateOne(ctx, filter, update)
    return err
}

func (r *NewsRepository) GetNewsWithViewCount(ctx context.Context, id string) (*model.News, error) {
    // Implementation to retrieve news with view count
}
```

### 4. Update the gRPC Service
Modify the service to increment the view count when an article is viewed:

```go
// In internal/news/service/news_service.go
func (s *NewsService) GetNews(ctx context.Context, req *pb.GetNewsRequest) (*pb.GetNewsResponse, error) {
    // First increment the view count
    err := s.newsRepository.IncrementViewCount(ctx, req.Id)
    if err != nil {
        // Log error but don't fail the request as view count is non-critical
        // The article will still be returned with potentially stale view count
        s.logger.Error("Failed to increment view count", zap.Error(err))
    }

    // Then get the news article with updated view count
    news, err := s.newsRepository.GetNewsWithViewCount(ctx, req.Id)
    if err != nil {
        return nil, err
    }

    return &pb.GetNewsResponse{
        Id:         news.Id.Hex(),
        Title:      news.Title,
        Content:    news.Content,
        Author:     news.Author,
        CreatedAt:  news.CreatedAt.Unix(),
        UpdatedAt:  news.UpdatedAt.Unix(),
        ViewCount:  news.ViewCount, // Include the view count in response
    }, nil
}
```

### 5. Update the Protobuf Definition
Update the proto file with the new field:

```protobuf
// In proto/news/news.proto
message GetNewsResponse {
  string id = 1;
  string title = 2;
  string content = 3;
  string author = 4;
  int64 created_at = 5;
  int64 updated_at = 6;
  int64 view_count = 7;  // New field for view counter
}
```

## Testing

### Unit Tests
```go
// Test the increment functionality
func TestNewsRepository_IncrementViewCount(t *testing.T) {
    // Implementation for testing the atomic increment
}
```

### Integration Tests
```go
// Verify the entire flow works end-to-end
func TestNewsService_GetNewsWithViewCount(t *testing.T) {
    // Implementation for integration testing
}
```

## Running the Service
1. Run `buf generate` to regenerate protobuf code after updating the proto files
2. Build the service: `go build -o bin/news-service cmd/news/main.go`
3. Run the service: `./bin/news-service`

## Verification
1. Make a request to get a news article: `GET /v1/news/{id}`
2. Verify that the response includes the `view_count` field
3. Make another request to the same article and verify the count increased by 1