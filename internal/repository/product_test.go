package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/varcharC2k/vape-crm/internal/db"
	"github.com/varcharC2k/vape-crm/internal/models"
)

// newTestDB — 테스트마다 새 인메모리 SQLite 를 만들고 스키마를 올린다.
//
// SetMaxOpenConns(1) 이 중요하다.
// :memory: 는 "연결마다 별도 메모리 DB" 를 만드므로, 커넥션 풀이 여러 개를 열면
// CREATE TABLE 한 DB 와 INSERT 한 DB 가 서로 다른 메모리를 바라보게 된다.
// 단일 커넥션으로 고정해 이 문제를 피한다.
func newTestDB(t *testing.T) *sql.DB {
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
	return conn
}

func newSampleProduct(name string) *models.Product {
	return &models.Product{
		Category:      models.CategoryLiquid,
		Name:          name,
		PurchasePrice: 3190,
		SalePrice:     8000,
		StockQty:      10,
	}
}

func TestProduct_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := NewProductRepository(newTestDB(t))

	p := newSampleProduct("망고 30ml")
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create 실패: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("Create 후 ID 가 0 — LastInsertId 반영 안 됨")
	}

	got, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID 실패: %v", err)
	}
	if got.Name != p.Name {
		t.Errorf("Name: got=%q want=%q", got.Name, p.Name)
	}
	if got.Category != p.Category {
		t.Errorf("Category: got=%d want=%d", got.Category, p.Category)
	}
	if got.PurchasePrice != p.PurchasePrice {
		t.Errorf("PurchasePrice: got=%d want=%d", got.PurchasePrice, p.PurchasePrice)
	}
	if got.SalePrice != p.SalePrice {
		t.Errorf("SalePrice: got=%d want=%d", got.SalePrice, p.SalePrice)
	}
	if got.StockQty != p.StockQty {
		t.Errorf("StockQty: got=%d want=%d", got.StockQty, p.StockQty)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt 이 zero value — DB default 적용 안 됨")
	}
}

func TestProduct_CreateDuplicateName(t *testing.T) {
	ctx := context.Background()
	repo := NewProductRepository(newTestDB(t))

	if err := repo.Create(ctx, newSampleProduct("같은이름")); err != nil {
		t.Fatalf("첫 번째 Create 실패: %v", err)
	}

	dup := newSampleProduct("같은이름")
	dup.Category = models.CategoryCoil // 분류가 달라도 이름이 같으면 거부되어야 함
	if err := repo.Create(ctx, dup); err == nil {
		t.Fatal("중복 이름인데 에러가 반환되지 않음 — UNIQUE 제약 확인 필요")
	}
}

func TestProduct_ListSortedByName(t *testing.T) {
	ctx := context.Background()
	repo := NewProductRepository(newTestDB(t))

	// 의도적으로 정렬된 순서가 아닌 상태로 등록
	for _, n := range []string{"체리", "레몬", "망고"} {
		if err := repo.Create(ctx, newSampleProduct(n)); err != nil {
			t.Fatalf("Create(%s) 실패: %v", n, err)
		}
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("개수: got=%d want=3", len(list))
	}

	// 한글 문자열 UTF-8 바이트 비교 기준: "레몬" < "망고" < "체리"
	want := []string{"레몬", "망고", "체리"}
	for i, n := range want {
		if list[i].Name != n {
			t.Errorf("list[%d].Name: got=%q want=%q", i, list[i].Name, n)
		}
	}
}

func TestProduct_ListEmpty(t *testing.T) {
	ctx := context.Background()
	repo := NewProductRepository(newTestDB(t))

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("빈 테이블 List 개수: got=%d want=0", len(list))
	}
}

func TestProduct_Update(t *testing.T) {
	ctx := context.Background()
	repo := NewProductRepository(newTestDB(t))

	p := &models.Product{
		Category: models.CategoryDevice, Name: "기기A",
		PurchasePrice: 10000, SalePrice: 30000, StockQty: 5,
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	p.StockQty = 20
	p.SalePrice = 35000
	p.Category = models.CategoryCoil // 분류도 변경 가능한지 확인
	if err := repo.Update(ctx, p); err != nil {
		t.Fatalf("Update 실패: %v", err)
	}

	got, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.StockQty != 20 {
		t.Errorf("StockQty 미반영: got=%d want=20", got.StockQty)
	}
	if got.SalePrice != 35000 {
		t.Errorf("SalePrice 미반영: got=%d want=35000", got.SalePrice)
	}
	if got.Category != models.CategoryCoil {
		t.Errorf("Category 미반영: got=%d want=%d", got.Category, models.CategoryCoil)
	}
}

func TestProduct_UpdateNotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewProductRepository(newTestDB(t))

	p := &models.Product{
		ID: 9999, Category: models.CategoryLiquid, Name: "없음",
		PurchasePrice: 1, SalePrice: 2, StockQty: 0,
	}
	if err := repo.Update(ctx, p); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("없는 ID 업데이트 시 sql.ErrNoRows 기대: got=%v", err)
	}
}

func TestProduct_Delete(t *testing.T) {
	ctx := context.Background()
	repo := NewProductRepository(newTestDB(t))

	p := newSampleProduct("삭제대상")
	if err := repo.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete 실패: %v", err)
	}

	if _, err := repo.GetByID(ctx, p.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("삭제 후 GetByID: sql.ErrNoRows 기대, got=%v", err)
	}
}

func TestProduct_DeleteNotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewProductRepository(newTestDB(t))

	if err := repo.Delete(ctx, 9999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("없는 ID 삭제 시 sql.ErrNoRows 기대: got=%v", err)
	}
}

func TestProduct_GetByIDNotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewProductRepository(newTestDB(t))

	if _, err := repo.GetByID(ctx, 9999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("없는 ID 조회 시 sql.ErrNoRows 기대: got=%v", err)
	}
}
