-- 002_add_purchases.sql
-- 매입 거래 (purchases)
--
-- products(id) 와 FK 로 연결. 등록 시 service 의 트랜잭션이 INSERT 와 함께
-- products.stock_qty 를 가산한다. 수정·삭제 시에도 service 가 재고를 정정한다.

CREATE TABLE IF NOT EXISTS purchases (
    id               INTEGER  PRIMARY KEY AUTOINCREMENT,
    transaction_date DATE     NOT NULL,                            -- 'YYYY-MM-DD' 형식의 TEXT
    product_id       INTEGER  NOT NULL REFERENCES products(id),    -- 품목 FK
    quantity         INTEGER  NOT NULL,                            -- 수량
    amount           INTEGER  NOT NULL,                            -- 매입 총액 (원 단위 정수)
    payment_method   TEXT     NOT NULL,                            -- 결제수단 (앱에서 10자 검증)
    memo             TEXT,                                         -- 비고 (선택, 앱에서 500자 검증)
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_purchases_date    ON purchases(transaction_date);
CREATE INDEX IF NOT EXISTS idx_purchases_product ON purchases(product_id);
