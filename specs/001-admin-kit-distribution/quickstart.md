# Quickstart: Admin Kit Distribution

**Feature**: Admin Kit Distribution | **Date**: November 17, 2025

## Overview
This guide provides a quick start to set up and run the Admin Kit Distribution service.

## Prerequisites
- Go 1.24 or higher
- Docker and Docker Compose
- NATS server
- MongoDB
- MinIO (optional, depending on file storage needs)

## Setting Up the Development Environment

### 1. Clone the Repository
```bash
git clone <repository-url>
cd vsservice
```

### 2. Install Dependencies
```bash
go mod download
```

### 3. Set Up Local Services with Docker
```bash
docker-compose -f compose.dev.yaml up -d
```

This will start:
- MongoDB
- NATS
- MinIO (if needed)

### 4. Environment Configuration
Create a `.env` file in the project root with the following variables:
```
MONGODB_URL=mongodb://localhost:27017
NATS_URL=nats://localhost:4222
MINIO_URL=localhost:9000
CDN_URL=http://localhost:9000
LOGTO_ENDPOINT=http://localhost:3001
LOGTO_APP_ID=your-app-id
LOGTO_APP_SECRET=your-app-secret
```

## Running the Service

### 1. Generate Code
```bash
# Generate protobuf code
buf generate

# Generate goverter mappers
go generate ./...
```

### 2. Run Migrations (if needed)
```bash
# Run any database migrations
go run cmd/migrate/main.go
```

### 3. Start the Service
```bash
go run main.go
```

## API Usage Examples

### 1. Get Available Kits
```bash
curl -H "Authorization: Bearer <admin-jwt-token>" \
     http://localhost:8080/kits
```

### 2. Assign a Kit to User
```bash
curl -X POST -H "Authorization: Bearer <admin-jwt-token>" \
     -H "Content-Type: application/json" \
     -d '{"userId": "user123", "kitName": "starter-kit"}' \
     http://localhost:8080/kits/assign
```

### 3. Check Assignment Status
```bash
curl -H "Authorization: Bearer <admin-jwt-token>" \
     http://localhost:8080/kits/assignments/assignment123
```

## NATS Integration Testing

### Publish a Test Event
```bash
# Use NATS CLI or a test client to publish events to test the integration:
nats pub kit.assignment.requested '{"assignmentId": "test123", "userId": "user123", "kitName": "test-kit", "requestedAt": "2025-11-17T10:00:00Z"}'
```

## Service Architecture

### Key Components
1. **Kit Service** - gRPC service handling kit assignment operations
2. **Repository Layer** - Interfaces and MongoDB implementations for data access
3. **Domain Models** - Business logic encapsulated in Kit and KitAssignment structs
4. **NATS Integration** - Communication with Vintage Story game via message queues
5. **Authentication/Authorization** - JWT-based security with Scoper interface

### Data Flow
1. Admin requests to assign kit via API
2. Service validates request and checks permissions
3. Assignment is created in database with PENDING status
4. NATS event is published to inform game of new assignment
5. Game acknowledges delivery, status updates to DELIVERED
6. User claims kit in game, status updates to CLAIMED

## Testing

### Run Unit Tests
```bash
go test ./internal/kit/...
```

### Run Integration Tests
```bash
# Make sure services are running via docker-compose
go test -tags=integration ./internal/kit/...
```

## Common Issues and Solutions

### 1. Connection Issues
- Ensure MongoDB and NATS are running
- Verify connection strings in `.env` file

### 2. Authorization Errors
- Ensure JWT token has proper admin scopes
- Verify token is not expired

### 3. NATS Event Processing
- Check NATS server is accessible
- Verify subject names match between publisher and subscriber