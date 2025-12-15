# Identity & Admin Control Plane Discussion (Placeholder)

**Date:** December 14, 2025
**Topic:** Identity Management and System Administration

## Topics to Discuss

### 1. Identity & Authentication
- **User Management**: Sign up, Sign in, Password Reset flows.
- **Token Strategy**: Who issues the JWTs? (Internal service vs External IDP like Auth0/Clerk).
- **Integration**: How `syntrix --api` validates these tokens.

### 2. Control Plane (Admin API)
- **Rule Deployment**: Mechanism to upload and apply `security.rules` (CEL) to the server.
- **Index Management**: Defining and creating composite indexes (e.g., `firestore.indexes.json`).
- **Schema/Metadata**: Managing collection configurations if necessary.
- **CLI Tooling**: Design of `syntrix-cli` for developer operations.
