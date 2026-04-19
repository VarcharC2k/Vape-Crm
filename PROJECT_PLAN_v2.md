# 전자담배샵 CRM 재개발 프로젝트 기획서 v2

> 기존 Excel(.xlsb) 기반 CRM을 Go + SQLite 기반의 단일 실행 웹 애플리케이션으로 재구축

**변경 이력**
- v1 → v2: 거래처 관리 제외, PDF 출력 제외, 데이터 마이그레이션 제외, DB는 SQLite로 확정, **부분일치(Like) 검색을 핵심 기능으로 격상**

---

## 1. 프로젝트 개요

### 1.1 배경
현재 전자담배 소매점에서 사용 중인 엑셀 기반 CRM 시스템은 다음과 같은 한계가 있다.

- VLOOKUP이 완전일치/앞쪽일치만 지원하여 품목 검색이 불편 (가장 큰 고충)
- 시트 보호/잠금으로 기능 확장이 불가능
- 이력이 쌓일수록 성능 저하
- 수식 손상 시 복구가 어려움

### 1.2 목표
- 기존 엑셀의 모든 기능을 **동등 이상**으로 구현
- **부분일치·실시간 검색**으로 품목 조회 속도 극적 개선
- **0원 운영**: 유료 SaaS·클라우드·결제 서비스 일체 미사용
- **단일 실행 파일** 배포 — 별도 런타임/DB 설치 불필요
- 로컬 데이터 보관, 인터넷 없이도 동작

### 1.3 제약사항

| 항목 | 제약 |
|---|---|
| 비용 | 100% 무료 (오픈소스만) |
| DB | 외부 DBMS 금지 → **SQLite 내장** |
| 인프라 | 클라우드/SaaS 금지 → 로컬 실행 |
| 배포 | 단일 실행파일 |
| 운영 환경 | 매장 PC 1대 (단일 사용자) |

---

## 2. 현재 엑셀 파일 분석

### 2.1 시트 구성

업로드된 `고객관리_매출관리_재고관리_기본_.xlsb`는 9개 시트로 구성되어 있으며, 이번 프로젝트에서 재구현 대상은 아래 표와 같다.

| 원본 시트 | 재구현 여부 | 비고 |
|---|:---:|---|
| 거래처관리 | ❌ 제외 | 실무에서 더 이상 사용하지 않음 |
| 매입등록 | ✅ | 거래처 필드는 자유 입력 텍스트로 대체 |
| 매입검색 | ✅ | |
| 고객관리 | ✅ | |
| 매출등록 | ✅ | **품목 부분검색 핵심 기능 적용** |
| 매출검색 | ✅ | |
| 결산 | ✅ | |
| 품목설정 | ✅ | 제품 마스터, 가장 중요 |
| 설정 | ✅ | 결제수단 등 공통 옵션 |

### 2.2 핵심 데이터 파악

**품목설정 시트** (가장 중요)
- 필드: `분류, 품목, 재고수량, 매입단가, 매출단가, 재고금액, 최초재고`
- 분류 4종: **액상 / 기계 / 코일 / 팟**
- 현재 품목 수: 약 **197개**
- `재고금액 = 재고수량 × 매입단가`

**고객관리**
- 필드: `ID, 생년월일, 고객명, 전화번호, 휴대번호, 이메일, 주소, 등록일, 비고`

**매입등록**
- 필드: `날짜, 거래처명(자유입력), 품목, 수량, 매입금액, 결제수단, 비고`

**매출등록**
- 필드: `날짜, 고객명, 휴대번호, 품목, 수량, 매입가(자동), 매출금액, 결제수단, 비고`
- **매출 시 매입가는 품목에서 자동 조회 (스냅샷으로 저장)**
- **순수익 = 매출금액 − (매입가 × 수량)**

### 2.3 핵심 비즈니스 로직

1. **재고 차감**: 매출 등록 시 품목 재고수량이 자동 감소
2. **재고 증가**: 매입 등록 시 품목 재고수량이 자동 증가
3. **매입단가 자동 참조 및 스냅샷 저장**: 매출 등록 시점의 매입가를 기록 (향후 마스터 변경에도 과거 순수익이 정확)
4. **결산 집계**: 매출/매입 내역을 연·월별로 자동 합산
5. **순수익 계산**: 선택된 기간의 매출액 − 매입 원가

---

## 3. 핵심 차별 포인트 — 부분일치(Like) 검색

> 기존 엑셀 대비 가장 큰 개선점. 모든 검색/선택 UI에 일관되게 적용.

### 3.1 기존 엑셀의 문제
- VLOOKUP은 **완전일치 또는 앞쪽일치**만 지원
- "뉴 스프라이트2"를 찾으려면 "뉴 스"까지 입력해야 함
- "스"만 입력해서는 결과가 나오지 않음

