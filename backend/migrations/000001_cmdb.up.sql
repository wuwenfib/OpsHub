-- CMDB：厂商、业务系统、主机，以及系统与主机、系统与系统之间的关系。

CREATE TABLE vendor (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         TEXT        NOT NULL UNIQUE,
    contact      TEXT        NOT NULL DEFAULT '',
    phone        TEXT        NOT NULL DEFAULT '',
    contract_end DATE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE business_system (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code        TEXT        NOT NULL UNIQUE,   -- 与告警的 system 标签一致
    name        TEXT        NOT NULL,
    department  TEXT        NOT NULL DEFAULT '',
    level       SMALLINT    NOT NULL CHECK (level BETWEEN 1 AND 3),  -- 重要等级，1 最高
    vendor_id   BIGINT      REFERENCES vendor (id) ON DELETE SET NULL,
    owner       TEXT        NOT NULL DEFAULT '',
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE host (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ip         TEXT        NOT NULL UNIQUE,
    hostname   TEXT        NOT NULL,
    os         TEXT        NOT NULL DEFAULT '',
    env        TEXT        NOT NULL CHECK (env IN ('prod', 'test')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 系统部署在哪些主机上（多对多）
CREATE TABLE system_host (
    system_id BIGINT NOT NULL REFERENCES business_system (id) ON DELETE CASCADE,
    host_id   BIGINT NOT NULL REFERENCES host (id) ON DELETE CASCADE,
    PRIMARY KEY (system_id, host_id)
);

-- 系统依赖：system_id 依赖 depends_on_id
CREATE TABLE system_dependency (
    system_id     BIGINT NOT NULL REFERENCES business_system (id) ON DELETE CASCADE,
    depends_on_id BIGINT NOT NULL REFERENCES business_system (id) ON DELETE CASCADE,
    PRIMARY KEY (system_id, depends_on_id),
    CHECK (system_id <> depends_on_id)
);
