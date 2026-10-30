-- SLA 策略：按"系统重要等级 × 故障严重级别"规定响应时限和恢复时限（分钟，按自然时间计）。
-- 这是参考数据（系统运行的前提），所以放在迁移里，而不是演示 seed 里。

CREATE TABLE sla_policy (
    level            SMALLINT NOT NULL CHECK (level BETWEEN 1 AND 3),
    severity         TEXT     NOT NULL CHECK (severity IN ('P1', 'P2', 'P3', 'P4')),
    response_minutes INTEGER  NOT NULL CHECK (response_minutes > 0),
    resolve_minutes  INTEGER  NOT NULL CHECK (resolve_minutes >= response_minutes),
    PRIMARY KEY (level, severity)
);

INSERT INTO sla_policy (level, severity, response_minutes, resolve_minutes) VALUES
    -- 一级系统
    (1, 'P1',   15,  120),   -- 15 分钟响应 / 2 小时恢复
    (1, 'P2',   30,  240),   -- 30 分钟 / 4 小时
    (1, 'P3',   60,  480),   -- 1 小时 / 8 小时
    (1, 'P4',  240, 1440),   -- 4 小时 / 24 小时
    -- 二级系统
    (2, 'P1',   30,  240),
    (2, 'P2',   60,  480),
    (2, 'P3',  120, 1440),
    (2, 'P4',  480, 2880),
    -- 三级系统
    (3, 'P1',   60,  480),
    (3, 'P2',  120, 1440),
    (3, 'P3',  240, 4320),   -- 4 小时 / 3 天（PLAN 里的"3 个工作日"先按自然日简化）
    (3, 'P4', 1440, 7200);
