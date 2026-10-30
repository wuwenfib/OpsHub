-- 演示数据：5 个模拟政务系统（名称、厂商、人员、电话均为虚构）。
-- 可重复执行：已存在的记录跳过，不会删除或覆盖手工录入的数据。用法：make seed
BEGIN;

INSERT INTO vendor (name, contact, phone, contract_end) VALUES
    ('星河软件', '张工', '138-0000-0001', '2027-06-30'),
    ('磐石科技', '李工', '138-0000-0002', '2026-12-31'),
    ('数联信息', '王工', '138-0000-0003', '2027-03-31'),
    ('云帆网络', '赵工', '138-0000-0004', '2026-11-30')
ON CONFLICT (name) DO NOTHING;

INSERT INTO business_system (code, name, department, level, vendor_id, owner, description)
SELECT s.code, s.name, s.department, s.level, v.id, s.owner, s.description
FROM (VALUES
    ('GOV-APPLY',  '一网通办政务服务平台', '政务服务管理办公室', 1, '星河软件', '陈主任', '群众和企业网上办事入口，唯一接入监控的演示系统'),
    ('GOV-AUTH',   '统一身份认证平台',     '大数据中心',         1, '磐石科技', '刘科长', '全市政务系统统一登录'),
    ('GOV-DATA',   '数据共享交换平台',     '大数据中心',         2, '数联信息', '刘科长', '部门间数据共享与交换'),
    ('GOV-OA',     '协同办公系统',         '办公厅',             2, '星河软件', '周处长', '公文流转与内部办公'),
    ('GOV-PORTAL', '政府门户网站',         '宣传处',             3, '云帆网络', '吴处长', '对外信息公开门户')
) AS s(code, name, department, level, vendor_name, owner, description)
JOIN vendor v ON v.name = s.vendor_name
ON CONFLICT (code) DO NOTHING;

INSERT INTO host (ip, hostname, os, env) VALUES
    ('10.10.1.11', 'apply-app01',  'Kylin V10', 'prod'),
    ('10.10.1.12', 'apply-app02',  'Kylin V10', 'prod'),
    ('10.10.1.21', 'apply-db01',   'Kylin V10', 'prod'),
    ('10.10.2.11', 'auth-app01',   'UOS 20',    'prod'),
    ('10.10.3.11', 'data-app01',   'Kylin V10', 'prod'),
    ('10.10.4.11', 'oa-app01',     'CentOS 7',  'prod'),
    ('10.10.5.11', 'portal-web01', 'CentOS 7',  'prod')
ON CONFLICT (ip) DO NOTHING;

INSERT INTO system_host (system_id, host_id)
SELECT s.id, h.id
FROM (VALUES
    ('GOV-APPLY', '10.10.1.11'), ('GOV-APPLY', '10.10.1.12'), ('GOV-APPLY', '10.10.1.21'),
    ('GOV-AUTH', '10.10.2.11'),
    ('GOV-DATA', '10.10.3.11'),
    ('GOV-OA', '10.10.4.11'),
    ('GOV-PORTAL', '10.10.5.11')
) AS m(code, ip)
JOIN business_system s ON s.code = m.code
JOIN host h ON h.ip = m.ip
ON CONFLICT DO NOTHING;

INSERT INTO system_dependency (system_id, depends_on_id)
SELECT a.id, b.id
FROM (VALUES
    ('GOV-APPLY', 'GOV-AUTH'),
    ('GOV-APPLY', 'GOV-DATA'),
    ('GOV-OA', 'GOV-AUTH'),
    ('GOV-PORTAL', 'GOV-DATA')
) AS d(code, depends_on)
JOIN business_system a ON a.code = d.code
JOIN business_system b ON b.code = d.depends_on
ON CONFLICT DO NOTHING;

COMMIT;
