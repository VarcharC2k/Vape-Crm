package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/varcharC2k/vape-crm/internal/db"
	"github.com/varcharC2k/vape-crm/internal/models"
	"github.com/varcharC2k/vape-crm/internal/repository"
)

// purchaseTestSetup — 인메모리 DB + 두 repo + service 한 세트로 만들어준다.
func purchaseTestSetup(t *testing.T) (*PurchaseService, *repository.ProductRepository, *sql.DB) {
	t.Helper()

	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("DB Open 실패: %v", err)
	}
	conn.SetMaxOpenConns(1) // :memory: 단일 커넥션 고정 (repository 테스트와 동일 패턴)
	if err := db.Migrate(conn); err != nil {
		conn.Close()
		t.Fatalf("Migrate 실패: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	productRepo := repository.NewProductRepository(conn)
	purchaseRepo := repository.NewPurchaseRepository(conn)
	svc := NewPurchaseService(conn, purchaseRepo, productRepo)
	return svc, productRepo, conn
}

func newTestProductForPurchase(t *testing.T, ctx context.Context, repo *repository.ProductRepository, name string, initialStock int) *models.Product {
	t.Helper()
	p := &models.Product{
		Category:  models.CategoryLiquid,
		Name:      name,
		SalePrice: 10000,
		StockQty:  initialStock,
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create 품목(%s) 실패: %v", name, err)
	}
	return p
}

func samplePurchaseFor(productID int64, qty int) *models.Purchase {
	return &models.Purchase{
		TransactionDate: time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
		ProductID:       productID,
		Quantity:        qty,
		Amount:          int64(qty * 3000),
		PaymentMethod:   "현금",
	}
}

// TestPurchaseService_Create_Stock — 매입 등록 시 재고가 수량만큼 증가.
func TestPurchaseService_Create_Stock(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := purchaseTestSetup(t)
	prod := newTestProductForPurchase(t, ctx, productRepo, "망고", 5)

	if err := svc.Create(ctx, samplePurchaseFor(prod.ID, 10)); err != nil {
		t.Fatalf("Create 실패: %v", err)
	}

	got, _ := productRepo.GetByID(ctx, prod.ID)
	if got.StockQty != 15 {
		t.Errorf("재고: got=%d want=15 (5+10)", got.StockQty)
	}
}

// TestPurchaseService_Update_SameProduct_StockDelta — 같은 품목의 수량 변경 시 차이만큼만 정정.
func TestPurchaseService_Update_SameProduct_StockDelta(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := purchaseTestSetup(t)
	prod := newTestProductForPurchase(t, ctx, productRepo, "망고", 0)

	p := samplePurchaseFor(prod.ID, 10)
	_ = svc.Create(ctx, p) // 재고: 10

	p.Quantity = 7
	if err := svc.Update(ctx, p); err != nil {
		t.Fatalf("Update 실패: %v", err)
	}

	got, _ := productRepo.GetByID(ctx, prod.ID)
	if got.StockQty != 7 {
		t.Errorf("재고: got=%d want=7 (10-3)", got.StockQty)
	}
}

// TestPurchaseService_Update_NoQuantityChange — 수량 변경 없이 다른 필드만 수정 시 재고 그대로.
func TestPurchaseService_Update_NoQuantityChange(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := purchaseTestSetup(t)
	prod := newTestProductForPurchase(t, ctx, productRepo, "망고", 0)

	p := samplePurchaseFor(prod.ID, 10)
	_ = svc.Create(ctx, p)

	p.Memo = "수정"
	p.Amount = 99999
	if err := svc.Update(ctx, p); err != nil {
		t.Fatalf("Update 실패: %v", err)
	}

	got, _ := productRepo.GetByID(ctx, prod.ID)
	if got.StockQty != 10 {
		t.Errorf("재고: got=%d want=10 (수량 동일)", got.StockQty)
	}
}

// TestPurchaseService_Update_ProductChange — 매입의 품목 자체가 바뀌면 양쪽 재고 이동.
func TestPurchaseService_Update_ProductChange(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := purchaseTestSetup(t)
	prodA := newTestProductForPurchase(t, ctx, productRepo, "망고", 0)
	prodB := newTestProductForPurchase(t, ctx, productRepo, "딸기", 0)

	p := samplePurchaseFor(prodA.ID, 10)
	_ = svc.Create(ctx, p) // 망고: 10, 딸기: 0

	p.ProductID = prodB.ID
	if err := svc.Update(ctx, p); err != nil {
		t.Fatalf("Update 실패: %v", err)
	}

	a, _ := productRepo.GetByID(ctx, prodA.ID)
	b, _ := productRepo.GetByID(ctx, prodB.ID)
	if a.StockQty != 0 {
		t.Errorf("이전 품목 재고: got=%d want=0", a.StockQty)
	}
	if b.StockQty != 10 {
		t.Errorf("새 품목 재고: got=%d want=10", b.StockQty)
	}
}

// TestPurchaseService_Update_ProductAndQuantityChange — 품목·수량 동시 변경.
func TestPurchaseService_Update_ProductAndQuantityChange(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := purchaseTestSetup(t)
	prodA := newTestProductForPurchase(t, ctx, productRepo, "망고", 5)
	prodB := newTestProductForPurchase(t, ctx, productRepo, "딸기", 5)

	p := samplePurchaseFor(prodA.ID, 10)
	_ = svc.Create(ctx, p) // 망고: 15, 딸기: 5

	p.ProductID = prodB.ID
	p.Quantity = 3
	if err := svc.Update(ctx, p); err != nil {
		t.Fatalf("Update 실패: %v", err)
	}

	a, _ := productRepo.GetByID(ctx, prodA.ID)
	b, _ := productRepo.GetByID(ctx, prodB.ID)
	if a.StockQty != 5 {
		t.Errorf("망고: got=%d want=5 (15-10)", a.StockQty)
	}
	if b.StockQty != 8 {
		t.Errorf("딸기: got=%d want=8 (5+3)", b.StockQty)
	}
}

// TestPurchaseService_Delete_Stock — 매입 삭제 시 재고가 수량만큼 감소.
func TestPurchaseService_Delete_Stock(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := purchaseTestSetup(t)
	prod := newTestProductForPurchase(t, ctx, productRepo, "망고", 0)

	p := samplePurchaseFor(prod.ID, 10)
	_ = svc.Create(ctx, p) // 재고: 10

	if err := svc.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete 실패: %v", err)
	}

	got, _ := productRepo.GetByID(ctx, prod.ID)
	if got.StockQty != 0 {
		t.Errorf("재고: got=%d want=0 (10-10)", got.StockQty)
	}

	if _, err := svc.Get(ctx, p.ID); !errors.Is(err, ErrPurchaseNotFound) {
		t.Errorf("삭제 후 Get: ErrPurchaseNotFound 기대 got=%v", err)
	}
}

