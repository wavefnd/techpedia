<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft, Database, Languages, Link2, ListTree, Trash2 } from '@lucide/vue'
import { deleteTechdataItem, getTechdataItem } from '../services/api'
import type { TechdataItemResponse } from '../types/api'
import { activeLanguageEditions, loadLanguageEditions } from '../services/editions'
import { useWikiI18n } from '../i18n'
import { hasPermission } from '../services/auth'

const props = defineProps<{ tid: string }>()
const router = useRouter()
const item = ref<TechdataItemResponse | null>(null)
const error = ref('')
const deleteConfirmation = ref('')
const deleteError = ref('')
const deleting = ref(false)
const { locale, uiLocale } = useWikiI18n()
const label = computed(() => localizedValue(item.value?.labels) || item.value?.id || '')
const description = computed(() => localizedValue(item.value?.descriptions))
const itemLocales = computed(() => {
  const locales = new Set(activeLanguageEditions.value.map((edition) => edition.tag))
  for (const values of [item.value?.labels, item.value?.descriptions, item.value?.aliases, item.value?.wiki_pages]) {
    Object.keys(values || {}).forEach((value) => locales.add(value))
  }
  return [...locales].sort((left, right) => left.localeCompare(right, uiLocale.value))
})
const itemHasDirectRelationships = computed(() => Boolean(
  item.value && (Object.keys(item.value.wiki_pages).length > 0 || item.value.statements.length > 0),
))

onMounted(async () => {
  try {
    const [loadedItem] = await Promise.all([getTechdataItem(props.tid), loadLanguageEditions()])
    item.value = loadedItem
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : 'Techdata 항목을 불러오지 못했습니다.'
  }
})

function displayValue(value: TechdataItemResponse['statements'][number]['value']): string {
  if (value.type === 'item') return value.item || ''
  if (value.type === 'quantity') return [value.amount, value.unit].filter(Boolean).join(' ')
  return value.text || ''
}

function localizedValue(values?: Partial<Record<string, string>>): string {
  if (!values) return ''
  return values[locale.value] || values[uiLocale.value] || Object.values(values).find(Boolean) || ''
}

