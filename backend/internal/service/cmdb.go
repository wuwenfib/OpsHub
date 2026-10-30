// Package service 编排用例：调用领域层做校验/决策，在事务里读写 store，并把数据库错误翻译成业务错误。
package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fh/opshub/backend/internal/apperr"
	"github.com/fh/opshub/backend/internal/cmdb"
	"github.com/fh/opshub/backend/internal/store"
)

type CMDB struct {
	st *store.Store
}

func NewCMDB(st *store.Store) *CMDB {
	return &CMDB{st: st}
}

// SystemDetail 是系统详情：系统本身 + 厂商 + 部署主机 + 上下游依赖。
type SystemDetail struct {
	store.BusinessSystem
	Vendor     *store.Vendor             `json:"vendor"`
	Hosts      []store.Host              `json:"hosts"`
	DependsOn  []store.ListDependsOnRow  `json:"depends_on"`  // 我依赖谁
	DependedBy []store.ListDependedByRow `json:"depended_by"` // 谁依赖我
}

// ---------- 厂商 ----------

func (s *CMDB) ListVendors(ctx context.Context) ([]store.Vendor, error) {
	return s.st.ListVendors(ctx)
}

func (s *CMDB) GetVendor(ctx context.Context, id int64) (store.Vendor, error) {
	v, err := s.st.GetVendor(ctx, id)
	return v, store.MapError(err, fmt.Sprintf("vendor %d", id))
}

func (s *CMDB) CreateVendor(ctx context.Context, in cmdb.VendorInput) (store.Vendor, error) {
	if err := in.Validate(); err != nil {
		return store.Vendor{}, err
	}
	v, err := s.st.CreateVendor(ctx, store.CreateVendorParams{
		Name: in.Name, Contact: in.Contact, Phone: in.Phone, ContractEnd: contractEnd(&in),
	})
	return v, store.MapError(err, "vendor "+in.Name)
}

func (s *CMDB) UpdateVendor(ctx context.Context, id int64, in cmdb.VendorInput) (store.Vendor, error) {
	if err := in.Validate(); err != nil {
		return store.Vendor{}, err
	}
	v, err := s.st.UpdateVendor(ctx, store.UpdateVendorParams{
		ID: id, Name: in.Name, Contact: in.Contact, Phone: in.Phone, ContractEnd: contractEnd(&in),
	})
	return v, store.MapError(err, fmt.Sprintf("vendor %d", id))
}

// DeleteVendor 删除厂商；它负责的系统 vendor_id 会被置空（见迁移里的 ON DELETE SET NULL）。
func (s *CMDB) DeleteVendor(ctx context.Context, id int64) error {
	n, err := s.st.DeleteVendor(ctx, id)
	return deleted(n, err, fmt.Sprintf("vendor %d", id))
}

// ---------- 业务系统 ----------

func (s *CMDB) ListSystems(ctx context.Context) ([]store.ListSystemsRow, error) {
	return s.st.ListSystems(ctx)
}

func (s *CMDB) GetSystem(ctx context.Context, code string) (SystemDetail, error) {
	return getSystemDetail(ctx, s.st.Queries, code)
}

func (s *CMDB) CreateSystem(ctx context.Context, in cmdb.SystemInput) (store.BusinessSystem, error) {
	if err := in.Validate(); err != nil {
		return store.BusinessSystem{}, err
	}
	sys, err := s.st.CreateSystem(ctx, store.CreateSystemParams{
		Code: in.Code, Name: in.Name, Department: in.Department, Level: int16(in.Level),
		VendorID: int8Ptr(in.VendorID), Owner: in.Owner, Description: in.Description,
	})
	return sys, store.MapError(err, "system "+in.Code)
}

// UpdateSystem 按编码更新；编码本身不可修改（它是告警关联的键）。
func (s *CMDB) UpdateSystem(ctx context.Context, code string, in cmdb.SystemInput) (store.BusinessSystem, error) {
	in.Code = code
	if err := in.Validate(); err != nil {
		return store.BusinessSystem{}, err
	}
	sys, err := s.st.UpdateSystemByCode(ctx, store.UpdateSystemByCodeParams{
		Code: in.Code, Name: in.Name, Department: in.Department, Level: int16(in.Level),
		VendorID: int8Ptr(in.VendorID), Owner: in.Owner, Description: in.Description,
	})
	return sys, store.MapError(err, "system "+code)
}

func (s *CMDB) DeleteSystem(ctx context.Context, code string) error {
	n, err := s.st.DeleteSystemByCode(ctx, code)
	return deleted(n, err, "system "+code)
}

