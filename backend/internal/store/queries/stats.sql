-- 取出统计时间范围内的事件单（左闭右开），聚合在 Go 里做（sla.Aggregate），便于用手算案例测试。
-- 数据量很大时应改为在 SQL 里 GROUP BY。
-- name: ListTicketsForStats :many
SELECT t.system_label,
       t.status,
       t.created_at,
       t.responded_at,
       t.resolved_at,
       t.response_deadline,
       t.resolve_deadline,
       COALESCE(s.name, '') AS system_name,
       COALESCE(v.name, '') AS vendor_name
FROM ticket t
LEFT JOIN business_system s ON s.id = t.system_id
LEFT JOIN vendor v ON v.id = s.vendor_id
WHERE t.type = 'incident'
  AND t.created_at >= sqlc.arg(from_time)
  AND t.created_at < sqlc.arg(to_time);
