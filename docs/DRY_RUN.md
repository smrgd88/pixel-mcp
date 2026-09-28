# 임시 복사본 dry-run (SAFE-02)

`feature/shared-dry-run`에서 구현한 계약이며 병합·릴리스 상태는 [NEXT_STEPS](NEXT_STEPS.md)를 따른다.

## 입력과 응답

`quantize_palette`, `apply_auto_shading`, `flatten_layers`에 optional boolean `dry_run`을 제공한다. 생략/false는 기존 실제 변경 경로다. true는 원본을 읽어 만든 private 복사본에 같은 처리 함수를 실행하고 원본에 반영하지 않는다. 다른 도구에는 이 입력을 제공하지 않으며 지원하지 않는 도구의 추가 입력은 MCP schema에서 거부한다.

성공한 dry-run은 기존 작업 결과 필드와 함께 다음을 반환한다.

```json
{
  "success": true,
  "dry_run": true,
  "preview": {
    "before": {"width": 16, "height": 16, "color_mode": "rgb", "frame_count": 1, "layer_count": 2, "palette_size": 256},
    "after": {"width": 16, "height": 16, "color_mode": "rgb", "frame_count": 1, "layer_count": 1, "palette_size": 256},
    "would_change_file": true
  }
}
```

위 예시는 flatten의 상태 요약이다. 기존 `warnings`도 조건에 따라 함께 반환한다.

- `success`는 복사본에서 시뮬레이션이 완료됐다는 뜻이다. 실제 적용 응답에는 `dry_run`과 `preview`를 생략한다.
- layer_count는 그룹·숨김 레이어를 포함한 모든 layer node 수다. palette_size는 첫 palette의 항목 수다.
- would_change_file은 복사본의 실행 전후 파일 bytes 비교다. 픽셀만의 차이나 시각적 손실량을 뜻하지 않는다.
- 기존 작업 결과(감색 palette/color_mode, shading colors_added 등)는 복사본에 실제 적용한 결과다. 경고 코드와 조건은 유지하며 `dry_run=true`에서는 실제 적용 시의 잠재 효과로 읽는다.
- 미리보기는 상태 요약이다. 영구 PNG/복사본 경로, 적용 token, snapshot ID를 반환하지 않는다. 적용하려면 같은 도구를 dry_run 생략/false로 새로 호출한다.

## 실행과 원본 보호

1. source의 canonical path를 잠근다. 같은 source를 수정하는 협력 호출은 dry-run이 끝날 때까지 기다린다.
2. 원본을 Go에서 읽고 시스템 임시 위치에 복사한다. 원본의 내용·identity·mode 및 복사 일관성을 검사한다. 복사본은 private 디렉터리에 생성하고 원래 basename/확장자를 유지한다.
3. 해당 호출의 `Client.ExecuteLua` source binding을 복사본으로 연결한다. Aseprite는 복사본만 열고 저장한다. 기존 세 도구의 Go 계산·Lua 작업과 임시 이미지 정리 경로를 그대로 사용한다.
4. 복사본의 전후 상태와 bytes 변경 여부를 읽고 원본이 다른 프로그램에 의해 바뀌지 않았는지 다시 확인한다. 실행 중 외부 변경이 감지되면 `file_changed` 오류로 반환하며 외부 변경을 덮어쓰지 않는다.
5. 성공·오류·취소·panic unwind 시 복사본을 제거하고 잠금을 해제한다. 원본으로 교체하는 commit 단계는 없다. 이미 sprite가 연결된 중첩 작업에서 새 preview를 시작하는 것은 거부한다.

읽기 전용 또는 hard-linked source도 읽을 수 있으면 preview할 수 있다. 실제 적용은 기존 쓰기 권한/hard-link 검사를 별도로 수행한다. 성공한 preview가 실제 파일 교체 권한까지 보장하는 것은 아니다.

이 기능은 감사한 세 도구의 source 변경을 격리한다. 임의 Lua/Go 코드의 파일 접근 전체를 sandbox하는 기능은 아니다. 지원 도구를 추가할 때는 source 쓰기가 모두 bound Client를 사용하고 영구 부수 출력이 없는지 확인해야 한다.

강제 종료 시 원본으로 commit되지는 않지만 고아 임시 디렉터리가 남을 수 있다. 자동 TTL/복구 정책은 후속 작업이다. 비협력 writer의 마지막 검사 이후 변경에 대한 기존 파일 보호 한계도 유지한다.

## 재현성과 경계

- 감색과 참조 분석은 PR #22의 결정적 farthest-point k-means 초기화를 공유한다. 같은 입력/옵션에서 dry-run과 실제 실행의 결과를 재현하며, 첫 cluster 평균 갱신 전 조기 수렴을 방지한다.
- 원본·옵션·실행 환경이 같을 때 작업 결과와 상태 요약이 실제 실행과 일치하는지 회귀 테스트한다. preview 후 원본이 바뀌면 다음 실제 실행은 새 원본을 사용한다. 오래된 preview를 적용하는 token/cache 계약은 제공하지 않는다.
- 각 도구의 기존 입력 제약을 유지한다. 예를 들어 감색의 단일 프레임 범위를 dry-run으로 확대하지 않는다.
- native 파일의 직렬화 bytes와 구조/픽셀 의미는 구분한다. would_change_file은 의미상 no-op 판정이 아니다.

## 검증

실제 MCP/Aseprite로 세 도구의 원본 bytes·identity·mode·mtime 보존, 반복 preview, 작업 결과 및 적용 후 상태 일치, optional schema와 기존 응답 호환성, 잘못된 입력과 복사본 변경 후 실패를 검사한다. 파일 helper에는 오류·취소·panic 정리, 읽기 전용/hard link, 외부 변경 감지, source 잠금 대기와 중첩 binding 거부 테스트를 둔다.

2026-09-27 검증: Linux 전체 build/vet/race/integration 통과 (pkg/tools 224.175초), 최종 관련 helper race·MCP 집중 회귀 통과, 최소 Aseprite 1.3.17.2의 MCP dry-run 회귀 통과 (12.142초), macOS arm64/Aseprite 1.3.18.2-arm64의 helper·재현성·MCP 회귀 통과.

2026-09-28 PR #22 통합 검증: develop `bd13cdb`를 병합하고 감색도 공유 farthest-point 초기화를 사용하도록 충돌을 해결했다. Linux Go 1.25.14/Aseprite 1.3.18.3-dev의 build·vet·전체 race·전체 integration 통과 (pkg/tools integration 279.337초). 최소 Aseprite 1.3.17.2 dry-run 회귀 통과 (12.296초), macOS arm64/Aseprite 1.3.18.2-arm64의 preview helper·palette 초기화/재현성·MCP dry-run 회귀 통과. 최소 버전과 macOS는 관련 회귀 범위다.
