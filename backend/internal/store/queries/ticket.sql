-- name: GetOpenTicketByFingerprint :one
SELECT * FROM ticket
WHERE alert_fingerprint = $1 AND status <> 'closed';

-- 告警建单。冲突（同指纹已有未关闭工单，可能是并发请求刚建的）时不插入、不返回行。
-- name: CreateAlertTicket :one
INSERT INTO ticket (
    type, title, system_id, system_label, severity, status, source, alert_fingerprint,
    created_at, response_deadline, resolve_deadline
) VALUES (
    'incident', $1, $2, $3, $4, 'new', 'alert', $5, $6, $7, $8
)
ON CONFLICT (alert_fingerprint) WHERE status <> 'closed' DO NOTHING
RETURNING *;

-- name: InsertTicketLog :exec
INSERT INTO ticket_log (ticket_id, actor, action, from_status, to_status, note, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListTickets :many
SELECT t.*,
       COALESCE(s.code, '') AS system_code,
       COALESCE(s.name, '') AS system_name,
       COALESCE(v.name, '') AS vendor_name
FROM ticket t
LEFT JOIN business_system s ON s.id = t.system_id
LEFT JOIN vendor v ON v.id = s.vendor_id
ORDER BY t.id DESC
LIMIT 200;

-- name: GetTicketView :one
SELECT t.*,
       COALESCE(s.code, '') AS system_code,
       COALESCE(s.name, '') AS system_name,
       COALESCE(v.name, '') AS vendor_name
FROM ticket t
LEFT JOIN business_system s ON s.id = t.system_id
LEFT JOIN vendor v ON v.id = s.vendor_id
WHERE t.id = $1;

-- name: ListTicketLogs :many
SELECT * FROM ticket_log WHERE ticket_id = $1 ORDER BY id;
