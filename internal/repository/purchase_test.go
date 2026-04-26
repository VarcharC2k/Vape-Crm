package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/varcharC2k/vape-crm/internal/models"
)

// purchaseTestProduct — FK 제약을 만족시키기 위한 테스트용 품목 생성.
func purchaseTestProduct(t *testing.T, ctx context.Context, db *sql.DB, name string) *models.Product {
	t.Helper()
	repo := NewProductRepository(db)
	p := &models.Product{
		Category:  models.CategoryLiquid,
		Name:      name,
		SalePrice: 10000,
		StockQty:  0,
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("테스트 품목 생성(%s) 실패: %v", name, err)
	}
	return p
}

// runInTx — 테스트에서 트랜잭션을 열어 fn 실행 후 commit.
// 실패 시 t.Fatal 로 종료되어 자동으로 rollback (Cleanup 의 Close 에 위임).
func runInTx(t *testing.T, db *sql.DB, fn func(tx *sql.Tx)) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin 실패: %v", err)
	}
	fn(tx)
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit 실패: %v", err)
	}
}

func newSamplePurchase(productID int64) *models.Purchase {
	return &models.Purchase{
		TransactionDate: time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
		ProductID:       productID,
		Quantity:        10,
		Amount:          50000,
		PaymentMethod:   "현금",
		Memo:            "정기 매입",
	}
}

func TestPurchase_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewPurchaseRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	p := newSamplePurchase(prod.ID)

	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.Create(ctx, tx, p); err != nil {
			t.Fatalf("Create 실패: %v", err)
		}
	})
	if p.ID == 0 {
		t.Fatal("Create 후 ID 가 0")
	}

	got, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID 실패: %v", err)
	}
	if got.ProductID != prod.ID {
		t.Errorf("ProductID: got=%d want=%d", got.ProductID, prod.ID)
	}
	if got.ProductName != "망고" {
		t.Errorf("ProductName(JOIN): got=%q want=%q", got.ProductName, "망고")
	}
	if got.Quantity != 10 || got.Amount != 50000 {
		t.Errorf("값 미일치: got=%+v", got)
	}
	if got.PaymentMethod != "현금" || got.Memo != "정기 매입" {
		t.Errorf("문자열 필드 미일치: got=%+v", got)
	}
	if !got.TransactionDate.Equal(p.TransactionDate) {
		t.Errorf("Date: got=%v want=%v", got.TransactionDate, p.TransactionDate)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt 이 zero — DB default 적용 안 됨")
	}
}

func TestPurchase_GetByIDNotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewPurchaseRepository(newTestDB(t))

	if _, err := repo.GetByID(ctx, 9999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("없는 ID 조회: sql.ErrNoRows 기대 got=%v", err)
	}
}

func TestPurchase_ListSortedByDateDesc(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewPurchaseRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	dates := []time.Time{
		time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
	}
	for _, d := range dates {
		p := newSamplePurchase(prod.ID)
		p.TransactionDate = d
		runInTx(t, db, func(tx *sql.Tx) {
			if err := repo.Create(ctx, tx, p); err != nil {
				t.Fatalf("Create 실패: %v", err)
			}
		})
	}

	list, err := repo.List(ctx, PurchaseFilter{})
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("개수: got=%d want=3", len(list))
	}
	wantOrder := []time.Time{dates[1], dates[2], dates[0]} // 4/20, 4/15, 4/10
	for i, w := range wantOrder {
		if !list[i].TransactionDate.Equal(w) {
			t.Errorf("정렬 [%d]: got=%v want=%v", i, list[i].TransactionDate, w)
		}
	}
}

func TestPurchase_ListByDateRange(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewPurchaseRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	for _, d := range []time.Time{
		time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
	} {
		p := newSamplePurchase(prod.ID)
		p.TransactionDate = d
		runInTx(t, db, func(tx *sql.Tx) {
			_ = repo.Create(ctx, tx, p)
		})
	}

	from := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	list, err := repo.List(ctx, PurchaseFilter{DateFrom: &from, DateTo: &to})
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("범위 [4/10~4/20] 결과 개수: got=%d want=1", len(list))
	}
	if !list[0].TransactionDate.Equal(time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("결과 날짜 잘못됨: %v", list[0].TransactionDate)
	}
}