### 3.2 해결 방식 — 3단계 조합

**① SQL LIKE 부분일치 검색 (MVP)**

```sql
SELECT id, category, name, sale_price, stock_qty
FROM products
WHERE name LIKE '%' || ? || '%'
ORDER BY
    CASE
        WHEN name LIKE ? || '%' THEN 1   -- 앞쪽 일치 우선
        ELSE 2                            -- 중간 포함 후순위
    END,
    name
LIMIT 20;
```

"스" 입력 시 결과 예시:
- 1순위: **스**프라이트 등 (앞쪽 일치)
- 2순위: 뉴 **스**프라이트2, 쥬**시**아이스 등 (포함 일치)

**② 실시간 검색 UI — HTMX 활용 (MVP)**

사용자가 타이핑할 때마다 200ms 지연 후 서버에 요청해 결과를 자동 갱신:

```html
<input type="text"
       name="q"
       hx-get="/api/products/search"
       hx-trigger="input changed delay:200ms"
       hx-target="#results"
       placeholder="품목명 일부 입력...">
<div id="results"></div>
```

**③ 한글 초성 검색 (2차 개선, 선택)**

- 품목 등록 시 `name_chosung` 컬럼에 초성만 추출해 저장 ("스프라이트" → "ㅅㅍㄹㅇㅌ")
- 검색어가 초성만 포함하면 자동으로 초성 검색으로 전환
- `ㅅㅍ` 입력 시 "스프라이트" 검색 가능

### 3.3 적용 대상 화면

| 화면 | 검색 필드 |
|---|---|
| 매출 등록 | 품목, 고객명/휴대번호 |
| 매입 등록 | 품목 |
| 품목 관리 | 품목명, 분류별 필터 |
| 고객 관리 | 이름, 휴대번호 |
| 매출/매입 검색 | 고객·품목·기간 필터 |

→ **공통 검색 컴포넌트**로 구현하여 모든 화면에서 재사용.

---

## 4. 기술 스택

### 4.1 최종 스택

| 계층 | 기술 | 선택 이유 |
|---|---|---|
| 언어 | Go 1.22+ | 단일 바이너리 빌드, 크로스 컴파일 |
| 웹 라우터 | `chi` (go-chi/chi) | 표준 `net/http` 호환, 경량 |
| DB | **SQLite** | 설치 불필요, 단일 파일 저장, LIKE 완전 지원 |
| DB 드라이버 | `modernc.org/sqlite` | Pure Go, CGO 불필요 → 크로스 컴파일 쉬움 |
| 템플릿 | `html/template` (Go 표준) | 서버사이드 렌더링 |
| 프론트 인터랙션 | **HTMX** + Alpine.js | SPA 빌드 도구 없이 동적 UI |
| CSS | Pico.css (CDN) | 클래스리스, 빌드 불필요, 깔끔한 기본값 |
| 차트 | Chart.js (CDN) | 결산 페이지 시각화 |
| 빌드 | `go build` | 단일 바이너리 |

### 4.2 SQLite 선택 이유 (PostgreSQL과 비교 결과)

- PC 1대 단독 사용이므로 동시 쓰기 이슈 없음
- 사용자가 `.exe` 파일만 실행하면 끝 — PostgreSQL 설치·설정 불필요
- 백업은 DB 파일 1개만 복사하면 완료
- **LIKE 검색 성능은 197~수천 품목 규모에서 두 DB 동일**
- 향후 PC가 늘어나면 그때 PostgreSQL로 마이그레이션 가능 (SQL 표준 호환)

---

## 5. 데이터 모델 (SQLite 스키마)

