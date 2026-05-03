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

// saleTestSetup — 인메모리 DB + 두 repo + service 한 세트.
func saleTestSetup(t *testing.T) (*SaleService, *repository.ProductRepository, *sql.DB) {
	t.Helper()

	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("DB Open 실패: %v", err)
	}
	conn.SetMaxOpenConns(1)
	if err := db.Migrate(conn); err != nil {
		conn.Close()
		t.Fatalf("Migrate 실패: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	productRepo := repository.NewProductRepository(conn)
	saleRepo := repository.NewSaleRepository(conn)
	svc := NewSaleService(conn, saleRepo, productRepo)
	return svc, productRepo, conn
}

func newTestProductForSale(t *testing.T, ctx context.Context, repo *repository.ProductRepository, name string, initialStock int) *models.Product {
	t.Helper()
	p := &models.Product{
		Category:  models.CategoryLiquid,
		Name:      name,
		SalePrice: 25000,
		StockQty:  initialStock,
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create 품목(%s) 실패: %v", name, err)
	}
	return p
}

func sampleSaleFor(productID int64, qty int) *models.Sale {
	return &models.Sale{
		TransactionDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		ProductID:       productID,
		CustomerName:    "테스트",
		CustomerMobile:  "010-1111-2222",
		Quantity:        qty,
		PaymentMethod:   "현금",
	}
}

// TestSaleService_Create_Stock — 매출 등록 시 재고 차감.
func TestSaleService_Create_Stock(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := saleTestSetup(t)
	prod := newTestProductForSale(t, ctx, productRepo, "망고", 10)

	if err := svc.Create(ctx, sampleSaleFor(prod.ID, 3)); err != nil {
		t.Fatalf("Create 실패: %v", err)
	}

	got, _ := productRepo.GetByID(ctx, prod.ID)
	if got.StockQty != 7 {
		t.Errorf("재고: got=%d want=7 (10-3)", got.StockQty)
	}
}

// TestSaleService_Create_NegativeStockAllowed — 음수 재고 허용 (사용자 결정).
// 재고 5 인데 7개 매출 등록하면 재고가 -2 가 되어야 함.
func TestSaleService_Create_NegativeStockAllowed(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := saleTestSetup(t)
	prod := newTestProductForSale(t, ctx, productRepo, "망고", 5)

	if err := svc.Create(ctx, sampleSaleFor(prod.ID, 7)); err != nil {
		t.Fatalf("Create 실패 — 음수 재고는 허용돼야 함: %v", err)
	}

	got, _ := productRepo.GetByID(ctx, prod.ID)
	if got.StockQty != -2 {
		t.Errorf("재고: got=%d want=-2 (5-7)", got.StockQty)
	}
}

// TestSaleService_Update_SameProduct_StockDelta — 같은 품목 수량 변경 시 차이만큼 정정.
// 매입과 부호가 반대: stock += (oldQty - newQty)
func TestSaleService_Update_SameProduct_StockDelta(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := saleTestSetup(t)
	prod := newTestProductForSale(t, ctx, productRepo, "망고", 10)

	s := sampleSaleFor(prod.ID, 3)
	_ = svc.Create(ctx, s) // 재고: 10 - 3 = 7

	s.Quantity = 5 // 더 많이 팔기
	if err := svc.Update(ctx, s); err != nil {
		t.Fatalf("Update 실패: %v", err)
	}

	got, _ := productRepo.GetByID(ctx, prod.ID)
	// 7 + (3 - 5) = 7 - 2 = 5
	if got.StockQty != 5 {
		t.Errorf("재고: got=%d want=5 (10-5)", got.StockQty)
	}
}

// TestSaleService_Update_NoQuantityChange — 수량 동일, 다른 필드만 변경.
func TestSaleService_Update_NoQuantityChange(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := saleTestSetup(t)
	prod := newTestProductForSale(t, ctx, productRepo, "망고", 10)

	s := sampleSaleFor(prod.ID, 3)
	_ = svc.Create(ctx, s) // 재고: 7

	s.Memo = "수정"
	s.CustomerName = "다른 고객"
	if err := svc.Update(ctx, s); err != nil {
		t.Fatalf("Update 실패: %v", err)
	}

	got, _ := productRepo.GetByID(ctx, prod.ID)
	if got.StockQty != 7 {
		t.Errorf("재고: got=%d want=7 (수량 동일)", got.StockQty)
	}
}

// TestSaleService_Update_ProductChange — 매출의 품목이 변경되면 양쪽 재고 이동.
// 이전 품목 +수량 (복구), 새 품목 -수량 (차감).
func TestSaleService_Update_ProductChange(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := saleTestSetup(t)
	prodA := newTestProductForSale(t, ctx, productRepo, "망고", 10)
	prodB := newTestProductForSale(t, ctx, productRepo, "딸기", 10)

	s := sampleSaleFor(prodA.ID, 4)
	_ = svc.Create(ctx, s) // 망고: 6, 딸기: 10

	s.ProductID = prodB.ID
	if err := svc.Update(ctx, s); err != nil {
		t.Fatalf("Update 실패: %v", err)
	}

	a, _ := productRepo.GetByID(ctx, prodA.ID)
	b, _ := productRepo.GetByID(ctx, prodB.ID)
	if a.StockQty != 10 {
		t.Errorf("이전 품목 복구: got=%d want=10", a.StockQty)
	}
	if b.StockQty != 6 {
		t.Errorf("새 품목 차감: got=%d want=6 (10-4)", b.StockQty)
	}
}

// TestSaleService_Update_ProductAndQuantityChange — 품목·수량 동시 변경.
func TestSaleService_Update_ProductAndQuantityChange(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := saleTestSetup(t)
	prodA := newTestProductForSale(t, ctx, productRepo, "망고", 10)
	prodB := newTestProductForSale(t, ctx, productRepo, "딸기", 5)

	s := sampleSaleFor(prodA.ID, 4)
	_ = svc.Create(ctx, s) // 망고: 6, 딸기: 5

	s.ProductID = prodB.ID
	s.Quantity = 2
	if err := svc.Update(ctx, s); err != nil {
		t.Fatalf("Update 실패: %v", err)
	}

	a, _ := productRepo.GetByID(ctx, prodA.ID)
	b, _ := productRepo.GetByID(ctx, prodB.ID)
	if a.StockQty != 10 {
		t.Errorf("망고: got=%d want=10 (6+4)", a.StockQty)
	}
	if b.StockQty != 3 {
		t.Errorf("딸기: got=%d want=3 (5-2)", b.StockQty)
	}
}

// TestSaleService_Delete_Stock — 매출 삭제 시 재고 복구.
func TestSaleService_Delete_Stock(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := saleTestSetup(t)
	prod := newTestProductForSale(t, ctx, productRepo, "망고", 10)

	s := sampleSaleFor(prod.ID, 3)
	_ = svc.Create(ctx, s) // 재고: 7

	if err := svc.Delete(ctx, s.ID); err != nil {
		t.Fatalf("Delete 실패: %v", err)
	}

	got, _ := productRepo.GetByID(ctx, prod.ID)
	if got.StockQty != 10 {
		t.Errorf("재고: got=%d want=10 (7+3 복구)", got.StockQty)
	}

	if _, err := svc.Get(ctx, s.ID); !errors.Is(err, ErrSaleNotFound) {
		t.Errorf("삭제 후 Get: ErrSaleNotFound 기대 got=%v", err)
	}
}

// TestSaleService_Validation — 필드별 검증.
// 음수 재고 검증은 안 함 (허용).
func TestSaleService_Validation(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := saleTestSetup(t)
	prod := newTestProductForSale(t, ctx, productRepo, "망고", 100)

	base := func() *models.Sale {
		return sampleSaleFor(prod.ID, 1)
	}

	cases := []struct {
		name  string
		mod   func(*models.Sale)
		field string
	}{
		{"날짜 zero", func(s *models.Sale) { s.TransactionDate = time.Time{} }, "transaction_date"},
		{"품목 ID 0", func(s *models.Sale) { s.ProductID = 0 }, "product_id"},
		{"수량 0", func(s *models.Sale) { s.Quantity = 0 }, "quantity"},
		{"수량 음수", func(s *models.Sale) { s.Quantity = -1 }, "quantity"},
		{"결제수단 빈값", func(s *models.Sale) { s.PaymentMethod = "" }, "payment_method"},
		{"결제수단 11자", func(s *models.Sale) { s.PaymentMethod = "12345678901" }, "payment_method"},
		{"고객명 51자", func(s *models.Sale) {
			r := make([]rune, 51)
			for i := range r {
				r[i] = '가'
			}
			s.CustomerName = string(r)
		}, "customer_name"},
		{"휴대폰 21자", func(s *models.Sale) { s.CustomerMobile = "012345678901234567890" }, "customer_mobile"},
		{"비고 501자", func(s *models.Sale) {
			r := make([]rune, 501)
			for i := range r {
				r[i] = 'a'
			}
			s.Memo = string(r)
		}, "memo"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base()
			tc.mod(s)
			err := svc.Create(ctx, s)
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

// TestSaleService_Update_NotFound
func TestSaleService_Update_NotFound(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := saleTestSetup(t)
	prod := newTestProductForSale(t, ctx, productRepo, "망고", 10)

	s := sampleSaleFor(prod.ID, 1)
	s.ID = 9999
	if err := svc.Update(ctx, s); !errors.Is(err, ErrSaleNotFound) {
		t.Errorf("ErrSaleNotFound 기대: %v", err)
	}
}

// TestSaleService_Delete_NotFound
func TestSaleService_Delete_NotFound(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := saleTestSetup(t)
	if err := svc.Delete(ctx, 9999); !errors.Is(err, ErrSaleNotFound) {
		t.Errorf("ErrSaleNotFound 기대: %v", err)
	}
}

// TestSaleService_BiznoMember_Create — 비회원 매출 등록 (고객 정보 빈값).
func TestSaleService_BizNoMember_Create(t *testing.T) {
	ctx := context.Background()
	svc, productRepo, _ := saleTestSetup(t)
	prod := newTestProductForSale(t, ctx, productRepo, "망고", 10)

	s := &models.Sale{
		TransactionDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		ProductID:       prod.ID,
		Quantity:        1,
		PaymentMethod:   "현금",
		// CustomerName, CustomerMobile 모두 빈값
	}
	if err := svc.Create(ctx, s); err != nil {
		t.Fatalf("비회원 매출 Create 실패 — 고객 정보는 선택이어야 함: %v", err)
	}
	if s.ID == 0 {
		t.Error("ID 부여 실패")
	}
}
