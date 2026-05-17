<script setup lang="ts">
import { onMounted, onUnmounted, watch } from 'vue'

const props = defineProps<{ open: boolean; title?: string }>()
const emit = defineEmits<{ 'update:open': [v: boolean] }>()

function close() {
  emit('update:open', false)
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.open) close()
}

onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))

watch(
  () => props.open,
  (v) => {
    document.body.style.overflow = v ? 'hidden' : ''
  },
)
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-150"
      leave-active-class="transition-opacity duration-150"
      enter-from-class="opacity-0"
      leave-to-class="opacity-0"
    >
      <div
        v-if="open"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
        @click.self="close"
      >
        <div
          class="w-full max-w-md rounded-lg border border-border bg-card text-card-foreground shadow-lg p-5"
          @click.stop
        >
          <div v-if="title" class="mb-4 text-lg font-semibold">{{ title }}</div>
          <slot />
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
