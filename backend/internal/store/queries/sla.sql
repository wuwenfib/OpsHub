-- name: GetSLAPolicy :one
SELECT * FROM sla_policy WHERE level = $1 AND severity = $2;
