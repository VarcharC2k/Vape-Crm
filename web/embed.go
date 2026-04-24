// Package web — 템플릿·정적파일을 바이너리에 embed 하여 단일 실행파일 배포를 유지한다.
package web

import "embed"

// Templates — web/templates 아래의 모든 HTML 템플릿.
// ParseFS(Templates, "templates/*.html", "templates/products/*.html") 처럼 접근.
//
// 주의: "all:" 프리픽스가 붙어 있다.
// go:embed 는 기본적으로 '_' 또는 '.' 로 시작하는 파일을 제외하는데,
// 이 프로젝트는 파셜 템플릿을 `_tbody.html`, `_form.html` 처럼 언더스코어로
// 구분하고 있어서 'all:' 이 없으면 파셜이 바이너리에 포함되지 않는다.
//
//go:embed all:templates
var Templates embed.FS