func TestPurchase_ListByProductID(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewPurchaseRepository(db)
	mango := purchaseTestProduct(t, ctx, db, "망고")
	straw := purchaseTestProduct(t, ctx, db, "딸기")

	runInTx(t, db, func(tx *sql.Tx) {
		_ = repo.Create(ctx, tx, newSamplePurchase(mango.ID))
		_ = repo.Create(ctx, tx, newSamplePurchase(mango.ID))
		_ = repo.Create(ctx, tx, newSamplePurchase(straw.ID))
	})

	list, err := repo.List(ctx, PurchaseFilter{ProductID: &mango.ID})
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("망고 필터: got=%d want=2", len(list))
	}
	for _, p := range list {
		if p.ProductID != mango.ID {
			t.Errorf("다른 품목 섞임: %+v", p)
		}
	}
}

func TestPurchase_Update(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewPurchaseRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	p := newSamplePurchase(prod.ID)
	runInTx(t, db, func(tx *sql.Tx) {
		_ = repo.Create(ctx, tx, p)
	})

	p.Quantity = 99
	p.Amount = 99000
	p.Memo = "수정된 비고"
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.Update(ctx, tx, p); err != nil {
			t.Fatalf("Update 실패: %v", err)
		}
	})

	got, _ := repo.GetByID(ctx, p.ID)
	if got.Quantity != 99 || got.Amount != 99000 || got.Memo != "수정된 비고" {
		t.Errorf("Update 미반영: %+v", got)
	}
}

func TestPurchase_UpdateNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewPurchaseRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	p := newSamplePurchase(prod.ID)
	p.ID = 9999
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.Update(ctx, tx, p); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("없는 ID Update: sql.ErrNoRows 기대 got=%v", err)
		}
	})
}

func TestPurchase_Delete(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewPurchaseRepository(db)
	prod := purchaseTestProduct(t, ctx, db, "망고")

	p := newSamplePurchase(prod.ID)
	runInTx(t, db, func(tx *sql.Tx) {
		_ = repo.Create(ctx, tx, p)
	})
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.Delete(ctx, tx, p.ID); err != nil {
			t.Fatalf("Delete 실패: %v", err)
		}
	})
	if _, err := repo.GetByID(ctx, p.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("삭제 후 GetByID: sql.ErrNoRows 기대 got=%v", err)
	}
}

func TestPurchase_DeleteNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewPurchaseRepository(db)
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.Delete(ctx, tx, 9999); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("없는 ID Delete: sql.ErrNoRows 기대 got=%v", err)
		}
	})
}

// TestProduct_AdjustStock — ProductRepository 의 새 메서드.
// 트랜잭션 안에서 +/- 양쪽 작동하는지 확인.
func TestProduct_AdjustStock(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewProductRepository(db)

	p := &models.Product{Category: models.CategoryLiquid, Name: "테스트", SalePrice: 1000, StockQty: 5}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	// +7 → 12
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.AdjustStock(ctx, tx, p.ID, 7); err != nil {
			t.Fatal(err)
		}
	})
	if got, _ := repo.GetByID(ctx, p.ID); got.StockQty != 12 {
		t.Errorf("+7 후 재고: got=%d want=12", got.StockQty)
	}

	// -3 → 9
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.AdjustStock(ctx, tx, p.ID, -3); err != nil {
			t.Fatal(err)
		}
	})
	if got, _ := repo.GetByID(ctx, p.ID); got.StockQty != 9 {
		t.Errorf("-3 후 재고: got=%d want=9", got.StockQty)
	}

	// 없는 ID
	runInTx(t, db, func(tx *sql.Tx) {
		if err := repo.AdjustStock(ctx, tx, 9999, 1); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("없는 ID AdjustStock: sql.ErrNoRows 기대 got=%v", err)
		}
	})
}
