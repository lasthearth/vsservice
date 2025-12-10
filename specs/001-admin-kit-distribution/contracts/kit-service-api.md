# Kit Service API Contract

## Kit Service

### Kit Service API Endpoints

#### Get Available Kits
- **Method**: `GET /kits`
- **Description**: Retrieve the list of available kits that can be assigned to users
- **Authentication**: Required (admin scope)
- **Request**:
  - Headers: Authorization: Bearer <token>
- **Response**:
  - 200: OK - Returns list of available kits
    ```json
    {
      "kits": [
        {
          "id": "string",
          "name": "string",
          "description": "string",
          "items": ["string"],
          "isActive": "boolean"
        }
      ]
    }
    ```
  - 401: Unauthorized
  - 403: Forbidden (insufficient permissions)

#### Assign Kit to User
- **Method**: `POST /kits/assign`
- **Description**: Assign a specific kit to a user
- **Authentication**: Required (admin scope)
- **Request**:
  - Headers: Authorization: Bearer <token>
  - Body:
    ```json
    {
      "userId": "string",
      "kitName": "string"
    }
    ```
- **Response**:
  - 200: OK - Kit assigned successfully
    ```json
    {
      "assignmentId": "string",
      "userId": "string",
      "kitName": "string",
      "status": "string",
      "assignedAt": "timestamp"
    }
    ```
  - 400: Bad Request - Invalid request data
  - 401: Unauthorized
  - 403: Forbidden (insufficient permissions)
  - 404: Kit or user not found

#### Get Assignment Status
- **Method**: `GET /kits/assignments/{assignmentId}`
- **Description**: Get the status of a specific kit assignment
- **Authentication**: Required (admin scope or user's own assignment)
- **Request**:
  - Headers: Authorization: Bearer <token>
  - Path Parameter: assignmentId
- **Response**:
  - 200: OK - Returns assignment details
    ```json
    {
      "id": "string",
      "userId": "string",
      "kitName": "string",
      "status": "string",
      "assignedAt": "timestamp",
      "deliveredAt": "timestamp",
      "claimedAt": "timestamp"
    }
    ```
  - 401: Unauthorized
  - 403: Forbidden
  - 404: Assignment not found

#### List User Assignments
- **Method**: `GET /kits/users/{userId}/assignments`
- **Description**: Get all kit assignments for a specific user
- **Authentication**: Required (admin scope or user's own assignments)
- **Request**:
  - Headers: Authorization: Bearer <token>
  - Path Parameter: userId
- **Response**:
  - 200: OK - Returns list of assignments
    ```json
    {
      "assignments": [
        {
          "id": "string",
          "kitName": "string",
          "status": "string",
          "assignedAt": "timestamp",
          "deliveredAt": "timestamp",
          "claimedAt": "timestamp"
        }
      ]
    }
    ```
  - 401: Unauthorized
  - 403: Forbidden
  - 404: User not found

## NATS Events

### Kit Assignment Events

#### KitAssignmentRequestedEvent
- **Subject**: `kit.assignment.requested`
- **Description**: Emitted when an admin assigns a kit to user
- **Payload**:
  ```json
  {
    "assignmentId": "string",
    "userId": "string",
    "kitName": "string",
    "requestedAt": "timestamp"
  }
  ```

#### KitAssignmentDeliveredEvent
- **Subject**: `kit.assignment.delivered`
- **Description**: Emitted when kit is delivered to game client
- **Payload**:
  ```json
  {
    "assignmentId": "string",
    "userId": "string",
    "deliveredAt": "timestamp"
  }
  ```

#### KitAssignmentClaimedEvent
- **Subject**: `kit.assignment.claimed`
- **Description**: Emitted when user claims kit in game
- **Payload**:
  ```json
  {
    "assignmentId": "string",
    "userId": "string",
    "claimedAt": "timestamp"
  }
  ```