import { onScopeDispose, ref, watch, type Ref } from 'vue'
import { ApiError, getArticle } from '../services/api'
import { LatestRequest } from '../services/latestRequest'
import { consumeInitialArticle } from '../services/initialPage'
import type { ArticleResponse, Locale } from '../types/api'

export function useArticle(locale: Ref<Locale>, name: Ref<string>) {
  const article = ref<ArticleResponse | null>(null)
  const loading = ref(true)
  const missing = ref(false)
  const inactiveEdition = ref(false)
  const error = ref<string | null>(null)
  const requests = new LatestRequest()

  async function load() {
    loading.value = true
    article.value = null
    missing.value = false
    inactiveEdition.value = false
    error.value = null
    await requests.run((signal) => getArticle(locale.value, name.value, signal), {
      success: (value) => { article.value = value },
      error: (reason) => {
        if (reason instanceof ApiError && reason.status === 404) missing.value = true
        else if (reason instanceof ApiError && reason.status === 409) inactiveEdition.value = true
        else error.value = reason instanceof Error ? reason.message : '문서를 불러오지 못했습니다.'
      },
      settled: () => { loading.value = false },
    })
  }

  watch([locale, name], () => {
    const initial = consumeInitialArticle(locale.value, name.value)
    if (initial) {
      requests.invalidate()
      article.value = initial
      loading.value = false
      missing.value = false
      inactiveEdition.value = false
      error.value = null
      return
    }
    void load()
  }, { immediate: true, flush: 'sync' })
  onScopeDispose(() => requests.dispose())

  return { article, loading, missing, inactiveEdition, error, reload: load }
}
