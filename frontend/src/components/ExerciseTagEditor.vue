<script setup lang="ts">
import { ref, watch } from 'vue'
import { api } from '@/lib/api'
import Dialog from '@/components/ui/Dialog.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{
  'update:open': [v: boolean]
  saved: []
}>()

const name = ref<string>('')
const saving = ref<boolean>(false)
const errMsg = ref<string>('')

function reset(): void {
  name.value = ''
  errMsg.value = ''
  saving.value = false
}

watch(
  () => props.open,
  (open) => {
    if (open) reset()
  },
)

async function save(): Promise<void> {
  const trimmed = name.value.trim()
  if (!trimmed) {
    errMsg.value = 'Name required'
    return
  }
  saving.value = true
  errMsg.value = ''
  try {
    await api.createExerciseTag(trimmed)
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to create tag'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog
    :open="open"
    title="New tag"
    @update:open="(v) => emit('update:open', v)"
  >
    <div class="flex flex-col gap-3">
      <div>
        <label class="text-xs text-muted-foreground">Tag name</label>
        <Input v-model="name" placeholder="e.g. upper" @keyup.enter="save" />
      </div>

      <p v-if="errMsg" class="text-sm text-destructive">{{ errMsg }}</p>

      <div class="flex justify-end gap-2">
        <Button variant="ghost" @click="emit('update:open', false)">Cancel</Button>
        <Button :disabled="saving" @click="save">
          {{ saving ? 'Saving…' : 'Add' }}
        </Button>
      </div>
    </div>
  </Dialog>
</template>
