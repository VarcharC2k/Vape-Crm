package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/varcharC2k/vape-crm/internal/models"
	"github.com/varcharC2k/vape-crm/internal/repository"
)

// CustomerService — repository 위에 검증과 비즈니스 메시지 변환을 얹는다.
type CustomerService struct {
	repo *repository.CustomerRepository
}

func NewCustomerService(repo *repository.CustomerRepository) *CustomerService {
	return &CustomerService{repo: repo}
}

// ErrCustomerNotFound — 없는 ID 조회/수정/삭제.
var ErrCustomerNotFound = errors.New("고객을 찾을 수 없습니다")

// List — 단순 위임.
func (s *CustomerService) List(ctx context.Context, filter repository.CustomerFilter) ([]*models.Customer, error) {
	return s.repo.List(ctx, filter)
}

// Get — 단일 조회. 없으면 ErrCustomerNotFound.
func (s *CustomerService) Get(ctx context.Context, id int64) (*models.Customer, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCustomerNotFound
		}
		return nil, err
	}
	return c, nil
}

// Create — 검증 + INSERT. 휴대폰 중복은 한글 메시지로 변환.
func (s *CustomerService) Create(ctx context.Context, c *models.Customer) error {
	if errs := validateCustomer(c); len(errs) > 0 {
		return errs
	}
	if err := s.repo.Create(ctx, c); err != nil {
		if isUniqueViolation(err) {
			return ValidationErrors{"mobile": "이미 등록된 휴대폰 번호입니다"}
		}
		return err
	}
	return nil
}

// Update — 검증 + UPDATE.
func (s *CustomerService) Update(ctx context.Context, c *models.Customer) error {
	if errs := validateCustomer(c); len(errs) > 0 {
		return errs
	}
	err := s.repo.Update(ctx, c)
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCustomerNotFound
	}
	if isUniqueViolation(err) {
		return ValidationErrors{"mobile": "이미 등록된 휴대폰 번호입니다"}
	}
	return err
}

// Delete — 단순 삭제. 매출 거래가 있어도 허용 (사용자 정책).
// 매출 테이블 추가 시 customer_id 처리 정책(NULL/스냅샷/cascade) 결정 필요.
func (s *CustomerService) Delete(ctx context.Context, id int64) error {
	err := s.repo.Delete(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCustomerNotFound
	}
	return err
}

// validateCustomer — 필드별 검증. 트림된 값을 c 에 반영.
//
// 형식 검증(이메일 @, 휴대폰 숫자/하이픈)은 의도적으로 안 한다 — 매장 자율 관리 + HTML5
// type=email 등 클라단 1차 체크에 위임. 서버는 길이만 보장.
func validateCustomer(c *models.Customer) ValidationErrors {
	errs := ValidationErrors{}

	name := strings.TrimSpace(c.Name)
	c.Name = name
	switch {
	case name == "":
		errs["name"] = "고객명은 필수입니다"
	case utf8.RuneCountInString(name) > 50:
		errs["name"] = "고객명은 50자를 넘을 수 없습니다"
	}

	mobile := strings.TrimSpace(c.Mobile)
	c.Mobile = mobile
	if mobile != "" && utf8.RuneCountInString(mobile) > 20 {
		errs["mobile"] = "휴대폰 번호는 20자를 넘을 수 없습니다"
	}

	email := strings.TrimSpace(c.Email)
	c.Email = email
	if email != "" && utf8.RuneCountInString(email) > 100 {
		errs["email"] = "이메일은 100자를 넘을 수 없습니다"
	}

	address := strings.TrimSpace(c.Address)
	c.Address = address
	if utf8.RuneCountInString(address) > 200 {
		errs["address"] = "주소는 200자를 넘을 수 없습니다"
	}

	if utf8.RuneCountInString(c.Memo) > 500 {
		errs["memo"] = "비고는 500자를 넘을 수 없습니다"
	}

	if c.RegistrationDate.IsZero() {
		errs["registration_date"] = "등록일은 필수입니다"
	}

	return errs
}
