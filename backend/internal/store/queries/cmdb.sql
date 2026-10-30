-- ========== 厂商 ==========

-- name: ListVendors :many
SELECT * FROM vendor ORDER BY id;

-- name: GetVendor :one
SELECT * FROM vendor WHERE id = $1;

-- name: CreateVendor :one
INSERT INTO vendor (name, contact, phone, contract_end)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateVendor :one
UPDATE vendor
SET name = $2, contact = $3, phone = $4, contract_end = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteVendor :execrows
DELETE FROM vendor WHERE id = $1;

-- ========== 业务系统 ==========

-- name: ListSystems :many
SELECT s.*, COALESCE(v.name, '') AS vendor_name
FROM business_system s
LEFT JOIN vendor v ON v.id = s.vendor_id
ORDER BY s.level, s.code;

-- name: GetSystemByCode :one
SELECT * FROM business_system WHERE code = $1;

-- name: CreateSystem :one
INSERT INTO business_system (code, name, department, level, vendor_id, owner, description)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateSystemByCode :one
UPDATE business_system
SET name = $2, department = $3, level = $4, vendor_id = $5, owner = $6, description = $7,
    updated_at = now()
WHERE code = $1
RETURNING *;

-- name: DeleteSystemByCode :execrows
DELETE FROM business_system WHERE code = $1;

-- ========== 主机 ==========

-- name: ListHosts :many
SELECT * FROM host ORDER BY id;

-- name: CreateHost :one
INSERT INTO host (ip, hostname, os, env)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateHost :one
UPDATE host
SET ip = $2, hostname = $3, os = $4, env = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteHost :execrows
DELETE FROM host WHERE id = $1;

-- ========== 系统 ↔ 主机 ==========

-- name: ListHostsBySystem :many
SELECT h.*
FROM host h
JOIN system_host sh ON sh.host_id = h.id
WHERE sh.system_id = $1
ORDER BY h.id;

-- name: ClearSystemHosts :exec
DELETE FROM system_host WHERE system_id = $1;

-- 不存在的 host_id 会触发外键错误，由 service 翻译成 400。
-- name: AddSystemHosts :execrows
INSERT INTO system_host (system_id, host_id)
SELECT sqlc.arg(system_id), unnest(sqlc.arg(host_ids)::bigint[]);

-- ========== 系统依赖 ==========

-- name: ListDependsOn :many
SELECT s.code, s.name, s.level
FROM system_dependency d
JOIN business_system s ON s.id = d.depends_on_id
WHERE d.system_id = $1
ORDER BY s.code;

-- name: ListDependedBy :many
SELECT s.code, s.name, s.level
FROM system_dependency d
JOIN business_system s ON s.id = d.system_id
WHERE d.depends_on_id = $1
ORDER BY s.code;

-- name: ClearSystemDependencies :exec
DELETE FROM system_dependency WHERE system_id = $1;

-- 按编码插入；返回插入行数，少于传入个数说明有编码不存在。
-- name: AddSystemDependencies :execrows
INSERT INTO system_dependency (system_id, depends_on_id)
SELECT sqlc.arg(system_id), s.id
FROM business_system s
WHERE s.code = ANY (sqlc.arg(codes)::text[]);