```sql
-- 고객
CREATE TABLE customers (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    name_chosung    TEXT,              -- 초성 검색용 (2차)
    birth_date      DATE,
    phone           TEXT,
    mobile          TEXT,
    email           TEXT,
    address         TEXT,
    memo            TEXT,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_customers_name      ON customers(name);
CREATE INDEX idx_customers_mobile    ON customers(mobile);
CREATE INDEX idx_customers_chosung   ON customers(name_chosung);

-- 품목 마스터
CREATE TABLE products (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    category        TEXT NOT NULL,    -- 액상/기계/코일/팟
    name            TEXT NOT NULL UNIQUE,
    name_chosung    TEXT,             -- 초성 검색용 (2차)
    purchase_price  INTEGER NOT NULL, -- 매입단가 (원 단위 정수)
    sale_price      INTEGER NOT NULL, -- 매출단가
    stock_qty       INTEGER NOT NULL DEFAULT 0,
    initial_stock   INTEGER NOT NULL DEFAULT 0,
    is_active       BOOLEAN NOT NULL DEFAULT 1,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_products_category  ON products(category);
CREATE INDEX idx_products_name      ON products(name);
CREATE INDEX idx_products_chosung   ON products(name_chosung);

-- 매입 거래
CREATE TABLE purchases (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    transaction_date    DATE NOT NULL,
    supplier_name       TEXT,             -- 거래처 관리 제외 → 자유 입력 텍스트
    product_id          INTEGER NOT NULL REFERENCES products(id),
    quantity            INTEGER NOT NULL,
    amount              INTEGER NOT NULL, -- 매입 총액
    payment_method      TEXT,
    memo                TEXT,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_purchases_date     ON purchases(transaction_date);
CREATE INDEX idx_purchases_product  ON purchases(product_id);

-- 매출 거래
CREATE TABLE sales (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    transaction_date    DATE NOT NULL,
    customer_id         INTEGER REFERENCES customers(id),  -- 비회원 거래는 NULL
    product_id          INTEGER NOT NULL REFERENCES products(id),
    quantity            INTEGER NOT NULL,
    purchase_price      INTEGER NOT NULL, -- 매출 시점의 매입단가 스냅샷
    sale_amount         INTEGER NOT NULL, -- 매출액
    payment_method      TEXT,
    memo                TEXT,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_sales_date         ON sales(transaction_date);
CREATE INDEX idx_sales_customer     ON sales(customer_id);
CREATE INDEX idx_sales_product      ON sales(product_id);

-- 설정 (결제수단, 카테고리 등 key-value)
CREATE TABLE settings (
    key     TEXT PRIMARY KEY,
    value   TEXT NOT NULL
);
```

### 주요 설계 포인트

- **금액은 모두 INTEGER (원 단위)** → 부동소수점 오차 없음
- **매출 시점 매입단가 스냅샷** → 마스터가 나중에 바뀌어도 과거 순수익 정확
- **거래처 관리 제외에 따라** `purchases.supplier_name`은 단순 텍스트 필드
- **비회원 매출 허용** → `customer_id`는 NULL 가능
- **초성 검색용 컬럼**은 MVP에서는 비워두고 2차에서 채움

---

## 6. 기능 명세

### 6.1 MVP — 1차 필수 기능

- [ ] **고객 관리** (CRUD)
- [ ] **품목 관리** (CRUD, 분류별 필터)
- [ ] **매입 등록** → 재고 자동 증가
- [ ] **매출 등록** → 재고 자동 차감, 매입단가 자동 조회·스냅샷
- [ ] **매입/매출 이력 조회** (날짜·품목·고객 필터)
- [ ] **결산 페이지** (월별·연도별 매출/매입/순수익)
- [ ] **부분일치 실시간 검색** (품목/고객 모든 화면 공통)
- [ ] **설정** (결제수단, 카테고리 관리)

### 6.2 2차 개선 기능

- [ ] 한글 초성 검색 지원
- [ ] 대시보드 (매출 추이 차트)
- [ ] 재고 알림 (임계치 미만 강조)
- [ ] 데이터 백업/복원 (SQLite 파일 단위)
- [ ] 엑셀(.xlsx) 익스포트 (결산 내보내기용)

### 6.3 명시적으로 제외

- ❌ 거래처(공급사) 관리 시스템
- ❌ 영수증/인보이스 PDF 출력
- ❌ 데이터 마이그레이션 (기존 xlsb → 신규 DB)
- ❌ 사용자 로그인/권한 (1인 매장 전제)
- ❌ 모바일 전용 앱 (웹 UI를 모바일 브라우저로 보는 수준까지만)
- ❌ 바코드 스캐너 (2차에서도 당분간 보류)

---

## 7. 프로젝트 구조

```
vape-crm/
├── cmd/
│   └── server/
│       └── main.go                  # 진입점
├── internal/
│   ├── config/                      # 환경 설정
│   ├── db/
│   │   ├── db.go                    # SQLite 연결, 초기화
│   │   └── migrations/              # 001_init.sql 등
│   ├── models/                      # 구조체 정의
│   ├── repository/                  # DB 접근 계층 (CRUD + 검색)
│   │   ├── customer_repo.go
│   │   ├── product_repo.go
│   │   ├── purchase_repo.go
│   │   └── sale_repo.go
│   ├── service/                     # 비즈니스 로직
│   │   ├── sale_service.go          # 매출 + 재고 차감 트랜잭션
│   │   └── purchase_service.go      # 매입 + 재고 증가 트랜잭션
│   ├── handler/                     # HTTP 핸들러
│   └── search/                      # 부분일치·초성 검색 공통 모듈
├── web/
│   ├── templates/                   # HTML 템플릿
│   │   ├── layout.html
│   │   ├── products/
│   │   ├── customers/
│   │   ├── sales/
│   │   └── components/              # 검색 컴포넌트 등 공용 조각
│   └── static/
│       ├── css/
│       └── js/
├── data/                            # SQLite 파일 (.gitignore)
├── go.mod
├── go.sum
├── CLAUDE.md                        # Claude Code용 프로젝트 가이드
└── README.md
```

