package models

import "time"

// Customer — 고객 마스터.
//
// Mobile, BirthDate, Email, Address, Memo 는 DB 에서 NULL 허용.
// Go 모델에서는 단순히 빈 문자열("")/zero time.Time 으로 처리하고,
// repository 에서 INSERT 시 빈 값을 NULL 로 변환한다.
//
// RegistrationDate 는 기본값이 오늘 (DB DEFAULT CURRENT_DATE) 이지만
// 폼에서 사용자가 수정 가능하다. 핸들러에서 항상 값을 채워서 저장한다.
type Customer struct {
	ID               int64
	Name             string    // 필수, 50자 이내
	Mobile           string    // 선택, 20자 이내, UNIQUE (DB)
	BirthDate        time.Time // 선택 (zero time = NULL)
	Email            string    // 선택, 100자 이내
	Address          string    // 선택, 200자 이내
	Memo             string    // 선택, 500자 이내
	RegistrationDate time.Time // 필수
}