// TestPurchaseService_Validation — 필드별 검증.
func TestPurchaseService_Validation(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := purchaseTestSetup(t)
	prod := newTestProductForPurchase(t, ctx, productRepo, "망고", 0)

	base := func() *models.Purchase {
		return samplePurchaseFor(prod.ID, 10)
	}

	cases := []struct {
		name  string
		mod   func(*models.Purchase)
		field string
	}{
		{"날짜 zero", func(p *models.Purchase) { p.TransactionDate = time.Time{} }, "transaction_date"},
		{"품목 ID 0", func(p *models.Purchase) { p.ProductID = 0 }, "product_id"},
		{"수량 0", func(p *models.Purchase) { p.Quantity = 0 }, "quantity"},
		{"수량 음수", func(p *models.Purchase) { p.Quantity = -1 }, "quantity"},
		{"매입금액 음수", func(p *models.Purchase) { p.Amount = -1 }, "amount"},
		{"결제수단 빈값", func(p *models.Purchase) { p.PaymentMethod = "" }, "payment_method"},
		{"결제수단 11자", func(p *models.Purchase) { p.PaymentMethod = "12345678901" }, "payment_method"},
		{"비고 501자", func(p *models.Purchase) {
			s := make([]rune, 501)
			for i := range s {
				s[i] = 'a'
			}
			p.Memo = string(s)
		}, "memo"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base()
			tc.mod(p)
			err := svc.Create(ctx, p)
			var valErrs ValidationErrors
			if !errors.As(err, &valErrs) {
				t.Fatalf("ValidationErrors 기대: %v", err)
			}
			if _, ok := valErrs[tc.field]; !ok {
				t.Errorf("필드 %q 에러 기대: %v", tc.field, valErrs)
			}
		})
	}
}

// TestPurchaseService_Update_NotFound — 없는 ID 수정.
func TestPurchaseService_Update_NotFound(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := purchaseTestSetup(t)
	prod := newTestProductForPurchase(t, ctx, productRepo, "망고", 0)

	p := samplePurchaseFor(prod.ID, 10)
	p.ID = 9999
	if err := svc.Update(ctx, p); !errors.Is(err, ErrPurchaseNotFound) {
		t.Errorf("ErrPurchaseNotFound 기대: %v", err)
	}
}

// TestPurchaseService_Delete_NotFound — 없는 ID 삭제.
func TestPurchaseService_Delete_NotFound(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := purchaseTestSetup(t)
	if err := svc.Delete(ctx, 9999); !errors.Is(err, ErrPurchaseNotFound) {
		t.Errorf("ErrPurchaseNotFound 기대: %v", err)
	}
}
