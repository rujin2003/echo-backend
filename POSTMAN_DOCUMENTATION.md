# Echo Backend API - Postman Documentation

This documentation provides a comprehensive guide for testing the Echo Backend WebSocket API using Postman.

## Overview

The Echo Backend is a WebSocket-based communication system that enables real-time data exchange between Mac and Watch devices. The API supports:

- Room management (create/join rooms)
- Device information sharing
- Battery, storage, and downloads monitoring
- Media control (play, pause, volume, etc.)
- System actions (shutdown, sleep)
- Generic request/response patterns

## Setup Instructions

### 1. Import Collection and Environment

1. Open Postman
2. Click **Import** and select both files:
   - `Echo-Backend-API.postman_collection.json`
   - `Echo-Backend-Environment.postman_environment.json`
3. Select the **Echo Backend Environment** from the environment dropdown

### 2. Generate JWT Tokens

Before testing, you need to generate valid JWT tokens for both Mac and Watch devices. The tokens must include:

```json
{
  "device_id": "unique_device_identifier",
  "device_type": "mac" or "watch",
  "iat": 1704067200
}
```

**Example JWT Generation (using jwt.io):**

For Mac device:
```json
{
  "alg": "HS256",
  "typ": "JWT"
}
{
  "device_id": "mac_device_123",
  "device_type": "mac",
  "iat": 1704067200
}
```

For Watch device:
```json
{
  "alg": "HS256",
  "typ": "JWT"
}
{
  "device_id": "watch_device_456",
  "device_type": "watch",
  "iat": 1704067200
}
```

**Secret:** Use the same secret as configured in your backend's `JWT_SECRET` environment variable.

### 3. Update Environment Variables

1. In Postman, go to **Environments** → **Echo Backend Environment**
2. Update the following variables:
   - `mac_jwt_token`: Your generated Mac JWT token
   - `watch_jwt_token`: Your generated Watch JWT token
   - `base_url`: Your server URL (default: `ws://localhost:8080`)

## API Endpoints

### WebSocket Connection

**Endpoint:** `ws://localhost:8080/ws?token={jwt_token}`

**Authentication:** JWT token in query parameter

**Device Types:**
- `mac`: Mac device (can create rooms, send device info, respond to actions)
- `watch`: Watch device (can join rooms, request actions, control media)

## Event Types

### Room Management Events

#### Create Room
```json
{
  "type": "create_room",
  "payload": {
    "room_id": "unique_room_identifier"
  }
}
```
- **Device:** Mac only
- **Response:** `room_joined` event with `role: "host"`

#### Join Room
```json
{
  "type": "join_room",
  "payload": {
    "room_id": "unique_room_identifier"
  }
}
```
- **Device:** Mac or Watch
- **Response:** `room_joined` event with `role: "client"`

### Device Information Events

#### Device Info
```json
{
  "type": "device_info",
  "payload": {
    "device_name": "MacBook Pro",
    "os_version": "macOS 14.0",
    "model": "MacBookPro18,1",
    "serial_number": "F5F123456789"
  }
}
```
- **Device:** Mac only
- **Caching:** 24 hours
- **Broadcast:** Sent to all Watch devices in room

### Monitoring Events

#### Battery Update
```json
{
  "type": "battery_update",
  "payload": {
    "level": 85,
    "charging": true,
    "time_remaining": "2h 30m"
  }
}
```
- **Device:** Mac only
- **Caching:** 30 seconds
- **Broadcast:** Sent to all Watch devices in room

#### Storage Update
```json
{
  "type": "storage_update",
  "payload": {
    "total_space": "1TB",
    "used_space": "750GB",
    "available_space": "250GB",
    "usage_percentage": 75
  }
}
```
- **Device:** Mac only
- **Caching:** 5 minutes
- **Broadcast:** Sent to all Watch devices in room

#### Downloads Update
```json
{
  "type": "downloads_update",
  "payload": {
    "active_downloads": 3,
    "completed_today": 12,
    "total_size_downloaded": "2.5GB"
  }
}
```
- **Device:** Mac only
- **Caching:** 10 seconds
- **Broadcast:** Sent to all Watch devices in room

### Media Control Events

#### Media Action
```json
{
  "type": "media_action",
  "payload": {
    "action": "play" // play, pause, volup, volDown, next, prev
  }
}
```
- **Device:** Watch only
- **Target:** Mac device in same room

### System Action Events

