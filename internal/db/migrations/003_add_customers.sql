-- 003_add_customers.sql
-- 고객 (customers)
--
-- 매출 (sales) 의 customer_id 가 참조한다 (이후 마이그레이션).
-- 고객 삭제는 매출 거래가 있어도 허용 — 매출 테이블 추가 시 customer_id 처리 정책 결정 필요
-- (NULL 처리 / 스냅샷 / cascade 중 택일).
--
-- 휴대폰 UNIQUE — 동일 번호 중복 등록 방지.
-- SQLite 의 UNIQUE 는 NULL 값을 여러 행에 허용하므로 휴대폰 미입력 고객은 여러 명 가능.

CREATE TABLE IF NOT EXISTS customers (
    id                INTEGER  PRIMARY KEY AUTOINCREMENT,
    name              TEXT     NOT NULL,                          -- 고객명 (앱에서 50자 검증)
    mobile            TEXT     UNIQUE,                            -- 휴대폰 (선택)
    birth_date        DATE,                                       -- 생년월일 (선택)
    email             TEXT,                                       -- 이메일 (선택)
    address           TEXT,                                       -- 주소 (선택)
    memo              TEXT,                                       -- 비고 (선택)
    registration_date DATE     NOT NULL DEFAULT CURRENT_DATE      -- 등록일 (오늘 자동, 폼에서 수정 가능)
);

CREATE INDEX IF NOT EXISTS idx_customers_name   ON customers(name);
CREATE INDEX IF NOT EXISTS idx_customers_mobile ON customers(mobile);
