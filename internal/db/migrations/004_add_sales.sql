-- 004_add_sales.sql
-- 매출 거래 (sales)
--
-- 품목은 FK 로 연결 (재고 차감에 필요). 매출 있는 품목은 service 단에서 삭제 거부 (A안 패턴).
-- 고객은 FK 없음. 등록 시점의 customer_name/customer_mobile 을 스냅샷으로 저장하여
-- 고객 삭제 후에도 매출 이력은 그대로 보존된다 (사용자 결정).
-- 비회원 거래는 customer_name/customer_mobile 이 모두 NULL.
--
-- 음수 재고 허용 (사용자 결정). 매출 등록 시 재고 부족 검증을 service 에서 하지 않는다.

CREATE TABLE IF NOT EXISTS sales (
    id                INTEGER  PRIMARY KEY AUTOINCREMENT,
    transaction_date  DATE     NOT NULL,                            -- 매출 일자
    product_id        INTEGER  NOT NULL REFERENCES products(id),    -- 품목 FK

    -- 고객 정보 스냅샷 (FK 없음, 비회원은 NULL)
    customer_name     TEXT,
    customer_mobile   TEXT,

    quantity          INTEGER  NOT NULL,
    payment_method    TEXT     NOT NULL,                            -- 결제수단 (앱에서 10자 검증)
    memo              TEXT,                                         -- 비고 (선택, 앱에서 500자 검증)
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sales_date    ON sales(transaction_date);
CREATE INDEX IF NOT EXISTS idx_sales_product ON sales(product_id);
CREATE INDEX IF NOT EXISTS idx_sales_mobile  ON sales(customer_mobile);  -- 휴대폰으로 고객 매출 이력 조회