---

## 8. 개발 로드맵

### Phase 0 — 환경 준비 (1~2일)
- Go 설치, 프로젝트 초기화
- GitHub 저장소 생성
- `CLAUDE.md` 작성 — 프로젝트 규칙·컨벤션 정리
- Hello World 웹서버 구동 확인
- SQLite 연결 확인

### Phase 1 — 데이터 계층 (3~5일)
- 스키마 마이그레이션 작성
- 각 테이블 repository 구현 (CRUD)
- repository 단위 테스트 (메모리 SQLite 사용)

### Phase 2 — 공통 검색 모듈 (2~3일)
- 부분일치 검색 함수
- HTMX 기반 실시간 검색 UI 컴포넌트
- 품목·고객에서 재사용 가능한 형태로 구축

### Phase 3 — 마스터 관리 UI (1주)
- 고객 관리 페이지 (목록, 등록, 수정, 삭제, 검색)
- 품목 관리 페이지 (분류 필터 + 부분 검색)

### Phase 4 — 거래 기능 (1~1.5주)
- 매입 등록 (트랜잭션 내 재고 증가)
- 매출 등록 (트랜잭션 내 재고 차감 + 매입단가 스냅샷)
- 매입/매출 이력 검색 페이지

### Phase 5 — 결산·리포트 (3~5일)
- 월별·연도별 집계 쿼리
- 결산 페이지 + Chart.js 기본 차트

### Phase 6 — 완성도·배포 (3~5일)
- 에러 처리·UI 다듬기
- 백업/복원 기능
- Windows용 단일 바이너리 빌드 테스트
- 매장 PC 설치 리허설

**예상 총 기간**: 주니어 기준 **6~8주** (주 10~15시간 기준)

---

## 9. 개발 원칙

### 9.1 코딩 원칙
- 함수는 짧게 (원칙적으로 30줄 이내)
- 금액 계산은 반드시 정수형 (INTEGER)
- SQL은 항상 prepared statement (SQL 인젝션 방지)
- 에러는 감추지 않고 반드시 로깅
- 매출/매입은 **트랜잭션**으로 처리 — 재고 변동까지 원자적으로

### 9.2 테스트 전략
- repository: 메모리 SQLite로 단위 테스트
- service: 재고 증감 로직, 결산 집계는 **반드시** 테스트
- handler: 주요 시나리오만 통합 테스트

### 9.3 UX 원칙
- 모든 검색은 **타이핑하는 즉시 결과 표시**
- 매출 등록 페이지는 **최소 클릭**으로 완료 (핵심 화면)
- 숫자 입력 시 자동 천단위 콤마 표시

---

## 10. 0원 운영 체크리스트

| 항목 | 선택 | 비용 |
|---|---|---|
| 언어/런타임 | Go (BSD) | 무료 |
| DB | SQLite (Public Domain) | 무료 |
| 웹 프레임워크 | chi (MIT) | 무료 |
| UI 라이브러리 | HTMX, Pico.css (CDN) | 무료 |
| 차트 | Chart.js (CDN) | 무료 |
| 호스팅 | 로컬 실행 | 무료 |
| 소스 관리 | GitHub (public) | 무료 |
| IDE | VS Code | 무료 |
| AI 어시스턴트 | Claude Max (보유중) | 추가비용 없음 |

**총 추가 비용: 0원** ✅

---

## 11. 다음 단계

이 v2가 확정되면 아래 순서로 진행.

1. **이 기획서 최종 확인**
2. **GitHub 저장소 생성** 및 초기 구조 커밋
3. **`CLAUDE.md` 작성** — Claude Code에서 일관된 개발을 위한 프로젝트 컨텍스트
4. **Phase 0 시작** — 환경 세팅 + Hello World

---

## 부록 A — 참고 자료

- SQLite Pure Go 드라이버: https://gitlab.com/cznic/sqlite
- HTMX 공식 문서: https://htmx.org/
- chi 라우터: https://github.com/go-chi/chi
- Pico.css: https://picocss.com/
- Chart.js: https://www.chartjs.org/

## 부록 B — 현재 엑셀 파일 요약 통계

- 총 시트 수: 9개 → 실제 재구현 대상 7개
- 품목 수: 약 197개
- 품목 분류: 액상, 기계, 코일, 팟 (4종)
- 결산 지원 기간: 2024 ~ 2035년
