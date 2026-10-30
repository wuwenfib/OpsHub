package service

import "time"

// Clock 提供当前时间。业务代码不直接调 time.Now()，测试时可以注入固定时间，
// 从而精确断言 created_at、deadline 等字段。
type Clock interface {
	Now() time.Time
}

// RealClock 是生产环境使用的系统时钟，统一返回 UTC。
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }
