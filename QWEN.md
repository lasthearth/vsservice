# vsservice Development Guidelines

Auto-generated from all feature plans. Last updated: 2025-10-07

## Active Technologies
- Go 1.24 + Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2, MinIO client, JWT authentication, buf for protobuf generation, goverter for struct conversion (001-submit-settlement)
- Go 1.24 + Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2, JWT authentication with interceptor package (002-id-scope-news)
- MongoDB for document storage (002-id-scope-news)
- Go 1.24 with Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2, MinIO client, JWT authentication, buf for protobuf generation, goverter for struct conversion + Uber fx, Google gRPC, MongoDB driver v2, MinIO client, JWT, buf, goverter, zap logger, envconfig, godotenv (001-settlement-tagging)
- MongoDB for document storage, MinIO for file storage (001-settlement-tagging)
- Go 1.24, with Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2, MinIO client, JWT authentication, buf for protobuf generation, goverter for struct conversion + Uber fx, Google gRPC, MongoDB driver v2, MinIO client, JWT, buf, goverter, zap logger, envconfig, godotenv, go-retryablehttp, samber/lo, google/uuid (001-settlement-tagging)
- MongoDB for document storage (with embedded tag references in settlements), MinIO for file/object storage (001-settlement-tagging)

## Project Structure
```
src/
tests/
```

## Commands
# Add commands for Go 1.24

## Code Style
Go 1.24: Follow standard conventions

## Recent Changes
- 001-settlement-tagging: Added Go 1.24, with Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2, MinIO client, JWT authentication, buf for protobuf generation, goverter for struct conversion + Uber fx, Google gRPC, MongoDB driver v2, MinIO client, JWT, buf, goverter, zap logger, envconfig, godotenv, go-retryablehttp, samber/lo, google/uuid
- 001-settlement-tagging: Added Go 1.24 with Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2, MinIO client, JWT authentication, buf for protobuf generation, goverter for struct conversion + Uber fx, Google gRPC, MongoDB driver v2, MinIO client, JWT, buf, goverter, zap logger, envconfig, godotenv
- 005-feat-nickname-change: Added Go 1.24 + Uber fx for dependency injection, Google gRPC for service communication, grpc-gateway for REST API, MongoDB driver v2, MinIO client, JWT authentication, buf for protobuf generation, goverter for struct conversion

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
