-- 001_init.sql
-- 초기 스키마: 품목 마스터(products)
-- 거래 테이블(purchases, sales)과 고객 테이블(customers)은 이후 마이그레이션에서 추가한다.

CREATE TABLE IF NOT EXISTS products (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    category        INTEGER NOT NULL,              -- 0:액상, 1:기계, 2:코일, 3:팟 (internal/models/product.go 의 Category iota 와 1:1 매칭)
    name            TEXT    NOT NULL UNIQUE,       -- 품목명. 앱 단에서 50자 이내로 검증. 중복 등록 방지 위해 UNIQUE.
    sale_price      INTEGER NOT NULL,              -- 매출단가 (원 단위 정수)
    stock_qty       INTEGER NOT NULL DEFAULT 0,    -- 재고 수량
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_products_category ON products(category);
CREATE INDEX IF NOT EXISTS idx_products_name     ON products(name);
