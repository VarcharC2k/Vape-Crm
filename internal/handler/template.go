package handler

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"github.com/varcharC2k/vape-crm/internal/models"
	"github.com/varcharC2k/vape-crm/web"
)

// tmplFuncs — 템플릿에서 호출할 수 있는 헬퍼들.
var tmplFuncs = template.FuncMap{
	"currency":    currency,
	"categories":  models.AllCategories,
	"categoryInt": func(c models.Category) int { return int(c) },
}

// currency — 원 단위 정수를 "1,234,567원" 형식으로.
func currency(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	raw := strconv.FormatInt(v, 10)
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

// PageTemplates — 페이지별로 분리된 템플릿 세트.
//
// 같은 이름의 블록(예: 각 페이지가 정의하는 "page") 이 여러 파일에 들어 있으면
// 단일 템플릿 세트에서는 한 정의만 살아남는다 (나중에 파싱된 게 이긴다).
// 이 충돌을 피하려고 페이지마다 ParseFS 를 따로 호출해 독립적인 트리를 만든다.
//
// 새 페이지(예: customers) 추가 시 여기 필드 하나 + LoadTemplates 에 ParseFS 한 번 추가.
type PageTemplates struct {
	Products  *template.Template
	Purchases *template.Template
}

// LoadTemplates — 페이지별 템플릿 세트를 한 번에 로드.
// 각 세트는 layout.html + 해당 페이지의 모든 파일들을 포함한다.
func LoadTemplates() (*PageTemplates, error) {
	products, err := template.New("").
		Funcs(tmplFuncs).
		ParseFS(web.Templates,
			"templates/layout.html",
			"templates/products/list.html",
			"templates/products/_tbody.html",
			"templates/products/_form.html",
			"templates/products/_options.html",
		)
	if err != nil {
		return nil, fmt.Errorf("products 템플릿 파싱 실패: %w", err)
	}

	purchases, err := template.New("").
		Funcs(tmplFuncs).
		ParseFS(web.Templates,
			"templates/layout.html",
			"templates/purchases/list.html",
			"templates/purchases/_tbody.html",
			"templates/purchases/_form.html",
		)
	if err != nil {
		return nil, fmt.Errorf("purchases 템플릿 파싱 실패: %w", err)
	}

	return &PageTemplates{
		Products:  products,
		Purchases: purchases,
	}, nil
}
