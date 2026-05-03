package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/varcharC2k/vape-crm/internal/models"
)

func newSampleSale(productID int64) *models.Sale {
	return &models.Sale{
		TransactionDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		ProductID:       productID,
		CustomerName:    "홍길동",
		CustomerMobile:  "010-1111-2222",
		Quantity:        2,
		PaymentMethod:   "현금",
		Memo:            "단골",
	}
}

func TestSale_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewSaleRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	s := newSampleSale(prod.ID)
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.Create(ctx, tx, s); err != nil {
			t.Fatalf("Create 실패: %v", err)
		}
	})
	if s.ID == 0 {
		t.Fatal("Create 후 ID 가 0")
	}

	got, err := repo.GetByID(ctx, s.ID)
	if err != nil {
		t.Fatalf("GetByID 실패: %v", err)
	}
	if got.ProductID != prod.ID {
		t.Errorf("ProductID: got=%d want=%d", got.ProductID, prod.ID)
	}
	if got.ProductName != "망고" {
		t.Errorf("ProductName(JOIN): got=%q want=망고", got.ProductName)
	}
	if got.ProductCategory != models.CategoryLiquid {
		t.Errorf("ProductCategory(JOIN): got=%d want=%d", got.ProductCategory, models.CategoryLiquid)
	}
	if got.CustomerName != "홍길동" || got.CustomerMobile != "010-1111-2222" {
		t.Errorf("고객 스냅샷 미일치: got=%+v", got)
	}
	if got.Quantity != 2 {
		t.Errorf("Quantity: got=%d want=2", got.Quantity)
	}
	if got.PaymentMethod != "현금" || got.Memo != "단골" {
		t.Errorf("문자열 필드 미일치: got=%+v", got)
	}
	if !got.TransactionDate.Equal(s.TransactionDate) {
		t.Errorf("Date: got=%v want=%v", got.TransactionDate, s.TransactionDate)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt 이 zero — DB default 적용 안 됨")
	}
}

// TestSale_CreateMinimal — 비회원 매출 (고객 정보 없음).
// customer_name/customer_mobile 이 NULL 로 저장되고 빈 문자열로 복구되어야 함.
func TestSale_CreateMinimal(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewSaleRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	s := &models.Sale{
		TransactionDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		ProductID:       prod.ID,
		Quantity:        1,
		PaymentMethod:   "현금",
	}
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.Create(ctx, tx, s); err != nil {
			t.Fatalf("Create 실패: %v", err)
		}
	})

	got, _ := repo.GetByID(ctx, s.ID)
	if got.CustomerName != "" || got.CustomerMobile != "" {
		t.Errorf("비회원 매출인데 고객 정보가 복구됨: %+v", got)
	}
}

func TestSale_GetByIDNotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewSaleRepository(newTestDB(t))

	if _, err := repo.GetByID(ctx, 9999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("없는 ID 조회: sql.ErrNoRows 기대 got=%v", err)
	}
}

func TestSale_ListSortedByDateDesc(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewSaleRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	dates := []time.Time{
		time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC),
	}
	for _, d := range dates {
		s := newSampleSale(prod.ID)
		s.TransactionDate = d
		runInTx(t, db, func(tx *sql.Tx) {
			_ = repo.Create(ctx, tx, s)
		})
	}

	list, err := repo.List(ctx, SaleFilter{})
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	wantOrder := []time.Time{dates[1], dates[2], dates[0]} // 5/20, 5/15, 5/10
	for i, w := range wantOrder {
		if !list[i].TransactionDate.Equal(w) {
			t.Errorf("정렬 [%d]: got=%v want=%v", i, list[i].TransactionDate, w)
		}
	}
}

func TestSale_ListByDateRange(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewSaleRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	for _, d := range []time.Time{
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC),
	} {
		s := newSampleSale(prod.ID)
		s.TransactionDate = d
		runInTx(t, db, func(tx *sql.Tx) {
			_ = repo.Create(ctx, tx, s)
		})
	}

	from := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	list, err := repo.List(ctx, SaleFilter{DateFrom: &from, DateTo: &to})
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("범위 [5/10~5/20] 결과 개수: got=%d want=1", len(list))
	}
}

func TestSale_ListByProductID(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewSaleRepository(db)
	mango := purchaseTestProduct(t, ctx, db, "망고")
	straw := purchaseTestProduct(t, ctx, db, "딸기")

	runInTx(t, db, func(tx *sql.Tx) {
		_ = repo.Create(ctx, tx, newSampleSale(mango.ID))
		_ = repo.Create(ctx, tx, newSampleSale(mango.ID))
		_ = repo.Create(ctx, tx, newSampleSale(straw.ID))
	})

	list, err := repo.List(ctx, SaleFilter{ProductID: &mango.ID})
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("망고 필터: got=%d want=2", len(list))
	}
}

// TestSale_ListByCustomerMobile — 휴대폰 스냅샷 부분일치 (뒤 4자리 검색).
func TestSale_ListByCustomerMobile(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewSaleRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	mobiles := []string{"010-1111-1234", "010-2222-5678", "010-3333-1234"}
	for _, m := range mobiles {
		s := newSampleSale(prod.ID)
		s.CustomerMobile = m
		runInTx(t, db, func(tx *sql.Tx) {
			_ = repo.Create(ctx, tx, s)
		})
	}
	// 비회원 매출도 하나
	s := newSampleSale(prod.ID)
	s.CustomerName = ""
	s.CustomerMobile = ""
	runInTx(t, db, func(tx *sql.Tx) {
		_ = repo.Create(ctx, tx, s)
	})

	list, err := repo.List(ctx, SaleFilter{CustomerMobile: "1234"})
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("뒤 4자리 1234 매칭: got=%d want=2 (비회원 제외)", len(list))
	}
}

func TestSale_Update(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewSaleRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	s := newSampleSale(prod.ID)
	runInTx(t, db, func(tx *sql.Tx) {
		_ = repo.Create(ctx, tx, s)
	})

	s.Quantity = 9
	s.Memo = "수정된 비고"
	s.CustomerName = "" // 비회원으로 변경
	s.CustomerMobile = ""
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.Update(ctx, tx, s); err != nil {
			t.Fatalf("Update 실패: %v", err)
		}
	})

	got, _ := repo.GetByID(ctx, s.ID)
	if got.Quantity != 9 || got.Memo != "수정된 비고" {
		t.Errorf("Update 미반영: %+v", got)
	}
	if got.CustomerName != "" || got.CustomerMobile != "" {
		t.Errorf("고객 정보 NULL 변환 안됨: %+v", got)
	}
}

func TestSale_UpdateNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewSaleRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	s := newSampleSale(prod.ID)
	s.ID = 9999
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.Update(ctx, tx, s); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("없는 ID Update: sql.ErrNoRows 기대 got=%v", err)
		}
	})
}

func TestSale_Delete(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewSaleRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	s := newSampleSale(prod.ID)
	runInTx(t, db, func(tx *sql.Tx) {
		_ = repo.Create(ctx, tx, s)
	})
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.Delete(ctx, tx, s.ID); err != nil {
			t.Fatalf("Delete 실패: %v", err)
		}
	})
	if _, err := repo.GetByID(ctx, s.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("삭제 후 GetByID: sql.ErrNoRows 기대 got=%v", err)
	}
}

func TestSale_DeleteNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewSaleRepository(db)
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.Delete(ctx, tx, 9999); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("없는 ID Delete: sql.ErrNoRows 기대 got=%v", err)
		}
	})
}
