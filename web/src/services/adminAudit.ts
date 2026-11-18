export const auditActionNames: Record<string, string> = {
  'box.save': '공용 박스 저장',
  'media.delete': '업로드 이미지 삭제', 'media.cleanup': '삭제 이미지 저장공간 정리',
  'document.move': '문서 이동', 'document.delete': '문서 삭제', 'document.restore': '문서 복구',
  'revision.restore': '리비전 복원', 'document.history_reset': '문서 역사 초기화', 'user.permissions': '사용자 권한 변경',
  'user.regular_permissions': '일반 사용자 정책 변경', 'edition.activate': '언어판 활성화',
  'edition.approve': '언어판 승인', 'edition.reject': '언어판 거절', 'edition.interface_translate': '인터페이스 자동 번역',
  'edition.interface': '레이아웃 번역 변경', 'edition.navigation': '둘러보기 변경', 'edition.portal': '언어 포털 설정 변경',
  'techdata.link': 'Techdata 연결', 'techdata.unlink': 'Techdata 연결 해제', 'techdata.item_delete': 'Techdata 항목 삭제',
  'techdata.create_duplicate_blocked': '중복 Techdata 생성 차단',
  'document.redirect_create': '문서 리다이렉트 생성', 'document.redirect_delete': '문서 리다이렉트 삭제',
  'oauth.client_register': 'ChatGPT 클라이언트 등록', 'oauth.client_disable': 'ChatGPT 클라이언트 중지',
  'oauth.approve': 'ChatGPT 연결 승인', 'oauth.deny': 'ChatGPT 연결 거절', 'oauth.revoke': 'ChatGPT 연결 해제',
  'oauth.reuse_revoke': '토큰 재사용 감지·연결 해제',
  'mcp.proposal_submit': 'ChatGPT 변경 제안', 'mcp.proposal_amend': 'ChatGPT 제안 수정본 제출', 'mcp.proposal_decide': 'ChatGPT 제안 검토', 'mcp.proposal_policy': 'ChatGPT 제안 보관 설정',
}
export const auditTargetNames: Record<string, string> = { box: '공용 박스', document: '문서', redirect: '리다이렉트', edition: '언어판', user: '사용자', user_policy: '사용자 정책', techdata: 'Techdata', oauth: 'ChatGPT 연결', mcp_proposal: 'ChatGPT 변경 제안', media: '업로드 이미지' }
export function auditActionName(action: string) { return auditActionNames[action] || '알 수 없는 작업' }
export function auditResult(result: string) {
  if (result === 'success') return { text: '성공', color: 'success' }
  if (result === 'applied') return { text: '승인·적용', color: 'success' }
  if (result === 'rejected') return { text: '거절', color: 'warning' }
  if (result === 'attempt') return { text: '요청', color: 'info' }
  if (result === 'blocked') return { text: '차단', color: 'warning' }
  if (result === 'failed' || result === 'failure') return { text: '실패', color: 'danger' }
  return { text: '알 수 없음', color: 'secondary' }
}
