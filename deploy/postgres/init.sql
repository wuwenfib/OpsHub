-- 仅在数据卷首次初始化时执行。主库 opshub 由 POSTGRES_DB 创建，这里补一个集成测试用的库。
CREATE DATABASE opshub_test OWNER opshub;
