package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/varcharC2k/vape-crm/internal/models"
)

func newSampleCustomer(name string) *models.Customer {
	return &models.Customer{
		Name:             name,
		Mobile:           "010-0000-0000", // 테스트마다 다른 번호 줄 수도 있어 호출자가 덮어씀
		Email:            "test@example.com",
		Address:          "서울특별시 강남구",
		Memo:             "단골",
		BirthDate:        time.Date(1990, 5, 15, 0, 0, 0, 0, time.UTC),
		RegistrationDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestCustomer_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	c := newSampleCustomer("홍길동")
	c.Mobile = "010-1111-2222"
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create 실패: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("Create 후 ID 가 0")
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID 실패: %v", err)
	}
	if got.Name != "홍길동" {
		t.Errorf("Name: got=%q want=%q", got.Name, "홍길동")
	}
	if got.Mobile != "010-1111-2222" {
		t.Errorf("Mobile: got=%q want=%q", got.Mobile, "010-1111-2222")
	}
	if got.Email != "test@example.com" {
		t.Errorf("Email: got=%q want=test@example.com", got.Email)
	}
	if got.Address != "서울특별시 강남구" {
		t.Errorf("Address: got=%q", got.Address)
	}
	if got.Memo != "단골" {
		t.Errorf("Memo: got=%q", got.Memo)
	}
	if !got.BirthDate.Equal(c.BirthDate) {
		t.Errorf("BirthDate: got=%v want=%v", got.BirthDate, c.BirthDate)
	}
	if !got.RegistrationDate.Equal(c.RegistrationDate) {
		t.Errorf("RegistrationDate: got=%v want=%v", got.RegistrationDate, c.RegistrationDate)
	}
}

// TestCustomer_CreateMinimal — name 과 registration_date 만 채운 최소 등록.
// 나머지 선택 필드는 빈 값/zero time → DB 에 NULL 저장 후 조회 시 빈 값으로 복구되어야 함.
func TestCustomer_CreateMinimal(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	c := &models.Customer{
		Name:             "이름만",
		RegistrationDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create 실패: %v", err)
	}

	got, _ := repo.GetByID(ctx, c.ID)
	if got.Mobile != "" || got.Email != "" || got.Address != "" || got.Memo != "" {
		t.Errorf("선택 필드들이 NULL → 빈 문자열로 복구돼야 함: %+v", got)
	}
	if !got.BirthDate.IsZero() {
		t.Errorf("BirthDate NULL → zero time 기대: %v", got.BirthDate)
	}
}

// TestCustomer_CreateDuplicateMobile — 동일 휴대폰 두 번째 등록 시 실패.
func TestCustomer_CreateDuplicateMobile(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	c1 := newSampleCustomer("첫번째")
	c1.Mobile = "010-2222-3333"
	if err := repo.Create(ctx, c1); err != nil {
		t.Fatalf("첫 Create 실패: %v", err)
	}

	c2 := newSampleCustomer("두번째")
	c2.Mobile = "010-2222-3333" // 중복
	if err := repo.Create(ctx, c2); err == nil {
		t.Fatal("중복 휴대폰인데 에러가 없음 — UNIQUE 제약 확인 필요")
	}
}

// TestCustomer_CreateDuplicateNullMobile — 휴대폰 NULL 인 고객 여러 명 가능 (SQLite UNIQUE 동작).
func TestCustomer_CreateDuplicateNullMobile(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	for _, n := range []string{"무번호1", "무번호2", "무번호3"} {
		c := &models.Customer{
			Name:             n,
			RegistrationDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		}
		if err := repo.Create(ctx, c); err != nil {
			t.Fatalf("Create(%s) 실패 — NULL 휴대폰은 다중 행 허용돼야 함: %v", n, err)
		}
	}
}

