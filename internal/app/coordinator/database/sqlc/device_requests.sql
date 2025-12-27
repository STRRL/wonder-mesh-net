-- name: CreateDeviceRequest :exec
INSERT INTO device_requests (device_code, user_code, status, created_at, expires_at)
VALUES (?, ?, 'pending', ?, ?);

-- name: GetDeviceRequestByDeviceCode :one
SELECT * FROM device_requests WHERE device_code = ?;

-- name: GetDeviceRequestByUserCode :one
SELECT * FROM device_requests WHERE user_code = ?;

-- name: ApproveDeviceRequest :exec
UPDATE device_requests
SET status = 'approved',
    realm_id = ?,
    headscale_user = ?,
    authkey = ?,
    headscale_url = ?,
    coordinator_url = ?
WHERE user_code = ? AND status = 'pending' AND expires_at > CURRENT_TIMESTAMP;

-- name: DeleteDeviceRequest :exec
DELETE FROM device_requests WHERE device_code = ?;

-- name: DeleteExpiredDeviceRequests :exec
DELETE FROM device_requests WHERE expires_at < datetime('now', '-1 minute');

-- name: UserCodeExists :one
SELECT EXISTS(SELECT 1 FROM device_requests WHERE user_code = ? AND expires_at > CURRENT_TIMESTAMP) as exists_flag;