// SetSystemHosts 整体替换系统的部署主机。先清空再插入，在同一事务里完成，失败则原关系不变。
func (s *CMDB) SetSystemHosts(ctx context.Context, code string, hostIDs []int64) (SystemDetail, error) {
	ids, err := cmdb.NormalizeHostIDs(hostIDs)
	if err != nil {
		return SystemDetail{}, err
	}
	var detail SystemDetail
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		sys, err := q.GetSystemByCode(ctx, code)
		if err != nil {
			return store.MapError(err, "system "+code)
		}
		if err := q.ClearSystemHosts(ctx, sys.ID); err != nil {
			return err
		}
		if _, err := q.AddSystemHosts(ctx, store.AddSystemHostsParams{SystemID: sys.ID, HostIDs: ids}); err != nil {
			return store.MapError(err, "host")
		}
		detail, err = getSystemDetail(ctx, q, code)
		return err
	})
	return detail, err
}

// SetSystemDependencies 整体替换系统依赖的上游系统列表（按编码）。
func (s *CMDB) SetSystemDependencies(ctx context.Context, code string, dependsOn []string) (SystemDetail, error) {
	codes, err := cmdb.NormalizeDependencies(code, dependsOn)
	if err != nil {
		return SystemDetail{}, err
	}
	var detail SystemDetail
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		sys, err := q.GetSystemByCode(ctx, code)
		if err != nil {
			return store.MapError(err, "system "+code)
		}
		if err := q.ClearSystemDependencies(ctx, sys.ID); err != nil {
			return err
		}
		n, err := q.AddSystemDependencies(ctx, store.AddSystemDependenciesParams{SystemID: sys.ID, Codes: codes})
		if err != nil {
			return store.MapError(err, "dependency")
		}
		detail, err = getSystemDetail(ctx, q, code)
		if err != nil {
			return err
		}
		if int(n) != len(codes) { // 有编码没匹配上：找出来报给调用方，事务回滚
			return fmt.Errorf("%w: 依赖的系统不存在: %s", apperr.ErrValidation, strings.Join(missing(codes, detail.DependsOn), ", "))
		}
		return nil
	})
	return detail, err
}

// ---------- 主机 ----------

func (s *CMDB) ListHosts(ctx context.Context) ([]store.Host, error) {
	return s.st.ListHosts(ctx)
}

func (s *CMDB) CreateHost(ctx context.Context, in cmdb.HostInput) (store.Host, error) {
	if err := in.Validate(); err != nil {
		return store.Host{}, err
	}
	h, err := s.st.CreateHost(ctx, store.CreateHostParams{IP: in.IP, Hostname: in.Hostname, OS: in.OS, Env: in.Env})
	return h, store.MapError(err, "host "+in.IP)
}

func (s *CMDB) UpdateHost(ctx context.Context, id int64, in cmdb.HostInput) (store.Host, error) {
	if err := in.Validate(); err != nil {
		return store.Host{}, err
	}
	h, err := s.st.UpdateHost(ctx, store.UpdateHostParams{ID: id, IP: in.IP, Hostname: in.Hostname, OS: in.OS, Env: in.Env})
	return h, store.MapError(err, fmt.Sprintf("host %d", id))
}

func (s *CMDB) DeleteHost(ctx context.Context, id int64) error {
	n, err := s.st.DeleteHost(ctx, id)
	return deleted(n, err, fmt.Sprintf("host %d", id))
}

// ---------- 辅助 ----------

// getSystemDetail 接收 *store.Queries，因此既能在事务里用，也能直接走连接池。
func getSystemDetail(ctx context.Context, q *store.Queries, code string) (SystemDetail, error) {
	sys, err := q.GetSystemByCode(ctx, code)
	if err != nil {
		return SystemDetail{}, store.MapError(err, "system "+code)
	}
	d := SystemDetail{BusinessSystem: sys}
	if sys.VendorID.Valid {
		v, err := q.GetVendor(ctx, sys.VendorID.Int64)
		if err != nil {
			return SystemDetail{}, err
		}
		d.Vendor = &v
	}
	if d.Hosts, err = q.ListHostsBySystem(ctx, sys.ID); err != nil {
		return SystemDetail{}, err
	}
	if d.DependsOn, err = q.ListDependsOn(ctx, sys.ID); err != nil {
		return SystemDetail{}, err
	}
	if d.DependedBy, err = q.ListDependedBy(ctx, sys.ID); err != nil {
		return SystemDetail{}, err
	}
	return d, nil
}

func deleted(rows int64, err error, what string) error {
	if err != nil {
		return store.MapError(err, what)
	}
	if rows == 0 {
		return fmt.Errorf("%w: %s", apperr.ErrNotFound, what)
	}
	return nil
}

func missing(want []string, got []store.ListDependsOnRow) []string {
	var out []string
	for _, c := range want {
		if !slices.ContainsFunc(got, func(r store.ListDependsOnRow) bool { return r.Code == c }) {
			out = append(out, c)
		}
	}
	return out
}

func contractEnd(in *cmdb.VendorInput) pgtype.Date {
	t, ok, _ := in.ContractEndDate() // Validate 已确保格式正确
	return pgtype.Date{Time: t, Valid: ok}
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
