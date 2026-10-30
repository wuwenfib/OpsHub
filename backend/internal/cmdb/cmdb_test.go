package cmdb

import (
	"errors"
	"slices"
	"testing"

	"github.com/fh/opshub/backend/internal/apperr"
)

// checkErr 断言 err 是否为校验错误。
func checkErr(t *testing.T, err error, wantErr bool) {
	t.Helper()
	if wantErr {
		if !errors.Is(err, apperr.ErrValidation) {
			t.Fatalf("want ErrValidation, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

func TestSystemInputValidate(t *testing.T) {
	ok := SystemInput{Code: "GOV-APPLY", Name: "一网通办", Level: 1}
	tests := []struct {
		name    string
		mutate  func(*SystemInput)
		wantErr bool
	}{
		{"valid", func(*SystemInput) {}, false},
		{"valid with vendor", func(s *SystemInput) { s.VendorID = ptr(int64(3)) }, false},
		{"code trimmed", func(s *SystemInput) { s.Code = "  GOV-APPLY " }, false},
		{"code lowercase", func(s *SystemInput) { s.Code = "gov-apply" }, true},
		{"code starts with digit", func(s *SystemInput) { s.Code = "1GOV" }, true},
		{"code with space", func(s *SystemInput) { s.Code = "GOV APPLY" }, true},
		{"code too short", func(s *SystemInput) { s.Code = "G" }, true},
		{"empty name", func(s *SystemInput) { s.Name = "  " }, true},
		{"level 0", func(s *SystemInput) { s.Level = 0 }, true},
		{"level 4", func(s *SystemInput) { s.Level = 4 }, true},
		{"vendor id 0", func(s *SystemInput) { s.VendorID = ptr(int64(0)) }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ok
			tt.mutate(&in)
			checkErr(t, in.Validate(), tt.wantErr)
		})
	}
}

func TestVendorInputValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      VendorInput
		wantErr bool
	}{
		{"valid", VendorInput{Name: "星河软件", ContractEnd: "2027-06-30"}, false},
		{"no contract end", VendorInput{Name: "星河软件"}, false},
		{"empty name", VendorInput{Name: " "}, true},
		{"bad date", VendorInput{Name: "星河软件", ContractEnd: "2027/06/30"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkErr(t, tt.in.Validate(), tt.wantErr)
		})
	}
}

func TestHostInputValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      HostInput
		wantErr bool
		wantIP  string
	}{
		{"ipv4", HostInput{IP: "10.10.1.11", Hostname: "app01", Env: "prod"}, false, "10.10.1.11"},
		{"ipv6 normalized", HostInput{IP: "2001:DB8::0001", Hostname: "app01", Env: "test"}, false, "2001:db8::1"},
		{"bad ip", HostInput{IP: "10.10.1.300", Hostname: "app01", Env: "prod"}, true, ""},
		{"empty hostname", HostInput{IP: "10.10.1.11", Env: "prod"}, true, ""},
		{"bad env", HostInput{IP: "10.10.1.11", Hostname: "app01", Env: "dev"}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate()
			checkErr(t, err, tt.wantErr)
			if err == nil && tt.in.IP != tt.wantIP {
				t.Errorf("ip = %q, want %q", tt.in.IP, tt.wantIP)
			}
		})
	}
}

func TestNormalizeDependencies(t *testing.T) {
	got, err := NormalizeDependencies("GOV-APPLY", []string{"GOV-AUTH", "GOV-DATA", "GOV-AUTH"})
	checkErr(t, err, false)
	if want := []string{"GOV-AUTH", "GOV-DATA"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	_, err = NormalizeDependencies("GOV-APPLY", []string{"GOV-APPLY"})
	checkErr(t, err, true)

	_, err = NormalizeDependencies("GOV-APPLY", []string{"bad code"})
	checkErr(t, err, true)

	got, err = NormalizeDependencies("GOV-APPLY", nil)
	checkErr(t, err, false)
	if len(got) != 0 {
		t.Errorf("empty input should give empty result, got %v", got)
	}
}

func TestNormalizeHostIDs(t *testing.T) {
	got, err := NormalizeHostIDs([]int64{3, 1, 3})
	checkErr(t, err, false)
	if want := []int64{3, 1}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	_, err = NormalizeHostIDs([]int64{1, -2})
	checkErr(t, err, true)
}
