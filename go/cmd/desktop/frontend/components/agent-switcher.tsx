import { createSignal, For } from "solid-js"

export type AgentType = "build" | "plan" | "general"

interface Agent {
  id: AgentType
  name: string
  description: string
  icon: string
  color: string
}

const AGENTS: Agent[] = [
  {
    id: "build",
    name: "Build",
    description: "Full access to tools, files, and shell",
    icon: "M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z",
    color: "text-green-400"
  },
  {
    id: "plan",
    name: "Plan",
    description: "Read-only exploration, safe for new codebases",
    icon: "M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01",
    color: "text-blue-400"
  },
  {
    id: "general",
    name: "General",
    description: "General knowledge and web search",
    icon: "M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z",
    color: "text-purple-400"
  }
]

interface AgentSwitcherProps {
  current: AgentType
  onChange: (agent: AgentType) => void
}

export function AgentSwitcher(props: AgentSwitcherProps) {
  const [isOpen, setIsOpen] = createSignal(false)

  const currentAgent = () => AGENTS.find(a => a.id === props.current) || AGENTS[0]

  function selectAgent(agent: AgentType) {
    props.onChange(agent)
    setIsOpen(false)
  }

  // Handle Tab key globally
  if (typeof window !== "undefined") {
    window.addEventListener("keydown", (e) => {
      if (e.key === "Tab" && !e.shiftKey && !e.ctrlKey && !e.altKey) {
        const target = e.target as HTMLElement
        // Don't capture Tab if in an input
        if (target.tagName === "INPUT" || target.tagName === "TEXTAREA") return

        e.preventDefault()
        const currentIdx = AGENTS.findIndex(a => a.id === props.current)
        const nextIdx = (currentIdx + 1) % AGENTS.length
        props.onChange(AGENTS[nextIdx].id)
      }
    })
  }

  return (
    <div class="relative">
      {/* Current agent button */}
      <button
        onClick={() => setIsOpen(!isOpen())}
        class="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-background-raised
               hover:bg-background-overlay border border-border-base transition-colors"
      >
        <svg class={`w-4 h-4 ${currentAgent().color}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={currentAgent().icon} />
        </svg>
        <span class="text-sm font-medium text-text-base">{currentAgent().name}</span>
        <span class="text-xs text-text-weaker px-1.5 py-0.5 bg-background-base rounded">Tab</span>
        <svg class="w-3 h-3 text-text-weaker" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {/* Dropdown */}
      {isOpen() && (
        <>
          {/* Backdrop */}
          <div class="fixed inset-0 z-10" onClick={() => setIsOpen(false)} />

          {/* Menu */}
          <div class="absolute top-full left-0 mt-1 w-64 py-1 bg-background-raised border border-border-base
                      rounded-lg shadow-lg z-20">
            <For each={AGENTS}>
              {(agent) => (
                <button
                  onClick={() => selectAgent(agent.id)}
                  classList={{
                    "w-full px-3 py-2 flex items-start gap-3 hover:bg-background-overlay transition-colors": true,
                    "bg-background-overlay": agent.id === props.current
                  }}
                >
                  <svg class={`w-5 h-5 mt-0.5 ${agent.color}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={agent.icon} />
                  </svg>
                  <div class="text-left">
                    <div class="text-sm font-medium text-text-base">{agent.name}</div>
                    <div class="text-xs text-text-weaker">{agent.description}</div>
                  </div>
                  {agent.id === props.current && (
                    <svg class="w-4 h-4 ml-auto text-accent-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                    </svg>
                  )}
                </button>
              )}
            </For>
          </div>
        </>
      )}
    </div>
  )
}
