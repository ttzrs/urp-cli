import { Show, createMemo } from "solid-js"
import { useUrp } from "../context/urp"

export function StatusBar() {
  const urp = useUrp()

  const focusDisplay = createMemo(() => {
    const f = urp.status().focus
    if (!f) return null
    return `${f.target} (d=${f.depth})`
  })

  const ctxDisplay = createMemo(() => {
    const c = urp.status().ctx
    return `${(c.tokens / 1000).toFixed(1)}k / ${c.files} files`
  })

  return (
    <div class="flex items-center gap-4 px-4 py-2 bg-background-raised border-b border-border-base text-xs">
      {/* Graph Status */}
      <div class="flex items-center gap-2">
        <div
          class="status-dot"
          classList={{
            connected: urp.status().graphConnected,
            disconnected: !urp.status().graphConnected
          }}
        />
        <span class="text-text-weak">Graph</span>
      </div>

      <div class="w-px h-4 bg-border-base" />

      {/* Project */}
      <div class="flex items-center gap-2">
        <span class="text-text-weak">PRJ:</span>
        <span class="text-text-strong font-medium">{urp.status().project}</span>
      </div>

      <div class="w-px h-4 bg-border-base" />

      {/* Focus */}
      <div class="flex items-center gap-2">
        <span class="text-text-weak">FOCUS:</span>
        <Show when={focusDisplay()} fallback={<span class="text-text-weaker">-</span>}>
          <span class="text-accent-secondary">{focusDisplay()}</span>
        </Show>
      </div>

      <div class="w-px h-4 bg-border-base" />

      {/* Context */}
      <div class="flex items-center gap-2">
        <span class="text-text-weak">CTX:</span>
        <span class="text-text-base">{ctxDisplay()}</span>
      </div>

      <div class="flex-1" />

      {/* Events & Workers */}
      <div class="flex items-center gap-4">
        <div class="flex items-center gap-2">
          <span class="text-text-weaker">Events:</span>
          <span class="text-text-base">{urp.status().eventCount}</span>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-text-weaker">Workers:</span>
          <span class="text-text-base">{urp.status().workers}</span>
        </div>
      </div>
    </div>
  )
}

export function ContextBar() {
  const urp = useUrp()

  return (
    <div class="flex items-center gap-2 px-4 py-1.5 bg-background-base border-b border-border-base text-xs">
      <Show when={urp.error()}>
        <div class="flex items-center gap-2 text-status-danger">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          <span>{urp.error()}</span>
        </div>
      </Show>

      <Show when={urp.loading()}>
        <div class="flex items-center gap-2 text-text-weak">
          <svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          <span>Loading...</span>
        </div>
      </Show>
    </div>
  )
}
