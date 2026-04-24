package handler

import (
	"html/template"
	"strconv"
	"strings"

	"github.com/varcharC2k/vape-crm/internal/models"
	"github.com/varcharC2k/vape-crm/web"
)

// tmplFuncs — 템플릿에서 호출할 수 있는 헬퍼들.
//
// currency: 1234567 -> "1,234,567원"
// categories: 드롭다운용 전체 분류 목록
// categoryInt: Category -> 0/1/2/3 (select 의 value 에 넣을 때)
var tmplFuncs = template.FuncMap{
	"currency":    currency,
	"categories":  models.AllCategories,
	"categoryInt": func(c models.Category) int { return int(c) },
}

// currency — 원 단위 정수를 "1,234,567원" 형식으로.
// 음수는 앞에 '-' 를 붙인다. 0 은 "0원".
func currency(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	raw := strconv.FormatInt(v, 10)
	// 뒤에서부터 3자리마다 콤마 삽입.
	n := len(raw)
	var b strings.Builder
	b.Grow(n + n/3 + 2)
	for i, r := range raw {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	b.WriteString("원")
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// LoadTemplates — 바이너리에 임베드된 템플릿을 하나의 세트로 파싱.
// 각 파일은 {{define "이름"}}…{{end}} 블록을 쓰고, 핸들러는 이 이름으로 Execute 한다.
func LoadTemplates() (*template.Template, error) {
	return template.New("").
		Funcs(tmplFuncs).
		ParseFS(web.Templates, "templates/*.html", "templates/products/*.html")
}
