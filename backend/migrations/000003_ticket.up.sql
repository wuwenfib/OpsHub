-- 工单与工单流水。

CREATE TABLE ticket (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type              TEXT        NOT NULL CHECK (type IN ('incident', 'problem', 'change')),
    title             TEXT        NOT NULL,
    system_id         BIGINT      REFERENCES business_system (id) ON DELETE SET NULL, -- NULL = 未登记系统
    system_label      TEXT        NOT NULL DEFAULT '',  -- 告警里原始的 system 标签值，关联不上时靠它追查
    severity          TEXT        NOT NULL CHECK (severity IN ('P1', 'P2', 'P3', 'P4')),
    status            TEXT        NOT NULL CHECK (status IN ('new', 'acknowledged', 'resolved', 'closed', 'reopened')),
    source            TEXT        NOT NULL CHECK (source IN ('alert', 'manual')),
    alert_fingerprint TEXT,
    assignee          TEXT        NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL,
    responded_at      TIMESTAMPTZ,
    resolved_at       TIMESTAMPTZ,
    closed_at         TIMESTAMPTZ,
    response_deadline TIMESTAMPTZ NOT NULL,
    resolve_deadline  TIMESTAMPTZ NOT NULL,
    CHECK (source <> 'alert' OR alert_fingerprint IS NOT NULL)
);

-- 去重的关键：同一个告警指纹，同时最多只能有一张未关闭的工单。
-- 并发推送时由数据库保证只有一个 INSERT 成功，其余走 ON CONFLICT DO NOTHING。
-- closed 的工单不在索引范围内，所以关单后同一告警再触发可以新建工单。
CREATE UNIQUE INDEX ticket_open_fingerprint_uq ON ticket (alert_fingerprint) WHERE status <> 'closed';

CREATE INDEX ticket_system_id_idx ON ticket (system_id);
CREATE INDEX ticket_status_idx ON ticket (status);

CREATE TABLE ticket_log (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ticket_id   BIGINT      NOT NULL REFERENCES ticket (id) ON DELETE CASCADE,
    actor       TEXT        NOT NULL,   -- alertmanager / system / 操作人姓名
    action      TEXT        NOT NULL,   -- created / duplicate_alert / alert_resolved / ack / resolve / close / reopen
    from_status TEXT,
    to_status   TEXT,
    note        TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX ticket_log_ticket_id_idx ON ticket_log (ticket_id, id);
