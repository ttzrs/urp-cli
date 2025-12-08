import { createSignal, For, Show, onMount, createEffect } from "solid-js"
import { useUrp } from "../context/urp"
import { MessageView, parseMessageContent } from "../components/message-view"

interface MessagePart {
  type: "text" | "code" | "tool"
  content?: string
  language?: string
  tool?: {
    name: string
    input: string
    output?: string
    status: "pending" | "running" | "done" | "error"
  }
}

interface Message {
  id: string
  role: "user" | "assistant"
  parts: MessagePart[]
  timestamp: number
}

export default function Session() {
  const urp = useUrp()
  const [prompt, setPrompt] = createSignal("")
  const [messages, setMessages] = createSignal<Message[]>([])
  const [isStreaming, setIsStreaming] = createSignal(false)
  let inputRef!: HTMLTextAreaElement
  let messagesRef!: HTMLDivElement

  onMount(() => {
    inputRef?.focus()
  })

  // Auto-resize textarea
  function autoResize(el: HTMLTextAreaElement) {
    el.style.height = "auto"
    el.style.height = Math.min(el.scrollHeight, 200) + "px"
  }

  async function handleSubmit(e?: Event) {
    e?.preventDefault()
    const text = prompt().trim()
    if (!text || isStreaming()) return

    // Add user message
    const userMsg: Message = {
      id: crypto.randomUUID(),
      role: "user",
      parts: [{ type: "text", content: text }],
      timestamp: Date.now()
    }
    setMessages(prev => [...prev, userMsg])
    setPrompt("")

    // Reset textarea height
    if (inputRef) {
      inputRef.style.height = "auto"
    }

    // Add placeholder for assistant response
    const assistantId = crypto.randomUUID()
    const assistantMsg: Message = {
      id: assistantId,
      role: "assistant",
      parts: [{ type: "text", content: "" }],
      timestamp: Date.now()
    }
    setMessages(prev => [...prev, assistantMsg])
    setIsStreaming(true)

    // Scroll to bottom
    scrollToBottom()

    // Stream response
    let fullContent = ""
    try {
      await urp.sendPromptStream(text, (chunk) => {
        fullContent += chunk + "\n"

        // Parse content into parts
        const parts = parseMessageContent(fullContent)

        setMessages(prev => prev.map(msg =>
          msg.id === assistantId
            ? { ...msg, parts }
            : msg
        ))

        scrollToBottom()
      })
    } catch (err) {
      // Error is handled by urp context
    } finally {
      setIsStreaming(false)
    }
  }

  function scrollToBottom() {
    setTimeout(() => {
      messagesRef?.scrollTo({ top: messagesRef.scrollHeight, behavior: "smooth" })
    }, 50)
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      handleSubmit()
    }
  }

  function clearChat() {
    setMessages([])
  }

  return (
    <div class="h-full flex flex-col">
      {/* Messages area */}
      <div ref={messagesRef} class="flex-1 overflow-y-auto">
        <Show
          when={messages().length > 0}
          fallback={<EmptyState />}
        >
          <div class="max-w-4xl mx-auto p-6 space-y-6">
            <For each={messages()}>
              {(msg) => <MessageView message={msg} />}
            </For>

            {/* Streaming indicator */}
            <Show when={isStreaming()}>
              <div class="flex items-center gap-2 text-text-weaker">
                <div class="flex gap-1">
                  <span class="w-2 h-2 bg-accent-primary rounded-full animate-bounce" style={{ "animation-delay": "0ms" }} />
                  <span class="w-2 h-2 bg-accent-primary rounded-full animate-bounce" style={{ "animation-delay": "150ms" }} />
                  <span class="w-2 h-2 bg-accent-primary rounded-full animate-bounce" style={{ "animation-delay": "300ms" }} />
                </div>
                <span class="text-sm">Thinking...</span>
              </div>
            </Show>
          </div>
        </Show>
      </div>

      {/* Input area */}
      <div class="border-t border-border-base bg-background-raised">
        <div class="max-w-4xl mx-auto p-4">
          <form onSubmit={handleSubmit} class="relative">
            {/* Textarea container */}
            <div class="relative rounded-xl border border-border-base bg-background-base
                        focus-within:border-accent-primary focus-within:ring-1 focus-within:ring-accent-primary/50
                        transition-all">
              <textarea
                id="prompt-input"
                ref={inputRef}
                value={prompt()}
                onInput={(e) => {
                  setPrompt(e.currentTarget.value)
                  autoResize(e.currentTarget)
                }}
                onKeyDown={handleKeyDown}
                placeholder="Ask anything... (Ctrl+Enter to send)"
                disabled={isStreaming()}
                class="w-full px-4 py-3 pr-24 bg-transparent text-text-base placeholder:text-text-weaker
                       resize-none focus:outline-none disabled:opacity-50"
                rows={1}
              />

              {/* Send button */}
              <div class="absolute right-2 bottom-2 flex items-center gap-2">
                <Show when={messages().length > 0}>
                  <button
                    type="button"
                    onClick={clearChat}
                    class="p-2 rounded-lg text-text-weaker hover:text-text-base hover:bg-background-overlay
                           transition-colors"
                    title="Clear chat"
                  >
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                        d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                  </button>
                </Show>

                <button
                  type="submit"
                  disabled={!prompt().trim() || isStreaming()}
                  class="flex items-center gap-2 px-4 py-2 bg-accent-primary text-white rounded-lg
                         font-medium hover:bg-accent-secondary disabled:opacity-50 disabled:cursor-not-allowed
                         transition-colors"
                >
                  <Show
                    when={!isStreaming()}
                    fallback={
                      <svg class="w-5 h-5 animate-spin" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                      </svg>
                    }
                  >
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                        d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
                    </svg>
                  </Show>
                  Send
                </button>
              </div>
            </div>

            {/* Quick actions */}
            <div class="flex items-center gap-2 mt-2 text-xs text-text-weaker">
              <button
                type="button"
                onClick={() => { setPrompt("urp sys vitals"); setTimeout(handleSubmit, 50) }}
                class="px-2 py-1 rounded hover:bg-background-overlay transition-colors"
              >
                /vitals
              </button>
              <button
                type="button"
                onClick={() => { setPrompt("urp code stats"); setTimeout(handleSubmit, 50) }}
                class="px-2 py-1 rounded hover:bg-background-overlay transition-colors"
              >
                /stats
              </button>
              <button
                type="button"
                onClick={() => { setPrompt("urp think wisdom"); setTimeout(handleSubmit, 50) }}
                class="px-2 py-1 rounded hover:bg-background-overlay transition-colors"
              >
                /wisdom
              </button>
              <button
                type="button"
                onClick={() => { setPrompt("urp events errors"); setTimeout(handleSubmit, 50) }}
                class="px-2 py-1 rounded hover:bg-background-overlay transition-colors"
              >
                /errors
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}

