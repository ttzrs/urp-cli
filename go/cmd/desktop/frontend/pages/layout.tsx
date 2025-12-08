import { ParentProps, createSignal, onMount, onCleanup } from "solid-js"
import { StatusBar, ContextBar } from "../components/status-bar"
import { Sidebar } from "../components/sidebar"
import { AgentSwitcher, AgentType } from "../components/agent-switcher"
import { CommandPalette, useCommandPalette } from "../components/command-palette"
import { useUrp } from "../context/urp"

export default function Layout(props: ParentProps) {
  const urp = useUrp()
  const [currentAgent, setCurrentAgent] = createSignal<AgentType>("build")
  const [sidebarOpen, setSidebarOpen] = createSignal(true)

  // Command palette
  const commands = [
    { id: "new-session", name: "New Session", shortcut: "Ctrl+N", category: "Session", action: () => console.log("new session") },
    { id: "clear-chat", name: "Clear Chat", shortcut: "Ctrl+L", category: "Session", action: () => console.log("clear") },
    { id: "toggle-sidebar", name: "Toggle Sidebar", shortcut: "Ctrl+B", category: "View", action: () => setSidebarOpen(!sidebarOpen()) },
    { id: "focus-input", name: "Focus Input", shortcut: "/", category: "Navigation", action: () => document.querySelector<HTMLTextAreaElement>("#prompt-input")?.focus() },
    { id: "urp-focus", name: "Set Focus Target", category: "URP", action: () => console.log("focus") },
    { id: "urp-refresh", name: "Refresh Status", shortcut: "Ctrl+R", category: "URP", action: () => urp.refresh() },
    { id: "urp-wisdom", name: "Query Wisdom", category: "URP", action: () => console.log("wisdom") },
    { id: "urp-vitals", name: "System Vitals", category: "URP", action: () => console.log("vitals") },
  ]

  const palette = useCommandPalette(commands)

  // Global keyboard shortcuts
  onMount(() => {
    function handleKeyDown(e: KeyboardEvent) {
      // Ctrl+B: toggle sidebar
      if ((e.ctrlKey || e.metaKey) && e.key === "b") {
        e.preventDefault()
        setSidebarOpen(!sidebarOpen())
      }

      // Ctrl+L: clear chat
      if ((e.ctrlKey || e.metaKey) && e.key === "l") {
        e.preventDefault()
        // TODO: implement clear
      }

      // ? for help (when not in input)
      if (e.key === "?" && !(e.target as HTMLElement).matches("input, textarea")) {
        e.preventDefault()
        palette.open()
      }
    }

    window.addEventListener("keydown", handleKeyDown)
    onCleanup(() => window.removeEventListener("keydown", handleKeyDown))
  })

  return (
    <div class="h-screen flex flex-col bg-background-base">
      {/* Command Palette */}
      <CommandPalette
        commands={palette.commands}
        isOpen={palette.isOpen()}
        onClose={palette.close}
      />

      {/* Header */}
      <header class="h-12 flex items-center px-4 bg-background-raised border-b border-border-base">
        <div class="flex items-center gap-3">
          {/* Sidebar toggle */}
          <button
            onClick={() => setSidebarOpen(!sidebarOpen())}
            class="p-1.5 rounded hover:bg-background-overlay text-text-weak hover:text-text-base"
            title="Toggle sidebar (Ctrl+B)"
          >
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M4 6h16M4 12h16M4 18h7" />
            </svg>
          </button>

          {/* Logo */}
          <div class="flex items-center gap-2">
            <svg class="w-6 h-6 text-accent-primary" viewBox="0 0 24 24" fill="currentColor">
              <path d="M13 3L4 14h7l-2 7 9-11h-7l2-7z" />
            </svg>
            <span class="text-sm font-bold text-text-strong">URP</span>
          </div>

          {/* Project name */}
          <div class="px-2 py-1 bg-background-base rounded text-xs text-text-weak font-mono">
            {urp.status().project}
          </div>
        </div>

        {/* Agent Switcher */}
        <div class="ml-6">
          <AgentSwitcher current={currentAgent()} onChange={setCurrentAgent} />
        </div>

        <div class="flex-1" />

        {/* Right side controls */}
        <div class="flex items-center gap-2">
          {/* Command palette button */}
          <button
            onClick={palette.open}
            class="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-background-base
                   border border-border-base text-sm text-text-weak hover:text-text-base
                   hover:bg-background-overlay transition-colors"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <span>Search</span>
            <kbd class="px-1.5 py-0.5 text-xs bg-background-raised rounded border border-border-base">
              Ctrl+K
            </kbd>
          </button>

          {/* Settings */}
          <button class="p-2 rounded hover:bg-background-overlay text-text-weak hover:text-text-base">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            </svg>
          </button>
        </div>
      </header>

      {/* Status Bar */}
      <StatusBar />

      {/* Context Bar */}
      <ContextBar />

      {/* Main content with sidebar */}
      <div class="flex-1 flex overflow-hidden">
        {/* Sidebar */}
        {sidebarOpen() && <Sidebar />}

        {/* Main content */}
        <main class="flex-1 overflow-hidden">
          {props.children}
        </main>
      </div>

      {/* Footer */}
      <footer class="h-7 flex items-center justify-between px-4 bg-background-raised border-t border-border-base text-xs text-text-weaker">
        <div class="flex items-center gap-4">
          <span class="flex items-center gap-1.5">
            <kbd class="px-1 py-0.5 bg-background-base rounded border border-border-base">Ctrl+Enter</kbd>
            send
          </span>
          <span class="flex items-center gap-1.5">
            <kbd class="px-1 py-0.5 bg-background-base rounded border border-border-base">Tab</kbd>
            switch agent
          </span>
          <span class="flex items-center gap-1.5">
            <kbd class="px-1 py-0.5 bg-background-base rounded border border-border-base">Ctrl+K</kbd>
            commands
          </span>
        </div>

        <div class="flex items-center gap-4">
          {/* Context usage */}
          <div class="flex items-center gap-2">
            <span>CTX</span>
            <div class="w-24 h-1.5 bg-background-base rounded-full overflow-hidden">
              <div
                class="h-full bg-accent-primary rounded-full transition-all duration-300"
                style={{ width: `${Math.min(100, (urp.status().ctx.tokens / 200000) * 100)}%` }}
              />
            </div>
            <span>{(urp.status().ctx.tokens / 1000).toFixed(0)}k</span>
          </div>

          {/* Version */}
          <span>URP Desktop v0.1.0</span>
        </div>
      </footer>
    </div>
  )
}
