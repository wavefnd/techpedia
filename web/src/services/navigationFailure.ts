import { ref } from 'vue'

export const navigationFailed = ref(false)
export function reportNavigationFailure() { navigationFailed.value = true }