func TestCustomer_ListSortedByName(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	for i, name := range []string{"체리", "레몬", "망고"} {
		c := &models.Customer{
			Name:             name,
			Mobile:           fmt.Sprintf("010-9999-%04d", i),
			RegistrationDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		}
		if err := repo.Create(ctx, c); err != nil {
			t.Fatalf("Create(%s) 실패: %v", name, err)
		}
	}

	list, err := repo.List(ctx, CustomerFilter{})
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	want := []string{"레몬", "망고", "체리"}
	for i, n := range want {
		if list[i].Name != n {
			t.Errorf("정렬 [%d]: got=%q want=%q", i, list[i].Name, n)
		}
	}
}

// TestCustomer_ListByName — 이름 부분일치 + 앞쪽 일치 우선 정렬.
func TestCustomer_ListByName(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	for i, n := range []string{"김민수", "이민수", "민수아빠", "박철수"} {
		c := &models.Customer{
			Name:             n,
			Mobile:           fmt.Sprintf("010-7777-%04d", i),
			RegistrationDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		}
		_ = repo.Create(ctx, c)
	}

	list, err := repo.List(ctx, CustomerFilter{Name: "민수"})
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("개수: got=%d want=3 (민수 포함)", len(list))
	}
	// 앞쪽 일치 우선: "민수아빠" 가 1순위
	// 그 다음 부분 일치: "김민수", "이민수" — 이름 ASC 로 김 < 이
	want := []string{"민수아빠", "김민수", "이민수"}
	for i, n := range want {
		if list[i].Name != n {
			t.Errorf("정렬 [%d]: got=%q want=%q", i, list[i].Name, n)
		}
	}
}

// TestCustomer_ListByMobile — 휴대폰 부분일치 (뒤 4자리 검색 시나리오).
func TestCustomer_ListByMobile(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	mobiles := []string{"010-1111-1234", "010-2222-5678", "010-3333-1234"}
	for i, m := range mobiles {
		c := newSampleCustomer(fmt.Sprintf("고객%d", i))
		c.Mobile = m
		_ = repo.Create(ctx, c)
	}

	list, err := repo.List(ctx, CustomerFilter{Mobile: "1234"})
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("뒤 4자리 1234 매칭: got=%d want=2", len(list))
	}
}

func TestCustomer_Update(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	c := newSampleCustomer("수정전")
	c.Mobile = "010-4444-1111"
	if err := repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}

	c.Name = "수정후"
	c.Email = "new@example.com"
	c.BirthDate = time.Time{} // 생일 정보 없음으로 변경 (NULL 로 저장)
	if err := repo.Update(ctx, c); err != nil {
		t.Fatalf("Update 실패: %v", err)
	}

	got, _ := repo.GetByID(ctx, c.ID)
	if got.Name != "수정후" || got.Email != "new@example.com" {
		t.Errorf("Update 미반영: %+v", got)
	}
	if !got.BirthDate.IsZero() {
		t.Errorf("BirthDate zero 로 변경 안됨: %v", got.BirthDate)
	}
}

func TestCustomer_UpdateNotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	c := newSampleCustomer("없음")
	c.ID = 9999
	if err := repo.Update(ctx, c); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("없는 ID Update: sql.ErrNoRows 기대, got=%v", err)
	}
}

func TestCustomer_Delete(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	c := newSampleCustomer("삭제대상")
	c.Mobile = "010-5555-6666"
	if err := repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, c.ID); err != nil {
		t.Fatalf("Delete 실패: %v", err)
	}
	if _, err := repo.GetByID(ctx, c.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("삭제 후 GetByID: sql.ErrNoRows 기대, got=%v", err)
	}
}

func TestCustomer_DeleteNotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	if err := repo.Delete(ctx, 9999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("없는 ID Delete: sql.ErrNoRows 기대, got=%v", err)
	}
}

func TestCustomer_GetByIDNotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewCustomerRepository(newTestDB(t))

	if _, err := repo.GetByID(ctx, 9999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("없는 ID Get: sql.ErrNoRows 기대, got=%v", err)
	}
}
