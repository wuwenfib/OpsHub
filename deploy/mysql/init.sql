-- 给 mysqld_exporter 用的最小权限账号：只能看状态，不能改数据。
CREATE USER IF NOT EXISTS 'exporter'@'%' IDENTIFIED BY 'exporter' WITH MAX_USER_CONNECTIONS 3;
GRANT PROCESS, REPLICATION CLIENT, SELECT ON *.* TO 'exporter'@'%';
