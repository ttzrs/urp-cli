import { createSignal, createEffect, For, Show, onMount, onCleanup } from "solid-js"

interface Command {
  id: string
  name: string
  shortcut?: string
  category: string
  action: () => void
}

interface CommandPaletteProps {
  commands: Command[]
  isOpen: boolean
  onClose: () => void
}

export function CommandPalette(props: CommandPaletteProps) {
  const [query, setQuery] = createSignal("")
  const [selectedIndex, setSelectedIndex] = createSignal(0)
  let inputRef: HTMLInputElement | undefined

  const filteredCommands = () => {
    const q = query().toLowerCase()
    if (!q) return props.commands
    return props.commands.filter(cmd =>
      cmd.name.toLowerCase().includes(q) ||
      cmd.category.toLowerCase().includes(q)
    )
  }

  // Reset selection when query changes
  createEffect(() => {
    query() // track
    setSelectedIndex(0)
  })

  // Focus input when opened
  createEffect(() => {
    if (props.isOpen) {
      setTimeout(() => inputRef?.focus(), 50)
    }
  })

  function handleKeyDown(e: KeyboardEvent) {
    const commands = filteredCommands()

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault()
        setSelectedIndex(i => Math.min(i + 1, commands.length - 1))
        break
      case "ArrowUp":
        e.preventDefault()
        setSelectedIndex(i => Math.max(i - 1, 0))
        break
      case "Enter":
        e.preventDefault()
        if (commands[selectedIndex()]) {
          commands[selectedIndex()].action()
          props.onClose()
        }
        break
      case "Escape":
        e.preventDefault()
        props.onClose()
        break
    }
  }

  // Group commands by category
  const groupedCommands = () => {
    const groups: Record<string, Command[]> = {}
    for (const cmd of filteredCommands()) {
      if (!groups[cmd.category]) groups[cmd.category] = []
      groups[cmd.category].push(cmd)
    }
    return groups
  }

  if (!props.isOpen) return null

  return (
    <div class="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]">
      {/* Backdrop */}
      <div
        class="absolute inset-0 bg-black/50 backdrop-blur-sm"
        onClick={props.onClose}
      />

      {/* Palette */}
      <div class="relative w-full max-w-lg bg-background-raised border border-border-base rounded-xl
                  shadow-2xl overflow-hidden animate-in fade-in slide-in-from-top-4 duration-200">
        {/* Search input */}
        <div class="flex items-center gap-3 px-4 py-3 border-b border-border-base">
          <svg class="w-5 h-5 text-text-weaker" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            ref={inputRef}
            type="text"
            value={query()}
            onInput={(e) => setQuery(e.currentTarget.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a command or search..."
            class="flex-1 bg-transparent text-sm text-text-base placeholder:text-text-weaker
                   focus:outline-none"
          />
          <kbd class="px-1.5 py-0.5 text-xs text-text-weaker bg-background-base rounded border border-border-base">
            esc
          </kbd>
        </div>

        {/* Commands list */}
        <div class="max-h-80 overflow-y-auto py-2">
          <Show
            when={filteredCommands().length > 0}
            fallback={
              <div class="px-4 py-8 text-center text-sm text-text-weaker">
                No commands found
              </div>
            }
          >
            <For each={Object.entries(groupedCommands())}>
              {([category, commands], categoryIndex) => {
                // Calculate flat index for selection
                let startIndex = 0
                const entries = Object.entries(groupedCommands())
                for (let i = 0; i < categoryIndex(); i++) {
                  startIndex += entries[i][1].length
                }

                return (
                  <div>
                    <div class="px-4 py-1.5 text-xs font-medium text-text-weaker uppercase tracking-wider">
                      {category}
                    </div>
                    <For each={commands}>
                      {(cmd, cmdIndex) => {
                        const flatIndex = startIndex + cmdIndex()
                        return (
                          <button
                            onClick={() => {
                              cmd.action()
                              props.onClose()
                            }}
                            classList={{
                              "w-full px-4 py-2 flex items-center justify-between": true,
                              "bg-accent-primary/10": selectedIndex() === flatIndex,
                              "hover:bg-background-overlay": selectedIndex() !== flatIndex
                            }}
                          >
                            <span class="text-sm text-text-base">{cmd.name}</span>
                            <Show when={cmd.shortcut}>
                              <kbd class="px-1.5 py-0.5 text-xs text-text-weaker bg-background-base rounded
                                          border border-border-base">
                                {cmd.shortcut}
                              </kbd>
                            </Show>
                          </button>
                        )
                      }}
                    </For>
                  </div>
                )
              }}
            </For>
          </Show>
        </div>
      </div>
    </div>
  )
}

// Global keyboard shortcut hook
export function useCommandPalette(commands: Command[]) {
  const [isOpen, setIsOpen] = createSignal(false)

  onMount(() => {
    function handleKeyDown(e: KeyboardEvent) {
      // Ctrl+K or Cmd+K to open
      if ((e.ctrlKey || e.metaKey) && e.key === "k") {
        e.preventDefault()
        setIsOpen(true)
      }
    }

    window.addEventListener("keydown", handleKeyDown)
    onCleanup(() => window.removeEventListener("keydown", handleKeyDown))
  })

  return {
    isOpen,
    open: () => setIsOpen(true),
    close: () => setIsOpen(false),
    commands
  }
}
