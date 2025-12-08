import { createSignal, For, Show } from "solid-js"
import { useUrp } from "../context/urp"

interface FileNode {
  name: string
  path: string
  type: "file" | "dir"
  children?: FileNode[]
}

export function Sidebar() {
  const urp = useUrp()
  const [activeTab, setActiveTab] = createSignal<"sessions" | "files" | "focus">("sessions")
  const [expandedDirs, setExpandedDirs] = createSignal<Set<string>>(new Set())

  function toggleDir(path: string) {
    setExpandedDirs(prev => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }

  return (
    <div class="w-64 h-full flex flex-col bg-background-base border-r border-border-base">
      {/* Tab buttons */}
      <div class="flex border-b border-border-base">
        <button
          onClick={() => setActiveTab("sessions")}
          classList={{
            "flex-1 px-3 py-2 text-xs font-medium": true,
            "bg-background-raised text-text-strong border-b-2 border-accent-primary": activeTab() === "sessions",
            "text-text-weak hover:bg-background-overlay": activeTab() !== "sessions"
          }}
        >
          Sessions
        </button>
        <button
          onClick={() => setActiveTab("files")}
          classList={{
            "flex-1 px-3 py-2 text-xs font-medium": true,
            "bg-background-raised text-text-strong border-b-2 border-accent-primary": activeTab() === "files",
            "text-text-weak hover:bg-background-overlay": activeTab() !== "files"
          }}
        >
          Files
        </button>
        <button
          onClick={() => setActiveTab("focus")}
          classList={{
            "flex-1 px-3 py-2 text-xs font-medium": true,
            "bg-background-raised text-text-strong border-b-2 border-accent-primary": activeTab() === "focus",
            "text-text-weak hover:bg-background-overlay": activeTab() !== "focus"
          }}
        >
          Focus
        </button>
      </div>

      {/* Content */}
      <div class="flex-1 overflow-y-auto">
        <Show when={activeTab() === "sessions"}>
          <SessionsPanel />
        </Show>
        <Show when={activeTab() === "files"}>
          <FilesPanel />
        </Show>
        <Show when={activeTab() === "focus"}>
          <FocusPanel />
        </Show>
      </div>

      {/* New session button */}
      <div class="p-2 border-t border-border-base">
        <button class="w-full px-3 py-2 text-xs font-medium text-text-base bg-accent-primary/10
                       hover:bg-accent-primary/20 rounded flex items-center justify-center gap-2">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          New Session
        </button>
      </div>
    </div>
  )
}

function SessionsPanel() {
  const urp = useUrp()

  return (
    <div class="p-2 space-y-1">
      <Show when={urp.sessions().length === 0}>
        <div class="text-xs text-text-weaker text-center py-4">
          No sessions yet
        </div>
      </Show>
      <For each={urp.sessions()}>
        {(session) => (
          <button class="w-full px-3 py-2 text-left rounded hover:bg-background-overlay group">
            <div class="flex items-center gap-2">
              <svg class="w-4 h-4 text-text-weaker" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
              </svg>
              <div class="flex-1 min-w-0">
                <div class="text-xs font-medium text-text-base truncate">
                  {session.title || `Session ${session.id.slice(0, 8)}`}
                </div>
                <div class="text-xs text-text-weaker">
                  {session.messages} msgs · {new Date(session.updatedAt).toLocaleDateString()}
                </div>
              </div>
            </div>
          </button>
        )}
      </For>
    </div>
  )
}

function FilesPanel() {
  return (
    <div class="p-2">
      <div class="text-xs text-text-weaker text-center py-4">
        File explorer coming soon
      </div>
    </div>
  )
}

function FocusPanel() {
  const urp = useUrp()
  const [target, setTarget] = createSignal("")
  const [depth, setDepth] = createSignal(2)

  async function handleFocus() {
    if (!target()) return
    await urp.setFocus(target(), depth())
    setTarget("")
  }

  return (
    <div class="p-3 space-y-4">
      {/* Current focus */}
      <Show when={urp.status().focus}>
        <div class="p-2 bg-background-raised rounded">
          <div class="text-xs text-text-weak mb-1">Current Focus</div>
          <div class="text-sm text-accent-secondary font-mono">
            {urp.status().focus?.target}
          </div>
          <div class="text-xs text-text-weaker mt-1">
            depth: {urp.status().focus?.depth}
          </div>
        </div>
      </Show>

      {/* Set focus form */}
      <div class="space-y-2">
        <label class="text-xs text-text-weak">Set Focus Target</label>
        <input
          type="text"
          value={target()}
          onInput={(e) => setTarget(e.currentTarget.value)}
          placeholder="e.g. main.go, UserService"
          class="w-full px-2 py-1.5 text-xs bg-background-base border border-border-base rounded
                 focus:outline-none focus:border-accent-primary"
        />
        <div class="flex items-center gap-2">
          <label class="text-xs text-text-weak">Depth:</label>
          <input
            type="number"
            value={depth()}
            onInput={(e) => setDepth(parseInt(e.currentTarget.value) || 2)}
            min={1}
            max={5}
            class="w-16 px-2 py-1 text-xs bg-background-base border border-border-base rounded
                   focus:outline-none focus:border-accent-primary"
          />
        </div>
        <button
          onClick={handleFocus}
          disabled={!target()}
          class="w-full px-3 py-1.5 text-xs font-medium bg-accent-primary text-white rounded
                 hover:bg-accent-secondary disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Set Focus
        </button>
      </div>

      {/* Quick focus buttons */}
      <div class="space-y-1">
        <div class="text-xs text-text-weak">Quick Focus</div>
        <div class="grid grid-cols-2 gap-1">
          <button
            onClick={() => { setTarget("main.go"); handleFocus() }}
            class="px-2 py-1 text-xs text-text-base bg-background-raised rounded hover:bg-background-overlay"
          >
            main.go
          </button>
          <button
            onClick={() => { setTarget("README"); handleFocus() }}
            class="px-2 py-1 text-xs text-text-base bg-background-raised rounded hover:bg-background-overlay"
          >
            README
          </button>
        </div>
      </div>
    </div>
  )
}
