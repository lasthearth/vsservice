# News Service API Contract - DeleteNews

## DeleteNews gRPC Service Definition

```protobuf
// DeleteNewsRequest represents the request to delete a news item by ID
message DeleteNewsRequest {
  // The unique identifier of the news item to delete
  string id = 1 [(validate.rules).string = {
    min_len: 1,
    max_len: 64
  }];
}

// DeleteNewsResponse represents the response after attempting to delete a news item
message DeleteNewsResponse {
  // Indicates whether the deletion was successful
  bool success = 1;
  
  // Optional message with additional information about the operation
  string message = 2;
}
```

## Service Definition

```protobuf
// News service definition
service NewsService {
  // Delete a news item by its unique identifier
  // Requires 'news:delete' scope
  rpc DeleteNews(DeleteNewsRequest) returns (DeleteNewsResponse) {
    option (google.api.http) = {
      delete: "/v1/news/{id}"
      additional_bindings: {
        post: "/v1/news/{id}/delete"
      }
    };
  }
}
```

## HTTP REST Mapping

### DELETE Request
```
DELETE /v1/news/{id}
Authorization: Bearer <JWT_TOKEN_WITH_NEWS_DELETE_SCOPE>
Content-Type: application/json

Response:
- Success: 200 OK with body {"success": true, "message": "News item deleted successfully"}
- Not Found: 404 Not Found with body {"success": false, "message": "News item not found"}
- Unauthorized: 401 Unauthorized
- Forbidden: 403 Forbidden (missing required scope)
- Internal Error: 500 Internal Server Error
```

### POST Request Alternative
```
POST /v1/news/{id}/delete
Authorization: Bearer <JWT_TOKEN_WITH_NEWS_DELETE_SCOPE>
Content-Type: application/json

Response:
- Success: 200 OK with body {"success": true, "message": "News item deleted successfully"}
- Not Found: 404 Not Found with body {"success": false, "message": "News item not found"}
- Unauthorized: 401 Unauthorized
- Forbidden: 403 Forbidden (missing required scope)
- Internal Error: 500 Internal Server Error
```

## Authentication & Authorization
- All requests must include a valid JWT token in the Authorization header
- The user must possess the `news:delete` scope to perform this operation
- If the token is invalid or missing, return 401 Unauthorized
- If the token is valid but lacks required scope, return 403 Forbidden

## Error Handling
- 401 Unauthorized: Token is missing or invalid
- 403 Forbidden: User does not have required scope
- 404 Not Found: News item with specified ID does not exist
- 500 Internal Server Error: General server error occurred during deletion

## Validation Rules
- The news `id` must be a valid identifier (non-empty string, max 64 characters)
- The user must be authenticated with a valid JWT token
- The user must have the `news:delete` scope