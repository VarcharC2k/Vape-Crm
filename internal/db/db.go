package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

// migrationFS — migrations/*.sql 을 바이너리에 포함시켜
// 단일 실행파일 배포 시 외부 파일 의존 없이 초기화할 수 있게 한다.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

// Open — 지정 경로로 SQLite DB를 열고 연결을 검증한다.
// path 로 ":memory:" 를 주면 테스트용 인메모리 DB 가 열린다.
func Open(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// Migrate — migrations 디렉토리의 *.sql 파일을 파일명 오름차순으로 실행한다.
// 파일명 규칙: 001_init.sql, 002_add_xxx.sql ... — 숫자 프리픽스로 순서 보장.
//
// 아직 버전 관리 테이블(schema_migrations)은 두지 않는다.
// 각 SQL 이 CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS 를 쓰므로
// 중복 실행돼도 안전하다. 마이그레이션 수가 늘어나면 schema_migrations 도입 예정.
func Migrate(conn *sql.DB) error {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("마이그레이션 디렉토리 읽기 실패: %w", err)
	}

	var filenames []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			filenames = append(filenames, e.Name())
		}
	}
	sort.Strings(filenames)

	for _, name := range filenames {
		content, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("마이그레이션 %s 읽기 실패: %w", name, err)
		}
		if _, err := conn.Exec(string(content)); err != nil {
			return fmt.Errorf("마이그레이션 %s 실행 실패: %w", name, err)
		}
	}
	return nil
}