async function removeItem() {
  const target = item.value
  if (!target || deleteConfirmation.value.trim() !== target.id || itemHasDirectRelationships.value) return
  if (!window.confirm(`${target.id} 항목을 영구 삭제합니다. 이 작업은 되돌릴 수 없습니다.`)) return
  deleting.value = true
  deleteError.value = ''
  try {
    await deleteTechdataItem(target.id, deleteConfirmation.value.trim())
    await router.push('/admin/pages')
  } catch (reason) {
    deleteError.value = reason instanceof Error ? reason.message : 'Techdata 항목을 삭제하지 못했습니다.'
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div class="admin-view techdata-admin-view">
    <div v-if="error" class="page-state error">{{ error }}</div>
    <div v-else-if="!item" class="admin-loading"><VaProgressCircle indeterminate /> Techdata 항목을 불러오는 중입니다.</div>
    <template v-else>
      <header class="admin-page-header techdata-admin-header">
        <div>
          <p>중앙 지식 데이터</p>
          <h1>{{ label }}</h1>
          <span>{{ description || '설명이 등록되지 않았습니다.' }}</span>
        </div>
        <div class="techdata-header-actions">
          <code>{{ item.id }}</code>
          <RouterLink class="techdata-back-link" to="/admin/pages"><ArrowLeft :size="16" />문서 관리</RouterLink>
        </div>
      </header>

      <div class="admin-metrics techdata-metrics">
        <VaCard><VaCardContent><span>언어별 이름</span><strong>{{ Object.keys(item.labels).length }}</strong><Languages :size="20" /></VaCardContent></VaCard>
        <VaCard><VaCardContent><span>연결된 문서</span><strong>{{ Object.keys(item.wiki_pages).length }}</strong><Link2 :size="20" /></VaCardContent></VaCard>
        <VaCard><VaCardContent><span>등록된 문장</span><strong>{{ item.statements.length }}</strong><ListTree :size="20" /></VaCardContent></VaCard>
      </div>

      <VaCard>
        <VaCardTitle><Database :size="18" />언어별 메타데이터</VaCardTitle>
        <VaCardContent class="techdata-data-list">
          <div v-for="itemLocale in itemLocales" :key="itemLocale" class="techdata-data-row">
            <b>{{ itemLocale }}</b>
            <div><strong>{{ item.labels[itemLocale] || '이름 없음' }}</strong><span>{{ item.descriptions[itemLocale] || '설명 없음' }}</span></div>
            <p>{{ item.aliases[itemLocale]?.join(', ') || '별칭 없음' }}</p>
          </div>
        </VaCardContent>
      </VaCard>

      <VaCard>
        <VaCardTitle><Link2 :size="18" />연결된 위키 문서</VaCardTitle>
        <VaCardContent class="techdata-wiki-list">
          <div v-for="(wid, itemLocale) in item.wiki_pages" :key="itemLocale" class="techdata-wiki-row">
            <VaBadge :text="String(itemLocale)" color="secondary" />
            <span>해당 언어판 문서</span>
            <code>{{ wid }}</code>
          </div>
          <p v-if="!Object.keys(item.wiki_pages).length" class="admin-empty">연결된 문서가 없습니다.</p>
          <small class="techdata-management-note">문서 연결 생성·변경·해제는 문서 관리에서 처리합니다.</small>
        </VaCardContent>
      </VaCard>

      <VaCard>
        <VaCardTitle><ListTree :size="18" />문장</VaCardTitle>
        <VaCardContent class="techdata-statement-list">
          <div v-for="statement in item.statements" :key="`${statement.property}-${displayValue(statement.value)}`" class="techdata-statement-row">
          <div><strong>속성</strong><small>{{ statement.property }}</small></div>
          <p>{{ displayValue(statement.value) }}</p>
            <VaBadge v-if="statement.rank === 'preferred'" text="선호 값" color="success" />
          </div>
          <p v-if="!item.statements.length" class="admin-empty">등록된 문장이 없습니다.</p>
        </VaCardContent>
      </VaCard>

      <section v-if="hasPermission('owner')" class="techdata-danger-zone" aria-labelledby="techdata-delete-title">
        <div>
          <h2 id="techdata-delete-title"><Trash2 :size="18" />Techdata 항목 영구 삭제</h2>
          <p>잘못 생성되어 더 이상 쓰이지 않는 항목만 삭제할 수 있습니다. TID는 삭제 후에도 재사용되지 않습니다.</p>
          <p v-if="itemHasDirectRelationships" class="techdata-delete-blocked">먼저 연결된 문서를 모두 해제하고 문장을 제거해야 합니다.</p>
          <p v-else>다른 Techdata 항목이 이 항목을 참조하면 서버가 삭제를 거부합니다.</p>
        </div>
        <form @submit.prevent="removeItem">
          <VaInput v-model="deleteConfirmation" :label="`${item.id} 입력`" :placeholder="item.id" :disabled="itemHasDirectRelationships || deleting" />
          <VaButton type="submit" color="danger" :loading="deleting" :disabled="itemHasDirectRelationships || deleteConfirmation.trim() !== item.id">영구 삭제</VaButton>
        </form>
        <VaAlert v-if="deleteError" color="danger">{{ deleteError }}</VaAlert>
      </section>
    </template>
  </div>
</template>

<style scoped>
.techdata-danger-zone {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(280px, 380px);
  gap: 20px;
  padding: 20px;
  border: 1px solid color-mix(in srgb, var(--va-danger) 55%, transparent);
  border-radius: 6px;
  background: color-mix(in srgb, var(--va-danger) 5%, transparent);
}

.techdata-danger-zone h2,
.techdata-danger-zone form {
  display: flex;
  align-items: center;
  gap: 10px;
}

.techdata-danger-zone h2 {
  margin: 0 0 8px;
  color: var(--va-danger);
  font-size: 1rem;
}

.techdata-danger-zone p {
  margin: 4px 0;
}

.techdata-danger-zone form > :first-child {
  flex: 1;
}

.techdata-delete-blocked {
  color: var(--va-danger);
  font-weight: 700;
}

@media (max-width: 760px) {
  .techdata-danger-zone {
    grid-template-columns: 1fr;
  }

  .techdata-danger-zone form {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
