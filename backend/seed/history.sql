-- 演示用的历史工单：过去 30 天内 11 张已关闭的事件单，分布在 4 个厂商，有按时也有违约。
-- 作用：SLA 看板一打开就有区分度（而不是全是 100%），现场新增一张工单时达标率会有可见变化。
-- 截止时间按 sla_policy 实时计算；告警指纹 history-N 保证可重复执行（已存在的跳过）。
--
--  fp          系统        级别 天前 受理(分) 恢复(分)  结论（对照 sla_policy）
--  history-01  GOV-APPLY   P2   20   12      95        一级P2 30m/4h：响应✓ 恢复✓
--  history-02  GOV-APPLY   P1   15   25      150       一级P1 15m/2h：响应✗ 恢复✗
--  history-03  GOV-OA      P3    9   40      300       二级P3 2h/24h：响应✓ 恢复✓
--  history-04  GOV-APPLY   P2    4   45      200       一级P2 30m/4h：响应✗ 恢复✓
--  history-05  GOV-AUTH    P1   18    8      70        一级P1 15m/2h：响应✓ 恢复✓
--  history-06  GOV-AUTH    P2    7   20      210       一级P2 30m/4h：响应✓ 恢复✓
--  history-07  GOV-DATA    P2   22   75      600       二级P2 1h/8h：响应✗ 恢复✗
--  history-08  GOV-DATA    P3   12  100      900       二级P3 2h/24h：响应✓ 恢复✓
--  history-09  GOV-DATA    P2    3   50      520       二级P2 1h/8h：响应✓ 恢复✗
--  history-10  GOV-PORTAL  P3   16  200      2000      三级P3 4h/72h：响应✓ 恢复✓
--  history-11  GOV-PORTAL  P2    6  150      1500      三级P2 2h/24h：响应✗ 恢复✗
--
-- 依赖 demo.sql 先导入台账（make seed 会按顺序执行）。

WITH h(fp, code, severity, title, days_ago, ack_min, resolve_min) AS (
    VALUES
        ('history-01', 'GOV-APPLY',  'P2', 'GOV-APPLY 接口错误率超过 5%',       20,  12,   95),
        ('history-02', 'GOV-APPLY',  'P1', 'GOV-APPLY 探活失败',                15,  25,  150),
        ('history-03', 'GOV-OA',     'P3', '协同办公公文流转缓慢',              9,   40,  300),
        ('history-04', 'GOV-APPLY',  'P2', 'GOV-APPLY 接口 P95 延迟超过 1s',     4,   45,  200),
        ('history-05', 'GOV-AUTH',   'P1', '统一身份认证登录失败',              18,   8,   70),
        ('history-06', 'GOV-AUTH',   'P2', '统一身份认证令牌刷新失败',          7,   20,  210),
        ('history-07', 'GOV-DATA',   'P2', '数据交换任务积压',                  22,  75,  600),
        ('history-08', 'GOV-DATA',   'P3', '数据共享接口响应超时',              12, 100,  900),
        ('history-09', 'GOV-DATA',   'P2', '部门数据同步中断',                  3,   50,  520),
        ('history-10', 'GOV-PORTAL', 'P3', '门户网站部分页面打不开',            16, 200, 2000),
        ('history-11', 'GOV-PORTAL', 'P2', '门户网站 HTTPS 证书过期',           6,  150, 1500)
),
src AS (
    SELECT h.*,
           s.id AS system_id,
           date_trunc('hour', now()) - make_interval(days => h.days_ago) AS created,
           p.response_minutes,
           p.resolve_minutes,
           COALESCE(left(v.name, 2) || '-' || v.contact, '厂商') AS assignee
    FROM h
    JOIN business_system s ON s.code = h.code
    JOIN sla_policy p ON p.level = s.level AND p.severity = h.severity
    LEFT JOIN vendor v ON v.id = s.vendor_id
    WHERE NOT EXISTS (SELECT 1 FROM ticket t WHERE t.alert_fingerprint = h.fp)
),
ins AS (
    INSERT INTO ticket (
        type, title, system_id, system_label, severity, status, source, alert_fingerprint, assignee,
        created_at, responded_at, resolved_at, closed_at, response_deadline, resolve_deadline
    )
    SELECT 'incident', title, system_id, code, severity, 'closed', 'alert', fp, assignee,
           created,
           created + make_interval(mins => ack_min),
           created + make_interval(mins => resolve_min),
           created + make_interval(mins => resolve_min + 30),
           created + make_interval(mins => response_minutes),
           created + make_interval(mins => resolve_minutes)
    FROM src
    RETURNING id, assignee, created_at, responded_at, resolved_at, closed_at
)
INSERT INTO ticket_log (ticket_id, actor, action, from_status, to_status, note, created_at)
SELECT id, 'alertmanager', 'created', NULL, 'new', '告警触发，自动建单（历史演示数据）', created_at FROM ins
UNION ALL
SELECT id, assignee, 'ack', 'new', 'acknowledged', '已收到，排查中', responded_at FROM ins
UNION ALL
SELECT id, assignee, 'resolve', 'acknowledged', 'resolved', '已处理恢复', resolved_at FROM ins
UNION ALL
SELECT id, '甲方-值班长', 'close', 'resolved', 'closed', '确认恢复', closed_at FROM ins;
