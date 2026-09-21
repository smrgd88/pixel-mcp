# 위험 작업 경고 계약 (SAFE-01)

`flatten_layers`, `quantize_palette`, `scale_sprite`의 성공 응답은 선택적 `warnings` 배열을 제공한다. 기존 필드와 입력은 유지한다. PR #11로 develop `6c9ac5e`에 병합했다. 릴리스 버전은 아직 Unreleased이며 태그 포함 시 [기능표](CAPABILITIES.md)를 갱신한다.

```json
{
  "success": true,
  "warnings": [
    {
      "code": "layer_flattening",
      "message": "Flattening merges layers into one and may discard editable layer structure. Keep a backup to preserve separate layers."
    }
  ]
}
```

## 필드와 의미

- `warnings`는 `{code, message}` 객체의 배열이다. 해당 경고가 없으면 필드 자체를 생략한다 (`null`이나 빈 배열을 요구하지 않는다).
- `code`는 안정적인 기계 판독용 식별자다. 클라이언트는 알 수 없는 코드도 표시할 수 있어야 한다. `message`는 표시용 설명이며 분기 조건으로 사용하지 않는다.
- 성공한 작업이 가질 수 있는 손실 가능성을 알린다. 실제 픽셀 손실량이나 변경 전후 차이를 측정한 결과가 아니다.
- 응답은 **작업 완료 후** 반환된다. 실행 전 승인·취소·preview·backup·undo를 제공하지 않는다. 실행 전 검토는 후속 dry-run(SAFE-02)의 범위다.
- 오류 반환 경로는 기존 오류 계약을 유지하며 성공 warnings를 붙이지 않는다. 경고가 없다고 모든 작업이 무손실이거나 안전하다는 뜻은 아니다. 다른 47개 도구는 이번 적용 범위 밖이다.
- MCP SDK는 같은 출력 객체를 `structuredContent`와 JSON `TextContent`로 직렬화한다. warnings는 두 표현에서 같다.
- stderr 로그는 운영 진단용이다. 클라이언트는 stderr 문구를 파싱하지 않고 응답의 warnings를 사용한다. warnings는 timing/debug 설정과 무관하며 경로·사용자 입력을 메시지에 삽입하지 않는다.

## 코드와 발생 조건

| 코드 | 도구 | 조건 | 의미 |
| --- | --- | --- | --- |
| `palette_quantization` | `quantize_palette` | 성공한 모든 감색 요청 | 팔레트 대체와 색상 정보 손실 가능성 |
| `color_mode_conversion` | `quantize_palette` | `convert_to_indexed=true` (생략 시 true) | indexed 변환 요청에 따른 색상·투명도 표현 변경 가능성. 이미 indexed인 경우도 요청 기준으로 반환 |
| `layer_flattening` | `flatten_layers`, `quantize_palette` | 모든 flatten 요청 및 `dither=true` 감색 요청 | 편집 가능한 레이어 구조 손실 가능성. 단일 레이어에서도 작업 종류 기준으로 반환 |
| `resampling` | `scale_sprite` | `algorithm=bilinear` 또는 `rotsprite`, X/Y 중 하나라도 배율이 1이 아님 | 픽셀 색·경계 패턴 변경 가능성 |

`scale_sprite`의 알고리즘 빈 문자열/nearest 및 배율 1×1은 resampling 경고가 없다. 현재 MCP 입력 schema는 algorithm 키를 필수로 요구하며 빈 문자열은 nearest로 처리한다. 실제 정수 출력 크기가 반올림 등으로 같아지는 경우도 입력 배율 기준이다. 감색 경고 순서는 palette_quantization, color_mode_conversion(변환 요청 시), layer_flattening(dither 사용 시)이며 동일 코드는 중복 반환하지 않는다.

`quantize_palette(dither=true)`는 현재 내부 `ReplaceWithImage` 경로에서 레이어를 병합하고 첫 프레임 cel을 대체한다. indexed 변환 옵션과 무관하게 layer_flattening 경고를 반환한다. `dither=false`에서는 이 경고를 추가하지 않는다.

범용 색상 모드 전환 도구는 아직 없다. 이번 색상 모드 경고는 기존 quantize_palette의 indexed 변환 옵션에 적용한다.

## 호환성·검증

- 기존 필드를 읽고 추가 필드를 무시하는 JSON 클라이언트는 기존 방식으로 응답을 읽을 수 있다.
- output schema에서 warnings는 optional이다. 경고 없는 이전 응답도 새 출력 타입으로 읽을 수 있다.
- 검증: 조건별 table-driven unit test, 실제 Aseprite 기반 MCP 호출, tools/list의 optional schema, text/structured 응답 일치, 경고 없는 응답의 필드 생략, 실패 응답, legacy JSON decode.
- 실제 Claude/Codex/Gemini 앱 UI에서의 표시 동작은 자동 MCP contract test와 별개이며 이번에 검증하지 않았다. 응답 필드를 고정하고 추가 필드를 거부하는 외부 클라이언트는 schema 갱신이 필요할 수 있다.

테스트 파일: [warnings_test.go](../pkg/tools/warnings_test.go), [warnings_integration_test.go](../pkg/tools/warnings_integration_test.go).

## 실행 검증 기록

2026-09-17, 기준 develop `3a20ded` + SAFE-01 작업 변경. Linux amd64 Docker, Go 1.25.14, Aseprite 실행 버전 `1.3.18.3-dev`에서 다음을 통과했다.

- `go build ./...`
- `go vet ./...`
- `go test -race -cover ./...` — pkg/tools coverage 68.9%
- `go test -tags=integration ./...` — pkg/tools 60.453초
- 집중 warning 조건/MCP contract 테스트 및 문서 링크·공백 검사

지원 하한 1.3.17.2 재실행은 이번 범위에서 제외했다. 기존 방침에 따라 릴리스 직전 또는 명시적 요청 시 재검증한다.
