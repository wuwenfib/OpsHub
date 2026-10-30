// Package cmdb 是台账的领域逻辑：输入的规范化与校验。不访问数据库。
package cmdb

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
)

// systemCodePattern：大写字母开头，大写字母/数字/连字符，2～32 位，例如 GOV-APPLY。
// 它要和 Prometheus 告警的 system 标签逐字相等，所以格式收紧，避免大小写、空格之类的歧义。
var systemCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9-]{1,31}$`)

type VendorInput struct {
	Name        string `json:"name"`
	Contact     string `json:"contact"`
	Phone       string `json:"phone"`
	ContractEnd string `json:"contract_end"` // YYYY-MM-DD，可为空
}

// Validate 去掉首尾空白并校验；通过后 ContractEndDate 可安全调用。
func (in *VendorInput) Validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Contact = strings.TrimSpace(in.Contact)
	in.Phone = strings.TrimSpace(in.Phone)
	in.ContractEnd = strings.TrimSpace(in.ContractEnd)
	if in.Name == "" {
		return invalid("name 不能为空")
	}
	if _, _, err := in.ContractEndDate(); err != nil {
		return err
	}
	return nil
}

// ContractEndDate 解析合同到期日；ok=false 表示未填写。
func (in *VendorInput) ContractEndDate() (t time.Time, ok bool, err error) {
	if in.ContractEnd == "" {
		return time.Time{}, false, nil
	}
	t, err = time.Parse(time.DateOnly, in.ContractEnd)
	if err != nil {
		return time.Time{}, false, invalid("contract_end 格式应为 YYYY-MM-DD")
	}
	return t, true, nil
}

type SystemInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Department  string `json:"department"`
	Level       int    `json:"level"` // 重要等级 1～3，1 最高
	VendorID    *int64 `json:"vendor_id"`
	Owner       string `json:"owner"`
	Description string `json:"description"`
}

func (in *SystemInput) Validate() error {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Department = strings.TrimSpace(in.Department)
	in.Owner = strings.TrimSpace(in.Owner)
	if err := ValidateCode(in.Code); err != nil {
		return err
	}
	if in.Name == "" {
		return invalid("name 不能为空")
	}
	if in.Level < 1 || in.Level > 3 {
		return invalid("level 必须是 1～3，当前为 %d", in.Level)
	}
	if in.VendorID != nil && *in.VendorID <= 0 {
		return invalid("vendor_id 必须为正数")
	}
	return nil
}

func ValidateCode(code string) error {
	if !systemCodePattern.MatchString(code) {
		return invalid("code %q 不合法：需大写字母开头，只含大写字母、数字、连字符，2～32 位", code)
	}
	return nil
}

type HostInput struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Env      string `json:"env"` // prod | test
}

func (in *HostInput) Validate() error {
	addr, err := netip.ParseAddr(strings.TrimSpace(in.IP))
	if err != nil {
		return invalid("ip %q 不合法", in.IP)
	}
	in.IP = addr.String() // 统一格式，避免同一地址不同写法绕过唯一约束
	in.Hostname = strings.TrimSpace(in.Hostname)
	in.OS = strings.TrimSpace(in.OS)
	if in.Hostname == "" {
		return invalid("hostname 不能为空")
	}
	if in.Env != "prod" && in.Env != "test" {
		return invalid("env 必须是 prod 或 test")
	}
	return nil
}

// NormalizeDependencies 去重并校验依赖编码列表；系统不能依赖自己。
func NormalizeDependencies(self string, codes []string) ([]string, error) {
	return dedupe(codes, func(c string) error {
		if c == self {
			return invalid("系统不能依赖自己")
		}
		return ValidateCode(c)
	})
}

// NormalizeHostIDs 去重并校验主机 ID 列表。
func NormalizeHostIDs(ids []int64) ([]int64, error) {
	return dedupe(ids, func(id int64) error {
		if id <= 0 {
			return invalid("host id 必须为正数")
		}
		return nil
	})
}

func dedupe[T comparable](in []T, check func(T) error) ([]T, error) {
	seen := make(map[T]bool, len(in))
	out := make([]T, 0, len(in))
	for _, v := range in {
		if err := check(v); err != nil {
			return nil, err
		}
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out, nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", apperr.ErrValidation, fmt.Sprintf(format, args...))
}