function EmptyState() {
  return (
    <div class="flex flex-col items-center justify-center h-full text-center p-8">
      {/* Logo */}
      <div class="w-20 h-20 rounded-2xl bg-gradient-to-br from-accent-primary/20 to-purple-500/20
                  flex items-center justify-center mb-6">
        <svg class="w-10 h-10 text-accent-primary" viewBox="0 0 24 24" fill="currentColor">
          <path d="M13 3L4 14h7l-2 7 9-11h-7l2-7z" />
        </svg>
      </div>

      <h1 class="text-2xl font-bold text-text-strong mb-2">URP Agent</h1>
      <p class="text-text-weak max-w-md mb-8">
        Embodied Agent Protocol with persistent memory, graph-backed context, and multi-agent orchestration.
      </p>

      {/* Feature grid */}
      <div class="grid grid-cols-2 gap-4 max-w-lg mb-8">
        <FeatureCard
          icon="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"
          title="Focus"
          description="Load context from graph with configurable depth"
        />
        <FeatureCard
          icon="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"
          title="Wisdom"
          description="Query learned solutions from similar problems"
        />
        <FeatureCard
          icon="M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z"
          title="Novelty"
          description="Detect when writing unusual patterns"
        />
        <FeatureCard
          icon="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4"
          title="Memory"
          description="Persistent graph + vector memory"
        />
      </div>

      {/* Quick commands */}
      <div class="text-sm text-text-weak">
        Try: <code class="px-2 py-1 bg-background-raised rounded mx-1">urp focus main.go</code>
        or <code class="px-2 py-1 bg-background-raised rounded mx-1">urp think wisdom "error handling"</code>
      </div>
    </div>
  )
}

function FeatureCard(props: { icon: string; title: string; description: string }) {
  return (
    <div class="p-4 rounded-xl bg-background-raised border border-border-base text-left">
      <svg class="w-6 h-6 text-accent-primary mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={props.icon} />
      </svg>
      <h3 class="text-sm font-semibold text-text-strong mb-1">{props.title}</h3>
      <p class="text-xs text-text-weak">{props.description}</p>
    </div>
  )
}