#### Action Request
```json
{
  "type": "action_request",
  "request_id": "unique_request_id",
  "payload": {
    "action": "shutdown" // shutdown, sleep
  }
}
```
- **Device:** Watch only
- **Response:** `action_result` event

#### Action Result
```json
{
  "type": "action_result",
  "request_id": "original_request_id",
  "payload": {
    "success": true,
    "message": "Action completed successfully"
  }
}
```
- **Device:** Mac only
- **Purpose:** Response to action requests

### Generic Request/Response Events

#### Request
```json
{
  "type": "request",
  "request_id": "unique_request_id",
  "payload": {
    "action": "get_device_info" // get_device_info, get_battery, etc.
  }
}
```
- **Device:** Any
- **Response:** `response` event

#### Response
```json
{
  "type": "response",
  "request_id": "original_request_id",
  "payload": {
    "data": "response_data_here"
  }
}
```
- **Device:** Any
- **Purpose:** Response to generic requests

### Error Events

#### Error
```json
{
  "type": "error",
  "request_id": "optional_request_id",
  "timestamp": "2024-01-01T00:00:00Z",
  "payload": {
    "code": "error_code",
    "message": "Error description"
  }
}
```

## Testing Workflow

### 1. Basic Connection Test

1. **Connect Mac Device:**
   - Use "Connect as Mac Device" request
   - Verify successful WebSocket connection

2. **Connect Watch Device:**
   - Use "Connect as Watch Device" request
   - Verify successful WebSocket connection

### 2. Room Management Test

1. **Create Room (Mac):**
   - Send `create_room` event with unique room ID
   - Verify `room_joined` response with `role: "host"`

2. **Join Room (Watch):**
   - Send `join_room` event with same room ID
   - Verify `room_joined` response with `role: "client"`

### 3. Data Sharing Test

1. **Send Device Info (Mac):**
   - Send `device_info` event with device details
   - Verify Watch receives the information

2. **Send Monitoring Data (Mac):**
   - Send `battery_update`, `storage_update`, `downloads_update`
   - Verify Watch receives all updates

### 4. Control Test

1. **Media Control (Watch → Mac):**
   - Send `media_action` events (play, pause, volume)
   - Verify Mac receives the commands

2. **System Actions (Watch → Mac):**
   - Send `action_request` for shutdown/sleep
   - Verify Mac responds with `action_result`

### 5. Request/Response Test

1. **Request Data (Watch → Mac):**
   - Send `request` for device info or battery
   - Verify Mac responds with `response`

## Caching Behavior

The API implements intelligent caching:

- **Device Info:** 24 hours (static data)
- **Storage Info:** 5 minutes (semi-dynamic)
- **Battery Info:** 30 seconds (dynamic)
- **Downloads Info:** 10 seconds (very dynamic)

When a Watch device joins a room, it automatically receives all cached data.

## Error Handling

Common error scenarios:

1. **Invalid JWT Token:** HTTP 401 Unauthorized
2. **Missing Device ID:** HTTP 400 Bad Request
3. **Invalid Device Type:** HTTP 400 Bad Request
4. **Room Not Found:** WebSocket error event
5. **Permission Denied:** WebSocket error event
6. **Timeout:** WebSocket error event (30 seconds)

## Environment Variables

| Variable | Description | Default Value |
|----------|-------------|---------------|
| `base_url` | WebSocket server URL | `ws://localhost:8080` |
| `mac_jwt_token` | JWT token for Mac device | Generated token |
| `watch_jwt_token` | JWT token for Watch device | Generated token |
| `room_id` | Default room identifier | `room_123` |
| `request_id` | Default request identifier | `req_123` |

## Troubleshooting

### Connection Issues

1. **WebSocket Connection Failed:**
   - Verify server is running on correct port
   - Check firewall settings
   - Ensure JWT token is valid

2. **Authentication Errors:**
   - Verify JWT secret matches backend configuration
   - Check token expiration
   - Ensure required claims are present

### Event Issues

1. **Events Not Received:**
   - Verify both devices are in the same room
   - Check device type permissions
   - Ensure proper event format

2. **Timeout Errors:**
   - Check network connectivity
   - Verify target device is connected
   - Increase timeout if necessary

## Security Notes

- JWT tokens should be generated with a strong secret
- Tokens should have appropriate expiration times
- Device IDs should be unique and unpredictable
- Consider implementing rate limiting for production use

## Support

For issues or questions about the API:

1. Check the server logs for detailed error messages
2. Verify JWT token validity using jwt.io
3. Test WebSocket connection using browser developer tools
4. Review the Go source code for implementation details
